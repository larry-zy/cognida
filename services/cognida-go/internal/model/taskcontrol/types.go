// Package taskcontrol defines durable cross-domain coordination records.
// Domain jobs remain authoritative; a bound child is not a completed job.
package taskcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrInvalid   = errors.New("invalid task control request")
	ErrForbidden = errors.New("task owner identity required")
	ErrNotFound  = errors.New("task control record not found")
	ErrConflict  = errors.New("task control version, state or idempotency conflict")
)

type Scope struct {
	TenantID int64 `json:"tenant_id"`
	UserID   int64 `json:"user_id"`
}

func (s Scope) Validate() error {
	if s.TenantID <= 0 || s.UserID <= 0 {
		return ErrForbidden
	}
	return nil
}

type Status string

const (
	StatusDraft              Status = "draft"
	StatusReady              Status = "ready"
	StatusRunning            Status = "running"
	StatusAwaitingApproval   Status = "awaiting_approval"
	StatusPartiallyCompleted Status = "partially_completed"
	StatusCompleted          Status = "completed"
	StatusFailed             Status = "failed"
	StatusTimedOut           Status = "timed_out"
	StatusCancelRequested    Status = "cancel_requested"
	StatusReconcileRequired  Status = "reconcile_required"
	StatusCancelled          Status = "cancelled"
)

type Task struct {
	Scope
	ID                 string          `json:"id"`
	Goal               string          `json:"goal"`
	InputRefs          json.RawMessage `json:"input_refs"`
	Budget             json.RawMessage `json:"budget"`
	ResultRefs         json.RawMessage `json:"result_refs"`
	RequestID          string          `json:"request_id"`
	Status             Status          `json:"status"`
	Version            int64           `json:"version"`
	CurrentPlanVersion int64           `json:"current_plan_version"`
	Reason             string          `json:"reason,omitempty"`
	CancelRequestedAt  *time.Time      `json:"cancel_requested_at,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type CreateTaskInput struct {
	Goal           string          `json:"goal"`
	InputRefs      json.RawMessage `json:"input_refs"`
	Budget         json.RawMessage `json:"budget"`
	RequestID      string          `json:"-"`
	IdempotencyKey string          `json:"-"`
}

type Plan struct {
	TaskID      string          `json:"task_id"`
	Version     int64           `json:"version"`
	Snapshot    json.RawMessage `json:"snapshot"`
	ContentHash string          `json:"content_hash"`
	CreatedAt   time.Time       `json:"created_at"`
}

type ChildStatus string

const (
	ChildPending           ChildStatus = "pending"
	ChildDispatching       ChildStatus = "dispatching"
	ChildReconcileRequired ChildStatus = "reconcile_required"
	ChildBound             ChildStatus = "bound"
	ChildCancelled         ChildStatus = "cancelled"
)

type Child struct {
	Scope
	ID           string          `json:"id"`
	TaskID       string          `json:"task_id"`
	PlanVersion  int64           `json:"plan_version"`
	NodeID       string          `json:"node_id"`
	Attempt      int             `json:"attempt"`
	Domain       string          `json:"domain"`
	OperationKey string          `json:"operation_key"`
	DomainID     string          `json:"domain_id,omitempty"`
	Status       ChildStatus     `json:"status"`
	Version      int64           `json:"version"`
	Receipt      json.RawMessage `json:"receipt,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// DispatchInput is accepted only after service-level plan and authorization
// checks. Enqueue creates a child handle and an outbox entry in one transaction.
type DispatchInput struct {
	PlanVersion int64
	NodeID      string
	Attempt     int
	Domain      string
	Payload     json.RawMessage
}

type Binding struct {
	DomainID string
	Receipt  json.RawMessage
}

type OutboxStatus string

const (
	OutboxPending           OutboxStatus = "pending"
	OutboxDispatching       OutboxStatus = "dispatching"
	OutboxReconcileRequired OutboxStatus = "reconcile_required"
	OutboxDelivered         OutboxStatus = "delivered"
	OutboxCancelled         OutboxStatus = "cancelled"
)

type Outbox struct {
	Scope
	ID           string          `json:"id"`
	TaskID       string          `json:"task_id"`
	ChildID      string          `json:"child_id"`
	OperationKey string          `json:"operation_key"`
	Payload      json.RawMessage `json:"payload"`
	PayloadHash  string          `json:"payload_hash"`
	Status       OutboxStatus    `json:"status"`
	Version      int64           `json:"version"`
	Reason       string          `json:"reason,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type ListFilter struct {
	Cursor string
	Limit  int
}
type TaskPage struct {
	Items      []Task `json:"items"`
	NextCursor string `json:"next_cursor"`
}
type OutboxFilter struct {
	ListFilter
	Status OutboxStatus
}
type OutboxPage struct {
	Items      []Outbox `json:"items"`
	NextCursor string   `json:"next_cursor"`
}
type TaskUpdate struct {
	Status     Status
	ResultRefs json.RawMessage
	Reason     string
}

type Repository interface {
	CreateTask(context.Context, Scope, CreateTaskInput) (*Task, error)
	GetTask(context.Context, Scope, string) (*Task, error)
	ListTasks(context.Context, Scope, ListFilter) (*TaskPage, error)
	AppendPlan(context.Context, Scope, string, int64, json.RawMessage) (*Plan, error)
	GetPlan(context.Context, Scope, string, int64) (*Plan, error)
	// EnqueueChild never invokes the domain executor. The task must be ready/running.
	EnqueueChild(context.Context, Scope, string, int64, DispatchInput) (*Child, error)
	GetChild(context.Context, Scope, string) (*Child, error)
	ListChildren(context.Context, Scope, string) ([]Child, error)
	// BindChild preserves real dispatch receipts even after cancellation was requested.
	BindChild(context.Context, Scope, string, int64, int, Binding) (*Child, error)
	RequestCancel(context.Context, Scope, string, int64) (*Task, error)
	UpdateTaskStatus(context.Context, Scope, string, int64, TaskUpdate) (*Task, error)
	ListOutbox(context.Context, Scope, OutboxFilter) (*OutboxPage, error)
	// Claim is one-way. A crash after claim requires reconciliation, never automatic replay.
	ClaimOutbox(context.Context, Scope, string, int64) (*Outbox, error)
	MarkDispatchUnknown(context.Context, Scope, string, int64, string) (*Outbox, error)
}
