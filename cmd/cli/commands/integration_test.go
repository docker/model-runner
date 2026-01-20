//go:build integration

package commands

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/oci/reference"
	"github.com/docker/model-runner/pkg/distribution/registry"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tc "github.com/testcontainers/testcontainers-go/modules/registry"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// testEnv holds the test environment components
type testEnv struct {
	ctx         context.Context
	registryURL string
	client      *desktop.Client
	net         *testcontainers.DockerNetwork
}

// modelInfo contains all the information needed to generate model references
type modelInfo struct {
	name         string // e.g., "test-model"
	org          string // e.g., "ai"
	tag          string // e.g., "latest"
	registry     string // e.g., "registry.local:5000"
	modelID      string // Full ID: sha256:...
	digest       string // sha256:...
	expectedName string // What we expect to see: "ai/test-model:latest"
}

// referenceTestCase represents a test case for a specific reference format
type referenceTestCase struct {
	name string
	ref  string
}

// generateReferenceTestCases generates a comprehensive list of reference formats for testing
func generateReferenceTestCases(info modelInfo) []referenceTestCase {
	cases := []referenceTestCase{
		{
			name: "short form (no registry, no org, no tag)",
			ref:  info.name,
		},
		{
			name: "with tag",
			ref:  fmt.Sprintf("%s:%s", info.name, info.tag),
		},
		{
			name: "with org",
			ref:  fmt.Sprintf("%s/%s", info.org, info.name),
		},
		{
			name: "with org and tag",
			ref:  fmt.Sprintf("%s/%s:%s", info.org, info.name, info.tag),
		},
		{
			name: "fqdn",
			ref:  fmt.Sprintf("%s/%s/%s:%s", info.registry, info.org, info.name, info.tag),
		},
		{
			name: "with registry and org",
			ref:  fmt.Sprintf("%s/%s/%s", info.registry, info.org, info.name),
		},
		{
			name: "by digest",
			ref:  fmt.Sprintf("%s@%s", info.name, info.digest),
		},
		{
			name: "by digest with org",
			ref:  fmt.Sprintf("%s/%s@%s", info.org, info.name, info.digest),
		},
		{
			name: "by digest with registry and org",
			ref:  fmt.Sprintf("%s/%s/%s@%s", info.registry, info.org, info.name, info.digest),
		},
		{
			name: "full model ID",
			ref:  info.modelID,
		},
		{
			name: "truncated model ID (12 chars)",
			ref:  info.modelID[7:19],
		},
		{
			name: "model ID without sha256 prefix",
			ref:  strings.TrimPrefix(info.modelID, "sha256:"),
		},
	}

	return cases
}

// setupTestEnv creates the complete test environment with registry and DMR
func setupTestEnv(t *testing.T) *testEnv {
	ctx := t.Context()

	// Set environment variables for the test process to match the DMR container.
	// This ensures CLI functions use the same default registry when parsing references.
	t.Setenv("DEFAULT_REGISTRY", "registry.local:5000")
	t.Setenv("INSECURE_REGISTRY", "true")

	// Create a custom network for container communication
	net, err := network.New(ctx)
	require.NoError(t, err)
	testcontainers.CleanupNetwork(t, net)

	registryURL := ociRegistry(t, ctx, net)
	dmrURL := dockerModelRunner(t, ctx, net)

	modelRunnerCtx, err := desktop.NewContextForTest(dmrURL, nil, types.ModelRunnerEngineKindMoby)
	require.NoError(t, err, "Failed to create model runner context")

	client := desktop.New(modelRunnerCtx)
	if !client.Status().Running {
		t.Fatal("DMR is not running")
	}

	return &testEnv{
		ctx:         ctx,
		registryURL: registryURL,
		client:      client,
		net:         net,
	}
}

func ociRegistry(t *testing.T, ctx context.Context, net *testcontainers.DockerNetwork) string {
	t.Log("Starting OCI registry container...")
	ctr, err := tc.Run(t.Context(), "registry:3",
		network.WithNetwork([]string{"registry.local"}, net),
	)
	require.NoError(t, err)
	testcontainers.CleanupContainer(t, ctr)

	registryEndpoint, err := ctr.Endpoint(ctx, "")
	require.NoError(t, err)
	registryURL := fmt.Sprintf("http://%s", registryEndpoint)
	t.Logf("Registry available at: %s", registryURL)
	return registryURL
}

// dmrConfig holds configuration options for Docker Model Runner container.
type dmrConfig struct {
	envVars map[string]string // Optional environment variables to set
	logMsg  string            // Custom log message (defaults to "Starting DMR container...")
}

// startDockerModelRunner starts a DMR container with the given configuration.
// If config.envVars is nil or empty, no extra environment variables are set.
func startDockerModelRunner(t *testing.T, ctx context.Context, net *testcontainers.DockerNetwork, config dmrConfig) string {
	containerCustomizerOpts := []testcontainers.ContainerCustomizer{
		testcontainers.WithExposedPorts("12434/tcp"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/engines/status").WithPort("12434/tcp").WithStartupTimeout(10 * time.Second)),
		network.WithNetwork([]string{"dmr"}, net),
	}

	// Add environment variables if provided
	if len(config.envVars) > 0 {
		containerCustomizerOpts = append(containerCustomizerOpts, testcontainers.WithEnv(config.envVars))
	}

	if os.Getenv("BUILD_DMR") == "1" {
		t.Log("Building DMR container...")
		out, err := exec.CommandContext(ctx, "make", "-C", "../../..", "docker-build").CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to build DMR container: %v\n%s", err, out)
		}
	} else {
		// Always pull the image if it's not build locally.
		containerCustomizerOpts = append(containerCustomizerOpts, testcontainers.WithAlwaysPull())
	}

	logMsg := config.logMsg
	if logMsg == "" {
		logMsg = "Starting DMR container..."
	}
	t.Log(logMsg)

	ctr, err := testcontainers.Run(
		ctx, "docker/model-runner:latest",
		containerCustomizerOpts...,
	)
	require.NoError(t, err)
	testcontainers.CleanupContainer(t, ctr)

	dmrEndpoint, err := ctr.Endpoint(ctx, "")
	require.NoError(t, err)

	dmrURL := fmt.Sprintf("http://%s", dmrEndpoint)
	t.Logf("DMR available at: %s", dmrURL)
	return dmrURL
}

