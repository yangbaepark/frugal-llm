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

type OllamaAdapter struct {
	BaseURL string
}

func NewOllamaAdapter(baseURL string) (*OllamaAdapter, bool) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, false
	}
	return &OllamaAdapter{
		BaseURL: baseURL,
	}, true
}

func (a *OllamaAdapter) Name() string {
	return "Ollama"
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaChatResponse struct {
	Model     string        `json:"model"`
	CreatedAt string        `json:"created_at"`
	Message   ollamaMessage `json:"message"`
	Done      bool          `json:"done"`
}

func (a *OllamaAdapter) BuildRequest(req *model.ChatCompletionRequest) ([]byte, string, map[string]string, error) {
	var msgs []ollamaMessage
	for _, m := range req.Messages {
		msgs = append(msgs, ollamaMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	ollamaReq := ollamaChatRequest{
		Model:    req.Model,
		Messages: msgs,
		Stream:   req.Stream,
	}

	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to marshal ollama request: %w", err)
	}

	targetURL := fmt.Sprintf("%s/api/chat", a.BaseURL)
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	return body, targetURL, headers, nil
}

func (a *OllamaAdapter) TransformResponse(resp *http.Response) (*model.ChatCompletionResponse, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama provider error (status %d): %s", resp.StatusCode, string(body))
	}

	var ollamaResp ollamaChatResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ollama response: %w", err)
	}

	return &model.ChatCompletionResponse{
		ID:      fmt.Sprintf("ollama-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   ollamaResp.Model,
		Choices: []model.ChatChoice{
			{
				Index: 0,
				Message: model.ChatMessage{
					Role:    ollamaResp.Message.Role,
					Content: ollamaResp.Message.Content,
				},
				FinishReason: "stop",
			},
		},
	}, nil
}

func (a *OllamaAdapter) HandleStream(ctx context.Context, resp *http.Response, w http.ResponseWriter) error {
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

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
			line := scanner.Bytes()
			var oResp ollamaChatResponse
			if err := json.Unmarshal(line, &oResp); err == nil {
				chunk := model.ChatCompletionChunk{
					ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
					Object:  "chat.completion.chunk",
					Created: time.Now().Unix(),
					Model:   oResp.Model,
					Choices: []model.StreamChoice{
						{
							Index: 0,
							Delta: model.DeltaMessage{
								Role:    oResp.Message.Role,
								Content: oResp.Message.Content,
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

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
	return scanner.Err()
}
