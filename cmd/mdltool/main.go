package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/distribution"
	"github.com/docker/model-runner/pkg/distribution/packaging"
	"github.com/docker/model-runner/pkg/distribution/registry"
	"github.com/docker/model-runner/pkg/distribution/tarball"
)

// stringSliceFlag is a flag that can be specified multiple times to collect multiple string values
type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ", ")
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

const (
	defaultStorePath = "./model-store"
	version          = "0.1.0"
)

var (
	storePath string
	showHelp  bool
	showVer   bool
)

func init() {
	flag.StringVar(&storePath, "store-path", defaultStorePath, "Path to the model store")
	flag.BoolVar(&showHelp, "help", false, "Show help")
	flag.BoolVar(&showVer, "version", false, "Show version")
}

func main() {
	flag.Parse()

	if showVer {
		fmt.Printf("model-distribution-tool version %s\n", version)
		return
	}

	if showHelp || flag.NArg() == 0 {
		printUsage()
		return
	}

	// Create absolute path for store
	absStorePath, err := filepath.Abs(storePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving store path: %v\n", err)
		os.Exit(1)
	}

	// Create the client with auth if environment variables are set
	clientOpts := []distribution.Option{
		distribution.WithStoreRootPath(absStorePath),
		distribution.WithUserAgent("model-distribution-tool/" + version),
	}

	if username := os.Getenv("DOCKER_USERNAME"); username != "" {
		if password := os.Getenv("DOCKER_PASSWORD"); password != "" {
			clientOpts = append(clientOpts, distribution.WithRegistryAuth(username, password))
		}
	}

	client, err := distribution.NewClient(clientOpts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating client: %v\n", err)
		os.Exit(1)
	}

	// Get the command and arguments
	command := flag.Arg(0)
	args := flag.Args()[1:]

	// Execute the command
	var exitCode int
	switch command {
	case "pull":
		exitCode = cmdPull(client, args)
	case "package":
		exitCode = cmdPackage(args)
	case "push":
		exitCode = cmdPush(client, args)
	case "list":
		exitCode = cmdList(client, args)
	case "get":
		exitCode = cmdGet(client, args)
	case "get-path":
		exitCode = cmdGetPath(client, args)
	case "rm":
		exitCode = cmdRm(client, args)
	case "tag":
		exitCode = cmdTag(client, args)
	case "load":
		exitCode = cmdLoad(client, args)
	case "bundle":
		exitCode = cmdBundle(client, args)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		exitCode = 1
	}

	os.Exit(exitCode)
}

func printUsage() {
	fmt.Println("Usage: model-distribution-tool [options] <command> [arguments]")
	fmt.Println("\nOptions:")
	flag.PrintDefaults()
	fmt.Println("\nCommands:")
	fmt.Println("  pull <reference>                Pull a model from a registry")
	fmt.Println("  package <source> <reference>    Package a model file as an OCI artifact and push it to a registry")
	fmt.Println("                                  (use --licenses to add license files, --mmproj for multimodal projector, --dir-tar for directories)")
	fmt.Println("  push <tag>                      Push a model from the content store to the registry")
	fmt.Println("  list                            List all models")
	fmt.Println("  get <reference>                 Get a model by reference")
	fmt.Println("  get-path <reference>            Get the local file path for a model")
	fmt.Println("  rm <reference>                  Remove a model by reference")
	fmt.Println("  bundle <reference>              Create a runtime bundle for model")
	fmt.Println("\nExamples:")
	fmt.Println("  model-distribution-tool --store-path ./models pull registry.example.com/models/llama:v1.0")
	fmt.Println("  model-distribution-tool package ./model.gguf registry.example.com/models/llama:v1.0 --licenses ./license1.txt --licenses ./license2.txt")
	fmt.Println("  model-distribution-tool package ./model.gguf registry.example.com/models/llama:v1.0 --mmproj ./model.mmproj")
	fmt.Println("  model-distribution-tool package ./model.gguf registry.example.com/models/llama:v1.0 --dir-tar ./config --dir-tar ./templates")
	fmt.Println("  model-distribution-tool push registry.example.com/models/llama:v1.0")
	fmt.Println("  model-distribution-tool list")
	fmt.Println("  model-distribution-tool rm registry.example.com/models/llama:v1.0")
	fmt.Println("  model-distribution-tool bundle registry.example.com/models/llama:v1.0")
}

