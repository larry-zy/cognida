// Package governedops defines explicitly confirmed writes to owned derived data.
package governedops

import (
	l "cognida/internal/model/lifecycle"
	"context"
	"errors"
	"time"
)

var (
	ErrInvalid   = errors.New("invalid governed operation")
	ErrForbidden = errors.New("governed operation access denied")
	ErrConflict  = errors.New("governed operation version or confirmation conflict")
	ErrNotFound  = errors.New("governed operation not found")
	ErrUncertain = errors.New("operation persisted; outcome requires reconciliation")
)

const JobType = "operation.execute"

// Cell uses explicit logical types. Text is never interpreted as a number.
// Values are stored losslessly in SQL LONGTEXT; this is not a native SQL numeric schema.
type Cell struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}
type Table struct {
	Columns []string          `json:"columns"`
	Types   map[string]string `json:"types"`
	Rows    [][]Cell          `json:"rows"`
}
type Request struct {
	Action          string          `json:"action"`
	Target          string          `json:"target"`
	ExpectedVersion int64           `json:"expected_version"`
	InputRef        *l.RefID        `json:"input_ref,omitempty"`
	Values          map[string]Cell `json:"values,omitempty"`
	Where           map[string]Cell `json:"where,omitempty"`
	RollbackVersion int64           `json:"rollback_version,omitempty"`
	SourceQueryHash string          `json:"source_query_hash,omitempty"`
}
type Confirmation struct {
	ExpectedVersion int64  `json:"expected_version"`
	PlanHash        string `json:"plan_hash"`
	Action          string `json:"action"`
}
type Target struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Version         int64  `json:"version"`
	ActiveVersionID string `json:"active_version_id,omitempty"`
}
type Version struct {
	ID            string    `json:"id"`
	TargetID      string    `json:"target_id"`
	Version       int64     `json:"version"`
	OperationID   string    `json:"operation_id"`
	PhysicalTable string    `json:"-"`
	Table         Table     `json:"table"`
	Digest        string    `json:"digest"`
	SourceRefs    []l.RefID `json:"source_refs"`
	CreatedAt     time.Time `json:"created_at"`
}
type Plan struct {
	Request           Request           `json:"request"`
	Hash              string            `json:"hash"`
	SQL               string            `json:"sql"`
	Parameters        []Cell            `json:"parameters"`
	BeforeVersionID   string            `json:"before_version_id,omitempty"`
	BeforeDigest      string            `json:"before_digest,omitempty"`
	After             Table             `json:"after"`
	AfterDigest       string            `json:"after_digest"`
	EstimatedAffected int64             `json:"estimated_affected"`
	SourceRefs        []l.RefID         `json:"source_refs"`
	SourceDigests     map[string]string `json:"source_digests"`
	RollbackVersionID string            `json:"rollback_version_id,omitempty"`
}
type Job struct {
	ID                 string     `json:"id"`
	TenantID           int64      `json:"-"`
	UserID             int64      `json:"-"`
	TargetID           string     `json:"target_id"`
	Status             string     `json:"status"`
	Version            int64      `json:"version"`
	Plan               Plan       `json:"plan"`
	Error              string     `json:"error,omitempty"`
	ConfirmedBy        int64      `json:"confirmed_by,omitempty"`
	ConfirmedAt        *time.Time `json:"confirmed_at,omitempty"`
	ExpiresAt          time.Time  `json:"expires_at"`
	Deadline           time.Time  `json:"deadline"`
	Epoch              int64      `json:"-"`
	LeaseUntil         time.Time  `json:"-"`
	CommittedVersionID string     `json:"committed_version_id,omitempty"`
	CommittedVersion   int64      `json:"committed_version,omitempty"`
	AffectedRows       int64      `json:"affected_rows"`
	OutputRef          *l.RefID   `json:"output_ref,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (j *Job) Terminal() bool {
	return j.Status == "completed" || j.Status == "cancelled" || j.Status == "failed" || j.Status == "timed_out"
}

type Repository interface {
	Prepare(context.Context, l.Principal, Plan, string) (*Job, error)
	Get(context.Context, l.Principal, string) (*Job, error)
	FindByKey(context.Context, l.Principal, string) (*Job, error)
	List(context.Context, l.Principal, string) ([]Job, string, error)
	Targets(context.Context, l.Principal) ([]Target, error)
	Target(context.Context, l.Principal, string) (*Target, error)
	ReadVersion(context.Context, l.Principal, string, int64) (*Version, error)
	Confirm(context.Context, l.Principal, string, Confirmation) (*Job, error)
	Cancel(context.Context, l.Principal, string) (*Job, error)
	Pending(context.Context, int) ([]Job, error)
	Claim(context.Context, l.Principal, string) (*Job, error)
	Commit(context.Context, l.Principal, string, int64) (*Job, error)
	Fail(context.Context, l.Principal, string, int64, string) (*Job, error)
	AttachOutput(context.Context, l.Principal, string, l.RefID) error
}
