package taskcontrol

import (
	"context"
	"errors"
	"testing"

	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	lifecyclesvc "cognida/internal/service/lifecycle"
	"github.com/stretchr/testify/require"
)

var testScope = d.Scope{TenantID: 7, UserID: 11}
var testRef = l.RefID{AssetID: "11111111-1111-4111-8111-111111111111", Version: 1}

func testCapabilities() []Capability {
	return []Capability{
		{ID: "lifecycle.clean", Version: 1, Domain: "lifecycle", InputKinds: []l.Kind{l.RawAsset}, OutputKinds: []l.Kind{l.CleanedAsset}, Tools: []string{"clean"}, Permissions: []string{"read"}},
		{ID: "lifecycle.publish", Version: 1, Domain: "lifecycle", InputKinds: []l.Kind{l.CleanedAsset}, OutputKinds: []l.Kind{l.PublishedAsset}, Tools: []string{"publish"}, Permissions: []string{"read", "write"}, RequiresApproval: true},
	}
}
func testPlan() PlanSpec {
	return PlanSpec{SchemaVersion: 1, Budget: Budget{MaxNodes: 4, MaxConcurrency: 2, MaxAttempts: 6, MaxCalls: 20, MaxCostMicros: 100, TimeoutSeconds: 60}, Nodes: []Node{
		{ID: "clean", Capability: "lifecycle.clean", CapabilityVersion: 1, Inputs: []InputBinding{{Asset: &testRef, Kind: l.RawAsset}}, OutputKinds: []l.Kind{l.CleanedAsset}, Tools: []string{"clean"}, Permissions: []string{"read"}, MaxAttempts: 2, MaxCallsPerAttempt: 2, MaxCostMicrosPerAttempt: 10, TimeoutSeconds: 10},
		{ID: "publish", Capability: "lifecycle.publish", CapabilityVersion: 1, DependsOn: []string{"clean"}, Inputs: []InputBinding{{FromNode: "clean", Kind: l.CleanedAsset}}, OutputKinds: []l.Kind{l.PublishedAsset}, Tools: []string{"publish"}, Permissions: []string{"read", "write"}, MaxAttempts: 1, MaxCallsPerAttempt: 1, MaxCostMicrosPerAttempt: 10, TimeoutSeconds: 10},
	}}
}
func testValidator(t *testing.T) (*Validator, *int) {
	t.Helper()
	calls := 0
	v, e := NewValidator(testCapabilities(), func(context.Context, d.Scope, string) error { return nil }, func(_ context.Context, p d.Scope, r l.RefID) (l.Kind, error) {
		calls++
		if p != testScope || r != testRef {
			return "", d.ErrForbidden
		}
		return l.RawAsset, nil
	})
	require.NoError(t, e)
	return v, &calls
}

func TestPlanValidationReservesRetriesAndDerivesApproval(t *testing.T) {
	v, calls := testValidator(t)
	result, e := v.Validate(context.Background(), testScope, testPlan())
	require.NoError(t, e)
	require.True(t, result.Valid)
	require.False(t, result.ExecutionEnabled)
	require.False(t, result.OriginRevalidated)
	require.Equal(t, "lifecycle_asset", result.AuthorizationScope)
	require.Equal(t, []string{"clean", "publish"}, result.TopologicalOrder)
	require.Equal(t, []string{"publish"}, result.ApprovalRequired)
	require.Equal(t, 3, result.ReservedAttempts)
	require.Equal(t, 5, result.ReservedCalls)
	require.EqualValues(t, 30, result.ReservedCostMicros)
	require.Equal(t, 30, result.ReservedSeconds)
	require.Equal(t, 1, *calls)
}

