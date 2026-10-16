// Package collection defines the bounded context for governed web collection.
package collection

import (
	l "cognida/internal/model/lifecycle"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type TaskStatus string

const (
	TaskStatusDraft     TaskStatus = "draft"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

func (s TaskStatus) IsTerminal() bool {
	return s == TaskStatusCompleted || s == TaskStatusFailed || s == TaskStatusCancelled
}

type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
)

func (s RunStatus) IsTerminal() bool {
	return s == RunStatusCompleted || s == RunStatusFailed || s == RunStatusCancelled
}

type ItemStatus string

const (
	ItemStatusPending   ItemStatus = "pending"
	ItemStatusCandidate ItemStatus = "candidate"
	ItemStatusAccepted  ItemStatus = "accepted"
	ItemStatusSkipped   ItemStatus = "skipped"
	ItemStatusRejected  ItemStatus = "rejected"
	ItemStatusFailed    ItemStatus = "failed"
)

// Limits bounds a task's outbound work. Zero-valued limits are rejected so a
// caller cannot accidentally create an unbounded collection task.
type Limits struct {
	MaxSources       int   `json:"max_sources"`
	MaxResponseBytes int64 `json:"max_response_bytes"`
}

// Schedule controls recurring, incremental collection. It intentionally uses
// an interval instead of a cron expression so the scheduler has one portable,
// bounded representation across local and distributed runners.
type Schedule struct {
	Enabled         bool       `json:"enabled"`
	IntervalSeconds int64      `json:"interval_seconds,omitempty"`
	NextRunAt       *time.Time `json:"next_run_at,omitempty"`
	LastScheduledAt *time.Time `json:"last_scheduled_at,omitempty"`
}

func (s Schedule) Validate() error {
	if !s.Enabled {
		return nil
	}
	if s.IntervalSeconds <= 0 || s.IntervalSeconds > 31536000 || s.NextRunAt == nil {
		return fmt.Errorf("%w: enabled schedule requires interval_seconds in 1..31536000 and next_run_at", ErrInvalidTask)
	}
	return nil
}

func (s Schedule) Next(now time.Time) time.Time {
	return now.UTC().Add(time.Duration(s.IntervalSeconds) * time.Second)
}

func (l Limits) Validate() error {
	if l.MaxSources <= 0 {
		return fmt.Errorf("%w: max_sources must be positive", ErrInvalidTask)
	}
	if l.MaxResponseBytes <= 0 {
		return fmt.Errorf("%w: max_response_bytes must be positive", ErrInvalidTask)
	}
	return nil
}

// Task is a tenant-scoped request to collect explicit HTTPS sources into one KB.
type Task struct {
	Mode            string     `json:"mode"`
	ID              string     `json:"id"`
	TenantID        int64      `json:"tenant_id"`
	UserID          int64      `json:"user_id"`
	KnowledgeBaseID string     `json:"knowledge_base_id"`
	IdempotencyKey  string     `json:"idempotency_key"`
	Status          TaskStatus `json:"status"`
	Limits          Limits     `json:"limits"`
	Schedule        Schedule   `json:"schedule"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (t *Task) Validate() error {
	if strings.TrimSpace(t.ID) == "" || t.TenantID <= 0 || t.UserID <= 0 || (t.Mode != SnapshotMode && strings.TrimSpace(t.KnowledgeBaseID) == "") || strings.TrimSpace(t.IdempotencyKey) == "" {
		return ErrInvalidTask
	}
	if err := t.Limits.Validate(); err != nil {
		return err
	}
	if err := t.Schedule.Validate(); err != nil {
		return err
	}
	if t.Status == "" {
		t.Status = TaskStatusDraft
	}
	if t.Status != TaskStatusDraft && t.Status != TaskStatusRunning && !t.Status.IsTerminal() {
		return fmt.Errorf("%w: unknown status %q", ErrInvalidTask, t.Status)
	}
	return nil
}

func (t *Task) Start() error {
	if t.Status != TaskStatusDraft && t.Status != TaskStatusCompleted && t.Status != TaskStatusFailed {
		return ErrInvalidTransition
	}
	t.Status = TaskStatusRunning
	t.ErrorMessage = ""
	t.UpdatedAt = time.Now()
	return nil
}

func (t *Task) Cancel() error {
	if t.Status.IsTerminal() {
		return ErrInvalidTransition
	}
	t.Status = TaskStatusCancelled
	t.UpdatedAt = time.Now()
	return nil
}

func (t *Task) Complete() error {
	if t.Status != TaskStatusRunning {
		return ErrInvalidTransition
	}
	t.Status = TaskStatusCompleted
	t.UpdatedAt = time.Now()
	return nil
}

func (t *Task) Fail(message string) error {
	if t.Status != TaskStatusRunning {
		return ErrInvalidTransition
	}
	t.Status = TaskStatusFailed
	t.ErrorMessage = message
	t.UpdatedAt = time.Now()
	return nil
}

// Source is an explicit seed URL belonging to a task.
type Source struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Source) Validate() error {
	if strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.TaskID) == "" {
		return ErrInvalidSource
	}
	u, err := url.Parse(s.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return fmt.Errorf("%w: source URL must be an absolute HTTPS URL", ErrInvalidSource)
	}
	return nil
}

// Run captures one invocation of a task.
type Run struct {
	CandidateOnly bool                `json:"candidate_only"`
	Governed      *GovernedInvocation `json:"governed_execution,omitempty"`
	OutputRefs    []l.RefID           `json:"output_refs,omitempty"`
	ID            string              `json:"id"`
	TaskID        string              `json:"task_id"`
	TenantID      int64               `json:"tenant_id"`
	Status        RunStatus           `json:"status"`
	ErrorMessage  string              `json:"error_message,omitempty"`
	StartedAt     time.Time           `json:"started_at"`
	CompletedAt   *time.Time          `json:"completed_at,omitempty"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

// Item is the durable outcome for one source URL in a run.
type Item struct {
	AssetRefID          string     `json:"asset_ref_id,omitempty"`
	ID                  string     `json:"id"`
	TenantID            int64      `json:"tenant_id"`
	TaskID              string     `json:"task_id"`
	RunID               string     `json:"run_id"`
	SourceID            string     `json:"source_id"`
	SourceURL           string     `json:"source_url"`
	CanonicalURL        string     `json:"canonical_url,omitempty"`
	Status              ItemStatus `json:"status"`
	ContentHash         string     `json:"content_hash,omitempty"`
	RawObjectKey        string     `json:"raw_object_key,omitempty"`
	NormalizedObjectKey string     `json:"normalized_object_key,omitempty"`
	RawHash             string     `json:"raw_hash,omitempty"`
	RawSize             int64      `json:"raw_size,omitempty"`
	NormalizedSize      int64      `json:"normalized_size,omitempty"`
	KnowledgeID         string     `json:"knowledge_id,omitempty"`
	HTTPStatus          int        `json:"http_status,omitempty"`
	ContentType         string     `json:"content_type,omitempty"`
	ErrorReason         string     `json:"error_reason,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}
