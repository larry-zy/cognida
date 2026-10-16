// Package strategy records governed policy versions separately from runtime skill catalogs.
package strategy

import (
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid strategy input")
	ErrForbidden   = errors.New("strategy or evidence is not authorized")
	ErrNotFound    = errors.New("strategy record not found")
	ErrConflict    = errors.New("strategy version or operation conflict")
	ErrGate        = errors.New("strategy regression gates did not pass")
	ErrUnavailable = errors.New("strategy consumer is not registered")
)

type Gates struct {
	MinimumQuality     float64 `json:"minimum_quality"`
	MaximumQualityDrop float64 `json:"maximum_quality_drop"`
	MaximumErrorRate   float64 `json:"maximum_error_rate"`
	MaximumCostRatio   float64 `json:"maximum_cost_ratio"`
}
type Version struct {
	ID            string            `json:"id"`
	TenantID      int64             `json:"tenant_id"`
	UserID        int64             `json:"user_id"`
	Slot          string            `json:"slot"`
	Number        int               `json:"version"`
	Kind          string            `json:"kind"`
	Body          json.RawMessage   `json:"body"`
	Digest        string            `json:"digest"`
	BaselineID    string            `json:"baseline_id,omitempty"`
	EvidenceRefs  []string          `json:"evidence_refs"`
	SourceDigests map[string]string `json:"source_digests"`
	Gates         Gates             `json:"gates"`
	CreatedAt     time.Time         `json:"created_at"`
	DisabledAt    *time.Time        `json:"disabled_at,omitempty"`
}
type Head struct {
	TenantID    int64  `json:"tenant_id"`
	UserID      int64  `json:"user_id"`
	Slot        string `json:"slot"`
	Kind        string `json:"kind"`
	Revision    int64  `json:"revision"`
	NextVersion int    `json:"next_version"`
	BaselineID  string `json:"baseline_id"`
	ActiveID    string `json:"active_id"`
}
type Event struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Key       string          `json:"-"`
	Hash      string          `json:"-"`
	VersionID string          `json:"version_id"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}
type Snapshot struct {
	EffectiveBody json.RawMessage `json:"effective_body"`
	VersionID     string          `json:"version_id"`
	Slot          string          `json:"slot"`
	Kind          string          `json:"kind"`
	Digest        string          `json:"digest"`
	Body          json.RawMessage `json:"body"`
	CacheIdentity string          `json:"cache_identity"`
	PinID         string          `json:"pin_id"`
}
type Metrics struct {
	Quality   float64 `json:"quality"`
	ErrorRate float64 `json:"error_rate"`
	Cost      float64 `json:"cost"`
	Samples   int     `json:"samples"`
}
type RegressionEvidence struct {
	BaselineTaskID      string  `json:"baseline_task_id"`
	CandidateTaskID     string  `json:"candidate_task_id"`
	BaselineVersionID   string  `json:"baseline_version_id"`
	CandidateVersionID  string  `json:"candidate_version_id"`
	DatasetVersionID    string  `json:"dataset_version_id"`
	DatasetManifest     string  `json:"dataset_manifest"`
	ConfigurationDigest string  `json:"configuration_digest"`
	GraderVersion       string  `json:"grader_version"`
	QualityMetric       string  `json:"quality_metric"`
	CostMetric          string  `json:"cost_metric"`
	Baseline            Metrics `json:"baseline"`
	Candidate           Metrics `json:"candidate"`
	// Per-sample evidence must be read from the two genuine persisted evaluations.
	Samples json.RawMessage `json:"samples"`
}
type Regression struct {
	Evidence RegressionEvidence `json:"evidence"`
	Gates    Gates              `json:"gates"`
	Passed   bool               `json:"passed"`
	Reasons  []string           `json:"reasons"`
}
type Approval struct {
	Digest       string `json:"digest"`
	RegressionID string `json:"regression_id"`
	ReviewerID   int64  `json:"reviewer_id"`
	Approved     bool   `json:"approved"`
	Comment      string `json:"comment"`
}
type Activation struct {
	VersionID         string `json:"version_id"`
	Digest            string `json:"digest"`
	PreviousVersionID string `json:"previous_version_id"`
	Revision          int64  `json:"revision"`
	ApprovalID        string `json:"approval_id,omitempty"`
	Reason            string `json:"reason"`
	Consumer          string `json:"consumer"`
}
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
}

// Tx is scoped by a locked head. Implementations must not expose unscoped mutations.
type Tx interface {
	Head() *Head
	Version(context.Context, string) (*Version, error)
	AppendVersion(context.Context, *Version) error
	SetHead(context.Context) error
	DisableVersion(context.Context, string) error
	Event(context.Context, string, string) (*Event, error)
	EventByID(context.Context, string) (*Event, error)
	AppendEvent(context.Context, *Event) error
}
type Repository interface {
	FindEvent(context.Context, l.Principal, string, string, string) (*Event, error)
	ReadEvent(context.Context, l.Principal, string, string) (*Event, error)
	InSlot(context.Context, l.Principal, string, string, func(Tx) error) error
	Head(context.Context, l.Principal, string) (*Head, error)
	Version(context.Context, l.Principal, string) (*Version, error)
	Versions(context.Context, l.Principal, string, string, int) (*Page[Version], error)
	Events(context.Context, l.Principal, string, string, string, int) (*Page[Event], error)
}