func TestPlanRejectsInvalidStructureBeforeAssetResolution(t *testing.T) {
	cases := []struct {
		name   string
		change func(*PlanSpec)
		code   string
	}{
		{"schema", func(p *PlanSpec) { p.SchemaVersion = 2 }, "unsupported_schema"},
		{"unknown", func(p *PlanSpec) { p.Nodes[0].Capability = "arbitrary.exec" }, "unknown_capability"},
		{"old capability", func(p *PlanSpec) { p.Nodes[0].CapabilityVersion = 2 }, "unknown_capability"},
		{"duplicate node", func(p *PlanSpec) { p.Nodes[1].ID = "clean" }, "duplicate_node"},
		{"tool escalation", func(p *PlanSpec) { p.Nodes[0].Tools = append(p.Nodes[0].Tools, "sql_mutate") }, "tool_mismatch"},
		{"omit write", func(p *PlanSpec) { p.Nodes[1].Permissions = []string{"read"} }, "scope_mismatch"},
		{"missing predecessor", func(p *PlanSpec) { p.Nodes[1].DependsOn = []string{"missing"} }, "invalid_dependency"},
		{"cycle", func(p *PlanSpec) { p.Nodes[0].DependsOn = []string{"publish"} }, "cycle"},
		{"unbound producer", func(p *PlanSpec) { p.Nodes[1].DependsOn = nil }, "unbound_output"},
		{"fake output", func(p *PlanSpec) { p.Nodes[0].OutputKinds = []l.Kind{l.PublishedAsset} }, "output_kind_mismatch"},
		{"dual binding", func(p *PlanSpec) { p.Nodes[0].Inputs[0].FromNode = "publish" }, "ambiguous_input"},
		{"retry cost", func(p *PlanSpec) { p.Budget.MaxCostMicros = 20 }, "budget_exceeded"},
		{"retry calls", func(p *PlanSpec) { p.Budget.MaxCalls = 3 }, "budget_exceeded"},
		{"timeout", func(p *PlanSpec) { p.Budget.TimeoutSeconds = 20 }, "budget_exceeded"},
		{"overflow", func(p *PlanSpec) { p.Nodes[0].MaxCostMicrosPerAttempt = 1 << 62 }, "invalid_node_budget"},
		{"missing input", func(p *PlanSpec) { p.Nodes[0].Inputs = nil }, "invalid_inputs"},
		{"duplicate tool", func(p *PlanSpec) { p.Nodes[0].Tools = []string{"clean", "clean"} }, "tool_mismatch"},
		{"unbounded parallel", func(p *PlanSpec) { p.Budget.MaxConcurrency = 50 }, "invalid_budget"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			v, calls := testValidator(t)
			plan := testPlan()
			tt.change(&plan)
			_, e := v.Validate(context.Background(), testScope, plan)
			var ve *ValidationError
			require.ErrorAs(t, e, &ve)
			require.ErrorIs(t, e, d.ErrInvalid)
			found := false
			for _, issue := range ve.Issues {
				if issue.Code == tt.code {
					found = true
				}
			}
			require.True(t, found, "issues=%+v", ve.Issues)
			require.Zero(t, *calls)
		})
	}
}

func TestPlanRechecksIdentityPermissionAndSourceKind(t *testing.T) {
	_, e := NewValidator(testCapabilities(), nil, nil)
	require.Error(t, e)
	v, _ := testValidator(t)
	_, e = v.Validate(context.Background(), d.Scope{}, testPlan())
	require.ErrorIs(t, e, d.ErrForbidden)
	v.authorize = func(_ context.Context, _ d.Scope, scope string) error {
		if scope == "write" {
			return errors.New("revoked")
		}
		return nil
	}
	_, e = v.Validate(context.Background(), testScope, testPlan())
	require.ErrorIs(t, e, d.ErrForbidden)
	v, _ = testValidator(t)
	v.resolve = func(context.Context, d.Scope, l.RefID) (l.Kind, error) { return l.PublishedAsset, nil }
	_, e = v.Validate(context.Background(), testScope, testPlan())
	var ve *ValidationError
	require.ErrorAs(t, e, &ve)
	require.Equal(t, "source_kind_mismatch", ve.Issues[0].Code)
	v.resolve = func(context.Context, d.Scope, l.RefID) (l.Kind, error) { return "", errors.New("storage unavailable") }
	_, e = v.Validate(context.Background(), testScope, testPlan())
	require.ErrorIs(t, e, d.ErrForbidden)
}

func TestCapabilityRegistryIsImmutable(t *testing.T) {
	capabilities := testCapabilities()
	capabilities[0].RequiredAnyInputKinds = []l.Kind{l.RawAsset}
	v, e := NewValidator(capabilities, func(context.Context, d.Scope, string) error { return nil }, func(context.Context, d.Scope, l.RefID) (l.Kind, error) { return l.RawAsset, nil })
	require.NoError(t, e)
	capabilities[0].Tools[0] = "root_shell"
	capabilities[0].RequiredAnyInputKinds[0] = l.PublishedAsset
	listed := v.Capabilities()
	listed[0].Tools[0] = "root_shell"
	listed[0].RequiredAnyInputKinds[0] = l.PublishedAsset
	_, e = v.Validate(context.Background(), testScope, testPlan())
	require.NoError(t, e)
}

