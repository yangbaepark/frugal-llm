package selector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"frugal-llm/internal/config"
	"frugal-llm/internal/logger"
)

// SystemOneRequest represents the wire request to a System One / Jev / Kev endpoint.
type SystemOneRequest struct {
	Model     string                        `json:"model,omitempty"`
	State     string                        `json:"state"`
	Questions map[string]SystemOneQuestion   `json:"questions"`
}

// SystemOneQuestion defines a Choice, Noul, or Score question within the System One protocol.
type SystemOneQuestion struct {
	Type         string            `json:"type"`                   // "choice", "noul", or "score"
	Instructions string            `json:"instructions,omitempty"`
	Criteria     any               `json:"criteria,omitempty"`     // map[string]string for choice, []string for score
	Options      []string          `json:"options,omitempty"`      // alternative array for choice
	Min          *int              `json:"min,omitempty"`          // for numeric score
	Max          *int              `json:"max,omitempty"`          // for numeric score
}

// SystemOneResponse represents the wire response from a System One endpoint.
type SystemOneResponse struct {
	Model     string                     `json:"model,omitempty"`
	Answers   map[string]SystemOneAnswer `json:"answers"`
	Usage     map[string]int             `json:"usage,omitempty"`
	LatencyMs float64                    `json:"latency_ms,omitempty"`
}

// SystemOneAnswer contains the typed result and calibrated probabilities.
type SystemOneAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice,omitempty"`        // for choice
	Noul          *float64           `json:"noul,omitempty"`          // for noul (0.0 - 1.0)
	Score         *float64           `json:"score,omitempty"`         // for score
	Confidence    float64            `json:"confidence,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
}

// classifyWithSystemOne evaluates prompt state with candidate criteria using the System One wire protocol.
func (s *DynamicSelector) classifyWithSystemOne(ctx context.Context, cfg config.SystemOneClassifierConfig, prompt string) (string, error) {
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	classCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build candidate criteria mapping from provider groups
	criteria := make(map[string]string)
	for _, group := range s.providers {
		for _, m := range group.Models {
			canonical := FormatTargetModel(group.Name, m.Name)
			desc := strings.TrimSpace(m.Description)
			if desc == "" {
				desc = fmt.Sprintf("Model %s under provider %s (cost factor: %s)", m.Name, group.Name, m.CostFactor)
			} else if m.CostFactor != "" {
				desc = fmt.Sprintf("%s (Cost: %s)", desc, m.CostFactor)
			}
			criteria[canonical] = desc
		}
	}

	if len(criteria) == 0 {
		return "", fmt.Errorf("no candidate models available for System One classification")
	}

	instructions := strings.TrimSpace(cfg.Instructions)
	if instructions == "" {
		instructions = "Select the most cost-effective and capable LLM model to fulfill this request."
	}

	reqBody := SystemOneRequest{
		Model: cfg.Model,
		State: prompt,
		Questions: map[string]SystemOneQuestion{
			"route": {
				Type:         "choice",
				Instructions: instructions,
				Criteria:     criteria,
			},
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal System One request: %w", err)
	}

	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(cfg.Endpoint)
	}
	if baseURL == "" {
		baseURL = "https://api.typesafe.ai/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	targetURL := baseURL
	if !strings.HasSuffix(targetURL, "/systemone") {
		targetURL = targetURL + "/systemone"
	}

	logger.Debugf("[Classifier:SystemOne] POST %s (Model: %s, Candidates: %d, State length: %d)", targetURL, cfg.Model, len(criteria), len(prompt))

	httpReq, err := http.NewRequestWithContext(classCtx, "POST", targetURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create System One HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(cfg.APIKey) != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", strings.TrimSpace(cfg.APIKey)))
	}

	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("System One HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read System One response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("System One server returned HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var sysOneResp SystemOneResponse
	if err := json.Unmarshal(respBytes, &sysOneResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal System One response: %w", err)
	}

	routeAnswer, ok := sysOneResp.Answers["route"]
	if !ok {
		// Fallback: search for any choice answer
		for _, ans := range sysOneResp.Answers {
			if ans.Choice != "" {
				routeAnswer = ans
				ok = true
				break
			}
		}
	}

	if !ok || routeAnswer.Choice == "" {
		return "", fmt.Errorf("System One response contains no choice answer")
	}

	// Check confidence threshold if configured
	if cfg.ConfidenceThreshold > 0 {
		confidence := routeAnswer.Confidence
		if confidence == 0 && len(routeAnswer.Probabilities) > 0 {
			confidence = routeAnswer.Probabilities[routeAnswer.Choice]
		}
		if confidence < cfg.ConfidenceThreshold {
			return "", fmt.Errorf("System One confidence (%.2f) below threshold (%.2f)", confidence, cfg.ConfidenceThreshold)
		}
	}

	choice := strings.TrimSpace(routeAnswer.Choice)
	lowerChoice := strings.ToLower(choice)

	// Exact target match
	if target, exists := s.targetsMap[lowerChoice]; exists {
		logger.Debugf("[Classifier:SystemOne] Matched target: '%s' (Choice: '%s', Confidence: %.2f)", target, choice, routeAnswer.Confidence)
		return target, nil
	}

	// Partial target match
	for targetKey, canonicalTarget := range s.targetsMap {
		if strings.Contains(lowerChoice, targetKey) {
			logger.Debugf("[Classifier:SystemOne] Partial matched target: '%s'", canonicalTarget)
			return canonicalTarget, nil
		}
	}

	return "", fmt.Errorf("unrecognized System One choice target: '%s'", choice)
}
