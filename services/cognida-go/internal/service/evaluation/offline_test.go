package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	d "cognida/internal/model/evaluation"
	l "cognida/internal/model/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestOfflineSnapshotColumnsAreExplicitAndTextOnly(t *testing.T) {
	columns := d.OfflineColumns{QuestionColumn: "q", ReferenceAnswerColumn: "ref", GeneratedAnswerColumn: "answer"}
	good := l.Input{Kind: "table", Columns: []string{"q", "ref", "answer", "id"}, Rows: [][]any{{"Why?", "00123", "00123", "9007199254740993"}, {"Other?", "nonempty", "", "0"}}}
	body, _ := json.Marshal(good)
	items, err := offlineItems(body, columns)
	require.NoError(t, err)
	require.Equal(t, "00123", items[0].GeneratedAnswer)
	require.Empty(t, items[1].GeneratedAnswer)
	for _, change := range []func(*l.Input){func(v *l.Input) { v.Rows[0][2] = 123 }, func(v *l.Input) { v.Columns[3] = "q" }, func(v *l.Input) { v.Rows[0][0] = " " }, func(v *l.Input) { v.Rows[0][1] = nil }, func(v *l.Input) { v.Rows = make([][]any, 101) }, func(v *l.Input) { v.Kind = "text"; v.Text = "q,ref,answer" }} {
		var next l.Input
		require.NoError(t, json.Unmarshal(body, &next))
		change(&next)
		raw, _ := json.Marshal(next)
		_, err = offlineItems(raw, columns)
		require.ErrorIs(t, err, d.ErrOfflineInvalid)
	}
	columns.GeneratedAnswerColumn = "missing"
	_, err = offlineItems(body, columns)
	require.ErrorIs(t, err, d.ErrOfflineInvalid)
}
func TestOfflinePythonScorerStrictContractAndSingleCall(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
		valid      bool
	}{
		{"valid", `{"items":[{"index":0,"scores":{"exact_match":100,"contains_match":50}}]}`, 200, true},
		{"missing_metric", `{"items":[{"index":0,"scores":{"exact_match":100}}]}`, 200, false},
		{"out_of_range", `{"items":[{"index":0,"scores":{"exact_match":101,"contains_match":50}}]}`, 200, false},
		{"missing_index", `{"items":[{"index":1,"scores":{"exact_match":100,"contains_match":50}}]}`, 200, false},
		{"unsupported", `{"items":[{"index":0,"scores":{"exact_match":100,"contains_match":50}}],"unsupported":["contains_match"]}`, 200, false},
		{"extra_metric", `{"items":[{"index":0,"scores":{"exact_match":100,"contains_match":50,"llm_score":99}}]}`, 200, false},
		{"provider_error", "private provider context", 503, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				require.Equal(t, "/api/v1/evaluation/compute-metrics", r.URL.Path)
				var request ComputeMetricsRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				require.Equal(t, []string{"exact_match", "contains_match"}, request.Graders)
				require.Empty(t, request.LLMJudge)
				require.Empty(t, request.Reference)
				require.Equal(t, "existing", request.Items[0].GeneratedAnswer)
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			scorer := NewOfflineScorer(NewPythonEvaluationClient(server.URL))
			out, err := scorer.Score(context.Background(), []d.OfflineItem{{Question: "q", ReferenceAnswer: "ref", GeneratedAnswer: "existing"}})
			require.EqualValues(t, 1, calls.Load())
			if test.valid {
				require.NoError(t, err)
				require.Equal(t, 50.0, out[0].ContainsMatch)
			} else {
				require.ErrorIs(t, err, errOfflineScoring)
				require.NotContains(t, err.Error(), "private provider")
			}
		})
	}
}
func TestOfflinePythonScorerDeadlineAndNil(t *testing.T) {
	_, err := NewOfflineScorer(nil).Score(context.Background(), []d.OfflineItem{{Question: "q", ReferenceAnswer: "r"}})
	require.Error(t, err)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = NewOfflineScorer(NewPythonEvaluationClient(server.URL)).Score(ctx, []d.OfflineItem{{Question: "q", ReferenceAnswer: "r"}})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
func TestOfflineReadFailsClosedAndLegacyRecoverySkips(t *testing.T) {
	tasks := newMockTaskRepo()
	tasks.tasks["off"] = &d.EvaluationTask{ID: "off", Type: d.OfflineType, Status: d.TaskStatusPending}
	s := NewService(nil, tasks, newMockResultRepo(), nil, nil, nil)
	_, err := s.GetEvaluationResult(context.Background(), "off")
	require.ErrorIs(t, err, d.ErrOfflineForbidden)
	_, err = s.GetQAResults(context.Background(), "off", 1, 20)
	require.ErrorIs(t, err, d.ErrOfflineForbidden)
	_, err = s.GetQAResultsByCursor(context.Background(), "off", "", 20)
	require.ErrorIs(t, err, d.ErrOfflineForbidden)
	_, err = s.GetTaskProgress(context.Background(), "off")
	require.ErrorIs(t, err, d.ErrOfflineForbidden)
	require.ErrorIs(t, s.DeleteEvaluationTask(context.Background(), "off"), d.ErrOfflineConflict)
	jobs, err := (RunRecoverySource{Tasks: tasks}).PendingJobs(context.Background(), 100)
	require.NoError(t, err)
	require.Empty(t, jobs)
	w := &EvaluationWorker{taskRepo: tasks}
	w.executeTaskWithContext(context.Background(), "off")
	w.handleTaskError(context.Background(), "off", errors.New("old error"), 1)
	require.Equal(t, d.TaskStatusPending, tasks.tasks["off"].Status)
}
func TestOfflineSnapshotSizeAndValidation(t *testing.T) {
	items := []d.OfflineItem{{Question: "q", ReferenceAnswer: "001", GeneratedAnswer: "001"}}
	require.NoError(t, d.ValidateOfflineSnapshot(items, d.OfflineSnapshotHash(items)))
	changed := append([]d.OfflineItem(nil), items...)
	changed[0].GeneratedAnswer = "002"
	require.Error(t, d.ValidateOfflineSnapshot(changed, d.OfflineSnapshotHash(items)))
	items[0].ReferenceAnswer = strings.Repeat("r", 8193)
	require.Error(t, d.ValidateOfflineSnapshot(items, d.OfflineSnapshotHash(items)))
}