func cmdPull(client *distribution.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: missing reference argument\n")
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool pull <reference>\n")
		return 1
	}

	reference := args[0]
	ctx := context.Background()

	if err := client.PullModel(ctx, reference, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error pulling model: %v\n", err)
		return 1
	}

	fmt.Printf("Successfully pulled model: %s\n", reference)
	return 0
}

func cmdPackage(args []string) int {
	fs := flag.NewFlagSet("package", flag.ExitOnError)
	var (
		licensePaths stringSliceFlag
		dirTarPaths  stringSliceFlag
		contextSize  uint64
		file         string
		tag          string
		mmproj       string
		chatTemplate string
	)

	fs.Var(&licensePaths, "licenses", "Paths to license files (can be specified multiple times)")
	fs.Var(&dirTarPaths, "dir-tar", "Relative paths to directories to package as tar (can be specified multiple times)")
	fs.Uint64Var(&contextSize, "context-size", 0, "Context size in tokens")
	fs.StringVar(&mmproj, "mmproj", "", "Path to Multimodal Projector file")
	fs.StringVar(&file, "file", "", "Write archived model to the given file")
	fs.StringVar(&tag, "tag", "", "Push model to the given registry tag")
	fs.StringVar(&chatTemplate, "chat-template", "", "Jinja chat template file")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool package [OPTIONS] <path-to-model-or-directory>\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  # GGUF model:\n")
		fmt.Fprintf(os.Stderr, "  model-distribution-tool package model.gguf --tag registry/model:tag\n\n")
		fmt.Fprintf(os.Stderr, "  # Safetensors model:\n")
		fmt.Fprintf(os.Stderr, "  model-distribution-tool package ./qwen-model-dir --tag registry/model:tag\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		return 1
	}
	args = fs.Args()

	// Get the source from positional argument
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: no model file or directory specified\n")
		fs.Usage()
		return 1
	}

	source := args[0]
	var isSafetensors bool
	var configArchive string      // For safetensors config
	var safetensorsPaths []string // For safetensors model files

	// Check if source exists
	sourceInfo, err := os.Stat(source)
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: source does not exist: %s\n", source)
		return 1
	}

	// Handle directory-based packaging (for safetensors models)
	if sourceInfo.IsDir() {
		fmt.Printf("Detected directory, scanning for safetensors model...\n")
		var err error
		safetensorsPaths, configArchive, err = packaging.PackageFromDirectory(source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error scanning directory: %v\n", err)
			return 1
		}

		isSafetensors = true
		fmt.Printf("Found %d safetensors file(s)\n", len(safetensorsPaths))

		// Clean up temp config archive when done
		if configArchive != "" {
			defer os.Remove(configArchive)
			fmt.Printf("Created temporary config archive from directory\n")
		}
	} else {
		// Handle single file (GGUF model)
		if strings.HasSuffix(strings.ToLower(source), ".gguf") {
			isSafetensors = false
			fmt.Println("Detected GGUF model file")
		} else {
			fmt.Fprintf(os.Stderr, "Warning: could not determine model type for: %s\n", source)
			fmt.Fprintf(os.Stderr, "Assuming GGUF format.\n")
		}
	}

	if file == "" && tag == "" {
		fmt.Fprintf(os.Stderr, "Error: one of --file or --tag is required\n")
		fs.Usage()
		return 1
	}

	ctx := context.Background()

	// Prepare registry client options
	registryClientOpts := []registry.ClientOption{
		registry.WithUserAgent("model-distribution-tool/" + version),
	}

	// Add auth if available
	if username := os.Getenv("DOCKER_USERNAME"); username != "" {
		if password := os.Getenv("DOCKER_PASSWORD"); password != "" {
			registryClientOpts = append(registryClientOpts, registry.WithAuthConfig(username, password))
		}
	}

	// Create registry client once with all options
	registryClient := registry.NewClient(registryClientOpts...)

	var target builder.Target
	if file != "" {
		target = tarball.NewFileTarget(file)
	} else {
		var err error
		target, err = registryClient.NewTarget(tag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Create packaging target: %v\n", err)
			return 1
		}
	}

	// Create builder based on model type
	var b *builder.Builder
	if isSafetensors {
		fmt.Println("Creating safetensors model")
		b, err = builder.FromSafetensors(safetensorsPaths)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating model from safetensors: %v\n", err)
			return 1
		}

		// Add config archive if provided
		if configArchive != "" {
			fmt.Printf("Adding config archive: %s\n", configArchive)
			b, err = b.WithConfigArchive(configArchive)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error adding config archive: %v\n", err)
				return 1
			}
		}
	} else {
		b, err = builder.FromGGUF(source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating model from gguf: %v\n", err)
			return 1
		}
	}

	// Add all license files as layers
	for _, path := range licensePaths {
		fmt.Println("Adding license file:", path)
		b, err = b.WithLicense(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error adding license layer for %s: %v\n", path, err)
			return 1
		}
	}

	if contextSize > 0 {
		fmt.Println("Setting context size:", contextSize)
		b = b.WithContextSize(int32(contextSize))
	}

	if mmproj != "" {
		fmt.Println("Adding multimodal projector file:", mmproj)
		b, err = b.WithMultimodalProjector(mmproj)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error adding multimodal projector layer for %s: %v\n", mmproj, err)
			return 1
		}
	}

	if chatTemplate != "" {
		fmt.Println("Adding chat template file:", chatTemplate)
		b, err = b.WithChatTemplateFile(chatTemplate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error adding chat template layer for %s: %v\n", chatTemplate, err)
			return 1
		}
	}

	// Process directory tar archives
	if len(dirTarPaths) > 0 {
		// Determine base directory for resolving relative paths
		var baseDir string
		if isSafetensors {
			baseDir = source
		} else {
			// For GGUF, use the directory containing the GGUF file
			baseDir = filepath.Dir(source)
		}

		processor := packaging.NewDirTarProcessor(dirTarPaths, baseDir)
		tarPaths, cleanup, err := processor.Process()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing dir-tar paths: %v\n", err)
			return 1
		}
		defer cleanup()

		for _, tarPath := range tarPaths {
			b, err = b.WithDirTar(tarPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error adding directory tar: %v\n", err)
				return 1
			}
		}
	}

	// Push the image
	if err := b.Build(ctx, target, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing model to registry: %v\n", err)
		return 1
	}
	if tag != "" {
		fmt.Printf("Successfully packaged and pushed model: %s\n", tag)
	} else {
		fmt.Printf("Successfully packaged model to file: %s\n", file)
	}
	return 0
}

