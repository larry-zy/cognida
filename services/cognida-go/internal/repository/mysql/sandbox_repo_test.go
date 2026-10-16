package mysql

import (
	"encoding/json"
	"testing"

	d "cognida/internal/model/sandbox"
	"github.com/stretchr/testify/require"
)

func TestSandboxExecutionAdvanceKeepsCleanupSeparateFromPublication(t *testing.T) {
	for _, phase := range []string{"", "launch_committed", "collected", "cleanup_pending"} {
		for _, state := range []d.State{d.StateCancelled, d.StateTimedOut, d.StateFailed} {
			require.False(t, sandboxExecutionAdvance(&d.Record{State: state, Phase: phase}), "%s/%s", state, phase)
		}
		require.True(t, sandboxExecutionAdvance(&d.Record{State: d.StateSucceeded, Phase: phase}), phase)
	}
	for _, phase := range []string{"launch_committed", "collected"} {
		require.True(t, sandboxExecutionAdvance(&d.Record{State: d.StateRunning, Phase: phase}))
		// A terminal label cannot bypass the fence while still retaining output.
		require.True(t, sandboxExecutionAdvance(&d.Record{State: d.StateCancelled, Phase: phase, OutputRefs: json.RawMessage(`[]`)}))
		require.True(t, sandboxExecutionAdvance(&d.Record{State: d.StateFailed, Phase: phase, Manifest: json.RawMessage(`{}`)}))
	}
	require.False(t, sandboxExecutionAdvance(&d.Record{State: d.StateReconcileRequired, Phase: "cleanup_pending", FinishState: d.StateCancelled}))
}
