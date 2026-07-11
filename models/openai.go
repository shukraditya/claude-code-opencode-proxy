package models

import "encoding/json"

type OpenAIMessage struct {
	Role       string               `json:"role"`
	Content    string               `json:"content,omitempty"`
	ToolCalls  []OpenAIToolCall     `json:"tool_calls,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
	Name       string               `json:"name,omitempty"`
	MultiContent []OpenAIContentPart `json:"-"`
}

type OpenAIContentPart struct {
	Type     string             `json:"type"`
	Text     string             `json:"text,omitempty"`
	ImageURL *OpenAIImageURL    `json:"image_url,omitempty"`
}

type OpenAIImageURL struct {
	URL string `json:"url"`
}

type OpenAIToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function OpenAIFunction   `json:"function"`
}

type OpenAIFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunctionDef `json:"function"`
}

type OpenAIFunctionDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type OpenAIToolChoice struct {
	Type     string               `json:"type"`
	Function *OpenAIToolChoiceFunction `json:"function,omitempty"`
}

type OpenAIToolChoiceFunction struct {
	Name string `json:"name"`
}

type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Stream      bool            `json:"stream,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	N           *int            `json:"n,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
	ToolChoice  any             `json:"tool_choice,omitempty"`
	StreamOptions *OpenAIStreamOptions `json:"stream_options,omitempty"`
}

type OpenAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type OpenAIResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []OpenAIChoice   `json:"choices"`
	Usage   *OpenAIUsage     `json:"usage,omitempty"`
}

type OpenAIChoice struct {
	Index        int               `json:"index"`
	Message      OpenAIMessage     `json:"message"`
	FinishReason *string           `json:"finish_reason,omitempty"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	PromptTokensDetails *OpenAIUsageDetails `json:"prompt_tokens_details,omitempty"`
}

type OpenAIUsageDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

type OpenAIStreamChunk struct {
	ID      string                `json:"id"`
	Object  string                `json:"object"`
	Created int64                 `json:"created"`
	Model   string                `json:"model"`
	Choices []OpenAIStreamChoice  `json:"choices"`
	Usage   *OpenAIUsage          `json:"usage,omitempty"`
}

type OpenAIStreamChoice struct {
	Index        int                  `json:"index"`
	Delta        OpenAIStreamDelta    `json:"delta"`
	FinishReason *string              `json:"finish_reason,omitempty"`
}

type OpenAIStreamDelta struct {
	Role             string                `json:"role,omitempty"`
	Content          string                `json:"content,omitempty"`
	ReasoningContent string                `json:"reasoning_content,omitempty"`
	ToolCalls        []OpenAIStreamToolCall `json:"tool_calls,omitempty"`
}

type OpenAIStreamToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function *OpenAIFunction    `json:"function,omitempty"`
}

type OpenAISSEEvent struct {
	Type    string
	Data    json.RawMessage
	IsFinal bool
}

type OpenAIMultiContentRequest struct {
	Model       string               `json:"model"`
	Messages    []OpenAIMultiMessage `json:"messages"`
	Stream      bool                 `json:"stream,omitempty"`
	Temperature *float64             `json:"temperature,omitempty"`
	TopP        *float64             `json:"top_p,omitempty"`
	Stop        []string             `json:"stop,omitempty"`
	MaxTokens   int                  `json:"max_tokens,omitempty"`
	Tools       []OpenAITool         `json:"tools,omitempty"`
	ToolChoice  any                  `json:"tool_choice,omitempty"`
}

type OpenAIMultiMessage struct {
	Role       string             `json:"role"`
	Content    []OpenAIContentPart `json:"content"`
	ToolCalls  []OpenAIToolCall   `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
}
