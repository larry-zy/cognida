package taskcontrol

import (
	"context"
	"encoding/json"
	"time"
)

// ExecutionFence is bound to one parent task and one lease epoch. A worker
// must supply it on every execution mutation; an expired owner cannot continue
// after another worker has taken over. It is never accepted from an HTTP body.
type ExecutionFence struct {
	TaskID string `json:"-"`
	Owner  string `json:"-"`
	Epoch  int64  `json:"-"`
}

type TaskLease struct {
	Scope
	TaskID     string    `json:"task_id"`
	Owner      string    `json:"owner"`
	Epoch      int64     `json:"epoch"`
	Until      time.Time `json:"until"`
	NextPollAt time.Time `json:"next_poll_at"`
}

func (l TaskLease) Fence() ExecutionFence {
	return ExecutionFence{TaskID: l.TaskID, Owner: l.Owner, Epoch: l.Epoch}
}

type StartExecutionInput struct {
	PlanVersion    int64
	IdempotencyKey string
	TimeoutSeconds int
}

const (
	DomainUnknown          = "unknown"
	DomainQueued           = "queued"
	DomainRunning          = "running"
	DomainAwaitingApproval = "awaiting_approval"
	DomainSucceeded        = "succeeded"
	DomainFailed           = "failed"
	DomainTimedOut         = "timed_out"
	DomainCancelled        = "cancelled"
	DomainQuarantined      = "quarantined"
)

// ChildObservation is an authoritative domain read, not a model prediction.
// ObservedAt is the domain record's revision time. Dispatch status and its real
// receipt remain separate from domain execution status and output references.
type ChildObservation struct {
	DomainID     string
	DomainStatus string
	OutputRefs   json.RawMessage
	Error        string
	ObservedAt   time.Time
	Fence        ExecutionFence
}

// RunnableTask contains only the scope needed for a subsequent scoped claim.
// The scanner is a privileged worker interface and must not be routed to HTTP.
type RunnableTask struct {
	Scope
	ID      string
	Version int64
}

type RunnableTaskPage struct {
	Items      []RunnableTask
	NextCursor string
}

type RuntimeRepository interface {
	Repository
	BeginExecution(context.Context, Scope, string, int64, StartExecutionInput) (*Task, error)
	ClaimTask(context.Context, Scope, string, string, time.Duration) (*TaskLease, error)
	RenewTaskLease(context.Context, Scope, string, ExecutionFence, time.Duration) (*TaskLease, error)
	ReleaseTaskLease(context.Context, Scope, string, ExecutionFence, time.Time) error
	ListRunnable(context.Context, ListFilter) (*RunnableTaskPage, error)
	ObserveChild(context.Context, Scope, string, int64, int, ChildObservation) (*Child, error)
	FinalizeTask(context.Context, Scope, string, int64, TaskUpdate) (*Task, error)
	ClaimOutboxWithLease(context.Context, Scope, string, int64, ExecutionFence) (*Outbox, error)
	MarkDispatchUnknownWithLease(context.Context, Scope, string, int64, string, ExecutionFence) (*Outbox, error)
}