// dockerModelRunner starts a DMR container configured for local registry tests.
// Sets DEFAULT_REGISTRY and INSECURE_REGISTRY environment variables.
func dockerModelRunner(t *testing.T, ctx context.Context, net *testcontainers.DockerNetwork) string {
	return startDockerModelRunner(t, ctx, net, dmrConfig{
		envVars: map[string]string{
			"DEFAULT_REGISTRY":  "registry.local:5000",
			"INSECURE_REGISTRY": "true",
		},
	})
}

// removeModel removes a model from the local store
func removeModel(client *desktop.Client, modelID string, force bool) error {
	_, err := client.Remove([]string{modelID}, force)
	return err
}

// verifyModelInspect inspects a model using the given reference and verifies it matches expectations
func verifyModelInspect(t *testing.T, client *desktop.Client, ref, expectedID, expectedDigest string) {
	t.Helper()

	model, err := client.Inspect(ref, false)
	require.NoError(t, err, "Failed to inspect model with reference: %s", ref)

	// Verify model ID matches
	require.Equal(t, expectedID, model.ID,
		"Model ID mismatch when inspecting with reference: %s. Expected: %s, Got: %s",
		ref, expectedID, model.ID)

	// Verify digest matches
	require.Equal(t, expectedDigest, model.ID,
		"Model digest mismatch when inspecting with reference: %s. Expected: %s, Got: %s",
		ref, expectedDigest, model.ID)

	// Verify model has tags
	require.NotEmpty(t, model.Tags, "Model should have at least one tag")

	t.Logf("✓ Successfully inspected model with reference: %s (ID: %s, Tags: %v)",
		ref, model.ID[7:19], model.Tags)
}

// createAndPushTestModel creates a minimal test model and pushes it to the local registry.
// Returns the model ID, FQDNs for host and network access, and the manifest digest.
func createAndPushTestModel(t *testing.T, registryURL, modelRef string, contextSize *int32) (modelID, hostFQDN, networkFQDN, digest string) {
	ctx := t.Context()

	// Use the dummy GGUF file from assets
	dummyGGUFPath := filepath.Join("../../../assets/dummy.gguf")
	absPath, err := filepath.Abs(dummyGGUFPath)
	require.NoError(t, err)

	// Check if the file exists
	_, err = os.Stat(absPath)
	require.NoError(t, err, "dummy.gguf not found at %s", absPath)

	// Create a builder from the GGUF file
	t.Logf("Creating test model %s from %s", modelRef, absPath)
	pkg, err := builder.FromPath(absPath)
	require.NoError(t, err)

	// Set context size if specified
	if contextSize != nil {
		pkg = pkg.WithContextSize(*contextSize)
	}

	// Construct the full reference with the local registry host for pushing from test host
	uri, err := url.Parse(registryURL)
	require.NoError(t, err)

	hostFQDN = fmt.Sprintf("%s/%s", uri.Host, modelRef)
	t.Logf("Pushing to local registry: %s", hostFQDN)

	// Create registry client
	client := registry.NewClient(registry.WithUserAgent("integration-test/1.0"))
	target, err := client.NewTarget(hostFQDN)
	require.NoError(t, err)

	// Push the model
	err = pkg.Build(ctx, target, io.Discard)
	require.NoError(t, err)
	t.Logf("Successfully pushed test model: %s", hostFQDN)

	// For pulling from DMR, use the network alias "registry.local:5000"
	// go-containerregistry will automatically use HTTP for .local hostnames
	networkFQDN = fmt.Sprintf("registry.local:5000/%s", modelRef)

	id, err := pkg.Model().ID()
	require.NoError(t, err)
	t.Logf("Model ID: %s", id)

	// Get the manifest digest
	manifestDigest, err := pkg.Model().Digest()
	require.NoError(t, err)
	digest = manifestDigest.String()
	t.Logf("Model digest: %s", digest)

	return id, hostFQDN, networkFQDN, digest
}

