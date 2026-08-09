package chat

import (
	"context"
	"fmt"
	"io"
	"cognida/internal/model/common"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// Chat 定义了聊天接口
type Chat interface {
	// Chat 进行非流式聊天
	Chat(ctx context.Context, messages []Message, opts *ChatOptions) (*ChatResponse, error)

	// ChatStream 进行流式聊天
	ChatStream(ctx context.Context, messages []Message, opts *ChatOptions) (<-chan StreamResponse, error)

	// GetModelName 获取模型名称
	GetModelName() string

	// GetModelID 获取模型ID
	GetModelID() string
}

// NewChat 创建聊天实例
func NewChat(config *ChatConfig) (Chat, error) {
	switch strings.ToLower(string(config.Source)) {
	case string(common.ModelSourceLocal):
		return NewOllamaChat(config)
	case string(common.ModelSourceRemote):
		return NewRemoteChat(config)
	default:
		return nil, fmt.Errorf("unsupported chat model source: %s", config.Source)
	}
}

// ========================================
// Base Chat Implementation
// ========================================

// baseChat 基础聊天实现
type baseChat struct {
	config  *ChatConfig
	modelID string
}

// GetModelName 获取模型名称
func (c *baseChat) GetModelName() string {
	return c.config.ModelName
}

// GetModelID 获取模型ID
func (c *baseChat) GetModelID() string {
	return c.modelID
}

// convertMessages 转换消息格式
func (c *baseChat) convertMessages(messages []Message) []*schema.Message {
	einoMessages := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		einoMsg := &schema.Message{
			Role:    schema.RoleType(msg.Role),
			Content: msg.Content,
		}
		einoMessages = append(einoMessages, einoMsg)
	}
	return einoMessages
}

// convertOptions 转换选项
func (c *baseChat) convertOptions(opts *ChatOptions) []model.Option {
	if opts == nil {
		return nil
	}

	modelOpts := []model.Option{}
	if opts.Temperature > 0 {
		modelOpts = append(modelOpts, model.WithTemperature(float32(opts.Temperature)))
	}
	if opts.TopP > 0 {
		modelOpts = append(modelOpts, model.WithTopP(float32(opts.TopP)))
	}
	if opts.MaxTokens > 0 {
		modelOpts = append(modelOpts, model.WithMaxTokens(opts.MaxTokens))
	}

	return modelOpts
}

// convertResponse 转换响应
func (c *baseChat) convertResponse(resp *schema.Message) *ChatResponse {
	var toolCalls []ToolCall
	if len(resp.ToolCalls) > 0 {
		toolCalls = make([]ToolCall, len(resp.ToolCalls))
		for i, tc := range resp.ToolCalls {
			toolCalls[i] = ToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}

	return &ChatResponse{
		MessageID: generateMessageID(),
		Content:   resp.Content,
		Role:      string(resp.Role),
		ToolCalls: toolCalls,
	}
}

// generateMessageID 生成消息ID
func generateMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}

// ========================================
// Helper Functions
// ========================================

// recvStream 接收流式响应
func recvStream(ctx context.Context, reader *schema.StreamReader[*schema.Message]) (<-chan StreamResponse, error) {
	respChan := make(chan StreamResponse, 10)

	go func() {
		defer close(respChan)
		defer func() {
			if reader != nil {
				reader.Close()
			}
		}()

		for {
			msg, err := reader.Recv()
			if err != nil {
				if err == io.EOF {
					respChan <- StreamResponse{
						Event: EventEnd,
					}
					return
				}
				respChan <- StreamResponse{
					Event: EventError,
					Error: err.Error(),
				}
				return
			}

			respChan <- StreamResponse{
				Event:   EventContent,
				Content: msg.Content,
			}
		}
	}()

	return respChan, nil
}
