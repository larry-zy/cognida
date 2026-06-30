package sse

import (
	"fmt"
	"net/http/httptest"
	"testing"
)

func TestSetSSEHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	SetSSEHeaders(w)

	if contentType := w.Header().Get("Content-Type"); contentType != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", contentType, "text/event-stream")
	}

	if cacheControl := w.Header().Get("Cache-Control"); cacheControl != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", cacheControl, "no-cache")
	}

	if connection := w.Header().Get("Connection"); connection != "keep-alive" {
		t.Errorf("Connection = %q, want %q", connection, "keep-alive")
	}

	if accelBuffering := w.Header().Get("X-Accel-Buffering"); accelBuffering != "no" {
		t.Errorf("X-Accel-Buffering = %q, want %q", accelBuffering, "no")
	}
}

func TestSendSSE(t *testing.T) {
	w := httptest.NewRecorder()
	SendSSE(w, EventTypeContent, ContentChunk{Content: "test", Done: false})

	body := w.Body.String()
	if body == "" {
		t.Error("Expected non-empty body")
	}

	// Check event type
	expectedEvent := "event: content\n"
	if body[:len(expectedEvent)] != expectedEvent {
		t.Errorf("Body prefix = %q, want %q", body[:len(expectedEvent)], expectedEvent)
	}
}

func TestSendSSE_WithComplexData(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]interface{}{
		"key":   "value",
		"count": 42,
	}
	SendSSE(w, EventTypeMetadata, data)

	body := w.Body.String()
	if body == "" {
		t.Error("Expected non-empty body")
	}

	// Should contain the data as JSON
	if body[:len("event: metadata\n")] != "event: metadata\n" {
		t.Errorf("Body should start with 'event: metadata\\n', got %q", body[:20])
	}
}

func TestSendError(t *testing.T) {
	w := httptest.NewRecorder()
	err := fmt.Errorf("test error")
	SendError(w, err)

	body := w.Body.String()
	if body == "" {
		t.Error("Expected non-empty body")
	}

	// Should be an error event
	if body[:len("event: error\n")] != "event: error\n" {
		t.Errorf("Body should start with 'event: error\\n', got %q", body[:20])
	}
}

func TestSendDone(t *testing.T) {
	w := httptest.NewRecorder()
	SendDone(w)

	body := w.Body.String()
	if body == "" {
		t.Error("Expected non-empty body")
	}

	// Should be a done event
	if body[:len("event: done\n")] != "event: done\n" {
		t.Errorf("Body should start with 'event: done\\n', got %q", body[:20])
	}
}

func TestEventTypes(t *testing.T) {
	if EventTypeContent != "content" {
		t.Errorf("EventTypeContent = %q, want %q", EventTypeContent, "content")
	}
	if EventTypeDone != "done" {
		t.Errorf("EventTypeDone = %q, want %q", EventTypeDone, "done")
	}
	if EventTypeError != "error" {
		t.Errorf("EventTypeError = %q, want %q", EventTypeError, "error")
	}
	if EventTypeMetadata != "metadata" {
		t.Errorf("EventTypeMetadata = %q, want %q", EventTypeMetadata, "metadata")
	}
}
