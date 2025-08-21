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

package sysinfo

import (
	"runtime"

	"github.com/elastic/go-sysinfo/internal/registry"
	"github.com/elastic/go-sysinfo/types"

	// Register host and process providers.
	_ "github.com/elastic/go-sysinfo/providers/aix"
	_ "github.com/elastic/go-sysinfo/providers/darwin"
	_ "github.com/elastic/go-sysinfo/providers/linux"
	_ "github.com/elastic/go-sysinfo/providers/windows"
)

type ProviderOption func(*registry.ProviderOptions)

// WithHostFS returns a provider with a custom HostFS root path,
// enabling use of the library from within a container, or an alternate root path on linux.
// For example, WithHostFS("/hostfs") can be used when /hostfs points to the root filesystem of the container host.
// For full functionality, the alternate hostfs should have:
//   - /proc
//   - /var
//   - /etc
func WithHostFS(hostfs string) ProviderOption {
	return func(po *registry.ProviderOptions) {
		po.Hostfs = hostfs
	}
}

// Go returns information about the Go runtime.
func Go() types.GoInfo {
	return types.GoInfo{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		MaxProcs: runtime.GOMAXPROCS(0),
		Version:  runtime.Version(),
	}
}

func applyOptsAndReturnProvider(opts ...ProviderOption) registry.ProviderOptions {
	options := registry.ProviderOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	return options
}

// setupProcessProvider returns a ProcessProvider.
// Most of the exported functions here deal with processes,
// so this just gets wrapped by all the external functions
func setupProcessProvider(opts ...ProviderOption) (registry.ProcessProvider, error) {
	provider := registry.GetProcessProvider(applyOptsAndReturnProvider(opts...))
	if provider == nil {
		return nil, types.ErrNotImplemented
	}
	return provider, nil
}

// Host returns information about host on which this process is running. If
// host information collection is not implemented for this platform then
// types.ErrNotImplemented is returned.
// On Darwin (macOS) a types.ErrNotImplemented is returned with cgo disabled.
func Host(opts ...ProviderOption) (types.Host, error) {
	provider := registry.GetHostProvider(applyOptsAndReturnProvider(opts...))
	if provider == nil {
		return nil, types.ErrNotImplemented
	}
	return provider.Host()
}

// Process returns a types.Process object representing the process associated
// with the given PID. The types.Process object can be used to query information
// about the process.  If process information collection is not implemented for
// this platform then types.ErrNotImplemented is returned.
func Process(pid int, opts ...ProviderOption) (types.Process, error) {
	provider, err := setupProcessProvider(opts...)
	if err != nil {
		return nil, err
	}
	return provider.Process(pid)
}

// Processes return a list of all processes. If process information collection
// is not implemented for this platform then types.ErrNotImplemented is
// returned.
func Processes(opts ...ProviderOption) ([]types.Process, error) {
	provider, err := setupProcessProvider(opts...)
	if err != nil {
		return nil, err
	}
	return provider.Processes()
}

// Self return a types.Process object representing this process. If process
// information collection is not implemented for this platform then
// types.ErrNotImplemented is returned.
func Self(opts ...ProviderOption) (types.Process, error) {
	provider, err := setupProcessProvider(opts...)
	if err != nil {
		return nil, err
	}
	return provider.Self()
}
