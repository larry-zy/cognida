package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"link/internal/service/agent/skills"
)

// registerSkill 把一个 skill 注册进全局管理器（供 skill_invoke 命中）。
func registerSkill(t *testing.T, s *skills.Skill) {
	t.Helper()
	if err := skills.GetGlobalManager().GetRegistry().Register(s); err != nil {
		t.Fatalf("注册 skill %q 失败: %v", s.Name, err)
	}
	t.Cleanup(func() { _ = skills.GetGlobalManager().GetRegistry().Unregister(s.Name) })
}

func invokeSkill(t *testing.T, name string) (string, error) {
	t.Helper()
	tl, err := NewSkillInvokeTool()
	if err != nil {
		t.Fatalf("构造 skill_invoke 失败: %v", err)
	}
	args, _ := json.Marshal(map[string]string{"skill_name": name})
	return tl.InvokableRun(context.Background(), string(args))
}

// TestSkillInvoke_ExecutesHandler 校验：CanInvoke+Handler 的 skill 命中即执行 handler，
// 回传其紧凑输出（而非 markdown 指导）。
func TestSkillInvoke_ExecutesHandler(t *testing.T) {
	const marker = "RESULT_ID=r-123 结论：GMV 同比 +12%"
	var gotInput string
	registerSkill(t, &skills.Skill{
		Name:        "exec-attribution",
		Description: "可执行归因",
		CanInvoke:   true,
		Handler: func(_ context.Context, input string) (string, error) {
			gotInput = input
			return marker, nil
		},
	})

	out, err := invokeSkill(t, "exec-attribution")
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.Contains(out, marker) {
		t.Fatalf("应回传 handler 输出, got %s", out)
	}
	if !strings.Contains(out, `"executed": true`) {
		t.Fatalf("应标记 executed, got %s", out)
	}
	// handler 应收到本次调用参数（含 skill_name）。
	if !strings.Contains(gotInput, "exec-attribution") {
		t.Fatalf("handler 未收到入参: %q", gotInput)
	}
}

// TestSkillInvoke_PureGuidanceUnchanged 校验：无 handler 的 skill 维持既有 markdown 指导路径，
// 不含 executed 标记（向后兼容）。
func TestSkillInvoke_PureGuidanceUnchanged(t *testing.T) {
	registerSkill(t, &skills.Skill{
		Name:        "guide-only",
		Description: "纯指导",
		Content:     "# 指导\n按此方法论推进。",
	})

	out, err := invokeSkill(t, "guide-only")
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if strings.Contains(out, `"executed": true`) {
		t.Fatalf("纯指导不应走 handler 路径: %s", out)
	}
	if !strings.Contains(out, "content") {
		t.Fatalf("应回传指导内容信封: %s", out)
	}
}

// TestSkillInvoke_HandlerPanicRecovered 校验：handler panic 被兜底为普通错误，不外溢崩溃。
func TestSkillInvoke_HandlerPanicRecovered(t *testing.T) {
	registerSkill(t, &skills.Skill{
		Name:        "panic-skill",
		Description: "会 panic",
		CanInvoke:   true,
		Handler: func(_ context.Context, _ string) (string, error) {
			panic("boom")
		},
	})

	_, err := invokeSkill(t, "panic-skill")
	if err == nil {
		t.Fatal("handler panic 应转为错误返回")
	}
	if !strings.Contains(err.Error(), "panic-skill") {
		t.Fatalf("错误应含 skill 名: %v", err)
	}
}

// TestSkillInvoke_CanInvokeWithoutHandlerFallsBack 校验：CanInvoke=true 但 Handler=nil 时
// 安全退回 markdown（防御性，不 nil 调用）。
func TestSkillInvoke_CanInvokeWithoutHandlerFallsBack(t *testing.T) {
	registerSkill(t, &skills.Skill{
		Name:        "flag-no-handler",
		Description: "标了 CanInvoke 但无 handler",
		CanInvoke:   true,
		Content:     "# 内容",
	})

	out, err := invokeSkill(t, "flag-no-handler")
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if strings.Contains(out, `"executed": true`) {
		t.Fatalf("无 handler 不应执行: %s", out)
	}
}
