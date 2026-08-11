package protocol

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"frugal-llm/internal/logger"
	"frugal-llm/internal/model"
)

type OpenAIProtocolHandler struct{}

func NewOpenAIProtocolHandler() *OpenAIProtocolHandler {
	return &OpenAIProtocolHandler{}
}

func (h *OpenAIProtocolHandler) Name() string {
	return "OpenAI-v1"
}

func (h *OpenAIProtocolHandler) RegisterRoutes(mux *http.ServeMux, engine Engine) {
	chatEndpoints := []string{
		"/v1/chat/completions",
		"/chat/completions",
		"/api/v0/chat/completions",
		"/api/v1/chat/completions",
	}
	for _, path := range chatEndpoints {
		mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
			if req.Method != http.MethodPost {
				http.Error(w, `{"error":{"message":"Method not allowed"}}`, http.StatusMethodNotAllowed)
				return
			}
			handleOpenAIChat(w, req, engine)
		})
	}

	modelEndpoints := []string{
		"/v1/models",
		"/models",
		"/api/v0/models",
		"/api/v1/models",
	}
	for _, path := range modelEndpoints {
		mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
			if req.Method != http.MethodGet {
				http.Error(w, `{"error":{"message":"Method not allowed"}}`, http.StatusMethodNotAllowed)
				return
			}
			models := engine.ListModels()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"object": "list",
				"data":   models,
			})
		})
	}
}

func handleOpenAIChat(w http.ResponseWriter, req *http.Request, engine Engine) {
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		logger.Errorf("[Protocol] Failed to read request body: %v", err)
		http.Error(w, `{"error":{"message":"Failed to read request body"}}`, http.StatusBadRequest)
		return
	}

	logger.Debugf("[Protocol] Received OpenAI chat completion payload (%d bytes)", len(bodyBytes))

	var chatReq model.ChatCompletionRequest
	if err := json.Unmarshal(bodyBytes, &chatReq); err != nil {
		logger.Errorf("[Protocol] Failed to unmarshal JSON payload: %v", err)
		http.Error(w, `{"error":{"message":"Invalid JSON payload"}}`, http.StatusBadRequest)
		return
	}

	logger.Debugf("[Protocol] Parsed chat request: Model='%s', MessagesCount=%d, Stream=%v", chatReq.Model, len(chatReq.Messages), chatReq.Stream)

	if chatReq.Stream {
		if err := engine.ExecuteStream(req.Context(), &chatReq, w); err != nil {
			logger.Errorf("[Protocol] ExecuteStream error: %v", err)
			http.Error(w, fmt.Sprintf(`{"error":{"message":"%s"}}`, err.Error()), http.StatusInternalServerError)
		}
		return
	}

	resp, err := engine.Execute(req.Context(), &chatReq)
	if err != nil {
		logger.Errorf("[Protocol] Execute error: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":{"message":"%s"}}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
