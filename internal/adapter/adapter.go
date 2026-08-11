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
