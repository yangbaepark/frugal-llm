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

// BedrockAdapter adapts Amazon Bedrock Runtime Converse & ConverseStream APIs to OpenAI Chat Completions.
type BedrockAdapter struct {
	BaseURL       string
	Region        string
	CredsProvider AWSCredentialsProvider
}

// NewBedrockAdapter initializes BedrockAdapter if credentials can be resolved.
func NewBedrockAdapter(baseURL string, opts *BedrockOptions) (*BedrockAdapter, bool) {
	provider := NewDynamicAWSCredentialsProvider(opts)
	_, err := provider.GetCredentials()
	if err != nil {
		return nil, false
	}

	region := ResolveBedrockRegion(opts)
	if region == "" {
		return nil, false
	}

	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com", region)
	}

	return &BedrockAdapter{
		BaseURL:       baseURL,
		Region:        region,
		CredsProvider: provider,
	}, true
}

func (a *BedrockAdapter) Name() string {
	return "Bedrock"
}

// Bedrock Converse API data structures
type bedrockContentBlock struct {
	Text string `json:"text"`
}

type bedrockMessage struct {
	Role    string                `json:"role"`
	Content []bedrockContentBlock `json:"content"`
}

type bedrockSystemContent struct {
	Text string `json:"text"`
}

type bedrockInferenceConfig struct {
	MaxTokens   int      `json:"maxTokens,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"topP,omitempty"`
}

type bedrockConverseRequest struct {
	System          []bedrockSystemContent  `json:"system,omitempty"`
	Messages        []bedrockMessage        `json:"messages"`
	InferenceConfig *bedrockInferenceConfig `json:"inferenceConfig,omitempty"`
}

type bedrockConverseResponse struct {
	Output struct {
		Message struct {
			Role    string `json:"role"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	} `json:"output"`
	StopReason string `json:"stopReason"`
	Usage      struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
		TotalTokens  int `json:"totalTokens"`
	} `json:"usage"`
	Message string `json:"message,omitempty"` // Bedrock error messages
}

func (a *BedrockAdapter) BuildRequest(req *model.ChatCompletionRequest) ([]byte, string, map[string]string, error) {
	creds, err := a.CredsProvider.GetCredentials()
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed resolving AWS credentials: %w", err)
	}

	var systems []bedrockSystemContent
	var messages []bedrockMessage

	for _, msg := range req.Messages {
		if strings.EqualFold(msg.Role, "system") {
			if strings.TrimSpace(msg.Content) != "" {
				systems = append(systems, bedrockSystemContent{Text: msg.Content})
			}
		} else {
			role := strings.ToLower(msg.Role)
			if role != "user" && role != "assistant" {
				role = "user"
			}
			messages = append(messages, bedrockMessage{
				Role: role,
				Content: []bedrockContentBlock{
					{Text: msg.Content},
				},
			})
		}
	}

	converseReq := bedrockConverseRequest{
		System:   systems,
		Messages: messages,
	}

	if req.MaxTokens > 0 || req.Temperature != nil || req.TopP != nil {
		converseReq.InferenceConfig = &bedrockInferenceConfig{
			MaxTokens:   req.MaxTokens,
			Temperature: req.Temperature,
			TopP:        req.TopP,
		}
	}

	body, err := json.Marshal(converseReq)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed marshaling Bedrock request: %w", err)
	}

	action := "converse"
	if req.Stream {
		action = "converse-stream"
	}
	targetURL := fmt.Sprintf("%s/model/%s/%s", a.BaseURL, req.Model, action)

	headers, err := SignBedrockRequest("POST", targetURL, body, creds, a.Region, time.Now())
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed signing Bedrock request: %w", err)
	}

	return body, targetURL, headers, nil
}

func (a *BedrockAdapter) TransformResponse(resp *http.Response) (*model.ChatCompletionResponse, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading Bedrock response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream Bedrock provider returned status %d: %s", resp.StatusCode, string(body))
	}

	var bResp bedrockConverseResponse
	if err := json.Unmarshal(body, &bResp); err != nil {
		return nil, fmt.Errorf("failed unmarshaling Bedrock response: %w", err)
	}

	if bResp.Message != "" {
		return nil, fmt.Errorf("bedrock API error: %s", bResp.Message)
	}

	var textBuilder strings.Builder
	for _, block := range bResp.Output.Message.Content {
		textBuilder.WriteString(block.Text)
	}

	finishReason := "stop"
	switch bResp.StopReason {
	case "max_tokens":
		finishReason = "length"
	case "stop_sequence":
		finishReason = "stop"
	case "content_filtered":
		finishReason = "content_filter"
	}

	openAIResp := &model.ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-bedrock-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Choices: []model.ChatChoice{
			{
				Index: 0,
				Message: model.ChatMessage{
					Role:    "assistant",
					Content: textBuilder.String(),
				},
				FinishReason: finishReason,
			},
		},
		Usage: model.Usage{
			PromptTokens:     bResp.Usage.InputTokens,
			CompletionTokens: bResp.Usage.OutputTokens,
			TotalTokens:      bResp.Usage.TotalTokens,
		},
	}

	return openAIResp, nil
}

func (a *BedrockAdapter) HandleStream(ctx context.Context, resp *http.Response, w http.ResponseWriter) error {
	defer resp.Body.Close()
	return StreamBedrockEventStreamToSSE(resp.Body, "bedrock", w)
}
