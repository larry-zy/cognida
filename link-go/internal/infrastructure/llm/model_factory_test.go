package llm

import (
	"context"
	"testing"

	domainllm "link/internal/model/llm"
)

// fakeLLM 最小 LLMClient，用于校验工厂包装行为。
type fakeLLM struct{ tag string }

func (f *fakeLLM) Chat(context.Context, *domainllm.ChatRequest) (*domainllm.ChatResponse, error) {
	return &domainllm.ChatResponse{Content: f.tag}, nil
}
func (f *fakeLLM) ChatStream(context.Context, *domainllm.ChatRequest) (<-chan *domainllm.ChatChunk, error) {
	ch := make(chan *domainllm.ChatChunk)
	close(ch)
	return ch, nil
}
func (f *fakeLLM) GetModelInfo(context.Context) (*domainllm.ModelInfo, error) {
	return &domainllm.ModelInfo{Name: f.tag}, nil
}
func (f *fakeLLM) SupportsTools() bool     { return true }
func (f *fakeLLM) SupportsStreaming() bool { return true }

const fakeProvider = domainllm.Provider("fake")

func registerFake(f domainllm.ModelFactory, sentinel *fakeLLM) {
	f.RegisterProvider(fakeProvider, func(*domainllm.ModelConfig) (interface{}, error) {
		return sentinel, nil
	})
}

func TestFactory_WithoutResilience_ReturnsRaw(t *testing.T) {
	f := NewModelFactory(WithoutResilience())
	sentinel := &fakeLLM{tag: "raw"}
	registerFake(f, sentinel)

	got, err := f.CreateChatModel(&domainllm.ModelConfig{Provider: fakeProvider, ModelName: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if got != domainllm.LLMClient(sentinel) {
		t.Fatal("WithoutResilience should return the raw client unchanged")
	}
}

func TestFactory_DefaultWrapsResilient(t *testing.T) {
	f := NewModelFactory()
	sentinel := &fakeLLM{tag: "wrapped"}
	registerFake(f, sentinel)

	got, err := f.CreateChatModel(&domainllm.ModelConfig{Provider: fakeProvider, ModelName: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if got == domainllm.LLMClient(sentinel) {
		t.Fatal("default factory should wrap client with resilience decorator")
	}
	// 透明：健康调用照常返回底层内容
	resp, err := got.Chat(context.Background(), &domainllm.ChatRequest{})
	if err != nil || resp.Content != "wrapped" {
		t.Fatalf("transparent passthrough failed: resp=%v err=%v", resp, err)
	}
}

func TestFactory_CreateChatModelChain(t *testing.T) {
	f := NewModelFactory()
	chainFactory, ok := f.(interface {
		CreateChatModelChain([]*domainllm.ModelConfig) (domainllm.LLMClient, error)
	})
	if !ok {
		t.Fatal("factory should expose CreateChatModelChain")
	}
	f.RegisterProvider(fakeProvider, func(cfg *domainllm.ModelConfig) (interface{}, error) {
		return &fakeLLM{tag: cfg.ModelName}, nil
	})

	client, err := chainFactory.CreateChatModelChain([]*domainllm.ModelConfig{
		{Provider: fakeProvider, ModelName: "primary"},
		{Provider: fakeProvider, ModelName: "backup"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Chat(context.Background(), &domainllm.ChatRequest{})
	if err != nil || resp.Content != "primary" {
		t.Fatalf("chain primary passthrough failed: resp=%v err=%v", resp, err)
	}
}

func TestFactory_EmptyChainErrors(t *testing.T) {
	f := NewModelFactory().(interface {
		CreateChatModelChain([]*domainllm.ModelConfig) (domainllm.LLMClient, error)
	})
	if _, err := f.CreateChatModelChain(nil); err == nil {
		t.Fatal("empty chain must error")
	}
}
