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
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/prometheus/procfs"

	"github.com/elastic/go-sysinfo/internal/registry"
	"github.com/elastic/go-sysinfo/providers/shared"
	"github.com/elastic/go-sysinfo/types"
)

func init() {
	// register wrappers that implement the HostFS versions of the ProcessProvider and HostProvider
	registry.Register(func(opts registry.ProviderOptions) registry.HostProvider { return newLinuxSystem(opts.Hostfs) })
	registry.Register(func(opts registry.ProviderOptions) registry.ProcessProvider { return newLinuxSystem(opts.Hostfs) })
}

type linuxSystem struct {
	procFS procFS
}

func newLinuxSystem(hostFS string) linuxSystem {
	mountPoint := filepath.Join(hostFS, procfs.DefaultMountPoint)
	fs, _ := procfs.NewFS(mountPoint)
	return linuxSystem{
		procFS: procFS{FS: fs, mountPoint: mountPoint, baseMount: hostFS},
	}
}

func (s linuxSystem) Host() (types.Host, error) {
	return newHost(s.procFS)
}

type host struct {
	procFS procFS
	stat   procfs.Stat
	info   types.HostInfo
}

// Info returns host info
func (h *host) Info() types.HostInfo {
	return h.info
}

// Memory returns memory info
func (h *host) Memory() (*types.HostMemoryInfo, error) {
	path := h.procFS.path("meminfo")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading meminfo file %s: %w", path, err)
	}

	return parseMemInfo(content)
}

func (h *host) FQDNWithContext(ctx context.Context) (string, error) {
	return shared.FQDNWithContext(ctx)
}

func (h *host) FQDN() (string, error) {
	return h.FQDNWithContext(context.Background())
}

// VMStat reports data from /proc/vmstat on linux.
func (h *host) VMStat() (*types.VMStatInfo, error) {
	path := h.procFS.path("vmstat")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading vmstat file %s: %w", path, err)
	}

	return parseVMStat(content)
}

// LoadAverage reports data from /proc/loadavg on linux.
func (h *host) LoadAverage() (*types.LoadAverageInfo, error) {
	loadAvg, err := h.procFS.LoadAvg()
	if err != nil {
		return nil, fmt.Errorf("error fetching load averages: %w", err)
	}

	return &types.LoadAverageInfo{
		One:     loadAvg.Load1,
		Five:    loadAvg.Load5,
		Fifteen: loadAvg.Load15,
	}, nil
}

// NetworkCounters reports data from /proc/net on linux
func (h *host) NetworkCounters() (*types.NetworkCountersInfo, error) {
	snmpFile := h.procFS.path("net/snmp")
	snmpRaw, err := os.ReadFile(snmpFile)
	if err != nil {
		return nil, fmt.Errorf("error fetching net/snmp file %s: %w", snmpFile, err)
	}
	snmp, err := getNetSnmpStats(snmpRaw)
	if err != nil {
		return nil, fmt.Errorf("error parsing SNMP stats: %w", err)
	}

	netstatFile := h.procFS.path("net/netstat")
	netstatRaw, err := os.ReadFile(netstatFile)
	if err != nil {
		return nil, fmt.Errorf("error fetching net/netstat file %s: %w", netstatFile, err)
	}
	netstat, err := getNetstatStats(netstatRaw)
	if err != nil {
		return nil, fmt.Errorf("error parsing netstat file: %w", err)
	}

	return &types.NetworkCountersInfo{SNMP: snmp, Netstat: netstat}, nil
}

// CPUTime returns host CPU usage metrics
func (h *host) CPUTime() (types.CPUTimes, error) {
	stat, err := h.procFS.Stat()
	if err != nil {
		return types.CPUTimes{}, fmt.Errorf("error fetching CPU stats: %w", err)
	}

	return types.CPUTimes{
		User:    time.Duration(stat.CPUTotal.User * float64(time.Second)),
		System:  time.Duration(stat.CPUTotal.System * float64(time.Second)),
		Idle:    time.Duration(stat.CPUTotal.Idle * float64(time.Second)),
		IOWait:  time.Duration(stat.CPUTotal.Iowait * float64(time.Second)),
		IRQ:     time.Duration(stat.CPUTotal.IRQ * float64(time.Second)),
		Nice:    time.Duration(stat.CPUTotal.Nice * float64(time.Second)),
		SoftIRQ: time.Duration(stat.CPUTotal.SoftIRQ * float64(time.Second)),
		Steal:   time.Duration(stat.CPUTotal.Steal * float64(time.Second)),
	}, nil
}

func newHost(fs procFS) (*host, error) {
	stat, err := fs.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to read proc stat: %w", err)
	}

	h := &host{stat: stat, procFS: fs}
	r := &reader{}
	r.architecture(h)
	r.nativeArchitecture(h)
	r.bootTime(h)
	r.containerized(h)
	r.hostname(h)
	r.network(h)
	r.kernelVersion(h)
	r.os(h)
	r.time(h)
	r.uniqueID(h)

	return h, r.Err()
}

type reader struct {
	errs []error
}

func (r *reader) addErr(err error) bool {
	if err != nil {
		if !errors.Is(err, types.ErrNotImplemented) {
			r.errs = append(r.errs, err)
		}
		return true
	}
	return false
}

func (r *reader) Err() error {
	if len(r.errs) > 0 {
		return errors.Join(r.errs...)
	}
	return nil
}

func (r *reader) architecture(h *host) {
	v, err := Architecture()
	if r.addErr(err) {
		return
	}
	h.info.Architecture = v
}

func (r *reader) nativeArchitecture(h *host) {
	v, err := NativeArchitecture()
	if r.addErr(err) {
		return
	}
	h.info.NativeArchitecture = v
}

func (r *reader) bootTime(h *host) {
	v, err := bootTime(h.procFS.FS)
	if r.addErr(err) {
		return
	}
	h.info.BootTime = v
}

func (r *reader) containerized(h *host) {
	v, err := IsContainerized()
	if r.addErr(err) {
		return
	}
	h.info.Containerized = &v
}

func (r *reader) hostname(h *host) {
	v, err := os.Hostname()
	if r.addErr(err) {
		return
	}
	h.info.Hostname = v
}

func (r *reader) network(h *host) {
	ips, macs, err := shared.Network()
	if r.addErr(err) {
		return
	}
	h.info.IPs = ips
	h.info.MACs = macs
}

func (r *reader) kernelVersion(h *host) {
	v, err := KernelVersion()
	if r.addErr(err) {
		return
	}
	h.info.KernelVersion = v
}

func (r *reader) os(h *host) {
	v, err := getOSInfo(h.procFS.baseMount)
	if r.addErr(err) {
		return
	}
	h.info.OS = v
}

func (r *reader) time(h *host) {
	h.info.Timezone, h.info.TimezoneOffsetSec = time.Now().Zone()
}

func (r *reader) uniqueID(h *host) {
	v, err := MachineIDHostfs(h.procFS.baseMount)
	if r.addErr(err) {
		return
	}
	h.info.UniqueID = v
}

type procFS struct {
	procfs.FS
	mountPoint string
	baseMount  string
}

func (fs *procFS) path(p ...string) string {
	elem := append([]string{fs.mountPoint}, p...)
	return filepath.Join(elem...)
}
