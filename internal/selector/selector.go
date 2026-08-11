package selector

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"
	"time"

	"frugal-llm/internal/config"
	"frugal-llm/internal/logger"
	"frugal-llm/internal/model"
)

// ClassificationExecutor defines the interface for running classification LLM prompts.
type ClassificationExecutor interface {
	ExecuteInternal(ctx context.Context, modelName string, prompt string, temperature *float64, maxTokens int) (string, error)
}

const defaultClassifierPromptTmpl = `You are an AI routing classifier. DO NOT answer the user's prompt.
Your ONLY task is to analyze the user's prompt and select the EXACT candidate provider and model (formatted as "provider/model") that is most cost-efficient and capable for the prompt's subject matter and complexity.

AVAILABLE PROVIDERS & MODELS:
{{range .Providers}}Provider: "{{.Name}}" (Type: {{.Type}})
{{range .Models}}  - Target: "{{$.Name}}/{{.Name}}"
    Description: {{.Description}}
{{end}}
{{end}}
CRITICAL INSTRUCTIONS:
1. Do NOT attempt to answer, solve, or fulfill the user's prompt.
2. Select the most cost-efficient provider/model target that satisfies the prompt's complexity requirements. Do NOT select a high-cost tier unless the prompt genuinely requires high-precision reasoning or domain expertise.
3. Respond ONLY with the exact Target identifier string (e.g. "provider/model").
4. Do NOT include any markdown formatting, quotes, explanation, or punctuation.

USER PROMPT TO CLASSIFY:
"""
{{.UserPrompt}}
"""`

// ProviderGroup represents a provider instance and its supported models for template rendering.
type ProviderGroup struct {
	Name   string
	Type   string
	Models []config.ModelInfo
}

// PromptData is the data structure passed to the Go prompt template.
type PromptData struct {
	Providers  []ProviderGroup
	UserPrompt string
}

// DynamicSelector manages LLM-based classification for model selection.
type DynamicSelector struct {
	cfg         config.DynamicRoutingConfig
	providers   []ProviderGroup
	targetsMap  map[string]string // maps lower-case target strings to canonical "provider/model"
	aliasesMap  map[string]bool
	executor    ClassificationExecutor
	promptTmpl  *template.Template
}

// NewDynamicSelector initializes dynamic routing rules and builds candidate provider model pools.
func NewDynamicSelector(cfg config.DynamicRoutingConfig, providerConfigs map[string]config.ProviderConfig, exec ClassificationExecutor) *DynamicSelector {
	aliasesMap := make(map[string]bool)
	for _, alias := range cfg.AutoAliases {
		aliasesMap[strings.ToLower(alias)] = true
	}

	var providerGroups []ProviderGroup
	targetsMap := make(map[string]string)

	for pName, pConfig := range providerConfigs {
		if len(pConfig.Models) > 0 {
			group := ProviderGroup{
				Name:   pName,
				Type:   pConfig.Type,
				Models: pConfig.Models,
			}
			providerGroups = append(providerGroups, group)
			for _, m := range pConfig.Models {
				canonical := FormatTargetModel(pName, m.Name)
				targetsMap[strings.ToLower(canonical)] = canonical
				// Also index by bare model name for resilient matching
				targetsMap[strings.ToLower(m.Name)] = canonical
			}
		}
	}

	var tmpl *template.Template
	tmplPath := ""
	for _, c := range cfg.Classifiers {
		if strings.ToLower(c.Type) == "llm" && c.LLM.PromptTemplatePath != "" {
			tmplPath = c.LLM.PromptTemplatePath
			break
		}
	}

	if tmplPath != "" {
		if _, err := os.Stat(tmplPath); err == nil {
			t, err := template.ParseFiles(tmplPath)
			if err == nil {
				tmpl = t
				logger.Infof("Loaded classifier prompt template from '%s'", tmplPath)
			} else {
				logger.Warnf("Failed to parse template at '%s': %v. Falling back to built-in template.", tmplPath, err)
			}
		}
	}

	if tmpl == nil {
		t, err := template.New("default_classifier").Parse(defaultClassifierPromptTmpl)
		if err != nil {
			log.Fatalf("Fatal error parsing default classifier prompt template: %v", err)
		}
		tmpl = t
	}

	return &DynamicSelector{
		cfg:        cfg,
		providers:  providerGroups,
		targetsMap: targetsMap,
		aliasesMap: aliasesMap,
		executor:   exec,
		promptTmpl: tmpl,
	}
}

// ShouldSelect returns true if dynamic routing should be applied to the requested model.
func (s *DynamicSelector) ShouldSelect(modelName string) bool {
	lower := strings.ToLower(modelName)
	if s.aliasesMap[lower] {
		return true
	}

	switch strings.ToLower(s.cfg.TriggerMode) {
	case "alias_only":
		return s.aliasesMap[lower]
	case "alias_or_unprefixed":
		return !strings.Contains(modelName, "/")
	case "always":
		fallthrough
	default:
		return true
	}
}

// SelectModel evaluates the prompt through the prioritized classifiers pipeline.
func (s *DynamicSelector) SelectModel(ctx context.Context, req *model.ChatCompletionRequest) (string, string) {
	fallbackTarget := FormatTargetModel(s.cfg.FallbackProvider, s.cfg.FallbackModel)

	for _, c := range s.cfg.Classifiers {
		switch strings.ToLower(c.Type) {
		case "llm":
			prompt := ExtractPrompt(req, c.LLM.MaxPromptChars)
			if prompt == "" {
				continue
			}
			if s.executor != nil {
				targetModel, err := s.classifyWithLLM(ctx, c.LLM, prompt)
				if err == nil && targetModel != "" {
					return targetModel, fmt.Sprintf("LLM Classifier: %s", targetModel)
				}
				logger.Warnf("LLM classification via classifier '%s' failed or timed out (%v). Trying next pipeline option...", c.Name, err)
			}
		}
	}

	// Default Fallback
	return fallbackTarget, "Fallback Model"
}

