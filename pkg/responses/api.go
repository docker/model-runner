// Package responses implements the OpenAI Responses API compatibility layer.
// The Responses API is a stateful API that combines chat completions with
// conversation state management and tool use capabilities.
package responses

import (
	"crypto/rand"
	"encoding/json"
	"time"
)

// APIPrefix is the URL prefix for the Responses API.
const APIPrefix = "/responses"

// Response status values
const (
	StatusQueued     = "queued"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusCancelled  = "cancelled"
	StatusFailed     = "failed"
)

// Content types
const (
	ContentTypeInputText          = "input_text"
	ContentTypeOutputText         = "output_text"
	ContentTypeInputImage         = "input_image"
	ContentTypeInputFile          = "input_file"
	ContentTypeRefusal            = "refusal"
	ContentTypeFunctionCall       = "function_call"
	ContentTypeFunctionCallOutput = "function_call_output"
)

// Item types
const (
	ItemTypeMessage            = "message"
	ItemTypeFunctionCall       = "function_call"
	ItemTypeFunctionCallOutput = "function_call_output"
)

// Streaming event types
const (
	EventResponseCreated       = "response.created"
	EventResponseInProgress    = "response.in_progress"
	EventResponseCompleted     = "response.completed"
	EventResponseFailed        = "response.failed"
	EventResponseIncomplete    = "response.incomplete"
	EventOutputItemAdded       = "response.output_item.added"
	EventOutputItemDone        = "response.output_item.done"
	EventContentPartAdded      = "response.content_part.added"
	EventContentPartDone       = "response.content_part.done"
	EventOutputTextDelta       = "response.output_text.delta"
	EventOutputTextDone        = "response.output_text.done"
	EventRefusalDelta          = "response.refusal.delta"
	EventRefusalDone           = "response.refusal.done"
	EventFunctionCallArgsDelta = "response.function_call_arguments.delta"
	EventFunctionCallArgsDone  = "response.function_call_arguments.done"
	EventError                 = "error"
)

// CreateRequest represents a request to create a response.
type CreateRequest struct {
	// Model is the model to use for generating the response.
	Model string `json:"model"`

	// Input is the input to the model. Can be a string or array of input items.
	Input json.RawMessage `json:"input"`

	// Instructions is an optional system prompt/instructions for the model.
	Instructions string `json:"instructions,omitempty"`

	// PreviousResponseID links this request to a previous response for conversation chaining.
	PreviousResponseID string `json:"previous_response_id,omitempty"`

	// Tools is the list of tools available to the model.
	Tools []Tool `json:"tools,omitempty"`

	// ToolChoice controls how the model uses tools.
	ToolChoice interface{} `json:"tool_choice,omitempty"`

	// ParallelToolCalls enables parallel tool calls.
	ParallelToolCalls *bool `json:"parallel_tool_calls,omitempty"`

	// Temperature controls randomness (0-2).
	Temperature *float64 `json:"temperature,omitempty"`

	// TopP controls nucleus sampling.
	TopP *float64 `json:"top_p,omitempty"`

	// MaxOutputTokens limits the response length.
	MaxOutputTokens *int `json:"max_output_tokens,omitempty"`

	// Store controls whether the response is stored for later retrieval.
	Store *bool `json:"store,omitempty"`

	// Text configures text output formatting.
	Text *ResponseTextConfig `json:"text,omitempty"`

	// Include requests additional hosted-tool output fields. Unsupported locally.
	Include []string `json:"include,omitempty"`

	// Stream enables streaming responses.
	Stream bool `json:"stream,omitempty"`

	// StreamOptions configures response streaming. Unsupported locally.
	StreamOptions json.RawMessage `json:"stream_options,omitempty"`

	// TopLogprobs requests token log probabilities. Unsupported locally.
	TopLogprobs *int `json:"top_logprobs,omitempty"`

	// Truncation configures OpenAI context truncation behavior.
	Truncation string `json:"truncation,omitempty"`

	// Background requests asynchronous background response execution.
	Background *bool `json:"background,omitempty"`

	// Conversation requests OpenAI hosted conversation state. Unsupported locally.
	Conversation json.RawMessage `json:"conversation,omitempty"`

	// Prompt requests an OpenAI hosted prompt template. Unsupported locally.
	Prompt json.RawMessage `json:"prompt,omitempty"`

	// ServiceTier selects OpenAI service tier. Unsupported locally.
	ServiceTier string `json:"service_tier,omitempty"`

	// SafetyIdentifier is an OpenAI safety identifier. Unsupported locally.
	SafetyIdentifier string `json:"safety_identifier,omitempty"`

	// Metadata is user-defined metadata for the response.
	Metadata map[string]string `json:"metadata,omitempty"`

	// ReasoningEffort controls reasoning model effort (low, medium, high).
	ReasoningEffort string `json:"reasoning_effort,omitempty"`

	// User is an optional user identifier.
	User string `json:"user,omitempty"`
}

