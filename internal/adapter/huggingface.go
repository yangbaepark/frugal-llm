package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"frugal-llm/internal/model"
)

type HuggingFaceAdapter struct {
	BaseURL string
	APIKey  string
}

func NewHuggingFaceAdapter(baseURL, apiKey string) *HuggingFaceAdapter {
	return &HuggingFaceAdapter{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:  strings.TrimSpace(apiKey),
	}
}

func (a *HuggingFaceAdapter) Name() string {
	return "HuggingFace"
}

type hfRequest struct {
	Inputs     string                 `json:"inputs"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

type hfResponseElement struct {
	GeneratedText string `json:"generated_text"`
}

func (a *HuggingFaceAdapter) BuildRequest(req *model.ChatCompletionRequest) ([]byte, string, map[string]string, error) {
	var promptBuilder strings.Builder
	for _, m := range req.Messages {
		promptBuilder.WriteString(fmt.Sprintf("<|%s|>\n%s\n", m.Role, m.Content))
	}
	promptBuilder.WriteString("<|assistant|>\n")

	hfReq := hfRequest{
		Inputs: promptBuilder.String(),
		Parameters: map[string]interface{}{
			"return_full_text": false,
		},
	}

	if req.MaxTokens > 0 {
		hfReq.Parameters["max_new_tokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		hfReq.Parameters["temperature"] = *req.Temperature
	}

	body, err := json.Marshal(hfReq)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to marshal huggingface request: %w", err)
	}

	targetURL := fmt.Sprintf("%s/%s", strings.TrimRight(a.BaseURL, "/"), req.Model)
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if a.APIKey != "" {
		headers["Authorization"] = fmt.Sprintf("Bearer %s", a.APIKey)
	}

	return body, targetURL, headers, nil
}

func (a *HuggingFaceAdapter) TransformResponse(resp *http.Response) (*model.ChatCompletionResponse, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("huggingface provider error (status %d): %s", resp.StatusCode, string(body))
	}

	var hfResps []hfResponseElement
	if err := json.Unmarshal(body, &hfResps); err != nil || len(hfResps) == 0 {
		return nil, fmt.Errorf("failed to parse huggingface response: %s", string(body))
	}

	return &model.ChatCompletionResponse{
		ID:      fmt.Sprintf("hf-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Choices: []model.ChatChoice{
			{
				Index: 0,
				Message: model.ChatMessage{
					Role:    "assistant",
					Content: hfResps[0].GeneratedText,
				},
				FinishReason: "stop",
			},
		},
	}, nil
}

func (a *HuggingFaceAdapter) HandleStream(ctx context.Context, resp *http.Response, w http.ResponseWriter) error {
	// Fallback to non-streaming response for basic HF API
	openAIResp, err := a.TransformResponse(resp)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "text/event-stream")
	chunk := model.ChatCompletionChunk{
		ID:      openAIResp.ID,
		Object:  "chat.completion.chunk",
		Created: openAIResp.Created,
		Choices: []model.StreamChoice{
			{
				Index: 0,
				Delta: model.DeltaMessage{
					Role:    "assistant",
					Content: openAIResp.Choices[0].Message.Content,
				},
			},
		},
	}
	chunkBytes, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
	fmt.Fprintf(w, "data: [DONE]\n\n")
	return nil
}
