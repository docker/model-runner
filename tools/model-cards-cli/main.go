package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/model-cards/tools/build-tables/internal/logger"
	"github.com/docker/model-cards/tools/build-tables/internal/markdown"
	"github.com/docker/model-cards/tools/build-tables/internal/registry"
	"github.com/docker/model-cards/tools/build-tables/internal/utils"
	"github.com/sirupsen/logrus"
)

// Application encapsulates the main application logic
type Application struct {
	client    registry.Client
	updater   markdown.Updater
	modelDir  string
	modelFile string
}

// NewApplication creates a new application instance
func NewApplication(client registry.Client, updater markdown.Updater, modelDir string, modelFile string) *Application {
	return &Application{
		client:    client,
		updater:   updater,
		modelDir:  modelDir,
		modelFile: modelFile,
	}
}

// Run executes the main application logic
func (a *Application) Run() error {
	var files []string
	var err error

	// Check if a specific model file is requested
	if a.modelFile != "" {
		// Process only the specified model file
		modelFilePath := filepath.Join(a.modelDir, a.modelFile)
		if !utils.FileExists(modelFilePath) {
			err := fmt.Errorf("model file '%s' does not exist", modelFilePath)
			logger.WithField("file", modelFilePath).Error("model file does not exist")
			return err
		}

		logger.Infof("🔍 Processing single model file: %s", a.modelFile)
		files = []string{modelFilePath}
	} else {
		// Process all model files in the directory
		logger.Info("🔍 Finding all model readme files in ai/ folder...")

		// Find all markdown files in the model directory
		files, err = markdown.FindMarkdownFiles(a.modelDir)
		if err != nil {
			logger.WithError(err).Error("error finding model files")
			return err
		}

		logger.Infof("Found %d model files", len(files))
	}

	// Count total models for progress tracking
	totalModels := len(files)
	current := 0

	// Process each markdown file
	for _, file := range files {
		// Extract the model name from the filename
		modelName := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))

		// Increment counter
		current++

		// Display progress
		logger.Info("===============================================")
		logger.Infof("🔄 Processing model %d/%d: %s/%s", current, totalModels, filepath.Base(a.modelDir), modelName)
		logger.Info("===============================================")

		// Process the model file
		err := a.processModelFile(file)
		if err != nil {
			logger.WithFields(logger.Fields{
				"model": modelName,
				"error": err,
			}).Error("Error processing model")
			continue
		} else {
			logger.WithField("model", modelName).Info("Successfully processed model")
		}

		logger.Infof("✅ Completed %s/%s", filepath.Base(a.modelDir), modelName)
	}

	logger.Info("===============================================")
	logger.Info("🎉 All model tables have been updated!")
	logger.Info("===============================================")

	return nil
}

// processModelFile processes a single model markdown file
func (a *Application) processModelFile(filePath string) error {
	// Extract the repository name from the file path
	repoName := utils.GetRepositoryName(filePath, filepath.Dir(a.modelDir))

	logger.WithField("file", filePath).Info("📄 Using readme file")

	// Check if the file exists
	if !utils.FileExists(filePath) {
		err := fmt.Errorf("readme file '%s' does not exist", filePath)
		logger.WithField("file", filePath).Error("readme file does not exist")
		return err
	}

	// List all tags for the repository
	logger.WithField("repository", repoName).Info("📦 Listing tags for repository")
	tags, err := a.client.ListTags(repoName)
	if err != nil {
		logger.WithFields(logger.Fields{
			"repository": repoName,
			"error":      err,
		}).Error("error listing tags")
		return fmt.Errorf("error listing tags: %v", err)
	}

	// Process each tag and collect model variants
	variants, err := a.client.ProcessTags(repoName, tags)
	if err != nil {
		logger.WithFields(logger.Fields{
			"repository": repoName,
			"error":      err,
		}).Error("error processing tags")
		return fmt.Errorf("error processing tags: %v", err)
	}

	// Update the markdown file with the new table
	err = a.updater.UpdateModelTable(filePath, variants)
	if err != nil {
		logger.WithFields(logger.Fields{
			"file":  filePath,
			"error": err,
		}).Error("error updating markdown file")
		return fmt.Errorf("error updating markdown file: %v", err)
	}

	return nil
}

// ModelInspector encapsulates the model inspection logic
type ModelInspector struct {
	client     registry.Client
	repository string
	tag        string
	showAll    bool
}

// NewModelInspector creates a new model inspector
func NewModelInspector(client registry.Client, repository, tag string, showAll bool) *ModelInspector {
	return &ModelInspector{
		client:     client,
		repository: repository,
		tag:        tag,
		showAll:    showAll,
	}
}

