package adapter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"frugal-llm/internal/model"
)

type OpenAIAdapter struct {
	BaseURL string
	APIKey  string
}

func NewOpenAIAdapter(name, baseURL, apiKey string) (*OpenAIAdapter, bool) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	if baseURL == "" {
		return nil, false
	}
	isLocal := strings.ToLower(name) == "local" || strings.Contains(baseURL, "localhost") || strings.Contains(baseURL, "127.0.0.1")
	if !isLocal && apiKey == "" {
		return nil, false
	}
	return &OpenAIAdapter{
		BaseURL: baseURL,
		APIKey:  apiKey,
	}, true
}

func (a *OpenAIAdapter) Name() string {
	return "OpenAI"
}

func (a *OpenAIAdapter) BuildRequest(req *model.ChatCompletionRequest) ([]byte, string, map[string]string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	targetURL := fmt.Sprintf("%s/chat/completions", a.BaseURL)
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	if a.APIKey != "" {
		headers["Authorization"] = fmt.Sprintf("Bearer %s", a.APIKey)
	}

	return body, targetURL, headers, nil
}

func (a *OpenAIAdapter) TransformResponse(resp *http.Response) (*model.ChatCompletionResponse, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream provider returned status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp model.ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &chatResp, nil
}

func (a *OpenAIAdapter) HandleStream(ctx context.Context, resp *http.Response, w http.ResponseWriter) error {
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
			fmt.Fprintf(w, "%s\n", line)
			flusher.Flush()
		}
	}

	return scanner.Err()
}
