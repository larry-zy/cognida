// Package rag: ExecuteStream 流式事件测试——慢消费者无丢块、ctx 取消及时返回。
package rag

import (
	"context"
	"fmt"
	"testing"
	"time"

	domainrag "cognida/internal/model/rag"
)

// ========================================
// Fakes
// ========================================

// fakeRetriever 固定返回 docs 的检索器。
type fakeRetriever struct {
	docs []*domainrag.Document
}

func (f *fakeRetriever) retrieve() (*domainrag.RetrieveResponse, error) {
	return &domainrag.RetrieveResponse{Results: f.docs}, nil
}

func (f *fakeRetriever) Retrieve(_ context.Context, _, _, _ string, _ *domainrag.RetrieveOptions) (*domainrag.RetrieveResponse, error) {
	return f.retrieve()
}

func (f *fakeRetriever) RetrieveWithEmbedding(_ context.Context, _, _, _ string, _ []float32, _ *domainrag.RetrieveOptions) (*domainrag.RetrieveResponse, error) {
	return f.retrieve()
}

func (f *fakeRetriever) VectorRetrieve(_ context.Context, _, _, _ string, _ *domainrag.RetrieveOptions) (*domainrag.RetrieveResponse, error) {
	return f.retrieve()
}

func (f *fakeRetriever) BM25Retrieve(_ context.Context, _, _, _ string, _ *domainrag.RetrieveOptions) (*domainrag.RetrieveResponse, error) {
	return f.retrieve()
}

func (f *fakeRetriever) HybridRetrieve(_ context.Context, _, _, _ string, _ *domainrag.RetrieveOptions) (*domainrag.RetrieveResponse, error) {
	return f.retrieve()
}

func (f *fakeRetriever) GraphRetrieve(_ context.Context, _, _, _ string, _ *domainrag.RetrieveOptions) (*domainrag.RetrieveResponse, error) {
	return f.retrieve()
}

// fakeReranker 原样返回。
type fakeReranker struct{}

func (f *fakeReranker) Rerank(_ context.Context, results []*domainrag.Document, _ string) ([]*domainrag.Document, error) {
	return results, nil
}
func (f *fakeReranker) SetStrategy(_ string) error { return nil }
func (f *fakeReranker) GetStrategy() string        { return "noop" }

// fakeLLMChat ChatStream 产出 chunkCount 个块；respectCtx 时随 ctx 取消停止。
type fakeLLMChat struct {
	chunkCount int
}

func (f *fakeLLMChat) Chat(_ context.Context, _ []domainrag.LLMMessage, _ *domainrag.ChatOptions) (*domainrag.LLMResponse, error) {
	return &domainrag.LLMResponse{Content: "answer"}, nil
}

