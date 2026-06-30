// Package sse provides standardized SSE event types and utilities
package sse

import (
	"encoding/json"
)

// ========================================
// Standardized Event Type Constants
// ========================================

const (
	EventTypeStart          = "start"           // Start event - beginning of response
	EventTypeContent        = "content"         // Content event - content fragment
	EventTypeTool           = "tool"            // Tool event - tool invocation
	EventTypeToolStart      = "tool_start"      // Tool start event - tool call started
	EventTypeToolResult     = "tool_result"     // Tool result event - tool call result
	EventTypeEnd            = "end"             // End event - response completed
	EventTypeError          = "error"           // Error event - error occurred
	EventTypeRetrieveStart  = "retrieve_start"  // RAG retrieve start event
	EventTypeDocument       = "document"        // RAG document event - retrieved document
	EventTypeRetrieveComplete = "retrieve_complete" // RAG retrieve complete event
)

// ========================================
// Event Data Structures
// ========================================

// BaseEvent represents the base SSE event structure
type BaseEvent struct {
	Event string                 `json:"event"`
	Data  map[string]interface{} `json:"data"`
}

// MarshalJSON implements custom JSON marshaling for SSE format
func (e *BaseEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.Data)
}

// ========================================
// Event Constructors
// ========================================

// NewStartEvent creates a new start event
func NewStartEvent(sessionID string) *BaseEvent {
	return &BaseEvent{
		Event: EventTypeStart,
		Data: map[string]interface{}{
			"session_id": sessionID,
		},
	}
}

// NewContentEvent creates a new content event
func NewContentEvent(content string) *BaseEvent {
	return &BaseEvent{
		Event: EventTypeContent,
		Data: map[string]interface{}{
			"content": content,
		},
	}
}

// NewToolEvent creates a new tool event (legacy, for compatibility)
func NewToolEvent(name, input string) *BaseEvent {
	return &BaseEvent{
		Event: EventTypeTool,
		Data: map[string]interface{}{
			"name":  name,
			"input": input,
		},
	}
}

// NewToolStartEvent creates a new tool start event
func NewToolStartEvent(name string, input map[string]interface{}) *BaseEvent {
	return &BaseEvent{
		Event: EventTypeToolStart,
		Data: map[string]interface{}{
			"name":  name,
			"input": input,
		},
	}
}

// NewToolResultEvent creates a new tool result event
func NewToolResultEvent(name string, output string, hasError bool) *BaseEvent {
	data := map[string]interface{}{
		"name":   name,
		"output": output,
	}
	if hasError {
		data["error"] = true
	}
	return &BaseEvent{
		Event: EventTypeToolResult,
		Data:  data,
	}
}

// NewEndEvent creates a new end event
func NewEndEvent(tokenCount int) *BaseEvent {
	data := map[string]interface{}{
		"done": true,
	}
	if tokenCount > 0 {
		data["token_count"] = tokenCount
	}
	return &BaseEvent{
		Event: EventTypeEnd,
		Data:  data,
	}
}

// NewErrorEvent creates a new error event
func NewErrorEvent(err error) *BaseEvent {
	return &BaseEvent{
		Event: EventTypeError,
		Data: map[string]interface{}{
			"error": err.Error(),
		},
	}
}

// ========================================
// RAG Event Constructors
// ========================================

// NewRetrieveStartEvent creates a new retrieve start event
func NewRetrieveStartEvent(query string) *BaseEvent {
	return &BaseEvent{
		Event: EventTypeRetrieveStart,
		Data: map[string]interface{}{
			"query": query,
		},
	}
}

// NewDocumentEvent creates a new document event
func NewDocumentEvent(docID, title, content string, score float64) *BaseEvent {
	return &BaseEvent{
		Event: EventTypeDocument,
		Data: map[string]interface{}{
			"doc_id":  docID,
			"title":   title,
			"content": content,
			"score":   score,
		},
	}
}

// NewRetrieveCompleteEvent creates a new retrieve complete event
func NewRetrieveCompleteEvent(count int) *BaseEvent {
	return &BaseEvent{
		Event: EventTypeRetrieveComplete,
		Data: map[string]interface{}{
			"count": count,
		},
	}
}
