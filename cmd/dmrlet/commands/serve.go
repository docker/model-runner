package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/model-runner/pkg/dmrlet/inference"
	"github.com/spf13/cobra"
)

type serveFlags struct {
	port    int
	backend string
	gpu     bool
	detach  bool
}

func newServeCmd() *cobra.Command {
	flags := &serveFlags{}

	cmd := &cobra.Command{
		Use:   "serve MODEL",
		Short: "Serve a model (pull if needed, start container, wait for ready)",
		Long: `Serve a model by pulling it if needed, starting an inference container,
and waiting for it to be ready. The model will be exposed on an OpenAI-compatible API.

Examples:
  dmrlet serve ai/smollm2
  dmrlet serve ai/smollm2 --port 8080
  dmrlet serve ai/smollm2 --gpu
  dmrlet serve ai/smollm2 --backend vllm --gpu
  dmrlet serve ai/smollm2 -d  # detached mode`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd, args[0], flags)
		},
	}

	cmd.Flags().IntVarP(&flags.port, "port", "p", 0, "Port to expose the API on (auto-allocated if not specified)")
	cmd.Flags().StringVarP(&flags.backend, "backend", "b", "", "Inference backend (llama-server, vllm)")
	cmd.Flags().BoolVar(&flags.gpu, "gpu", false, "Enable GPU support")
	cmd.Flags().BoolVarP(&flags.detach, "detach", "d", false, "Run in detached mode (return immediately)")

	return cmd
}

func runServe(cmd *cobra.Command, modelRef string, flags *serveFlags) error {
	ctx := cmd.Context()

	if err := initManager(ctx); err != nil {
		return fmt.Errorf("initializing manager: %w", err)
	}

	opts := inference.ServeOptions{
		Port:     flags.port,
		Backend:  flags.backend,
		GPU:      flags.gpu,
		Detach:   flags.detach,
		Progress: os.Stdout,
	}

	running, err := manager.Serve(ctx, modelRef, opts)
	if err != nil {
		return fmt.Errorf("serving model: %w", err)
	}

	cmd.Printf("\nModel %s is ready!\n", modelRef)
	cmd.Printf("Endpoint: %s\n", running.Endpoint)
	cmd.Printf("Backend:  %s\n", running.Backend)
	cmd.Printf("Port:     %d\n", running.Port)
	cmd.Println()
	cmd.Printf("Example usage:\n")
	cmd.Printf("  curl %s/chat/completions -H 'Content-Type: application/json' \\\n", running.Endpoint)
	cmd.Printf("    -d '{\"model\":\"%s\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello!\"}]}'\n", modelRef)

	if flags.detach {
		return nil
	}

	// Wait for interrupt signal
	cmd.Println()
	cmd.Println("Press Ctrl+C to stop the model...")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	cmd.Println()
	cmd.Println("Stopping model...")

	if err := manager.Stop(context.Background(), modelRef); err != nil {
		return fmt.Errorf("stopping model: %w", err)
	}

	cmd.Println("Model stopped.")
	return nil
}
