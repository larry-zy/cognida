package governance

import (
	"context"
	"time"
)

type FeatureJobRequest struct {
	FeatureID      string     `json:"feature_id"`
	FeatureVersion int        `json:"feature_version"`
	SourceRefID    string     `json:"source_ref_id"`
	Mode           string     `json:"mode"`
	EventFrom      *time.Time `json:"event_from,omitempty"`
	EventTo        *time.Time `json:"event_to,omitempty"`
	PartitionSize  int        `json:"partition_size"`
}
type FeaturePartition struct {
	Index       int        `json:"index"`
	Status      string     `json:"status"`
	Count       int        `json:"count"`
	Digest      string     `json:"digest"`
	OutputRefID string     `json:"output_ref_id"`
	AvailableAt *time.Time `json:"available_at,omitempty"`
}
type FeatureJob struct {
	RequestID        string             `json:"request_id,omitempty"`
	OperationID      string             `json:"operation_id,omitempty"`
	Deadline         *time.Time         `json:"deadline,omitempty"`
	ID               string             `json:"id"`
	TenantID         int64              `json:"-"`
	UserID           int64              `json:"-"`
	Version          int                `json:"version"`
	Status           string             `json:"status"`
	Request          FeatureJobRequest  `json:"request"`
	DefinitionDigest string             `json:"definition_digest"`
	SourceDigest     string             `json:"source_digest"`
	AnchorRefID      string             `json:"anchor_ref_id"`
	WatermarkBefore  *time.Time         `json:"watermark_before,omitempty"`
	WatermarkAfter   *time.Time         `json:"watermark_after,omitempty"`
	Embedding        bool               `json:"embedding"`
	Partitions       []FeaturePartition `json:"partitions"`
	TotalRows        int                `json:"total_rows"`
	CommittedRows    int                `json:"committed_rows"`
	Error            string             `json:"error,omitempty"`
	CreatedAt        time.Time          `json:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at"`
	LeaseToken       string             `json:"-"`
	LeaseUntil       *time.Time         `json:"-"`
}
type FeatureJobRepository interface {
	CreateFeatureJob(context.Context, *FeatureJob, string) (*FeatureJob, error)
	GetFeatureJob(context.Context, int64, int64, string) (*FeatureJob, error)
	FindFeatureJobByKey(context.Context, int64, int64, string) (*FeatureJob, error)
	ListFeatureJobs(context.Context, int64, int64, string, int) ([]FeatureJob, string, error)
	PendingFeatureJobs(context.Context, int) ([]FeatureJob, error)
	ClaimFeatureJob(context.Context, int64, int64, string) (*FeatureJob, error)
	CommitFeaturePartition(context.Context, *FeatureJob, FeaturePartition, []FeatureValue) error
	FinishFeatureJob(context.Context, *FeatureJob, string, string, *time.Time) error
	CancelFeatureJob(context.Context, int64, int64, string, int) error
	WithdrawFeatureJob(context.Context, int64, int64, string, int) error
	RetryFeatureJob(context.Context, int64, int64, string, int) error
	FeatureQueryOwned(context.Context, int64, int64, []string, []FeatureValue, time.Time) ([]FeatureValue, error)
}
