package convcontext

import (
	"context"
	"strings"
	"testing"

	"cognida/internal/model/conversation"
	"cognida/internal/model/memory"
)

// stubMessageRepo 仅实现 Build 所需的 FindRecentBySessionID，其余方法留空满足接口。
type stubMessageRepo struct {
	recent []*conversation.Message
	err    error
}

func (s *stubMessageRepo) FindRecentBySessionID(ctx context.Context, sessionID string, limit int) ([]*conversation.Message, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.recent, nil
}

func (s *stubMessageRepo) Create(ctx context.Context, m *conversation.Message) error { return nil }
func (s *stubMessageRepo) CreateBatch(ctx context.Context, m []*conversation.Message) error {
	return nil
}
func (s *stubMessageRepo) FindByID(ctx context.Context, id string) (*conversation.Message, error) {
	return nil, nil
}
func (s *stubMessageRepo) FindBySessionID(ctx context.Context, sessionID string, req *conversation.ListMessagesRequest) ([]*conversation.Message, int64, error) {
	return nil, 0, nil
}
func (s *stubMessageRepo) FindByRequestID(ctx context.Context, requestID string) (*conversation.Message, error) {
	return nil, nil
}
func (s *stubMessageRepo) Update(ctx context.Context, m *conversation.Message) error       { return nil }
func (s *stubMessageRepo) UpdateContent(ctx context.Context, id, content string) error      { return nil }
func (s *stubMessageRepo) UpdateCompleted(ctx context.Context, id string, done bool) error  { return nil }
func (s *stubMessageRepo) Delete(ctx context.Context, id string) error                      { return nil }
func (s *stubMessageRepo) DeleteBySessionID(ctx context.Context, sessionID string) error    { return nil }
func (s *stubMessageRepo) CountBySessionID(ctx context.Context, sessionID string) (int64, error) {
	return 0, nil
}
func (s *stubMessageRepo) GetTokenCountBySessionID(ctx context.Context, sessionID string) (int, error) {
	return 0, nil
}

func msg(role, content string) *conversation.Message {
	return &conversation.Message{Role: role, Content: content}
}

func buildReq(current string) *memory.BuildContextRequest {
	return &memory.BuildContextRequest{
		SessionID:      "sess-1",
		CurrentMessage: current,
		Config:         &memory.ContextBuilderConfig{SystemPrompt: "SYS"},
	}
}

// TestBuild_AssemblesSystemHistoryAndCurrent 历史末条为 assistant 时，追加当前问题。
func TestBuild_AssemblesSystemHistoryAndCurrent(t *testing.T) {
	repo := &stubMessageRepo{recent: []*conversation.Message{
		msg(conversation.RoleUser, "第一轮问题"),
		msg(conversation.RoleAssistant, "第一轮回答"),
	}}
	b := NewConversationContextBuilder(repo)

	got, err := b.Build(context.Background(), buildReq("继续"))
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	// 期望: system, user(第一轮问题), assistant(第一轮回答), user(继续)
	want := []struct{ role, content string }{
		{"system", "SYS"},
		{"user", "第一轮问题"},
		{"assistant", "第一轮回答"},
		{"user", "继续"},
	}
	if len(got.Messages) != len(want) {
		t.Fatalf("消息条数 = %d, 期望 %d: %+v", len(got.Messages), len(want), got.Messages)
	}
	for i, w := range want {
		if got.Messages[i].Role != w.role || got.Messages[i].Content != w.content {
			t.Errorf("消息[%d] = (%s,%q), 期望 (%s,%q)", i, got.Messages[i].Role, got.Messages[i].Content, w.role, w.content)
		}
	}
}

// TestBuild_DedupCurrentMessage 历史末条已是当前问题（handler 已落库）时不重复追加。
func TestBuild_DedupCurrentMessage(t *testing.T) {
	repo := &stubMessageRepo{recent: []*conversation.Message{
		msg(conversation.RoleAssistant, "上轮回答"),
		msg(conversation.RoleUser, "本轮问题"),
	}}
	b := NewConversationContextBuilder(repo)

	got, err := b.Build(context.Background(), buildReq("本轮问题"))
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	// 期望: system, assistant(上轮回答), user(本轮问题) —— 不出现两条 user(本轮问题)
	last := got.Messages[len(got.Messages)-1]
	if last.Role != "user" || last.Content != "本轮问题" {
		t.Fatalf("末条 = (%s,%q), 期望 user/本轮问题", last.Role, last.Content)
	}
	userCount := 0
	for _, m := range got.Messages {
		if m.Role == "user" && m.Content == "本轮问题" {
			userCount++
		}
	}
	if userCount != 1 {
		t.Errorf("当前问题出现 %d 次, 期望 1 次（去重）", userCount)
	}
}

// TestBuild_FiltersNoiseAndEmpty 过滤空内容与非 user/assistant 角色（system/tool 噪声）。
func TestBuild_FiltersNoiseAndEmpty(t *testing.T) {
	repo := &stubMessageRepo{recent: []*conversation.Message{
		msg(conversation.RoleSystem, "旧系统提示"), // 应过滤
		msg(conversation.RoleUser, ""),          // 空内容，应过滤
		msg("tool", "工具输出"),                     // 非对话角色，应过滤
		msg(conversation.RoleAssistant, "有效回答"),
	}}
	b := NewConversationContextBuilder(repo)

	got, err := b.Build(context.Background(), buildReq("新问题"))
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	// 历史仅剩 assistant(有效回答)
	if len(got.History) != 1 || got.History[0].Content != "有效回答" {
		t.Fatalf("History = %+v, 期望仅 [有效回答]", got.History)
	}
	// 完整列表: system, assistant(有效回答), user(新问题)
	if len(got.Messages) != 3 {
		t.Fatalf("消息条数 = %d, 期望 3: %+v", len(got.Messages), got.Messages)
	}
}

