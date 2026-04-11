package desktop

// Tool represents an OpenAI function tool definition.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction holds the schema for a tool.
type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// ToolCall represents a tool call in a message or streaming delta.
type ToolCall struct {
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Index    int              `json:"index"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction holds the name and accumulated arguments for a tool call.
type ToolCallFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type OpenAIChatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content,omitempty"` // Can be string or []ContentPart for multimodal
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
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
	Tools    []Tool              `json:"tools,omitempty"`
}

type OpenAIChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content          string     `json:"content"`
			Role             string     `json:"role,omitempty"`
			ReasoningContent string     `json:"reasoning_content,omitempty"`
			ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		Message struct {
			Content   string     `json:"content"`
			Role      string     `json:"role,omitempty"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		CompletionTokens int `json:"completion_tokens"`
		PromptTokens     int `json:"prompt_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}
