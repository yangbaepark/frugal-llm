package config

import (
	"fmt"
	"os"
	"strings"

	"frugal-llm/internal/adapter"
	"frugal-llm/internal/logger"

	"gopkg.in/yaml.v3"
)

const EnvPrefix = "FRUGAL_LLM_"

// LLMClassifierConfig configures parameters specific to LLM-based prompt classification.
type LLMClassifierConfig struct {
	Provider           string   `yaml:"provider,omitempty"`
	Model              string   `yaml:"model,omitempty"`
	TimeoutMS          int      `yaml:"timeout_ms,omitempty"`
	MaxPromptChars     int      `yaml:"max_prompt_chars,omitempty"`
	Temperature        *float64 `yaml:"temperature,omitempty"`
	MaxTokens          int      `yaml:"max_tokens,omitempty"`
	PromptTemplatePath string   `yaml:"prompt_template_path,omitempty"`
}

// ClassifierConfig represents a generic classifier entry embedding common metadata and type-specific configurations.
type ClassifierConfig struct {
	Name string `yaml:"name"` // Generic metadata: "llm-classifier", "regex-classifier"
	Type string `yaml:"type"` // Generic metadata: "llm", "regex"

	// LLM-specific parameters (inlined from YAML)
	LLM LLMClassifierConfig `yaml:",inline"`
}

// DynamicRoutingConfig holds settings for dynamic provider/model selection and prioritized classifiers pipeline.
type DynamicRoutingConfig struct {
	TriggerMode      string             `yaml:"trigger_mode"` // "always", "alias_only", "alias_or_unprefixed"
	AutoAliases      []string           `yaml:"auto_aliases"`
	FallbackProvider string             `yaml:"fallback_provider"`
	FallbackModel    string             `yaml:"fallback_model"`
	Classifiers      []ClassifierConfig `yaml:"classifiers"`
}

// ModelInfo defines a supported model under a provider instance with use-case description and cost factor.
type ModelInfo struct {
	Name        string `yaml:"name"`        // Model identifier e.g. "claude-fable-5", "gpt-5.6-sol"
	CostFactor  string `yaml:"cost_factor"` // Relative cost indicator e.g. "$0", "$", "$$", "$$$", "$$$$"
	Description string `yaml:"description"` // Short description of best use-cases & tier
}

// ServerConfig holds server-level configuration settings.
type ServerConfig struct {
	Port     string `yaml:"port"`
	APIKey   string `yaml:"api_key"`
	LogLevel string `yaml:"log_level"` // "debug", "info", "warn", "error"
}

// ProviderConfig represents configuration for a specific provider instance.
type ProviderConfig struct {
	Name         string      `yaml:"name"`           // Unique instance identifier e.g. "openai-main", "work-openai", "deepseek"
	Type         string      `yaml:"type"`           // Protocol type: "openai", "anthropic", "ollama", "huggingface"
	BaseURL      string      `yaml:"base_url"`
	APIKey       string      `yaml:"api_key"`
	TimeoutMS    int         `yaml:"timeout_ms"`     // Initial HTTP request timeout
	MaxRetries   int         `yaml:"max_retries"`    // Exponential backoff max retry count
	MaxTimeoutMS int         `yaml:"max_timeout_ms"` // Cap limit for exponential backoff delay
	Models       []ModelInfo `yaml:"models"`         // Supported models & use-case descriptions
}

// RawYAMLConfig matches the YAML file structure.
type RawYAMLConfig struct {
	Server         ServerConfig         `yaml:"server"`
	Providers      []ProviderConfig     `yaml:"providers"`
	DynamicRouting DynamicRoutingConfig `yaml:"dynamic_routing"`
}

// Config is the runtime configuration used by the proxy.
type Config struct {
	Port             string
	LogLevel         string
	ProxyAPIKey      string
	Providers        map[string]ProviderConfig
	DefinedProviders map[string]bool
	DynamicRouting   DynamicRoutingConfig
}