// TestIntegration_PullModel tests pulling a model from the local OCI registry via DMR
// with various model reference formats to ensure proper normalization.
func TestIntegration_PullModel(t *testing.T) {
	env := setupTestEnv(t)

	models, err := listModels(false, env.client, true, false, "")
	require.NoError(t, err)

	if len(models) != 0 {
		t.Fatal("Expected no initial models, but found some")
	}

	// Create and push two test models with different organizations
	// Model 1: custom org (test/test-model:latest)
	modelRef1 := "test/test-model:latest"
	modelID1, hostFQDN1, networkFQDN1, digest1 := createAndPushTestModel(t, env.registryURL, modelRef1, int32ptr(2048))
	t.Logf("Test model 1 pushed: %s (ID: %s) FQDN: %s Digest: %s", hostFQDN1, modelID1, networkFQDN1, digest1)

	// Generate test cases for custom org model (test/test-model)
	customOrgInfo := modelInfo{
		name:         "test-model",
		org:          "test",
		tag:          "latest",
		registry:     "registry.local:5000",
		modelID:      modelID1,
		digest:       digest1,
		expectedName: "test/test-model:latest",
	}
	customOrgCases := generateReferenceTestCases(customOrgInfo)

	// Model 2: default org (ai/test-model:latest)
	modelRef2 := "ai/test-model:latest"
	modelID2, hostFQDN2, networkFQDN2, digest2 := createAndPushTestModel(t, env.registryURL, modelRef2, int32ptr(2048))
	t.Logf("Test model 2 pushed: %s (ID: %s) FQDN: %s Digest: %s", hostFQDN2, modelID2, networkFQDN2, digest2)

	// Generate test cases for default org model (ai/test-model)
	defaultOrgInfo := modelInfo{
		name:         "test-model",
		org:          "ai",
		tag:          "latest",
		registry:     "registry.local:5000",
		modelID:      modelID2,
		digest:       digest2,
		expectedName: "ai/test-model:latest",
	}
	defaultOrgCases := generateReferenceTestCases(defaultOrgInfo)

	// Combine test cases with expected model IDs
	// References without explicit org should resolve to ai/ (default org)
	// References with explicit "test" org should resolve to test/test-model
	type pullTestCase struct {
		referenceTestCase
		expectedModelID   string
		expectedModelName string
		expectedDigest    string
	}

	var testCases []pullTestCase

	// Add custom org cases (with explicit "test" org in reference)
	// Only include cases where the reference explicitly contains the "test" org
	// Cases without explicit org will normalize to "ai/" (default org)
	for _, tc := range customOrgCases {
		// Skip ID-based references for pull tests (can't pull by ID)
		if strings.Contains(tc.name, "model ID") {
			continue
		}
		// Skip cases that don't have explicit org in the reference
		// These will normalize to the default org (ai/) and should only be in defaultOrgCases
		if !strings.Contains(tc.ref, "test/") && !strings.Contains(tc.ref, "registry.local:5000/test/") {
			continue
		}
		testCases = append(testCases, pullTestCase{
			referenceTestCase: tc,
			expectedModelID:   modelID1,
			expectedModelName: "test/test-model:latest",
			expectedDigest:    digest1,
		})
	}

	// Add default org cases (references that should default to ai/)
	for _, tc := range defaultOrgCases {
		// Skip ID-based references for pull tests (can't pull by ID)
		if strings.Contains(tc.name, "model ID") {
			continue
		}
		testCases = append(testCases, pullTestCase{
			referenceTestCase: tc,
			expectedModelID:   modelID2,
			expectedModelName: "ai/test-model:latest",
			expectedDigest:    digest2,
		})
	}

	// Run all test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Pull the model using the test case reference
			t.Logf("Pulling model with reference: %s", tc.ref)
			err := pullModel(newPullCmd(), env.client, tc.ref)
			require.NoError(t, err, "Failed to pull model with reference: %s", tc.ref)

			// List models and verify the expected model is present
			models, err := listModels(false, env.client, true, false, "")
			require.NoError(t, err)

			if len(models) == 0 {
				t.Fatalf("No models found after pulling %s", tc.ref)
			}

			models = strings.TrimSpace(models)

			// Extract truncated ID format (sha256:xxx... -> xxx where xxx is 12 chars)
			// listModels with quiet=true returns modelID[7:19]
			truncatedID := tc.expectedModelID[7:19]
			if models != truncatedID {
				t.Errorf("Expected model ID %s (truncated: %s) not found in model list after pulling %s.\nExpected model: %s\nModel list:\n%s",
					tc.expectedModelID, truncatedID, tc.ref, tc.expectedModelName, models)
			} else {
				t.Logf("✓ Successfully verified model %s (ID: %s) after pulling with reference: %s",
					tc.expectedModelName, truncatedID, tc.ref)
			}

			// Verify inspect works with the same reference used for pulling
			verifyModelInspect(t, env.client, tc.ref, tc.expectedModelID, tc.expectedDigest)

			// Clean up: remove the model for the next test iteration
			t.Logf("Removing model %s", truncatedID)
			err = removeModel(env.client, tc.expectedModelID, true)
			require.NoError(t, err, "Failed to remove model")
		})
	}
}

// TestIntegration_InspectModel tests inspecting a model with various reference formats
// to ensure proper reference normalization and consistent output.
func TestIntegration_InspectModel(t *testing.T) {
	env := setupTestEnv(t)

	// Ensure no models exist initially
	models, err := listModels(false, env.client, true, false, "")
	require.NoError(t, err)
	if len(models) != 0 {
		t.Fatal("Expected no initial models, but found some")
	}

	// Create and push a test model with default org (ai/inspect-test:latest)
	modelRef := "ai/inspect-test:latest"
	modelID, hostFQDN, networkFQDN, digest := createAndPushTestModel(t, env.registryURL, modelRef, int32ptr(2048))
	t.Logf("Test model pushed: %s (ID: %s) FQDN: %s Digest: %s", hostFQDN, modelID, networkFQDN, digest)

	// Pull the model using a short reference
	pullRef := "inspect-test"
	t.Logf("Pulling model with reference: %s", pullRef)
	err = pullModel(newPullCmd(), env.client, pullRef)
	require.NoError(t, err, "Failed to pull model")

	// Verify the model was pulled
	models, err = listModels(false, env.client, true, false, "")
	require.NoError(t, err)
	truncatedID := modelID[7:19]
	require.Equal(t, truncatedID, strings.TrimSpace(models), "Model not found after pull")

	// Generate all reference test cases using the unified system
	info := modelInfo{
		name:         "inspect-test",
		org:          "ai",
		tag:          "latest",
		registry:     "registry.local:5000",
		modelID:      modelID,
		digest:       digest,
		expectedName: "ai/inspect-test:latest",
	}
	testCases := generateReferenceTestCases(info)

	// Verify inspect works with all reference formats
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			verifyModelInspect(t, env.client, tc.ref, modelID, digest)
		})
	}

	// Cleanup: remove the model
	t.Logf("Removing model %s", truncatedID)
	err = removeModel(env.client, modelID, true)
	require.NoError(t, err, "Failed to remove model")

	// Verify model was removed
	models, err = listModels(false, env.client, true, false, "")
	require.NoError(t, err)
	require.Empty(t, strings.TrimSpace(models), "Model should be removed")
}

