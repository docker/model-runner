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

//go:build amd64 || arm64

package darwin

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

const (
	hardwareMIB    = "hw.machine"
	procTranslated = "sysctl.proc_translated"
	archIntel      = "x86_64"
	archApple      = "arm64"
)

func Architecture() (string, error) {
	arch, err := unix.Sysctl(hardwareMIB)
	if err != nil {
		return "", fmt.Errorf("failed to get architecture: %w", err)
	}

	return arch, nil
}

func NativeArchitecture() (string, error) {
	processArch, err := Architecture()
	if err != nil {
		return "", err
	}

	// https://developer.apple.com/documentation/apple-silicon/about-the-rosetta-translation-environment

	translated, err := unix.SysctlUint32(procTranslated)
	if err != nil {
		// macos without Rosetta installed doesn't have sysctl.proc_translated
		if os.IsNotExist(err) {
			return processArch, nil
		}
		return "", fmt.Errorf("failed to read sysctl.proc_translated: %w", err)
	}

	var nativeArch string

	switch translated {
	case 0:
		nativeArch = processArch
	case 1:
		// Rosetta 2 is supported only on Apple silicon
		nativeArch = archApple
	}

	return nativeArch, nil
}
