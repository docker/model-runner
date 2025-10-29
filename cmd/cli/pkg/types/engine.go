package types

// ModelRunnerEngineKind encodes the kind of Docker engine associated with the
// model runner context.
type ModelRunnerEngineKind uint8

const (
	// ModelRunnerEngineKindMoby represents a non-Desktop/Cloud engine on which
	// the Model CLI command is responsible for managing a Model Runner.
	ModelRunnerEngineKindMoby ModelRunnerEngineKind = iota
	// ModelRunnerEngineKindMobyManual represents a non-Desktop/Cloud engine
	// that's explicitly targeted by a MODEL_RUNNER_HOST environment variable on
	// which the user is responsible for managing a Model Runner.
	ModelRunnerEngineKindMobyManual
	// ModelRunnerEngineKindDesktop represents a Docker Desktop engine. It only
	// refers to a Docker Desktop Linux engine, i.e. not a Windows container
	// engine in the case of Docker Desktop for Windows.
	ModelRunnerEngineKindDesktop
	// ModelRunnerEngineKindCloud represents a Docker Cloud engine.
	ModelRunnerEngineKindCloud
)

// String returns a human-readable engine kind description.
func (k ModelRunnerEngineKind) String() string {
	switch k {
	case ModelRunnerEngineKindMoby:
		return "Docker Engine"
	case ModelRunnerEngineKindMobyManual:
		return "Docker Engine (Manual Install)"
	case ModelRunnerEngineKindDesktop:
		return "Docker Desktop"
	case ModelRunnerEngineKindCloud:
		return "Docker Cloud"
	default:
		return "Unknown"
	}
}
