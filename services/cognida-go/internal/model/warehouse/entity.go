// Package warehouse defines bounded, versioned transformations of authorized snapshots.
package warehouse

import (
	g "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"errors"
	"time"
)

var ErrInvalid = errors.New("invalid warehouse definition")
var ErrConflict = errors.New("warehouse version conflict")
var ErrForbidden = errors.New("warehouse access denied")
var ErrNotFound = errors.New("warehouse record not found")

type Diagnostic struct {
	Code    string `json:"code"`
	Node    string `json:"node,omitempty"`
	Message string `json:"message"`
}

func (e *Diagnostic) Error() string { return e.Code + ": " + e.Message }
func (e *Diagnostic) Unwrap() error { return ErrInvalid }

type Projection struct {
	Name       string              `json:"name"`
	Type       string              `json:"type"`
	Expression g.FeatureExpression `json:"expression"`
}
type Metric struct {
	Name      string `json:"name"`
	Field     string `json:"field,omitempty"`
	Aggregate string `json:"aggregate"`
	Caliber   string `json:"caliber"`
}
type Join struct {
	LeftKeys     []string `json:"left_keys"`
	RightKeys    []string `json:"right_keys"`
	RightColumns []string `json:"right_columns"`
	Prefix       string   `json:"prefix"`
}
type Node struct {
	ID        string       `json:"id"`
	Layer     string       `json:"layer"`
	Kind      string       `json:"kind"`
	Inputs    []string     `json:"inputs,omitempty"`
	SourceRef string       `json:"source_ref,omitempty"`
	Columns   []Projection `json:"columns,omitempty"`
	Join      *Join        `json:"join,omitempty"`
	GroupBy   []string     `json:"group_by,omitempty"`
	Metrics   []Metric     `json:"metrics,omitempty"`
}
type Definition struct {
	Name            string   `json:"name"`
	Subject         string   `json:"subject"`
	Grain           string   `json:"grain"`
	GrainKeys       []string `json:"grain_keys"`
	SemanticModelID string   `json:"semantic_model_id,omitempty"`
	SemanticVersion int      `json:"semantic_version,omitempty"`
	FactNode        string   `json:"fact_node"`
	FactKeys        []string `json:"fact_keys"`
	EventField      string   `json:"event_field,omitempty"`
	UpdatePolicy    string   `json:"update_policy"`
	DeletePolicy    string   `json:"delete_policy"`
	LatePolicy      string   `json:"late_policy"`
	Nodes           []Node   `json:"nodes"`
	OutputNode      string   `json:"output_node"`
}
type Model struct {
	ID          string     `json:"id"`
	TenantID    int64      `json:"-"`
	UserID      int64      `json:"-"`
	Version     int        `json:"version"`
	Definition  Definition `json:"definition"`
	Digest      string     `json:"digest"`
	ActiveRunID string     `json:"active_run_id,omitempty"`
	ActiveRef   string     `json:"active_ref,omitempty"`
	Watermark   *time.Time `json:"watermark,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}
type ColumnLineage struct {
	Dependency  string `json:"dependency,omitempty"` // row_set means count(*) reads row membership, not column values.
	Node        string `json:"node"`
	Column      string `json:"column"`
	InputNode   string `json:"input_node"`
	InputColumn string `json:"input_column"`
	SourceRef   string `json:"source_ref,omitempty"`
}
type Stage struct {
	Node       string `json:"node"`
	Layer      string `json:"layer"`
	Kind       string `json:"kind"`
	InputRows  int    `json:"input_rows"`
	OutputRows int    `json:"output_rows"`
	Digest     string `json:"digest"`
}
type Preview struct {
	DefinitionDigest string            `json:"definition_digest"`
	SourceDigests    map[string]string `json:"source_digests"`
	Stages           []Stage           `json:"stages"`
	Lineage          []ColumnLineage   `json:"lineage"`
	Output           l.Input           `json:"output"`
	ExactRows        [][]string        `json:"exact_rows"`
	TotalRows        int               `json:"total_rows"`
	Limitations      []string          `json:"limitations"`
	Impact           []Impact          `json:"impact"`
}
type Impact struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Unknown bool   `json:"unknown,omitempty"`
}
type RunRequest struct {
	ModelID       string     `json:"model_id"`
	ModelVersion  int        `json:"model_version"`
	Mode          string     `json:"mode"`
	FactSourceRef string     `json:"fact_source_ref,omitempty"`
	EventFrom     *time.Time `json:"event_from,omitempty"`
	EventTo       *time.Time `json:"event_to,omitempty"`
}
type Partition struct {
	Index       int       `json:"index"`
	Count       int       `json:"count"`
	Digest      string    `json:"digest"`
	OutputRef   string    `json:"output_ref"`
	CommittedAt time.Time `json:"committed_at"`
}
type Run struct {
	CommandDigest     string            `json:"command_digest,omitempty"`
	ID                string            `json:"id"`
	TenantID          int64             `json:"-"`
	UserID            int64             `json:"-"`
	Version           int               `json:"version"`
	RequestID         string            `json:"request_id,omitempty"`
	Request           RunRequest        `json:"request"`
	Status            string            `json:"status"`
	DefinitionDigest  string            `json:"definition_digest"`
	SourceDigests     map[string]string `json:"source_digests"`
	BaseRunID         string            `json:"base_run_id,omitempty"`
	FactRef           string            `json:"fact_ref,omitempty"`
	OutputRef         string            `json:"output_ref,omitempty"`
	PublishedRef      string            `json:"published_ref,omitempty"`
	Watermark         *time.Time        `json:"watermark,omitempty"`
	Partitions        []Partition       `json:"partitions"`
	Stages            []Stage           `json:"stages"`
	Lineage           []ColumnLineage   `json:"lineage"`
	LineageRegistered bool              `json:"lineage_registered"`
	Error             string            `json:"error,omitempty"`
	Lease             string            `json:"-"`
	LeaseUntil        *time.Time        `json:"-"`
	Deadline          *time.Time        `json:"deadline,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}
type Event struct {
	ID        string    `json:"id"`
	RunID     string    `json:"run_id"`
	Action    string    `json:"action"`
	ActorID   int64     `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}
type Repository interface {
	SaveModel(context.Context, *Model, int, string) (*Model, error)
	GetModel(context.Context, l.Principal, string, int) (*Model, error)
	ListModels(context.Context, l.Principal, string) ([]Model, string, error)
	CreateRun(context.Context, *Run, string) (*Run, error)
	GetRun(context.Context, l.Principal, string) (*Run, error)
	FindRunByKey(context.Context, l.Principal, string) (*Run, error)
	ListRuns(context.Context, l.Principal, string, string) ([]Run, string, error)
	Pending(context.Context) ([]Run, error)
	Claim(context.Context, l.Principal, string) (*Run, error)
	Update(context.Context, *Run, func(*Run) error) error
	Cancel(context.Context, l.Principal, string, int) (*Run, error)
	Approve(context.Context, l.Principal, string, int, string) (*Run, error)
	Events(context.Context, l.Principal, string) ([]Event, error)
	Impact(context.Context, l.Principal, []string) ([]Impact, error)
}

// Exact JSON text is persisted by repositories; MySQL binary JSON is not used for numeric payloads.