// LoadConfig attempts to load configuration from YAML file and strictly validates required providers and classifier settings.
func LoadConfig(configPath string) (*Config, error) {
	if configPath == "" {
		configPath = os.Getenv(EnvPrefix + "CONFIG")
	}
	if configPath == "" {
		configPath = "config.yaml"
	}

	cfg, err := loadYAMLConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse configuration file '%s': %w", configPath, err)
	}

	logger.SetLevel(cfg.LogLevel)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	logger.Infof("Loaded and validated configuration from '%s'", configPath)
	return cfg, nil
}

func loadYAMLConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Expand environment variables like ${FRUGAL_LLM_OPENAI_API_KEY} inside YAML file
	expandedData := os.ExpandEnv(string(data))

	var raw RawYAMLConfig
	if err := yaml.Unmarshal([]byte(expandedData), &raw); err != nil {
		return nil, err
	}

	logLevel := strings.ToLower(strings.TrimSpace(raw.Server.LogLevel))
	if logLevel == "" {
		logLevel = "debug"
	}

	cfg := &Config{
		Port:             raw.Server.Port,
		LogLevel:         logLevel,
		ProxyAPIKey:      raw.Server.APIKey,
		Providers:        make(map[string]ProviderConfig),
		DefinedProviders: make(map[string]bool),
		DynamicRouting:   raw.DynamicRouting,
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	if cfg.DynamicRouting.TriggerMode == "" {
		cfg.DynamicRouting.TriggerMode = "always"
	}
	if len(cfg.DynamicRouting.AutoAliases) == 0 {
		cfg.DynamicRouting.AutoAliases = []string{"auto", "frugal-router"}
	}
	if cfg.DynamicRouting.FallbackModel == "" {
		cfg.DynamicRouting.FallbackModel = "anthropic/claude-sonnet-5"
	}

	if len(cfg.DynamicRouting.Classifiers) == 0 {
		zeroTemp := 0.0
		cfg.DynamicRouting.Classifiers = []ClassifierConfig{
			{
				Name: "llm-classifier",
				Type: "llm",
				LLM: LLMClassifierConfig{
					Provider:           "local",
					Model:              "local-model",
					Temperature:        &zeroTemp,
					MaxTokens:          300,
					TimeoutMS:          60000,
					MaxPromptChars:     2000,
					PromptTemplatePath: "templates/classifier_prompt.tmpl",
				},
			},
		}
	}

	for i := range cfg.DynamicRouting.Classifiers {
		cl := &cfg.DynamicRouting.Classifiers[i]
		if cl.Type == "" {
			cl.Type = "llm"
		}
		if cl.Name == "" {
			cl.Name = cl.Type + "-classifier"
		}
		if strings.ToLower(cl.Type) == "llm" {
			if cl.LLM.TimeoutMS <= 0 {
				cl.LLM.TimeoutMS = 60000
			}
			if cl.LLM.MaxPromptChars <= 0 {
				cl.LLM.MaxPromptChars = 2000
			}
			if cl.LLM.MaxTokens <= 0 {
				cl.LLM.MaxTokens = 300
			}
			if cl.LLM.Temperature == nil {
				zeroTemp := 0.0
				cl.LLM.Temperature = &zeroTemp
			}
			if cl.LLM.PromptTemplatePath == "" {
				cl.LLM.PromptTemplatePath = "templates/classifier_prompt.tmpl"
			}
		}
	}

	for _, p := range raw.Providers {
		pName := strings.ToLower(strings.TrimSpace(p.Name))
		pType := strings.ToLower(strings.TrimSpace(p.Type))
		if pType == "" {
			pType = "openai"
		}

		baseURL := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
		apiKey := strings.TrimSpace(p.APIKey)

		p.Type = pType
		p.Name = pName
		p.BaseURL = baseURL
		p.APIKey = apiKey

		cfg.DefinedProviders[pName] = true

		// Check activation via provider adapter implementation
		if !adapter.IsActive(p.Name, p.Type, p.BaseURL, p.APIKey) {
			logger.Debugf("Provider instance '%s' skipped (inactive / missing credentials)", pName)
			continue
		}

		cfg.Providers[pName] = p
	}

	return cfg, nil
}

