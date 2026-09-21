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

func TestSystemOneClassifierConfigParsing(t *testing.T) {
	t.Setenv("TEST_FRUGAL_OPENAI_KEY", "sk-test-key")

	tmpFile, err := os.CreateTemp("", "config_sysone_*.yaml")
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
dynamic_routing:
  classifiers:
    - name: "my-system-one"
      type: "system-one"
      base_url: "https://api.typesafe.ai/v1"
      model: "jev-latest"
      confidence_threshold: 0.75
      timeout_ms: 2500
      max_prompt_chars: 3500
      instructions: "Route request"
`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp config file: %v", err)
	}
	tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(cfg.DynamicRouting.Classifiers) != 1 {
		t.Fatalf("Expected 1 classifier, got %d", len(cfg.DynamicRouting.Classifiers))
	}

	cl := cfg.DynamicRouting.Classifiers[0]
	if cl.Type != "system-one" {
		t.Errorf("Expected type 'system-one', got '%s'", cl.Type)
	}
	if cl.SystemOne.BaseURL != "https://api.typesafe.ai/v1" {
		t.Errorf("Expected BaseURL 'https://api.typesafe.ai/v1', got '%s'", cl.SystemOne.BaseURL)
	}
	if cl.SystemOne.Model != "jev-latest" {
		t.Errorf("Expected Model 'jev-latest', got '%s'", cl.SystemOne.Model)
	}
	if cl.SystemOne.ConfidenceThreshold != 0.75 {
		t.Errorf("Expected ConfidenceThreshold 0.75, got %f", cl.SystemOne.ConfidenceThreshold)
	}
	if cl.SystemOne.TimeoutMS != 2500 {
		t.Errorf("Expected TimeoutMS 2500, got %d", cl.SystemOne.TimeoutMS)
	}
	if cl.SystemOne.MaxPromptChars != 3500 {
		t.Errorf("Expected MaxPromptChars 3500, got %d", cl.SystemOne.MaxPromptChars)
	}
}

func TestSystemOneClassifierValidationConfidenceRange(t *testing.T) {
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
					Name: "system-one-classifier",
					Type: "system-one",
					SystemOne: SystemOneClassifierConfig{
						BaseURL:             "https://api.typesafe.ai/v1",
						ConfidenceThreshold: 1.5, // Invalid: must be <= 1.0
					},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Errorf("Expected error for confidence_threshold > 1.0, got nil")
	}
}

func TestClassifierSequenceInConfig(t *testing.T) {
	t.Setenv("TEST_FRUGAL_OPENAI_KEY", "sk-test-key")

	tmpFile, err := os.CreateTemp("", "config_seq_*.yaml")
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
dynamic_routing:
  classifiers:
    - name: "fast-system-one"
      type: "system-one"
      base_url: "http://localhost:8009/v1"
      model: "kev-4b"
      confidence_threshold: 0.8
    - name: "secondary-llm"
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

	if len(cfg.DynamicRouting.Classifiers) != 2 {
		t.Fatalf("Expected 2 classifiers in sequence, got %d", len(cfg.DynamicRouting.Classifiers))
	}
	if cfg.DynamicRouting.Classifiers[0].Name != "fast-system-one" || cfg.DynamicRouting.Classifiers[0].Type != "system-one" {
		t.Errorf("Classifier 0 unexpected: %+v", cfg.DynamicRouting.Classifiers[0])
	}
	if cfg.DynamicRouting.Classifiers[1].Name != "secondary-llm" || cfg.DynamicRouting.Classifiers[1].Type != "llm" {
		t.Errorf("Classifier 1 unexpected: %+v", cfg.DynamicRouting.Classifiers[1])
	}
}
