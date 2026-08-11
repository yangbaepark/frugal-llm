package router

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"frugal-llm/internal/adapter"
	"frugal-llm/internal/config"
	"frugal-llm/internal/logger"
	"frugal-llm/internal/model"
	"frugal-llm/internal/protocol"
	"frugal-llm/internal/selector"
)

// Router acts as the core proxy engine and routes requests between inbound protocol handlers and outbound provider adapters.
type Router struct {
	cfg        *config.Config
	httpClient *http.Client
	adapters   map[string]adapter.ProviderAdapter
	mux        *http.ServeMux
	selector   *selector.DynamicSelector
}

func NewRouter(cfg *config.Config) *Router {
	r := &Router{
		cfg: cfg,
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		adapters: make(map[string]adapter.ProviderAdapter),
		mux:      http.NewServeMux(),
	}

	// 1. Instantiates adapters for all configured provider instances
	for pName, pConfig := range cfg.Providers {
		switch strings.ToLower(pConfig.Type) {
		case "anthropic":
			r.adapters[pName] = adapter.NewAnthropicAdapter(pConfig.BaseURL, pConfig.APIKey)
		case "ollama":
			r.adapters[pName] = adapter.NewOllamaAdapter(pConfig.BaseURL)
		case "huggingface":
			r.adapters[pName] = adapter.NewHuggingFaceAdapter(pConfig.BaseURL, pConfig.APIKey)
		default: // "openai" or any OpenAI-compatible provider
			r.adapters[pName] = adapter.NewOpenAIAdapter(pConfig.BaseURL, pConfig.APIKey)
		}
		logger.Infof("[Proxy] Registered provider instance '%s' (Type: %s, Endpoint: %s)", pName, pConfig.Type, pConfig.BaseURL)
	}

	// 2. Initialize Dynamic Selector layer
	r.selector = selector.NewDynamicSelector(cfg.DynamicRouting, cfg.Providers, r)

	// 3. Register health check endpoint
	r.mux.HandleFunc("/health", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// 4. Register inbound protocol handlers
	r.registerProtocolHandler(protocol.NewOpenAIProtocolHandler())

	return r
}

func (r *Router) registerProtocolHandler(handler protocol.ProtocolHandler) {
	logger.Infof("[Proxy] Registered inbound protocol handler: %s", handler.Name())
	handler.RegisterRoutes(r.mux, r)
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.statusCode = code
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *statusRecorder) Flush() {
	if f, ok := rec.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logger.Debugf("[HTTP] %s %s", req.Method, req.URL.Path)

	if r.cfg.ProxyAPIKey != "" && req.URL.Path != "/health" {
		authHeader := req.Header.Get("Authorization")
		expected := fmt.Sprintf("Bearer %s", r.cfg.ProxyAPIKey)
		if authHeader != expected {
			http.Error(w, `{"error":{"message":"Unauthorized"}}`, http.StatusUnauthorized)
			return
		}
	}

	rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
	r.mux.ServeHTTP(rec, req)

	if rec.statusCode == http.StatusNotFound {
		logger.Warnf("[HTTP 404] Unhandled route path: %s %s", req.Method, req.URL.Path)
	}
}

func (r *Router) selectAdapter(modelName string) (adapter.ProviderAdapter, string, string) {
	modelName = strings.TrimSpace(modelName)

	// 1. Check for explicit "provider_instance/model_name" format
	if idx := strings.Index(modelName, "/"); idx != -1 {
		providerPrefix := strings.ToLower(modelName[:idx])
		actualModel := modelName[idx+1:]
		if targetAdapter, ok := r.adapters[providerPrefix]; ok {
			return targetAdapter, actualModel, providerPrefix
		}
	}

	// 2. Check if model is listed under a specific provider
	lowerModel := strings.ToLower(modelName)
	for pName, pConfig := range r.cfg.Providers {
		for _, m := range pConfig.Models {
			if strings.ToLower(m.Name) == lowerModel {
				return r.adapters[pName], m.Name, pName
			}
		}
	}

	// Also check default fallback matching for Anthropic
	if strings.HasPrefix(lowerModel, "claude") {
		if antAdapter, ok := r.adapters["anthropic"]; ok {
			return antAdapter, modelName, "anthropic"
		}
	}

	// 3. Default fallback to primary "openai" instance
	if openAIAdapter, ok := r.adapters["openai"]; ok {
		return openAIAdapter, modelName, "openai"
	}

	// Fallback to first registered adapter if "openai" is not configured
	for pName, a := range r.adapters {
		return a, modelName, pName
	}

	return nil, modelName, ""
}

// ExecuteInternal implements selector.ClassificationExecutor for non-recursive LLM classifier queries.
func (r *Router) ExecuteInternal(ctx context.Context, modelName string, prompt string, temperature *float64, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 300
	}
	req := &model.ChatCompletionRequest{
		Model: modelName,
		Messages: []model.ChatMessage{
			{Role: "user", Content: prompt},
		},
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	resp, err := r.executeDirect(ctx, req)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		logger.Debugf("[ExecuteInternal] Classifier choice: finish_reason=%q, content=%q", choice.FinishReason, choice.Message.Content)
		return choice.Message.Content, nil
	}
	return "", fmt.Errorf("empty choice returned by classifier")
}

