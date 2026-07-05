// Package chat: 会话归属校验（IDOR 防护）单元测试。
// authorizeSession 为 fail-closed：ctx 必须同时携带有效 user_id 与 tenant_id，
// 缺失或不匹配一律拒绝，不退化为默认身份。
package chat

import (
	"context"
	"testing"

	"link/internal/model/conversation"
)

// newAuthzFixture 造一个属于 (ownerID, tenantID) 的会话，返回装配好的 service 与 sessionRepo。
// retrievalRepo 传 nil：被测的越权拒绝路径在触达 RAG 加载前即返回。
func newAuthzFixture(ownerID, tenantID int64) (*SessionService, *mockSessionRepository) {
	sessionRepo := newMockSessionRepository()
	sessionRepo.sessions["sess-owned"] = &conversation.Session{
		ID:       "sess-owned",
		UserID:   ownerID,
		TenantID: tenantID,
		Title:    "owner's session",
		Status:   1,
	}
	svc := NewSessionService(sessionRepo, newMockMessageRepository(), nil)
	return svc, sessionRepo
}

func ctxWithIdentity(userID, tenantID int64) context.Context {
	ctx := context.WithValue(context.Background(), "user_id", userID)
	return context.WithValue(ctx, "tenant_id", tenantID)
}

// TestAuthz_GetSessionDetail_DeniesOtherUser 他人不得读取会话详情。
func TestAuthz_GetSessionDetail_DeniesOtherUser(t *testing.T) {
	svc, _ := newAuthzFixture(100, 1)

	_, err := svc.GetSessionDetail(ctxWithIdentity(200, 1), "sess-owned")
	if err == nil {
		t.Fatal("期望越权读取被拒绝，却成功了")
	}
}

// TestAuthz_GetSessionDetail_AllowsOwner 归属者（user+tenant 均匹配）可读取。
func TestAuthz_GetSessionDetail_AllowsOwner(t *testing.T) {
	svc, _ := newAuthzFixture(100, 1)

	got, err := svc.GetSessionDetail(ctxWithIdentity(100, 1), "sess-owned")
	if err != nil {
		t.Fatalf("归属者读取应成功: %v", err)
	}
	if got.Session.ID != "sess-owned" {
		t.Errorf("会话 ID = %s, 期望 sess-owned", got.Session.ID)
	}
}

// TestAuthz_TenantMismatch_Denied tenant 不符即拒绝，即使 user_id 匹配。
func TestAuthz_TenantMismatch_Denied(t *testing.T) {
	svc, _ := newAuthzFixture(100, 1)

	if _, err := svc.GetSessionDetail(ctxWithIdentity(100, 2), "sess-owned"); err == nil {
		t.Fatal("期望跨租户读取被拒绝，却成功了")
	}
}

// TestAuthz_MissingIdentity_Denied 缺身份（fail-closed）：
// 无 user_id / 无 tenant_id / 零值身份，均拒绝，不退化为默认身份。
func TestAuthz_MissingIdentity_Denied(t *testing.T) {
	svc, _ := newAuthzFixture(100, 1)

	cases := []struct {
		name string
		ctx  context.Context
	}{
		{"完全无身份", context.Background()},
		{"仅有 user_id 缺 tenant_id", context.WithValue(context.Background(), "user_id", int64(100))},
		{"仅有 tenant_id 缺 user_id", context.WithValue(context.Background(), "tenant_id", int64(1))},
		{"user_id 为零值", ctxWithIdentity(0, 1)},
		{"tenant_id 为零值", ctxWithIdentity(100, 0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.GetSessionDetail(tc.ctx, "sess-owned"); err == nil {
				t.Errorf("%s：期望被拒绝，却成功了", tc.name)
			}
		})
	}
}

// TestAuthz_DeleteSession_DeniesOtherUser 他人不得删除，且会话不被删除。
func TestAuthz_DeleteSession_DeniesOtherUser(t *testing.T) {
	svc, repo := newAuthzFixture(100, 1)

	err := svc.DeleteSession(ctxWithIdentity(200, 1), "sess-owned")
	if err == nil {
		t.Fatal("期望越权删除被拒绝，却成功了")
	}
	if _, ok := repo.sessions["sess-owned"]; !ok {
		t.Error("越权删除被拒后，会话不应被删除")
	}
}

// TestAuthz_ArchiveActivate_DeniesOtherUser 他人不得归档/激活。
func TestAuthz_ArchiveActivate_DeniesOtherUser(t *testing.T) {
	svc, _ := newAuthzFixture(100, 1)

	if err := svc.ArchiveSession(ctxWithIdentity(200, 1), "sess-owned"); err == nil {
		t.Error("期望越权归档被拒绝")
	}
	if err := svc.ActivateSession(ctxWithIdentity(200, 1), "sess-owned"); err == nil {
		t.Error("期望越权激活被拒绝")
	}
}
