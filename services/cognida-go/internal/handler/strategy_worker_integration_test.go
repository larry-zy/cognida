//go:build integration

package handler

import (
	"cognida/internal/infrastructure/job"
	agentctx "cognida/internal/model/agent"
	d "cognida/internal/model/evaluation"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/model/rag"
	policy "cognida/internal/model/strategy"
	repo "cognida/internal/repository/mysql"
	ev "cognida/internal/service/evaluation"
	"cognida/internal/service/evaluation/executor"
	lifecycle "cognida/internal/service/lifecycle"
	strategies "cognida/internal/service/strategy"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type strategyProgress struct{}

func (strategyProgress) SetProgress(context.Context, string, *d.Progress) error { return nil }
func (strategyProgress) SetError(context.Context, string, string, int) error    { return nil }

type strategyModel struct {
	rag.LLMChat
	questions []string
}

func (m *strategyModel) Chat(_ context.Context, messages []rag.LLMMessage, _ *rag.ChatOptions) (*rag.LLMResponse, error) {
	m.questions = append(m.questions, messages[len(messages)-1].Content)
	time.Sleep(20 * time.Millisecond)
	return &rag.LLMResponse{Content: "42"}, nil
}

// Production MySQL repositories, evaluation worker/executor/HTTP clients. The
// model and Python endpoints are deterministic local fixtures, not live providers.
func TestStrategyMySQLActualWorkerConsumesFixedCleaningSnapshot(t *testing.T) {
	f := newReviewedFixture(t)
	ctx := context.Background()
	t.Cleanup(func() {
		for _, table := range []string{"strategy_events", "strategy_versions", "strategy_heads"} {
			require.NoError(t, f.db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
	})
	f.assets.input.Rows = [][]any{{"  How   much?  ", "42"}}
	task, item, revision := f.approvedQA(t)
	version, e := f.vs.Publish(ctx, f.p, d.PublishDatasetVersion{DatasetID: "policy-" + uuid.NewString(), Name: "fixed", Profile: "qa", Samples: []d.SampleSelection{{TaskID: task, ItemID: item, ExpectedVersion: revision}}}, "dataset")
	require.NoError(t, e)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/profile") {
			_ = json.NewEncoder(w).Encode(lifecycle.ComputeResult{SchemaVersion: "1", Method: "fixture-profile", Result: map[string]any{"overall_score": 95}})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/clean") {
			var input struct {
				Input   l.Input        `json:"input"`
				Options map[string]any `json:"options"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
			for key := range input.Options {
				require.Contains(t, []string{"operations", "keep", "columns"}, key)
			}
			for _, op := range input.Options["operations"].([]any) {
				switch op {
				case "trim":
					input.Input.Text = strings.TrimSpace(input.Input.Text)
				case "normalize_ws":
					input.Input.Text = strings.Join(strings.Fields(input.Input.Text), " ")
				}
			}
			_ = json.NewEncoder(w).Encode(lifecycle.ComputeResult{SchemaVersion: "1", Method: "fixture-clean", Result: map[string]any{"data": input.Input}})
			return
		}
		var request ev.ComputeMetricsRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		items := []*ev.ComputeItemResult{}
		for i, item := range request.Items {
			score := 0.
			if item.GeneratedAnswer == item.ReferenceAnswer {
				score = 1
			}
			items = append(items, &ev.ComputeItemResult{Index: i, Scores: map[string]float64{"rouge_l": score}})
		}
		_ = json.NewEncoder(w).Encode(ev.ComputeMetricsResponse{Items: items, Aggregate: map[string]float64{"rouge_l": 1}})
	}))
	defer endpoint.Close()
	compute := lifecycle.NewHTTPCompute(endpoint.URL, "fixture-token")
	if url := os.Getenv("COGNIDA_STRATEGY_PYTHON_URL"); url != "" {
		require.NotEmpty(t, os.Getenv("INTERNAL_HTTP_AUTH_TOKEN"))
		compute = lifecycle.NewHTTPCompute(url, os.Getenv("INTERNAL_HTTP_AUTH_TOKEN"))
		t.Log("Cleaning and quality profile use the explicitly configured real Python service; model and graders remain deterministic fixtures")
	}
	s := strategies.NewService(repo.NewStrategyRepository(f.db), f.assets, func(_ context.Context, p l.Principal, _ string) error { return p.Validate() }, map[string]strategies.Consumer{"cleaning": {Name: "lifecycle.cleaning.v1", Baseline: lifecycle.DefaultCleaningStrategy}})
	base, e := s.RegisterBaseline(ctx, f.p, "cleaning.default", "cleaning", "baseline")
	require.NoError(t, e)
	candidate, e := s.CreateCandidate(ctx, f.p, "cleaning.default", strategies.CandidateInput{Kind: "cleaning", BaselineID: base.ID, Body: json.RawMessage(`{"operations":["trim","normalize_ws"],"keep":"first","min_score":70}`), EvidenceRefs: []string{f.assets.ref.AssetRefID}}, "candidate")
	require.NoError(t, e)
	ss := ev.NewSnapshotService(f.vs, nil, func(context.Context, l.Principal, *ev.EvaluationTaskConfig) (json.RawMessage, error) {
		return json.RawMessage(`{"model":"fixture-v1","grader":"fixture-v1"}`), nil
	}).WithStrategies(s, compute)
	f.es.WithSnapshots(ss)
	s.WithRegressionReader(f.es.StrategyRegression)
	model := &strategyModel{}
	registry := executor.NewExecutorRegistry()
	require.NoError(t, registry.Register(executor.NewQAExecutor(model)))
	tasks, results := repo.NewEvaluationTaskRepository(f.db), repo.NewEvaluationResultRepository(f.db)
	worker := ev.NewWorker(nil, strategyProgress{}, ev.NewPythonEvaluationClient(endpoint.URL), nil, registry, tasks, results, nil).WithSnapshots(ss)
	created := []string{}
	t.Cleanup(func() {
		for _, id := range created {
			require.NoError(t, f.db.Table("evaluation_qa_results").Where("task_id=?", id).Delete(map[string]any{}).Error)
		}
	})
	for _, v := range []*policy.Version{base, candidate} {
		code, raw := f.request(t, "POST", "/evaluation/tasks", uuid.NewString(), map[string]any{"dataset_id": version.DatasetID, "dataset_version_id": version.ID, "type": "qa", "strategy_version_id": v.ID})
		require.Equal(t, 201, code)
		var result struct {
			TaskID string `json:"task_id"`
		}
		require.NoError(t, json.Unmarshal(raw, &result))
		created = append(created, result.TaskID)
		j, e := job.New(result.TaskID, ev.EvaluationRunJobType, ev.RunJobPayload{TaskID: result.TaskID})
		require.NoError(t, e)
		require.NoError(t, (ev.RunJobHandler{Worker: worker}).Handle(ctx, j))
		stored, e := tasks.FindByID(ctx, result.TaskID)
		require.NoError(t, e)
		require.Equal(t, d.TaskStatusCompleted, stored.Status, stored.ErrorMessage)
		rows, e := results.FindByTaskID(ctx, result.TaskID)
		require.NoError(t, e)
		require.Len(t, rows, 1)
		require.Equal(t, "  How   much?  ", rows[0].Question)
		require.Equal(t, float64(1), rows[0].Scores["strategy_applied"])
	}
	require.Equal(t, []string{"How   much?", "How much?"}, model.questions)
	proof, e := f.es.StrategyRegression(ctx, f.p, created[0], created[1])
	require.NoError(t, e)
	require.Equal(t, candidate.ID, proof.CandidateVersionID)
	require.Equal(t, base.ID, proof.BaselineVersionID)
	require.Equal(t, float64(1), proof.Candidate.Quality)
	require.Greater(t, proof.Candidate.Cost, float64(20))
	f.assets.deny = true
	_, e = f.es.StrategyRegression(agentctx.WithUserID(ctx, f.p.UserID), f.p, created[0], created[1])
	require.Error(t, e)
}
