package framework

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"

	agentreflection "cognida/internal/model/agent/reflection"
)

// TestWithReflectionFromToolModel_RegistersAfterHook 验证 data_agent 预设走的接线路径：
// 只持有 ToolCallingChatModel 时，WithReflectionFromToolModel 会用内部适配器装配反思 AfterHook，
// 且该 Hook 在回答后真正驱动 Critic→Refine，改写内容与反思元数据落到 Response。
func TestWithReflectionFromToolModel_RegistersAfterHook(t *testing.T) {
	// 脚本顺序：Critic 首评(低分+问题) → Actor 改写 → Critic 次评(高分收敛)。
	// Critic 与 Actor 共享同一底层模型，按序消费脚本。
	toolModel := &scriptedToolModel{script: []*schema.Message{
		{Role: schema.Assistant, Content: `{"overall_score":0.4,"dimensions":{},"issues":["过于简略"],"should_refine":true}`},
		{Role: schema.Assistant, Content: "REFINED"},
		{Role: schema.Assistant, Content: `{"overall_score":0.95,"dimensions":{},"issues":[],"should_refine":false}`},
	}}

	b := New(nil).WithToolModel(toolModel)
	b = b.WithReflectionFromToolModel(&agentreflection.ReflectionConfig{
		Enabled:       true,
		MaxIterations: 3,
		CriticType:    "llm",
	}, "agent-data-agent", nil)

	if len(b.afterHooks) != 1 {
		t.Fatalf("期望注册 1 个 afterHook，实际 %d", len(b.afterHooks))
	}

	resp := &Response{Content: "太简略的初始回答"}
	if err := b.afterHooks[0](context.Background(), resp); err != nil {
		t.Fatalf("afterHook 执行失败: %v", err)
	}

	if resp.Content != "REFINED" {
		t.Fatalf("期望改写内容落地为 REFINED，实际 %q", resp.Content)
	}
	meta, ok := resp.Metadata["reflection"].(map[string]interface{})
	if !ok {
		t.Fatalf("期望 Response.Metadata[reflection] 存在，实际 %#v", resp.Metadata)
	}
	if meta["success"] != true {
		t.Fatalf("期望反思成功，实际 metadata=%#v", meta)
	}
}

// TestWithReflectionFromToolModel_NoOpWhenDisabledOrNil 验证零回归护栏：
// 配置禁用、或 Builder 未持有 toolModel 时，不注册任何 AfterHook。
func TestWithReflectionFromToolModel_NoOpWhenDisabledOrNil(t *testing.T) {
	// 禁用配置 → 不注册。
	b := New(nil).WithToolModel(&scriptedToolModel{})
	b = b.WithReflectionFromToolModel(&agentreflection.ReflectionConfig{Enabled: false}, "id", nil)
	if len(b.afterHooks) != 0 {
		t.Fatalf("禁用配置不应注册 afterHook，实际 %d", len(b.afterHooks))
	}

	// 未设置 toolModel → 直接返回，不注册。
	b2 := New(nil)
	b2 = b2.WithReflectionFromToolModel(&agentreflection.ReflectionConfig{
		Enabled: true, MaxIterations: 3, CriticType: "llm",
	}, "id", nil)
	if len(b2.afterHooks) != 0 {
		t.Fatalf("无 toolModel 时不应注册 afterHook，实际 %d", len(b2.afterHooks))
	}
}
