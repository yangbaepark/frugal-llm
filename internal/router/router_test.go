package router

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"frugal-llm/internal/config"
	"frugal-llm/internal/model"
)

func TestRouterDynamicSelectionWithLLMClassifier(t *testing.T) {
	// Mock upstream server for Classifier, Anthropic, and OpenAI
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		bodyBytes, _ := io.ReadAll(r.Body)
		bodyStr := string(bodyBytes)

		// 1. Classifier LLM request (detect prompt template instruction)
		if strings.Contains(bodyStr, "You are an AI routing classifier") {
			if strings.Contains(bodyStr, "contractual agreement") {
				w.Write([]byte(`{
					"id": "chatcmpl-class",
					"object": "chat.completion",
					"created": 123456,
					"model": "cheap-classifier",
					"choices": [{"index": 0, "message": {"role": "assistant", "content": "openai/gpt-5.6-sol"}, "finish_reason": "stop"}]
				}`))
				return
			}
			w.Write([]byte(`{
				"id": "chatcmpl-class",
				"object": "chat.completion",
				"created": 123456,
				"model": "cheap-classifier",
				"choices": [{"index": 0, "message": {"role": "assistant", "content": "anthropic/claude-sonnet-5"}, "finish_reason": "stop"}]
			}`))
			return
		}

		// 2. Anthropic request
		if r.URL.Path == "/v1/messages" {
			w.Write([]byte(`{
				"id": "msg_123",
				"type": "message",
				"role": "assistant",
				"content": [{"type": "text", "text": "Claude response"}],
				"model": "claude-sonnet-5",
				"stop_reason": "end_turn",
				"usage": {"input_tokens": 10, "output_tokens": 5}
			}`))
			return
		}

		// 3. OpenAI request
		w.Write([]byte(`{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"created": 123456,
			"model": "gpt-5.6-sol",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "Legal advice response"}, "finish_reason": "stop"}]
		}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		Port: "8080",
		Providers: map[string]config.ProviderConfig{
			"anthropic": {
				Name:    "anthropic",
				Type:    "anthropic",
				BaseURL: server.URL + "/v1",
				Models: []config.ModelInfo{
					{Name: "claude-sonnet-5", Description: "Coding model"},
				},
			},
			"openai": {
				Name:    "openai",
				Type:    "openai",
				BaseURL: server.URL + "/v1",
				Models: []config.ModelInfo{
					{Name: "gpt-5.6-sol", Description: "Legal precision model"},
				},
			},
			"classifier-provider": {
				Name:    "classifier-provider",
				Type:    "openai",
				BaseURL: server.URL + "/v1",
				Models: []config.ModelInfo{
					{Name: "cheap-classifier", Description: "Classifier model"},
				},
			},
		},
		DynamicRouting: config.DynamicRoutingConfig{
			TriggerMode:      "always",
			AutoAliases:      []string{"auto"},
			FallbackProvider: "anthropic",
			FallbackModel:    "claude-sonnet-5",
			Classifiers: []config.ClassifierConfig{
				{
					Name: "llm-classifier",
					Type: "llm",
					LLM: config.LLMClassifierConfig{
						Provider:           "classifier-provider",
						Model:              "cheap-classifier",
						TimeoutMS:          1000,
						PromptTemplatePath: "../../templates/classifier_prompt.tmpl",
					},
				},
			},
		},
	}

	r := NewRouter(cfg)

	// 1. Test LLM Classifier match for Legal prompt -> OpenAI
	reqLegal := &model.ChatCompletionRequest{
		Model: "auto",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "What is your advice regarding this contractual agreement?"},
		},
	}
	respLegal, err := r.Execute(context.Background(), reqLegal)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if respLegal.Choices[0].Message.Content != "Legal advice response" {
		t.Errorf("Unexpected response content: %s", respLegal.Choices[0].Message.Content)
	}

	// 2. Test LLM Classifier match for Coding prompt -> Anthropic
	reqCoding := &model.ChatCompletionRequest{
		Model: "auto",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "How do I format JSON in Go?"},
		},
	}
	respCoding, err := r.Execute(context.Background(), reqCoding)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if respCoding.Choices[0].Message.Content != "Claude response" {
		t.Errorf("Unexpected response content: %s", respCoding.Choices[0].Message.Content)
	}
}
