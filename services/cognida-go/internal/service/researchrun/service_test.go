package researchrun

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"cognida/internal/infrastructure/llm/chat"
	agentctx "cognida/internal/model/agent"
	l "cognida/internal/model/lifecycle"
	r "cognida/internal/model/researchrun"
	d "cognida/internal/model/taskcontrol"
	agent "cognida/internal/service/agent"
	"github.com/stretchr/testify/require"
)

type contextRunRepository struct {
	r.Repository
	run r.Run
}

type unprovenPartialEngine struct{}

func (unprovenPartialEngine) Available() bool { return true }
func (unprovenPartialEngine) ExecuteGoverned(context.Context, *agent.DeepResearchRequest) (*agent.DeepResearchResponse, error) {
	return &agent.DeepResearchResponse{Sources: []agent.ReportSourceDTO{{ID: "S1", Excerpt: "unproven model output"}}, Metadata: agent.ReportMetadataDTO{CitationChecked: true, SemanticReview: true, Partial: true, ReviewRounds: 1, SupplementalReviewIncomplete: true, Failure: &agent.ResearchFailureDTO{Stage: "supplement_reviewing", Code: "http_error", HTTPStatus: 402}}}, errors.Join(agent.ErrResearchSupplementIncomplete, &chat.HTTPStatusError{StatusCode: 402})
}
func TestProcessRejectsPartialWithoutCompletedReviewSnapshot(t *testing.T) {
	scope := d.Scope{TenantID: 7, UserID: 9}
	in := r.Submission{OperationKey: strings.Repeat("a", 64), PayloadHash: strings.Repeat("b", 64), Profile: r.Profile, Request: r.Request{Query: "question", KBIDs: []string{"allowed"}}, Deadline: time.Now().Add(time.Minute), TimeoutSeconds: 60, MaxCalls: r.MaxCalls}
	body, err := json.Marshal(in)
	require.NoError(t, err)
	repo := &contextRunRepository{run: r.Run{Scope: scope, ID: "partial-fixture", Status: "queued", Submission: body}}
	outputs := 0
	svc, err := New(repo, func(context.Context, d.Scope, []string, *agent.DeepResearchResponse) error { return nil }, func(context.Context, d.Scope, string, *agent.DeepResearchResponse) (l.RefID, error) {
		outputs++
		return l.RefID{}, nil
	})
	require.NoError(t, err)
	svc.SetEngine(unprovenPartialEngine{})
	require.NoError(t, svc.Process(context.Background(), scope, repo.run.ID))
	require.Equal(t, "failed", repo.run.Status)
	require.Zero(t, outputs)
}

func (repo *contextRunRepository) Claim(context.Context, d.Scope, string, string, time.Duration) (*r.Run, error) {
	if !repo.run.Terminal() {
		repo.run.Status = "running"
	}
	return &repo.run, nil
}
func (repo *contextRunRepository) Complete(_ context.Context, _ d.Scope, _, _ string, completion r.Completion) (*r.Run, error) {
	repo.run.Status = completion.Status
	return &repo.run, nil
}

type contextEngine struct{ contexts []context.Context }

func (*contextEngine) Available() bool { return true }
func (engine *contextEngine) ExecuteGoverned(ctx context.Context, _ *agent.DeepResearchRequest) (*agent.DeepResearchResponse, error) {
	engine.contexts = append(engine.contexts, ctx)
	return &agent.DeepResearchResponse{Success: true, Sources: []agent.ReportSourceDTO{{ID: "S1", Excerpt: "real test boundary"}}, Metadata: agent.ReportMetadataDTO{CitationChecked: true}}, nil
}

func TestProcessSuppliesStableRetrievalIdentityAndPreservesScope(t *testing.T) {
	for _, requestID := range []string{"", "original-request"} {
		t.Run("request="+requestID, func(t *testing.T) {
			scope := d.Scope{TenantID: 17, UserID: 29}
			input := r.Submission{RequestID: requestID, OperationKey: strings.Repeat("a", 64), PayloadHash: strings.Repeat("b", 64), Profile: r.Profile, Request: r.Request{Query: "Evidence?", KBIDs: []string{"authorized-kb"}}, Deadline: time.Now().Add(time.Minute), TimeoutSeconds: 60, MaxCalls: r.MaxCalls}
			raw, err := json.Marshal(input)
			require.NoError(t, err)
			repo := &contextRunRepository{run: r.Run{Scope: scope, ID: "durable-run", Status: "queued", Submission: raw}}
			service, err := New(repo, func(context.Context, d.Scope, []string, *agent.DeepResearchResponse) error { return nil }, func(context.Context, d.Scope, string, *agent.DeepResearchResponse) (l.RefID, error) {
				return l.RefID{AssetID: "b6d61fe4-66ee-4508-b22a-ef5c8cf89713", Version: 1}, nil
			})
			require.NoError(t, err)
			engine := &contextEngine{}
			service.SetEngine(engine)
			// A worker's unrelated context cannot choose the persisted run's pin.
			parent := agentctx.WithRequestID(context.Background(), "unrelated-worker-request")
			require.NoError(t, service.Process(parent, scope, repo.run.ID))
			require.Equal(t, "succeeded", repo.run.Status)
			require.Len(t, engine.contexts, 1)
			ctx := engine.contexts[0]
			expected := requestID
			if expected == "" {
				expected = "research:" + repo.run.ID
			}
			require.Equal(t, expected, agentctx.MustGetRequestID(ctx))
			require.Equal(t, scope.TenantID, agentctx.MustGetTenantID(ctx))
			require.Equal(t, scope.UserID, agentctx.MustGetUserID(ctx))
			require.Equal(t, []string{"authorized-kb"}, agentctx.MustGetAllowedKBIDs(ctx))
			require.Equal(t, "manual", agentctx.MustGetKBScopeMode(ctx))
			require.NoError(t, service.Process(parent, scope, repo.run.ID))
			require.Len(t, engine.contexts, 1, "terminal reconciliation must not repeat model work")
		})
	}
}
