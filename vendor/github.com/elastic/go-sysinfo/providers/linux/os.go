// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package linux

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/elastic/go-sysinfo/types"
)

const (
	osRelease       = "/etc/os-release"
	lsbRelease      = "/etc/lsb-release"
	distribRelease  = "/etc/*-release"
	versionGrok     = `(?P<version>(?P<major>[0-9]+)\.?(?P<minor>[0-9]+)?\.?(?P<patch>\w+)?)(?: \((?P<codename>[-\w ]+)\))?`
	versionGrokSuse = `(?P<version>(?P<major>[0-9]+)(?:[.-]?(?:SP)?(?P<minor>[0-9]+))?(?:[.-](?P<patch>[0-9]+|\w+))?)(?: \((?P<codename>[-\w ]+)\))?`
)

var (
	// distribReleaseRegexp parses the /etc/<distrib>-release file. See man lsb-release.
	distribReleaseRegexp = regexp.MustCompile(`(?P<name>[\w]+).* ` + versionGrok)

	// versionRegexp parses version numbers (e.g. 6 or 6.1 or 6.1.0 or 6.1.0_20150102).
	versionRegexp = regexp.MustCompile(versionGrok)

	// versionRegexpSuse parses version numbers for SUSE (e.g. 15-SP1).
	versionRegexpSuse = regexp.MustCompile(versionGrokSuse)
)

// familyMap contains a mapping of family -> []platforms.
var familyMap = map[string][]string{
	"alpine": {"alpine"},
	"arch":   {"arch", "antergos", "manjaro"},
	"redhat": {
		"redhat", "fedora", "centos", "scientific", "oraclelinux", "ol",
		"amzn", "rhel", "almalinux", "openeuler", "rocky",
	},
	"debian": {"debian", "ubuntu", "raspbian", "linuxmint"},
	"suse":   {"suse", "sles", "opensuse"},
}

var platformToFamilyMap map[string]string

func init() {
	platformToFamilyMap = map[string]string{}
	for family, platformList := range familyMap {
		for _, platform := range platformList {
			platformToFamilyMap[platform] = family
		}
	}
}

// OperatingSystem returns OS info. This does not take an alternate hostfs.
// to get OS info from an alternate root path, use reader.os()
func OperatingSystem() (*types.OSInfo, error) {
	return getOSInfo("")
}

func getOSInfo(baseDir string) (*types.OSInfo, error) {
	osInfo, err := getOSRelease(baseDir)
	if err != nil {
		// Fallback
		return findDistribRelease(baseDir)
	}

	// For the redhat family, enrich version info with data from
	// /etc/[distrib]-release because the minor and patch info isn't always
	// present in os-release.
	if osInfo.Family != "redhat" {
		return osInfo, nil
	}

	distInfo, err := findDistribRelease(baseDir)
	if err != nil {
		return osInfo, err
	}
	osInfo.Major = distInfo.Major
	osInfo.Minor = distInfo.Minor
	osInfo.Patch = distInfo.Patch
	osInfo.Codename = distInfo.Codename
	return osInfo, nil
}

func getOSRelease(baseDir string) (*types.OSInfo, error) {
	lsbRel, _ := os.ReadFile(filepath.Join(baseDir, lsbRelease))

	osRel, err := os.ReadFile(filepath.Join(baseDir, osRelease))
	if err != nil {
		return nil, err
	}
	if len(osRel) == 0 {
		return nil, fmt.Errorf("%v is empty: %w", osRelease, err)
	}

	return parseOSRelease(append(lsbRel, osRel...))
}

func parseOSRelease(content []byte) (*types.OSInfo, error) {
	fields := map[string]string{}

	s := bufio.NewScanner(bytes.NewReader(content))
	for s.Scan() {
		line := bytes.TrimSpace(s.Bytes())

		// Skip blank lines and comments.
		if len(line) == 0 || bytes.HasPrefix(line, []byte("#")) {
			continue
		}

		parts := bytes.SplitN(s.Bytes(), []byte("="), 2)
		if len(parts) != 2 {
			continue
		}

		key := string(bytes.TrimSpace(parts[0]))
		val := string(bytes.TrimSpace(parts[1]))
		fields[key] = val

		// Trim quotes.
		val, err := strconv.Unquote(val)
		if err == nil {
			fields[key] = strings.TrimSpace(val)
		}
	}

	if s.Err() != nil {
		return nil, s.Err()
	}

	return makeOSInfo(fields)
}

