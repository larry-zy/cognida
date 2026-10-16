package warehouseadapter

import (
	l "cognida/internal/model/lifecycle"
	m "cognida/internal/model/taskcontrol"
	tc "cognida/internal/service/taskcontrol"
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func command() tc.ExecutionCommand {
	a := &Adapter{}
	cap := a.Capability()
	ref := l.RefID{AssetID: "00000000-0000-4000-8000-000000000001", Version: 1}
	c := tc.ExecutionCommand{Scope: m.Scope{TenantID: 1, UserID: 2}, TaskID: "task", PlanVersion: 1, Attempt: 1, Deadline: time.Now().Add(time.Minute), InputRefs: []l.RefID{ref}, Node: tc.Node{ID: "warehouse", Capability: cap.ID, CapabilityVersion: 1, Inputs: []tc.InputBinding{{Asset: &ref, Kind: l.RawAsset}}, OutputKinds: cap.OutputKinds, Tools: cap.Tools, Permissions: cap.Permissions, MaxAttempts: 1, MaxCallsPerAttempt: 103, TimeoutSeconds: 120, Parameters: &tc.NodeParameters{Warehouse: &tc.WarehouseParameters{ModelID: "model", Version: 1, Mode: "full"}}}}
	seal(&c)
	return c
}
func seal(c *tc.ExecutionCommand) {
	raw, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
	c.OperationKey = m.Digest(raw)
	raw, _ = json.Marshal(c)
	raw, _ = m.CanonicalJSON(raw, '{', m.MaxSnapshotBytes)
	c.PayloadHash = m.Digest(raw)
}
func TestWarehouseCommandBoundToIdentityModelAndInputs(t *testing.T) {
	a := &Adapter{}
	c := command()
	r, e := a.command(c)
	require.NoError(t, e)
	require.Equal(t, "model", r.ModelID)
	for _, mutate := range []func(*tc.ExecutionCommand){func(c *tc.ExecutionCommand) { c.Scope.UserID++ }, func(c *tc.ExecutionCommand) { c.Node.Parameters.Warehouse.Version++ }, func(c *tc.ExecutionCommand) { c.InputRefs[0].Version++ }, func(c *tc.ExecutionCommand) { c.Attempt++ }, func(c *tc.ExecutionCommand) { c.OperationKey = "forged" }} {
		c := command()
		mutate(&c)
		_, e := a.command(c)
		require.Error(t, e)
	}
}
func TestWarehouseRejectsUnsupportedExecutionAndParameterAliases(t *testing.T) {
	a := &Adapter{}
	for _, mutate := range []func(*tc.Node){func(n *tc.Node) { n.Parameters.Feature = &tc.FeatureParameters{} }, func(n *tc.Node) { n.Parameters.Warehouse.Mode = "sql" }, func(n *tc.Node) { n.Parameters.Warehouse.Mode = "backfill" }, func(n *tc.Node) { n.MaxCallsPerAttempt = 102 }, func(n *tc.Node) { n.MaxAttempts = 2 }, func(n *tc.Node) { n.OutputKinds = []l.Kind{l.PublishedAsset} }, func(n *tc.Node) { n.Permissions = []string{"admin"} }, func(n *tc.Node) { n.Inputs[0].Asset = nil; n.Inputs[0].FromNode = "prior" }, func(n *tc.Node) { n.Inputs = append(n.Inputs, n.Inputs[0]) }} {
		c := command()
		mutate(&c.Node)
		require.Error(t, a.Validate(context.Background(), c.Scope, c.Node))
	}
}
func TestWarehouseBackfillRequiresOrderedExplicitInterval(t *testing.T) {
	c := command()
	p := c.Node.Parameters.Warehouse
	p.Mode = "backfill"
	p.FactSourceRef = c.InputRefs[0].String()
	p.EventFrom = "2026-01-01T00:00:00Z"
	p.EventTo = "2026-01-02T00:00:00Z"
	r, e := request(c.Node)
	require.NoError(t, e)
	require.True(t, r.EventTo.After(*r.EventFrom))
	p.EventTo = p.EventFrom
	_, e = request(c.Node)
	require.Error(t, e)
}
