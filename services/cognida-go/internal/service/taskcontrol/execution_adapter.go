package taskcontrol

import (
	"context"
	"encoding/json"
	"time"

	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
)

// ExecutionSupport is trusted server configuration for a bounded preset. Calls
// count logical input resolutions and operations, not transport retries or SQL.
type ExecutionSupport struct {
	Profile           string `json:"profile"`
	BaseCalls         int    `json:"base_calls"`
	CallsPerInput     int    `json:"calls_per_input"`
	MaxTimeoutSeconds int    `json:"max_timeout_seconds"`
}

// ExecutionCommand is persisted in the outbox. Identity, key and payload hash
// are taken from its trusted envelope, never accepted from the HTTP request.
type ExecutionCommand struct {
	Scope        d.Scope   `json:"-"`
	OperationKey string    `json:"-"`
	PayloadHash  string    `json:"-"`
	TaskID       string    `json:"task_id"`
	PlanVersion  int64     `json:"plan_version"`
	Node         Node      `json:"node"`
	Attempt      int       `json:"attempt"`
	InputRefs    []l.RefID `json:"input_refs"`
	Deadline     time.Time `json:"deadline"`
	RequestID    string    `json:"request_id"`
}

type ExecutionHandle struct {
	DomainID string          `json:"domain_id"`
	Receipt  json.RawMessage `json:"receipt"`
}

type DomainObservation struct {
	Status        string    `json:"status"`
	OutputRefs    []l.RefID `json:"output_refs"`
	Error         string    `json:"error,omitempty"`
	ObservedAt    time.Time `json:"observed_at"`
	DomainKind    string    `json:"domain_kind"`
	CallsReserved int       `json:"calls_reserved"`
}

// Lookup returns nil,nil only for an authoritative absence. Submit must be
// atomically idempotent for the same scope/key/payload, including after Lookup;
// this is required before registering an adapter with the runtime.
type ExecutionAdapter interface {
	Support() ExecutionSupport
	Validate(context.Context, d.Scope, Node) error
	ValidateInputs(context.Context, d.Scope, []l.RefID) error
	Submit(context.Context, ExecutionCommand) (*ExecutionHandle, error)
	Lookup(context.Context, ExecutionCommand) (*ExecutionHandle, error)
	// CancelSubmission atomically records cancellation for the same operation
	// even if no job exists yet, fencing a late Submit after an uncertain crash.
	CancelSubmission(context.Context, ExecutionCommand) (*ExecutionHandle, error)
	Inspect(context.Context, ExecutionCommand, ExecutionHandle) (*DomainObservation, error)
	Cancel(context.Context, ExecutionCommand, ExecutionHandle) (*DomainObservation, error)
}

// ExecutionOutputAuthorizer separates result visibility from dispatch input
// cardinality. Source adapters accept no inputs but still produce readable data.
type ExecutionOutputAuthorizer interface {
	ValidateOutputs(context.Context, d.Scope, []l.RefID) error
}

func authorizeExecutionOutputs(ctx context.Context, p d.Scope, adapter ExecutionAdapter, refs []l.RefID) error {
	if adapter == nil {
		return d.ErrForbidden
	}
	if output, ok := adapter.(ExecutionOutputAuthorizer); ok {
		return output.ValidateOutputs(ctx, p, refs)
	}
	// Existing adapters use the same authorized provenance traversal for inputs
	// and outputs. Preserve that boundary until they declare a separate contract.
	return adapter.ValidateInputs(ctx, p, refs)
}
