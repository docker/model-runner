package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestRemoveEngineOnlyCommands(t *testing.T) {
	root := &cobra.Command{Use: "dmr"}
	for _, name := range append([]string{"run", "ls", "serve"}, engineOnlyCommands...) {
		root.AddCommand(&cobra.Command{Use: name})
	}

	removeEngineOnlyCommands(root)

	remaining := make(map[string]bool)
	for _, cmd := range root.Commands() {
		remaining[cmd.Name()] = true
	}

	for _, name := range engineOnlyCommands {
		if remaining[name] {
			t.Errorf("expected engine-only command %q to be removed", name)
		}
	}

	for _, name := range []string{"run", "ls", "serve"} {
		if !remaining[name] {
			t.Errorf("expected command %q to remain", name)
		}
	}
}

// TestServePersistentPreRunEOverridesParent locks in the cobra behavior that
// newServeCmd() relies on: a child command's own PersistentPreRunE replaces
// (rather than chains after) its parent's, so attaching "serve" — with its
// no-op PersistentPreRunE — to the root command built by commands.NewRootCmd
// is sufficient on its own to skip that root's Docker CLI plugin
// initialization. See https://pkg.go.dev/github.com/spf13/cobra#Command,
// "Persistent*Run functions will be inherited by children if they do not
// declare their own."
func TestServePersistentPreRunEOverridesParent(t *testing.T) {
	var parentRan, childRan bool

	root := &cobra.Command{
		Use: "dmr",
		PersistentPreRunE: func(*cobra.Command, []string) error {
			parentRan = true
			return nil
		},
	}

	serve := newServeCmd()
	// Swap out the real RunE (which starts the daemon) for a no-op so this
	// stays a pure command-tree test.
	serve.RunE = func(*cobra.Command, []string) error {
		childRan = true
		return nil
	}
	root.AddCommand(serve)

	root.SetArgs([]string{"serve"})
	if err := root.Execute(); err != nil {
		t.Fatalf("root.Execute() error = %v", err)
	}

	if parentRan {
		t.Error("expected root's PersistentPreRunE NOT to run for \"serve\", but it did")
	}
	if !childRan {
		t.Error("expected serve's RunE to run, but it didn't")
	}
}
