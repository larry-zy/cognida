package agent_test

import (
	"testing"
	"time"

	"link/internal/model/agent"
)

// TestNewCollaborationContext 测试创建协作上下文
func TestNewCollaborationContext(t *testing.T) {
	sessionID := "test-session"
	tenantID := int64(1)
	originalQuery := "分析最近一周的销售数据"

	ctx := agent.NewCollaborationContext(sessionID, tenantID, originalQuery)

	if ctx.SessionID != sessionID {
		t.Errorf("expected SessionID %s, got %s", sessionID, ctx.SessionID)
	}
	if ctx.TenantID != tenantID {
		t.Errorf("expected TenantID %d, got %d", tenantID, ctx.TenantID)
	}
	if ctx.OriginalQuery != originalQuery {
		t.Errorf("expected OriginalQuery %s, got %s", originalQuery, ctx.OriginalQuery)
	}
	if ctx.Mode != agent.ContextModeSummary {
		t.Errorf("expected default Mode %s, got %s", agent.ContextModeSummary, ctx.Mode)
	}
	if ctx.ContextLimit != 5 {
		t.Errorf("expected default ContextLimit 5, got %d", ctx.ContextLimit)
	}
	if len(ctx.DelegateChain) != 0 {
		t.Errorf("expected empty DelegateChain, got %d elements", len(ctx.DelegateChain))
	}
	if len(ctx.SharedResults) != 0 {
		t.Errorf("expected empty SharedResults, got %d elements", len(ctx.SharedResults))
	}
}

// TestCollaborationContext_AddDelegate 测试添加委派
func TestCollaborationContext_AddDelegate(t *testing.T) {
	ctx := agent.NewCollaborationContext("session-1", 1, "test query")

	// 添加第一个委派
	ctx.AddDelegate("analyst")
	if len(ctx.DelegateChain) != 1 {
		t.Errorf("expected DelegateChain length 1, got %d", len(ctx.DelegateChain))
	}
	if ctx.DelegateChain[0] != "analyst" {
		t.Errorf("expected delegate 'analyst', got %s", ctx.DelegateChain[0])
	}

	// 添加第二个委派
	ctx.AddDelegate("query_agent")
	if len(ctx.DelegateChain) != 2 {
		t.Errorf("expected DelegateChain length 2, got %d", len(ctx.DelegateChain))
	}
	if ctx.DelegateChain[1] != "query_agent" {
		t.Errorf("expected delegate 'query_agent', got %s", ctx.DelegateChain[1])
	}
}

// TestCollaborationContext_IsCyclic 测试循环检测
func TestCollaborationContext_IsCyclic(t *testing.T) {
	tests := []struct {
		name         string
		delegateName string
		chain        []string
		wantCyclic   bool
	}{
		{
			name:         "空链路不循环",
			delegateName: "analyst",
			chain:        []string{},
			wantCyclic:   false,
		},
		{
			name:         "单一委派不循环",
			delegateName: "analyst",
			chain:        []string{"researcher"},
			wantCyclic:   false,
		},
		{
			name:         "检测到循环",
			delegateName: "analyst",
			chain:        []string{"researcher", "analyst"},
			wantCyclic:   true,
		},
		{
			name:         "检测到自循环",
			delegateName: "analyst",
			chain:        []string{"analyst"},
			wantCyclic:   true,
		},
		{
			name:         "复杂链路循环",
			delegateName: "writer",
			chain:        []string{"analyst", "researcher", "writer"},
			wantCyclic:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := agent.NewCollaborationContext("session-1", 1, "test")
			ctx.DelegateChain = tt.chain

			got := ctx.IsCyclic(tt.delegateName)
			if got != tt.wantCyclic {
				t.Errorf("IsCyclic(%q) = %v, want %v", tt.delegateName, got, tt.wantCyclic)
			}
		})
	}
}