// Validate ensures that required provider and LLM classifier configurations exist and are valid.
func (c *Config) Validate() error {
	modelsCount := 0

	for pName, p := range c.Providers {
		if strings.TrimSpace(p.BaseURL) == "" {
			return fmt.Errorf("configuration error: provider '%s' base_url cannot be empty", pName)
		}
		if len(p.Models) == 0 {
			return fmt.Errorf("configuration error: provider '%s' has no models configured under 'models:' list", pName)
		}
		modelsCount += len(p.Models)
	}

	if len(c.Providers) == 0 {
		return fmt.Errorf("configuration error: no active provider instances found in configuration (check API key / local base_url environment variables)")
	}
	if modelsCount == 0 {
		return fmt.Errorf("configuration error: no supported models configured under active providers")
	}

	if len(c.DynamicRouting.Classifiers) == 0 {
		return fmt.Errorf("configuration error: dynamic_routing requires at least one classifier configured in 'classifiers:' list")
	}

	for i := range c.DynamicRouting.Classifiers {
		cl := &c.DynamicRouting.Classifiers[i]
		if strings.ToLower(cl.Type) == "llm" {
			if strings.TrimSpace(cl.LLM.Provider) == "" {
				return fmt.Errorf("configuration error: classifier '%s' requires 'provider' field", cl.Name)
			}
			if strings.TrimSpace(cl.LLM.Model) == "" {
				return fmt.Errorf("configuration error: classifier '%s' requires 'model' field", cl.Name)
			}
			pName := strings.ToLower(strings.TrimSpace(cl.LLM.Provider))
			if c.DefinedProviders != nil && !c.DefinedProviders[pName] {
				return fmt.Errorf("configuration error: classifier provider '%s' is not defined in providers list", cl.LLM.Provider)
			}
			_, active := c.Providers[pName]
			if !active {
				// Auto-assign classifier provider to first active provider if defined provider is inactive
				var firstActiveName string
				var firstActiveModel string
				for activeName, activeConfig := range c.Providers {
					firstActiveName = activeName
					if len(activeConfig.Models) > 0 {
						firstActiveModel = activeConfig.Models[0].Name
					}
					break
				}
				if firstActiveName != "" {
					logger.Infof("Configured classifier provider '%s' for classifier '%s' is inactive. Automatically routing LLM classifier to active provider '%s' (model: '%s')", cl.LLM.Provider, cl.Name, firstActiveName, firstActiveModel)
					cl.LLM.Provider = firstActiveName
					cl.LLM.Model = firstActiveModel
				} else {
					return fmt.Errorf("configuration error: classifier provider '%s' is defined but has no API key / base_url configured", cl.LLM.Provider)
				}
			}
		}
	}

	// Auto-assign fallback provider if configured fallback provider is inactive
	fbPName := strings.ToLower(strings.TrimSpace(c.DynamicRouting.FallbackProvider))
	_, fbExists := c.Providers[fbPName]
	if !fbExists {
		var firstActiveName string
		var firstActiveModel string
		for activeName, activeConfig := range c.Providers {
			firstActiveName = activeName
			if len(activeConfig.Models) > 0 {
				firstActiveModel = activeConfig.Models[0].Name
			}
			break
		}
		if firstActiveName != "" {
			c.DynamicRouting.FallbackProvider = firstActiveName
			c.DynamicRouting.FallbackModel = firstActiveModel
		}
	}

	return nil
}

func getEnv(key, fallback string) string {
	prefixedKey := EnvPrefix + key
	if val, ok := os.LookupEnv(prefixedKey); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	legacyKey := "SMART_LLM_PROXY_" + key
	if val, ok := os.LookupEnv(legacyKey); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return fallback
}
