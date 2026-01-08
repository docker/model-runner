package scheduling

// ImageGenerationRequest represents an OpenAI-compatible image generation request.
// See https://platform.openai.com/docs/api-reference/images/create
type ImageGenerationRequest struct {
	// Model is the model to use for image generation.
	Model string `json:"model"`
	// Prompt is the text description of the desired image(s).
	Prompt string `json:"prompt"`
	// N is the number of images to generate. Defaults to 1.
	N int `json:"n,omitempty"`
	// Size is the size of the generated images. Defaults to "1024x1024".
	Size string `json:"size,omitempty"`
	// Quality is the quality of the image. "standard" or "hd". Defaults to "standard".
	Quality string `json:"quality,omitempty"`
	// ResponseFormat is the format of the generated images. "url" or "b64_json". Defaults to "url".
	ResponseFormat string `json:"response_format,omitempty"`
	// Style is the style of the generated images. "vivid" or "natural". Defaults to "vivid".
	Style string `json:"style,omitempty"`
}

// ImageGenerationResponse represents an OpenAI-compatible image generation response.
type ImageGenerationResponse struct {
	// Created is the Unix timestamp of when the images were created.
	Created int64 `json:"created"`
	// Data is the list of generated images.
	Data []ImageData `json:"data"`
}

// ImageData represents a single generated image.
type ImageData struct {
	// URL is the URL of the generated image. Present when response_format is "url".
	URL string `json:"url,omitempty"`
	// B64JSON is the base64-encoded JSON of the generated image. Present when response_format is "b64_json".
	B64JSON string `json:"b64_json,omitempty"`
	// RevisedPrompt is the prompt that was used if it was revised.
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}
