//go:build integration

package evaluationadapter

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	objects "cognida/internal/infrastructure/objectstore"
	e "cognida/internal/model/evaluation"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	repo "cognida/internal/repository/mysql"
	"cognida/internal/service/evaluation"
	"cognida/internal/service/lifecycle"
	"github.com/gin-gonic/gin"
	sqlmysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
)

// Real migrated MySQL and local object storage; the bounded grader and delivery
// queue below are explicit test substitutes, not live Python/Redis acceptance.
type testQueue struct {
	job.Queue
	mu   sync.Mutex
	jobs map[string]job.Job
}

func (q *testQueue) Enqueue(_ context.Context, j job.Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs[j.ID] = j
	return nil
}

type testScorer struct {
	calls atomic.Int32
	rows  atomic.Int32
}

func (s *testScorer) Score(_ context.Context, items []e.OfflineItem) ([]e.OfflineScore, error) {
	s.calls.Add(1)
	s.rows.Add(int32(len(items)))
	out := make([]e.OfflineScore, len(items))
	for i, v := range items {
		out[i].Index = i
		if v.ReferenceAnswer == v.GeneratedAnswer {
			out[i].ExactMatch = 100
		}
		if strings.Contains(v.GeneratedAnswer, v.ReferenceAnswer) {
			out[i].ContainsMatch = 100
		}
	}
	return out, nil
}
func TestOfflineIntegrationSnapshotRecoveryReceiptAndHTTP(t *testing.T) {
	dsn := os.Getenv("COGNIDA_TASKCONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated DSN required")
	}
	config, err := sqlmysql.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(config.DBName, "cognida_review_"))
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	p := l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 917}
	ctx := context.Background()
	var revoked atomic.Bool
	t.Cleanup(func() {
		require.NoError(t, db.Table("evaluation_qa_results").Where("task_id IN (?)", db.Table("evaluation_tasks").Select("id").Where("tenant_id=?", p.TenantID)).Delete(map[string]any{}).Error)
		for _, table := range []string{"evaluation_offline_runs", "evaluation_tasks", "lifecycle_assets"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sql.Close())
	})
	store, err := objects.NewLocal(t.TempDir())
	require.NoError(t, err)
	q := &testQueue{jobs: map[string]job.Job{}}
	scorer := &testScorer{}
	var service *evaluation.OfflineService
	assets, err := lifecycle.NewService(repo.NewLifecycleRepository(db), store, q, lifecycle.Config{Authorize: func(_ context.Context, who l.Principal, _ string) error {
		if who != p || revoked.Load() {
			return l.ErrForbidden
		}
		return nil
	}, ValidateGovernedOrigin: func(ctx context.Context, who l.Principal, ref l.DataAssetRef) error {
		return service.ValidateGovernedOrigin(ctx, who, ref)
	}})
	require.NoError(t, err)
	repository := repo.NewOfflineEvaluationRepository(db)
	service, err = evaluation.NewOfflineService(repository, assets, q, scorer)
	require.NoError(t, err)
	adapter, err := New(service)
	require.NoError(t, err)
	rows := make([][]any, 21)
	for i := range rows {
		rows[i] = []any{"question", "00123", "00123"}
	}
	input, err := assets.ImportWithKey(ctx, p, l.Input{Kind: "table", Columns: []string{"question", "reference", "answer"}, Rows: rows}, "review existing answers", uuid.NewString())
	require.NoError(t, err)
	cmd := command()
	cmd.Scope = d.Scope{TenantID: p.TenantID, UserID: p.UserID}
	cmd.InputRefs = []l.RefID{input.RefID}
	cmd.Node.Inputs[0].Asset = &input.RefID
	bind(&cmd)
	handle, err := adapter.Submit(ctx, cmd)
	require.NoError(t, err)
	require.NotNil(t, handle)
	// MySQL stores JSON with its own key order and whitespace. Recovery reads
	// that representation rather than the compact in-memory submission receipt.
	var persisted struct{ Receipt json.RawMessage }
	require.NoError(t, db.Raw("SELECT CAST(? AS JSON) AS receipt", string(handle.Receipt)).Scan(&persisted).Error)
	handle.Receipt = persisted.Receipt
	req, err := adapter.request(ctx, cmd)
	require.NoError(t, err)
	scope := scope(cmd.Scope)
	// Commit the first batch, then release an interrupted worker. Recovery scores
	// exactly the remaining sample; already committed scores are not regenerated.
	run, err := repository.Claim(ctx, scope, handle.DomainID, time.Second*30)
	require.NoError(t, err)
	first := make([]e.OfflineScore, 20)
	for i := range first {
		first[i] = e.OfflineScore{Index: i, ExactMatch: 100, ContainsMatch: 100}
	}
	_, err = repository.Checkpoint(ctx, scope, run.ID, run.Epoch, 0, first)
	require.NoError(t, err)
	require.NoError(t, repository.Release(ctx, scope, run.ID, run.Epoch))
	jobs, err := service.PendingJobs(ctx, 100)
	require.NoError(t, err)
	var own job.Job
	for _, j := range jobs {
		if j.ID == "evaluation-offline:"+run.ID {
			own = j
		}
	}
	require.NotEmpty(t, own.ID)
	require.NoError(t, service.Handle(ctx, own))
	observed, err := adapter.Inspect(ctx, cmd, *handle)
	require.NoError(t, err)
	require.Equal(t, d.DomainSucceeded, observed.Status)
	require.Len(t, observed.OutputRefs, 1)
	require.EqualValues(t, 1, scorer.rows.Load())
	_, body, err := assets.Resolve(ctx, p.TenantID, p.UserID, observed.OutputRefs[0])
	require.NoError(t, err)
	require.Contains(t, string(body), `"model_calls":0`)
	require.Contains(t, string(body), `"generated_answer":"00123"`)
	var report struct {
		Scored int `json:"scored_count"`
	}
	require.NoError(t, json.Unmarshal(body, &report))
	require.Equal(t, 21, report.Scored)
	again, err := adapter.Submit(ctx, cmd)
	require.NoError(t, err)
	require.Equal(t, handle.DomainID, again.DomainID)
	require.NoError(t, service.Handle(ctx, own))
	require.EqualValues(t, 1, scorer.calls.Load())
	// Existing real evaluation HTTP reads the real result projection and uses the
	// offline source/owner authorizer before exposing answer text.
	reads := evaluation.NewService(nil, repo.NewEvaluationTaskRepository(db), repo.NewEvaluationResultRepository(db), nil, nil, nil).WithOfflineReadAuthorizer(service.AuthorizeRead)
	h := handler.NewEvaluationHandler(reads, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		user := p.UserID
		if c.GetHeader("X-Test-Other") == "1" {
			user++
		}
		c.Set("tenant_id", p.TenantID)
		c.Set("user_id", user)
		c.Next()
	})
	router.GET("/evaluation/tasks/:id", h.GetTask)
	router.GET("/evaluation/results/:task_id", h.GetResults)
	for _, path := range []string{"/evaluation/tasks/" + run.ID, "/evaluation/results/" + run.ID} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		require.Equal(t, 200, response.Code, response.Body.String())
		other := httptest.NewRequest("GET", path, nil)
		other.Header.Set("X-Test-Other", "1")
		response = httptest.NewRecorder()
		router.ServeHTTP(response, other)
		require.True(t, response.Code == 403 || response.Code == 404, response.Body.String())
	}
	revoked.Store(true)
	_, err = adapter.Inspect(ctx, cmd, *handle)
	require.ErrorIs(t, err, d.ErrForbidden)
	stopped, err := adapter.Cancel(ctx, cmd, *handle)
	require.NoError(t, err)
	require.Equal(t, d.DomainSucceeded, stopped.Status)
	require.Empty(t, stopped.OutputRefs)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/evaluation/results/"+run.ID, nil))
	require.Equal(t, 403, response.Code)
	revoked.Store(false)
	// An unknown submission cancellation persists a tombstone. A late Submit with
	// the same complete binding cannot enqueue or execute a new evaluation.
	cancelled := command()
	cancelled.Scope = cmd.Scope
	cancelled.InputRefs = cmd.InputRefs
	cancelled.Node.Inputs[0].Asset = &input.RefID
	bind(&cancelled)
	tombstone, err := adapter.CancelSubmission(ctx, cancelled)
	require.NoError(t, err)
	late, err := adapter.Submit(ctx, cancelled)
	require.NoError(t, err)
	require.Equal(t, tombstone.DomainID, late.DomainID)
	state, err := adapter.Inspect(ctx, cancelled, *late)
	require.NoError(t, err)
	require.Equal(t, d.DomainCancelled, state.Status)
	// Snapshot/hash mismatch and foreign owners never overwrite the real run.
	wrong := req
	wrong.PayloadHash = strings.Repeat("b", 64)
	_, err = service.Lookup(ctx, scope, wrong)
	require.ErrorIs(t, err, e.ErrOfflineConflict)
	_, err = repository.Get(ctx, e.OfflineScope{TenantID: p.TenantID, UserID: p.UserID + 1}, run.ID)
	require.ErrorIs(t, err, e.ErrTaskNotFound)
}