func cmdLoad(client *distribution.Client, args []string) int {
	fs := flag.NewFlagSet("load", flag.ExitOnError)
	var (
		tag string
	)
	fs.StringVar(&tag, "tag", "", "Apply tag to the loaded model")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool load [OPTIONS] <path-to-archive>\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		return 1
	}
	args = fs.Args()

	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: missing required argument\n")
		fs.Usage()
		return 1
	}
	path := args[0]

	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening model file: %v\n", err)
		return 1
	}
	defer f.Close()

	id, err := client.LoadModel(f, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading model: %v\n", err)
		return 1
	}
	fmt.Fprintln(os.Stdout, "Loaded model:", id)
	if err := client.Tag(id, tag); err != nil {
		fmt.Fprintf(os.Stderr, "Error tagging model: %v\n", err)
	}
	fmt.Fprintln(os.Stdout, "Tagged model:", tag)
	return 0
}

func cmdPush(client *distribution.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: missing tag argument\n")
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool push <tag>\n")
		return 1
	}

	tag := args[0]
	ctx := context.Background()

	if err := client.PushModel(ctx, tag, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error pushing model: %v\n", err)
		return 1
	}

	fmt.Printf("Successfully pushed model: %s\n", tag)
	return 0
}

func cmdList(client *distribution.Client, args []string) int {
	models, err := client.ListModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing models: %v\n", err)
		return 1
	}

	if len(models) == 0 {
		fmt.Println("No models found")
		return 0
	}

	fmt.Println("Models:")
	for i, model := range models {
		id, err := model.ID()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting model ID: %v\n", err)
			continue
		}
		fmt.Printf("%d. ID: %s\n", i+1, id)
		fmt.Printf("   Tags: %s\n", strings.Join(model.Tags(), ", "))

		ggufPaths, err := model.GGUFPaths()
		if err == nil {
			fmt.Print("   GGUF Paths:\n")
			for _, path := range ggufPaths {
				fmt.Printf("\t%s\n", path)
			}
		}
	}
	return 0
}

