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
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/procfs"

	"github.com/elastic/go-sysinfo/types"
)

const userHz = 100

// Processes returns a list of processes on the system
func (s linuxSystem) Processes() ([]types.Process, error) {
	procs, err := s.procFS.AllProcs()
	if err != nil {
		return nil, fmt.Errorf("error fetching all processes: %w", err)
	}

	processes := make([]types.Process, 0, len(procs))
	for _, proc := range procs {
		processes = append(processes, &process{Proc: proc, fs: s.procFS})
	}
	return processes, nil
}

// Process returns the given process
func (s linuxSystem) Process(pid int) (types.Process, error) {
	proc, err := s.procFS.Proc(pid)
	if err != nil {
		return nil, fmt.Errorf("error fetching process: %w", err)
	}

	return &process{Proc: proc, fs: s.procFS}, nil
}

// Self returns process info for the caller's own PID
func (s linuxSystem) Self() (types.Process, error) {
	proc, err := s.procFS.Self()
	if err != nil {
		return nil, fmt.Errorf("error fetching self process info: %w", err)
	}

	return &process{Proc: proc, fs: s.procFS}, nil
}

type process struct {
	procfs.Proc
	fs   procFS
	info *types.ProcessInfo
}

// PID returns the PID of the process
func (p *process) PID() int {
	return p.Proc.PID
}

// Parent returns the parent process
func (p *process) Parent() (types.Process, error) {
	info, err := p.Info()
	if err != nil {
		return nil, fmt.Errorf("error fetching process info: %w", err)
	}

	proc, err := p.fs.Proc(info.PPID)
	if err != nil {
		return nil, fmt.Errorf("error fetching data for parent process: %w", err)
	}

	return &process{Proc: proc, fs: p.fs}, nil
}

func (p *process) path(pa ...string) string {
	return p.fs.path(append([]string{strconv.Itoa(p.PID())}, pa...)...)
}

// CWD returns the current working directory
func (p *process) CWD() (string, error) {
	cwd, err := os.Readlink(p.path("cwd"))
	if os.IsNotExist(err) {
		return "", nil
	}

	return cwd, err
}

// Info returns basic process info
func (p *process) Info() (types.ProcessInfo, error) {
	if p.info != nil {
		return *p.info, nil
	}

	stat, err := p.Stat()
	if err != nil {
		return types.ProcessInfo{}, fmt.Errorf("error fetching process stats: %w", err)
	}

	exe, err := p.Executable()
	if err != nil {
		return types.ProcessInfo{}, fmt.Errorf("error fetching process executable info: %w", err)
	}

	args, err := p.CmdLine()
	if err != nil {
		return types.ProcessInfo{}, fmt.Errorf("error fetching process cmdline: %w", err)
	}

	cwd, err := p.CWD()
	if err != nil {
		return types.ProcessInfo{}, fmt.Errorf("error fetching process CWD: %w", err)
	}

	bootTime, err := bootTime(p.fs.FS)
	if err != nil {
		return types.ProcessInfo{}, fmt.Errorf("error fetching boot time: %w", err)
	}

	p.info = &types.ProcessInfo{
		Name:      stat.Comm,
		PID:       p.PID(),
		PPID:      stat.PPID,
		CWD:       cwd,
		Exe:       exe,
		Args:      args,
		StartTime: bootTime.Add(ticksToDuration(stat.Starttime)),
	}

	return *p.info, nil
}

// Memory returns memory stats for the process
func (p *process) Memory() (types.MemoryInfo, error) {
	stat, err := p.Stat()
	if err != nil {
		return types.MemoryInfo{}, err
	}

	return types.MemoryInfo{
		Resident: uint64(stat.ResidentMemory()),
		Virtual:  uint64(stat.VirtualMemory()),
	}, nil
}

// CPUTime returns CPU usage time for the process
func (p *process) CPUTime() (types.CPUTimes, error) {
	stat, err := p.Stat()
	if err != nil {
		return types.CPUTimes{}, err
	}

	return types.CPUTimes{
		User:   ticksToDuration(uint64(stat.UTime)),
		System: ticksToDuration(uint64(stat.STime)),
	}, nil
}

// OpenHandles returns the list of open file descriptors of the process.
func (p *process) OpenHandles() ([]string, error) {
	return p.Proc.FileDescriptorTargets()
}

// OpenHandles returns the number of open file descriptors of the process.
func (p *process) OpenHandleCount() (int, error) {
	return p.Proc.FileDescriptorsLen()
}

// Environment returns a list of environment variables for the process
func (p *process) Environment() (map[string]string, error) {
	// TODO: add Environment to procfs
	content, err := os.ReadFile(p.path("environ"))
	if err != nil {
		return nil, err
	}

	env := map[string]string{}
	pairs := bytes.Split(content, []byte{0})
	for _, kv := range pairs {
		parts := bytes.SplitN(kv, []byte{'='}, 2)
		if len(parts) != 2 {
			continue
		}

		key := string(bytes.TrimSpace(parts[0]))
		if key == "" {
			continue
		}

		env[key] = string(parts[1])
	}

	return env, nil
}

// Seccomp returns seccomp info for the process
func (p *process) Seccomp() (*types.SeccompInfo, error) {
	content, err := os.ReadFile(p.path("status"))
	if err != nil {
		return nil, err
	}

	return readSeccompFields(content)
}

// Capabilities returns capability info for the process
func (p *process) Capabilities() (*types.CapabilityInfo, error) {
	content, err := os.ReadFile(p.path("status"))
	if err != nil {
		return nil, err
	}

	return readCapabilities(content)
}

// User returns user info for the process
func (p *process) User() (types.UserInfo, error) {
	content, err := os.ReadFile(p.path("status"))
	if err != nil {
		return types.UserInfo{}, err
	}

	var user types.UserInfo
	err = parseKeyValue(content, ':', func(key, value []byte) error {
		// See proc(5) for the format of /proc/[pid]/status
		switch string(key) {
		case "Uid":
			ids := strings.Split(string(value), "\t")
			if len(ids) >= 3 {
				user.UID = ids[0]
				user.EUID = ids[1]
				user.SUID = ids[2]
			}
		case "Gid":
			ids := strings.Split(string(value), "\t")
			if len(ids) >= 3 {
				user.GID = ids[0]
				user.EGID = ids[1]
				user.SGID = ids[2]
			}
		}
		return nil
	})
	if err != nil {
		return user, fmt.Errorf("error partsing key-values in user data: %w", err)
	}

	return user, nil
}

// NetworkStats reports network stats for an individual PID.
func (p *process) NetworkCounters() (*types.NetworkCountersInfo, error) {
	snmpRaw, err := os.ReadFile(p.path("net/snmp"))
	if err != nil {
		return nil, fmt.Errorf("error reading net/snmp file: %w", err)
	}
	snmp, err := getNetSnmpStats(snmpRaw)
	if err != nil {
		return nil, fmt.Errorf("error parsing SNMP network data: %w", err)
	}

	netstatRaw, err := os.ReadFile(p.path("net/netstat"))
	if err != nil {
		return nil, fmt.Errorf("error reading net/netstat file: %w", err)
	}
	netstat, err := getNetstatStats(netstatRaw)
	if err != nil {
		return nil, fmt.Errorf("error parsing netstat file: %w", err)
	}

	return &types.NetworkCountersInfo{SNMP: snmp, Netstat: netstat}, nil
}

func ticksToDuration(ticks uint64) time.Duration {
	seconds := float64(ticks) / float64(userHz) * float64(time.Second)
	return time.Duration(int64(seconds))
}