// TestIntegration_TagModel tests tagging a model with various source and target reference formats
// to ensure proper reference normalization and tag creation.
func TestIntegration_TagModel(t *testing.T) {
	env := setupTestEnv(t)

	// Ensure no models exist initially
	models, err := listModels(false, env.client, true, false, "")
	require.NoError(t, err)
	if len(models) != 0 {
		t.Fatal("Expected no initial models, but found some")
	}

	// Create and push a test model with default org (ai/tag-test:latest)
	modelRef := "ai/tag-test:latest"
	modelID, hostFQDN, networkFQDN, digest := createAndPushTestModel(t, env.registryURL, modelRef, int32ptr(2048))
	t.Logf("Test model pushed: %s (ID: %s) FQDN: %s Digest: %s", hostFQDN, modelID, networkFQDN, digest)

	// Pull the model using a simple reference
	pullRef := "tag-test"
	t.Logf("Pulling model with reference: %s", pullRef)
	err = pullModel(newPullCmd(), env.client, pullRef)
	require.NoError(t, err, "Failed to pull model")

	// Verify the model was pulled
	models, err = listModels(false, env.client, true, false, "")
	require.NoError(t, err)
	truncatedID := modelID[7:19]
	require.Equal(t, truncatedID, strings.TrimSpace(models), "Model not found after pull")

	// Generate all possible source references using the unified system
	sourceInfo := modelInfo{
		name:         "tag-test",
		org:          "ai",
		tag:          "latest",
		registry:     "registry.local:5000",
		modelID:      modelID,
		digest:       digest,
		expectedName: "ai/tag-test:v1",
	}
	sourceRefs := generateReferenceTestCases(sourceInfo)

	// Define target reference formats to test (including explicit registry references)
	targetFormats := []struct {
		name   string
		target string
	}{
		{name: "simple name", target: "tag-test"},
		{name: "simple name with tag", target: "target-tag-test:v2"},
		{name: "with org", target: "ai/target-tag-test:latest"},
		{name: "different org", target: "test/target-tag-test:v1"},
		{name: "custom org", target: "custom/target-tag-test:cross-org"},
		{name: "explicit registry with org", target: "registry.local:5000/ai/target-tag-test:explicit"},
		{name: "explicit registry different org", target: "registry.local:5000/target-test/tag-test:fqdn"},
		{name: "explicit registry custom org", target: "registry.local:5000/custom/target-tag-test:with-reg"},
		{name: "explicit custom registry custom org", target: "other-registry.local:5000/custom/target-tag-test:with-reg"},
	}

	// Build test cases by combining source references with target formats
	type tagTestCase struct {
		name      string
		sourceRef string
		targetRef string
	}

	var testCases []tagTestCase

	// Test all combinations of source references and target formats
	for _, srcCase := range sourceRefs {

		if strings.Contains(srcCase.name, "model ID") {
			// Skip ID-based references for tagging tests
			// TODO : Support tagging by ID in the future
			continue
		}

		// Nested loop - test this source with ALL targets
		for _, targetFormat := range targetFormats {
			testCases = append(testCases, tagTestCase{
				name:      fmt.Sprintf("source: %s -> target: %s", srcCase.name, targetFormat.name),
				sourceRef: srcCase.ref,
				targetRef: targetFormat.target,
			})
		}
	}

	// Track all created tags for verification
	createdTags := make(map[string]bool)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Tagging %s as %s", tc.sourceRef, tc.targetRef)

			// Perform the tag operation
			err := tagModel(newTagCmd(), env.client, tc.sourceRef, tc.targetRef)
			require.NoError(t, err, "Failed to tag model with source=%s target=%s", tc.sourceRef, tc.targetRef)

			// Track this tag
			createdTags[tc.targetRef] = true

			// Verify the new tag exists and points to the correct model
			taggedModel, err := env.client.Inspect(tc.targetRef, false)
			require.NoError(t, err, "Failed to inspect newly tagged model with reference: %s", tc.targetRef)

			// Verify the model ID matches the original
			require.Equal(t, modelID, taggedModel.ID,
				"Tagged model ID mismatch. Expected: %s, Got: %s", modelID, taggedModel.ID)

			// Verify the digest matches
			require.Equal(t, digest, taggedModel.ID,
				"Tagged model digest mismatch. Expected: %s, Got: %s", digest, taggedModel.ID)

			t.Logf("✓ Successfully created tag %s pointing to model %s", tc.targetRef, truncatedID)

			// Verify the original source tag/reference still exists
			originalModel, err := env.client.Inspect(tc.sourceRef, false)
			require.NoError(t, err, "Original source reference should still be valid: %s", tc.sourceRef)
			require.Equal(t, modelID, originalModel.ID, "Original source should point to same model")
			t.Logf("✓ Verified original source %s still exists", tc.sourceRef)
		})
	}

	// Test error case: tagging non-existent model
	t.Run("error on non-existent model", func(t *testing.T) {
		err := tagModel(newTagCmd(), env.client, "non-existent-model:v1", "ai/should-fail:latest")
		require.Error(t, err, "Should fail when tagging non-existent model")
		t.Logf("✓ Correctly failed to tag non-existent model: %v", err)
	})

	// Cleanup: remove the model
	t.Logf("Removing model %s", truncatedID)
	err = removeModel(env.client, modelID, true)
	require.NoError(t, err, "Failed to remove model")

	// Verify model was removed
	models, err = listModels(false, env.client, true, false, "")
	require.NoError(t, err)
	require.Empty(t, strings.TrimSpace(models), "Model should be removed")
}

