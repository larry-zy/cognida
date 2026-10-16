package sftadapter

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	m "cognida/internal/model/taskcontrol"
	"cognida/internal/service/governance"
	"cognida/internal/service/lifecycle"
	tc "cognida/internal/service/taskcontrol"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func command() tc.ExecutionCommand {
	ref := l.RefID{AssetID: "00000000-0000-4000-8000-000000000001", Version: 1}
	n := tc.Node{ID: "sft", Capability: CapabilityID, CapabilityVersion: 1, Inputs: []tc.InputBinding{{Kind: l.CleanedAsset, Asset: &ref}}, OutputKinds: []l.Kind{l.Result}, Tools: []string{CapabilityID}, Permissions: []string{"read"}, MaxAttempts: 1, MaxCallsPerAttempt: 3, TimeoutSeconds: 60, Parameters: &tc.NodeParameters{SFT: &tc.SFTParameters{Name: "Support", StandardVersionID: "version", Format: "qa", Columns: d.SFTColumns{Question: "q", ReferenceAnswer: "a"}}}}
	c := tc.ExecutionCommand{Scope: m.Scope{TenantID: 1, UserID: 2}, TaskID: "task", PlanVersion: 1, Attempt: 1, Deadline: time.Now().Add(time.Minute), Node: n, InputRefs: []l.RefID{ref}}
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
func TestSFTCommandScopeAndWholePayloadBound(t *testing.T) {
	c := command()
	in, err := invocation(context.Background(), c)
	require.NoError(t, err)
	require.Equal(t, d.SFTProfile, in.Profile)
	require.Equal(t, 3, in.CallsReserved)
	for name, mutate := range map[string]func(*tc.ExecutionCommand){
		"user":     func(c *tc.ExecutionCommand) { c.Scope.UserID++ },
		"tenant":   func(c *tc.ExecutionCommand) { c.Scope.TenantID++ },
		"standard": func(c *tc.ExecutionCommand) { c.Node.Parameters.SFT.StandardVersionID = "different" },
		"columns":  func(c *tc.ExecutionCommand) { c.Node.Parameters.SFT.Columns.ReferenceAnswer = "other" },
		"deadline": func(c *tc.ExecutionCommand) { c.Deadline = c.Deadline.Add(time.Second) },
	} {
		t.Run(name, func(t *testing.T) {
			c := command()
			mutate(&c)
			_, err := invocation(context.Background(), c)
			require.ErrorIs(t, err, m.ErrConflict)
		})
	}
	c = command()
	c.Node.Inputs[0].Asset = nil
	c.Node.Inputs[0].FromNode = "prepare"
	c.Node.DependsOn = []string{"prepare"}
	seal(&c)
	in, err = invocation(context.Background(), c)
	require.NoError(t, err)
	require.Equal(t, c.InputRefs[0], in.InputRef)
}
func TestSFTNodeCannotApprovePublishOrExpandBudget(t *testing.T) {
	for name, mutate := range map[string]func(*tc.Node){
		"raw":           func(n *tc.Node) { n.Inputs[0].Kind = l.RawAsset },
		"no-standard":   func(n *tc.Node) { n.Parameters.SFT.StandardVersionID = "" },
		"guess-answer":  func(n *tc.Node) { n.Parameters.SFT.Columns.ReferenceAnswer = "" },
		"other-branch":  func(n *tc.Node) { n.Parameters.Feature = &tc.FeatureParameters{} },
		"retry":         func(n *tc.Node) { n.MaxAttempts = 2 },
		"cost":          func(n *tc.Node) { n.MaxCostMicrosPerAttempt = 1 },
		"too-few-calls": func(n *tc.Node) { n.MaxCallsPerAttempt = 2 },
		"timeout":       func(n *tc.Node) { n.TimeoutSeconds = 121 },
		"admin":         func(n *tc.Node) { n.Permissions = []string{"admin"} },
		"published":     func(n *tc.Node) { n.OutputKinds = []l.Kind{l.PublishedAsset} },
	} {
		t.Run(name, func(t *testing.T) {
			c := command()
			mutate(&c.Node)
			require.ErrorIs(t, validateNode(c.Scope, c.Node), m.ErrInvalid)
		})
	}
	a, err := New(&governance.Service{}, &lifecycle.Service{})
	require.NoError(t, err)
	require.Equal(t, "sft", a.Capability().ParameterKind)
	require.False(t, a.Capability().RequiresApproval)
	c := command()
	r := &d.Record{Meta: d.Meta{ID: "operation"}}
	h := handle(c, r)
	require.True(t, validHandle(c, *h, r))
	h.DomainID = "other"
	require.False(t, validHandle(c, *h, r))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = invocation(ctx, c)
	require.ErrorIs(t, err, context.Canceled)
}