// TestCollaborationContext_GetDepth 测试获取深度
func TestCollaborationContext_GetDepth(t *testing.T) {
	tests := []struct {
		name     string
		chain    []string
		wantDepth int
	}{
		{
			name:     "空链路深度为0",
			chain:    []string{},
			wantDepth: 0,
		},
		{
			name:     "单个委派深度为1",
			chain:    []string{"analyst"},
			wantDepth: 1,
		},
		{
			name:     "三个委派深度为3",
			chain:    []string{"analyst", "researcher", "writer"},
			wantDepth: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := agent.NewCollaborationContext("session-1", 1, "test")
			ctx.DelegateChain = tt.chain

			got := ctx.GetDepth()
			if got != tt.wantDepth {
				t.Errorf("GetDepth() = %d, want %d", got, tt.wantDepth)
			}
		})
	}
}

// TestCollaborationContext_IsDepthExceeded 测试深度限制检查
func TestCollaborationContext_IsDepthExceeded(t *testing.T) {
	tests := []struct {
		name         string
		chain        []string
		maxDepth     int
		wantExceeded bool
	}{
		{
			name:         "未超过深度限制",
			chain:        []string{"analyst", "researcher"},
			maxDepth:     5,
			wantExceeded: false,
		},
		{
			name:         "刚好达到深度限制",
			chain:        []string{"analyst", "researcher", "writer"},
			maxDepth:     3,
			wantExceeded: true, // 使用 >= 判断，达到限制即认为超过
		},
		{
			name:         "超过深度限制",
			chain:        []string{"analyst", "researcher", "writer", "reviewer"},
			maxDepth:     3,
			wantExceeded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := agent.NewCollaborationContext("session-1", 1, "test")
			ctx.DelegateChain = tt.chain
			ctx.MaxDepth = tt.maxDepth

			got := ctx.IsDepthExceeded()
			if got != tt.wantExceeded {
				t.Errorf("IsDepthExceeded() = %v, want %v", got, tt.wantExceeded)
			}
		})
	}
}

// TestCollaborationContext_ChainDescription 测试链路描述
func TestCollaborationContext_ChainDescription(t *testing.T) {
	tests := []struct {
		name        string
		chain       []string
		wantDescription string
	}{
		{
			name:        "空链路",
			chain:       []string{},
			wantDescription: "无",
		},
		{
			name:        "单个委派",
			chain:       []string{"analyst"},
			wantDescription: "analyst",
		},
		{
			name:        "多个委派",
			chain:       []string{"analyst", "researcher", "writer"},
			wantDescription: "analyst → researcher → writer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := agent.NewCollaborationContext("session-1", 1, "test")
			ctx.DelegateChain = tt.chain

			got := ctx.ChainDescription()
			if got != tt.wantDescription {
				t.Errorf("ChainDescription() = %q, want %q", got, tt.wantDescription)
			}
		})
	}
}

// TestCollaborationContext_StoreResult 测试存储结果
func TestCollaborationContext_StoreResult(t *testing.T) {
	ctx := agent.NewCollaborationContext("session-1", 1, "test query")

	// 存储成功结果
	result1 := &agent.TaskResult{
		AgentName: "analyst",
		Content:   "分析完成",
		Duration:  100 * time.Millisecond,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(100 * time.Millisecond),
	}
	ctx.StoreResult("analyst", result1)

	if len(ctx.SharedResults) != 1 {
		t.Errorf("expected 1 result, got %d", len(ctx.SharedResults))
	}

	stored := ctx.SharedResults["analyst"]
	if stored == nil {
		t.Fatal("expected result to be stored")
	}
	if stored.AgentName != "analyst" {
		t.Errorf("expected AgentName 'analyst', got %s", stored.AgentName)
	}
	if stored.Content != "分析完成" {
		t.Errorf("expected Content '分析完成', got %s", stored.Content)
	}

	// 覆盖结果
	result2 := &agent.TaskResult{
		AgentName: "analyst",
		Content:   "重新分析完成",
		Duration:  50 * time.Millisecond,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(50 * time.Millisecond),
	}
	ctx.StoreResult("analyst", result2)

	if len(ctx.SharedResults) != 1 {
		t.Errorf("expected still 1 result after overwrite, got %d", len(ctx.SharedResults))
	}

	stored = ctx.SharedResults["analyst"]
	if stored.Content != "重新分析完成" {
		t.Errorf("expected Content '重新分析完成' after overwrite, got %s", stored.Content)
	}
}

