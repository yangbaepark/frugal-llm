package adapter

import (
	"testing"
)

func TestCreateAdapterOpenAI(t *testing.T) {
	// Remote without API key -> inactive
	_, active := CreateAdapter("openai", "openai", "https://api.openai.com/v1", "")
	if active {
		t.Errorf("Expected remote OpenAI without API key to be inactive")
	}

	// Remote with API key -> active
	ad, active := CreateAdapter("openai", "openai", "https://api.openai.com/v1", "sk-test-key")
	if !active || ad == nil {
		t.Fatalf("Expected remote OpenAI with API key to be active")
	}
	if ad.Name() != "OpenAI" {
		t.Errorf("Expected adapter name 'OpenAI', got '%s'", ad.Name())
	}

	// Local provider without API key -> active
	localAd, localActive := CreateAdapter("local", "openai", "http://localhost:1234/v1", "")
	if !localActive || localAd == nil {
		t.Errorf("Expected local generic provider to be active without API key")
	}
}

func TestCreateAdapterAnthropic(t *testing.T) {
	// Remote without API key -> inactive
	_, active := CreateAdapter("anthropic", "anthropic", "https://api.anthropic.com/v1", "")
	if active {
		t.Errorf("Expected Anthropic without API key to be inactive")
	}

	// Remote with API key -> active
	ad, active := CreateAdapter("anthropic", "anthropic", "https://api.anthropic.com/v1", "sk-ant-test")
	if !active || ad == nil {
		t.Fatalf("Expected Anthropic with API key to be active")
	}
	if ad.Name() != "Anthropic" {
		t.Errorf("Expected adapter name 'Anthropic', got '%s'", ad.Name())
	}
}

func TestCreateAdapterOllama(t *testing.T) {
	// Empty BaseURL -> inactive
	_, active := CreateAdapter("ollama", "ollama", "", "")
	if active {
		t.Errorf("Expected Ollama with empty BaseURL to be inactive")
	}

	// Non-empty BaseURL -> active
	ad, active := CreateAdapter("ollama", "ollama", "http://localhost:11434", "")
	if !active || ad == nil {
		t.Fatalf("Expected Ollama with BaseURL to be active")
	}
	if ad.Name() != "Ollama" {
		t.Errorf("Expected adapter name 'Ollama', got '%s'", ad.Name())
	}
}

func TestCreateAdapterHuggingFace(t *testing.T) {
	// Remote without API key -> inactive
	_, active := CreateAdapter("huggingface", "huggingface", "https://api-inference.huggingface.co/models", "")
	if active {
		t.Errorf("Expected HuggingFace without API key to be inactive")
	}

	// Remote with API key -> active
	ad, active := CreateAdapter("huggingface", "huggingface", "https://api-inference.huggingface.co/models", "hf_test")
	if !active || ad == nil {
		t.Fatalf("Expected HuggingFace with API key to be active")
	}
	if ad.Name() != "HuggingFace" {
		t.Errorf("Expected adapter name 'HuggingFace', got '%s'", ad.Name())
	}
}