func (s *DynamicSelector) classifyWithLLM(ctx context.Context, cfg config.LLMClassifierConfig, prompt string) (string, error) {
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	classCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Render prompt template
	data := PromptData{
		Providers:  s.providers,
		UserPrompt: prompt,
	}

	var buf bytes.Buffer
	if err := s.promptTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render classifier prompt template: %w", err)
	}

	classifierModel := FormatTargetModel(cfg.Provider, cfg.Model)
	logger.Debugf("[Classifier] Requesting prompt classification via '%s' (Extracted prompt: %d chars, Rendered template: %d bytes)...", classifierModel, len(prompt), buf.Len())

	rawResp, err := s.executor.ExecuteInternal(classCtx, classifierModel, buf.String(), cfg.Temperature, cfg.MaxTokens)
	if err != nil {
		logger.Warnf("[Classifier] LLM classification request failed: %v", err)
		return "", err
	}

	logger.Debugf("[Classifier] Raw classifier response: '%s'", rawResp)

	cleaned := strings.TrimSpace(strings.ToLower(rawResp))
	// Clean punctuation if model returned quotes or trailing periods
	cleaned = strings.Trim(cleaned, "\"`'.,\n\r")

	// Exact target match
	if target, ok := s.targetsMap[cleaned]; ok {
		logger.Debugf("[Classifier] Successfully matched target: '%s'", target)
		return target, nil
	}

	// Partial target match
	for targetKey, canonicalTarget := range s.targetsMap {
		if strings.Contains(cleaned, targetKey) {
			logger.Debugf("[Classifier] Partial matched target: '%s'", canonicalTarget)
			return canonicalTarget, nil
		}
	}

	logger.Warnf("[Classifier] Unrecognized classifier output: '%s'", rawResp)
	return "", fmt.Errorf("unrecognized classifier output: '%s'", rawResp)
}

// FormatTargetModel formats the provider instance and model into a canonical "provider/model" string.
func FormatTargetModel(provider, modelName string) string {
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	if provider != "" {
		if idx := strings.Index(modelName, "/"); idx != -1 {
			modelName = modelName[idx+1:]
		}
		return fmt.Sprintf("%s/%s", provider, modelName)
	}
	return modelName
}

// ExtractPrompt collects user prompts while enforcing maxChars. Truncation occurs ONLY if total combined content size exceeds maxChars.
func ExtractPrompt(req *model.ChatCompletionRequest, maxChars int) string {
	if maxChars <= 0 {
		maxChars = 2000
	}
	if len(req.Messages) == 0 {
		return ""
	}

	// 1. Collect non-empty user message contents
	var userMsgs []string
	for _, m := range req.Messages {
		if strings.EqualFold(m.Role, "user") && strings.TrimSpace(m.Content) != "" {
			userMsgs = append(userMsgs, strings.TrimSpace(m.Content))
		}
	}

	if len(userMsgs) == 0 {
		fallback := strings.TrimSpace(req.Messages[len(req.Messages)-1].Content)
		if len(fallback) > maxChars {
			return fallback[:maxChars] + "... [truncated]"
		}
		return fallback
	}

	// Single user message: use up to 100% of maxChars
	if len(userMsgs) == 1 {
		msg := userMsgs[0]
		if len(msg) > maxChars {
			return msg[:maxChars] + "... [truncated]"
		}
		return msg
	}

	// Multiple user messages: calculate total un-truncated length
	totalLen := 0
	for _, msg := range userMsgs {
		totalLen += len(msg)
	}

	// If total combined content fits within maxChars, return all messages completely
	if totalLen <= maxChars {
		var parts []string
		parts = append(parts, fmt.Sprintf("[Initial Goal]: %s", userMsgs[0]))
		for i := 1; i < len(userMsgs); i++ {
			parts = append(parts, fmt.Sprintf("[Recent Prompt]: %s", userMsgs[i]))
		}
		return strings.Join(parts, "\n---\n")
	}

	// Combined content EXCEEDS maxChars. Dynamically balance Initial Goal (40%) vs Recent Requests (60%)
	recentCap := int(float64(maxChars) * 0.6)
	initialCap := maxChars - recentCap

	firstMsg := userMsgs[0]
	if len(firstMsg) > initialCap {
		firstMsg = firstMsg[:initialCap] + "... [truncated]"
	}
	firstHeader := fmt.Sprintf("[Initial Goal]: %s", firstMsg)
	usedChars := len(firstHeader)

	var recentParts []string
	for i := len(userMsgs) - 1; i > 0; i-- {
		msg := userMsgs[i]
		header := fmt.Sprintf("[Recent Prompt]: %s", msg)
		needed := len(header) + 5

		if usedChars+needed > maxChars {
			rem := maxChars - usedChars - 25
			if rem > 80 {
				recentParts = append([]string{fmt.Sprintf("[Recent Prompt]: %s... [truncated]", msg[:rem])}, recentParts...)
			}
			break
		}

		recentParts = append([]string{header}, recentParts...)
		usedChars += needed
	}

	parts := append([]string{firstHeader}, recentParts...)
	return strings.Join(parts, "\n---\n")
}
