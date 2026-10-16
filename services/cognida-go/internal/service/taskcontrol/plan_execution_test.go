package taskcontrol

import (
	"context"
	"testing"

	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	"github.com/stretchr/testify/require"
)

func TestExecutionPlanBudgetCheckedBeforeResolvingSources(t *testing.T) {
	for _, scenario := range []struct {
		name, code     string
		calls, timeout int
	}{
		{"base calls omit input", "execution_call_budget", 2, 10},
		{"timeout above profile", "execution_timeout_limit", 3, 11},
		{"exact execution minimum", "", 3, 10},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			caps := testCapabilities()
			caps[0].Execution = &ExecutionSupport{Profile: "test-clean-v1", BaseCalls: 2, CallsPerInput: 1, MaxTimeoutSeconds: 10}
			resolved := 0
			validator, err := NewValidator(caps, func(context.Context, d.Scope, string) error { return nil }, func(context.Context, d.Scope, l.RefID) (l.Kind, error) { resolved++; return l.RawAsset, nil })
			require.NoError(t, err)
			plan := testPlan()
			plan.Nodes[0].MaxCallsPerAttempt = scenario.calls
			plan.Nodes[0].TimeoutSeconds = scenario.timeout
			result, err := validator.Validate(context.Background(), testScope, plan)
			if scenario.code == "" {
				require.NoError(t, err)
				require.True(t, result.Valid)
				require.Equal(t, 7, result.ReservedCalls, "minimum also applies to every reserved retry")
				require.Equal(t, 1, resolved)
				return
			}
			var validation *ValidationError
			require.ErrorAs(t, err, &validation)
			codes := []string{}
			for _, issue := range validation.Issues {
				require.Equal(t, "clean", issue.NodeID)
				codes = append(codes, issue.Code)
			}
			require.Contains(t, codes, scenario.code)
			require.Zero(t, resolved, "invalid hand-authored plans cannot reach source reads or execution")
		})
	}
}

func TestExecutionBudgetCountsDynamicInputsAndProtectsRegistry(t *testing.T) {
	caps := testCapabilities()
	caps[1].Execution = &ExecutionSupport{Profile: "test-publish-v1", BaseCalls: 2, CallsPerInput: 1, MaxTimeoutSeconds: 10}
	validator, err := NewValidator(caps, func(context.Context, d.Scope, string) error { return nil }, func(context.Context, d.Scope, l.RefID) (l.Kind, error) { return l.RawAsset, nil })
	require.NoError(t, err)
	// Neither the supplied pointer nor a public registry copy may weaken limits.
	caps[1].Execution.BaseCalls = 0
	listed := validator.Capabilities()
	listed[1].Execution.CallsPerInput = 0
	plan := testPlan()
	plan.Nodes[1].MaxCallsPerAttempt = 2
	_, err = validator.Validate(context.Background(), testScope, plan)
	var validation *ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, "execution_call_budget", validation.Issues[0].Code)
	plan.Nodes[1].MaxCallsPerAttempt = 3
	_, err = validator.Validate(context.Background(), testScope, plan)
	require.NoError(t, err, "from_node inputs reserve the same resolution calls as fixed assets")
}