// Run executes the model inspection
func (m *ModelInspector) Run() error {
	// If a specific tag is provided, inspect only that tag
	if m.tag != "" {
		return m.inspectTag(m.repository, m.tag)
	}

	// Otherwise, list all tags and inspect each one
	tags, err := m.client.ListTags(m.repository)
	if err != nil {
		return fmt.Errorf("failed to list tags: %v", err)
	}

	logger.Infof("Found %d tags for repository %s", len(tags), m.repository)

	// Otherwise, output in text format
	for _, tag := range tags {
		if err := m.inspectTag(m.repository, tag); err != nil {
			logger.Warnf("Failed to inspect %s:%s: %v", m.repository, tag, err)
		}
		fmt.Println("----------------------------------------")
	}

	return nil
}

// inspectTag inspects a specific tag and outputs the requested information
func (m *ModelInspector) inspectTag(repository, tag string) error {
	logger.Infof("Inspecting %s:%s", repository, tag)

	// Get model variant information
	variant, err := m.client.GetModelVariant(context.Background(), repository, tag)
	if err != nil {
		return fmt.Errorf("failed to get model variant: %v", err)
	}

	fmt.Printf("🔍 Model: %s:%s\n", repository, tag)
	fmt.Printf("   • Parameters   : %s\n", variant.Parameters)
	fmt.Printf("   • Architecture : %s\n", variant.Descriptor.GetArchitecture())
	fmt.Printf("   • Quantization : %s\n", variant.Quantization)
	fmt.Printf("   • Size         : %s\n", utils.FormatSize(variant.Size))
	fmt.Printf("   • Context      : %s\n", utils.FormatContextLength(variant.ContextLength))
	fmt.Printf("   • VRAM         : %s\n", utils.FormatVRAM(variant.VRAM))

	// Only print metadata if showAll is true
	if m.showAll {
		fmt.Println("   • Metadata     :")
		for key, value := range variant.Descriptor.GetMetadata() {
			fmt.Printf("     • %s: %s\n", key, value)
		}
	}

	return nil
}

func main() {
	// Define command flags
	updateCmd := flag.NewFlagSet("update", flag.ExitOnError)
	inspectCmd := flag.NewFlagSet("inspect-model", flag.ExitOnError)

	// Update command flags
	updateLogLevel := updateCmd.String("log-level", "info", "Log level (debug, info, warn, error)")
	updateModelDir := updateCmd.String("model-dir", "../../ai", "Directory containing model markdown files")
	updateModelFile := updateCmd.String("model-file", "", "Specific model markdown file to update (without path)")

	// Inspect command flags
	inspectLogLevel := inspectCmd.String("log-level", "info", "Log level (debug, info, warn, error)")
	inspectTag := inspectCmd.String("tag", "", "Specific tag to inspect")
	inspectAll := inspectCmd.Bool("all", false, "Show all metadata")

	// Check if a command is provided
	if len(os.Args) < 2 {
		fmt.Println("Expected 'update' or 'inspect-model' subcommand")
		fmt.Println("Usage:")
		fmt.Println("  model-cards-cli update [options]")
		fmt.Println("  model-cards-cli inspect-model [options] REPOSITORY")
		os.Exit(1)
	}

	// Configure logger based on the command
	var logLevel string

	// Parse the appropriate command
	switch os.Args[1] {
	case "update":
		updateCmd.Parse(os.Args[2:])
		logLevel = *updateLogLevel
	case "inspect-model":
		inspectCmd.Parse(os.Args[2:])
		logLevel = *inspectLogLevel
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		fmt.Println("Expected 'update' or 'inspect-model' subcommand")
		os.Exit(1)
	}

	// Configure logger
	switch logLevel {
	case "debug":
		logger.Log.SetLevel(logrus.DebugLevel)
	case "info":
		logger.Log.SetLevel(logrus.InfoLevel)
	case "warn":
		logger.Log.SetLevel(logrus.WarnLevel)
	case "error":
		logger.Log.SetLevel(logrus.ErrorLevel)
	default:
		logger.Log.SetLevel(logrus.InfoLevel)
	}

	logger.Debugf("Log level set to: %s", logLevel)

	// Create dependencies
	client := registry.NewClient()

	// Execute the appropriate command
	if updateCmd.Parsed() {
		logger.Info("Starting model-cards updater")
		app := NewApplication(client, markdown.Updater{}, *updateModelDir, *updateModelFile)
		if err := app.Run(); err != nil {
			logger.WithError(err).Errorf("Application failed: %v", err)
			os.Exit(1)
		}

		logger.Info("Application completed successfully")
	} else if inspectCmd.Parsed() {
		logger.Info("Starting model inspector")

		// Check if a repository is provided
		args := inspectCmd.Args()
		if len(args) < 1 {
			fmt.Println("Error: Repository argument is required")
			fmt.Println("Usage: updater inspect-model [options] REPOSITORY")
			os.Exit(1)
		}

		repository := args[0]

		inspector := NewModelInspector(client, repository, *inspectTag, *inspectAll)

		if err := inspector.Run(); err != nil {
			logger.WithError(err).Errorf("Inspection failed: %v", err)
			os.Exit(1)
		}

		logger.Info("Inspection completed successfully")
	}
}
