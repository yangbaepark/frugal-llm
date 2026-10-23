package adapter

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"frugal-llm/internal/model"
)

// DecodeEventStream decodes binary AWS EventStream frames from r and invokes onPayload for each frame's payload.
func DecodeEventStream(r io.Reader, onPayload func(payload []byte) error) error {
	prelude := make([]byte, 12)

	for {
		_, err := io.ReadFull(r, prelude)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}

		totalLen := binary.BigEndian.Uint32(prelude[0:4])
		headersLen := binary.BigEndian.Uint32(prelude[4:8])

		if totalLen < 16 || totalLen > 16*1024*1024 {
			return fmt.Errorf("invalid event stream frame total length: %d", totalLen)
		}
		if headersLen > totalLen-16 {
			return fmt.Errorf("invalid event stream frame headers length: %d", headersLen)
		}

		rest := make([]byte, totalLen-12)
		if _, err := io.ReadFull(r, rest); err != nil {
			return fmt.Errorf("failed reading event stream frame body: %w", err)
		}

		payloadLen := totalLen - 16 - headersLen
		if payloadLen > 0 {
			payload := rest[headersLen : headersLen+payloadLen]
			if err := onPayload(payload); err != nil {
				return err
			}
		}
	}
}

// BedrockConverseStreamPayload models the common JSON structures in AWS Bedrock converse-stream events.
type BedrockConverseStreamPayload struct {
	ContentBlockDelta *struct {
		Delta struct {
			Text string `json:"text"`
		} `json:"delta"`
		ContentBlockIndex int `json:"contentBlockIndex"`
	} `json:"contentBlockDelta,omitempty"`
	MessageStop *struct {
		StopReason string `json:"stopReason"`
	} `json:"messageStop,omitempty"`
	Metadata *struct {
		Usage struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
			TotalTokens  int `json:"totalTokens"`
		} `json:"usage"`
	} `json:"metadata,omitempty"`
	Message string `json:"message,omitempty"` // Bedrock error messages
}

// StreamBedrockEventStreamToSSE parses AWS Bedrock converse-stream events and pipes them to the client as OpenAI SSE.
func StreamBedrockEventStreamToSSE(respBody io.Reader, modelName string, w http.ResponseWriter) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("response writer does not support flushing")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	chunkID := fmt.Sprintf("chatcmpl-bedrock-%d", time.Now().UnixNano())
	createdTime := time.Now().Unix()

	err := DecodeEventStream(respBody, func(payload []byte) error {
		var event BedrockConverseStreamPayload
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil // Skip unknown non-JSON or internal heartbeat frames
		}

		if event.Message != "" {
			return fmt.Errorf("bedrock stream error: %s", event.Message)
		}

		if event.ContentBlockDelta != nil && event.ContentBlockDelta.Delta.Text != "" {
			chunk := model.ChatCompletionChunk{
				ID:      chunkID,
				Object:  "chat.completion.chunk",
				Created: createdTime,
				Model:   modelName,
				Choices: []model.StreamChoice{
					{
						Index: 0,
						Delta: model.DeltaMessage{
							Content: event.ContentBlockDelta.Delta.Text,
						},
					},
				},
			}
			chunkBytes, err := json.Marshal(chunk)
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
				flusher.Flush()
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
	return nil
}
