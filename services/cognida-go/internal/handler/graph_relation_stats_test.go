package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"cognida/internal/handler/middleware"
	agentctx "cognida/internal/model/agent"
	domain "cognida/internal/model/knowledge"
	knowledgeapp "cognida/internal/service/knowledge"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type graphStatsAPIKB struct {
	domain.KnowledgeBaseRepository
	kb  *domain.KnowledgeBase
	err error
}

func (r *graphStatsAPIKB) FindByIDForTenant(context.Context, string, int64) (*domain.KnowledgeBase, error) {
	return r.kb, r.err
}

type graphStatsAPIRepo struct {
	domain.GraphRepository
	ns     domain.NameSpace
	calls  int
	writes int
	err    error
}

func (r *graphStatsAPIRepo) GetRelationStats(_ context.Context, ns domain.NameSpace) (*domain.RelationStatistics, error) {
	r.calls++
	r.ns = ns
	return &domain.RelationStatistics{TotalRelations: 3, AverageStrength: 2.5, AverageWeight: 4, AverageChunkCount: 2, StrengthDistribution: map[int]int{2: 3}, TypeDistribution: map[string]int{"owns": 3}}, r.err
}

func (r *graphStatsAPIRepo) AddRelation(_ context.Context, ns domain.NameSpace, relation *domain.GraphRelation) (*domain.GraphRelation, error) {
	r.writes++
	r.ns = ns
	return relation, r.err
}

func graphStatsAPIRouter(svc *knowledgeapp.GraphService, tenant, user any, withTenantMiddleware bool) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if tenant != nil {
			c.Set("tenant_id", tenant)
		}
		if user != nil {
			c.Set("user_id", user)
		}
		c.Set("request_id", "relation-stats-test")
		// An unrelated/stale typed scope and a request header cannot replace the
		// verified identity established in Gin by authentication middleware.
		ctx := agentctx.WithTenantID(c.Request.Context(), 999)
		c.Request = c.Request.WithContext(agentctx.WithUserID(ctx, 999))
		c.Next()
	})
	if withTenantMiddleware {
		r.Use(middleware.NewTenantMiddleware().Apply())
	}
	r.GET("/graph/stats/relations", NewGraphHandler(svc).GetRelationStats)
	r.POST("/knowledge-bases/:id/graph/relations", NewGraphHandler(svc).AddRelation)
	return r
}

func graphStatsAPICreateRelation(r http.Handler, kbID, source string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{"source": source, "target": "B", "type": "api-created", "strength": 5, "chunk_ids": []string{"one", "two", "three"}, "tenant_id": 1})
	req := httptest.NewRequest(http.MethodPost, "/knowledge-bases/"+url.PathEscape(kbID)+"/graph/relations", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func graphStatsAPIRequest(r http.Handler, kbID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/graph/stats/relations?kb_id="+url.QueryEscape(kbID), nil)
	req.Header.Set("X-Tenant-ID", "1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestGraphRelationStatsHTTPIdentityAuthorizationAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name            string
		tenant, user    any
		kbID            string
		kb              *domain.KnowledgeBase
		kbErr, graphErr error
		want            int
	}{
		{name: "missing identities", kbID: "kb", want: 401},
		{name: "missing tenant", user: int64(9), kbID: "kb", want: 401},
		{name: "missing user", tenant: int64(41), kbID: "kb", want: 401},
		{name: "string scope rejected", tenant: "41", user: "9", kbID: "kb", want: 401},
		{name: "floating scope rejected", tenant: 41.0, user: 9.0, kbID: "kb", want: 401},
		{name: "integer scope ambiguity rejected", tenant: 41, user: 9, kbID: "kb", want: 401},
		{name: "empty kb", tenant: int64(41), user: int64(9), kbID: " ", want: 400},
		{name: "not found", tenant: int64(41), user: int64(9), kbID: "kb", kbErr: errors.New("知识库不存在"), want: 404},
		{name: "private", tenant: int64(41), user: int64(9), kbID: "kb", kb: &domain.KnowledgeBase{ID: "kb", TenantID: 41, UserID: 10, Status: 1}, want: 403},
		{name: "cross tenant", tenant: int64(41), user: int64(9), kbID: "kb", kb: &domain.KnowledgeBase{ID: "kb", TenantID: 42, UserID: 9, Status: 1, IsPublic: true}, want: 404},
		{name: "KB DB failure", tenant: int64(41), user: int64(9), kbID: "kb", kbErr: errors.New("private database details"), want: 500},
		{name: "Neo4j failure", tenant: int64(41), user: int64(9), kbID: "kb", kb: &domain.KnowledgeBase{ID: "kb", TenantID: 41, UserID: 9, Status: 1}, graphErr: errors.New("private graph details"), want: 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &graphStatsAPIRepo{err: tc.graphErr}
			svc := knowledgeapp.NewGraphService(repo, nil)
			svc.SetKnowledgeBaseRepository(&graphStatsAPIKB{kb: tc.kb, err: tc.kbErr})
			w := graphStatsAPIRequest(graphStatsAPIRouter(svc, tc.tenant, tc.user, false), tc.kbID)
			require.Equal(t, tc.want, w.Code, w.Body.String())
			require.NotContains(t, w.Body.String(), "total_relations")
			require.NotContains(t, w.Body.String(), "private database details")
			require.NotContains(t, w.Body.String(), "private graph details")
			if tc.graphErr == nil {
				require.Zero(t, repo.calls)
			}
		})
	}
	w := graphStatsAPIRequest(graphStatsAPIRouter(knowledgeapp.NewGraphService(nil, nil), int64(41), int64(9), true), "kb")
	require.Equal(t, 503, w.Code)
}

