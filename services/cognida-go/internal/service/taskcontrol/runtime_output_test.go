package taskcontrol

import (
	"context"
	"encoding/json"
	"testing"

	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	"github.com/stretchr/testify/require"
)

type outputSummaryRepository struct {
	*dataWorkflowSummaryRepository
	update d.TaskUpdate
}

func (r *outputSummaryRepository) FinalizeTask(_ context.Context, _ d.Scope, _ string, _ int64, update d.TaskUpdate) (*d.Task, error) {
	r.update = update
	r.source.task.ResultRefs = update.ResultRefs
	r.source.task.Status = update.Status
	return r.source.task, nil
}

type sourceOutputAdapter struct {
	ExecutionAdapter
	denied  bool
	inputs  int
	outputs int
}

func (a *sourceOutputAdapter) ValidateInputs(context.Context, d.Scope, []l.RefID) error {
	a.inputs++
	return d.ErrInvalid
}
func (a *sourceOutputAdapter) ValidateOutputs(context.Context, d.Scope, []l.RefID) error {
	a.outputs++
	if a.denied {
		return d.ErrForbidden
	}
	return nil
}

func TestSourceOutputsUseSeparateAuthorizationForFinalizationAndSummary(t *testing.T) {
	for _, scenario := range []string{"allowed", "output revoked", "resolver revoked"} {
		t.Run(scenario, func(t *testing.T) {
			s, stored, _ := supervisorFixture(t)
			stored.task.CurrentPlanVersion, stored.task.ExecutionPlanVersion = 1, 1
			stored.task.Status = d.StatusRunning
			raw, err := json.Marshal(PlanSpec{SchemaVersion: 1, Nodes: []Node{{ID: "import", Capability: "huggingface.import"}}})
			require.NoError(t, err)
			stored.plan = &d.Plan{TaskID: "draft", Version: 1, Snapshot: raw}
			refs, err := json.Marshal([]l.RefID{testRef})
			require.NoError(t, err)
			stored.children = []d.Child{{ID: "child", NodeID: "import", PlanVersion: 1, Attempt: 1, Domain: "huggingface", DomainID: "real-import", Status: d.ChildBound, DomainStatus: d.DomainSucceeded, OutputRefs: refs}}
			adapter := &sourceOutputAdapter{denied: scenario == "output revoked"}
			if scenario == "resolver revoked" {
				s.tasks.validator.resolve = func(context.Context, d.Scope, l.RefID) (l.Kind, error) { return "", d.ErrForbidden }
			}
			repository := &outputSummaryRepository{dataWorkflowSummaryRepository: &dataWorkflowSummaryRepository{source: stored}}
			runtime := &Runtime{tasks: s.tasks, repo: repository, adapters: map[string]ExecutionAdapter{"huggingface.import": adapter}, enabled: true}
			require.NoError(t, runtime.finalize(context.Background(), testScope, stored.task, stored.children, d.ExecutionFence{}, d.StatusCompleted, ""))
			var final []l.RefID
			require.NoError(t, json.Unmarshal(repository.update.ResultRefs, &final))
			summary := &SupervisorSummary{TaskID: "draft", TaskVersion: stored.task.Version}
			require.NoError(t, runtime.DecorateSummary(context.Background(), testScope, summary))
			if scenario == "allowed" {
				require.Equal(t, []l.RefID{testRef}, final)
				require.Equal(t, final, summary.Nodes[0].OutputRefs)
				require.Equal(t, final, summary.ResultRefs)
			} else {
				require.Empty(t, final)
				require.Empty(t, summary.Nodes[0].OutputRefs)
				require.Empty(t, summary.ResultRefs)
			}
			require.Zero(t, adapter.inputs, "zero-input dispatch must not be weakened for output reads")
			require.Equal(t, 2, adapter.outputs)
		})
	}
}

type legacyOutputAdapter struct {
	ExecutionAdapter
	calls int
}

func (a *legacyOutputAdapter) ValidateInputs(context.Context, d.Scope, []l.RefID) error {
	a.calls++
	return d.ErrForbidden
}
func TestExistingOutputAuthorizationRemainsFailClosed(t *testing.T) {
	adapter := &legacyOutputAdapter{}
	require.ErrorIs(t, authorizeExecutionOutputs(context.Background(), testScope, adapter, []l.RefID{testRef}), d.ErrForbidden)
	require.Equal(t, 1, adapter.calls)
	require.ErrorIs(t, authorizeExecutionOutputs(context.Background(), testScope, nil, []l.RefID{testRef}), d.ErrForbidden)
}