// TestIntegration_PushModel tests pushing a model to registries with various reference formats
// to ensure proper reference normalization and successful push operations.
func TestIntegration_PushModel(t *testing.T) {
	env := setupTestEnv(t)

	// Set up custom registry with different alias
	t.Log("Starting custom OCI registry container...")
	customRegistryCtr, err := tc.Run(t.Context(), "registry:3",
		network.WithNetwork([]string{"custom-registry.local"}, env.net),
	)
	require.NoError(t, err)
	testcontainers.CleanupContainer(t, customRegistryCtr)

	customRegistryEndpoint, err := customRegistryCtr.Endpoint(t.Context(), "")
	require.NoError(t, err)
	customRegistryURL := fmt.Sprintf("http://%s", customRegistryEndpoint)
	t.Logf("Custom registry available at: %s", customRegistryURL)

	// Ensure no models exist initially
	models, err := listModels(false, env.client, true, false, "")
	require.NoError(t, err)
	if len(models) != 0 {
		t.Fatal("Expected no initial models, but found some")
	}

	// Create and push a test model with default org (ai/tag-test:latest)
	modelRef := "ai/tag-test:latest"
	modelID, hostFQDN, networkFQDN, digest := createAndPushTestModel(t, env.registryURL, modelRef, int32ptr(2048))
	t.Logf("Test model pushed: %s (ID: %s) FQDN: %s Digest: %s", hostFQDN, modelID, networkFQDN, digest)

	// Pull the model using a simple reference
	pullRef := "tag-test"
	t.Logf("Pulling model with reference: %s", pullRef)
	err = pullModel(newPullCmd(), env.client, pullRef)
	require.NoError(t, err, "Failed to pull model")

	// Verify the model was pulled
	models, err = listModels(false, env.client, true, false, "")
	require.NoError(t, err)
	truncatedID := modelID[7:19]
	require.Equal(t, truncatedID, strings.TrimSpace(models), "Model not found after pull")

	// Test 1: Push to default registry with various reference formats
	t.Run("push to default registry", func(t *testing.T) {
		// Generate test cases for pushing to default registry
		pushInfo := modelInfo{
			name:         "push-test",
			org:          "ai",
			tag:          "v1",
			registry:     "registry.local:5000",
			modelID:      modelID,
			digest:       digest,
			expectedName: "ai/push-test:v1",
		}
		testCases := generateReferenceTestCases(pushInfo)

		for _, tc := range testCases {
			// Skip ID-based or digest-based references for push tests
			if strings.Contains(tc.name, "model ID") || strings.Contains(tc.name, "digest") {
				continue
			}

			t.Run(tc.name, func(t *testing.T) {
				// First tag the model with the desired reference
				t.Logf("Tagging %s as %s", "tag-test", tc.ref)
				err := tagModel(newTagCmd(), env.client, "tag-test", tc.ref)
				require.NoError(t, err, "Failed to tag model for custom registry")

				// Push the tagged model
				t.Logf("Pushing model to custom registry with reference: %s", tc.ref)
				_, _, err = env.client.Push(tc.ref, desktop.NewSimplePrinter(func(msg string) {
					t.Logf("Progress: %s", msg)
				}))
				require.NoError(t, err, "Failed to push model to custom registry")
				t.Logf("✓ Successfully pushed model to custom registry: %s", tc.ref)
			})
		}
	})

	// Test 2: Push to custom registry with explicit registry in reference
	t.Run("push to custom registry", func(t *testing.T) {
		customTestCases := []struct {
			name      string
			sourceRef string
			targetRef string
		}{
			{
				name:      "push with custom registry and org",
				sourceRef: "tag-test",
				targetRef: "custom-registry.local:5000/ai/push-test:custom",
			},
			{
				name:      "push with custom registry and different org",
				sourceRef: "tag-test",
				targetRef: "custom-registry.local:5000/test/push-test:v2",
			},
			{
				name:      "push with custom registry FQDN",
				sourceRef: "tag-test",
				targetRef: "custom-registry.local:5000/custom/push-test:latest",
			},
		}

		for _, tc := range customTestCases {
			t.Run(tc.name, func(t *testing.T) {
				// First tag the model with the custom registry reference
				t.Logf("Tagging %s as %s", tc.sourceRef, tc.targetRef)
				err := tagModel(newTagCmd(), env.client, tc.sourceRef, tc.targetRef)
				require.NoError(t, err, "Failed to tag model for custom registry")

				// Push the tagged model
				t.Logf("Pushing model to custom registry with reference: %s", tc.targetRef)
				_, _, err = env.client.Push(tc.targetRef, desktop.NewSimplePrinter(func(msg string) {
					t.Logf("Progress: %s", msg)
				}))
				require.NoError(t, err, "Failed to push model to custom registry")
				t.Logf("✓ Successfully pushed model to custom registry: %s", tc.targetRef)
			})
		}
	})

	// Test 3: Error cases
	t.Run("error cases", func(t *testing.T) {
		t.Run("push non-existent model", func(t *testing.T) {
			_, _, err := env.client.Push("non-existent-model:v1", desktop.NewSimplePrinter(func(msg string) {}))
			require.Error(t, err, "Should fail when pushing non-existent model")
			t.Logf("✓ Correctly failed to push non-existent model: %v", err)
		})

		t.Run("push with invalid reference", func(t *testing.T) {
			_, _, err := env.client.Push("", desktop.NewSimplePrinter(func(msg string) {}))
			require.Error(t, err, "Should fail with empty reference")
			t.Logf("✓ Correctly failed to push with invalid reference: %v", err)
		})
	})

	// Final cleanup: remove the model
	t.Logf("Removing model %s", truncatedID)
	err = removeModel(env.client, modelID, true)
	require.NoError(t, err, "Failed to remove model")

	// Verify model was removed
	models, err = listModels(false, env.client, true, false, "")
	require.NoError(t, err)
	require.Empty(t, strings.TrimSpace(models), "Model should be removed")
}

