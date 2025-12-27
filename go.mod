module github.com/docker/model-runner

go 1.24.3

require (
	github.com/containerd/containerd/v2 v2.2.1
	github.com/containerd/errdefs v1.0.0
	github.com/containerd/platforms v1.0.0-rc.2
	github.com/distribution/reference v0.6.0
	github.com/docker/go-units v0.5.0
	github.com/gpustack/gguf-parser-go v0.22.1
	github.com/jaypipes/ghw v0.21.2
	github.com/kolesnikovae/go-winjob v1.0.0
	github.com/mattn/go-shellwords v1.0.12
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.1
	github.com/prometheus/client_model v0.6.2
	github.com/prometheus/common v0.67.4
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.11.1
	golang.org/x/sync v0.19.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/typeurl/v2 v2.2.3 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/henvic/httpretty v0.1.4 // indirect
	github.com/jaypipes/pcidb v1.1.1 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.18.1 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/smallnest/ringbuffer v0.0.0-20241116012123-461381446e3d // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.62.0 // indirect
	go.opentelemetry.io/otel v1.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.39.0 // indirect
	go.opentelemetry.io/otel/trace v1.39.0 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	golang.org/x/crypto v0.45.0 // indirect
	golang.org/x/exp v0.0.0-20250106191152-7588d65b2ba8 // indirect
	golang.org/x/mod v0.29.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/term v0.38.0 // indirect
	golang.org/x/tools v0.38.0 // indirect
	gonum.org/v1/gonum v0.16.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	howett.net/plist v1.0.2-0.20250314012144-ee69052608d9 // indirect
)

replace github.com/kolesnikovae/go-winjob => github.com/docker/go-winjob v0.0.0-20250829235554-57b487ebcbc5
