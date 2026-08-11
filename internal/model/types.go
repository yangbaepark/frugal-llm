package model

import (
	"encoding/json"
	"strings"
)

// ContentPart represents a single part of structured message content (multimodal text/images).
type ContentPart struct {
	Type     string      `json:"type"`
	Text     string      `json:"text,omitempty"`
	ImageURL interface{} `json:"image_url,omitempty"`
}

// ChatMessage represents a single message in a chat conversation.
type ChatMessage struct {
	Role      string      `json:"role"`
	Content   string      `json:"content"`
	Name      string      `json:"name,omitempty"`
	ToolCalls interface{} `json:"tool_calls,omitempty"`
}

// UnmarshalJSON implements flexible unmarshaling for ChatMessage to support both string and array-based content payloads.
func (m *ChatMessage) UnmarshalJSON(data []byte) error {
	var raw struct {
		Role      string          `json:"role"`
		Content   json.RawMessage `json:"content"`
		Name      string          `json:"name,omitempty"`
		ToolCalls interface{}     `json:"tool_calls,omitempty"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	m.Role = raw.Role
	m.Name = raw.Name
	m.ToolCalls = raw.ToolCalls

	if len(raw.Content) == 0 || string(raw.Content) == "null" {
		m.Content = ""
		return nil
	}

	// 1. Try string content
	var str string
	if err := json.Unmarshal(raw.Content, &str); err == nil {
		m.Content = str
		return nil
	}

	// 2. Try array of ContentPart (structured text/multimodal)
	var parts []ContentPart
	if err := json.Unmarshal(raw.Content, &parts); err == nil {
		var sb strings.Builder
		for _, p := range parts {
			if p.Text != "" {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(p.Text)
			}
		}
		m.Content = sb.String()
		return nil
	}

	// 3. Fallback to raw string
	m.Content = string(raw.Content)
	return nil
}

// ChatCompletionRequest is the OpenAI-compatible chat completion payload.
type ChatCompletionRequest struct {
	Model          string        `json:"model"`
	Messages       []ChatMessage `json:"messages"`
	Temperature    *float64      `json:"temperature,omitempty"`
	TopP           *float64      `json:"top_p,omitempty"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	Stream         bool          `json:"stream,omitempty"`
	Stop           interface{}   `json:"stop,omitempty"`
	Tools          interface{}   `json:"tools,omitempty"`
	ToolChoice     interface{}   `json:"tool_choice,omitempty"`
	ResponseFormat interface{}   `json:"response_format,omitempty"`
	User           string        `json:"user,omitempty"`
}

// Usage represents token usage information.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatChoice represents a choice in non-streaming response.
type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// ChatCompletionResponse is the standard OpenAI non-streaming response.
type ChatCompletionResponse struct {
	ID        string       `json:"id"`
	Object    string       `json:"object"`
	Created   int64        `json:"created"`
	Model     string       `json:"model"`
	Choices   []ChatChoice `json:"choices"`
	Usage     Usage        `json:"usage"`
}

// DeltaMessage represents partial content in streaming responses.
type DeltaMessage struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// StreamChoice represents a single choice delta in SSE streams.
type StreamChoice struct {
	Index        int          `json:"index"`
	Delta        DeltaMessage `json:"delta"`
	FinishReason *string      `json:"finish_reason"`
}

// ChatCompletionChunk represents an OpenAI-compatible SSE chunk.
type ChatCompletionChunk struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
}