// TestIntegration_RemoveModel tests removing models with various reference formats
// to ensure proper reference normalization and correct removal behavior.
func TestIntegration_RemoveModel(t *testing.T) {
	env := setupTestEnv(t)

	// Ensure no models exist initially
	models, err := listModels(false, env.client, true, false, "")
	require.NoError(t, err)
	if len(models) != 0 {
		t.Fatal("Expected no initial models, but found some")
	}

	// Create and push a test model with default org (ai/rm-test:latest)
	modelRef := "ai/rm-test:latest"
	modelID, hostFQDN, networkFQDN, digest := createAndPushTestModel(t, env.registryURL, modelRef, int32ptr(2048))
	t.Logf("Test model pushed: %s (ID: %s) FQDN: %s Digest: %s", hostFQDN, modelID, networkFQDN, digest)

	// Generate all reference test cases
	info := modelInfo{
		name:         "rm-test",
		org:          "ai",
		tag:          "latest",
		registry:     "registry.local:5000",
		modelID:      modelID,
		digest:       digest,
		expectedName: "ai/rm-test:latest",
	}
	testCases := generateReferenceTestCases(info)

	// Remove model using various reference formats
	t.Run("remove with various reference formats", func(t *testing.T) {
		for _, tc := range testCases {

			t.Run(tc.name, func(t *testing.T) {
				// Pull the model
				pullRef := "rm-test"
				t.Logf("Pulling model with reference: %s", pullRef)
				err := pullModel(newPullCmd(), env.client, pullRef)
				require.NoError(t, err, "Failed to pull model")

				// Verify model exists
				models, err := listModels(false, env.client, true, false, "")
				require.NoError(t, err)
				truncatedID := modelID[7:19]
				require.Equal(t, truncatedID, strings.TrimSpace(models), "Model not found after pull")

				// Remove using the test case reference
				t.Logf("Removing model with reference: %s", tc.ref)
				err = removeModel(env.client, tc.ref, false)
				require.NoError(t, err, "Failed to remove model with reference: %s", tc.ref)

				// Verify model is removed
				models, err = listModels(false, env.client, true, false, "")
				require.NoError(t, err)
				require.Empty(t, strings.TrimSpace(models), "Model should be removed after rm with reference: %s", tc.ref)

				t.Logf("✓ Successfully removed model using reference: %s", tc.ref)
			})
		}
	})

	// Remove multiple models in one command
	t.Run("remove multiple models", func(t *testing.T) {
		// Create and push two different models
		modelRef1 := "ai/rm-multi-1:latest"
		modelID1, _, _, _ := createAndPushTestModel(t, env.registryURL, modelRef1, int32ptr(2048))
		modelRef2 := "ai/rm-multi-2:latest"
		modelID2, _, _, _ := createAndPushTestModel(t, env.registryURL, modelRef2, int32ptr(2048))

		// Pull both models
		t.Logf("Pulling first model: rm-multi-1")
		err := pullModel(newPullCmd(), env.client, "rm-multi-1")
		require.NoError(t, err, "Failed to pull first model")

		t.Logf("Pulling second model: rm-multi-2")
		err = pullModel(newPullCmd(), env.client, "rm-multi-2")
		require.NoError(t, err, "Failed to pull second model")

		// Verify both models exist
		models, err := listModels(false, env.client, false, false, "")
		require.NoError(t, err)
		require.Contains(t, models, modelID1[7:19], "First model should exist")
		require.Contains(t, models, modelID2[7:19], "Second model should exist")

		// Remove both models in one command
		t.Logf("Removing both models: rm-multi-1 and rm-multi-2")
		_, err = env.client.Remove([]string{"rm-multi-1", "rm-multi-2"}, false)
		require.NoError(t, err, "Failed to remove multiple models")

		// Verify both models are removed
		models, err = listModels(false, env.client, true, false, "")
		require.NoError(t, err)
		require.Empty(t, strings.TrimSpace(models), "All models should be removed")

		t.Logf("✓ Successfully removed multiple models in one command")
	})

	// Tag-specific removal (removing one tag keeps others)
	t.Run("remove specific tag keeps other tags", func(t *testing.T) {
		// Pull the model
		t.Logf("Pulling model: rm-test")
		err := pullModel(newPullCmd(), env.client, "rm-test")
		require.NoError(t, err, "Failed to pull model")

		// Add multiple tags to the same model
		t.Logf("Adding tags v1, v2, and v3 to the model")
		err = tagModel(newTagCmd(), env.client, "rm-test", "rm-test:v1")
		require.NoError(t, err, "Failed to create v1 tag")
		err = tagModel(newTagCmd(), env.client, "rm-test", "rm-test:v2")
		require.NoError(t, err, "Failed to create v2 tag")
		err = tagModel(newTagCmd(), env.client, "rm-test", "rm-test:v3")
		require.NoError(t, err, "Failed to create v3 tag")

		// Verify all tags exist
		model, err := env.client.Inspect(modelID, false)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(model.Tags), 4, "Model should have at least 4 tags")
		t.Logf("Model has %d tags: %v", len(model.Tags), model.Tags)

		// Remove one specific tag (v1)
		t.Logf("Removing only the v1 tag")
		err = removeModel(env.client, "rm-test:v1", false)
		require.NoError(t, err, "Failed to remove v1 tag")

		// Verify the model still exists with remaining tags
		model, err = env.client.Inspect(modelID, false)
		require.NoError(t, err, "Model should still exist after removing one tag")

		// Verify v1 tag is gone but others remain
		tagFound := false
		for _, tag := range model.Tags {
			if tag == "rm-test:v1" || tag == "ai/rm-test:v1" {
				tagFound = true
				break
			}
		}
		require.False(t, tagFound, "v1 tag should be removed")

		// Verify other tags still exist
		v2Found := false
		v3Found := false
		for _, tag := range model.Tags {
			if strings.Contains(tag, ":v2") {
				v2Found = true
			}
			if strings.Contains(tag, ":v3") {
				v3Found = true
			}
		}
		require.True(t, v2Found, "v2 tag should still exist")
		require.True(t, v3Found, "v3 tag should still exist")

		t.Logf("✓ Successfully removed specific tag while keeping others")

		// Cleanup: remove the entire model (force=true since multiple tags remain)
		err = removeModel(env.client, modelID, true)
		require.NoError(t, err, "Failed to cleanup model")
	})

	// Model ID removal removes all tags
	t.Run("remove by model ID removes all tags", func(t *testing.T) {
		// Pull the model
		t.Logf("Pulling model: rm-test")
		err := pullModel(newPullCmd(), env.client, "rm-test")
		require.NoError(t, err, "Failed to pull model")

		// Add multiple tags
		t.Logf("Adding multiple tags to the model")
		err = tagModel(newTagCmd(), env.client, "rm-test", "rm-test:tag1")
		require.NoError(t, err, "Failed to create tag1")
		err = tagModel(newTagCmd(), env.client, "rm-test", "rm-test:tag2")
		require.NoError(t, err, "Failed to create tag2")
		err = tagModel(newTagCmd(), env.client, "rm-test", "rm-test:tag3")
		require.NoError(t, err, "Failed to create tag3")

		// Verify tags exist
		model, err := env.client.Inspect(modelID, false)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(model.Tags), 4, "Model should have multiple tags")
		t.Logf("Model has %d tags before ID removal", len(model.Tags))

		// Remove by model ID (should remove entire model and all tags)
		t.Logf("Removing by model ID: %s", modelID)
		err = removeModel(env.client, modelID, false)
		if !strings.Contains(err.Error(), "(must be forced) due to multiple tag references") {
			t.Fatalf("Expected error about multiple tag references when removing by ID without force, got: %v", err)
		}
		require.Error(t, err, "(must be forced) due to multiple tag references")
	})

	// Force flag behavior
	t.Run("force flag", func(t *testing.T) {
		// Pull the model
		t.Logf("Pulling model: rm-test")
		err := pullModel(newPullCmd(), env.client, "rm-test")
		require.NoError(t, err, "Failed to pull model")

		// Test removal with force flag
		t.Logf("Removing model with force flag")
		err = removeModel(env.client, modelID, true)
		require.NoError(t, err, "Failed to remove with force flag")

		// Verify model is removed
		models, err := listModels(false, env.client, true, false, "")
		require.NoError(t, err)
		require.Empty(t, strings.TrimSpace(models), "Model should be removed with force flag")

		t.Logf("✓ Successfully removed model with force flag")
	})

	// Error cases
	t.Run("error cases", func(t *testing.T) {
		t.Run("remove non-existent model", func(t *testing.T) {
			err := removeModel(env.client, "non-existent-model:v1", false)
			require.Error(t, err, "Should fail when removing non-existent model")
			t.Logf("✓ Correctly failed to remove non-existent model: %v", err)
		})

		t.Run("force remove non-existent model", func(t *testing.T) {
			err := removeModel(env.client, "non-existent-model:v1", true)
			require.Error(t, err, "Should fail when removing non-existent model")
			t.Logf("✓ Correctly failed to remove non-existent model: %v", err)
		})

		t.Run("remove with empty reference", func(t *testing.T) {
			_, err := env.client.Remove([]string{""}, false)
			require.Error(t, err, "Should fail with empty reference")
			t.Logf("✓ Correctly failed to remove with empty reference: %v", err)
		})

		t.Run("remove with invalid reference", func(t *testing.T) {
			err := removeModel(env.client, "invalid:reference:format", false)
			require.Error(t, err, "Should fail with invalid reference format")
			t.Logf("✓ Correctly failed to remove with invalid reference: %v", err)
		})
	})
}

