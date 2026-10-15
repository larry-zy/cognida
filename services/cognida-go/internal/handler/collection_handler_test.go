package handler

import (
	domain "cognida/internal/model/collection"
	kbmodel "cognida/internal/model/knowledge"
	knowledge "cognida/internal/service/knowledge"
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type collectionReadRepo struct {
	domain.Repository
	runErr error
}

type collectionTargetKBService struct {
	knowledge.KnowledgeBaseService
	kb *kbmodel.KnowledgeBase
}

func (s collectionTargetKBService) FindByID(context.Context, string, int64) (*kbmodel.KnowledgeBase, error) {
	return s.kb, nil
}

func TestCollectionCreateRejectsUnavailableKnowledgeBase(t *testing.T) {
	now := time.Now()
	for _, kb := range []*kbmodel.KnowledgeBase{
		nil,
		{ID: "kb", TenantID: 1, Status: 0},
		{ID: "kb", TenantID: 1, Status: 1, DeletedAt: &now},
		{ID: "kb", TenantID: 2, Status: 1},
	} {
		router := gin.New()
		router.Use(func(c *gin.Context) { c.Set("tenant_id", int64(1)); c.Set("user_id", int64(2)); c.Next() })
		h := NewCollectionHandler(nil, nil, collectionTargetKBService{kb: kb})
		router.POST("/collection", h.Create)
		request := httptest.NewRequest(http.MethodPost, "/collection", strings.NewReader(`{"knowledge_base_id":"kb","sources":["https://example.com"]}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "create-fixture")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusNotFound, response.Code, "unavailable target must fail before creating a task")
	}
}

type collectionOutcomeRepo struct {
	collectionReadRepo
	status domain.RunStatus
	items  []*domain.Item
}

func (r collectionOutcomeRepo) FindLatestRun(context.Context, int64, string) (*domain.Run, error) {
	return &domain.Run{ID: "run", Status: r.status}, nil
}
func (r collectionOutcomeRepo) ListItems(context.Context, int64, string) ([]*domain.Item, error) {
	return r.items, nil
}
func (r collectionOutcomeRepo) ListSources(context.Context, int64, string) ([]*domain.Source, error) {
	return make([]*domain.Source, 2), nil
}
func TestCollectionGetExposesPartialFailureAndDuplicateOutcome(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  domain.RunStatus
		items   []*domain.Item
		outcome string
		usable  int
	}{
		{"partial", domain.RunStatusCompleted, []*domain.Item{{Status: domain.ItemStatusAccepted, KnowledgeID: "new"}, {Status: domain.ItemStatusFailed}}, "partial", 1},
		{"unchanged", domain.RunStatusCompleted, []*domain.Item{{Status: domain.ItemStatusSkipped, KnowledgeID: "old"}, {Status: domain.ItemStatusSkipped, KnowledgeID: "old"}}, "success", 2},
		{"failed", domain.RunStatusFailed, []*domain.Item{{Status: domain.ItemStatusFailed}, {Status: domain.ItemStatusRejected}}, "failed", 0},
		{"cancelled", domain.RunStatusCancelled, []*domain.Item{{Status: domain.ItemStatusAccepted, KnowledgeID: "new"}}, "cancelled", 1},
		{"running", domain.RunStatusRunning, nil, "running", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("tenant_id", int64(1)); c.Next() })
			h := NewCollectionHandler(nil, collectionOutcomeRepo{status: tc.status, items: tc.items}, nil)
			router.GET("/collection/:id", h.Get)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/collection/task", nil))
			require.Equal(t, http.StatusOK, response.Code)
			var body struct {
				Data struct {
					Summary domain.OutcomeSummary `json:"summary"`
					Items   []*domain.Item        `json:"items"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			require.Equal(t, tc.outcome, body.Data.Summary.Outcome)
			require.Equal(t, tc.usable, body.Data.Summary.Usable)
			require.Equal(t, tc.items, body.Data.Items, "usable and failed source receipts remain inspectable")
		})
	}
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
