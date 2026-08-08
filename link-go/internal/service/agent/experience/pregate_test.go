package experience

import (
	"testing"
	"time"

	domain_conversation "link/internal/model/conversation"
)

// msg 便捷构造一条消息，createdAt 用序号 i 排开，保证时间正序可比。
func msg(role, content string, i int, completed bool) *domain_conversation.Message {
	return &domain_conversation.Message{
		Role:        role,
		Content:     content,
		IsCompleted: completed,
		CreatedAt:   time.Date(2026, 1, 1, 0, i, 0, 0, time.UTC),
	}
}

func TestPreGate_HealthySessionPasses(t *testing.T) {
	msgs := []*domain_conversation.Message{
		msg(domain_conversation.RoleUser, "统计上月各城市营收", 0, true),
		msg(domain_conversation.RoleAssistant, "已用 sql_execute 查询并出图，华东最高", 1, true),
	}
	if skip, reason := objectiveFailureGate(msgs); skip {
		t.Errorf("正常会话不应被前置门拦截, got skip=%v reason=%q", skip, reason)
	}
}

func TestPreGate_NoAssistantReply(t *testing.T) {
	msgs := []*domain_conversation.Message{
		msg(domain_conversation.RoleUser, "帮我查数据", 0, true),
	}
	if skip, _ := objectiveFailureGate(msgs); !skip {
		t.Error("无任何助手回复应被拦截")
	}
}

func TestPreGate_LastAssistantNotCompleted(t *testing.T) {
	msgs := []*domain_conversation.Message{
		msg(domain_conversation.RoleUser, "查数据", 0, true),
		msg(domain_conversation.RoleAssistant, "正在查询...", 1, false), // 未收尾
	}
	if skip, reason := objectiveFailureGate(msgs); !skip {
		t.Errorf("末轮助手未收尾应被拦截, got skip=%v reason=%q", skip, reason)
	}
}

func TestPreGate_EmptyAssistantNoOutput(t *testing.T) {
	msgs := []*domain_conversation.Message{
		msg(domain_conversation.RoleUser, "查数据", 0, true),
		msg(domain_conversation.RoleAssistant, "   ", 1, true), // 空正文、无工具/引用
	}
	if skip, _ := objectiveFailureGate(msgs); !skip {
		t.Error("末轮助手空正文且无产出应被拦截")
	}
}

func TestPreGate_AllToolCallsFailed(t *testing.T) {
	m := msg(domain_conversation.RoleAssistant, "抱歉，查询失败了", 1, true)
	m.ToolCalls = `[{"name":"sql_execute","error":"syntax error"},{"name":"get_schema","error":"timeout"}]`
	msgs := []*domain_conversation.Message{
		msg(domain_conversation.RoleUser, "查数据", 0, true),
		m,
	}
	if skip, reason := objectiveFailureGate(msgs); !skip {
		t.Errorf("末轮工具全部失败应被拦截, got skip=%v reason=%q", skip, reason)
	}
}

func TestPreGate_PartialToolFailurePasses(t *testing.T) {
	// 有一个工具成功（error 空）→ 非全失败，不拦截。
	m := msg(domain_conversation.RoleAssistant, "已取到部分数据", 1, true)
	m.ToolCalls = `[{"name":"sql_execute","error":""},{"name":"get_schema","error":"timeout"}]`
	msgs := []*domain_conversation.Message{
		msg(domain_conversation.RoleUser, "查数据", 0, true),
		m,
	}
	if skip, _ := objectiveFailureGate(msgs); skip {
		t.Error("部分工具成功不应被拦截")
	}
}

func TestPreGate_UserNegativeFeedbackAtEnd(t *testing.T) {
	msgs := []*domain_conversation.Message{
		msg(domain_conversation.RoleUser, "查营收", 0, true),
		msg(domain_conversation.RoleAssistant, "结果如上", 1, true),
		msg(domain_conversation.RoleUser, "不对，这不是我要的，重来", 2, true), // 用户末轮否定
	}
	if skip, reason := objectiveFailureGate(msgs); !skip {
		t.Errorf("用户末轮负反馈应被拦截, got skip=%v reason=%q", skip, reason)
	}
}

func TestPreGate_MidConversationCorrectionPasses(t *testing.T) {
	// 会话中途用户纠偏（"不对"），但最后助手正常收尾、用户未再否定 → 应放行沉淀。
	msgs := []*domain_conversation.Message{
		msg(domain_conversation.RoleUser, "查营收", 0, true),
		msg(domain_conversation.RoleAssistant, "华北最高", 1, true),
		msg(domain_conversation.RoleUser, "不对，我要华东", 2, true),
		msg(domain_conversation.RoleAssistant, "已更正：华东各月营收如下", 3, true),
	}
	if skip, reason := objectiveFailureGate(msgs); skip {
		t.Errorf("中途纠偏后成功收尾不应被拦截, got skip=%v reason=%q", skip, reason)
	}
}
