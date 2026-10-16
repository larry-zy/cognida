package taskcontrol

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	g "cognida/internal/model/governance"
	hf "cognida/internal/model/huggingface"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	"github.com/stretchr/testify/require"
)

func TestSupervisorAllowsNoExistingAssetsOnlyWithExecutableAcquisition(t *testing.T) {
	s, repo, calls := supervisorFixture(t)
	budget := testPlan().Budget
	in := AssessmentInput{Goal: "从明确的固定 HF 文件导入技术支持数据", Budget: &budget}
	result, err := s.Assess(context.Background(), testScope, in)
	require.NoError(t, err)
	require.False(t, result.Ready)
	require.Equal(t, "inputs", result.Questions[0].ID)
	capability := Capability{ID: "huggingface.import", AllowEmptyInputs: true}
	s.tasks.validator.capabilities[capability.ID] = capability
	result, err = s.Assess(context.Background(), testScope, in)
	require.NoError(t, err)
	require.False(t, result.Ready, "a planning-only declaration cannot remove the input requirement")
	capability.Execution = &ExecutionSupport{Profile: "hf-import-v1", BaseCalls: 11, MaxTimeoutSeconds: 120}
	s.tasks.validator.capabilities[capability.ID] = capability
	result, err = s.Assess(context.Background(), testScope, in)
	require.NoError(t, err)
	require.True(t, result.Ready)
	require.Empty(t, result.Questions)
	require.Zero(t, *calls)
	require.Zero(t, repo.creates)
	require.False(t, result.ExecutionEnabled)
	capability.Permissions = []string{"hf.read"}
	s.tasks.validator.capabilities[capability.ID] = capability
	s.tasks.validator.authorize = func(_ context.Context, _ d.Scope, permission string) error {
		if permission == "hf.read" {
			return d.ErrForbidden
		}
		return nil
	}
	result, err = s.Assess(context.Background(), testScope, in)
	require.NoError(t, err)
	require.False(t, result.Ready, "an unauthorized acquisition capability cannot remove the input requirement")
	s.tasks.validator.resolve = func(context.Context, d.Scope, l.RefID) (l.Kind, error) { return "", d.ErrForbidden }
	in.InputRefs = []l.RefID{testRef}
	_, err = s.Assess(context.Background(), testScope, in)
	require.ErrorIs(t, err, d.ErrForbidden, "acquisition does not bypass supplied-source authorization")
}

