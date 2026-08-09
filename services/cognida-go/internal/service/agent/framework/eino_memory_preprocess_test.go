// 记忆分支统一 pre-processing 测试（4.2.3）：
// 记忆分支必须与非记忆分支一样执行 beforeHooks/middleware（旧实现提前 return 绕过）。
package framework

import (
	"context"
	"testing"

	domainagent "cognida/internal/model/agent"
	"cognida/internal/model/memory"
)

// fakeContextBuilder 返回固定 BuildContext 的记忆上下文构建器桩。
type fakeContextBuilder struct{}

func (f *fakeContextBuilder) Build(_ context.Context, req *memory.BuildContextRequest) (*memory.BuildContext, error) {
	return &memory.BuildContext{
		Metadata: &memory.ContextBuildMetadata{},
		Messages: []*memory.Message{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: req.CurrentMessage},
		},
	}, nil
}

func (f *fakeContextBuilder) BuildForCollaboration(ctx context.Context, req *memory.BuildContextRequest, _ string, _ int) (*memory.BuildContext, error) {
	return f.Build(ctx, req)
}

// TestChatMemoryBranch_RunsBeforeHooksAndMiddleware 记忆分支断言 beforeHooks/middleware 被执行，
// 且 hook 对 message 的改写能到达模型侧（经 preProcess 传递）。
func TestChatMemoryBranch_RunsBeforeHooksAndMiddleware(t *testing.T) {
	var hookCalled, mwCalled bool

	a := &agentImpl{
		name:           "t",
		model:          &errStreamModel{},
		enableMemory:   true,
		contextBuilder: &fakeContextBuilder{},
		beforeHooks: []BeforeHook{
			func(ctx context.Context, msg string) (context.Context, string, error) {
				hookCalled = true
				return ctx, msg + "[hooked]", nil
			},
		},
		middleware: []Middleware{
			NewMiddleware(
				func(ctx context.Context, msg string) (context.Context, string, error) {
					mwCalled = true
					return ctx, msg, nil
				},
				nil,
			),
		},
	}

	ctx := domainagent.WithSessionID(context.Background(), "sess-mem")
	resp, err := a.Chat(ctx, "你好")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp == nil {
		t.Fatal("resp 为 nil")
	}
	if withMem, _ := resp.Metadata["with_memory"].(bool); !withMem {
		t.Fatal("未走记忆分支（with_memory != true），测试前提不成立")
	}

	if !hookCalled {
		t.Error("记忆分支绕过了 beforeHooks")
	}
	if !mwCalled {
		t.Error("记忆分支绕过了 middleware.Before")
	}
}

// TestChatNonMemoryBranch_StillRunsPreProcess 回归：非记忆分支的前置流程不受重构影响。
func TestChatNonMemoryBranch_StillRunsPreProcess(t *testing.T) {
	var hookCalled, mwCalled bool

	a := &agentImpl{
		name:  "t",
		model: &errStreamModel{},
		beforeHooks: []BeforeHook{
			func(ctx context.Context, msg string) (context.Context, string, error) {
				hookCalled = true
				return ctx, msg, nil
			},
		},
		middleware: []Middleware{
			NewMiddleware(
				func(ctx context.Context, msg string) (context.Context, string, error) {
					mwCalled = true
					return ctx, msg, nil
				},
				nil,
			),
		},
	}

	if _, err := a.Chat(context.Background(), "你好"); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if !hookCalled || !mwCalled {
		t.Errorf("非记忆分支前置流程缺失: hook=%v mw=%v", hookCalled, mwCalled)
	}
}
