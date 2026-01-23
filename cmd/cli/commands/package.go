package commands

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/distribution"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/oci/reference"
	"github.com/docker/model-runner/pkg/distribution/packaging"
	"github.com/docker/model-runner/pkg/distribution/registry"
	"github.com/docker/model-runner/pkg/distribution/tarball"
	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/spf13/cobra"
)

// validateAbsolutePath validates that a path is absolute and returns the cleaned path
func validateAbsolutePath(path, name string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf(
			"%s path must be absolute.\n\n"+
				"See 'docker model package --help' for more information",
			name,
		)
	}
	return filepath.Clean(path), nil
}

func newPackagedCmd() *cobra.Command {
	var opts packageOptions

	c := &cobra.Command{
		Use:   "package (--gguf <path> | --safetensors-dir <path> | --dduf <path> | --from <model>) [--license <path>...] [--mmproj <path>] [--context-size <tokens>] [--push] MODEL",
		Short: "Package a GGUF file, Safetensors directory, DDUF file, or existing model into a Docker model OCI artifact.",
		Long: "Package a GGUF file, Safetensors directory, DDUF file, or existing model into a Docker model OCI artifact, with optional licenses and multimodal projector. The package is sent to the model-runner, unless --push is specified.\n" +
			"When packaging a sharded GGUF model, --gguf should point to the first shard. All shard files should be siblings and should include the index in the file name (e.g. model-00001-of-00015.gguf).\n" +
			"When packaging a Safetensors model, --safetensors-dir should point to a directory containing .safetensors files and config files (*.json, merges.txt). All files will be auto-discovered and config files will be packaged into a tar archive.\n" +
			"When packaging a DDUF file (Diffusers Unified Format), --dduf should point to a .dduf archive file.\n" +
			"When packaging from an existing model using --from, you can modify properties like context size to create a variant of the original model.\n" +
			"For multimodal models, use --mmproj to include a multimodal projector file.",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := requireExactArgs(1, "package", "MODEL")(cmd, args); err != nil {
				return err
			}

			// Validate that exactly one of --gguf, --safetensors-dir, --dduf, or --from is provided (mutually exclusive)
			sourcesProvided := 0
			if opts.ggufPath != "" {
				sourcesProvided++
			}
			if opts.safetensorsDir != "" {
				sourcesProvided++
			}
			if opts.ddufPath != "" {
				sourcesProvided++
			}
			if opts.fromModel != "" {
				sourcesProvided++
			}

			if sourcesProvided == 0 {
				return fmt.Errorf(
					"One of --gguf, --safetensors-dir, --dduf, or --from is required.\n\n" +
						"See 'docker model package --help' for more information",
				)
			}
			if sourcesProvided > 1 {
				return fmt.Errorf(
					"Cannot specify more than one of --gguf, --safetensors-dir, --dduf, or --from. Please use only one source.\n\n" +
						"See 'docker model package --help' for more information",
				)
			}

			// Validate GGUF path if provided
			if opts.ggufPath != "" {
				var err error
				opts.ggufPath, err = validateAbsolutePath(opts.ggufPath, "GGUF")
				if err != nil {
					return err
				}
			}

			// Validate safetensors directory if provided
			if opts.safetensorsDir != "" {
				if !filepath.IsAbs(opts.safetensorsDir) {
					return fmt.Errorf(
						"Safetensors directory path must be absolute.\n\n" +
							"See 'docker model package --help' for more information",
					)
				}
				opts.safetensorsDir = filepath.Clean(opts.safetensorsDir)

				// Check if it's a directory
				info, err := os.Stat(opts.safetensorsDir)
				if err != nil {
					if os.IsNotExist(err) {
						return fmt.Errorf(
							"Safetensors directory does not exist: %s\n\n"+
								"See 'docker model package --help' for more information",
							opts.safetensorsDir,
						)
					}
					return fmt.Errorf("could not access safetensors directory %q: %w", opts.safetensorsDir, err)
				}
				if !info.IsDir() {
					return fmt.Errorf(
						"Safetensors path must be a directory: %s\n\n"+
							"See 'docker model package --help' for more information",
						opts.safetensorsDir,
					)
				}
			}

			for i, l := range opts.licensePaths {
				var err error
				opts.licensePaths[i], err = validateAbsolutePath(l, "license")
				if err != nil {
					return err
				}
			}

			// Validate chat template path if provided
			if opts.chatTemplatePath != "" {
				var err error
				opts.chatTemplatePath, err = validateAbsolutePath(opts.chatTemplatePath, "chat template")
				if err != nil {
					return err
				}
			}

			// Validate mmproj path if provided
			if opts.mmprojPath != "" {
				var err error
				opts.mmprojPath, err = validateAbsolutePath(opts.mmprojPath, "mmproj")
				if err != nil {
					return err
				}
			}

			// Validate DDUF path if provided
			if opts.ddufPath != "" {
				var err error
				opts.ddufPath, err = validateAbsolutePath(opts.ddufPath, "DDUF")
				if err != nil {
					return err
				}
			}

			// Validate dir-tar paths are relative (not absolute)
			for _, dirPath := range opts.dirTarPaths {
				if filepath.IsAbs(dirPath) {
					return fmt.Errorf(
						"dir-tar path must be relative, got absolute path: %s\n\n"+
							"See 'docker model package --help' for more information",
						dirPath,
					)
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.tag = args[0]
			if err := packageModel(cmd.Context(), cmd, desktopClient, opts); err != nil {
				cmd.PrintErrln("Failed to package model")
				return fmt.Errorf("package model: %w", err)
			}
			return nil
		},
		ValidArgsFunction: completion.NoComplete,
	}

	c.Flags().StringVar(&opts.ggufPath, "gguf", "", "absolute path to gguf file")
	c.Flags().StringVar(&opts.safetensorsDir, "safetensors-dir", "", "absolute path to directory containing safetensors files and config")
	c.Flags().StringVar(&opts.ddufPath, "dduf", "", "absolute path to DDUF archive file (Diffusers Unified Format)")
	c.Flags().StringVar(&opts.fromModel, "from", "", "reference to an existing model to repackage")
	c.Flags().StringVar(&opts.chatTemplatePath, "chat-template", "", "absolute path to chat template file (must be Jinja format)")
	c.Flags().StringArrayVarP(&opts.licensePaths, "license", "l", nil, "absolute path to a license file")
	c.Flags().StringArrayVar(&opts.dirTarPaths, "dir-tar", nil, "relative path to directory to package as tar (can be specified multiple times)")
	c.Flags().StringVar(&opts.mmprojPath, "mmproj", "", "absolute path to multimodal projector file")
	c.Flags().BoolVar(&opts.push, "push", false, "push to registry (if not set, the model is loaded into the Model Runner content store)")
	c.Flags().Uint64Var(&opts.contextSize, "context-size", 0, "context size in tokens")
	return c
}

type packageOptions struct {
	chatTemplatePath string
	contextSize      uint64
	ggufPath         string
	safetensorsDir   string
	ddufPath         string
	fromModel        string
	licensePaths     []string
	dirTarPaths      []string
	mmprojPath       string
	push             bool
	tag              string
}

// builderInitResult contains the result of initializing a builder from various sources
type builderInitResult struct {
	builder     *builder.Builder
	distClient  *distribution.Client // Only set when building from existing model
	cleanupFunc func()               // Optional cleanup function for temporary files
}

// initializeBuilder creates a package builder from GGUF, Safetensors, DDUF, or existing model
func initializeBuilder(cmd *cobra.Command, opts packageOptions) (*builderInitResult, error) {
	result := &builderInitResult{}

	if opts.fromModel != "" {
		// Get the model store path
		userHomeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get user home directory: %w", err)
		}
		modelStorePath := filepath.Join(userHomeDir, ".docker", "models")
		if envPath := os.Getenv("MODELS_PATH"); envPath != "" {
			modelStorePath = envPath
		}

		// Create a distribution client to access the model store
		distClient, err := distribution.NewClient(distribution.WithStoreRootPath(modelStorePath))
		if err != nil {
			return nil, fmt.Errorf("create distribution client: %w", err)
		}
		result.distClient = distClient

		// Package from existing model
		cmd.PrintErrf("Reading model from store: %q\n", opts.fromModel)

		// Get the model from the local store
		mdl, err := distClient.GetModel(opts.fromModel)
		if err != nil {
			return nil, fmt.Errorf("get model from store: %w", err)
		}

		// Type assert to ModelArtifact - the Model from store implements both interfaces
		modelArtifact, ok := mdl.(types.ModelArtifact)
		if !ok {
			return nil, fmt.Errorf("model does not implement ModelArtifact interface")
		}

		cmd.PrintErrf("Creating builder from existing model\n")
		result.builder, err = builder.FromModel(modelArtifact)
		if err != nil {
			return nil, fmt.Errorf("create builder from model: %w", err)
		}
	} else if opts.ggufPath != "" {
		cmd.PrintErrf("Adding GGUF file from %q\n", opts.ggufPath)
		pkg, err := builder.FromPath(opts.ggufPath)
		if err != nil {
			return nil, fmt.Errorf("add gguf file: %w", err)
		}
		result.builder = pkg
	} else if opts.ddufPath != "" {
		cmd.PrintErrf("Adding DDUF file from %q\n", opts.ddufPath)
		pkg, err := builder.FromPath(opts.ddufPath)
		if err != nil {
			return nil, fmt.Errorf("add dduf file: %w", err)
		}
		result.builder = pkg
	} else if opts.safetensorsDir != "" {
		// Safetensors model from directory
		cmd.PrintErrf("Scanning directory %q for safetensors model...\n", opts.safetensorsDir)
		safetensorsPaths, tempConfigArchive, err := packaging.PackageFromDirectory(opts.safetensorsDir)
		if err != nil {
			return nil, fmt.Errorf("scan safetensors directory: %w", err)
		}

		// Set up cleanup for temp config archive
		if tempConfigArchive != "" {
			result.cleanupFunc = func() {
				os.Remove(tempConfigArchive)
			}
		}

		cmd.PrintErrf("Found %d safetensors file(s)\n", len(safetensorsPaths))
		pkg, err := builder.FromPaths(safetensorsPaths)
		if err != nil {
			return nil, fmt.Errorf("create safetensors model: %w", err)
		}

		// Add config archive if it was created
		if tempConfigArchive != "" {
			cmd.PrintErrf("Adding config archive from directory\n")
			pkg, err = pkg.WithConfigArchive(tempConfigArchive)
			if err != nil {
				return nil, fmt.Errorf("add config archive: %w", err)
			}
		}
		result.builder = pkg
	} else {
		return nil, fmt.Errorf("no model source specified")
	}

	return result, nil
}

