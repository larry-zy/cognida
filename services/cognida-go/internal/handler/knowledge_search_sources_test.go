package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cognida/internal/model/rag"
	knowledge "cognida/internal/service/knowledge"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type sourcesKBService struct{ knowledge.KnowledgeBaseService }

func (sourcesKBService) FilterAccessibleKBIDs(context.Context, []string, int64) []string {
	return []string{"allowed"}
}

type sourcesRetriever struct {
	rag.Retriever
	sources []string
}

func (r sourcesRetriever) VectorRetrieve(context.Context, string, string, string, *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	return &rag.RetrieveResponse{Results: []*rag.Document{{ChunkID: "chunk", KnowledgeID: "doc", KnowledgeBaseID: "allowed", Content: "proof", Score: 1, Sources: r.sources}}, Warnings: []string{"fixture warning"}}, nil
}
func TestKnowledgeSearchExposesActualSourcesAndWarnings(t *testing.T) {
	for _, sources := range [][]string{{"milvus_vector"}, {"database_lexical"}, nil} {
		h := NewKnowledgeBaseHandler(sourcesKBService{}, nil, knowledge.NewRetrievalCapability(sourcesRetriever{sources: sources}, nil), nil)
		router := gin.New()
		router.Use(func(c *gin.Context) { c.Set("tenant_id", int64(1)); c.Next() })
		router.POST("/search", h.SearchKnowledge)
		req := httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(`{"kb_ids":["allowed"],"query":"proof","retrieval_mode":"vector"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var body struct {
			Data struct {
				Items []struct {
					Sources []string `json:"sources"`
				}
				Warnings []string
			}
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body.Data.Items, 1)
		require.Equal(t, append([]string{}, sources...), body.Data.Items[0].Sources)
		require.Equal(t, []string{"fixture warning"}, body.Data.Warnings)
	}
}
