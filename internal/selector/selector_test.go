package selector

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"frugal-llm/internal/config"
	"frugal-llm/internal/model"
)

type mockExecutor struct {
	lastPrompt string
	response   string
	err        error
}

func (m *mockExecutor) ExecuteInternal(ctx context.Context, modelName string, prompt string, temperature *float64, maxTokens int) (string, error) {
	m.lastPrompt = prompt
	return m.response, m.err
}

func testProviders() map[string]config.ProviderConfig {
	return map[string]config.ProviderConfig{
		"openai": {
			Name: "openai",
			Type: "openai",
			Models: []config.ModelInfo{
				{Name: "gpt-5.6-sol", Description: "Legal precision model"},
				{Name: "gpt-5.6-terra", Description: "Ultra-fast casual chat model"},
			},
		},
		"anthropic": {
			Name: "anthropic",
			Type: "anthropic",
			Models: []config.ModelInfo{
				{Name: "claude-sonnet-5", Description: "Balanced coding model"},
			},
		},
	}
}

func testConfig() config.DynamicRoutingConfig {
	return config.DynamicRoutingConfig{
		TriggerMode:      "always",
		AutoAliases:      []string{"auto", "frugal-router"},
		FallbackProvider: "anthropic",
		FallbackModel:    "claude-sonnet-5",
		Classifiers: []config.ClassifierConfig{
			{
				Name: "llm-classifier",
				Type: "llm",
				LLM: config.LLMClassifierConfig{
					Provider:           "lmstudio",
					Model:              "local-model",
					TimeoutMS:          1000,
					PromptTemplatePath: "../../templates/classifier_prompt.tmpl",
				},
			},
		},
	}
}

func TestShouldSelect(t *testing.T) {
	cfg := testConfig()
	sel := NewDynamicSelector(cfg, testProviders(), nil)

	if !sel.ShouldSelect("auto") {
		t.Errorf("Expected 'auto' to trigger dynamic selection")
	}

	if !sel.ShouldSelect("frugal-router") {
		t.Errorf("Expected 'frugal-router' to trigger dynamic selection")
	}

	if !sel.ShouldSelect("gpt-4o") {
		t.Errorf("Expected 'gpt-4o' to trigger dynamic selection under 'always' mode")
	}
}

func TestLLMClassifierTemplateAndSelection(t *testing.T) {
	cfg := testConfig()
	exec := &mockExecutor{response: "openai/gpt-5.6-sol"}
	sel := NewDynamicSelector(cfg, testProviders(), exec)

	req := &model.ChatCompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "Should I sign an NDA before pitching my startup?"}},
	}

	gotModel, reason := sel.SelectModel(context.Background(), req)
	if gotModel != "openai/gpt-5.6-sol" {
		t.Errorf("Expected LLM classifier to route to 'openai/gpt-5.6-sol', got %v", gotModel)
	}
	if reason != "LLM Classifier: openai/gpt-5.6-sol" {
		t.Errorf("Expected reason 'LLM Classifier: openai/gpt-5.6-sol', got %v", reason)
	}

	// Verify template prompt content
	if !strings.Contains(exec.lastPrompt, "Provider: \"openai\" (Type: openai)") {
		t.Errorf("Prompt template missing provider header")
	}
	if !strings.Contains(exec.lastPrompt, "Target: \"openai/gpt-5.6-sol\"") {
		t.Errorf("Prompt template missing target model entry")
	}
	if !strings.Contains(exec.lastPrompt, "Should I sign an NDA before pitching my startup?") {
		t.Errorf("Prompt template missing user prompt")
	}
}

func TestClassifierFallbackOnFailure(t *testing.T) {
	cfg := testConfig()
	exec := &mockExecutor{err: fmt.Errorf("network timeout")}
	sel := NewDynamicSelector(cfg, testProviders(), exec)

	req := &model.ChatCompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "Unrecognized topic prompt"}},
	}

	gotModel, reason := sel.SelectModel(context.Background(), req)
	if gotModel != "anthropic/claude-sonnet-5" {
		t.Errorf("Expected fallback model 'anthropic/claude-sonnet-5', got %v", gotModel)
	}
	if reason != "Fallback Model" {
		t.Errorf("Expected reason 'Fallback Model', got %v", reason)
	}
}

