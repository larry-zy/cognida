package sandbox

import (
	"cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"time"
)

// Record persists the execution binding before any container side effect.
type Record struct {
	ID              string          `json:"id" gorm:"primaryKey"`
	TenantID        int64           `json:"tenant_id"`
	UserID          int64           `json:"user_id"`
	IdempotencyKey  string          `json:"-"`
	RequestSHA256   string          `json:"request_sha256"`
	Submission      json.RawMessage `json:"-" gorm:"type:json"`
	State           State           `json:"state"`
	Phase           string          `json:"phase"`
	ExecID          string          `json:"-"`
	FinishState     State           `json:"-"`
	FinishError     string          `json:"-"`
	RequestID       string          `json:"request_id"`
	Version         int64           `json:"version"`
	LeaseToken      string          `json:"-"`
	LeaseUntil      *time.Time      `json:"-"`
	CancelRequested bool            `json:"cancel_requested"`
	Deadline        time.Time       `json:"deadline"`
	Manifest        json.RawMessage `json:"manifest,omitempty" gorm:"type:json"`
	OutputRefs      json.RawMessage `json:"output_refs,omitempty" gorm:"type:json"`
	ErrorCode       string          `json:"error_code,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

func (Record) TableName() string { return "sandbox_runs" }
func (r Record) Principal() lifecycle.Principal {
	return lifecycle.Principal{TenantID: r.TenantID, UserID: r.UserID}
}
func (r Record) Binding() Binding {
	return Binding{ExecutionRef: ExecutionRef{ID: r.ID, TenantID: r.TenantID, UserID: r.UserID, RequestSHA256: r.RequestSHA256}, IdempotencyKey: r.IdempotencyKey}
}
func Terminal(s State) bool {
	return s == StateSucceeded || s == StateFailed || s == StateCancelled || s == StateTimedOut || s == StateReconcileRequired
}

type Repository interface {
	Create(context.Context, *Record) (*Record, error)
	FindByKey(context.Context, lifecycle.Principal, string) (*Record, error)
	Get(context.Context, lifecycle.Principal, string) (*Record, error)
	List(context.Context, lifecycle.Principal, string, int) ([]Record, string, error)
	RequestCancel(context.Context, lifecycle.Principal, string) (*Record, error)
	Claim(context.Context, time.Duration) (*Record, error)
	Save(context.Context, *Record) error
}