func packageModel(ctx context.Context, cmd *cobra.Command, client *desktop.Client, opts packageOptions) error {
	var (
		target builder.Target
		err    error
	)
	if opts.push {
		target, err = registry.NewClient(
			registry.WithUserAgent("docker-model-cli/" + desktop.Version),
		).NewTarget(opts.tag)
	} else {
		// Ensure standalone runner is available when loading locally
		if _, err := ensureStandaloneRunnerAvailable(ctx, asPrinter(cmd), false); err != nil {
			return fmt.Errorf("unable to initialize standalone model runner: %w", err)
		}
		target, err = newModelRunnerTarget(client, opts.tag)
	}
	if err != nil {
		return err
	}

	// Initialize the package builder based on model format
	initResult, err := initializeBuilder(cmd, opts)
	if err != nil {
		return err
	}
	// Clean up any temporary files when done
	if initResult.cleanupFunc != nil {
		defer initResult.cleanupFunc()
	}

	pkg := initResult.builder
	distClient := initResult.distClient

	// Set context size
	if cmd.Flags().Changed("context-size") {
		cmd.PrintErrf("Setting context size %d\n", opts.contextSize)
		pkg = pkg.WithContextSize(int32(opts.contextSize))
	}

	// Add license files
	for _, path := range opts.licensePaths {
		cmd.PrintErrf("Adding license file from %q\n", path)
		pkg, err = pkg.WithLicense(path)
		if err != nil {
			return fmt.Errorf("add license file: %w", err)
		}
	}

	if opts.chatTemplatePath != "" {
		cmd.PrintErrf("Adding chat template file from %q\n", opts.chatTemplatePath)
		if pkg, err = pkg.WithChatTemplateFile(opts.chatTemplatePath); err != nil {
			return fmt.Errorf("add chat template file from path %q: %w", opts.chatTemplatePath, err)
		}
	}

	if opts.mmprojPath != "" {
		cmd.PrintErrf("Adding multimodal projector file from %q\n", opts.mmprojPath)
		pkg, err = pkg.WithMultimodalProjector(opts.mmprojPath)
		if err != nil {
			return fmt.Errorf("add multimodal projector file: %w", err)
		}
	}

	// Check if we can use lightweight repackaging (config-only changes from existing model)
	useLightweight := opts.fromModel != "" && pkg.HasOnlyConfigChanges()

	if useLightweight {
		cmd.PrintErrln("Creating lightweight model variant...")

		// Get the model artifact with new config
		builtModel := pkg.Model()

		// Write using lightweight method
		if err := distClient.WriteLightweightModel(builtModel, []string{opts.tag}); err != nil {
			return fmt.Errorf("failed to create lightweight model: %w", err)
		}

		cmd.PrintErrln("Model variant created successfully")
		return nil // Return early to avoid the Build operation in lightweight case
	} else {
		// Process directory tar archives
		if len(opts.dirTarPaths) > 0 {
			// Determine base directory for resolving relative paths
			var baseDir string
			if opts.safetensorsDir != "" {
				baseDir = opts.safetensorsDir
			} else {
				// For GGUF, use the directory containing the GGUF file
				baseDir = filepath.Dir(opts.ggufPath)
			}

			processor := packaging.NewDirTarProcessor(opts.dirTarPaths, baseDir)
			tarPaths, cleanup, err := processor.Process()
			if err != nil {
				return err
			}
			defer cleanup()

			for _, tarPath := range tarPaths {
				pkg, err = pkg.WithDirTar(tarPath)
				if err != nil {
					return fmt.Errorf("add directory tar: %w", err)
				}
			}
		}
	}
	if opts.push {
		cmd.PrintErrln("Pushing model to registry...")
	} else {
		cmd.PrintErrln("Loading model to Model Runner...")
	}
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		defer pw.Close()
		done <- pkg.Build(ctx, target, pw)
	}()

	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		progressLine := scanner.Text()
		if progressLine == "" {
			continue
		}

		// Parse the progress message
		var progressMsg oci.ProgressMessage
		if err := json.Unmarshal([]byte(html.UnescapeString(progressLine)), &progressMsg); err != nil {
			cmd.PrintErrln("Error displaying progress:", err)
		}

		// Print progress messages
		fmt.Print("\r\033[K", progressMsg.Message)
	}
	cmd.PrintErrln("") // newline after progress

	if err := scanner.Err(); err != nil {
		cmd.PrintErrln("Error streaming progress:", err)
	}
	if err := <-done; err != nil {
		if opts.push {
			return fmt.Errorf("failed to save packaged model: %w", err)
		}
		return fmt.Errorf("failed to load packaged model: %w", err)
	}

	if opts.push {
		cmd.PrintErrln("Model pushed successfully")
	} else {
		cmd.PrintErrln("Model loaded successfully")
	}
	return nil
}

