package convcontext

import (
	"context"
	"testing"

	"link/internal/model/conversation"
	"link/internal/model/memory"
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
