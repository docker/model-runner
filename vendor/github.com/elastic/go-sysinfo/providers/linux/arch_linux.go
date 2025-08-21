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
	"fmt"
	"os"
	"strings"
	"syscall"
)

const (
	procSysKernelArch = "/proc/sys/kernel/arch"
	procVersion       = "/proc/version"
	arch8664          = "x86_64"
	archAmd64         = "amd64"
	archArm64         = "arm64"
	archAarch64       = "aarch64"
)

func Architecture() (string, error) {
	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err != nil {
		return "", fmt.Errorf("architecture: %w", err)
	}

	data := make([]byte, 0, len(uname.Machine))
	for _, v := range uname.Machine {
		if v == 0 {
			break
		}
		data = append(data, byte(v))
	}

	return string(data), nil
}

func NativeArchitecture() (string, error) {
	// /proc/sys/kernel/arch was introduced in Kernel 6.1
	// https://www.kernel.org/doc/html/v6.1/admin-guide/sysctl/kernel.html#arch
	// It's the same as uname -m, except that for a process running in emulation
	// machine returned from syscall reflects the emulated machine, whilst /proc
	// filesystem is read as file so its value is not emulated
	data, err := os.ReadFile(procSysKernelArch)
	if err != nil {
		if os.IsNotExist(err) {
			// fallback to checking version string for older kernels
			version, err := os.ReadFile(procVersion)
			if err != nil && !os.IsNotExist(err) {
				return "", fmt.Errorf("failed to read kernel version: %w", err)
			}

			versionStr := string(version)
			if strings.Contains(versionStr, archAmd64) || strings.Contains(versionStr, arch8664) {
				return archAmd64, nil
			} else if strings.Contains(versionStr, archArm64) || strings.Contains(versionStr, archAarch64) {
				// for parity with Architecture() and /proc/sys/kernel/arch
				// as aarch64 and arm64 are used interchangeably
				return archAarch64, nil
			}
			return "", nil
		}

		return "", fmt.Errorf("failed to read kernel arch: %w", err)
	}

	nativeArch := string(data)
	nativeArch = strings.TrimRight(nativeArch, "\n")

	return nativeArch, nil
}
