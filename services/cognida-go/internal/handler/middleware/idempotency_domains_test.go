package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDurableDomainReplayRechecksRevocation(t *testing.T) {
	for _, path := range []string{"/api/v1/feature-jobs", "/api/v1/drift/compare", "/api/v1/annotation-tasks/t/preannotate", "/api/v1/tagging-results/r/apply", "/api/v1/evaluation/dataset-versions", "/api/v1/research/runs", "/api/v1/collection-tasks", "/api/v1/lifecycle/assets", "/api/v1/query-jobs", "/api/v1/feedback-cases", "/api/v1/feedback-publications/p/approve", "/api/v1/quality/evaluate/datasource"} {
		t.Run(path, func(t *testing.T) {
			r := gin.New()
			r.Use(NewIdempotency(nil).Apply())
			allowed := true
			r.POST(path, func(c *gin.Context) {
				if !allowed {
					c.AbortWithStatus(403)
					return
				}
				c.JSON(200, gin.H{"private": "content"})
			})
			call := func() *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, path, nil)
				req.Header.Set(idemHeader, "same")
				r.ServeHTTP(w, req)
				return w
			}
			require.Equal(t, 200, call().Code)
			allowed = false
			w := call()
			require.Equal(t, 403, w.Code)
			require.NotContains(t, w.Body.String(), "content")
			require.Empty(t, w.Header().Get("Idempotency-Replayed"))
		})
	}
}
