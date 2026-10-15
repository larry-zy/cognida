package taskcontrol

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateIdentityPreservesExactJSONAndBusinessIntent(t *testing.T) {
	input := CreateTaskInput{Goal: "分析收入", InputRefs: json.RawMessage(`[{"version":1,"asset_id":"source"}]`), Budget: json.RawMessage(`{"max_cost_micros":9007199254740993,"limit":2}`), IdempotencyKey: "Key-A", RequestID: "request-1"}
	normalized, hash, err := NormalizeCreate(input)
	require.NoError(t, err)
	require.Contains(t, string(normalized.Budget), "9007199254740993")
	input.RequestID = "request-2"
	input.Budget = json.RawMessage(`{ "limit":2, "max_cost_micros":9007199254740993 }`)
	_, retryHash, err := NormalizeCreate(input)
	require.NoError(t, err)
	require.Equal(t, hash, retryHash)
	input.Goal = "另一个目标"
	_, changedHash, err := NormalizeCreate(input)
	require.NoError(t, err)
	require.NotEqual(t, hash, changedHash)
	input.Goal = "分析收入"
	input.Budget = json.RawMessage(`{"max_cost_micros":9007199254740992,"limit":2}`)
	_, changedHash, err = NormalizeCreate(input)
	require.NoError(t, err)
	require.NotEqual(t, hash, changedHash)
	require.NotEqual(t, Digest([]byte("Key-A")), Digest([]byte("key-a")))
}

func TestPersistenceEnvelopeValidation(t *testing.T) {
	valid := CreateTaskInput{Goal: strings.Repeat("字", 4000), InputRefs: json.RawMessage(`[]`), Budget: json.RawMessage(`{}`), IdempotencyKey: "key"}
	_, _, err := NormalizeCreate(valid)
	require.NoError(t, err)
	cases := []struct {
		name   string
		mutate func(*CreateTaskInput)
	}{
		{"long unicode goal", func(in *CreateTaskInput) { in.Goal += "字" }},
		{"empty goal", func(in *CreateTaskInput) { in.Goal = "  " }},
		{"invalid utf8 goal", func(in *CreateTaskInput) { in.Goal = string([]byte{255}) }},
		{"no key", func(in *CreateTaskInput) { in.IdempotencyKey = "" }},
		{"long key", func(in *CreateTaskInput) { in.IdempotencyKey = strings.Repeat("a", 257) }},
		{"nul key", func(in *CreateTaskInput) { in.IdempotencyKey = "a\x00b" }},
		{"references must be array", func(in *CreateTaskInput) { in.InputRefs = json.RawMessage(`{}`) }},
		{"budget must be object", func(in *CreateTaskInput) { in.Budget = json.RawMessage(`[]`) }},
		{"trailing json", func(in *CreateTaskInput) { in.Budget = json.RawMessage(`{} {}`) }},
		{"malformed", func(in *CreateTaskInput) { in.Budget = json.RawMessage(`{"x":`) }},
		{"null", func(in *CreateTaskInput) { in.Budget = json.RawMessage(`null`) }},
		{"oversized", func(in *CreateTaskInput) {
			in.Budget = json.RawMessage(`{"x":"` + strings.Repeat("a", MaxBudgetBytes) + `"}`)
		}},
		{"request too long", func(in *CreateTaskInput) { in.RequestID = strings.Repeat("a", 129) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := valid
			tc.mutate(&in)
			_, _, err := NormalizeCreate(in)
			require.ErrorIs(t, err, ErrInvalid)
		})
	}
	_, err = CanonicalJSON(json.RawMessage{255}, '{', 100)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = CanonicalJSON(json.RawMessage(`   `), '{', 100)
	require.ErrorIs(t, err, ErrInvalid)
}

func TestScopeAndTerminalFencing(t *testing.T) {
	for _, scope := range []Scope{{}, {TenantID: 1}, {UserID: 1}, {TenantID: -1, UserID: 1}} {
		require.ErrorIs(t, scope.Validate(), ErrForbidden)
	}
	require.NoError(t, (Scope{TenantID: 1, UserID: 1}).Validate())
	for _, status := range []Status{StatusCompleted, StatusFailed, StatusTimedOut, StatusPartiallyCompleted, StatusCancelled} {
		require.True(t, status.Terminal())
		for _, to := range []Status{StatusDraft, StatusReady, StatusRunning, StatusCompleted} {
			require.False(t, CanTransition(status, to))
		}
	}
	require.False(t, CanTransition(StatusCancelRequested, StatusCompleted))
	require.False(t, CanTransition(StatusCancelRequested, StatusRunning))
	require.False(t, CanTransition(StatusDraft, StatusRunning))
	require.False(t, CanTransition("unknown", StatusReady))
	require.True(t, CanTransition(StatusDraft, StatusReady))
	require.True(t, CanTransition(StatusRunning, StatusCancelRequested))
	require.True(t, CanTransition(StatusCancelRequested, StatusReconcileRequired))
	require.True(t, CanTransition(StatusCancelRequested, StatusPartiallyCompleted))
	require.True(t, CanTransition(StatusReconcileRequired, StatusRunning))
	require.True(t, CanTransition(StatusReady, StatusAwaitingApproval))
	require.False(t, OutboxStatus("unknown").Valid())
	for _, s := range []OutboxStatus{OutboxPending, OutboxDispatching, OutboxReconcileRequired, OutboxDelivered, OutboxCancelled} {
		require.True(t, s.Valid())
	}
}
