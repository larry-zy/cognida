package sandboxadapter

import (
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/sandbox"
	m "cognida/internal/model/taskcontrol"
	tc "cognida/internal/service/taskcontrol"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func command() tc.ExecutionCommand {
	cap := (&Adapter{}).Capability()
	ref := l.RefID{AssetID: "00000000-0000-4000-8000-000000000001", Version: 1}
	c := tc.ExecutionCommand{Scope: m.Scope{TenantID: 1, UserID: 2}, TaskID: "task", PlanVersion: 1, Attempt: 1, Deadline: time.Now().Add(time.Minute), InputRefs: []l.RefID{ref}, Node: tc.Node{ID: "sandbox", Capability: cap.ID, CapabilityVersion: 1, Inputs: []tc.InputBinding{{Asset: &ref, Kind: l.RawAsset}}, OutputKinds: cap.OutputKinds, Tools: cap.Tools, Permissions: cap.Permissions, MaxAttempts: 1, MaxCallsPerAttempt: 19, TimeoutSeconds: 60, Parameters: &tc.NodeParameters{Sandbox: &tc.SandboxParameters{ProfileID: "python", ProfileVersion: 1, ImageSHA256: "sha256:" + strings.Repeat("a", 64), Code: d.InputRef{Name: "code", Ref: ref, SHA256: "sha256:" + strings.Repeat("b", 64), SizeBytes: 10}, Resources: d.ResourceLimits{TimeoutSeconds: 60, OutputFiles: 16}}}}}
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
func TestSandboxParentCommandBindsDeadlineCodeAndOwner(t *testing.T) {
	a := &Adapter{}
	c := command()
	_, e := a.command(c)
	require.NoError(t, e)
	for _, change := range []func(*tc.ExecutionCommand){func(c *tc.ExecutionCommand) { c.Scope.UserID++ }, func(c *tc.ExecutionCommand) { c.Deadline = c.Deadline.Add(time.Second) }, func(c *tc.ExecutionCommand) {
		c.Node.Parameters.Sandbox.Code.SHA256 = "sha256:" + strings.Repeat("f", 64)
	}, func(c *tc.ExecutionCommand) { c.Node.Parameters.Feature = &tc.FeatureParameters{} }, func(c *tc.ExecutionCommand) { c.InputRefs = nil }} {
		c := command()
		change(&c)
		_, e = a.command(c)
		require.Error(t, e)
	}
}
func TestSandboxBoundedProfileCannotBecomePublisherOrRetry(t *testing.T) {
	for _, change := range []func(*tc.Node){func(n *tc.Node) { n.MaxAttempts = 2 }, func(n *tc.Node) { n.MaxCallsPerAttempt = 18 }, func(n *tc.Node) { n.Permissions = []string{"read"} }, func(n *tc.Node) { n.OutputKinds = []l.Kind{l.PublishedAsset} }, func(n *tc.Node) { n.Parameters.Sandbox.Resources.OutputFiles = 17 }, func(n *tc.Node) { n.Parameters.Sandbox.Resources.TimeoutSeconds = 59 }, func(n *tc.Node) { n.Inputs[0].Asset = nil; n.Inputs[0].FromNode = "other" }} {
		c := command()
		change(&c.Node)
		_, e := request(c.Node, c.OperationKey)
		require.Error(t, e)
	}
}

func TestSandboxCleanupIsNotAParentTerminalFailure(t *testing.T) {
	require.Equal(t, m.DomainRunning, domainStatus(d.StateReconcileRequired))
	require.Equal(t, m.DomainRunning, domainStatus("cleanup_pending"))
	require.Equal(t, m.DomainTimedOut, domainStatus(d.StateTimedOut))
	require.Equal(t, m.DomainCancelled, domainStatus(d.StateCancelled))
}