// modelRunnerTarget loads model to Docker Model Runner via models/load endpoint
type modelRunnerTarget struct {
	client *desktop.Client
	tag    *reference.Tag
}

func newModelRunnerTarget(client *desktop.Client, tag string) (*modelRunnerTarget, error) {
	target := &modelRunnerTarget{
		client: client,
	}
	if tag != "" {
		var err error
		target.tag, err = reference.NewTag(tag, registry.GetDefaultRegistryOptions()...)
		if err != nil {
			return nil, fmt.Errorf("invalid tag: %w", err)
		}
	}
	return target, nil
}

func (t *modelRunnerTarget) Write(ctx context.Context, mdl types.ModelArtifact, progressWriter io.Writer) error {
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		defer pw.Close()
		target, err := tarball.NewTarget(pw)
		if err != nil {
			errCh <- err
			return
		}
		errCh <- target.Write(ctx, mdl, progressWriter)
	}()

	loadErr := t.client.LoadModel(ctx, pr)
	writeErr := <-errCh

	if loadErr != nil {
		return fmt.Errorf("loading model archive: %w", loadErr)
	}
	if writeErr != nil {
		return fmt.Errorf("writing model archive: %w", writeErr)
	}
	id, err := mdl.ID()
	if err != nil {
		return fmt.Errorf("get model ID: %w", err)
	}
	if t.tag != nil {
		if err := t.client.Tag(id, parseRepo(t.tag), t.tag.TagStr()); err != nil {
			return fmt.Errorf("tag model: %w", err)
		}
	}
	return nil
}
