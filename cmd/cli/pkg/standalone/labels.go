package standalone

const (
	// labelDesktopService is the label used to identify a container or image as
	// a Docker Desktop service component. This causes the object to be hidden
	// from listing requests (unless --filter label=com.docker.desktop.service
	// is specified). This applies to both Docker Desktop and Docker Cloud.
	labelDesktopService = "com.docker.desktop.service"

	// serviceModelRunner is the service label value used to identify model
	// runner components.
	serviceModelRunner = "model-runner"

	// labelRole is the label used to identify a Docker object's role in the
	// standalone model runner infrastructure.
	labelRole = "com.docker.model-runner.role"

	// roleController is the role label value used to identify the model runner
	// controller container.
	roleController = "controller"

	// roleRunner is not currently defined because model runner processes
	// currently execute within the controller container. This may change in a
	// future release.

	// roleModelStorage is the role label value used to identify the model
	// runner model storage volume.
	roleModelStorage = "model-storage"

	// labelRunnerType is the label used to identify the type of runner
	// (e.g., "model-runner" or "ollama").
	labelRunnerType = "com.docker.model-runner.type"

	// runnerTypeModelRunner is the runner type label value for model-runner.
	runnerTypeModelRunner = "model-runner"

	// runnerTypeOllama is the runner type label value for ollama.
	runnerTypeOllama = "ollama"
)
