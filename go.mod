module github.com/docker/model-runner

go 1.25.6

require (
	github.com/containerd/containerd/v2 v2.2.1
	github.com/containerd/errdefs v1.0.0
	github.com/containerd/platforms v1.0.0-rc.2
	github.com/distribution/reference v0.6.0
	github.com/docker/docker-credential-helpers v0.9.5
	github.com/docker/go-units v0.5.0
	github.com/gpustack/gguf-parser-go v0.23.1
	github.com/jaypipes/ghw v0.21.3
	github.com/kolesnikovae/go-winjob v1.0.0
	github.com/mattn/go-shellwords v1.0.12
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.1
	github.com/opencontainers/runtime-spec v1.3.0
	github.com/prometheus/client_model v0.6.2
	github.com/prometheus/common v0.67.5
	github.com/sirupsen/logrus v1.9.4
	github.com/stretchr/testify v1.11.1
	golang.org/x/sync v0.19.0
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/Microsoft/hcsshim v0.14.0-rc.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containerd/cgroups/v3 v3.1.2 // indirect
	github.com/containerd/containerd/api v1.10.0 // indirect
	github.com/containerd/continuity v0.4.5 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/fifo v1.1.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/plugin v1.0.0 // indirect
	github.com/containerd/ttrpc v1.2.7 // indirect
	github.com/containerd/typeurl/v2 v2.2.3 // indirect
	github.com/cyphar/filepath-securejoin v0.5.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/henvic/httpretty v0.1.4 // indirect
	github.com/jaypipes/pcidb v1.1.1 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.18.1 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/sys/mountinfo v0.7.2 // indirect
	github.com/moby/sys/sequential v0.6.0 // indirect
	github.com/moby/sys/signal v0.7.1 // indirect
	github.com/moby/sys/user v0.4.0 // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/opencontainers/selinux v1.13.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/smallnest/ringbuffer v0.0.0-20241116012123-461381446e3d // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.62.0 // indirect
	go.opentelemetry.io/otel v1.40.0 // indirect
	go.opentelemetry.io/otel/metric v1.40.0 // indirect
	go.opentelemetry.io/otel/trace v1.40.0 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/exp v0.0.0-20250106191152-7588d65b2ba8 // indirect
	golang.org/x/mod v0.30.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/term v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	golang.org/x/tools v0.39.0 // indirect
	gonum.org/v1/gonum v0.16.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250804133106-a7a43d27e69b // indirect
	google.golang.org/grpc v1.76.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	howett.net/plist v1.0.2-0.20250314012144-ee69052608d9 // indirect
)

replace github.com/kolesnikovae/go-winjob => github.com/docker/go-winjob v0.0.0-20250829235554-57b487ebcbc5
