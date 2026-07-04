// Package chat: 会话归属校验（IDOR 防护）单元测试。
package chat

import (
	"context"
	"testing"

	"link/internal/model/conversation"
)

// newAuthzFixture 造一个属于 ownerID 的会话，返回装配好的 service 与 sessionRepo。
// retrievalRepo 传 nil：被测的越权拒绝路径在触达 RAG 加载前即返回。
func newAuthzFixture(ownerID int64) (*SessionService, *mockSessionRepository) {
	sessionRepo := newMockSessionRepository()
	sessionRepo.sessions["sess-owned"] = &conversation.Session{
		ID:     "sess-owned",
		UserID: ownerID,
		Title:  "owner's session",
		Status: 1,
	}
	svc := NewSessionService(sessionRepo, newMockMessageRepository(), nil)
	return svc, sessionRepo
}

func ctxWithUser(userID int64) context.Context {
	return context.WithValue(context.Background(), "user_id", userID)
}

// TestAuthz_GetSessionDetail_DeniesOtherUser 他人不得读取会话详情。
func TestAuthz_GetSessionDetail_DeniesOtherUser(t *testing.T) {
	svc, _ := newAuthzFixture(100)

	_, err := svc.GetSessionDetail(ctxWithUser(200), "sess-owned")
	if err == nil {
		t.Fatal("期望越权读取被拒绝，却成功了")
	}
}

// TestAuthz_GetSessionDetail_AllowsOwner 归属者可读取。
func TestAuthz_GetSessionDetail_AllowsOwner(t *testing.T) {
	svc, _ := newAuthzFixture(100)

	got, err := svc.GetSessionDetail(ctxWithUser(100), "sess-owned")
	if err != nil {
		t.Fatalf("归属者读取应成功: %v", err)
	}
	if got.Session.ID != "sess-owned" {
		t.Errorf("会话 ID = %s, 期望 sess-owned", got.Session.ID)
	}
}

// TestAuthz_NoUserID_BackwardCompatible 无 user_id 的内部调用保持向后兼容（不校验）。
func TestAuthz_NoUserID_BackwardCompatible(t *testing.T) {
	svc, _ := newAuthzFixture(100)

	if _, err := svc.GetSessionDetail(context.Background(), "sess-owned"); err != nil {
		t.Fatalf("无 user_id 的调用应放行: %v", err)
	}
}

// TestAuthz_DeleteSession_DeniesOtherUser 他人不得删除，且会话不被删除。
func TestAuthz_DeleteSession_DeniesOtherUser(t *testing.T) {
	svc, repo := newAuthzFixture(100)

	err := svc.DeleteSession(ctxWithUser(200), "sess-owned")
	if err == nil {
		t.Fatal("期望越权删除被拒绝，却成功了")
	}
	if _, ok := repo.sessions["sess-owned"]; !ok {
		t.Error("越权删除被拒后，会话不应被删除")
	}
}

// TestAuthz_ArchiveActivate_DeniesOtherUser 他人不得归档/激活。
func TestAuthz_ArchiveActivate_DeniesOtherUser(t *testing.T) {
	svc, _ := newAuthzFixture(100)

	if err := svc.ArchiveSession(ctxWithUser(200), "sess-owned"); err == nil {
		t.Error("期望越权归档被拒绝")
	}
	if err := svc.ActivateSession(ctxWithUser(200), "sess-owned"); err == nil {
		t.Error("期望越权激活被拒绝")
	}
}
