package distribution

import "strings"

const (
	defaultOrg = "ai"
	defaultTag = "latest"
)

// NormalizeModelName adds the default organization prefix (ai/) and tag (:latest) if missing.
// It also converts Hugging Face model names to lowercase.
// Examples:
//   - "gemma3" -> "ai/gemma3:latest"
//   - "gemma3:v1" -> "ai/gemma3:v1"
//   - "myorg/gemma3" -> "myorg/gemma3:latest"
//   - "ai/gemma3:latest" -> "ai/gemma3:latest" (unchanged)
//   - "hf.co/model" -> "huggingface.co/model:latest" (changed registry and lowercased)
//   - "hf.co/Model" -> "huggingface.co/model:latest" (converted to lowercase)
func NormalizeModelName(model string) string {
	// If the model is empty, return as-is
	if model == "" {
		return model
	}

	// Normalize HuggingFace model names (lowercase)
	if strings.HasPrefix(model, "hf.co/") {
		// Replace hf.co with huggingface.co to avoid losing the Authorization header on redirect.
		model = "huggingface.co" + strings.ToLower(strings.TrimPrefix(model, "hf.co"))
	}

	if strings.HasPrefix(model, "sha256:") {
		return model
	}

	// Check if model contains a registry (domain with dot before first slash)
	firstSlash := strings.Index(model, "/")
	if firstSlash > 0 && strings.Contains(model[:firstSlash], ".") {
		// Has a registry, just ensure tag
		if !strings.Contains(model, ":") {
			return model + ":" + defaultTag
		}
		return model
	}

	// Split by colon to check for tag
	parts := strings.SplitN(model, ":", 2)
	nameWithOrg := parts[0]
	tag := defaultTag
	if len(parts) == 2 {
		tag = parts[1]
	}

	// If name doesn't contain a slash, add the default org
	if !strings.Contains(nameWithOrg, "/") {
		nameWithOrg = defaultOrg + "/" + nameWithOrg
	}

	return nameWithOrg + ":" + tag
}
