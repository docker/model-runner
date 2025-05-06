package registry

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/docker/model-cards/tools/build-tables/internal/domain"
	"github.com/docker/model-cards/tools/build-tables/internal/gguf"
	"github.com/docker/model-cards/tools/build-tables/internal/logger"
)

// Client implements the domain.RegistryClient interface
type Client struct{}

// NewClient creates a new registry client
func NewClient() Client {
	return Client{}
}

// ListTags lists all tags for a repository
func (c *Client) ListTags(repoName string) ([]string, error) {
	repo, err := name.NewRepository(repoName)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository reference: %v", err)
	}
	logger.Infof("Listing tags for repository: %s", repo.String())
	tags, err := remote.List(repo, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %v", err)
	}
	logger.Infof("Found %d tags: %v", len(tags), tags)
	return tags, nil
}

// ProcessTags processes all tags for a repository and returns model variants
func (c *Client) ProcessTags(repoName string, tags []string) ([]domain.ModelVariant, error) {
	// Map to store variants by their descriptor hash
	variantMap := make(map[string]*domain.ModelVariant)

	// Process each tag
	for _, tag := range tags {
		// Get model info for this tag
		variant, err := c.GetModelVariant(context.Background(), repoName, tag)
		if err != nil {
			logger.WithFields(logger.Fields{
				"repository": repoName,
				"tag":        tag,
				"error":      err,
			}).Warn("Failed to get info for tag")
			continue
		}

		// Create a unique key based on the model's properties
		key := fmt.Sprintf("%s-%s", variant.Parameters, variant.Quantization)

		// Check if we already have a variant with these properties
		if existingVariant, exists := variantMap[key]; exists {
			// Add the tag to the existing variant
			existingVariant.Tags = append(existingVariant.Tags, tag)
		} else {
			// Create a new variant with the tag
			variant.Tags = []string{tag}
			variantMap[key] = &variant
		}
	}

	// Convert map to slice
	var variants []domain.ModelVariant
	for _, variant := range variantMap {
		variants = append(variants, *variant)
	}

	return variants, nil
}

// GetModelVariant gets information about a specific model tag
func (c *Client) GetModelVariant(ctx context.Context, repoName, tag string) (domain.ModelVariant, error) {
	logger.Debugf("Getting model info for %s:%s", repoName, tag)

	variant := domain.ModelVariant{
		RepoName: repoName,
		Tags:     []string{tag},
	}

	// Create a reference to the image
	ref, err := name.ParseReference(fmt.Sprintf("%s:%s", repoName, tag))
	if err != nil {
		return variant, fmt.Errorf("failed to parse reference: %v", err)
	}

	// Get the image descriptor
	desc, err := remote.Get(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return variant, fmt.Errorf("failed to get image descriptor: %v", err)
	}

	// Get the image
	img, err := desc.Image()
	if err != nil {
		return variant, fmt.Errorf("failed to get image: %v", err)
	}

	// Get the manifest
	manifest, err := img.Manifest()
	if err != nil {
		return variant, fmt.Errorf("failed to get manifest: %v", err)
	}

	// Find GGUF layer and parse it
	var ggufURL string
	for _, layer := range manifest.Layers {
		if layer.MediaType == "application/vnd.docker.ai.gguf.v3" {
			// Construct the URL for the GGUF file using the proper registry blob URL format
			ggufURL = fmt.Sprintf("https://%s/v2/%s/blobs/%s", ref.Context().RegistryStr(), ref.Context().RepositoryStr(), layer.Digest.String())
			break
		}
	}

	if ggufURL == "" {
		return variant, fmt.Errorf("no GGUF layer found")
	}

	tr, err := transport.NewWithContext(
		ctx,
		ref.Context().Registry,
		authn.Anonymous, // You can use authn.DefaultKeychain if you want support for config-based login
		http.DefaultTransport,
		[]string{ref.Scope(transport.PullScope)},
	)
	if err != nil {
		return variant, fmt.Errorf("failed to create transport: %w", err)
	}

	// Extract token from Authorization header
	req, _ := http.NewRequest("GET", ggufURL, nil)
	resp, err := tr.RoundTrip(req)
	if err != nil {
		return variant, fmt.Errorf("failed to get auth token: %w", err)
	}
	token := resp.Request.Header.Get("Authorization")
	if token == "" {
		return variant, fmt.Errorf("no Authorization token found")
	}
	token = token[len("Bearer "):] // Strip "Bearer "

	// Parse the GGUF file
	parser := gguf.NewParser()
	parsedGGUF, err := parser.ParseRemote(ctx, ggufURL, token)
	if err != nil {
		return variant, fmt.Errorf("failed to parse GGUF: %w", err)
	}

	variant.Descriptor = parsedGGUF

	// Fill in the variant information
	_, formattedParams, err := parsedGGUF.GetParameters()
	if err != nil {
		logger.WithFields(logger.Fields{
			"repository": repoName,
			"tag":        tag,
			"error":      err,
		}).Warn("Failed to get parameters")
	}
	variant.Parameters = formattedParams

	quantization := parsedGGUF.GetQuantization()
	if err != nil {
		logger.WithFields(logger.Fields{
			"repository": repoName,
			"tag":        tag,
			"error":      err,
		}).Warn("Failed to get quantization")
	}
	variant.Quantization = quantization.String()

	size, err := parsedGGUF.GetSize()
	if err != nil {
		logger.WithFields(logger.Fields{
			"repository": repoName,
			"tag":        tag,
			"error":      err,
		}).Warn("Failed to get size")
	}
	variant.Size = size

	contextLength, err := parsedGGUF.GetContextLength()
	if err != nil {
		logger.WithFields(logger.Fields{
			"repository": repoName,
			"tag":        tag,
			"error":      err,
		}).Warn("Failed to get context length")
		variant.ContextLength = 0
	} else {
		variant.ContextLength = contextLength
	}

	vram, err := parsedGGUF.GetVRAM()
	if err != nil {
		logger.WithFields(logger.Fields{
			"repository": repoName,
			"tag":        tag,
			"error":      err,
		}).Warn("Failed to get VRAM")
		variant.VRAM = 0
	} else {
		variant.VRAM = vram
	}

	return variant, nil
}
