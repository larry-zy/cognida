package governance

import (
	"context"
	"encoding/json"
	"time"
)

type DriftSampling struct {
	Limit   int    `json:"limit"`
	Seed    string `json:"seed"`
	Minimum int    `json:"minimum"`
}
type DriftBaseline struct {
	SourceRefID  string        `json:"source_ref_id"`
	SourceDigest string        `json:"source_digest"`
	Name         string        `json:"name"`
	Sampling     DriftSampling `json:"sampling"`
	ApprovedBy   int64         `json:"approved_by,omitempty"`
	ApprovedAt   *time.Time    `json:"approved_at,omitempty"`
}
type DriftMetric struct {
	Field         string   `json:"field"`
	Method        string   `json:"method"`
	BaselineCount int      `json:"baseline_count"`
	CurrentCount  int      `json:"current_count"`
	Effect        *float64 `json:"effect,omitempty"`
	Threshold     float64  `json:"threshold"`
	Status        string   `json:"status"`
	Detail        string   `json:"detail,omitempty"`
}
type DriftReport struct {
	BaselineID      string        `json:"baseline_id"`
	BaselineVersion int           `json:"baseline_version"`
	BaselineRefID   string        `json:"baseline_ref_id"`
	CurrentRefID    string        `json:"current_ref_id"`
	BaselineDigest  string        `json:"baseline_digest"`
	CurrentDigest   string        `json:"current_digest"`
	Sampling        DriftSampling `json:"sampling"`
	BaselineRows    int           `json:"baseline_rows"`
	CurrentRows     int           `json:"current_rows"`
	BaselineSample  int           `json:"baseline_sample"`
	CurrentSample   int           `json:"current_sample"`
	Metrics         []DriftMetric `json:"metrics"`
	Changed         bool          `json:"changed"`
	Limitations     []string      `json:"limitations"`
	TaskID          string        `json:"task_id,omitempty"`
}
type DriftMonitor struct {
	Name              string         `json:"name"`
	BaselineID        string         `json:"baseline_id"`
	CurrentRefID      string         `json:"current_ref_id"`
	Refresh           *RefreshSource `json:"refresh,omitempty"`
	IntervalSeconds   int            `json:"interval_seconds"`
	Threshold         float64        `json:"threshold"`
	PendingBaselineID string         `json:"pending_baseline_id,omitempty"`
	LastReportID      string         `json:"last_report_id,omitempty"`
	LastError         string         `json:"last_error,omitempty"`
}
type DriftRecord struct {
	ID        string          `json:"id"`
	TenantID  int64           `json:"-"`
	UserID    int64           `json:"-"`
	Kind      string          `json:"kind"`
	Version   int             `json:"version"`
	Status    string          `json:"status"`
	Body      json.RawMessage `json:"body"`
	NextRunAt *time.Time      `json:"next_run_at,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}
type DriftEvent struct {
	ID        string    `json:"id"`
	RecordID  string    `json:"record_id"`
	Action    string    `json:"action"`
	ActorID   int64     `json:"actor_id"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
}
type DriftRepository interface {
	CreateDrift(context.Context, *DriftRecord, string) (*DriftRecord, error)
	GetDrift(context.Context, int64, int64, string) (*DriftRecord, error)
	ListDrift(context.Context, int64, int64, string, string) ([]DriftRecord, string, error)
	UpdateDrift(context.Context, *DriftRecord, int, string) error
	ClaimDriftMonitors(context.Context, int) ([]DriftRecord, error)
	DriftEvents(context.Context, int64, int64, string) ([]DriftEvent, error)
}
