package taskcontrol

import (
	"context"
	"encoding/json"
	"testing"

	g "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	"github.com/stretchr/testify/require"
)

func TestPlanParametersAreClosedTypedAndBoundToDispatch(t *testing.T) {
	v, _ := testValidator(t)
	plan := testPlan()
	cap := v.capabilities[plan.Nodes[0].Capability]
	cap.ParameterKind = "governance"
	v.capabilities[cap.ID] = cap
	good := &NodeParameters{Governance: &GovernanceParameters{LabelSchema: []g.Label{{ID: "revenue", Name: "营收", Keywords: []string{"收入"}}}}}
	for _, bad := range []*NodeParameters{nil, {}, {Feature: &FeatureParameters{FeatureID: "id", Version: 1}}, {Feature: &FeatureParameters{}, Governance: &GovernanceParameters{}}} {
		plan.Nodes[0].Parameters = bad
		_, err := v.Validate(context.Background(), testScope, plan)
		require.ErrorIs(t, err, d.ErrInvalid)
	}
	plan.Nodes[0].Parameters = good
	_, err := v.Validate(context.Background(), testScope, plan)
	require.NoError(t, err)
	cmd := ExecutionCommand{TaskID: "task", PlanVersion: 1, Node: plan.Nodes[0], Attempt: 1, InputRefs: []l.RefID{testRef}}
	raw, err := json.Marshal(cmd)
	require.NoError(t, err)
	canonical, err := d.CanonicalJSON(raw, '{', d.MaxSnapshotBytes)
	require.NoError(t, err)
	child := d.Child{NodeID: "clean", PlanVersion: 1, Attempt: 1, OperationKey: "operation"}
	event := d.Outbox{OperationKey: "operation", Payload: raw, PayloadHash: d.Digest(canonical)}
	r := &Runtime{}
	_, err = r.command(testScope, "task", plan, child, event)
	require.NoError(t, err)
	plan.Nodes[0].Parameters.Governance.LabelSchema[0].Keywords[0] = "费用"
	_, err = r.command(testScope, "task", plan, child, event)
	require.ErrorIs(t, err, d.ErrInvalid, "rule edits cannot reuse an existing frozen command")
}
