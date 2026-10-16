package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	l "cognida/internal/model/lifecycle"
)

const (
	OfflineType      EvaluationType = "offline"
	OfflineProfile                  = "offline-text-v1; exact_match,contains_match; existing answers only; max_rows=100; batch_size=20; no model calls"
	OfflineMaxRows                  = 100
	OfflineBatchSize                = 20
	OfflineQueued                   = "queued"
	OfflineRunning                  = "running"
	OfflineSucceeded                = "succeeded"
	OfflineFailed                   = "failed"
	OfflineCancelled                = "cancelled"
	OfflineTimedOut                 = "timed_out"
)

var (
	ErrOfflineInvalid   = errors.New("invalid offline evaluation request")
	ErrOfflineConflict  = errors.New("offline evaluation state conflict")
	ErrOfflineForbidden = errors.New("offline evaluation access denied")
)

type OfflineScope struct {
	TenantID int64 `json:"tenant_id"`
	UserID   int64 `json:"user_id"`
}

func (p OfflineScope) Validate() error {
	if p.TenantID <= 0 || p.UserID <= 0 {
		return ErrOfflineForbidden
	}
	return nil
}

type OfflineColumns struct {
	QuestionColumn        string `json:"question_column"`
	ReferenceAnswerColumn string `json:"reference_answer_column"`
	GeneratedAnswerColumn string `json:"generated_answer_column"`
}

func (c OfflineColumns) Validate() error {
	seen := map[string]bool{}
	for _, s := range []string{c.QuestionColumn, c.ReferenceAnswerColumn, c.GeneratedAnswerColumn} {
		if strings.TrimSpace(s) == "" || len(s) > 128 || !utf8.ValidString(s) || strings.ContainsRune(s, 0) || seen[s] {
			return ErrOfflineInvalid
		}
		seen[s] = true
	}
	return nil
}

type OfflineRequest struct {
	OperationKey   string         `json:"operation_key"`
	PayloadHash    string         `json:"payload_hash"`
	Profile        string         `json:"profile"`
	InputRef       l.RefID        `json:"input_ref"`
	Columns        OfflineColumns `json:"columns"`
	TimeoutSeconds int            `json:"timeout_seconds"`
	Deadline       time.Time      `json:"deadline"`
	RequestID      string         `json:"request_id"`
}

func offlineDigest(s string) bool {
	if len(s) != 64 || strings.ToLower(s) != s {
		return false
	}
	_, e := hex.DecodeString(s)
	return e == nil
}
func (r OfflineRequest) Validate() error {
	if !offlineDigest(r.OperationKey) || !offlineDigest(r.PayloadHash) || r.Profile != OfflineProfile || r.InputRef.Validate() != nil || r.Columns.Validate() != nil || r.TimeoutSeconds < 1 || r.TimeoutSeconds > 300 || r.Deadline.IsZero() || len(r.RequestID) > 256 || !utf8.ValidString(r.RequestID) {
		return ErrOfflineInvalid
	}
	return nil
}

type OfflineItem struct {
	Question        string `json:"question"`
	ReferenceAnswer string `json:"reference_answer"`
	GeneratedAnswer string `json:"generated_answer"`
}

func (i OfflineItem) Validate() error {
	for _, s := range []string{i.Question, i.ReferenceAnswer, i.GeneratedAnswer} {
		if len(s) > 8192 || !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
			return ErrOfflineInvalid
		}
	}
	if strings.TrimSpace(i.Question) == "" || strings.TrimSpace(i.ReferenceAnswer) == "" {
		return ErrOfflineInvalid
	}
	return nil
}

type OfflineScore struct {
	Index         int     `json:"index"`
	ExactMatch    float64 `json:"exact_match"`
	ContainsMatch float64 `json:"contains_match"`
}
type OfflineRun struct {
	ID                string         `json:"id"`
	Scope             OfflineScope   `json:"scope"`
	Request           OfflineRequest `json:"request"`
	SnapshotHash      string         `json:"snapshot_hash"`
	Items             []OfflineItem  `json:"items"`
	Scores            []OfflineScore `json:"scores"`
	NextIndex         int            `json:"next_index"`
	Status            string         `json:"status"`
	Error             string         `json:"error,omitempty"`
	Version           int64          `json:"version"`
	Epoch             int64          `json:"epoch"`
	LeaseUntil        time.Time      `json:"lease_until"`
	ExecutionDeadline time.Time      `json:"execution_deadline"`
	OutputRef         *l.RefID       `json:"output_ref,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

func (r *OfflineRun) Terminal() bool {
	return r.Status == OfflineSucceeded || r.Status == OfflineFailed || r.Status == OfflineCancelled || r.Status == OfflineTimedOut
}

// OfflineRepository atomically maintains the trusted binding/checkpoint and the
// existing evaluation_tasks/evaluation_results projections. Scope is mandatory.
type OfflineRepository interface {
	Submit(context.Context, OfflineScope, OfflineRequest, []OfflineItem, string) (*OfflineRun, error)
	Lookup(context.Context, OfflineScope, string) (*OfflineRun, error)
	Get(context.Context, OfflineScope, string) (*OfflineRun, error)
	CancelSubmission(context.Context, OfflineScope, OfflineRequest) (*OfflineRun, error)
	Claim(context.Context, OfflineScope, string, time.Duration) (*OfflineRun, error)
	Renew(context.Context, OfflineScope, string, int64, time.Duration) error
	Checkpoint(context.Context, OfflineScope, string, int64, int, []OfflineScore) (*OfflineRun, error)
	Finish(context.Context, OfflineScope, string, int64, string, string, *l.RefID) (*OfflineRun, error)
	Release(context.Context, OfflineScope, string, int64) error
	Pending(context.Context, int) ([]OfflineRun, error)
}

func OfflineSnapshotHash(items []OfflineItem) string {
	raw, _ := json.Marshal(items)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func ValidateOfflineSnapshot(items []OfflineItem, hash string) error {
	if len(items) < 1 || len(items) > OfflineMaxRows || hash != OfflineSnapshotHash(items) {
		return ErrOfflineInvalid
	}
	for _, item := range items {
		if e := item.Validate(); e != nil {
			return e
		}
	}
	raw, _ := json.Marshal(items)
	if len(raw) > 1<<20 {
		return ErrOfflineInvalid
	}
	return nil
}
func (s OfflineScore) Validate() error {
	if s.Index < 0 || s.Index >= OfflineMaxRows {
		return ErrOfflineInvalid
	}
	for _, v := range []float64{s.ExactMatch, s.ContainsMatch} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 100 {
			return ErrOfflineInvalid
		}
	}
	return nil
}
func OfflineAverages(scores []OfflineScore) map[string]float64 {
	out := map[string]float64{}
	if len(scores) == 0 {
		return out
	}
	for _, s := range scores {
		out["exact_match"] += s.ExactMatch
		out["contains_match"] += s.ContainsMatch
	}
	out["exact_match"] /= float64(len(scores))
	out["contains_match"] /= float64(len(scores))
	return out
}
