//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"cognida/internal/handler/sse"
	"cognida/internal/model/conversation"
	repository "cognida/internal/repository/mysql"
	agentuc "cognida/internal/service/agent"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Uses the migrated isolated review DB and production SSE/persistence path.
// Synthetic stream chunks exercise storage only; no model/retrieval is claimed.
func TestAgentStreamMySQLPersistenceUsesMigratedMessageID(t *testing.T) {
	f := newSupervisorHTTPFixture(t)
	ctx := context.Background()
	sessions := repository.NewSessionRepository(f.db)
	messages := repository.NewMessageRepository(f.db)
	persistence := agentuc.NewAgentPersistenceService(sessions, messages)
	session, err := persistence.PrepareSession(ctx, &agentuc.PrepareSessionRequest{AgentID: "agent-rag-001", TenantID: f.owner.TenantID, UserID: f.owner.UserID, Query: "Synthetic stream storage regression"})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, f.db.Exec("DELETE FROM messages WHERE session_id = ?", session.ID).Error)
		require.NoError(t, f.db.Exec("DELETE FROM sessions WHERE id = ? AND tenant_id = ? AND user_id = ?", session.ID, f.owner.TenantID, f.owner.UserID).Error)
	})
	require.NoError(t, persistence.SaveUserMessage(ctx, &agentuc.SaveMessageRequest{MessageID: uuid.NewString(), SessionID: session.ID, Content: "synthetic question", UserID: f.owner.UserID}))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/stream", nil)
	chunks := make(chan *agentuc.ChatChunkDTO, 3)
	toolOutput := `{"chunks":[{"chunk_id":"fixture-chunk","knowledge_id":"fixture-document","knowledge_base_id":"fixture-kb","content":"synthetic evidence","score":0.7}]}`
	chunks <- &agentuc.ChatChunkDTO{Metadata: map[string]interface{}{"type": "tool_result", "tool_name": "rag_query", "tool_id": "fixture-call", "status": "success", "tool_output": toolOutput}}
	chunks <- &agentuc.ChatChunkDTO{Content: "synthetic answer"}
	chunks <- &agentuc.ChatChunkDTO{Done: true}
	handler := &AgentHandler{persistenceService: persistence}
	handler.streamAgentChunks(c, sse.NewWriter(w), chunks, genUIOption{sessionID: session.ID})
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"persistence_failed":false`)
	// Recreate repositories to read durable state, not an in-memory callback.
	rows, total, err := repository.NewMessageRepository(f.db).FindBySessionID(ctx, session.ID, &conversation.ListMessagesRequest{Page: 1, Size: 10})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	var assistant *conversation.Message
	for _, row := range rows {
		if row.Role == conversation.RoleAssistant {
			assistant = row
		}
	}
	require.NotNil(t, assistant)
	require.Len(t, assistant.ID, 36)
	_, err = uuid.Parse(assistant.ID)
	require.NoError(t, err)
	require.True(t, assistant.IsCompleted)
	require.Equal(t, "synthetic answer", assistant.Content)
	var saved struct {
		Steps []map[string]interface{} `json:"steps"`
	}
	require.NoError(t, json.Unmarshal([]byte(assistant.AgentSteps), &saved))
	require.Len(t, saved.Steps, 1)
	require.Equal(t, toolOutput, saved.Steps[0]["tool_output"])
	storedSession, err := repository.NewSessionRepository(f.db).FindByID(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, 2, storedSession.MessageCount)
}
