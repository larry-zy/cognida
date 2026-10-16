package collection

import (
	"context"
	"time"
)

// Repository persists collection state. Every task-facing read and write is
// explicitly tenant-scoped; workers carry the run's tenant ID on item writes.
type Repository interface {
	CreateTask(ctx context.Context, task *Task, sources []*Source) error
	FindTask(ctx context.Context, tenantID int64, taskID string) (*Task, error)
	ListTasks(ctx context.Context, tenantID int64, status TaskStatus, limit int) ([]*Task, error)
	FindTaskByIdempotencyKey(ctx context.Context, tenantID int64, key string) (*Task, error)
	ListSources(ctx context.Context, tenantID int64, taskID string) ([]*Source, error)
	UpdateTask(ctx context.Context, task *Task) error
	// ClaimDueTasks atomically creates each durable run and advances its schedule.
	// Queue recovery can recover the run even if dispatch never happens.
	ClaimDueTasks(ctx context.Context, now time.Time, limit int) ([]*Task, error)

	// CreateRun atomically starts the task and creates its sole active run.
	CreateRun(ctx context.Context, run *Run) error
	FindActiveRun(ctx context.Context, tenantID int64, taskID string) (*Run, error)
	FindLatestRun(ctx context.Context, tenantID int64, taskID string) (*Run, error)
	ListRunningRuns(ctx context.Context, limit int) ([]*Run, error)
	UpdateRun(ctx context.Context, run *Run) error
	// FinishRun atomically commits the run outcome and its task status.
	FinishRun(ctx context.Context, run *Run) error

	CreateItem(ctx context.Context, item *Item) error
	UpdateItem(ctx context.Context, item *Item) error
	ListItems(ctx context.Context, tenantID int64, runID string) ([]*Item, error)
	FindAcceptedContent(ctx context.Context, tenantID int64, taskID, contentHash string) (*Item, error)
}

type FetchRequest struct {
	URL              string
	MaxResponseBytes int64
}

type FetchResponse struct {
	CanonicalURL string
	StatusCode   int
	ContentType  string
	Body         []byte
}

// Fetcher is deliberately independent from agent tools and HTTP handlers.
type Fetcher interface {
	Fetch(ctx context.Context, request FetchRequest) (*FetchResponse, error)
}

type IngestRequest struct {
	TenantID        int64
	UserID          int64
	KnowledgeBaseID string
	SourceURL       string
	CanonicalURL    string
	Content         string
	ContentHash     string
}

type IngestResult struct {
	KnowledgeID string
	Duplicate   bool
}

// DocumentIngestor is the sole dependency on the knowledge bounded context.
type DocumentIngestor interface {
	Ingest(ctx context.Context, request IngestRequest) (*IngestResult, error)
}

// TargetValidator rechecks publication availability before deduplication; an
// unchanged source is usable only while its target is still active.
type TargetValidator interface {
	ValidateTarget(context.Context, int64, string) error
}

// Runner makes execution replaceable without leaking goroutine state into APIs.
type Runner interface {
	Start(ctx context.Context, tenantID int64, taskID string) error
	Cancel(ctx context.Context, tenantID int64, taskID string) error
}

// Clock is a small port used by services that need deterministic timestamps.
type Clock interface{ Now() time.Time }

// LegacyReceiptRepository only reconciles historical committed ingestion; it never writes knowledge.
type LegacyReceiptRepository interface {
	FindLegacyReceipt(context.Context, int64, string, string) (string, error)
}
