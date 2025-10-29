package standalone

// StatusPrinter is the interface used to print status updates.
type StatusPrinter interface {
	// Printf should perform formatted printing.
	Printf(format string, args ...any)
	// Println should perform line-based printing.
	Println(args ...any)
}

// noopPrinter is used to silence auto-install progress if desired.
type noopPrinter struct{}

// Printf implements StatusPrinter.Printf.
func (*noopPrinter) Printf(format string, args ...any) {}

// Println implements StatusPrinter.Println.
func (*noopPrinter) Println(args ...any) {}

// NoopPrinter returns a StatusPrinter that does nothing.
func NoopPrinter() StatusPrinter {
	return &noopPrinter{}
}