// TestIntegration_PackageModel tests packaging a GGUF model file
// to ensure the model is properly loaded and tagged in the model store.
// This test reproduces issue #461 where packaging fails with "model not found" during tagging.
func TestIntegration_PackageModel(t *testing.T) {
	env := setupTestEnv(t)

	// Ensure no models exist initially
	models, err := listModels(false, env.client, true, false, "")
	require.NoError(t, err)
	if len(models) != 0 {
		t.Fatal("Expected no initial models, but found some")
	}

	// Use the dummy GGUF file from assets
	dummyGGUFPath := filepath.Join("../../../assets/dummy.gguf")
	absPath, err := filepath.Abs(dummyGGUFPath)
	require.NoError(t, err)

	// Check if the file exists
	_, err = os.Stat(absPath)
	require.NoError(t, err, "dummy.gguf not found at %s", absPath)

	// Test case 1: Package a GGUF file with a simple tag
	t.Run("package GGUF with simple tag", func(t *testing.T) {
		targetTag := "ai/packaged-test:latest"

		// Create package options
		opts := packageOptions{
			ggufPath: absPath,
			tag:      targetTag,
		}

		// Execute the package command using the helper function with test client
		t.Logf("Packaging GGUF file %s as %s", absPath, targetTag)
		err := packageModel(env.ctx, newPackagedCmd(), env.client, opts)
		require.NoError(t, err, "Failed to package GGUF model")

		// Verify the model was loaded and tagged
		t.Logf("Verifying model was loaded and tagged")
		models, err := listModels(false, env.client, false, false, "")
		require.NoError(t, err)
		require.NotEmpty(t, models, "No models found after packaging")

		// Verify we can inspect the model by tag
		model, err := env.client.Inspect(targetTag, false)
		require.NoError(t, err, "Failed to inspect packaged model by tag: %s", targetTag)
		require.NotEmpty(t, model.ID, "Model ID should not be empty")
		require.Contains(t, model.Tags, normalizeRef(t, targetTag), "Model should have the expected tag")

		t.Logf("✓ Successfully packaged and tagged model: %s (ID: %s)", targetTag, model.ID[7:19])

		// Cleanup
		err = removeModel(env.client, model.ID, true)
		require.NoError(t, err, "Failed to remove model")
	})

	// Test case 2: Package with context size override
	t.Run("package GGUF with context size", func(t *testing.T) {
		targetTag := "ai/packaged-ctx:latest"

		// Create package options with context size
		opts := packageOptions{
			ggufPath:    absPath,
			tag:         targetTag,
			contextSize: 4096,
		}

		// Create a command for context
		cmd := newPackagedCmd()
		// Set the flag as changed for context size
		cmd.Flags().Set("context-size", "4096")

		// Execute the package command using the helper function with test client
		t.Logf("Packaging GGUF file with context size 4096 as %s", targetTag)
		err := packageModel(env.ctx, cmd, env.client, opts)
		require.NoError(t, err, "Failed to package GGUF model with context size")

		// Verify the model was loaded and tagged
		model, err := env.client.Inspect(targetTag, false)
		require.NoError(t, err, "Failed to inspect packaged model")
		require.Contains(t, model.Tags, normalizeRef(t, targetTag), "Model should have the expected tag")

		t.Logf("✓ Successfully packaged model with context size: %s", targetTag)

		// Cleanup
		err = removeModel(env.client, model.ID, true)
		require.NoError(t, err, "Failed to remove model")
	})

	// Test case 3: Package with different org
	t.Run("package GGUF with custom org", func(t *testing.T) {
		targetTag := "myorg/packaged-test:v1"

		// Create package options
		opts := packageOptions{
			ggufPath: absPath,
			tag:      targetTag,
		}

		// Create a command for context
		cmd := newPackagedCmd()

		// Execute the package command using the helper function with test client
		t.Logf("Packaging GGUF file as %s", targetTag)
		err := packageModel(env.ctx, cmd, env.client, opts)
		require.NoError(t, err, "Failed to package GGUF model with custom org")

		// Verify the model was loaded and tagged
		model, err := env.client.Inspect(targetTag, false)
		require.NoError(t, err, "Failed to inspect packaged model")
		require.Contains(t, model.Tags, normalizeRef(t, targetTag), "Model should have the expected tag")

		t.Logf("✓ Successfully packaged model with custom org: %s", targetTag)

		// Cleanup
		err = removeModel(env.client, model.ID, true)
		require.NoError(t, err, "Failed to remove model")
	})

	// Verify all models are cleaned up
	models, err = listModels(false, env.client, true, false, "")
	require.NoError(t, err)
	require.Empty(t, strings.TrimSpace(models), "All models should be removed after cleanup")
}

