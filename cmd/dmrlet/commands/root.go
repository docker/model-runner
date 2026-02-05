// Package commands implements the dmrlet CLI commands.
package commands

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/model-runner/pkg/dmrlet/inference"
	"github.com/docker/model-runner/pkg/dmrlet/models"
	"github.com/docker/model-runner/pkg/dmrlet/runtime"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	verbose bool
	logJSON bool

	// Shared state
	log     *logrus.Entry
	store   *models.Store
	rt      *runtime.Runtime
	manager *inference.Manager
)

// rootCmd is the root command for dmrlet.
var rootCmd = &cobra.Command{
	Use:   "dmrlet",
	Short: "Lightweight node agent for Docker Model Runner",
	Long: `dmrlet is a lightweight node agent for Docker Model Runner - a "Kubelet for AI"
that runs inference containers directly with zero YAML overhead.

Example:
  dmrlet serve ai/smollm2
  # Pulls model, starts inference container, exposes OpenAI API at http://localhost:30000/v1`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip initialization for help and version commands
		if cmd.Name() == "help" || cmd.Name() == "version" {
			return nil
		}

		// Setup logging
		logger := logrus.New()
		if verbose {
			logger.SetLevel(logrus.DebugLevel)
		} else {
			logger.SetLevel(logrus.InfoLevel)
		}
		if logJSON {
			logger.SetFormatter(&logrus.JSONFormatter{})
		}

		// Check DMRLET_LOG_LEVEL environment variable
		if level := os.Getenv("DMRLET_LOG_LEVEL"); level != "" {
			if lvl, err := logrus.ParseLevel(level); err == nil {
				logger.SetLevel(lvl)
			}
		}

		log = logger.WithField("component", "dmrlet")

		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command.
func Execute() error {
	// Setup context with signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return rootCmd.ExecuteContext(ctx)
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&logJSON, "log-json", false, "Output logs in JSON format")

	rootCmd.AddCommand(
		newServeCmd(),
		newStopCmd(),
		newListCmd(),
		newPullCmd(),
		newVersionCmd(),
	)
}

// initStore initializes the model store.
func initStore() error {
	if store != nil {
		return nil
	}

	var err error
	store, err = models.NewStore(
		models.WithLogger(log),
	)
	if err != nil {
		return err
	}
	return nil
}

// initRuntime initializes the containerd runtime.
func initRuntime(ctx context.Context) error {
	if rt != nil {
		return nil
	}

	var err error
	rt, err = runtime.NewRuntime(ctx,
		runtime.WithRuntimeLogger(log),
	)
	if err != nil {
		return err
	}
	return nil
}

// initManager initializes the inference manager.
func initManager(ctx context.Context) error {
	if err := initStore(); err != nil {
		return err
	}
	if err := initRuntime(ctx); err != nil {
		return err
	}

	if manager == nil {
		manager = inference.NewManager(store, rt,
			inference.WithManagerLogger(log),
		)
	}
	return nil
}
