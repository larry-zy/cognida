//go:build integration

package wire

import (
	"bytes"
	"cognida/internal/handler"
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/governance"
	"cognida/internal/service/knowledge"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

type suggestionProcessor struct {
	knowledge.DocumentProcessorService
	calls int
	key   string
}

func (p *suggestionProcessor) ProcessDocument(_ context.Context, tenant, user int64, in *knowledge.ProcessDocumentRequest) (*knowledge.ProcessDocumentResponse, error) {
	p.calls++
	p.key = in.IdempotencyKey
	return &knowledge.ProcessDocumentResponse{KnowledgeID: "durable-knowledge", ParseStatus: "completed"}, nil
}
func TestSuggestionIntegrationDiscoveryReviewAndAdoption(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	require.NotEmpty(t, dsn)
	db, e := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	p := l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 991}
	ctx := context.Background()
	modelID, metricID := uuid.NewString(), uuid.NewString()
	sessionID, messageID := uuid.NewString(), uuid.NewString()
	taskID := uuid.NewString()
	t.Cleanup(func() {
		require.NoError(t, db.Table("agent_semantic_metrics").Where("id=?", metricID).Delete(map[string]any{}).Error)
		require.NoError(t, db.Table("messages").Where("session_id=?", sessionID).Delete(map[string]any{}).Error)
		require.NoError(t, db.Table("evaluation_qa_results").Where("task_id=?", taskID).Delete(map[string]any{}).Error)
		for _, table := range []string{"governance_records", "agent_semantic_coverage_logs", "agent_semantic_models", "agent_experiences", "sessions", "evaluation_tasks"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sql.Close())
	})
	now := time.Now().UTC()
	require.NoError(t, db.Create(&mysql.SemanticModelModel{ID: modelID, TenantID: p.TenantID, Name: "sales", Version: 1, Status: "active", CreatedAt: now, UpdatedAt: now}).Error)
	require.NoError(t, db.Create(&mysql.SemanticMetricModel{ID: metricID, ModelID: modelID, Name: "revenue", Expr: "SUM(sales)", Synonyms: []byte(`[]`), CreatedAt: now, UpdatedAt: now}).Error)
	coverage := &mysql.SemanticCoverageLogModel{TenantID: p.TenantID, Model: "sales", RequestID: uuid.NewString(), Outcome: "fallback", Uncovered: "revenue total", CreatedAt: now}
	require.NoError(t, db.Create(coverage).Error)
	require.NoError(t, db.Create(&mysql.SessionModel{ID: sessionID, TenantID: p.TenantID, UserID: p.UserID, Title: "test", AgentType: "data_analysis", Status: 1}).Error)
	require.NoError(t, db.Create(&mysql.MessageModel{ID: uuid.NewString(), SessionID: sessionID, KnowledgeReferences: "[]", AgentSteps: "[]", ToolCalls: "[]", Role: "user", Content: "How to verify revenue?", IsCompleted: true, CreatedAt: now.Add(-time.Second)}).Error)
	require.NoError(t, db.Create(&mysql.MessageModel{ID: messageID, SessionID: sessionID, KnowledgeReferences: "[]", AgentSteps: "[]", ToolCalls: "[]", Role: "assistant", Content: "Check the source and reconcile totals.", IsCompleted: true, CreatedAt: now}).Error)
	require.NoError(t, db.Create(&mysql.EvaluationTaskModel{ID: taskID, TenantID: p.TenantID, UserID: p.UserID, DatasetID: uuid.NewString(), Type: "agent", Status: "completed", CreatedAt: now, UpdatedAt: now}).Error)
	evaluation := &mysql.EvaluationResultModel{TaskID: taskID, Question: "Correct answer?", ReferenceAnswer: "Reference evidence", GeneratedAnswer: "wrong", RetrievedPIDs: "[]", RelevantPIDs: "[]", Success: false, CreatedAt: now}
	require.NoError(t, db.Create(evaluation).Error)
	processor := &suggestionProcessor{}
	validate, apply, discover := NewSuggestionHooks(db, processor)
	s := governance.NewService(mysql.NewGovernanceRepository(db), nil, nil, nil, func(_ context.Context, p l.Principal, scope string) error {
		if scope == "admin" && p.UserID != 991 {
			return d.ErrForbidden
		}
		return p.Validate()
	})
	s.ValidateSource = validate
	s.Apply = apply
	s.DiscoverSource = discover
	require.ErrorIs(t, validate(ctx, l.Principal{TenantID: p.TenantID, UserID: 123}, d.SuggestionSource{Type: "conversation_insight", ID: messageID}), d.ErrForbidden)
	require.ErrorIs(t, validate(ctx, l.Principal{TenantID: p.TenantID + 1, UserID: p.UserID}, d.SuggestionSource{Type: "uncovered_term", ID: strconv.FormatInt(coverage.ID, 10)}), d.ErrNotFound)
	items, e := s.DiscoverSuggestions(ctx, p, []string{"coverage", "conversation", "evaluation"}, 20)
	require.NoError(t, e)
	require.Len(t, items, 3)
	again, e := s.DiscoverSuggestions(ctx, p, []string{"coverage", "conversation", "evaluation"}, 20)
	require.NoError(t, e)
	require.Len(t, again, 3)
	for i := range items {
		require.Equal(t, items[i].ID, again[i].ID)
	}
	for _, item := range items {
		require.ErrorIs(t, func() error {
			_, e := s.ReviewSuggestion(ctx, l.Principal{TenantID: p.TenantID, UserID: 123}, item.ID, "approve", "", item.Version)
			return e
		}(), d.ErrForbidden)
		_, e = s.ReviewSuggestion(ctx, p, item.ID, "approve", "", item.Version+1)
		require.ErrorIs(t, e, d.ErrConflict)
		applied, e := s.ReviewSuggestion(ctx, p, item.ID, "approve", "verified", item.Version)
		require.NoError(t, e)
		require.Equal(t, "applied", applied.Status)
		replay, e := s.ReviewSuggestion(ctx, p, item.ID, "approve", "retry", item.Version)
		require.NoError(t, e)
		require.JSONEq(t, string(applied.Payload), string(replay.Payload))
	}
	var metric mysql.SemanticMetricModel
	require.NoError(t, db.Where("id=?", metricID).Take(&metric).Error)
	require.JSONEq(t, `["revenue total"]`, string(metric.Synonyms))
	var model mysql.SemanticModelModel
	require.NoError(t, db.Where("id=?", modelID).Take(&model).Error)
	require.Equal(t, 2, model.Version)
	var expCount int64
	require.NoError(t, db.Table("agent_experiences").Where("tenant_id=?", p.TenantID).Count(&expCount).Error)
	require.EqualValues(t, 2, expCount)
	proposal, _ := json.Marshal(map[string]any{"knowledge_base_id": uuid.NewString(), "title": "Reviewed fact", "content": "Approved source material"})
	created, e := s.CreateSuggestion(ctx, p, d.Suggestion{Source: d.SuggestionSource{Type: "evaluation_feedback", ID: strconv.FormatInt(evaluation.ID, 10)}, Kind: "knowledge", Title: "Review knowledge", Content: "evidence", Proposal: proposal}, "knowledge")
	require.NoError(t, e)
	applied, e := s.ReviewSuggestion(ctx, p, created.ID, "approved", "", created.Version)
	require.NoError(t, e)
	require.Equal(t, "applied", applied.Status)
	_, e = s.ReviewSuggestion(ctx, p, created.ID, "approve", "", created.Version)
	require.NoError(t, e)
	require.Equal(t, 1, processor.calls)
	require.Equal(t, "suggestion:"+created.ID, processor.key)
	g := gin.New()
	g.Use(func(c *gin.Context) { c.Set("tenant_id", p.TenantID); c.Set("user_id", p.UserID); c.Next() })
	handler.NewGovernanceHandler(s).Register(g.Group("/api"))
	request := func(path string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "http-suggestion")
		out := httptest.NewRecorder()
		g.ServeHTTP(out, req)
		return out
	}
	response := request("/api/improvement-suggestions/discover", map[string]any{"source_types": []string{"conversation"}, "limit": 10})
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), messageID)
	response = request("/api/improvement-suggestions", d.Suggestion{Source: d.SuggestionSource{Type: "evaluation_feedback", ID: strconv.FormatInt(evaluation.ID, 10)}, Kind: "knowledge", Title: "HTTP suggestion", Content: "review", Proposal: proposal})
	require.Equal(t, 200, response.Code, response.Body.String())
	var envelope struct {
		Data struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	response = request("/api/improvement-suggestions/"+envelope.Data.ID+"/review", map[string]any{"decision": "reject", "expected_version": envelope.Data.Version})
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "rejected")

}
