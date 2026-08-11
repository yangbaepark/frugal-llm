package config

import (
	"os"
	"testing"
)

func TestConfigValidationSuccess(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"openai": {
				Name:    "openai",
				Type:    "openai",
				BaseURL: "https://api.openai.com/v1",
				Models: []ModelInfo{
					{Name: "gpt-4o", Description: "Flagship model"},
				},
			},
		},
		DynamicRouting: DynamicRoutingConfig{
			Classifiers: []ClassifierConfig{
				{
					Name: "llm-classifier",
					Type: "llm",
					LLM: LLMClassifierConfig{
						Provider: "openai",
						Model:    "gpt-4o",
					},
				},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Expected valid config, got error: %v", err)
	}
}

func TestConfigValidationMissingProviders(t *testing.T) {
	cfg := &Config{
		Providers: make(map[string]ProviderConfig),
	}

	err := cfg.Validate()
	if err == nil {
		t.Errorf("Expected error when no providers are configured, got nil")
	}
}

func TestConfigValidationMissingModels(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"openai": {
				Name:    "openai",
				Type:    "openai",
				BaseURL: "https://api.openai.com/v1",
				Models:  []ModelInfo{}, // Empty models list
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Errorf("Expected error when provider models list is empty, got nil")
	}
}

func TestConfigValidationMissingClassifierProvider(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"openai": {
				Name:    "openai",
				Type:    "openai",
				BaseURL: "https://api.openai.com/v1",
				Models: []ModelInfo{
					{Name: "gpt-4o", Description: "Flagship model"},
				},
			},
		},
		DynamicRouting: DynamicRoutingConfig{
			Classifiers: []ClassifierConfig{
				{
					Name: "llm-classifier",
					Type: "llm",
					LLM: LLMClassifierConfig{
						Provider: "", // Missing classifier provider
						Model:    "gpt-4o",
					},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Errorf("Expected error when LLM classifier provider is missing, got nil")
	}
}

func TestConfigValidationNonExistentClassifierProvider(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"openai": {
				Name:    "openai",
				Type:    "openai",
				BaseURL: "https://api.openai.com/v1",
				Models: []ModelInfo{
					{Name: "gpt-4o", Description: "Flagship model"},
				},
			},
		},
		DefinedProviders: map[string]bool{
			"openai": true,
		},
		DynamicRouting: DynamicRoutingConfig{
			Classifiers: []ClassifierConfig{
				{
					Name: "llm-classifier",
					Type: "llm",
					LLM: LLMClassifierConfig{
						Provider: "non-existent-provider", // Not defined in providers map
						Model:    "gpt-4o",
					},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Errorf("Expected error when LLM classifier provider does not match any enabled provider, got nil")
	}
}

func TestEnvVarExpansionInConfig(t *testing.T) {
	t.Setenv("TEST_FRUGAL_OPENAI_KEY", "sk-test-openai-12345")
	t.Setenv("TEST_FRUGAL_GEMINI_KEY", "AIzaSyTestGemini67890")

	tmpFile, err := os.CreateTemp("", "config_test_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `
server:
  port: "8080"
providers:
  - name: "openai"
    type: "openai"
    base_url: "https://api.openai.com/v1"
    api_key: "${TEST_FRUGAL_OPENAI_KEY}"
    models:
      - name: "gpt-4o"
        description: "Flagship"
  - name: "gemini"
    type: "openai"
    base_url: "https://generativelanguage.googleapis.com/v1beta/openai"
    api_key: "${TEST_FRUGAL_GEMINI_KEY}"
    models:
      - name: "gemini-3.1-pro"
        description: "Reasoning"
dynamic_routing:
  classifiers:
    - name: "llm-classifier"
      type: "llm"
      provider: "openai"
      model: "gpt-4o"
`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp config file: %v", err)
	}
	tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Providers["openai"].APIKey != "sk-test-openai-12345" {
		t.Errorf("Expected openai APIKey 'sk-test-openai-12345', got '%s'", cfg.Providers["openai"].APIKey)
	}
	if cfg.Providers["gemini"].APIKey != "AIzaSyTestGemini67890" {
		t.Errorf("Expected gemini APIKey 'AIzaSyTestGemini67890', got '%s'", cfg.Providers["gemini"].APIKey)
	}
}

func TestLocalProviderActivationViaBaseURL(t *testing.T) {
	t.Setenv("TEST_FRUGAL_LOCAL_URL", "http://localhost:1234/v1")

	tmpFile, err := os.CreateTemp("", "config_test_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `
server:
  port: "8080"
providers:
  - name: "local"
    type: "openai"
    base_url: "${TEST_FRUGAL_LOCAL_URL}"
    api_key: ""
    models:
      - name: "local-model"
        description: "Local model"
dynamic_routing:
  classifiers:
    - name: "llm-classifier"
      type: "llm"
      provider: "local"
      model: "local-model"
`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp config file: %v", err)
	}
	tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if _, ok := cfg.Providers["local"]; !ok {
		t.Errorf("Expected 'local' provider to be active when BaseURL is set, but it was missing")
	}
}