func cmdGet(client *distribution.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: missing reference argument\n")
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool get <reference>\n")
		return 1
	}

	reference := args[0]

	model, err := client.GetModel(reference)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting model: %v\n", err)
		return 1
	}

	fmt.Printf("Model: %s\n", reference)

	id, err := model.ID()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting model ID %v\n", err)
		return 1
	}
	fmt.Printf("ID: %s\n", id)

	ggufPaths, err := model.GGUFPaths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting gguf path %v\n", err)
		return 1
	}
	fmt.Print("   GGUF Paths:\n")
	for _, path := range ggufPaths {
		fmt.Printf("\t%s\n", path)
	}

	cfg, err := model.Config()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading model config: %v\n", err)
		return 1
	}
	fmt.Printf("Format: %s\n", cfg.Format)
	fmt.Printf("Architecture: %s\n", cfg.Architecture)
	fmt.Printf("Parameters: %s\n", cfg.Parameters)
	fmt.Printf("Quantization: %s\n", cfg.Quantization)
	return 0
}

func cmdGetPath(client *distribution.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: missing reference argument\n")
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool get-path <reference>\n")
		return 1
	}

	reference := args[0]

	model, err := client.GetModel(reference)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get model: %v\n", err)
		return 1
	}

	modelPaths, err := model.GGUFPaths()
	if err != nil || len(modelPaths) == 0 {
		fmt.Fprintf(os.Stderr, "Error getting model path: %v\n", err)
		return 1
	}

	fmt.Println(modelPaths[0])
	return 0
}

func cmdRm(client *distribution.Client, args []string) int {
	var force bool
	fs := flag.NewFlagSet("rm", flag.ExitOnError)
	fs.BoolVar(&force, "force", false, "Force remove the model")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		return 1
	}
	args = fs.Args()

	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: missing reference argument\n")
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool rm [--force] <reference>\n")
		return 1
	}

	reference := args[0]

	if _, err := client.DeleteModel(reference, force); err != nil {
		fmt.Fprintf(os.Stderr, "Error removing model: %v\n", err)
		return 1
	}

	fmt.Printf("Successfully removed model: %s\n", reference)
	return 0
}

func cmdTag(client *distribution.Client, args []string) int {
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool tag <reference> <tag>\n")
		return 1
	}

	source := args[0]
	target := args[1]

	if err := client.Tag(source, target); err != nil {
		fmt.Fprintf(os.Stderr, "Error tagging model: %v\n", err)
		return 1
	}

	fmt.Printf("Successfully applied tag %s to model: %s\n", target, source)
	return 0
}

func cmdBundle(client *distribution.Client, args []string) int {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool bundle <reference>\n")
		return 1
	}
	bundle, err := client.GetBundle(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting model bundle: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "Successfully created bundle for model %s\n", args[0])
	fmt.Fprint(os.Stdout, bundle.RootDir())
	return 0
}
