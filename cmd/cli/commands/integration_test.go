//go:build integration

package commands

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/registry"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// testEnv holds the test environment components
type testEnv struct {
	ctx         context.Context
	registryURL string
	client      *desktop.Client
}

// setupTestEnv creates the complete test environment with registry and DMR
func setupTestEnv(t *testing.T) *testEnv {
	ctx := context.Background()

	// Create a custom network for container communication
	net, err := network.New(ctx)
	require.NoError(t, err)
	testcontainers.CleanupNetwork(t, net)

	registryURL := ociRegistry(t, ctx, net)
	dmrURL := dockerModelRunner(t, ctx, net)

	modelRunnerCtx, err := desktop.NewContextForTest(dmrURL, nil)
	require.NoError(t, err, "Failed to create model runner context")

	client := desktop.New(modelRunnerCtx)
	if !client.Status().Running {
		t.Fatal("DMR is not running")
	}

	return &testEnv{
		ctx:         ctx,
		registryURL: registryURL,
		client:      client,
	}
}

func ociRegistry(t *testing.T, ctx context.Context, net *testcontainers.DockerNetwork) string {
	t.Log("Starting OCI registry container...")
	ctr, err := testcontainers.Run(
		ctx, "registry:2",
		testcontainers.WithExposedPorts("5000/tcp"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/v2/").WithPort("5000/tcp").WithStartupTimeout(30*time.Second)),
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

func dockerModelRunner(t *testing.T, ctx context.Context, net *testcontainers.DockerNetwork) string {
	t.Log("Starting DMR container...")
	ctr, err := testcontainers.Run(
		ctx, "docker/model-runner:latest",
		testcontainers.WithExposedPorts("12434/tcp"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/engines/status").WithPort("12434/tcp").WithStartupTimeout(10*time.Second)),
		network.WithNetwork([]string{"dmr"}, net),
	)
	require.NoError(t, err)
	testcontainers.CleanupContainer(t, ctr)

	dmrEndpoint, err := ctr.Endpoint(ctx, "")
	require.NoError(t, err)

	dmrURL := fmt.Sprintf("http://%s", dmrEndpoint)
	t.Logf("DMR available at: %s", dmrURL)
	return dmrURL
}

// removeModel removes a model from the local store
func removeModel(client *desktop.Client, modelID string) error {
	_, err := client.Remove([]string{modelID}, true)
	return err
}

// createAndPushTestModel creates a minimal test model and pushes it to the local registry.
// Returns the model ID and FQDNs for host and network access.
func createAndPushTestModel(t *testing.T, registryURL, modelRef string, contextSize uint64) (modelID, hostFQDN, networkFQDN string) {
	ctx := context.Background()

	// Use the dummy GGUF file from assets
	dummyGGUFPath := filepath.Join("../../../assets/dummy.gguf")
	absPath, err := filepath.Abs(dummyGGUFPath)
	require.NoError(t, err)

	// Check if the file exists
	_, err = os.Stat(absPath)
	require.NoError(t, err, "dummy.gguf not found at %s", absPath)

	// Create a builder from the GGUF file
	t.Logf("Creating test model %s from %s", modelRef, absPath)
	pkg, err := builder.FromGGUF(absPath)
	require.NoError(t, err)

	// Set context size if specified
	if contextSize > 0 {
		pkg = pkg.WithContextSize(contextSize)
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

	return id, hostFQDN, networkFQDN
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
	modelID1, hostFQDN1, networkFQDN1 := createAndPushTestModel(t, env.registryURL, modelRef1, 2048)
	t.Logf("Test model 1 pushed: %s (ID: %s)", hostFQDN1, modelID1)

	// Model 2: default org (ai/test-model:latest)
	modelRef2 := "ai/test-model:latest"
	modelID2, hostFQDN2, networkFQDN2 := createAndPushTestModel(t, env.registryURL, modelRef2, 2048)
	t.Logf("Test model 2 pushed: %s (ID: %s)", hostFQDN2, modelID2)

	// Test cases for different model reference formats
	testCases := []struct {
		name              string
		pullRef           string // Reference to use when pulling
		expectedModelID   string // Expected model ID after pull
		expectedModelName string // Expected model name for logging
	}{
		{
			name:              "explicit custom org and tag",
			pullRef:           networkFQDN1, // registry.local:5000/test/test-model:latest
			expectedModelID:   modelID1,
			expectedModelName: "test/test-model:latest",
		},
		{
			name:              "custom org without tag (should default to :latest)",
			pullRef:           "registry.local:5000/test/test-model",
			expectedModelID:   modelID1,
			expectedModelName: "test/test-model:latest",
		},
		{
			name:              "explicit default org and tag",
			pullRef:           networkFQDN2, // registry.local:5000/ai/test-model:latest
			expectedModelID:   modelID2,
			expectedModelName: "ai/test-model:latest",
		},
		{
			name:              "default org without tag (should default to :latest)",
			pullRef:           "registry.local:5000/ai/test-model",
			expectedModelID:   modelID2,
			expectedModelName: "ai/test-model:latest",
		},
		// TODO: these tests require to override the default OCI registry,
		//{
		//	name:              "no org with tag (should default to ai/)",
		//	pullRef:           "test-model:latest",
		//	expectedModelID:   modelID2,
		//	expectedModelName: "ai/test-model:latest",
		//},
		//{
		//	name:              "no org and no tag (should default to ai/:latest)",
		//	pullRef:           "test-model",
		//	expectedModelID:   modelID2,
		//	expectedModelName: "ai/test-model:latest",
		//},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Pull the model using the test case reference
			t.Logf("Pulling model with reference: %s", tc.pullRef)
			err := pullModel(newPullCmd(), env.client, tc.pullRef, false)
			require.NoError(t, err, "Failed to pull model with reference: %s", tc.pullRef)

			// List models and verify the expected model is present
			models, err := listModels(false, env.client, true, false, "")
			require.NoError(t, err)

			if len(models) == 0 {
				t.Fatalf("No models found after pulling %s", tc.pullRef)
			}

			// Extract truncated ID format (sha256:xxx... -> xxx where xxx is 12 chars)
			// listModels with quiet=true returns modelID[7:19]
			truncatedID := tc.expectedModelID[7:19]
			if !strings.Contains(models, truncatedID) {
				t.Errorf("Expected model ID %s (truncated: %s) not found in model list after pulling %s.\nExpected model: %s\nModel list:\n%s",
					tc.expectedModelID, truncatedID, tc.pullRef, tc.expectedModelName, models)
			} else {
				t.Logf("✓ Successfully verified model %s (ID: %s) after pulling with reference: %s",
					tc.expectedModelName, truncatedID, tc.pullRef)
			}

			// Clean up: remove the model for the next test iteration
			// Note: We use the full model ID for removal to ensure we remove the correct model
			t.Logf("Removing model %s", truncatedID)
			err = removeModel(env.client, tc.expectedModelID)
			if err != nil {
				t.Logf("Warning: Failed to remove model %s: %v (continuing anyway)", truncatedID, err)
			}
		})
	}
}
