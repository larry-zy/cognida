package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"cognida/internal/model/audit"
	"cognida/internal/model/conversation"
	svc "cognida/internal/service/chat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type scopeSessionRepo struct {
	conversation.SessionRepository
	session *conversation.Session
}

func (r *scopeSessionRepo) FindByID(context.Context, string) (*conversation.Session, error) {
	return r.session, nil
}

type scopeMessageRepo struct {
	conversation.MessageRepository
	msg           *conversation.Message
	writes, reads int
}

func (r *scopeMessageRepo) FindByID(context.Context, string) (*conversation.Message, error) {
	r.reads++
	return r.msg, nil
}
func (r *scopeMessageRepo) Create(_ context.Context, m *conversation.Message) error {
	r.writes++
	m.ID = "created"
	return nil
}
func (r *scopeMessageRepo) FindBySessionID(context.Context, string, *conversation.ListMessagesRequest) ([]*conversation.Message, int64, error) {
	r.reads++
	return []*conversation.Message{r.msg}, 1, nil
}
func (r *scopeMessageRepo) UpdateContent(_ context.Context, _ string, s string) error {
	r.writes++
	r.msg.Content = s
	return nil
}
func (r *scopeMessageRepo) Delete(context.Context, string) error { r.writes++; return nil }
func messageScopeRouter(messages conversation.MessageRepository, sessions conversation.SessionRepository, tenant, user int64) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if tenant > 0 {
			c.Set("tenant_id", tenant)
		}
		if user > 0 {
			c.Set("user_id", user)
		}
	})
	h := NewMessageHandler(svc.NewMessageService(messages, sessions))
	r.POST("/messages", h.CreateMessage)
	r.GET("/messages", h.ListMessages)
	r.GET("/messages/:id", h.GetMessage)
	r.PUT("/messages/:id", h.UpdateMessage)
	r.DELETE("/messages/:id", h.DeleteMessage)
	return r
}
func TestMessageHTTPRequiresSessionOwnerAndExplicitScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, identity := range []struct {
		name         string
		tenant, user int64
		want         int
	}{{"owner", 7, 42, 200}, {"other user", 7, 43, 403}, {"other tenant", 8, 42, 403}, {"missing tenant", 0, 42, 403}, {"missing user", 7, 0, 403}} {
		for _, op := range []struct{ method, path, body string }{{"POST", "/messages", `{"session_id":"s","role":"user","content":"new"}`}, {"GET", "/messages?session_id=s", ""}, {"GET", "/messages/m", ""}, {"PUT", "/messages/m", `{"content":"updated"}`}, {"DELETE", "/messages/m", ""}} {
			t.Run(identity.name+op.method+op.path, func(t *testing.T) {
				messages := &scopeMessageRepo{msg: &conversation.Message{ID: "m", SessionID: "s", Content: "private-message"}}
				sessions := &scopeSessionRepo{session: &conversation.Session{ID: "s", TenantID: 7, UserID: 42}}
				r := messageScopeRouter(messages, sessions, identity.tenant, identity.user)
				q := httptest.NewRequest(op.method, op.path, bytes.NewBufferString(op.body))
				q.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				r.ServeHTTP(w, q)
				want := identity.want
				if want == 200 && op.method == "POST" {
					want = 201
				}
				require.Equal(t, want, w.Code, w.Body.String())
				if want == 403 {
					require.Zero(t, messages.writes)
					require.NotContains(t, w.Body.String(), "private-message")
				}
			})
		}
	}
}

type scopedAuditRepo struct {
	audit.Repository
	calls  int
	tenant int64
}

func (r *scopedAuditRepo) FindByID(_ context.Context, tenant, id int64) (*audit.AuditLog, error) {
	r.calls++
	r.tenant = tenant
	if tenant != 7 {
		return nil, fmt.Errorf("not found")
	}
	return &audit.AuditLog{ID: id, TenantID: &tenant}, nil
}
func TestAuditDetailPassesTenantAndRejectsMissingScope(t *testing.T) {
	for _, tc := range []struct {
		tenant int64
		status int
	}{{7, 200}, {8, 404}, {0, 401}} {
		repo := &scopedAuditRepo{}
		r := gin.New()
		r.Use(func(c *gin.Context) {
			if tc.tenant > 0 {
				c.Set("tenant_id", tc.tenant)
			}
		})
		r.GET("/audit/:id", NewAuditHandler(repo, nil).GetAuditLog)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/audit/12", nil))
		require.Equal(t, tc.status, w.Code)
		if tc.tenant == 0 {
			require.Zero(t, repo.calls)
		} else {
			require.Equal(t, tc.tenant, repo.tenant)
		}
	}
}
