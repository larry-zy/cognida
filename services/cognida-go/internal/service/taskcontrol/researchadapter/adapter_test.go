package researchadapter

import (
	l "cognida/internal/model/lifecycle"
	r "cognida/internal/model/researchrun"
	d "cognida/internal/model/taskcontrol"
	tc "cognida/internal/service/taskcontrol"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func validNode() tc.Node {
	c := Capability()
	return tc.Node{ID: "research", Capability: c.ID, CapabilityVersion: c.Version, OutputKinds: c.OutputKinds, Tools: c.Tools, Permissions: c.Permissions, MaxAttempts: 1, MaxCallsPerAttempt: r.MaxCalls, TimeoutSeconds: 120, Parameters: &tc.NodeParameters{Research: &tc.ResearchParameters{Query: "bounded question", KBIDs: []string{"allowed"}}}}
}
func TestResearchAdapterRejectsUnboundedOrAmbiguousPlans(t *testing.T) {
	n := validNode()
	_, err := request(n)
	require.NoError(t, err)
	for name, change := range map[string]func(*tc.Node){"retry paid": func(n *tc.Node) { n.MaxAttempts = 2 }, "arbitrary input": func(n *tc.Node) {
		n.Inputs = []tc.InputBinding{{Asset: &l.RefID{AssetID: "input", Version: 1}, Kind: l.RawAsset}}
	}, "false cost guarantee": func(n *tc.Node) { n.MaxCostMicrosPerAttempt = 1 }, "extra tools": func(n *tc.Node) { n.Tools = append(n.Tools, "web_search") }, "union smuggling": func(n *tc.Node) { n.Parameters.Collection = &tc.CollectionParameters{} }, "empty scope": func(n *tc.Node) { n.Parameters.Research.KBIDs = nil }, "wrong output": func(n *tc.Node) { n.OutputKinds = []l.Kind{l.RawAsset} }} {
		t.Run(name, func(t *testing.T) { node := validNode(); change(&node); _, e := request(node); require.Error(t, e) })
	}
}
func TestResearchCommandBindsPaidAttemptAndTypedQuery(t *testing.T) {
	c := tc.ExecutionCommand{Scope: d.Scope{TenantID: 7, UserID: 9}, TaskID: "task", PlanVersion: 1, Node: validNode(), Attempt: 1, Deadline: time.Now().Add(time.Minute)}
	identity, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
	c.OperationKey = d.Digest(identity)
	raw, _ := json.Marshal(c)
	raw, _ = d.CanonicalJSON(raw, '{', d.MaxSnapshotBytes)
	c.PayloadHash = d.Digest(raw)
	in, e := command(c)
	require.NoError(t, e)
	require.Equal(t, "bounded question", in.Request.Query)
	c.Attempt = 2
	_, e = command(c)
	require.Error(t, e)
	c.Attempt = 1
	c.InputRefs = []l.RefID{{AssetID: "fake", Version: 1}}
	_, e = command(c)
	require.Error(t, e)
}