func TestLifecyclePlansRespectActualAdapterInputAndOutputTypes(t *testing.T) {
	caps := LifecycleCapabilities(lifecyclesvc.Contracts())
	byID := map[string]Capability{}
	for _, c := range caps {
		byID[c.ID] = c
	}
	node := func(id, capability string, inputs []InputBinding, outputs ...l.Kind) Node {
		c := byID["lifecycle."+capability]
		return Node{ID: id, Capability: c.ID, CapabilityVersion: c.Version, Inputs: inputs, OutputKinds: outputs, Tools: c.Tools, Permissions: c.Permissions, MaxAttempts: 1, MaxCallsPerAttempt: 2, TimeoutSeconds: 10}
	}
	raw := InputBinding{Asset: &testRef, Kind: l.RawAsset}
	quality := node("quality", "data-quality", []InputBinding{raw}, l.RawAsset, l.QualityReport)
	clean := node("clean", "data-cleaning", []InputBinding{{FromNode: "quality", Kind: l.RawAsset}, {FromNode: "quality", Kind: l.QualityReport}}, l.CleanedAsset, l.QualityReport)
	clean.DependsOn = []string{"quality"}
	base := testPlan()
	base.Nodes = []Node{quality, clean}
	cases := []struct {
		name   string
		change func(*PlanSpec)
		code   string
	}{
		{"valid quality and cleaning", func(*PlanSpec) {}, ""},
		{"quality cannot publish", func(p *PlanSpec) { p.Nodes[0].OutputKinds = []l.Kind{l.PublishedAsset, l.RawAsset, l.QualityReport} }, "passthrough_kind_mismatch"},
		{"clean cannot consume report alone", func(p *PlanSpec) { p.Nodes[1].Inputs = []InputBinding{{FromNode: "quality", Kind: l.QualityReport}} }, "required_input_missing"},
		{"quality only returns first data input", func(p *PlanSpec) {
			p.Nodes[0].Inputs = append(p.Nodes[0].Inputs, InputBinding{Asset: &testRef, Kind: l.PublishedAsset})
			p.Nodes[0].OutputKinds = append(p.Nodes[0].OutputKinds, l.PublishedAsset)
		}, "passthrough_kind_mismatch"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			resolved := 0
			v, err := NewValidator(caps, func(context.Context, d.Scope, string) error { return nil }, func(context.Context, d.Scope, l.RefID) (l.Kind, error) { resolved++; return l.RawAsset, nil })
			require.NoError(t, err)
			p := base
			p.Nodes = append([]Node(nil), base.Nodes...)
			tt.change(&p)
			_, err = v.Validate(context.Background(), testScope, p)
			if tt.code == "" {
				require.NoError(t, err)
				require.Equal(t, 1, resolved)
				return
			}
			var ve *ValidationError
			require.ErrorAs(t, err, &ve)
			codes := []string{}
			for _, issue := range ve.Issues {
				codes = append(codes, issue.Code)
			}
			require.Contains(t, codes, tt.code)
			require.Zero(t, resolved)
		})
	}
}

func TestTrustedCapabilityConfigurationRejectsUnsafeDeclarations(t *testing.T) {
	for _, mutate := range []func(*Capability){
		func(c *Capability) { c.Tools = []string{"clean", "clean"} },
		func(c *Capability) { c.Permissions = []string{""} },
		func(c *Capability) { c.Permissions = []string{"write"}; c.RequiresApproval = false },
		func(c *Capability) { c.RequiredAnyInputKinds = []l.Kind{l.PublishedAsset} },
		func(c *Capability) { c.FirstInputPassthroughKinds = []l.Kind{l.CleanedAsset} },
	} {
		caps := testCapabilities()
		mutate(&caps[0])
		_, err := NewValidator(caps, func(context.Context, d.Scope, string) error { return nil }, func(context.Context, d.Scope, l.RefID) (l.Kind, error) { return l.RawAsset, nil })
		require.ErrorIs(t, err, d.ErrInvalid)
	}
}
