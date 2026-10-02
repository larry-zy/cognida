package handler

import (
	domain "cognida/internal/model/collection"
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

type collectionReadRepo struct {
	domain.Repository
	runErr error
}

func (r collectionReadRepo) FindTask(context.Context, int64, string) (*domain.Task, error) {
	return &domain.Task{ID: "task", TenantID: 1}, nil
}
func (r collectionReadRepo) ListSources(context.Context, int64, string) ([]*domain.Source, error) {
	return []*domain.Source{}, nil
}
func (r collectionReadRepo) FindLatestRun(context.Context, int64, string) (*domain.Run, error) {
	return nil, r.runErr
}
func TestCollectionGetDistinguishesAbsentRunAndDatabaseFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{{"no run", domain.ErrRunNotFound, http.StatusOK}, {"database failure", errors.New("database unavailable"), http.StatusInternalServerError}} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("tenant_id", int64(1)); c.Next() })
			h := NewCollectionHandler(nil, collectionReadRepo{runErr: tc.err}, nil)
			router.GET("/collection/:id", h.Get)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/collection/task", nil))
			require.Equal(t, tc.status, response.Code)
		})
	}
}
