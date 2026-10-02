//go:build integration

package handler

import (
	"bytes"
	d "cognida/internal/model/evaluation"
	repo "cognida/internal/repository/mysql"
	svc "cognida/internal/service/evaluation"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestEvaluationComparisonHTTPRealDB(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("MYSQL_DSN")
	require.NotEmpty(t, dsn)
	db, e := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	t.Cleanup(func() { sql.Close() })
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	ids := []string{uuid.NewString(), uuid.NewString()}
	tr := repo.NewEvaluationTaskRepository(db)
	rr := repo.NewEvaluationResultRepository(db)
	t.Cleanup(func() {
		for _, id := range ids {
			require.NoError(t, rr.DeleteByTaskID(ctx, id))
			require.NoError(t, db.Exec("DELETE FROM evaluation_tasks WHERE id=?", id).Error)
		}
	})
	for i, id := range ids {
		task := d.NewEvaluationTask(id, tenant, 1, "same-dataset", d.EvaluationTypeAgent, 2)
		task.Status = d.TaskStatusCompleted
		task.SuccessCount = 2
		require.NoError(t, tr.Create(ctx, task))
		for _, q := range []string{"a", "b"} {
			require.NoError(t, rr.Create(ctx, &d.EvaluationResult{TaskID: id, Question: q, ReferenceAnswer: "reference", Success: true, Scores: map[string]float64{"answer_accuracy": .4 + float64(i)*.4}, CreatedAt: time.Now()}))
		}
	}
	h := NewEvaluationHandler(svc.NewService(nil, tr, rr, nil, nil, nil), nil, nil, nil)
	api := gin.New()
	api.Use(func(c *gin.Context) {
		who := tenant
		if c.GetHeader("X-Other") == "true" {
			who++
		}
		c.Set("tenant_id", who)
		c.Set("user_id", int64(1))
	})
	api.POST("/evaluation/compare", h.Compare)
	call := func(other string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(svc.ComparisonRequest{BaselineID: ids[0], CandidateID: ids[1], Metric: "answer_accuracy", HigherIsBetter: true, MinPairs: 2})
		r := httptest.NewRequest("POST", "/evaluation/compare", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Other", other)
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		return w
	}
	w := call("")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "eligible_for_review")
	require.Equal(t, 404, call("true").Code)
}