// ResponseTextConfig configures text output for a Responses API request.
type ResponseTextConfig struct {
	Format    *ResponseTextFormat `json:"format,omitempty"`
	Verbosity string              `json:"verbosity,omitempty"`
}

// ResponseTextFormat configures the output text format.
type ResponseTextFormat struct {
	Type        string          `json:"type"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

// Response represents a complete response from the API.
type Response struct {
	// ID is the unique identifier for this response.
	ID string `json:"id"`

	// Object is always "response".
	Object string `json:"object"`

	// CreatedAt is the Unix timestamp when the response was created.
	CreatedAt float64 `json:"created_at"`

	// Model is the model used to generate the response.
	Model string `json:"model"`

	// Status is the current status of the response.
	Status string `json:"status"`

	// Output is the array of output items (messages, function calls, etc.).
	Output []OutputItem `json:"output"`

	// OutputText is a convenience field containing concatenated text output.
	OutputText string `json:"output_text,omitempty"`

	// Error contains error details if status is "failed".
	Error *ErrorDetail `json:"error"`

	// IncompleteDetails contains details if the response was incomplete.
	IncompleteDetails *IncompleteDetails `json:"incomplete_details"`

	// FinishReason contains the reason the model stopped generating (e.g., stop, length, function_call, etc.).
	FinishReason string `json:"finish_reason,omitempty"`

	// Instructions is the system instructions used.
	Instructions *string `json:"instructions"`

	// Metadata is user-defined metadata.
	Metadata map[string]string `json:"metadata"`

	// ParallelToolCalls indicates if parallel tool calls were enabled.
	ParallelToolCalls *bool `json:"parallel_tool_calls"`

	// Temperature used for generation.
	Temperature *float64 `json:"temperature"`

	// ToolChoice used for generation.
	ToolChoice interface{} `json:"tool_choice"`

	// Tools available during generation.
	Tools []Tool `json:"tools"`

	// TopP used for generation.
	TopP *float64 `json:"top_p"`

	// MaxOutputTokens limit used.
	MaxOutputTokens *int `json:"max_output_tokens"`

	// Store indicates whether the response was stored.
	Store *bool `json:"store,omitempty"`

	// Text config used for the response.
	Text *ResponseTextConfig `json:"text,omitempty"`

	// PreviousResponseID is the ID of the previous response in the chain.
	PreviousResponseID *string `json:"previous_response_id"`

	// Reasoning contains reasoning details for reasoning models.
	Reasoning *ReasoningDetails `json:"reasoning"`

	// Usage contains token usage statistics.
	Usage *Usage `json:"usage"`

	// User identifier if provided.
	User *string `json:"user"`

	// ReasoningEffort used for reasoning models.
	ReasoningEffort *string `json:"reasoning_effort"`
}

// OutputItem represents an item in the response output.
type OutputItem struct {
	// ID is the unique identifier for this output item.
	ID string `json:"id"`

	// Type is the type of output item (message, function_call, etc.).
	Type string `json:"type"`

	// Role is the role for message items (assistant).
	Role string `json:"role,omitempty"`

	// Content is the content array for message items.
	Content []ContentPart `json:"content,omitempty"`

	// Status is the status of this output item.
	Status string `json:"status,omitempty"`

	// CallID is the ID for function call items.
	CallID string `json:"call_id,omitempty"`

	// Name is the function name for function call items.
	Name string `json:"name,omitempty"`

	// Arguments is the function arguments for function call items.
	Arguments string `json:"arguments,omitempty"`

	// Output is the function output for function_call_output items.
	Output string `json:"output,omitempty"`
}

// ContentPart represents a part of content within an output item.
type ContentPart struct {
	// Type is the content type (output_text, refusal, etc.).
	Type string `json:"type"`

	// Text is the text content for output_text type.
	Text string `json:"text,omitempty"`

	// Refusal is the refusal message for refusal type.
	Refusal string `json:"refusal,omitempty"`

	// Annotations contains any annotations on the content.
	Annotations []Annotation `json:"annotations,omitempty"`
}

// Annotation represents an annotation on content.
type Annotation struct {
	Type       string `json:"type"`
	StartIndex int    `json:"start_index,omitempty"`
	EndIndex   int    `json:"end_index,omitempty"`
	URL        string `json:"url,omitempty"`
	Title      string `json:"title,omitempty"`
}

// InputItem represents an input item in the request.
type InputItem struct {
	// Type is the type of input item.
	Type string `json:"type,omitempty"`

	// Role is the role for message-style inputs.
	Role string `json:"role,omitempty"`

	// Content can be a string or array of content parts.
	Content json.RawMessage `json:"content,omitempty"`

	// CallID is for function_call_output items.
	CallID string `json:"call_id,omitempty"`

	// Output is the function output for function_call_output items.
	Output string `json:"output,omitempty"`

	// ID is for referencing items.
	ID string `json:"id,omitempty"`

	// Name is for function calls.
	Name string `json:"name,omitempty"`

	// Arguments is for function calls.
	Arguments string `json:"arguments,omitempty"`
}

// InputContentPart represents a content part in the input.
type InputContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	FileID   string `json:"file_id,omitempty"`
	FileData string `json:"file_data,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// Tool represents a tool available to the model.
type Tool struct {
	// Type is the tool type (function, etc.).
	Type string `json:"type"`

	// Name is the function name (for function tools).
	Name string `json:"name,omitempty"`

	// Description is the function description.
	Description string `json:"description,omitempty"`

	// Parameters is the JSON schema for function parameters.
	Parameters interface{} `json:"parameters,omitempty"`

	// Function contains function details (alternative structure).
	Function *FunctionDef `json:"function,omitempty"`
}

// FunctionDef defines a function tool.
type FunctionDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

// Usage contains token usage statistics.
type Usage struct {
	InputTokens         int                  `json:"input_tokens"`
	OutputTokens        int                  `json:"output_tokens"`
	TotalTokens         int                  `json:"total_tokens"`
	OutputTokensDetails *OutputTokensDetails `json:"output_tokens_details,omitempty"`
}

// OutputTokensDetails contains detailed output token breakdown.
type OutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// ErrorDetail contains error information.
type ErrorDetail struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// IncompleteDetails contains details about why a response was incomplete.
type IncompleteDetails struct {
	Reason string `json:"reason,omitempty"`
}

// ReasoningDetails contains reasoning information for reasoning models.
type ReasoningDetails struct {
	EncryptedContent string `json:"encrypted_content,omitempty"`
}

// StreamEvent represents a streaming event.
type StreamEvent struct {
	// Type is the event type.
	Type string `json:"type"`

	// SequenceNumber is the sequence number for ordering.
	SequenceNumber int `json:"sequence_number"`

	// Response is included in response lifecycle events.
	Response *Response `json:"response,omitempty"`

	// Item is included in output item events.
	Item *OutputItem `json:"item,omitempty"`

	// OutputIndex is the index in the output array.
	OutputIndex int `json:"output_index,omitempty"`

	// ContentIndex is the index within content array.
	ContentIndex int `json:"content_index,omitempty"`

	// Part is the content part for content events.
	Part *ContentPart `json:"part,omitempty"`

	// Delta is the text delta for delta events.
	Delta string `json:"delta,omitempty"`

	// ItemID is the ID of the item being modified.
	ItemID string `json:"item_id,omitempty"`

	// Error is included in error events.
	Error *ErrorDetail `json:"error,omitempty"`
}

// NewResponse creates a new Response with default values.
func NewResponse(id, model string) *Response {
	return &Response{
		ID:        id,
		Object:    "response",
		CreatedAt: float64(time.Now().Unix()),
		Model:     model,
		Status:    StatusInProgress,
		Output:    []OutputItem{},
		Tools:     []Tool{},
		Metadata:  map[string]string{},
	}
}

// GenerateResponseID generates a unique response ID.
func GenerateResponseID() string {
	return "resp_" + GenerateID(24)
}

// GenerateItemID generates a unique item ID.
func GenerateItemID() string {
	return "item_" + GenerateID(24)
}

// GenerateMessageID generates a unique message ID.
func GenerateMessageID() string {
	return "msg_" + GenerateID(24)
}

// GenerateCallID generates a unique call ID for function calls.
func GenerateCallID() string {
	return "call_" + GenerateID(24)
}

// GenerateID generates a random alphanumeric ID of the specified length.
func GenerateID(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		// A failure of crypto/rand indicates a critical problem with the OS's
		// entropy source, so we should panic.
		panic("failed to read random bytes for ID generation: " + err.Error())
	}
	for i := range b {
		b[i] = charset[b[i]%byte(len(charset))]
	}
	return string(b)
}
