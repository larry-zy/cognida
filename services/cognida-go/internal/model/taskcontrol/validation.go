package taskcontrol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	MaxSnapshotBytes   = 256 << 10
	MaxReferencesBytes = 64 << 10
	MaxBudgetBytes     = 16 << 10
	MaxReceiptBytes    = 64 << 10
)

// CanonicalJSON preserves JSON numbers exactly (including values above 2^53).
// It only validates the persistence envelope; the service owns the typed schema.
func CanonicalJSON(raw json.RawMessage, kind byte, limit int) (json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return nil, ErrInvalid
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != kind {
		return nil, ErrInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, fmt.Errorf("%w: malformed JSON", ErrInvalid)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing JSON", ErrInvalid)
	}
	out, err := json.Marshal(value)
	if err != nil {
		return nil, ErrInvalid
	}
	return out, nil
}

func Digest(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}

func NormalizeCreate(in CreateTaskInput) (CreateTaskInput, string, error) {
	if !utf8.ValidString(in.Goal) || strings.TrimSpace(in.Goal) == "" || utf8.RuneCountInString(in.Goal) > 4000 ||
		!ValidText(in.IdempotencyKey, 256) || len(in.RequestID) > 128 || !utf8.ValidString(in.RequestID) {
		return in, "", ErrInvalid
	}
	var err error
	if in.InputRefs, err = CanonicalJSON(in.InputRefs, '[', MaxReferencesBytes); err != nil {
		return in, "", err
	}
	if in.Budget, err = CanonicalJSON(in.Budget, '{', MaxBudgetBytes); err != nil {
		return in, "", err
	}
	// Request IDs identify attempts, not business intent. A retry may have a new request ID.
	identity, err := json.Marshal(struct {
		Goal   string          `json:"goal"`
		Inputs json.RawMessage `json:"inputs"`
		Budget json.RawMessage `json:"budget"`
	}{in.Goal, in.InputRefs, in.Budget})
	if err != nil {
		return in, "", ErrInvalid
	}
	return in, Digest(identity), nil
}

func ValidText(value string, maxBytes int) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" && len(value) <= maxBytes && !strings.ContainsRune(value, '\x00')
}

func (s Status) Valid() bool {
	switch s {
	case StatusDraft, StatusReady, StatusRunning, StatusAwaitingApproval, StatusPartiallyCompleted,
		StatusCompleted, StatusFailed, StatusTimedOut, StatusCancelRequested, StatusReconcileRequired, StatusCancelled:
		return true
	}
	return false
}

func (s Status) Terminal() bool {
	switch s {
	case StatusPartiallyCompleted, StatusCompleted, StatusFailed, StatusTimedOut, StatusCancelled:
		return true
	}
	return false
}

// CanTransition rejects terminal resurrection and cancellation being disguised
// as a normal successful run. Receipt reconciliation is a separate operation.
func CanTransition(from, to Status) bool {
	if !from.Valid() || !to.Valid() || from.Terminal() {
		return false
	}
	if from == to {
		return true
	}
	if to == StatusCancelRequested || to == StatusReconcileRequired {
		return true
	}
	if from == StatusCancelRequested {
		return to == StatusCancelled || to == StatusPartiallyCompleted || to == StatusFailed || to == StatusTimedOut
	}
	if to.Terminal() {
		return true
	}
	switch from {
	case StatusDraft:
		return to == StatusReady
	case StatusReady:
		return to == StatusDraft || to == StatusRunning || to == StatusAwaitingApproval
	case StatusRunning, StatusAwaitingApproval, StatusReconcileRequired:
		return to == StatusRunning || to == StatusAwaitingApproval
	}
	return false
}

func (s OutboxStatus) Valid() bool {
	switch s {
	case OutboxPending, OutboxDispatching, OutboxReconcileRequired, OutboxDelivered, OutboxCancelled:
		return true
	}
	return false
}