func TestFormatTargetModel(t *testing.T) {
	tests := []struct {
		provider string
		model    string
		want     string
	}{
		{provider: "groq", model: "llama3", want: "groq/llama3"},
		{provider: "groq", model: "groq/llama3", want: "groq/llama3"},
		{provider: "", model: "openai/gpt-4o", want: "openai/gpt-4o"},
		{provider: "", model: "gpt-4o", want: "gpt-4o"},
	}

	for _, tt := range tests {
		got := FormatTargetModel(tt.provider, tt.model)
		if got != tt.want {
			t.Errorf("FormatTargetModel(%q, %q) = %q, want %q", tt.provider, tt.model, got, tt.want)
		}
	}
}

func TestExtractPromptSingleMessageExceedsLimit(t *testing.T) {
	// 1. Single prompt exceeding limit: Should use 100% of maxChars (not capped at 40%)
	longPrompt := strings.Repeat("A", 3000)
	req := &model.ChatCompletionRequest{
		Messages: []model.ChatMessage{
			{Role: "user", Content: longPrompt},
		},
	}

	maxChars := 1000
	extracted := ExtractPrompt(req, maxChars)

	if !strings.HasSuffix(extracted, "... [truncated]") {
		t.Errorf("Expected truncated output, got: %s", extracted)
	}

	// Length should be maxChars + len("... [truncated]")
	expectedLen := maxChars + len("... [truncated]")
	if len(extracted) != expectedLen {
		t.Errorf("Expected extracted length %d (100%% of budget), got %d", expectedLen, len(extracted))
	}
}

func TestExtractPromptMultiMessageUnderLimit(t *testing.T) {
	// 2. Multi-turn session under limit: Should NOT truncate any message
	req := &model.ChatCompletionRequest{
		Messages: []model.ChatMessage{
			{Role: "system", Content: "System instructions"},
			{Role: "user", Content: "Initial Goal: Build a Go HTTP router server"},
			{Role: "assistant", Content: "Sure, let's start!"},
			{Role: "user", Content: "Intermediate prompt: Add CORS middleware"},
			{Role: "user", Content: "Latest prompt: Fix routing deadlock in ServeHTTP"},
		},
	}

	extracted := ExtractPrompt(req, 2000)
	if strings.Contains(extracted, "... [truncated]") {
		t.Errorf("Did not expect any truncation when under limit, got: %s", extracted)
	}
	if !strings.Contains(extracted, "[Initial Goal]: Initial Goal: Build a Go HTTP router server") {
		t.Errorf("Extracted prompt missing initial goal: %s", extracted)
	}
	if !strings.Contains(extracted, "[Recent Prompt]: Latest prompt: Fix routing deadlock in ServeHTTP") {
		t.Errorf("Extracted prompt missing latest prompt: %s", extracted)
	}
}

func TestExtractPromptMultiMessageOverLimitLargeInitial(t *testing.T) {
	// 3. Multi-turn session over limit with huge initial prompt:
	// Initial prompt (3000 chars) + Recent prompt (400 chars) with maxChars = 1000.
	// Initial goal should be capped at ~40% (400 chars), allowing recent prompt to be included!
	hugeInitial := strings.Repeat("X", 3000)
	recentPrompt := "Latest request: Fix race condition"
	req := &model.ChatCompletionRequest{
		Messages: []model.ChatMessage{
			{Role: "user", Content: hugeInitial},
			{Role: "assistant", Content: "OK"},
			{Role: "user", Content: recentPrompt},
		},
	}

	maxChars := 1000
	extracted := ExtractPrompt(req, maxChars)

	if !strings.Contains(extracted, "[Initial Goal]:") {
		t.Errorf("Expected [Initial Goal] header, got: %s", extracted)
	}
	if !strings.Contains(extracted, "[Recent Prompt]: Latest request: Fix race condition") {
		t.Errorf("Expected recent prompt to be preserved, got: %s", extracted)
	}
	if !strings.Contains(extracted, "... [truncated]") {
		t.Errorf("Expected initial prompt to be truncated, got: %s", extracted)
	}
}
