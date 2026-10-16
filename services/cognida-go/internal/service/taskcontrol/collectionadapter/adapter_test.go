package collectionadapter

import (
	l "cognida/internal/model/lifecycle"
	m "cognida/internal/model/taskcontrol"
	"cognida/internal/service/collection"
	"cognida/internal/service/lifecycle"
	tc "cognida/internal/service/taskcontrol"
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func testCommand(a *Adapter) tc.ExecutionCommand {
	c := a.Capability()
	cmd := tc.ExecutionCommand{Scope: m.Scope{TenantID: 1, UserID: 2}, TaskID: "task", PlanVersion: 1, Attempt: 1, Deadline: time.Now().Add(time.Minute), Node: tc.Node{ID: "snapshot", Capability: c.ID, CapabilityVersion: 1, OutputKinds: c.OutputKinds, Tools: c.Tools, Permissions: c.Permissions, MaxAttempts: 1, MaxCallsPerAttempt: 3, TimeoutSeconds: 60, Parameters: &tc.NodeParameters{Collection: &tc.CollectionParameters{URL: "https://example.com/source"}}}}
	seal(&cmd)
	return cmd
}
func seal(c *tc.ExecutionCommand) {
	raw, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
	c.OperationKey = m.Digest(raw)
	raw, _ = json.Marshal(c)
	raw, _ = m.CanonicalJSON(raw, '{', m.MaxSnapshotBytes)
	c.PayloadHash = m.Digest(raw)
}
func TestSnapshotCommandAndReceiptAreBound(t *testing.T) {
	a, e := New(&collection.Service{}, &lifecycle.Service{})
	require.NoError(t, e)
	cmd := testCommand(a)
	in, e := a.invocation(context.Background(), cmd)
	require.NoError(t, e)
	require.EqualValues(t, 2<<20, in.MaxResponseBytes)
	require.Equal(t, 3, in.CallsReserved)
	for _, mutate := range []func(*tc.ExecutionCommand){func(c *tc.ExecutionCommand) { c.Scope.UserID++ }, func(c *tc.ExecutionCommand) { c.Node.Parameters.Collection.URL = "https://other.example" }, func(c *tc.ExecutionCommand) { c.OperationKey = "bad" }, func(c *tc.ExecutionCommand) { c.InputRefs = []l.RefID{{AssetID: "forged", Version: 1}} }} {
		c := testCommand(a)
		mutate(&c)
		_, e := a.invocation(context.Background(), c)
		require.Error(t, e)
	}
	raw, _ := json.Marshal(receipt{"collection", "run", cmd.OperationKey, cmd.PayloadHash})
	require.NoError(t, validateHandle(cmd, tc.ExecutionHandle{DomainID: "run", Receipt: raw}))
	require.ErrorIs(t, validateHandle(cmd, tc.ExecutionHandle{DomainID: "other", Receipt: raw}), m.ErrConflict)
}
func TestSnapshotValidationRejectsUnsafeOrUnboundedOptions(t *testing.T) {
	a, _ := New(&collection.Service{}, &lifecycle.Service{})
	for _, mutate := range []func(*tc.Node){func(n *tc.Node) { n.Parameters.Collection.URL = "http://example.com" }, func(n *tc.Node) { n.Parameters.Collection.URL = "https://user:secret@example.com" }, func(n *tc.Node) { n.Parameters.Collection.MaxResponseBytes = 3 << 20 }, func(n *tc.Node) { n.MaxCallsPerAttempt = 2 }, func(n *tc.Node) { n.TimeoutSeconds = 121 }, func(n *tc.Node) { n.Parameters.Governance = &tc.GovernanceParameters{} }, func(n *tc.Node) { n.OutputKinds = []l.Kind{l.PublishedAsset} }, func(n *tc.Node) { n.Permissions = []string{"write"} }, func(n *tc.Node) { n.Inputs = []tc.InputBinding{{Kind: l.RawAsset}} }} {
		c := testCommand(a)
		mutate(&c.Node)
		require.Error(t, a.Validate(context.Background(), c.Scope, c.Node))
	}
}
