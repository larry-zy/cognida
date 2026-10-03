// Package governance defines subscriptions, labeling, features and reviewed learning.
package governance

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var ErrNotFound = errors.New("resource not found")
var ErrInvalid = errors.New("invalid input")
var ErrConflict = errors.New("resource version conflict")
var ErrForbidden = errors.New("permission denied")

type Meta struct {
	ID        string    `json:"id"`
	TenantID  int64     `json:"tenant_id"`
	UserID    int64     `json:"user_id"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Schedule struct {
	Cron     string `json:"cron"`
	Timezone string `json:"timezone"`
}
type RefreshSource struct {
	Mode       string `json:"mode"`
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
}
type AnalysisInput struct {
	Refresh     *RefreshSource `json:"refresh,omitempty"`
	AssetRefID  string         `json:"asset_ref_id"`
	Prompt      string         `json:"prompt"`
	Metric      string         `json:"metric"`
	TimeColumn  string         `json:"time_column"`
	ValueColumn string         `json:"value_column"`
}
type Watch struct {
	Method                string   `json:"method"`
	Threshold             float64  `json:"threshold"`
	AttributionDimensions []string `json:"attribution_dimensions"`
}
type Subscription struct {
	Meta
	Name          string        `json:"name"`
	Kind          string        `json:"kind"`
	Enabled       bool          `json:"enabled"`
	Schedule      Schedule      `json:"schedule"`
	Input         AnalysisInput `json:"input"`
	Watch         *Watch        `json:"watch"`
	Channel       string        `json:"channel"`
	NextRunAt     *time.Time    `json:"next_run_at"`
	ScheduleError string        `json:"schedule_error,omitempty"`
}
type SubscriptionRun struct {
	Meta
	SubscriptionID string    `json:"subscription_id"`
	Status         string    `json:"status"`
	ScheduledAt    time.Time `json:"scheduled_at"`
	ResultRefID    string    `json:"result_ref_id"`
	AlertTriggered bool      `json:"alert_triggered"`
	Error          string    `json:"error,omitempty"`
}
type Notification struct {
	Meta
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	RunID       string     `json:"run_id"`
	ResultRefID string     `json:"result_ref_id"`
	ReadAt      *time.Time `json:"read_at"`
}
type Label struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type AnnotationTask struct {
	Meta
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	SourceRefID string  `json:"source_ref_id"`
	Labels      []Label `json:"labels"`
	Status      string  `json:"status"`
	ItemCount   int     `json:"item_count"`
}
type Entity struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}
type Relation struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
}
type Annotation struct {
	Labels    []string   `json:"labels"`
	Entities  []Entity   `json:"entities"`
	Relations []Relation `json:"relations"`
}
type Provenance struct {
	Source     string   `json:"source"`
	Method     string   `json:"method"`
	Model      string   `json:"model,omitempty"`
	Confidence *float64 `json:"confidence"`
	Evidence   []string `json:"evidence"`
}
type AnnotationItem struct {
	Meta
	TaskID        string     `json:"task_id"`
	Content       string     `json:"content"`
	Annotation    Annotation `json:"annotation"`
	Provenance    Provenance `json:"provenance"`
	ReviewStatus  string     `json:"review_status"`
	ReviewComment string     `json:"review_comment"`
	ReviewedBy    int64      `json:"reviewed_by"`
}
type TaggingConfig struct {
	EnableTags           bool     `json:"enable_tags"`
	EnableSentiment      bool     `json:"enable_sentiment"`
	EnableExtraction     bool     `json:"enable_extraction"`
	EnableClassification bool     `json:"enable_classification"`
	TagSchema            []string `json:"tag_schema"`
}
type TaggingTask struct {
	Meta
	Name        string        `json:"name"`
	SourceRefID string        `json:"source_ref_id"`
	Config      TaggingConfig `json:"config"`
	Status      string        `json:"status"`
	Error       string        `json:"error,omitempty"`
}
type TaggingResult struct {
	Meta
	TaskID             string             `json:"task_id"`
	Content            string             `json:"content"`
	Tags               []string           `json:"tags"`
	TagConfidence      map[string]float64 `json:"tag_confidence"`
	Sentiment          string             `json:"sentiment"`
	Entities           json.RawMessage    `json:"entities"`
	Keywords           []string           `json:"keywords"`
	Summary            string             `json:"summary"`
	Category           string             `json:"category"`
	CategoryConfidence *float64           `json:"category_confidence"`
	QualityScore       *float64           `json:"quality_score"`
	Provenance         Provenance         `json:"provenance"`
	ReviewStatus       string             `json:"review_status"`
	ReviewComment      string             `json:"review_comment"`
	ReviewedBy         int64              `json:"reviewed_by"`
}
type Feature struct {
	Meta
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Description string          `json:"description"`
	Definition  json.RawMessage `json:"definition"`
	SourceRefID string          `json:"source_ref_id"`
}
type FeatureValue struct {
	FeatureID   string          `json:"feature_id"`
	Version     int             `json:"version"`
	EntityID    string          `json:"entity_id"`
	Value       json.RawMessage `json:"value"`
	Timestamp   time.Time       `json:"timestamp"`
	SourceRefID string          `json:"source_ref_id"`
}
type SuggestionSource struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}
type Suggestion struct {
	Meta
	Source         SuggestionSource `json:"source"`
	Kind           string           `json:"kind"`
	Title          string           `json:"title"`
	Content        string           `json:"content"`
	EvidenceRefIDs []string         `json:"evidence_ref_ids"`
	Proposal       json.RawMessage  `json:"proposal"`
	Status         string           `json:"status"`
	TargetRefID    string           `json:"target_ref_id"`
	Error          string           `json:"error,omitempty"`
	ReviewedBy     int64            `json:"reviewed_by"`
	ReviewComment  string           `json:"review_comment"`
}

// Record is an indexed aggregate envelope. Payloads are decoded to the typed
// structures above at service boundaries; callers never choose arbitrary kinds.
type Record struct {
	Meta
	Kind           string
	ParentID       string
	Status         string
	IdempotencyKey string
	Payload        json.RawMessage
	DueAt          *time.Time
}
type Filter struct {
	Kind, ParentID, Status, Cursor string
	UserID                         int64
	Limit                          int
}
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
}
type Repository interface {
	Transaction(context.Context, func(Repository) error) error
	PublishedRefs(context.Context, int64, string, int) ([]string, error)
	Create(context.Context, *Record) (*Record, error)
	Get(context.Context, int64, string, string) (*Record, error)
	List(context.Context, int64, Filter) (*Page[Record], error)
	Update(context.Context, *Record, int) error
	Due(context.Context, time.Time, int) ([]Record, error)
	Pending(context.Context, int) ([]Record, error)
	FeatureWrite(context.Context, int64, []FeatureValue) error
	FeatureQuery(context.Context, int64, []string, []FeatureValue, time.Time) ([]FeatureValue, error)
}
