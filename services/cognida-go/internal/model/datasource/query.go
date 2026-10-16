package datasource

import (
	"context"
	"time"
)

// QueryAdapter deliberately does not expose database/sql. Remote job engines
// implement the same bounded result contract and may additionally expose Jobs.
type QueryAdapter interface {
	Capabilities() QueryCapabilities
	Query(context.Context, QueryRequest) (*QueryResult, error)
}
type QueryProvider interface {
	AcquireQuery(context.Context, int64, string) (QueryAdapter, *DataSource, error)
}
type QueryCapabilities struct {
	Engine       Type   `json:"engine"`
	Client       string `json:"client"`
	Dialect      string `json:"dialect"`
	Parameters   bool   `json:"parameters"`
	Cancellation string `json:"cancellation"`
	Pagination   string `json:"pagination"`
	ScanBudget   bool   `json:"scan_budget"`
	Async        bool   `json:"async"`
}
type QueryRequest struct {
	SQL          string        `json:"sql"`
	Parameters   []any         `json:"parameters,omitempty"`
	MaxRows      int           `json:"max_rows"`
	MaxBytes     int           `json:"max_bytes"`
	Timeout      time.Duration `json:"-"`
	MaxScanBytes int64         `json:"max_scan_bytes,omitempty"`
	OperationID  string        `json:"operation_id,omitempty"`
}
type QueryResult struct {
	Columns       []string          `json:"columns"`
	ColumnTypes   map[string]string `json:"column_types"`
	Rows          []map[string]any  `json:"rows"`
	Truncated     bool              `json:"truncated"`
	ExecutedSQL   string            `json:"executed_sql"`
	SourceVersion string            `json:"source_version"`
	RemoteID      string            `json:"remote_id,omitempty"`
}

// AsyncQueryAdapter supports deterministic submission and restart recovery;
// callers must persist the operation ID before Submit, then reconcile Status.
type AsyncQueryAdapter interface {
	QueryAdapter
	Submit(context.Context, QueryRequest) (string, error)
	Status(context.Context, string) (string, error)
	Cancel(context.Context, string) error
	Page(context.Context, string, string, int) (*QueryResult, string, error)
}

// SubmissionLookup performs no write. Durable callers use it when cancellation
// races a response-less deterministic submission; absence is not permission to
// submit a new query after cancellation.
type SubmissionLookup interface {
	LookupSubmission(context.Context, QueryRequest) (id string, state string, found bool, err error)
}

// PreparedAsyncQueryAdapter exposes the pre-submission resource needed for
// protocols without a caller-chosen job ID. Persist Prepare's opaque handle,
// then fence dispatch before calling SubmitPrepared. An ambiguous submission
// must be abandoned, never automatically replayed.
type PreparedAsyncQueryAdapter interface {
	AsyncQueryAdapter
	Prepare(context.Context, QueryRequest) (string, error)
	SubmitPrepared(context.Context, string, QueryRequest) (string, error)
	AbandonPrepared(context.Context, string) error
}