func makeOSInfo(osRelease map[string]string) (*types.OSInfo, error) {
	os := &types.OSInfo{
		Type:     "linux",
		Platform: firstOf(osRelease, "ID", "DISTRIB_ID"),
		Name:     firstOf(osRelease, "NAME", "PRETTY_NAME"),
		Version:  firstOf(osRelease, "VERSION", "VERSION_ID", "DISTRIB_RELEASE"),
		Build:    osRelease["BUILD_ID"],
		Codename: firstOf(osRelease, "VERSION_CODENAME", "DISTRIB_CODENAME"),
	}

	if os.Codename == "" {
		// Some OSes use their own CODENAME keys (e.g UBUNTU_CODENAME).
		for k, v := range osRelease {
			if strings.Contains(k, "CODENAME") {
				os.Codename = v
				break
			}
		}
	}

	if os.Platform == "" {
		// Fallback to the first word of the Name field.
		os.Platform, _, _ = strings.Cut(os.Name, " ")
	}

	os.Family = linuxFamily(os.Platform)
	if os.Family == "" {
		// ID_LIKE is a space-separated list of OS identifiers that this
		// OS is similar to. Use this to figure out the Linux family.
		for _, id := range strings.Fields(osRelease["ID_LIKE"]) {
			os.Family = linuxFamily(id)
			if os.Family != "" {
				break
			}
		}
	}

	if osRelease["ID_LIKE"] == "suse" {
		extractVersionDetails(os, os.Version, versionRegexpSuse)
	} else if os.Version != "" {
		extractVersionDetails(os, os.Version, versionRegexp)
	}

	return os, nil
}

func extractVersionDetails(os *types.OSInfo, version string, re *regexp.Regexp) {
	keys := re.SubexpNames()
	for i, match := range re.FindStringSubmatch(version) {
		switch keys[i] {
		case "major":
			os.Major, _ = strconv.Atoi(match)
		case "minor":
			os.Minor, _ = strconv.Atoi(match)
		case "patch":
			os.Patch, _ = strconv.Atoi(match)
		case "codename":
			if os.Codename == "" {
				os.Codename = match
			}
		}
	}
}

func findDistribRelease(baseDir string) (*types.OSInfo, error) {
	matches, err := filepath.Glob(filepath.Join(baseDir, distribRelease))
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, path := range matches {
		if strings.HasSuffix(path, osRelease) || strings.HasSuffix(path, lsbRelease) {
			continue
		}

		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Size() == 0 {
			continue
		}

		osInfo, err := getDistribRelease(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("in %s: %w", path, err))
			continue
		}
		return osInfo, nil
	}
	return nil, fmt.Errorf("no valid /etc/<distrib>-release file found: %w", errors.Join(errs...))
}

func getDistribRelease(file string) (*types.OSInfo, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	parts := bytes.SplitN(data, []byte("\n"), 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("failed to parse %v", file)
	}

	// Use distrib as platform name.
	var platform string
	if parts := strings.SplitN(filepath.Base(file), "-", 2); len(parts) > 0 {
		platform = strings.ToLower(parts[0])
	}

	return parseDistribRelease(platform, parts[0])
}

func parseDistribRelease(platform string, content []byte) (*types.OSInfo, error) {
	var (
		line = string(bytes.TrimSpace(content))
		keys = distribReleaseRegexp.SubexpNames()
		os   = &types.OSInfo{
			Type:     "linux",
			Platform: platform,
		}
	)

	for i, m := range distribReleaseRegexp.FindStringSubmatch(line) {
		switch keys[i] {
		case "name":
			os.Name = m
		case "version":
			os.Version = m
		case "major":
			os.Major, _ = strconv.Atoi(m)
		case "minor":
			os.Minor, _ = strconv.Atoi(m)
		case "patch":
			os.Patch, _ = strconv.Atoi(m)
		case "codename":
			os.Version += " (" + m + ")"
			os.Codename = m
		}
	}

	os.Family = linuxFamily(os.Platform)
	return os, nil
}

// firstOf returns the first non-empty value found in the map while
// iterating over keys.
func firstOf(kv map[string]string, keys ...string) string {
	for _, key := range keys {
		if v := kv[key]; v != "" {
			return v
		}
	}
	return ""
}

// linuxFamily returns the linux distribution family associated to the OS platform.
// If there is no family associated then it returns an empty string.
func linuxFamily(platform string) string {
	if platform == "" {
		return ""
	}

	platform = strings.ToLower(platform)

	// First try a direct lookup.
	if family, found := platformToFamilyMap[platform]; found {
		return family
	}

	// Try prefix matching (e.g. opensuse matches opensuse-tumpleweed).
	for platformPrefix, family := range platformToFamilyMap {
		if strings.HasPrefix(platform, platformPrefix) {
			return family
		}
	}
	return ""
}
