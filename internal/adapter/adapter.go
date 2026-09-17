package adapter

import (
	"context"
	"net/http"

	"frugal-llm/internal/model"
)

// ProviderAdapter defines the interface for mapping requests/responses to LLM providers.
type ProviderAdapter interface {
	Name() string
	// BuildRequest serializes and prepares the outbound request payload, target URL, and headers.
	BuildRequest(req *model.ChatCompletionRequest) (body []byte, targetURL string, headers map[string]string, err error)
	// TransformResponse transforms a non-streaming HTTP response into OpenAI format.
	TransformResponse(resp *http.Response) (*model.ChatCompletionResponse, error)
	// HandleStream streams responses from the provider back to the client in SSE format.
	HandleStream(ctx context.Context, resp *http.Response, w http.ResponseWriter) error
}

// CreateAdapter instantiates a ProviderAdapter matching the given provider type if active.
// Returns (adapter, true) if active, or (nil, false) if inactive / missing required credentials.
func CreateAdapter(name, provType, baseURL, apiKey string) (ProviderAdapter, bool) {
	switch provType {
	case "anthropic":
		return NewAnthropicAdapter(baseURL, apiKey)
	case "ollama":
		return NewOllamaAdapter(baseURL)
	case "huggingface":
		return NewHuggingFaceAdapter(baseURL, apiKey)
	case "openai":
		fallthrough
	default:
		return NewOpenAIAdapter(name, baseURL, apiKey)
	}
}

// IsActive checks if a provider instance has all necessary settings to be activated.
func IsActive(name, provType, baseURL, apiKey string) bool {
	_, active := CreateAdapter(name, provType, baseURL, apiKey)
	return active
}
