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

package windows

import (
	"errors"

	"golang.org/x/sys/windows"

	gowindows "github.com/elastic/go-windows"
)

const (
	imageFileMachineAmd64 = 0x8664
	imageFileMachineArm64 = 0xAA64
	archIntel             = "x86_64"
	archArm64             = "arm64"
)

func Architecture() (string, error) {
	systemInfo, err := gowindows.GetNativeSystemInfo()
	if err != nil {
		return "", err
	}

	return systemInfo.ProcessorArchitecture.String(), nil
}

func NativeArchitecture() (string, error) {
	var processMachine, nativeMachine uint16
	// the pseudo handle doesn't need to be closed
	currentProcessHandle := windows.CurrentProcess()

	// IsWow64Process2 was introduced in version 1709 (build 16299 acording to the tables)
	// https://learn.microsoft.com/en-us/windows/release-health/release-information
	// https://learn.microsoft.com/en-us/windows/release-health/windows-server-release-info
	err := windows.IsWow64Process2(currentProcessHandle, &processMachine, &nativeMachine)
	if err != nil {
		if errors.Is(err, windows.ERROR_PROC_NOT_FOUND) {
			major, minor, build := windows.RtlGetNtVersionNumbers()
			if major < 10 || (major == 10 && minor == 0 && build < 16299) {
				return "", nil
			}
		}
		return "", err
	}

	var nativeArch string

	switch nativeMachine {
	case imageFileMachineAmd64:
		// for parity with Architecture() as amd64 and x86_64 are used interchangeably
		nativeArch = archIntel
	case imageFileMachineArm64:
		nativeArch = archArm64
	}

	return nativeArch, nil
}
