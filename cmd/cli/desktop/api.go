package desktop

// ProgressMessage represents a structured message for progress reporting
type ProgressMessage struct {
	Type    string `json:"type"`    // "progress", "success", or "error"
	Message string `json:"message"` // Deprecated: the message should be defined by clients based on Message.Total and Message.Layer
	Total   uint64 `json:"total"`
	Pulled  uint64 `json:"pulled"` // Deprecated: use Layer.Current
	Layer   Layer  `json:"layer"`  // Current layer information
	Mode    string `json:"mode"`   // "push", "pull"
}

type Layer struct {
	ID      string // Layer ID
	Size    uint64 // Layer size
	Current uint64 // Current bytes transferred
}

type OpenAIChatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // Can be string or []ContentPart for multimodal
}

// ContentPart represents a part of multimodal content (text or image)
type ContentPart struct {
	Type     string    `json:"type"` // "text" or "image_url"
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL represents an image in a message
type ImageURL struct {
	URL string `json:"url"` // data:image/jpeg;base64,...
}

type OpenAIChatRequest struct {
	Model    string              `json:"model"`
	Messages []OpenAIChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

type OpenAIChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			Role             string `json:"role,omitempty"`
			ReasoningContent string `json:"reasoning_content,omitempty"`
		} `json:"delta"`
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		CompletionTokens int `json:"completion_tokens"`
		PromptTokens     int `json:"prompt_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}
