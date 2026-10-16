// Package researchrun describes durable, scoped research execution records.
package researchrun

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
)

const Profile = "research-evidence-review-v1; questions<=3; supplementary_rounds<=1; model_calls<=4; no monetary guarantee"
const MaxCalls = 20 // bounded logical generations and retrievals; not transport or monetary accounting
const MaxTimeoutSeconds = 120

type Request struct {
	Query           string   `json:"query"`
	KBIDs           []string `json:"kb_ids"`
	GraphEnabled    bool     `json:"graph_enabled"`
	EnableWebSearch bool     `json:"enable_web_search"`
}

func (r Request) Validate() error {
	if !utf8.ValidString(r.Query) || strings.TrimSpace(r.Query) == "" || utf8.RuneCountInString(r.Query) > 4000 || strings.ContainsRune(r.Query, 0) || len(r.KBIDs) < 1 || len(r.KBIDs) > 50 {
		return d.ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range r.KBIDs {
		if !d.ValidText(id, 128) || strings.TrimSpace(id) != id || seen[id] {
			return d.ErrInvalid
		}
		seen[id] = true
	}
	return nil
}

type Submission struct {
	RequestID      string    `json:"request_id,omitempty"`
	OperationKey   string    `json:"operation_key"`
	PayloadHash    string    `json:"payload_hash"`
	Profile        string    `json:"profile"`
	Request        Request   `json:"request"`
	Deadline       time.Time `json:"deadline"`
	TimeoutSeconds int       `json:"timeout_seconds"`
	MaxCalls       int       `json:"max_calls"`
	MaxCostMicros  int64     `json:"max_cost_micros"`
}

func (s Submission) Validate() error {
	if s.RequestID != "" && !d.ValidText(s.RequestID, 128) {
		return d.ErrInvalid
	}
	for _, x := range []string{s.OperationKey, s.PayloadHash} {
		if len(x) != 64 || x != strings.ToLower(x) {
			return d.ErrInvalid
		}
		if _, e := hex.DecodeString(x); e != nil {
			return d.ErrInvalid
		}
	}
	if s.Profile != Profile || s.Deadline.IsZero() || s.TimeoutSeconds < 1 || s.TimeoutSeconds > MaxTimeoutSeconds || s.MaxCalls < MaxCalls || s.MaxCalls > 128 || s.MaxCostMicros != 0 {
		return d.ErrInvalid
	}
	return s.Request.Validate()
}

type Run struct {
	d.Scope      `gorm:"embedded" json:"-"`
	ID           string          `gorm:"column:id" json:"id"`
	OperationKey string          `gorm:"column:operation_key" json:"-"`
	PayloadHash  string          `gorm:"column:payload_hash" json:"-"`
	Submission   json.RawMessage `gorm:"column:submission;type:json" json:"submission"`
	Status       string          `gorm:"column:status" json:"status"`
	Stage        string          `gorm:"column:stage" json:"stage"`
	Progress     float64         `gorm:"column:progress" json:"progress"`
	Version      int64           `gorm:"column:version" json:"version"`
	Owner        string          `gorm:"column:owner" json:"-"`
	LeaseUntilUS int64           `gorm:"column:lease_until_us" json:"-"`
	Report       json.RawMessage `gorm:"column:report;type:json" json:"report,omitempty"`
	OutputRefs   json.RawMessage `gorm:"column:output_refs;type:json" json:"output_refs"`
	Error        string          `gorm:"column:error" json:"error,omitempty"`
	CreatedAt    time.Time       `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time       `gorm:"column:updated_at" json:"updated_at"`
}

func (Run) TableName() string { return "research_runs" }
func (r Run) Terminal() bool {
	return r.Status == "succeeded" || r.Status == "partial" || r.Status == "failed" || r.Status == "cancelled" || r.Status == "timed_out"
}

type Completion struct {
	Report     json.RawMessage
	OutputRefs json.RawMessage
	Error      string
	Status     string
}
type Repository interface {
	Submit(context.Context, d.Scope, Submission, bool) (*Run, error)
	Lookup(context.Context, d.Scope, string, string) (*Run, error)
	Get(context.Context, d.Scope, string) (*Run, error)
	FindOutput(context.Context, d.Scope, l.RefID) (*Run, error)
	Cancel(context.Context, d.Scope, string) (*Run, error)
	Pending(context.Context, int) ([]Run, error)
	Claim(context.Context, d.Scope, string, string, time.Duration) (*Run, error)
	Renew(context.Context, d.Scope, string, string, time.Duration) (*Run, error)
	Checkpoint(context.Context, d.Scope, string, string, string, float64, json.RawMessage) (*Run, error)
	Complete(context.Context, d.Scope, string, string, Completion) (*Run, error)
}
