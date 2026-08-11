// Package sse provides Server-Sent Events utility functions for HTTP handlers
package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
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

// writeSSE 无锁写出一条 SSE 事件到底层 ResponseWriter。
// 仅供已持有 Writer.mu 的方法内部调用——外部一律经 *Writer 的加锁方法，
// 以保证数据帧与心跳帧对同一 ResponseWriter 的写入被串行化〔R2-3〕。
func writeSSE(w http.ResponseWriter, eventType string, data interface{}) {
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

// ========================================
// Serialized SSE Writer
// ========================================

// Writer 串行化对同一 http.ResponseWriter 的所有 SSE 写入（数据帧 + 心跳帧）。
// 消除心跳 goroutine 与主发送流并发写同一 ResponseWriter 的数据竞争〔R2-3〕。
// 一个请求应只构造一个 Writer，并把它同时用于 StartHeartbeat 与所有发送。
type Writer struct {
	mu sync.Mutex
	w  http.ResponseWriter
}

// NewWriter 基于底层 ResponseWriter 构造串行化 SSE 写入器。
func NewWriter(w http.ResponseWriter) *Writer {
	return &Writer{w: w}
}

// Send 加锁写出一条 SSE 事件。
func (sw *Writer) Send(eventType string, data interface{}) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	writeSSE(sw.w, eventType, data)
}

// SendError 加锁写出错误事件。
func (sw *Writer) SendError(err error) {
	sw.Send(EventTypeError, map[string]interface{}{
		"error": err.Error(),
	})
}

// SendDone 加锁写出完成事件。
func (sw *Writer) SendDone() {
	sw.Send(EventTypeDone, ContentChunk{Done: true})
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

// StartHeartbeat 启动心跳机制，返回一个 stop 函数供调用方停止心跳。
// 心跳帧与数据帧共用 Writer 的同一把锁写出，二者互斥、不再竞争同一 ResponseWriter。
func (sw *Writer) StartHeartbeat(ctx context.Context, config *HeartbeatConfig) func() {
	if config == nil {
		config = DefaultHeartbeatConfig
	}

	ctx, cancel := context.WithCancel(ctx)
	ticker := time.NewTicker(config.Interval)

	go func() {
		defer safego.Recover("sse-heartbeat")
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 发送 SSE 心跳（注释行，不会触发客户端事件）；与数据帧共用锁。
				sw.mu.Lock()
				fmt.Fprintf(sw.w, ": %s\n\n", config.Comment)
				if f, ok := sw.w.(http.Flusher); ok {
					f.Flush()
				}
				sw.mu.Unlock()
			}
		}
	}()

	// 返回停止函数
	return func() {
		cancel()
	}
}
