package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	domagent "link/internal/model/agent"
)

// stubAgentRegistry 仅用于评测 handler 的 agent_id 校验测试：只关心 Exists。
type stubAgentRegistry struct {
	exists map[string]bool
}

func (s *stubAgentRegistry) Register(context.Context, *domagent.AgentDefinition) error { return nil }
func (s *stubAgentRegistry) Unregister(context.Context, string) error                  { return nil }
func (s *stubAgentRegistry) Get(context.Context, string) (*domagent.AgentDefinition, error) {
	return nil, nil
}
func (s *stubAgentRegistry) List(context.Context) ([]*domagent.AgentDefinition, error) {
	return nil, nil
}
func (s *stubAgentRegistry) FindByType(context.Context, domagent.AgentType) ([]*domagent.AgentDefinition, error) {
	return nil, nil
}
func (s *stubAgentRegistry) FindByName(context.Context, string) (*domagent.AgentDefinition, error) {
	return nil, nil
}
func (s *stubAgentRegistry) Exists(_ context.Context, agentID string) (bool, error) {
	return s.exists[agentID], nil
}
func (s *stubAgentRegistry) UpdateStatus(context.Context, string, domagent.AgentStatus) error {
	return nil
}

// postCreateTask 用给定 body 打到 CreateTask，返回响应记录器。
func postCreateTask(t *testing.T, h *EvaluationHandler, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("tenant_id", int64(1))
	c.Set("user_id", int64(1))
	raw, _ := json.Marshal(body)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/evaluation/tasks", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	h.CreateTask(c)
	return w
}

// TestCreateTask_AgentMissingID agent 类型缺少 agent_id → 400，且在触达 service 前短路。
func TestCreateTask_AgentMissingID(t *testing.T) {
	h := &EvaluationHandler{agentRegistry: &stubAgentRegistry{}}
	w := postCreateTask(t, h, map[string]interface{}{
		"dataset_id": "ds-1",
		"type":       "agent",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing agent_id, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCreateTask_AgentUnknownID agent 类型带不存在的 agent_id → 400。
func TestCreateTask_AgentUnknownID(t *testing.T) {
	h := &EvaluationHandler{agentRegistry: &stubAgentRegistry{exists: map[string]bool{}}}
	w := postCreateTask(t, h, map[string]interface{}{
		"dataset_id": "ds-1",
		"type":       "agent",
		"agent_id":   "ghost",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown agent_id, got %d: %s", w.Code, w.Body.String())
	}
}
