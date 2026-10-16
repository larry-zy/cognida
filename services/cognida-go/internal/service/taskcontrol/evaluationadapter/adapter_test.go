package evaluationadapter

import (
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	tc "cognida/internal/service/taskcontrol"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func command() tc.ExecutionCommand {
	ref := l.RefID{AssetID: uuid.NewString(), Version: 1}
	return tc.ExecutionCommand{Scope: d.Scope{TenantID: 1, UserID: 2}, TaskID: uuid.NewString(), PlanVersion: 1, Attempt: 1, InputRefs: []l.RefID{ref}, Deadline: time.Now().UTC().Add(time.Minute), Node: tc.Node{ID: "score", Capability: CapabilityID, CapabilityVersion: 1, Inputs: []tc.InputBinding{{Asset: &ref, Kind: l.RawAsset}}, OutputKinds: []l.Kind{l.Result}, Tools: []string{"evaluation.offline-score"}, Permissions: []string{"read"}, Parameters: &tc.NodeParameters{Evaluation: &tc.EvaluationParameters{QuestionColumn: "question", ReferenceAnswerColumn: "reference", GeneratedAnswerColumn: "answer"}}, MaxAttempts: 1, MaxCallsPerAttempt: 6, TimeoutSeconds: 30}}
}
func bind(c *tc.ExecutionCommand) {
	raw, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
	c.OperationKey = d.Digest(raw)
	raw, _ = json.Marshal(c)
	raw, _ = d.CanonicalJSON(raw, '{', d.MaxSnapshotBytes)
	c.PayloadHash = d.Digest(raw)
}
func TestOfflineAdapterOnlyDeclaredPresetAndBoundCommand(t *testing.T) {
	a := &Adapter{}
	cmd := command()
	bind(&cmd)
	req, err := a.request(context.Background(), cmd)
	require.NoError(t, err)
	require.Equal(t, cmd.InputRefs[0], req.InputRef)
	for _, mutate := range []func(*tc.ExecutionCommand){func(c *tc.ExecutionCommand) { c.Scope.UserID++ }, func(c *tc.ExecutionCommand) { c.InputRefs[0].AssetID = uuid.NewString() }, func(c *tc.ExecutionCommand) { c.Deadline = c.Deadline.Add(time.Second) }, func(c *tc.ExecutionCommand) { c.Node.Permissions = []string{"publish"} }, func(c *tc.ExecutionCommand) { c.Node.MaxCallsPerAttempt = 5 }, func(c *tc.ExecutionCommand) { c.Node.TimeoutSeconds = 301 }, func(c *tc.ExecutionCommand) { c.Node.Parameters.Evaluation.GeneratedAnswerColumn = "question" }, func(c *tc.ExecutionCommand) { c.Node.Parameters.Research = &tc.ResearchParameters{Query: "untrusted"} }, func(c *tc.ExecutionCommand) { c.Node.Tools = []string{"evaluation.llm"} }} {
		c := command()
		bind(&c)
		mutate(&c)
		_, err = a.request(context.Background(), c)
		require.Error(t, err)
	}
	cap := a.Capability()
	require.False(t, cap.RequiresApproval)
	require.Equal(t, "evaluation", cap.ParameterKind)
	require.Equal(t, []l.Kind{l.Result}, cap.OutputKinds)
	require.Equal(t, 5, a.Support().BaseCalls)
	require.Equal(t, 1, a.Support().CallsPerInput)
}

func TestReceiptAcceptsDatabaseJSONFormatting(t *testing.T) {
	cmd := tc.ExecutionCommand{OperationKey: "key", PayloadHash: "hash"}
	for _, raw := range []string{`{"payload_hash": "hash", "domain_id": "eval-one", "operation_key": "key"}`, "{\n  \"domain_id\": \"eval-one\", \"operation_key\": \"key\", \"payload_hash\": \"hash\"\n}"} {
		if e := checkHandle(cmd, tc.ExecutionHandle{DomainID: "eval-one", Receipt: json.RawMessage(raw)}); e != nil {
			t.Fatalf("database formatted receipt rejected: %v", e)
		}
	}
	for _, raw := range []string{`{"domain_id":"eval-one","operation_key":"key","payload_hash":"hash","extra":true}`, `{"domain_id":"eval-one","operation_key":"key","payload_hash":"hash","payload_hash":"hash"}`} {
		if e := checkHandle(cmd, tc.ExecutionHandle{DomainID: "eval-one", Receipt: json.RawMessage(raw)}); e == nil {
			t.Fatal("ambiguous or expanded receipt accepted")
		}
	}
}
