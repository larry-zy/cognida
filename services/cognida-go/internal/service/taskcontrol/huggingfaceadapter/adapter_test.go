package huggingfaceadapter

import (
	d "cognida/internal/model/huggingface"
	l "cognida/internal/model/lifecycle"
	m "cognida/internal/model/taskcontrol"
	hf "cognida/internal/service/huggingface"
	tc "cognida/internal/service/taskcontrol"
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func seal(c *tc.ExecutionCommand) {
	raw, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
	c.OperationKey = m.Digest(raw)
	raw, _ = json.Marshal(c)
	raw, _ = m.CanonicalJSON(raw, '{', m.MaxSnapshotBytes)
	c.PayloadHash = m.Digest(raw)
}
func testCommand() tc.ExecutionCommand {
	c := tc.ExecutionCommand{Scope: m.Scope{TenantID: 1, UserID: 2}, TaskID: "task", PlanVersion: 1, Attempt: 1, Deadline: time.Now().Add(time.Minute), Node: tc.Node{ID: "download", Capability: "huggingface.import", CapabilityVersion: 1, OutputKinds: []l.Kind{l.RawAsset}, Tools: []string{"huggingface.import"}, Permissions: []string{"read"}, MaxAttempts: 1, MaxCallsPerAttempt: d.GovernedCalls, TimeoutSeconds: 60, Parameters: &tc.NodeParameters{HuggingFace: &tc.HFImportParameters{Request: d.Request{RepoID: "org/data", Revision: strings.Repeat("a", 40), Config: "default", Split: "train", Files: []string{"train.jsonl"}, FieldMapping: d.FieldMapping{TextField: "text"}, MaxRows: 3, MaxBytes: 4096}}}}}
	seal(&c)
	return c
}
func TestHFCommandBindsScopeAndEveryParameter(t *testing.T) {
	a, e := New(&hf.Service{})
	require.NoError(t, e)
	require.True(t, a.Capability().AllowEmptyInputs)
	in, e := command(testCommand())
	require.NoError(t, e)
	require.Equal(t, "train", in.Request.Split)
	for _, mutate := range []func(*tc.ExecutionCommand){
		func(c *tc.ExecutionCommand) { c.Scope.UserID++ }, func(c *tc.ExecutionCommand) { c.OperationKey = "wrong" }, func(c *tc.ExecutionCommand) { c.Deadline = c.Deadline.Add(time.Second) }, func(c *tc.ExecutionCommand) { c.Node.Parameters.HuggingFace.Request.MaxRows++ }, func(c *tc.ExecutionCommand) { c.Attempt = 2 }, func(c *tc.ExecutionCommand) { c.InputRefs = []l.RefID{{AssetID: "unexpected", Version: 1}} },
	} {
		c := testCommand()
		mutate(&c)
		_, e := command(c)
		require.Error(t, e)
	}
	_, e = New(nil)
	require.ErrorIs(t, e, m.ErrInvalid)
	_, e = command(tc.ExecutionCommand{})
	require.ErrorIs(t, e, m.ErrForbidden)
	require.ErrorIs(t, a.ValidateInputs(context.Background(), m.Scope{}, nil), m.ErrForbidden)
}
func TestHFValidationRejectsUnboundedOrAmbiguousPlan(t *testing.T) {
	for _, mutate := range []func(*tc.Node){
		func(n *tc.Node) { n.MaxAttempts = 2 }, func(n *tc.Node) { n.MaxCallsPerAttempt = 10 }, func(n *tc.Node) { n.MaxCostMicrosPerAttempt = 1 }, func(n *tc.Node) { n.TimeoutSeconds = 121 }, func(n *tc.Node) { n.Parameters.Collection = &tc.CollectionParameters{} }, func(n *tc.Node) { n.Parameters.HuggingFace.Request.Revision = "main" }, func(n *tc.Node) { n.Parameters.HuggingFace.Request.Files = []string{"../data.jsonl"} }, func(n *tc.Node) { n.OutputKinds = []l.Kind{l.PublishedAsset} }, func(n *tc.Node) { n.Tools = []string{"sql_mutate"} }, func(n *tc.Node) { n.Inputs = []tc.InputBinding{{Kind: l.RawAsset}} }, func(n *tc.Node) { n.Parameters.HuggingFace.Request.MaxBytes = d.MaxDownloadBytes + 1 }, func(n *tc.Node) { n.Parameters.HuggingFace.Request.MaxRows = 1001 },
	} {
		c := testCommand()
		mutate(&c.Node)
		require.ErrorIs(t, validate(c.Node), m.ErrInvalid)
	}
}

func TestHFOutputContractIsSeparateFromZeroInputDispatch(t *testing.T) {
	a, e := New(&hf.Service{})
	require.NoError(t, e)
	var output tc.ExecutionOutputAuthorizer = a
	require.ErrorIs(t, output.ValidateOutputs(context.Background(), m.Scope{}, nil), m.ErrForbidden)
	require.ErrorIs(t, a.ValidateInputs(context.Background(), m.Scope{TenantID: 1, UserID: 2}, []l.RefID{{AssetID: "not-an-input", Version: 1}}), m.ErrInvalid)
}
