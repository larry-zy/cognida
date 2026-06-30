package chat

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ========================================
// SSE Response Writer
// ========================================

// SSEResponseWriter SSE响应写入器
type SSEResponseWriter struct {
	writer  io.Writer
	flusher http.Flusher
	closed  bool
}

// NewSSEResponseWriter 创建SSE响应写入器
func NewSSEResponseWriter(c *gin.Context) *SSEResponseWriter {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		// 如果不支持flushing，返回一个no-op flusher
		flusher = &nopFlusher{}
	}

	// 设置SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no") // 禁用Nginx缓冲

	return &SSEResponseWriter{
		writer:  c.Writer,
		flusher: flusher,
		closed:  false,
	}
}

// WriteEvent 写入SSE事件
func (w *SSEResponseWriter) WriteEvent(event string, data interface{}) error {
	if w.closed {
		return fmt.Errorf("writer is closed")
	}

	// 序列化数据
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// 构造SSE格式: "event: {event}\ndata: {data}\n\n"
	format := "event: %s\ndata: %s\n\n"
	if _, err := fmt.Fprintf(w.writer, format, event, string(jsonData)); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	// 立即刷新
	w.flusher.Flush()
	return nil
}

// WriteData 写入SSE数据（无事件类型）
func (w *SSEResponseWriter) WriteData(data interface{}) error {
	if w.closed {
		return fmt.Errorf("writer is closed")
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	if _, err := fmt.Fprintf(w.writer, "data: %s\n\n", string(jsonData)); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	w.flusher.Flush()
	return nil
}

// WriteComment 写入SSE注释
func (w *SSEResponseWriter) WriteComment(comment string) error {
	if w.closed {
		return fmt.Errorf("writer is closed")
	}

	if _, err := fmt.Fprintf(w.writer, ": %s\n\n", comment); err != nil {
		return fmt.Errorf("failed to write comment: %w", err)
	}

	w.flusher.Flush()
	return nil
}

// Close 关闭writer
func (w *SSEResponseWriter) Close() error {
	w.closed = true
	return nil
}

// ========================================
// SSE Event Parser
// ========================================

// SSEParser SSE解析器
type SSEParser struct {
	scanner *bufio.Scanner
}

// NewSSEParser 创建SSE解析器
func NewSSEParser(reader io.Reader) *SSEParser {
	return &SSEParser{
		scanner: bufio.NewScanner(reader),
	}
}

// Parse 解析SSE流
func (p *SSEParser) Parse(ctx context.Context) (<-chan SSEEvent, <-chan error) {
	eventChan := make(chan SSEEvent, 10)
	errChan := make(chan error, 1)

	go func() {
		defer close(eventChan)
		defer close(errChan)

		var currentEvent SSEEvent
		currentEvent.Event = "message" // 默认事件类型

		for p.scanner.Scan() {
			line := p.scanner.Text()

			// 检查上下文是否已取消
			select {
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			default:
			}

			// 空行表示一个事件结束
			if line == "" {
				if currentEvent.Data != "" {
					eventChan <- currentEvent
					currentEvent = SSEEvent{Event: "message"}
				}
				continue
			}

			// 解析行
			if strings.HasPrefix(line, "event:") {
				currentEvent.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			} else if strings.HasPrefix(line, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if currentEvent.Data != "" {
					currentEvent.Data += "\n" + data
				} else {
					currentEvent.Data = data
				}
			} else if strings.HasPrefix(line, "id:") {
				// 可以处理id字段
			} else if strings.HasPrefix(line, "retry:") {
				// 可以处理retry字段
			} else if strings.HasPrefix(line, ":") {
				// 注释行，忽略
			}
		}

		if err := p.scanner.Err(); err != nil {
			errChan <- fmt.Errorf("scanner error: %w", err)
		}
	}()

	return eventChan, errChan
}

// ========================================
// Stream Response Handler
// ========================================

// HandleStreamResponse 处理流式响应
func HandleStreamResponse(ctx context.Context, c *gin.Context, respChan <-chan StreamResponse) error {
	sseWriter := NewSSEResponseWriter(c)
	defer sseWriter.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case resp, ok := <-respChan:
			if !ok {
				// 通道已关闭
				return nil
			}

			if err := sseWriter.WriteEvent(resp.Event, resp); err != nil {
				return fmt.Errorf("failed to write sse event: %w", err)
			}

			// 如果是结束或错误事件，完成发送
			if resp.Event == EventEnd || resp.Event == EventError {
				return nil
			}
		}
	}
}

// ========================================
// Helper Types
// ========================================

// nopFlusher no-op flusher for when http.Flusher is not available
type nopFlusher struct{}

func (f *nopFlusher) Flush() {}

// ========================================
// Server-Sent Events Helpers
// ========================================

// SendSSEMessage 发送SSE消息（辅助函数）
func SendSSEMessage(c *gin.Context, event string, data interface{}) error {
	writer := NewSSEResponseWriter(c)
	defer writer.Close()

	return writer.WriteEvent(event, data)
}

// StreamToSSE 将流式响应转换为SSE（辅助函数）
func StreamToSSE(ctx context.Context, c *gin.Context, chat Chat, messages []Message, opts *ChatOptions) error {
	respChan, err := chat.ChatStream(ctx, messages, opts)
	if err != nil {
		return fmt.Errorf("failed to start stream: %w", err)
	}

	return HandleStreamResponse(ctx, c, respChan)
}
