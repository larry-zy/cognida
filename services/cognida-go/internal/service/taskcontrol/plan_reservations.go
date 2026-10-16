package taskcontrol

// executionMinimumCalls uses only trusted capability metadata. It is shared by
// strict plan validation and unsaved Supervisor candidate compilation.
func executionMinimumCalls(support ExecutionSupport, inputCount int) (int, bool) {
	if support.BaseCalls < 1 || support.BaseCalls > 128 || support.CallsPerInput < 0 || support.CallsPerInput > 128 || inputCount < 0 || inputCount > 16 {
		return 0, false
	}
	minimum := support.BaseCalls + support.CallsPerInput*inputCount
	if minimum > 128 {
		return 0, false
	}
	return minimum, true
}

// Both validation and Supervisor compilation reserve every allowed attempt,
// including retries. Bounds are checked before multiplication; with at most
// 32 nodes the maxima are 160 attempts, 20,480 calls, 160e9 cost micros and
// 576,000 seconds, all safely representable even on a 32-bit platform (cost
// uses int64). These are arithmetic bounds, not permission to spend them.
type nodeReservations struct {
	nodes, attempts, calls, seconds int
	costMicros                      int64
}

func (r *nodeReservations) add(n Node) bool {
	if r.nodes >= 32 || n.MaxAttempts < 1 || n.MaxAttempts > 5 || n.MaxCallsPerAttempt < 1 || n.MaxCallsPerAttempt > 128 || n.MaxCostMicrosPerAttempt < 0 || n.MaxCostMicrosPerAttempt > 1_000_000_000 || n.TimeoutSeconds < 1 || n.TimeoutSeconds > 3600 {
		return false
	}
	r.nodes++
	r.attempts += n.MaxAttempts
	r.calls += n.MaxAttempts * n.MaxCallsPerAttempt
	r.costMicros += int64(n.MaxAttempts) * n.MaxCostMicrosPerAttempt
	r.seconds += n.MaxAttempts * n.TimeoutSeconds
	return true
}