func TestSupervisorProposesAcquisitionWithoutInventingSourceAssets(t *testing.T) {
	s, repo, _ := supervisorFixture(t)
	repo.task.InputRefs = json.RawMessage(`[]`)
	capability := Capability{
		ID: "huggingface.import", Version: 1, Domain: "huggingface", ParameterKind: "huggingface", AllowEmptyInputs: true,
		OutputKinds: []l.Kind{l.RawAsset}, Tools: []string{"huggingface.import"}, Permissions: []string{"hf.read"},
		Execution: &ExecutionSupport{Profile: "hf-import-v1", BaseCalls: 11, MaxTimeoutSeconds: 120},
	}
	s.tasks.validator.capabilities[capability.ID] = capability
	plan := testPlan()
	plan.Nodes = []Node{{ID: "import", Capability: capability.ID, CapabilityVersion: 1, OutputKinds: capability.OutputKinds, Tools: capability.Tools, Permissions: capability.Permissions,
		Parameters:  &NodeParameters{HuggingFace: &HFImportParameters{Request: hf.Request{RepoID: "nvidia/TechQA-RAG-Eval", Revision: strings.Repeat("a", 40), Config: "default", Split: "train", Files: []string{"train.json"}, FieldMapping: hf.FieldMapping{Columns: []hf.ColumnMapping{{Source: "question", Target: "question"}}}, MaxRows: 2, MaxBytes: 8 << 20}}},
		MaxAttempts: 1, MaxCallsPerAttempt: 11, TimeoutSeconds: 30,
	}}
	calls := 0
	s.planner = plannerFunc(func(_ context.Context, input PlanningInput) (PlannerDecision, error) {
		calls++
		require.Empty(t, input.Sources)
		return PlannerDecision{Kind: "proposal", Explanation: "导入固定公开文件", Plan: &plan}, nil
	})
	result, err := s.Propose(context.Background(), testScope, "draft", ProposeInput{ExpectedVersion: 1})
	require.NoError(t, err)
	require.True(t, result.Validation.Valid)
	require.False(t, result.Saved)
	require.False(t, result.ExecutionEnabled)
	require.Empty(t, result.Decision.Plan.Nodes[0].Inputs)
	require.Equal(t, 1, calls)
	require.Zero(t, repo.creates)
	require.Zero(t, repo.appends)

	s.tasks.validator.authorize = func(_ context.Context, _ d.Scope, permission string) error {
		if permission == "hf.read" {
			return d.ErrForbidden
		}
		return nil
	}
	_, err = s.Propose(context.Background(), testScope, "draft", ProposeInput{ExpectedVersion: 1})
	require.ErrorIs(t, err, d.ErrInvalid)
	require.Equal(t, 1, calls, "revoked source acquisition must be rejected before spending another model call")
	s.tasks.validator.authorize = func(context.Context, d.Scope, string) error { return nil }
	capability.Execution = nil
	s.tasks.validator.capabilities[capability.ID] = capability
	_, err = s.Propose(context.Background(), testScope, "draft", ProposeInput{ExpectedVersion: 1})
	require.ErrorIs(t, err, d.ErrInvalid)
	require.Equal(t, 1, calls)
}

func TestHFAndSFTParametersRemainExclusiveAndBoundToDispatch(t *testing.T) {
	request := hf.Request{RepoID: "nvidia/TechQA-RAG-Eval", Revision: strings.Repeat("a", 40), Config: "default", Split: "train", Files: []string{"train.json"}, FieldMapping: hf.FieldMapping{Columns: []hf.ColumnMapping{{Source: "question", Target: "question"}, {Source: "answer", Target: "reference_answer"}}}, MaxRows: 2, MaxBytes: 8 << 20}
	require.NoError(t, request.Validate())
	for _, test := range []struct {
		kind   string
		params *NodeParameters
		mutate func(*NodeParameters)
	}{
		{"huggingface", &NodeParameters{HuggingFace: &HFImportParameters{Request: request}}, func(p *NodeParameters) { p.HuggingFace.Request.Revision = strings.Repeat("b", 40) }},
		{"sft", &NodeParameters{SFT: &SFTParameters{Name: "Technical support", StandardVersionID: "standard-version", Format: "qa", Columns: g.SFTColumns{Question: "question", ReferenceAnswer: "reference_answer"}}}, func(p *NodeParameters) { p.SFT.StandardVersionID = "another-standard" }},
	} {
		t.Run(test.kind, func(t *testing.T) {
			require.True(t, HasOnlyParameter(test.params, test.kind))
			mixed := *test.params
			mixed.Research = &ResearchParameters{Query: "unexpected branch"}
			require.False(t, HasOnlyParameter(&mixed, test.kind))
			plan := testPlan()
			plan.Nodes[0].Parameters = test.params
			cmd := ExecutionCommand{TaskID: "task", PlanVersion: 1, Node: plan.Nodes[0], Attempt: 1, InputRefs: []l.RefID{testRef}}
			raw, err := json.Marshal(cmd)
			require.NoError(t, err)
			canonical, err := d.CanonicalJSON(raw, '{', d.MaxSnapshotBytes)
			require.NoError(t, err)
			child := d.Child{NodeID: "clean", PlanVersion: 1, Attempt: 1, OperationKey: "operation"}
			event := d.Outbox{OperationKey: "operation", Payload: raw, PayloadHash: d.Digest(canonical)}
			runtime := &Runtime{}
			_, err = runtime.command(testScope, "task", plan, child, event)
			require.NoError(t, err)
			test.mutate(plan.Nodes[0].Parameters)
			_, err = runtime.command(testScope, "task", plan, child, event)
			require.ErrorIs(t, err, d.ErrInvalid, "changing fixed source/standard cannot reuse a persisted command")
		})
	}
}

