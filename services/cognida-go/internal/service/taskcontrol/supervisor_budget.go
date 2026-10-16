package taskcontrol

// BudgetAdjustment is a controller-produced arithmetic explanation. It never
// grants new task authority or changes a node's operation, retries or limits.
type BudgetAdjustment struct {
	Field  string `json:"field"`
	Before int64  `json:"before"`
	After  int64  `json:"after"`
	Reason string `json:"reason"`
}

// compileSupervisorBudget only fills underestimated aggregate reservations in
// an otherwise bounded candidate. The caller must still run full capability,
// source, permission and DAG validation before exposing the compiled plan.
// Direct AppendPlan requests never use this normalization.
func compileSupervisorBudget(plan PlanSpec, ceiling Budget) (PlanSpec, []BudgetAdjustment, error) {
	if !plannerPlanShape(plan) || !validBudget(ceiling) {
		return PlanSpec{}, nil, ErrPlannerOutput
	}
	exceeded := func() (PlanSpec, []BudgetAdjustment, error) {
		return PlanSpec{}, nil, &ValidationError{[]Issue{{Field: "budget", Code: "task_budget_exceeded", Message: "proposal reservations cannot expand the task budget"}}}
	}
	if !withinBudget(plan.Budget, ceiling) {
		return exceeded()
	}
	var reserved nodeReservations
	for _, node := range plan.Nodes {
		if !reserved.add(node) {
			return PlanSpec{}, nil, ErrPlannerOutput
		}
	}
	var adjustments []BudgetAdjustment
	raise := func(field string, before, minimum int64) int64 {
		if minimum > before {
			adjustments = append(adjustments, BudgetAdjustment{Field: field, Before: before, After: minimum, Reason: "node_retry_reservations"})
			return minimum
		}
		return before
	}
	// PlanSpec and Budget are values. Nodes and their backing data are only read.
	compiled := plan
	compiled.Budget.MaxAttempts = int(raise("max_attempts", int64(plan.Budget.MaxAttempts), int64(reserved.attempts)))
	compiled.Budget.MaxCalls = int(raise("max_calls", int64(plan.Budget.MaxCalls), int64(reserved.calls)))
	compiled.Budget.MaxCostMicros = raise("max_cost_micros", plan.Budget.MaxCostMicros, reserved.costMicros)
	compiled.Budget.TimeoutSeconds = int(raise("timeout_seconds", int64(plan.Budget.TimeoutSeconds), int64(reserved.seconds)))
	if !withinBudget(compiled.Budget, ceiling) {
		return exceeded()
	}
	return compiled, adjustments, nil
}