// TestCollaborationContext_Clone 测试克隆
func TestCollaborationContext_Clone(t *testing.T) {
	original := agent.NewCollaborationContext("session-1", 1, "test query")
	original.Mode = agent.ContextModeFull
	original.ContextLimit = 10
	original.MaxDepth = 20
	original.AddDelegate("analyst")
	original.AddDelegate("researcher")

	result := &agent.TaskResult{
		AgentName: "analyst",
		Content:   "完成",
	}
	original.StoreResult("analyst", result)

	// 克隆
	cloned := original.Clone()

	// 验证基本字段
	if cloned.SessionID != original.SessionID {
		t.Errorf("cloned SessionID mismatch")
	}
	if cloned.Mode != original.Mode {
		t.Errorf("cloned Mode mismatch")
	}
	if cloned.ContextLimit != original.ContextLimit {
		t.Errorf("cloned ContextLimit mismatch")
	}
	if cloned.MaxDepth != original.MaxDepth {
		t.Errorf("cloned MaxDepth mismatch")
	}

	// 验证委派链路被复制
	if len(cloned.DelegateChain) != len(original.DelegateChain) {
		t.Errorf("cloned DelegateChain length mismatch")
	}

	// 验证共享结果被复制
	if len(cloned.SharedResults) != len(original.SharedResults) {
		t.Errorf("cloned SharedResults length mismatch")
	}

	// 修改克隆对象不应影响原始对象
	cloned.AddDelegate("writer")
	if len(original.DelegateChain) != 2 {
		t.Errorf("modifying clone affected original DelegateChain")
	}
	if len(cloned.DelegateChain) != 3 {
		t.Errorf("clone modification didn't work")
	}
}

// TestCollaborationContext_UpdateSummary 测试更新摘要
func TestCollaborationContext_UpdateSummary(t *testing.T) {
	ctx := agent.NewCollaborationContext("session-1", 1, "test query")

	newSummary := "用户询问了销售数据，我进行了分析"
	ctx.UpdateSummary(newSummary)

	if ctx.Summary != newSummary {
		t.Errorf("expected Summary %q, got %q", newSummary, ctx.Summary)
	}

	// 再次更新
	anotherSummary := "用户询问了销售数据，我进行了分析，然后生成了报告"
	ctx.UpdateSummary(anotherSummary)

	if ctx.Summary != anotherSummary {
		t.Errorf("expected Summary %q after update, got %q", anotherSummary, ctx.Summary)
	}
}

// TestCollaborationContextMode_Validate 测试上下文模式验证
func TestCollaborationContextMode_Validate(t *testing.T) {
	validModes := []agent.CollaborationContextMode{
		agent.ContextModeNone,
		agent.ContextModeSummary,
		agent.ContextModeRecent,
		agent.ContextModeFull,
		agent.ContextModeIsolated,
	}

	for _, mode := range validModes {
		if mode == "" {
			t.Errorf("mode %q should not be empty", mode)
		}
	}

	// 测试模式字符串值
	modeTests := []struct {
		mode    agent.CollaborationContextMode
		wantStr string
	}{
		{agent.ContextModeNone, "none"},
		{agent.ContextModeSummary, "summary"},
		{agent.ContextModeRecent, "recent"},
		{agent.ContextModeFull, "full"},
		{agent.ContextModeIsolated, "isolated"},
	}

	for _, tt := range modeTests {
		if string(tt.mode) != tt.wantStr {
			t.Errorf("mode string = %q, want %q", string(tt.mode), tt.wantStr)
		}
	}
}
