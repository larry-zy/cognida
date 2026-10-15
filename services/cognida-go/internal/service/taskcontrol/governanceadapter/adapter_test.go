package governanceadapter

import (
	l "cognida/internal/model/lifecycle"
	m "cognida/internal/model/taskcontrol"
	"cognida/internal/service/governance"
	"cognida/internal/service/lifecycle"
	tc "cognida/internal/service/taskcontrol"
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func command(a *Adapter) tc.ExecutionCommand {
	c := a.Capability()
	ref := l.RefID{AssetID: "00000000-0000-4000-8000-000000000001", Version: 1}
	out := tc.ExecutionCommand{Scope: m.Scope{TenantID: 1, UserID: 2}, TaskID: "task", PlanVersion: 1, Attempt: 1, Deadline: time.Now().Add(time.Minute), InputRefs: []l.RefID{ref}, Node: tc.Node{ID: "labels", Capability: c.ID, CapabilityVersion: 1, Inputs: []tc.InputBinding{{Asset: &ref, Kind: l.RawAsset}}, OutputKinds: c.OutputKinds, Tools: c.Tools, Permissions: c.Permissions, MaxAttempts: 1, MaxCallsPerAttempt: 34, TimeoutSeconds: 60, Parameters: &tc.NodeParameters{Governance: &tc.GovernanceParameters{Labels: []string{"sales"}}}}}
	seal(&out)
	return out
}
func seal(c *tc.ExecutionCommand) {
	raw, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
	c.OperationKey = m.Digest(raw)
	raw, _ = json.Marshal(c)
	raw, _ = m.CanonicalJSON(raw, '{', m.MaxSnapshotBytes)
	c.PayloadHash = m.Digest(raw)
}
func TestGovernanceCommandBoundToOwnerInputsLabels(t *testing.T) {
	for _, cap := range []string{"governance.tagging", "governance.classification"} {
		a, e := New(&governance.Service{}, &lifecycle.Service{}, cap)
		require.NoError(t, e)
		c := command(a)
		in, e := a.invocation(context.Background(), c)
		require.NoError(t, e)
		require.Equal(t, 34, in.CallsReserved)
		for _, mutate := range []func(*tc.ExecutionCommand){func(c *tc.ExecutionCommand) { c.Scope.UserID++ }, func(c *tc.ExecutionCommand) { c.Node.Parameters.Governance.Labels = []string{"changed"} }, func(c *tc.ExecutionCommand) { c.InputRefs[0].AssetID = "other" }} {
			c := command(a)
			mutate(&c)
			_, e := a.invocation(context.Background(), c)
			require.Error(t, e)
		}
	}
}
func TestGovernanceValidationNeverApprovesOrGuessesLabels(t *testing.T) {
	a, _ := New(&governance.Service{}, &lifecycle.Service{}, "governance.classification")
	for _, mutate := range []func(*tc.Node){func(n *tc.Node) { n.Parameters.Governance.Labels = nil }, func(n *tc.Node) { n.Parameters.Governance.Labels = []string{"sales", "sales"} }, func(n *tc.Node) { n.Parameters.Governance.Labels = []string{" sales"} }, func(n *tc.Node) { n.Parameters.Feature = &tc.FeatureParameters{} }, func(n *tc.Node) { n.MaxCallsPerAttempt = 33 }, func(n *tc.Node) { n.OutputKinds = []l.Kind{l.PublishedAsset} }, func(n *tc.Node) { n.Inputs = nil }, func(n *tc.Node) { n.Inputs[0].Kind = l.Result }, func(n *tc.Node) { n.Permissions = []string{"admin"} }} {
		c := command(a)
		mutate(&c.Node)
		require.Error(t, a.Validate(context.Background(), c.Scope, c.Node))
	}
}
