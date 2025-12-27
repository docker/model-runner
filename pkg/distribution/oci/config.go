package oci

import (
	"encoding/json"
	"io"
	"time"
)

// ConfigFile is the configuration file that holds the metadata describing
// how to launch a container. See:
// https://github.com/opencontainers/image-spec/blob/master/config.md
type ConfigFile struct {
	Architecture  string          `json:"architecture"`
	Author        string          `json:"author,omitempty"`
	Container     string          `json:"container,omitempty"`
	Created       Time            `json:"created,omitempty"`
	DockerVersion string          `json:"docker_version,omitempty"`
	History       []History       `json:"history,omitempty"`
	OS            string          `json:"os"`
	RootFS        RootFS          `json:"rootfs"`
	Config        ContainerConfig `json:"config"`
	OSVersion     string          `json:"os.version,omitempty"`
	Variant       string          `json:"variant,omitempty"`
	OSFeatures    []string        `json:"os.features,omitempty"`
}

// History is one entry of a list recording how this container image was built.
type History struct {
	Author     string `json:"author,omitempty"`
	Created    Time   `json:"created,omitempty"`
	CreatedBy  string `json:"created_by,omitempty"`
	Comment    string `json:"comment,omitempty"`
	EmptyLayer bool   `json:"empty_layer,omitempty"`
}

// Time is a wrapper around time.Time to help with deep copying
type Time struct {
	time.Time
}

// DeepCopyInto creates a deep-copy of the Time value.
func (t *Time) DeepCopyInto(out *Time) {
	*out = *t
}

// RootFS holds the ordered list of file system deltas that comprise the
// container image's root filesystem.
type RootFS struct {
	Type    string `json:"type"`
	DiffIDs []Hash `json:"diff_ids"`
}

// HealthConfig holds configuration settings for the HEALTHCHECK feature.
type HealthConfig struct {
	Test        []string      `json:",omitempty"`
	Interval    time.Duration `json:",omitempty"`
	Timeout     time.Duration `json:",omitempty"`
	StartPeriod time.Duration `json:",omitempty"`
	Retries     int           `json:",omitempty"`
}

// ContainerConfig is the execution parameters configuration.
type ContainerConfig struct {
	AttachStderr    bool                `json:"AttachStderr,omitempty"`
	AttachStdin     bool                `json:"AttachStdin,omitempty"`
	AttachStdout    bool                `json:"AttachStdout,omitempty"`
	Cmd             []string            `json:"Cmd,omitempty"`
	Healthcheck     *HealthConfig       `json:"Healthcheck,omitempty"`
	Domainname      string              `json:"Domainname,omitempty"`
	Entrypoint      []string            `json:"Entrypoint,omitempty"`
	Env             []string            `json:"Env,omitempty"`
	Hostname        string              `json:"Hostname,omitempty"`
	Image           string              `json:"Image,omitempty"`
	Labels          map[string]string   `json:"Labels,omitempty"`
	OnBuild         []string            `json:"OnBuild,omitempty"`
	OpenStdin       bool                `json:"OpenStdin,omitempty"`
	StdinOnce       bool                `json:"StdinOnce,omitempty"`
	Tty             bool                `json:"Tty,omitempty"`
	User            string              `json:"User,omitempty"`
	Volumes         map[string]struct{} `json:"Volumes,omitempty"`
	WorkingDir      string              `json:"WorkingDir,omitempty"`
	ExposedPorts    map[string]struct{} `json:"ExposedPorts,omitempty"`
	ArgsEscaped     bool                `json:"ArgsEscaped,omitempty"`
	NetworkDisabled bool                `json:"NetworkDisabled,omitempty"`
	MacAddress      string              `json:"MacAddress,omitempty"`
	StopSignal      string              `json:"StopSignal,omitempty"`
	Shell           []string            `json:"Shell,omitempty"`
}

// ParseConfigFile parses the io.Reader's contents into a ConfigFile.
func ParseConfigFile(r io.Reader) (*ConfigFile, error) {
	cf := ConfigFile{}
	if err := json.NewDecoder(r).Decode(&cf); err != nil {
		return nil, err
	}
	return &cf, nil
}
