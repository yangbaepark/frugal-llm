package protocol

import (
	"context"
	"net/http"

	"frugal-llm/internal/model"
)

// Engine defines the core processing engine that executes requests against upstream providers.
type Engine interface {
	Execute(ctx context.Context, req *model.ChatCompletionRequest) (*model.ChatCompletionResponse, error)
	ExecuteStream(ctx context.Context, req *model.ChatCompletionRequest, w http.ResponseWriter) error
	ListModels() []map[string]interface{}
}

// ProtocolHandler defines the interface for accepting incoming client requests in various API formats.
type ProtocolHandler interface {
	Name() string
	RegisterRoutes(mux *http.ServeMux, engine Engine)
}
