package rag

import (
	"context"
	"fmt"

	domainrag "cognida/internal/model/rag"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// EinoLLMAdapter lets bounded retrieval workflows share the configured model.
// Missing models are errors; retrieval must never synthesize fallback evidence.
type EinoLLMAdapter struct{ Model model.BaseChatModel }

func (a EinoLLMAdapter) Chat(ctx context.Context, messages []domainrag.LLMMessage, opts *domainrag.ChatOptions) (*domainrag.LLMResponse, error) {
	if a.Model == nil {
		return nil, fmt.Errorf("retrieval model is not configured")
	}
	input := make([]*schema.Message, 0, len(messages))
	for _, m := range messages {
		input = append(input, &schema.Message{Role: schema.RoleType(m.Role), Content: m.Content})
	}
	var options []model.Option
	if opts != nil && opts.MaxTokens > 0 {
		options = append(options, model.WithMaxTokens(opts.MaxTokens))
	}
	resp, err := a.Model.Generate(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Content == "" {
		return nil, fmt.Errorf("retrieval model returned empty output")
	}
	return &domainrag.LLMResponse{Content: resp.Content}, nil
}

func (a EinoLLMAdapter) ChatStream(ctx context.Context, messages []domainrag.LLMMessage, opts *domainrag.ChatOptions) (<-chan *domainrag.LLMStreamEvent, error) {
	resp, err := a.Chat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}
	ch := make(chan *domainrag.LLMStreamEvent, 1)
	ch <- &domainrag.LLMStreamEvent{Content: resp.Content, Done: true}
	close(ch)
	return ch, nil
}