// budgetReq 构造带 token 预算的请求，触发窗口化 + 摘要 + 预算治理路径。
func budgetReq(current string, maxTokens int) *memory.BuildContextRequest {
	return &memory.BuildContextRequest{
		SessionID:      "sess-1",
		CurrentMessage: current,
		Config:         &memory.ContextBuilderConfig{SystemPrompt: "SYS", MaxTokens: maxTokens},
	}
}

// TestBuild_OverBudgetSummarizesOlderTurns 历史超 token 预算时，早期轮被摘要为紧凑记忆，
// 作为「紧邻系统提示的独立记忆消息」注入（③缓存前缀稳定：绝不并入 message[0]），而非静默丢弃；
// 最近若干轮仍逐字保留；系统提示（Pinned）始终逐字保留。
func TestBuild_OverBudgetSummarizesOlderTurns(t *testing.T) {
	repo := &stubMessageRepo{recent: []*conversation.Message{
		msg(conversation.RoleUser, "第一轮很长的问题内容甲乙丙丁"),
		msg(conversation.RoleAssistant, "第一轮很长的回答内容甲乙丙丁"),
		msg(conversation.RoleUser, "第二轮问题戊己庚辛"),
		msg(conversation.RoleAssistant, "第二轮回答戊己庚辛"),
	}}
	b := NewConversationContextBuilder(repo)

	// 预算紧张：只够装下最近少数轮，早期轮必须被摘要。
	got, err := b.Build(context.Background(), budgetReq("最新问题", 12))
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	// message[0] 必须是逐字稳定的纯系统提示 SYS（预算再紧也永不被裁、永不被摘要污染）——
	// 这是服务端前缀/KV 缓存能覆盖「系统提示+工具契约」的前提。
	if got.Messages[0].Role != "system" || got.Messages[0].Content != "SYS" {
		t.Fatalf("message[0] 应为逐字纯系统提示 SYS, got (%s,%q)", got.Messages[0].Role, got.Messages[0].Content)
	}
	// 超预算触发窗口化：早期摘要作为紧邻的独立 system 消息（message[1]）注入，非并入 message[0]。
	if len(got.Messages) < 2 || got.Messages[1].Role != "system" || got.Messages[1].Content == "" {
		t.Fatalf("超预算应把早期摘要作为独立 system 消息注入 message[1], got %+v", got.Messages)
	}
	if len(got.History) >= 4 {
		t.Errorf("最近窗口应裁剪到 <4 条, got %d", len(got.History))
	}
	last := got.Messages[len(got.Messages)-1]
	if last.Role != "user" || last.Content != "最新问题" {
		t.Errorf("末条应为当前问题, got (%s,%q)", last.Role, last.Content)
	}
}

// TestBuild_SummaryPreservesResultID 被摘要的早期轮里的 result_id 必须保留到摘要文本，
// 否则后续轮次无法按引用取回完整结果集。
func TestBuild_SummaryPreservesResultID(t *testing.T) {
	repo := &stubMessageRepo{recent: []*conversation.Message{
		msg(conversation.RoleAssistant, "查询完成，结果见 rs_alpha 甲乙丙丁戊"),
		msg(conversation.RoleUser, "第二轮问题戊己庚辛"),
		msg(conversation.RoleAssistant, "第二轮回答戊己庚辛"),
	}}
	b := NewConversationContextBuilder(repo)

	got, err := b.Build(context.Background(), budgetReq("最新问题", 10))
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	// message[0] 仍是逐字纯系统提示；result_id 保留在紧邻的独立摘要消息（message[1]）里。
	if got.Messages[0].Content != "SYS" {
		t.Errorf("message[0] 应为逐字纯系统提示 SYS, got: %q", got.Messages[0].Content)
	}
	if len(got.Messages) < 2 || !strings.Contains(got.Messages[1].Content, "rs_alpha") {
		t.Errorf("摘要（独立 message[1]）必须保留早期轮的 result_id rs_alpha, got: %+v", got.Messages)
	}
}

// TestBuild_WithinBudgetKeepsVerbatim 预算充足时逐字保留全部历史，不触发摘要（零回归）。
func TestBuild_WithinBudgetKeepsVerbatim(t *testing.T) {
	repo := &stubMessageRepo{recent: []*conversation.Message{
		msg(conversation.RoleUser, "问题"),
		msg(conversation.RoleAssistant, "回答"),
	}}
	b := NewConversationContextBuilder(repo)

	got, err := b.Build(context.Background(), budgetReq("继续", 4000))
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if got.Messages[0].Content != "SYS" {
		t.Errorf("预算充足时系统消息应为原始 SYS（无摘要）, got %q", got.Messages[0].Content)
	}
	if len(got.History) != 2 {
		t.Errorf("预算充足应保留全部 2 条历史, got %d", len(got.History))
	}
}

// TestBuild_NilRepoDegradesToCurrentOnly messageRepo 为 nil 时退化为仅 [系统提示 + 当前问题]。
func TestBuild_NilRepoDegradesToCurrentOnly(t *testing.T) {
	b := NewConversationContextBuilder(nil)

	got, err := b.Build(context.Background(), buildReq("孤立问题"))
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("消息条数 = %d, 期望 2: %+v", len(got.Messages), got.Messages)
	}
	if got.Messages[0].Role != "system" || got.Messages[1].Role != "user" {
		t.Errorf("期望 [system,user], 得到 [%s,%s]", got.Messages[0].Role, got.Messages[1].Role)
	}
}