func TestGraphRelationStatsHTTPVerifiedScopeAndDTO(t *testing.T) {
	repo := &graphStatsAPIRepo{}
	svc := knowledgeapp.NewGraphService(repo, nil)
	svc.SetKnowledgeBaseRepository(&graphStatsAPIKB{kb: &domain.KnowledgeBase{ID: "kb", TenantID: 41, UserID: 9, Status: 1}})
	w := graphStatsAPIRequest(graphStatsAPIRouter(svc, int64(41), int64(9), true), "kb")
	require.Equal(t, 200, w.Code, w.Body.String())
	var envelope struct {
		Code      int                           `json:"code"`
		Data      knowledgeapp.RelationStatsDTO `json:"data"`
		RequestID string                        `json:"request_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)
	require.Equal(t, "relation-stats-test", envelope.RequestID)
	require.Equal(t, 3, envelope.Data.TotalRelations)
	require.Equal(t, 2.5, envelope.Data.AverageStrength)
	require.Equal(t, map[string]int{"2": 3}, envelope.Data.StrengthDistribution)
	require.Equal(t, domain.NameSpace{TenantID: "41", KnowledgeBaseID: "kb"}, repo.ns)
}

func TestGraphRelationStatsHTTPWriteRequiresOwner(t *testing.T) {
	for _, tc := range []struct {
		name              string
		tenant, user      any
		kbTenant, kbOwner int64
		public            bool
		want              int
	}{
		{name: "owner", tenant: int64(41), user: int64(9), kbTenant: 41, kbOwner: 9, want: 200},
		{name: "missing identity", kbTenant: 41, kbOwner: 9, want: 401},
		{name: "string identity", tenant: "41", user: "9", kbTenant: 41, kbOwner: 9, want: 401},
		{name: "other private owner", tenant: int64(41), user: int64(9), kbTenant: 41, kbOwner: 10, want: 403},
		{name: "public is not a write grant", tenant: int64(41), user: int64(9), kbTenant: 41, kbOwner: 10, public: true, want: 403},
		{name: "cross tenant", tenant: int64(41), user: int64(9), kbTenant: 42, kbOwner: 9, public: true, want: 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &graphStatsAPIRepo{}
			svc := knowledgeapp.NewGraphService(repo, nil)
			svc.SetKnowledgeBaseRepository(&graphStatsAPIKB{kb: &domain.KnowledgeBase{ID: "kb", TenantID: tc.kbTenant, UserID: tc.kbOwner, Status: 1, IsPublic: tc.public}})
			w := graphStatsAPICreateRelation(graphStatsAPIRouter(svc, tc.tenant, tc.user, false), "kb", "A")
			require.Equal(t, tc.want, w.Code, w.Body.String())
			if tc.want == 200 {
				require.Equal(t, 1, repo.writes)
				require.Equal(t, domain.NameSpace{TenantID: "41", KnowledgeBaseID: "kb"}, repo.ns)
			} else {
				require.Zero(t, repo.writes)
			}
		})
	}
}
