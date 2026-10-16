package evaluation

import (
	d "cognida/internal/model/evaluation"
	f "cognida/internal/model/feedback"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestFeedbackSnapshotReadMapsOnlyUnavailableSourceErrors(t *testing.T) {
	versions, _, _, p, _ := versionFixture(t)
	for _, sourceError := range []error{f.ErrForbidden, f.ErrNotFound, l.ErrForbidden, l.ErrNotFound, l.ErrExpired} {
		t.Run(sourceError.Error(), func(t *testing.T) {
			snapshots := &SnapshotService{Versions: versions, FeedbackView: func(context.Context, l.Principal, f.View) error {
				return fmt.Errorf("feedback view: %w", sourceError)
			}}
			_, err := snapshots.ValidateRead(context.Background(), p, &d.EvaluationSnapshot{FeedbackView: &f.View{CaseID: "case"}})
			require.ErrorIs(t, err, d.ErrVersionForbidden)
		})
	}
	unexpected := errors.New("projection database unavailable")
	snapshots := &SnapshotService{Versions: versions, FeedbackView: func(context.Context, l.Principal, f.View) error { return unexpected }}
	_, err := snapshots.ValidateRead(context.Background(), p, &d.EvaluationSnapshot{FeedbackView: &f.View{CaseID: "case"}})
	require.ErrorIs(t, err, unexpected)
	require.NotErrorIs(t, err, d.ErrVersionForbidden)
}

func TestFeedbackComparisonRejectsChangedConfigSampleAndUnverifiedView(t *testing.T) {
	versions, _, inputs, p, request := versionFixture(t)
	version, e := versions.Publish(context.Background(), p, request, "key")
	require.NoError(t, e)
	tasks, results := newMockTaskRepo(), newMockResultRepo()
	service := NewService(nil, tasks, results, nil, nil, nil)
	service.snapshots = &SnapshotService{Versions: versions, FeedbackView: func(context.Context, l.Principal, f.View) error { return nil }}
	fixed := d.EvaluationSnapshot{DatasetVersionID: version.ID, DatasetManifest: version.ManifestDigest, Profile: "qa", QAPairs: []d.QAPair{{Question: "q1", ReferenceAnswer: "a1"}, {Question: "q2", ReferenceAnswer: "a2"}}, Configuration: json.RawMessage(`{"type":"rag","kb_id":"kb","config":{"top_k":5}}`), Target: json.RawMessage(`{"model":"fixed-model","prompt":"fixed"}`), GraderVersion: "fixed", CreatedAt: time.Now().UTC()}
	raw, e := json.Marshal(fixed)
	require.NoError(t, e)
	// Empty labels are omitted in the real persisted JSON snapshot. They must
	// retain the same authorization semantics after being decoded as nil.
	base := f.View{CaseID: "case", PublicationID: "baseline", KnowledgeBaseID: "kb", KnowledgeID: "base", Labels: []string{}}
	next := f.View{CaseID: "case", PublicationID: "candidate", KnowledgeBaseID: "kb", KnowledgeID: "next", Labels: []string{}}
	for i, id := range []string{"base", "next"} {
		snap := fixed
		view := base
		if i == 1 {
			view = next
		}
		snap.FeedbackView = &view
		body, _ := json.Marshal(snap)
		tasks.tasks[id] = &d.EvaluationTask{ID: id, TenantID: p.TenantID, UserID: p.UserID, Status: d.TaskStatusCompleted, Snapshot: body}
		for _, qa := range fixed.QAPairs {
			results.results[id] = append(results.results[id], &d.EvaluationResult{Question: qa.Question, ReferenceAnswer: qa.ReferenceAnswer, Success: true, Scores: map[string]float64{"rouge_l": .2 + float64(i)*.5, "feedback_view_applied": 1}})
		}
	}
	gate := f.Gate{Metric: "rouge_l", HigherIsBetter: true, MinPairs: 2}
	compare := func() (*Comparison, error) {
		return service.FeedbackCompare(context.Background(), p, "base", "next", raw, base, next, gate)
	}
	out, e := compare()
	require.NoError(t, e)
	require.Equal(t, "eligible_for_review", out.Gate)
	require.InDelta(t, .5, out.Improvement, 1e-9)
	original := append([]byte{}, tasks.tasks["next"].Snapshot...)
	var altered d.EvaluationSnapshot
	require.NoError(t, json.Unmarshal(original, &altered))
	altered.FeedbackView.Labels = []string{"unapproved-label"}
	tasks.tasks["next"].Snapshot, _ = json.Marshal(altered)
	_, e = compare()
	require.ErrorIs(t, e, f.ErrGate)
	require.NoError(t, json.Unmarshal(original, &altered))
	altered.Target = json.RawMessage(`{"model":"different"}`)
	tasks.tasks["next"].Snapshot, _ = json.Marshal(altered)
	_, e = compare()
	require.ErrorIs(t, e, f.ErrGate)
	tasks.tasks["next"].Snapshot = original
	results.results["next"][0].Scores["feedback_view_applied"] = 0
	_, e = compare()
	require.ErrorIs(t, e, f.ErrGate)
	results.results["next"][0].Scores["feedback_view_applied"] = 1
	results.results["next"][0].ReferenceAnswer = "changed ground truth"
	_, e = compare()
	require.ErrorIs(t, e, f.ErrGate)
	results.results["next"][0].ReferenceAnswer = "a1"
	inputs.deny = true
	_, e = compare()
	require.ErrorIs(t, e, d.ErrVersionForbidden)
}

func TestFeedbackConfigRejectsInvalidTypesBeforeFreezingPaidWork(t *testing.T) {
	for _, raw := range []string{
		`{"top_k":"20"}`, `{"top_k":null}`, `{"top_k":0}`, `{"top_k":20.5}`,
		`{"applied_labels":"approved"}`, `{"applied_labels":[null]}`, `{"applied_labels":[" "]}`,
		`{"graders":["unknown"]}`, `{"arbitrary_model_options":{}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			var config map[string]any
			require.NoError(t, json.Unmarshal([]byte(raw), &config))
			require.ErrorIs(t, validateFeedbackConfig(EvaluationTaskConfig{Config: config}), f.ErrInvalid)
		})
	}
	var config map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"top_k":20,"applied_labels":["approved"],"graders":["rouge","retrieval"]}`), &config))
	require.NoError(t, validateFeedbackConfig(EvaluationTaskConfig{Config: config}))
	require.NoError(t, validateFeedbackConfig(EvaluationTaskConfig{}))
}
