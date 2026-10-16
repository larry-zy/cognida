package handler

import (
	k "cognida/internal/model/knowledge"
	l "cognida/internal/model/lifecycle"
	app "cognida/internal/service/knowledge"
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

type knowledgeReadFixture struct{ app.KnowledgeBaseService }

func (knowledgeReadFixture) GetKnowledgeDetail(context.Context, string, int64, string) (*k.Knowledge, error) {
	return &k.Knowledge{ID: "retired", KnowledgeBaseID: "kb", Title: "private old content"}, nil
}
func (knowledgeReadFixture) GetKnowledgeStatus(ctx context.Context, kb string, tenant int64, id string) (*k.Knowledge, error) {
	return (knowledgeReadFixture{}).GetKnowledgeDetail(ctx, kb, tenant, id)
}
func (knowledgeReadFixture) GetChunkDetail(context.Context, string, int64, string) (*k.Chunk, error) {
	return &k.Chunk{ID: "chunk", KnowledgeID: "retired", KnowledgeBaseID: "kb", Content: "private old content"}, nil
}
func (knowledgeReadFixture) GetChunks(context.Context, string, int64, int, int, string) ([]*k.Chunk, int64, error) {
	return []*k.Chunk{{ID: "chunk", KnowledgeID: "retired", KnowledgeBaseID: "kb", Content: "private old content"}}, 1, nil
}

func TestKnowledgeDirectReadsRespectPublicationGuard(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		h := NewKnowledgeBaseHandler(knowledgeReadFixture{}, nil, nil, nil)
		h.SetKnowledgeConsumptionGuard(func(_ context.Context, p l.Principal, kb, id string) (bool, bool, error) {
			require.Equal(t, l.Principal{TenantID: 3, UserID: 5}, p)
			require.Equal(t, "kb", kb)
			require.Equal(t, "retired", id)
			if unavailable {
				return true, false, errors.New("publication state unavailable")
			}
			return true, false, nil
		})
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("tenant_id", int64(3)); c.Set("user_id", int64(5)); c.Next() })
		r.GET("/:id/doc/:kid", h.GetKnowledgeDetail)
		r.GET("/:id/status/:kid", h.GetKnowledgeStatus)
		r.GET("/:id/chunk/:cid", h.GetChunkDetail)
		r.GET("/:id/chunks", h.GetChunks)
		for _, path := range []string{"/kb/doc/retired", "/kb/status/retired", "/kb/chunk/chunk", "/kb/chunks"} {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			require.NotContains(t, w.Body.String(), "private old content")
			if path != "/kb/chunks" {
				require.Equal(t, 404, w.Code)
			} else if unavailable {
				require.Equal(t, 500, w.Code)
			} else {
				require.Equal(t, 200, w.Code)
				require.Contains(t, w.Body.String(), `"items":[]`)
			}
		}
	}
}