func (f *fakeLLMChat) ChatStream(ctx context.Context, _ []domainrag.LLMMessage, _ *domainrag.ChatOptions) (<-chan *domainrag.LLMStreamEvent, error) {
	ch := make(chan *domainrag.LLMStreamEvent)
	go func() {
		defer close(ch)
		for i := 0; i < f.chunkCount; i++ {
			select {
			case ch <- &domainrag.LLMStreamEvent{Content: fmt.Sprintf("chunk-%d", i)}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// fakeStrengthener 不增强（测试请求不开启增强，本实现不应被调用）。
type fakeStrengthener struct{}

func (f *fakeStrengthener) StrengthenQuery(_ context.Context, query string, _ string, _ *domainrag.StrengthOptions) (*domainrag.StrengthenedQuery, error) {
	return &domainrag.StrengthenedQuery{OriginalQuery: query}, nil
}
func (f *fakeStrengthener) RewriteQuery(_ context.Context, query string, _ string, _ *domainrag.StrengthOptions) (string, error) {
	return query, nil
}
func (f *fakeStrengthener) SplitQuery(_ context.Context, query string, _ string, _ *domainrag.StrengthOptions) ([]string, error) {
	return []string{query}, nil
}

func newTestPipeline(docCount, chunkCount int) domainrag.Pipeline {
	docs := make([]*domainrag.Document, docCount)
	for i := range docs {
		docs[i] = &domainrag.Document{Content: fmt.Sprintf("doc-%d", i)}
	}
	return NewPipeline(
		&fakeRetriever{docs: docs},
		&fakeReranker{},
		&fakeLLMChat{chunkCount: chunkCount},
		&fakeStrengthener{},
	)
}

// ========================================
// 3.3.2 慢消费者无丢块
// ========================================

// TestExecuteStream_SlowConsumerNoDroppedEvents 消费速度慢于生产（事件数远超
// channel 缓冲 10）时，旧实现的 select{default} 会静默丢块；新实现阻塞发送，
// 断言所有事件按序完整到达。
func TestExecuteStream_SlowConsumerNoDroppedEvents(t *testing.T) {
	const (
		docCount   = 5
		chunkCount = 30 // 远超缓冲 10
	)
	p := newTestPipeline(docCount, chunkCount)

	eventChan, err := p.ExecuteStream(context.Background(), &domainrag.PipelineRequest{
		TenantID:        "1",
		KnowledgeBaseID: "kb-1",
		Query:           "测试查询",
		Options:         &domainrag.PipelineConfig{},
	})
	if err != nil {
		t.Fatalf("ExecuteStream: %v", err)
	}

	var events []*domainrag.PipelineEvent
	for ev := range eventChan {
		time.Sleep(time.Millisecond) // 模拟慢消费者
		events = append(events, ev)
	}

	// query_strengthening + retrieve + retrieve_complete + docs + generate + chunks + done
	wantTotal := 3 + docCount + 1 + chunkCount + 1
	if len(events) != wantTotal {
		t.Fatalf("事件数 = %d, 期望 %d（丢块）", len(events), wantTotal)
	}

	// chunk 内容与顺序完整
	var chunks []string
	for _, ev := range events {
		if ev.Event == "error" {
			t.Fatalf("意外 error 事件: %v", ev.Error)
		}
		if ev.Event == "chunk" {
			chunks = append(chunks, ev.Content)
		}
	}
	if len(chunks) != chunkCount {
		t.Fatalf("chunk 数 = %d, 期望 %d", len(chunks), chunkCount)
	}
	for i, c := range chunks {
		if want := fmt.Sprintf("chunk-%d", i); c != want {
			t.Fatalf("chunk[%d] = %q, 期望 %q（乱序或丢块）", i, c, want)
		}
	}
	if events[len(events)-1].Event != "done" {
		t.Fatalf("最后事件 = %q, 期望 done", events[len(events)-1].Event)
	}
}

// ========================================
// 3.3.2 取消时及时返回
// ========================================

// TestExecuteStream_CancelReturnsPromptly 消费者中途取消 ctx 且不再读取，
// 生产者不得永久阻塞在 ch<-，应经 ctx.Done 分支退出并关闭 channel。
func TestExecuteStream_CancelReturnsPromptly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := newTestPipeline(3, 1000)

	eventChan, err := p.ExecuteStream(ctx, &domainrag.PipelineRequest{
		TenantID:        "1",
		KnowledgeBaseID: "kb-1",
		Query:           "测试查询",
		Options:         &domainrag.PipelineConfig{},
	})
	if err != nil {
		t.Fatalf("ExecuteStream: %v", err)
	}

	// 读一个事件后取消，且不再消费
	select {
	case <-eventChan:
	case <-time.After(2 * time.Second):
		t.Fatal("未收到首个事件")
	}
	cancel()

	// 生产者应及时退出并 close(eventChan)：带超时地把剩余缓冲读完直到关闭
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-eventChan:
			if !ok {
				return // channel 已关闭，生产者退出 ✓
			}
		case <-deadline:
			t.Fatal("取消后 2s 内 channel 未关闭（生产者未退出）")
		}
	}
}
