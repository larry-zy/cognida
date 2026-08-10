// Package sse provides Server-Sent Events utility functions for HTTP handlers
package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"cognida/internal/pkg/safego"
)

// ========================================
// Event Type Constants (Legacy - use events.go for new code)
// ========================================

const (
	EventTypeDone     = "done"
	EventTypeMetadata = "metadata"
)

// ========================================
// Chunk Structures
// ========================================

// ContentChunk represents a streaming content chunk
type ContentChunk struct {
	Content string `json:"content"`
	Done    bool   `json:"done"`
}

// MetadataChunk represents a metadata event
type MetadataChunk struct {
	Metadata interface{} `json:"metadata"`
}

// ========================================
// SSE Helper Functions
// ========================================

// SetSSEHeaders sets the proper headers for Server-Sent Events responses
func SetSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
}

// SendSSE sends a Server-Sent Event to the client
func SendSSE(w http.ResponseWriter, eventType string, data interface{}) {
	fmt.Fprintf(w, "event: %s\n", eventType)
	jsonData, err := json.Marshal(data)
	if err != nil {
		// If marshaling fails, send an error event
		fmt.Fprintf(w, "event: error\ndata: {\"error\": \"failed to marshal data\"}\n\n")
	} else {
		fmt.Fprintf(w, "data: %s\n\n", jsonData)
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// SendError sends an error event via SSE
func SendError(w http.ResponseWriter, err error) {
	SendSSE(w, EventTypeError, map[string]interface{}{
		"error": err.Error(),
	})
}

// SendDone sends a completion event via SSE
func SendDone(w http.ResponseWriter) {
	SendSSE(w, EventTypeDone, ContentChunk{Done: true})
}

// ========================================
// Heartbeat Mechanism
// ========================================

// HeartbeatConfig 心跳配置
type HeartbeatConfig struct {
	Interval time.Duration // 心跳间隔
	Comment  string        // 心跳注释内容
}

// DefaultHeartbeatConfig 默认心跳配置（30秒）
var DefaultHeartbeatConfig = &HeartbeatConfig{
	Interval: 30 * time.Second,
	Comment:  "keepalive",
}

// StartHeartbeat 启动心跳机制
// 返回一个 stop 函数，调用可停止心跳
func StartHeartbeat(ctx context.Context, w http.ResponseWriter, config *HeartbeatConfig) func() {
	if config == nil {
		config = DefaultHeartbeatConfig
	}

	ctx, cancel := context.WithCancel(ctx)
	ticker := time.NewTicker(config.Interval)

	go func() {
		defer safego.Recover("sse-forward")
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 发送 SSE 心跳（注释行，不会触发客户端事件）
				fmt.Fprintf(w, ": %s\n\n", config.Comment)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
	}()

	// 返回停止函数
	return func() {
		cancel()
	}
}