func TestPlannerContractUsesExactHFRequestFields(t *testing.T) {
	start := strings.Index(supervisorPlannerPrompt, `{"huggingface":{"request":`)
	require.GreaterOrEqual(t, start, 0)
	end := strings.Index(supervisorPlannerPrompt[start:], "，具体字段")
	require.Positive(t, end)
	raw := []byte(supervisorPlannerPrompt[start : start+end])
	var params NodeParameters
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(&params), "the documented model example must match the typed contract")
	require.NotNil(t, params.HuggingFace)
	require.Len(t, params.HuggingFace.Request.FieldMapping.Columns, 2)
}

type dataWorkflowSummaryRepository struct {
	d.RuntimeRepository
	source *supervisorRepository
}

func (r *dataWorkflowSummaryRepository) GetPlan(ctx context.Context, p d.Scope, id string, version int64) (*d.Plan, error) {
	return r.source.GetPlan(ctx, p, id, version)
}
func (r *dataWorkflowSummaryRepository) ListChildren(ctx context.Context, p d.Scope, id string) ([]d.Child, error) {
	return r.source.ListChildren(ctx, p, id)
}

func TestRuntimeSummaryRestoresHFAndSFTDomainLinksWithoutExecuting(t *testing.T) {
	s, stored, _ := supervisorFixture(t)
	stored.task.CurrentPlanVersion, stored.task.ExecutionPlanVersion = 1, 1
	stored.task.Status = d.StatusCompleted
	plan := PlanSpec{Nodes: []Node{{ID: "import", Capability: "huggingface.import"}, {ID: "sft", Capability: "governance.sft"}}}
	raw, err := json.Marshal(plan)
	require.NoError(t, err)
	stored.plan = &d.Plan{TaskID: "draft", Version: 1, Snapshot: raw}
	stored.children = []d.Child{
		{ID: "hf-child", NodeID: "import", PlanVersion: 1, Attempt: 1, Domain: "huggingface", DomainID: "persisted-import", Status: d.ChildBound, DomainStatus: d.DomainSucceeded, Receipt: json.RawMessage(`{"id":"persisted-import"}`), CallsReserved: 11},
		{ID: "sft-child", NodeID: "sft", PlanVersion: 1, Attempt: 1, Domain: "governance", DomainID: "persisted-execution", Status: d.ChildBound, DomainStatus: d.DomainSucceeded, Receipt: json.RawMessage(`{"id":"persisted-execution"}`), CallsReserved: 3},
	}
	// No executable adapter is installed. Reading the persisted summary must
	// still restore links while never dispatching, polling or constructing data.
	runtime := &Runtime{tasks: s.tasks, repo: &dataWorkflowSummaryRepository{source: stored}, enabled: true}
	summary := &SupervisorSummary{TaskID: "draft", TaskVersion: stored.task.Version}
	require.NoError(t, runtime.DecorateSummary(context.Background(), testScope, summary))
	require.Equal(t, "huggingface_import", summary.Nodes[0].DomainKind)
	require.Equal(t, "persisted-import", summary.Nodes[0].DomainID)
	require.Equal(t, "sft_execution", summary.Nodes[1].DomainKind)
	require.Equal(t, "persisted-execution", summary.Nodes[1].DomainID)
	require.Equal(t, "huggingface_import", summary.Children[0].DomainKind)
	require.Equal(t, "sft_execution", summary.Children[1].DomainKind)
	require.True(t, summary.Children[0].ReceiptPresent)
	require.True(t, summary.Children[1].ReceiptPresent)
	require.Equal(t, 14, summary.Usage.ReservedCalls)
	require.Zero(t, stored.creates)
	require.Zero(t, stored.appends)
}