func (r *Router) executeDirect(ctx context.Context, req *model.ChatCompletionRequest) (*model.ChatCompletionResponse, error) {
	provAdapter, actualModel, providerName := r.selectAdapter(req.Model)
	if provAdapter == nil {
		return nil, fmt.Errorf("no matching provider adapter found for model '%s'", req.Model)
	}
	reqCopy := *req
	reqCopy.Model = actualModel

	pConfig, hasConfig := r.cfg.Providers[providerName]

	maxRetries := 0
	maxTimeoutMS := 30000
	if hasConfig {
		maxRetries = pConfig.MaxRetries
		if pConfig.MaxTimeoutMS > 0 {
			maxTimeoutMS = pConfig.MaxTimeoutMS
		}
	}

	logger.Infof("[Proxy] Executing model '%s' via Provider Instance [%s]", actualModel, providerName)

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with randomized jitter
			backoff := time.Duration(1<<attempt*100) * time.Millisecond
			jitter := time.Duration(rand.Intn(50)) * time.Millisecond
			delay := backoff + jitter
			maxCap := time.Duration(maxTimeoutMS) * time.Millisecond
			if maxCap > 0 && delay > maxCap {
				delay = maxCap
			}

			logger.Warnf("Retry attempt %d/%d for model '%s' via [%s] after %v delay...", attempt, maxRetries, actualModel, providerName, delay)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		outBody, targetURL, headers, err := provAdapter.BuildRequest(&reqCopy)
		if err != nil {
			return nil, fmt.Errorf("failed to build outbound request: %w", err)
		}

		logger.Debugf("[Execution] Outbound HTTP POST to '%s' (Provider: %s, Payload size: %d bytes)...", targetURL, providerName, len(outBody))

		outReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewBuffer(outBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create outbound HTTP request: %w", err)
		}

		for k, v := range headers {
			outReq.Header.Set(k, v)
		}

		resp, err := r.httpClient.Do(outReq)
		if err != nil {
			logger.Errorf("[Execution] Provider [%s] unreachable: %v", providerName, err)
			lastErr = fmt.Errorf("upstream provider unreachable: %w", err)
			continue
		}

		if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			logger.Errorf("[Execution] Provider [%s] returned HTTP status %d: %s", providerName, resp.StatusCode, string(bodyBytes))
			lastErr = fmt.Errorf("upstream provider returned status %d: %s", resp.StatusCode, string(bodyBytes))
			if resp.StatusCode >= 500 {
				continue
			}
			return nil, lastErr
		}

		logger.Debugf("[Execution] Provider [%s] returned HTTP status 200 OK", providerName)
		return provAdapter.TransformResponse(resp)
	}

	return nil, lastErr
}

// --- Engine Interface Implementation ---

func (r *Router) Execute(ctx context.Context, req *model.ChatCompletionRequest) (*model.ChatCompletionResponse, error) {
	if r.selector != nil && r.selector.ShouldSelect(req.Model) {
		selectedModel, reason := r.selector.SelectModel(ctx, req)
		logger.Debugf("[Proxy] Dynamic selection routed '%s' -> '%s' (Reason: %s)", req.Model, selectedModel, reason)
		req.Model = selectedModel
	}
	return r.executeDirect(ctx, req)
}

func (r *Router) ExecuteStream(ctx context.Context, req *model.ChatCompletionRequest, w http.ResponseWriter) error {
	if r.selector != nil && r.selector.ShouldSelect(req.Model) {
		selectedModel, reason := r.selector.SelectModel(ctx, req)
		logger.Debugf("[Proxy] Dynamic stream selection routed '%s' -> '%s' (Reason: %s)", req.Model, selectedModel, reason)
		req.Model = selectedModel
	}

	provAdapter, actualModel, providerName := r.selectAdapter(req.Model)
	if provAdapter == nil {
		return fmt.Errorf("no matching provider adapter found for model '%s'", req.Model)
	}
	req.Model = actualModel

	logger.Infof("[Proxy] Streaming model '%s' via Provider Instance [%s]", actualModel, providerName)

	outBody, targetURL, headers, err := provAdapter.BuildRequest(req)
	if err != nil {
		return fmt.Errorf("failed to build outbound stream request: %w", err)
	}

	logger.Debugf("[Execution Stream] Outbound HTTP POST to '%s' (Provider: %s, Payload size: %d bytes)...", targetURL, providerName, len(outBody))

	outReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewBuffer(outBody))
	if err != nil {
		return fmt.Errorf("failed to create outbound stream HTTP request: %w", err)
	}

	for k, v := range headers {
		outReq.Header.Set(k, v)
	}

	resp, err := r.httpClient.Do(outReq)
	if err != nil {
		logger.Errorf("[Execution Stream] Provider [%s] unreachable: %v", providerName, err)
		return fmt.Errorf("upstream provider unreachable: %w", err)
	}

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		logger.Errorf("[Execution Stream] Provider [%s] returned HTTP status %d: %s", providerName, resp.StatusCode, string(bodyBytes))
		return fmt.Errorf("upstream provider returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	logger.Debugf("[Execution Stream] Provider [%s] returned HTTP status 200 OK, handling stream...", providerName)
	return provAdapter.HandleStream(ctx, resp, w)
}

func (r *Router) ListModels() []map[string]interface{} {
	var models []map[string]interface{}
	for _, alias := range r.cfg.DynamicRouting.AutoAliases {
		models = append(models, map[string]interface{}{
			"id":          alias,
			"object":      "model",
			"owned_by":    "frugal-llm-auto",
			"description": "Auto-selection dynamic router alias",
		})
	}
	for pName, pConfig := range r.cfg.Providers {
		if len(pConfig.Models) > 0 {
			for _, m := range pConfig.Models {
				models = append(models, map[string]interface{}{
					"id":          fmt.Sprintf("%s/%s", pName, m.Name),
					"object":      "model",
					"owned_by":    pName,
					"description": m.Description,
				})
			}
		} else {
			models = append(models, map[string]interface{}{
				"id":       fmt.Sprintf("%s/*", pName),
				"object":   "model",
				"owned_by": pName,
			})
		}
	}
	return models
}
