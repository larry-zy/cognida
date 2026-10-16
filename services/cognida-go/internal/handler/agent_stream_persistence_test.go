package handler

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"cognida/internal/handler/sse"
	"cognida/internal/model/conversation"
	agentuc "cognida/internal/service/agent"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type streamMessageRepo struct {
	conversation.MessageRepository
	w     *httptest.ResponseRecorder
	t     *testing.T
	saved int
	err   error
}

func (r *streamMessageRepo) Create(ctx context.Context, m *conversation.Message) error {
	if len(m.ID) != 36 {
		r.t.Fatalf("stream message ID does not fit the migrated UUID column: %d bytes", len(m.ID))
	}
	if _, err := uuid.Parse(m.ID); err != nil {
		r.t.Fatal("stream message ID must retain a complete UUID")
	}
	if ctx.Err() != nil {
		r.t.Fatal("persistence inherited canceled request")
	}
	if _, ok := ctx.Deadline(); !ok {
		r.t.Fatal("persistence must have a deadline")
	}
	if strings.Contains(r.w.Body.String(), `"event":"done"`) {
		r.t.Fatal("done emitted before persistence")
	}
	r.saved++
	return r.err
}
func (r *streamMessageRepo) CountBySessionID(context.Context, string) (int64, error) { return 2, nil }

type streamSessionRepo struct{ conversation.SessionRepository }

func (*streamSessionRepo) UpdateMessageCount(context.Context, string, int) error { return nil }

func TestStreamPersistsBeforeDoneAndReportsFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fail := range []bool{false, true} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", nil)
		r := &streamMessageRepo{w: w, t: t}
		if fail {
			r.err = errors.New("database unavailable")
		}
		h := &AgentHandler{persistenceService: agentuc.NewAgentPersistenceService(&streamSessionRepo{}, r)}
		chunks := make(chan *agentuc.ChatChunkDTO, 2)
		chunks <- &agentuc.ChatChunkDTO{Content: "answer"}
		chunks <- &agentuc.ChatChunkDTO{Done: true}
		h.streamAgentChunks(c, sse.NewWriter(w), chunks, genUIOption{sessionID: "owned-session"})
		if r.saved != 1 {
			t.Fatalf("saved %d times", r.saved)
		}
		want := `"persistence_failed":false`
		if fail {
			want = `"persistence_failed":true`
		}
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("missing persistence outcome: %s", w.Body.String())
		}
	}
}

func TestPersistStreamResultSurvivesCanceledRequest(t *testing.T) {
	w := httptest.NewRecorder()
	r := &streamMessageRepo{w: w, t: t}
	h := &AgentHandler{persistenceService: agentuc.NewAgentPersistenceService(&streamSessionRepo{}, r)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.persistStreamResult(ctx, "owned-session", agentStreamResult{Content: "partial answer"}); err != nil {
		t.Fatal(err)
	}
	if r.saved != 1 {
		t.Fatal("partial response was not persisted")
	}
}