func int32ptr(n int32) *int32 {
	return &n
}

// setupDockerHubTestEnv creates a test environment for Docker Hub tests.
// Unlike setupTestEnv, this does NOT set DEFAULT_REGISTRY, so it uses
// the real Docker Hub (index.docker.io) as the default registry.
// This is used to test that pulling from Docker Hub works correctly.
func setupDockerHubTestEnv(t *testing.T) *testEnv {
	ctx := t.Context()

	// Create a custom network for container communication
	net, err := network.New(ctx)
	require.NoError(t, err)
	testcontainers.CleanupNetwork(t, net)

	// dockerModelRunnerForDockerHub starts a DMR container configured for Docker Hub tests.
	// it uses the real Docker Hub as the default registry.
	dmrURL := startDockerModelRunner(t, ctx, net, dmrConfig{
		logMsg: "Starting DMR container for Docker Hub tests (no DEFAULT_REGISTRY)...",
	})

	modelRunnerCtx, err := desktop.NewContextForTest(dmrURL, nil, types.ModelRunnerEngineKindMoby)
	require.NoError(t, err, "Failed to create model runner context")

	client := desktop.New(modelRunnerCtx)
	if !client.Status().Running {
		t.Fatal("DMR is not running")
	}

	return &testEnv{
		ctx:    ctx,
		client: client,
		net:    net,
	}
}

// TestIntegration_PullFromDockerHub is a smoke test that pulls a real model
// from Docker Hub to verify that the OCI registry code works correctly
// with the real Docker Hub registry (index.docker.io -> registry-1.docker.io).
//
// This test catches regressions where the code doesn't properly handle
// Docker Hub's hostname remapping requirements.
func TestIntegration_PullFromDockerHub(t *testing.T) {
	env := setupDockerHubTestEnv(t)

	// Ensure no models exist initially
	models, err := listModels(false, env.client, true, false, "")
	require.NoError(t, err)
	if len(models) != 0 {
		t.Fatal("Expected no initial models, but found some")
	}

	// Pull a small model from Docker Hub
	// ai/smollm2:135M-Q4_0 is a small model that's quick to download
	modelRef := "ai/smollm2:135M-Q4_0"
	t.Logf("Pulling model from Docker Hub: %s", modelRef)

	err = pullModel(newPullCmd(), env.client, modelRef)
	require.NoError(t, err, "Failed to pull model from Docker Hub: %s", modelRef)

	// Verify the model was pulled
	t.Log("Verifying model was pulled successfully")
	models, err = listModels(false, env.client, true, false, "")
	require.NoError(t, err)
	require.NotEmpty(t, strings.TrimSpace(models), "Model should exist after pull from Docker Hub")

	// Verify we can inspect the model
	model, err := env.client.Inspect(modelRef, false)
	require.NoError(t, err, "Failed to inspect model pulled from Docker Hub")
	require.NotEmpty(t, model.ID, "Model ID should not be empty")

	t.Logf("✓ Successfully pulled model from Docker Hub: %s (ID: %s)", modelRef, model.ID[7:19])

	// Cleanup: remove the model
	t.Logf("Cleaning up: removing model %s", model.ID[7:19])
	err = removeModel(env.client, model.ID, true)
	require.NoError(t, err, "Failed to remove model")

	// Verify model was removed
	models, err = listModels(false, env.client, true, false, "")
	require.NoError(t, err)
	require.Empty(t, strings.TrimSpace(models), "Model should be removed after cleanup")
}

// normalizeRef normalizes a reference to its fully qualified form.
// This is used in tests to compare against the stored tags which are always normalized.
func normalizeRef(t *testing.T, ref string) string {
	t.Helper()
	parsed, err := reference.ParseReference(ref, registry.GetDefaultRegistryOptions()...)
	require.NoError(t, err, "Failed to parse reference: %s", ref)
	return parsed.String()
}
