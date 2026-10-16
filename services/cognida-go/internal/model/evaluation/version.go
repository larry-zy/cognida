package evaluation

import (
	f "cognida/internal/model/feedback"
	g "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	policy "cognida/internal/model/strategy"
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrVersionInvalid   = errors.New("invalid dataset version")
	ErrVersionConflict  = errors.New("dataset version conflict")
	ErrVersionForbidden = errors.New("dataset source access revoked")
	ErrVersionNotFound  = errors.New("dataset version not found")
)

type ReviewedSample struct {
	ID             string       `json:"id"`
	TaskID         string       `json:"task_id"`
	AnnotationType string       `json:"annotation_type"`
	ReviewVersion  int          `json:"review_version"`
	ReviewedBy     int64        `json:"reviewed_by"`
	Content        string       `json:"content"`
	Annotation     g.Annotation `json:"annotation"`
	LabelSchema    []g.Label    `json:"label_schema"`
	SourceRef      string       `json:"source_ref"`
	SourceDigest   string       `json:"source_digest"`
	SampleHash     string       `json:"sample_hash"`
	Group          string       `json:"group"`
	Split          string       `json:"split"`
	SourceTime     time.Time    `json:"source_time"`
}
type DatasetSplit struct {
	Algorithm              string     `json:"algorithm"` // none / source_hash_v1 / source_time_v1
	Seed                   string     `json:"seed,omitempty"`
	TrainPercent           int        `json:"train_percent,omitempty"`
	ValidationPercent      int        `json:"validation_percent,omitempty"`
	TrainBefore            *time.Time `json:"train_before,omitempty"`
	ValidationBefore       *time.Time `json:"validation_before,omitempty"`
	NearDuplicateThreshold float64    `json:"near_duplicate_threshold"`
}
type DatasetVersion struct {
	ID             string           `json:"id"`
	DatasetID      string           `json:"dataset_id"`
	Version        int              `json:"version"`
	TenantID       int64            `json:"tenant_id"`
	UserID         int64            `json:"user_id"`
	Name           string           `json:"name"`
	Profile        string           `json:"profile"`
	ManifestDigest string           `json:"manifest_digest"`
	Samples        []ReviewedSample `json:"samples"`
	QAPairs        []QAPair         `json:"qa_pairs,omitempty"`
	SourceRefs     []string         `json:"source_refs"`
	Split          DatasetSplit     `json:"split"`
	CreatedAt      time.Time        `json:"created_at"`
	LegacySource   string           `json:"legacy_source,omitempty"`
}
type SampleSelection struct {
	TaskID          string `json:"task_id"`
	ItemID          string `json:"item_id"`
	ExpectedVersion int    `json:"expected_version"`
}
type PublishDatasetVersion struct {
	DatasetID string            `json:"dataset_id"`
	Name      string            `json:"name"`
	Profile   string            `json:"profile"`
	Samples   []SampleSelection `json:"samples"`
	Split     DatasetSplit      `json:"split"`
}
type VersionPage struct {
	Items      []DatasetVersion `json:"items"`
	NextCursor string           `json:"next_cursor"`
	HasMore    bool             `json:"has_more"`
}

// Put atomically checks every selected reviewed row and serializes a dataset's
// version counter. No update/delete operation exists for a published version.
type DatasetVersionRepository interface {
	Put(context.Context, l.Principal, string, *DatasetVersion, map[string]int) (*DatasetVersion, error)
	Get(context.Context, l.Principal, string) (*DatasetVersion, error)
	List(context.Context, l.Principal, string, string, int) (*VersionPage, error)
}
type EvaluationSnapshot struct {
	FeedbackView     *f.View          `json:"feedback_view,omitempty"`
	Strategy         *policy.Snapshot `json:"strategy,omitempty"`
	DatasetVersionID string           `json:"dataset_version_id"`
	DatasetManifest  string           `json:"dataset_manifest"`
	Profile          string           `json:"profile"`
	QAPairs          []QAPair         `json:"qa_pairs"`
	Configuration    json.RawMessage  `json:"configuration"`
	Target           json.RawMessage  `json:"target"`
	GraderVersion    string           `json:"grader_version"`
	CreatedAt        time.Time        `json:"created_at"`
}
