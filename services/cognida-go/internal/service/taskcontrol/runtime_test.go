package taskcontrol

import (
	"context"
	"encoding/json"
	"testing"

	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	"github.com/stretchr/testify/require"
)

func TestRuntimeOnlyTerminalDomainFactsUnblockDependencies(t *testing.T) {
	require.False(t, childTerminal(d.Child{Status: d.ChildBound, DomainStatus: d.DomainRunning}))
	require.False(t, childTerminal(d.Child{Status: d.ChildReconcileRequired, DomainStatus: d.DomainSucceeded}))
	require.True(t, childTerminal(d.Child{Status: d.ChildBound, DomainStatus: d.DomainQuarantined}))
	require.True(t, childTerminal(d.Child{Status: d.ChildCancelled}))
	latest := latestChildren([]d.Child{{NodeID: "clean", PlanVersion: 1, Attempt: 2, DomainStatus: d.DomainFailed}, {NodeID: "clean", PlanVersion: 1, Attempt: 1, DomainStatus: d.DomainSucceeded}, {NodeID: "clean", PlanVersion: 2, Attempt: 3}}, 1)
	require.Equal(t, 2, latest["clean"].Attempt)
	require.Equal(t, d.DomainFailed, latest["clean"].DomainStatus)
}
func TestRuntimeBindingsRequireOneAuthorizedTypedResult(t *testing.T) {
	v, _ := testValidator(t)
	other := testRef
	other.Version = 2
	v.resolve = func(_ context.Context, _ d.Scope, ref l.RefID) (l.Kind, error) {
		if ref != testRef && ref != other {
			return "", d.ErrForbidden
		}
		return l.CleanedAsset, nil
	}
	r := &Runtime{tasks: &Service{validator: v}}
	node := Node{Inputs: []InputBinding{{FromNode: "clean", Kind: l.CleanedAsset}}, OutputKinds: []l.Kind{l.CleanedAsset}}
	raw, _ := json.Marshal([]l.RefID{testRef})
	latest := map[string]d.Child{"clean": {DomainStatus: d.DomainRunning, OutputRefs: raw}}
	_, err := r.resolveBindings(context.Background(), testScope, node, latest)
	require.ErrorIs(t, err, d.ErrConflict)
	latest["clean"] = d.Child{DomainStatus: d.DomainSucceeded, OutputRefs: raw}
	refs, err := r.resolveBindings(context.Background(), testScope, node, latest)
	require.NoError(t, err)
	require.Equal(t, []l.RefID{testRef}, refs)
	raw, _ = json.Marshal([]l.RefID{testRef, other})
	latest["clean"] = d.Child{DomainStatus: d.DomainSucceeded, OutputRefs: raw}
	_, err = r.resolveBindings(context.Background(), testScope, node, latest)
	require.ErrorIs(t, err, d.ErrInvalid, "ambiguous results must not pick an arbitrary version")
	_, err = r.checkedOutputs(context.Background(), testScope, node, &DomainObservation{Status: d.DomainSucceeded})
	require.ErrorIs(t, err, d.ErrInvalid, "declared results must exist")
	refs, err = r.checkedOutputs(context.Background(), testScope, node, &DomainObservation{Status: d.DomainSucceeded, OutputRefs: []l.RefID{testRef, testRef}})
	require.NoError(t, err)
	require.Len(t, refs, 1)
}
func TestRuntimeImmutableDispatchPayloadRejectsChangedNodeAndHash(t *testing.T) {
	r := &Runtime{}
	plan := testPlan()
	cmd := ExecutionCommand{TaskID: "task", PlanVersion: 1, Node: plan.Nodes[0], Attempt: 1, InputRefs: []l.RefID{testRef}}
	raw, _ := json.Marshal(cmd)
	canonical, err := d.CanonicalJSON(raw, '{', d.MaxSnapshotBytes)
	require.NoError(t, err)
	child := d.Child{NodeID: "clean", PlanVersion: 1, Attempt: 1, OperationKey: "operation"}
	event := d.Outbox{OperationKey: "operation", Payload: raw, PayloadHash: d.Digest(canonical)}
	out, err := r.command(testScope, "task", plan, child, event)
	require.NoError(t, err)
	require.Equal(t, testScope, out.Scope)
	require.Equal(t, event.PayloadHash, out.PayloadHash)
	changed := plan
	changed.Nodes = append([]Node(nil), plan.Nodes...)
	changed.Nodes[0].MaxCallsPerAttempt++
	_, err = r.command(testScope, "task", changed, child, event)
	require.ErrorIs(t, err, d.ErrInvalid)
	event.PayloadHash = "tampered"
	_, err = r.command(testScope, "task", plan, child, event)
	require.ErrorIs(t, err, d.ErrInvalid)
}

func TestRuntimeReplanIncludesOriginalReceiptsWithoutResettingAttempts(t *testing.T) {
	children := []d.Child{{ID: "original", NodeID: "first", PlanVersion: 1, Attempt: 1, DomainStatus: d.DomainSucceeded}, {ID: "retry", NodeID: "first", PlanVersion: 2, Attempt: 2, DomainStatus: d.DomainFailed}, {ID: "future", NodeID: "second", PlanVersion: 3, Attempt: 1}}
	latest := latestChildren(children, 2)
	require.Len(t, latest, 1)
	require.Equal(t, "retry", latest["first"].ID)
	require.Equal(t, 2, latest["first"].Attempt)
	latest = latestChildren(children[:1], 2)
	require.Equal(t, "original", latest["first"].ID)
	require.Equal(t, d.DomainSucceeded, latest["first"].DomainStatus)
}
