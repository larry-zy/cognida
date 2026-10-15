package handler

import (
	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/conversation"
	agentuc "cognida/internal/service/agent"
	"cognida/internal/service/agent/tools"
	"context"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"
)

type researchHTTPModel struct{ model.BaseChatModel }

func (researchHTTPModel) Generate(_ context.Context, msgs []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	if strings.Contains(msgs[1].Content, "max_questions") {
		return schema.AssistantMessage(`{"questions":[{"question":"document evidence","strategy":"hybrid"}]}`, nil), nil
	}
	return schema.AssistantMessage(`{"summary":"Summary [S1]","analysis":"Analysis [S1]","findings":[{"statement":"Supported finding","citations":[{"source_id":"S1","quote":"Verified evidence in the source document."}]}]}`, nil), nil
}

type researchHTTPRegistry map[string]tool.BaseTool

func (r researchHTTPRegistry) Get(n string) (tool.BaseTool, bool) { v, ok := r[n]; return v, ok }

type researchSessionRepo struct {
	conversation.SessionRepository
	session *conversation.Session
}

func (r researchSessionRepo) FindByID(context.Context, string) (*conversation.Session, error) {
	return r.session, nil
}
func TestResearchHTTPAuthorizationErrorsAndSSEReport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := researchHTTPRegistry{"rag_query": tools.NewTypedBaseTool("rag_query", "retrieval", func(ctx context.Context, _ *tools.RAGQueryRequest) (*tools.RAGQueryResult, error) {
		require.Equal(t, int64(7), agentctx.MustGetTenantID(ctx))
		require.Equal(t, int64(9), agentctx.MustGetUserID(ctx))
		require.Equal(t, []string{"allowed"}, agentctx.MustGetAllowedKBIDs(ctx))
		return &tools.RAGQueryResult{Chunks: []tools.DocumentChunk{{Content: "Verified evidence in the source document.", ChunkID: "c1", KnowledgeBaseID: "allowed"}}}, nil
	})}
	svc := agentuc.NewResearchService(nil, nil).WithDependencies(researchHTTPModel{}, registry)
	persistence := agentuc.NewAgentPersistenceService(researchSessionRepo{session: &conversation.Session{ID: "session", TenantID: 7, UserID: 9}}, nil)
	h := NewAgentHandler(nil, svc, nil, nil, persistence, nil)
	request := func(path, body string, tenant, user int64) *httptest.ResponseRecorder {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("tenant_id", tenant); c.Set("user_id", user) })
		r.POST("/research", h.DeepResearch)
		r.POST("/stream", h.DeepResearchStream)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	for _, owner := range []struct{ tenant, user int64 }{{8, 9}, {7, 10}} {
		w := request("/research", `{"query":"q","session_id":"session"}`, owner.tenant, owner.user)
		require.Equal(t, 403, w.Code, w.Body.String())
	}
	require.Equal(t, 401, request("/research", `{"query":"q"}`, 0, 0).Code)
	require.Equal(t, 400, request("/stream", `{"query":"q","options":{"max_sub_questions":6}}`, 7, 9).Code)
	w := request("/stream", `{"query":"q","kb_ids":["allowed"],"session_id":"session","context":{"tenant_id":999,"user_id":999}}`, 7, 9)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
	require.Contains(t, w.Body.String(), "event: complete")
	require.Contains(t, w.Body.String(), `"report":`)
	require.Contains(t, w.Body.String(), `"success":true`)
	require.Contains(t, w.Body.String(), `"chunk_id":"c1"`)
	delete(registry, "rag_query")
	w = request("/research", `{"query":"q"}`, 7, 9)
	require.Equal(t, 422, w.Code)
	require.Contains(t, w.Body.String(), `"success":false`)
	w = request("/stream", `{"query":"q"}`, 7, 9)
	require.Contains(t, w.Body.String(), "event: error")
	require.NotContains(t, w.Body.String(), "event: complete")
}
