package adapter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"frugal-llm/internal/model"
)

type AnthropicAdapter struct {
	BaseURL string
	APIKey  string
}

func NewAnthropicAdapter(baseURL, apiKey string) (*AnthropicAdapter, bool) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	if baseURL == "" {
		return nil, false
	}
	isLocal := strings.Contains(baseURL, "localhost") || strings.Contains(baseURL, "127.0.0.1")
	if !isLocal && apiKey == "" {
		return nil, false
	}
	return &AnthropicAdapter{
		BaseURL: baseURL,
		APIKey:  apiKey,
	}, true
}

func (a *AnthropicAdapter) Name() string {
	return "Anthropic"
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Role       string                  `json:"role"`
	Content    []anthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (a *AnthropicAdapter) BuildRequest(req *model.ChatCompletionRequest) ([]byte, string, map[string]string, error) {
	var systemPrompts []string
	var anthropicMsgs []anthropicMessage

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			if strings.TrimSpace(msg.Content) != "" {
				systemPrompts = append(systemPrompts, msg.Content)
			}
		} else {
			anthropicMsgs = append(anthropicMsgs, anthropicMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	systemPrompt := strings.Join(systemPrompts, "\n\n")

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096 // Default max tokens for Claude
	}

	anthropicReq := anthropicRequest{
		Model:       req.Model,
		Messages:    anthropicMsgs,
		System:      systemPrompt,
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
		Stream:      req.Stream,
	}

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	targetURL := fmt.Sprintf("%s/messages", a.BaseURL)
	headers := map[string]string{
		"Content-Type":      "application/json",
		"x-api-key":         a.APIKey,
		"anthropic-version": "2023-06-01",
	}

	return body, targetURL, headers, nil
}

func (a *AnthropicAdapter) TransformResponse(resp *http.Response) (*model.ChatCompletionResponse, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic provider error (status %d): %s", resp.StatusCode, string(body))
	}

	var antResp anthropicResponse
	if err := json.Unmarshal(body, &antResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal anthropic response: %w", err)
	}

	var contentText string
	for _, block := range antResp.Content {
		if block.Type == "text" {
			contentText += block.Text
		}
	}

	openAIResp := &model.ChatCompletionResponse{
		ID:      antResp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   antResp.Model,
		Choices: []model.ChatChoice{
			{
				Index: 0,
				Message: model.ChatMessage{
					Role:    "assistant",
					Content: contentText,
				},
				FinishReason: antResp.StopReason,
			},
		},
		Usage: model.Usage{
			PromptTokens:     antResp.Usage.InputTokens,
			CompletionTokens: antResp.Usage.OutputTokens,
			TotalTokens:      antResp.Usage.InputTokens + antResp.Usage.OutputTokens,
		},
	}

	return openAIResp, nil
}

type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

func (a *AnthropicAdapter) HandleStream(ctx context.Context, resp *http.Response, w http.ResponseWriter) error {
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("response writer does not support flushing")
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				var event anthropicStreamEvent
				if err := json.Unmarshal([]byte(data), &event); err == nil && event.Type == "content_block_delta" {
					chunk := model.ChatCompletionChunk{
						ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
						Object:  "chat.completion.chunk",
						Created: time.Now().Unix(),
						Choices: []model.StreamChoice{
							{
								Index: 0,
								Delta: model.DeltaMessage{
									Content: event.Delta.Text,
								},
							},
						},
					}
					chunkBytes, _ := json.Marshal(chunk)
					fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
					flusher.Flush()
				}
			}
		}
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
	return scanner.Err()
}
