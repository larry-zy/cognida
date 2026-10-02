package agent

import (
	"context"
	"strings"
	"testing"

	"cognida/internal/model/conversation"
)

// captureSessionRepo 记录 Create 收到的会话，其余方法为无操作占位。
type captureSessionRepo struct {
	conversation.SessionRepository
	created *conversation.Session
}

func (r *captureSessionRepo) Create(_ context.Context, s *conversation.Session) error {
	r.created = s
	return nil
}

func (r *captureSessionRepo) FindByID(_ context.Context, _ string) (*conversation.Session, error) {
	return r.created, nil
}

func TestPrepareSession_RequiresBothTenantAndUserOwnership(t *testing.T) {
	repo := &captureSessionRepo{created: &conversation.Session{ID: "private", TenantID: 1, UserID: 2}}
	svc := NewAgentPersistenceService(repo, nil)
	for _, tc := range []struct {
		name         string
		tenant, user int64
		allowed      bool
	}{
		{"owner", 1, 2, true},
		{"another user in same tenant", 1, 3, false},
		{"another tenant", 3, 2, false},
		{"unauthenticated", 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.PrepareSession(context.Background(), &PrepareSessionRequest{SessionID: "private", TenantID: tc.tenant, UserID: tc.user})
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v, error=%v", tc.allowed, err)
			}
		})
	}
	// A repository that returns no row without an error must fail closed.
	repo.created = nil
	if _, err := svc.PrepareSession(context.Background(), &PrepareSessionRequest{SessionID: "missing", TenantID: 1, UserID: 2}); err == nil {
		t.Fatal("missing session accepted")
	}
}

// TestPrepareSession_TitleFromQuery 回归：新建会话时 Query 必须透传进标题，
// 否则所有会话退化为同名 agent 名（本次修复的 bug）。
func TestPrepareSession_TitleFromQuery(t *testing.T) {
	repo := &captureSessionRepo{}
	svc := NewAgentPersistenceService(repo, nil)

	_, err := svc.PrepareSession(context.Background(), &PrepareSessionRequest{
		AgentID:  "agent-rag-001",
		UserID:   1,
		TenantID: 1,
		Query:    "知识库里关于退款政策怎么说",
	})
	if err != nil {
		t.Fatalf("PrepareSession 失败: %v", err)
	}
	if repo.created == nil {
		t.Fatal("未创建会话")
	}
	if !strings.Contains(repo.created.Title, "退款政策") {
		t.Errorf("标题未包含 Query 内容: %q", repo.created.Title)
	}
	// 两次不同 Query 必须得到不同标题（不再同名）
	repo2 := &captureSessionRepo{}
	svc2 := NewAgentPersistenceService(repo2, nil)
	_, _ = svc2.PrepareSession(context.Background(), &PrepareSessionRequest{
		AgentID:  "agent-rag-001",
		UserID:   1,
		TenantID: 1,
		Query:    "帮我分析上月销量",
	})
	if repo.created.Title == repo2.created.Title {
		t.Errorf("不同 Query 生成了相同标题: %q", repo.created.Title)
	}
}
