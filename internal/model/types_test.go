package model

import (
	"encoding/json"
	"testing"
)

func TestChatMessageUnmarshalStringContent(t *testing.T) {
	data := []byte(`{"role":"user","content":"Hello world"}`)
	var msg ChatMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("Failed to unmarshal string content: %v", err)
	}

	if msg.Role != "user" || msg.Content != "Hello world" {
		t.Errorf("Unexpected message: %+v", msg)
	}
}

func TestChatMessageUnmarshalArrayContent(t *testing.T) {
	data := []byte(`{
		"role":"user",
		"content":[
			{"type":"text","text":"Please fix this bug"},
			{"type":"text","text":"Here is additional context"}
		]
	}`)
	var msg ChatMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("Failed to unmarshal array content: %v", err)
	}

	expected := "Please fix this bug\nHere is additional context"
	if msg.Role != "user" || msg.Content != expected {
		t.Errorf("Expected content %q, got %q", expected, msg.Content)
	}
}
