package standalone

const (
	// DefaultControllerPortMoby is the default TCP port on which the standalone
	// controller will listen for requests in Moby environments.
	DefaultControllerPortMoby = 12434
	// DefaultControllerPortCloud is the default TCP port on which the
	// standalone controller will listen for requests in Cloud environments.
	DefaultControllerPortCloud = 12435

	// DefaultTLSPortMoby is the default TCP port on which the standalone
	// controller will listen for TLS requests in Moby environments.
	DefaultTLSPortMoby = 12444
	// DefaultTLSPortCloud is the default TCP port on which the standalone
	// controller will listen for TLS requests in Cloud environments.
	DefaultTLSPortCloud = 12445
)
