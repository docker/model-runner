package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
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
	namespace string
}

// NewApplication creates a new application instance
func NewApplication(client registry.Client, updater markdown.Updater, modelDir string, modelFile string, namespace string) *Application {
	return &Application{
		client:    client,
		updater:   updater,
		modelDir:  modelDir,
		modelFile: modelFile,
		namespace: namespace,
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
	var repoName string
	if a.namespace != "" {
		name := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
		repoName = fmt.Sprintf("%s/%s", a.namespace, name)
	} else {
		repoName = utils.GetRepositoryName(filePath, filepath.Dir(a.modelDir))
	}

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

// OverviewUploader encapsulates the overview upload logic
type OverviewUploader struct {
	filePath   string
	repository string
	username   string
	token      string
}

// NewOverviewUploader creates a new overview uploader
func NewOverviewUploader(filePath, repository, username, token string) *OverviewUploader {
	return &OverviewUploader{
		filePath:   filePath,
		repository: repository,
		username:   username,
		token:      token,
	}
}

// getAccessToken authenticates with Docker Hub and returns an access token
func (o *OverviewUploader) getAccessToken() (string, error) {
	// Create login payload
	loginPayload := map[string]string{
		"username": o.username,
		"password": o.token, // PAT is used as password
	}

	// Convert payload to JSON
	payloadBytes, err := json.Marshal(loginPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal login payload: %v", err)
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", "https://hub.docker.com/v2/users/login", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create login request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{}
	logger.Info("Authenticating with Docker Hub...")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send login request: %v", err)
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read login response: %v", err)
	}

	// Check the response status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("authentication failed: %s - %s", resp.Status, string(respBody))
	}

	// Parse the response to get the token
	var loginResponse map[string]interface{}
	if err := json.Unmarshal(respBody, &loginResponse); err != nil {
		return "", fmt.Errorf("failed to parse login response: %v", err)
	}

	// Extract the token
	token, ok := loginResponse["token"].(string)
	if !ok {
		return "", fmt.Errorf("token not found in login response")
	}

	logger.Info("✅ Authentication successful")
	return token, nil
}

// Run executes the overview upload
func (o *OverviewUploader) Run() error {
	// Check if the file exists
	if !utils.FileExists(o.filePath) {
		err := fmt.Errorf("overview file '%s' does not exist", o.filePath)
		logger.WithField("file", o.filePath).Error("overview file does not exist")
		return err
	}

	// Read the overview file
	content, err := os.ReadFile(o.filePath)
	if err != nil {
		logger.WithError(err).Error("failed to read overview file")
		return fmt.Errorf("failed to read overview file: %v", err)
	}

	// Parse the repository name to extract namespace and repository
	parts := strings.Split(o.repository, "/")
	if len(parts) != 2 {
		err := fmt.Errorf("invalid repository format: %s (expected 'namespace/repository')", o.repository)
		logger.WithField("repository", o.repository).Error("invalid repository format")
		return err
	}

	namespace := parts[0]
	repository := parts[1]

	// Get access token
	accessToken, err := o.getAccessToken()
	if err != nil {
		logger.WithError(err).Error("failed to get access token")
		return err
	}

	// Create the payload
	payload := map[string]interface{}{
		//"description":      "", // Short description (optional)
		"full_description": string(content),
		"status":           1, // Repository active
	}

	// Convert payload to JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.WithError(err).Error("failed to marshal payload")
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	// Construct the API URL
	url := fmt.Sprintf("https://hub.docker.com/v2/namespaces/%s/repositories/%s", namespace, repository)

	// Create the HTTP request
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		logger.WithError(err).Error("failed to create HTTP request")
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	// Send the request
	client := &http.Client{}
	logger.Infof("Uploading overview to %s/%s...", namespace, repository)
	resp, err := client.Do(req)
	if err != nil {
		logger.WithError(err).Error("failed to send HTTP request")
		return fmt.Errorf("failed to send HTTP request: %v", err)
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.WithError(err).Error("failed to read response body")
		return fmt.Errorf("failed to read response body: %v", err)
	}

	// Check the response status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		logger.WithFields(logger.Fields{
			"status": resp.Status,
			"body":   string(respBody),
		}).Error("failed to upload overview")
		return fmt.Errorf("failed to upload overview: %s - %s", resp.Status, string(respBody))
	}

	logger.Info("✅ Overview uploaded successfully!")
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
	uploadCmd := flag.NewFlagSet("upload-overview", flag.ExitOnError)

	// Update command flags
	updateLogLevel := updateCmd.String("log-level", "info", "Log level (debug, info, warn, error)")
	updateModelDir := updateCmd.String("model-dir", "../../ai", "Directory containing model markdown files")
	updateModelFile := updateCmd.String("model-file", "", "Specific model markdown file to update (without path)")
	updateNamespace := updateCmd.String("namespace", "", "Namespace to use for repositories (overrides deriving from file path)")

	// Inspect command flags
	inspectLogLevel := inspectCmd.String("log-level", "info", "Log level (debug, info, warn, error)")
	inspectTag := inspectCmd.String("tag", "", "Specific tag to inspect")
	inspectAll := inspectCmd.Bool("all", false, "Show all metadata")

	// Upload overview command flags
	uploadLogLevel := uploadCmd.String("log-level", "info", "Log level (debug, info, warn, error)")
	uploadFile := uploadCmd.String("file", "", "Path to the overview file to upload")
	uploadRepo := uploadCmd.String("repository", "", "Repository to upload the overview to (format: namespace/repository)")
	uploadUsername := uploadCmd.String("username", "", "Docker Hub username")
	uploadToken := uploadCmd.String("token", "", "Personal Access Token (PAT)")

	// Check if a command is provided
	if len(os.Args) < 2 {
		fmt.Println("Expected 'update', 'inspect-model', or 'upload-overview' subcommand")
		fmt.Println("Usage:")
		fmt.Println("  model-cards-cli update [options]")
		fmt.Println("  model-cards-cli inspect-model [options] REPOSITORY")
		fmt.Println("  model-cards-cli upload-overview [options]")
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
	case "upload-overview":
		uploadCmd.Parse(os.Args[2:])
		logLevel = *uploadLogLevel
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		fmt.Println("Expected 'update', 'inspect-model', or 'upload-overview' subcommand")
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
		app := NewApplication(client, markdown.Updater{}, *updateModelDir, *updateModelFile, *updateNamespace)
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
			fmt.Println("Usage: model-cards-cli inspect-model [options] REPOSITORY")
			os.Exit(1)
		}

		repository := args[0]

		inspector := NewModelInspector(client, repository, *inspectTag, *inspectAll)

		if err := inspector.Run(); err != nil {
			logger.WithError(err).Errorf("Inspection failed: %v", err)
			os.Exit(1)
		}

		logger.Info("Inspection completed successfully")
	} else if uploadCmd.Parsed() {
		logger.Info("Starting overview uploader")

		// Check if required parameters are provided
		if *uploadFile == "" {
			fmt.Println("Error: --file parameter is required")
			fmt.Println("Usage: model-cards-cli upload-overview --file=<file> --repository=<namespace/repository> --username=<username> --token=<token>")
			os.Exit(1)
		}

		if *uploadRepo == "" {
			fmt.Println("Error: --repository parameter is required")
			fmt.Println("Usage: model-cards-cli upload-overview --file=<file> --repository=<namespace/repository> --username=<username> --token=<token>")
			os.Exit(1)
		}

		if *uploadUsername == "" {
			fmt.Println("Error: --username parameter is required")
			fmt.Println("Usage: model-cards-cli upload-overview --file=<file> --repository=<namespace/repository> --username=<username> --token=<token>")
			os.Exit(1)
		}

		if *uploadToken == "" {
			fmt.Println("Error: --token parameter is required")
			fmt.Println("Usage: model-cards-cli upload-overview --file=<file> --repository=<namespace/repository> --username=<username> --token=<token>")
			os.Exit(1)
		}

		uploader := NewOverviewUploader(*uploadFile, *uploadRepo, *uploadUsername, *uploadToken)

		if err := uploader.Run(); err != nil {
			logger.WithError(err).Errorf("Upload failed: %v", err)
			os.Exit(1)
		}

		logger.Info("Upload completed successfully")
	}
}
