// Package queryjob defines durable, bounded, owner-scoped external read queries.
package queryjob

import (
	ds "cognida/internal/model/datasource"
	l "cognida/internal/model/lifecycle"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalid   = errors.New("invalid query job")
	ErrForbidden = errors.New("query source access denied")
	ErrNotFound  = errors.New("query job not found")
	ErrConflict  = errors.New("query job state conflict")
)

type Input struct {
	DataSourceID   string `json:"datasource_id"`
	SQL            string `json:"sql"`
	MaxRows        int    `json:"max_rows"`
	MaxBytes       int    `json:"max_bytes"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MaxScanBytes   int64  `json:"max_scan_bytes,omitempty"`
}

func (i Input) Validate() error {
	if strings.TrimSpace(i.DataSourceID) == "" || len(i.DataSourceID) > 64 || strings.TrimSpace(i.SQL) == "" || len(i.SQL) > 64<<10 || i.MaxRows < 1 || i.MaxRows > 10000 || i.MaxBytes < 1024 || i.MaxBytes > 8<<20 || i.TimeoutSeconds < 1 || i.TimeoutSeconds > 30 || i.MaxScanBytes < 0 {
		return ErrInvalid
	}
	return nil
}
func ValidKey(s string) bool {
	if len(s) < 1 || len(s) > 100 {
		return false
	}
	for _, c := range s {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
func Digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type Job struct {
	ID              string    `json:"id"`
	TenantID        int64     `json:"-"`
	UserID          int64     `json:"-"`
	Input           Input     `json:"input"`
	Engine          ds.Type   `json:"engine"`
	SourceVersion   string    `json:"source_version"`
	Version         int64     `json:"version"`
	Status          string    `json:"status"`
	RemoteStatus    string    `json:"remote_status,omitempty"`
	Error           string    `json:"error,omitempty"`
	RowCount        int       `json:"row_count"`
	Bytes           int       `json:"bytes"`
	Truncated       bool      `json:"truncated"`
	CancelRequested bool      `json:"cancel_requested"`
	CleanupPending  bool      `json:"cleanup_pending"`
	Deadline        time.Time `json:"deadline"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (j Job) Terminal() bool {
	switch j.Status {
	case "succeeded", "failed", "cancelled", "timed_out", "aborted_unknown":
		return !j.CleanupPending
	}
	return false
}

// Record is server-private. Handle and cursor ciphertext must never appear in
// HTTP representations or logs, including reconciliation errors.
type Record struct {
	Job         Job             `json:"job"`
	Prepared    string          `json:"prepared"`
	Operation   string          `json:"operation"`
	Cursor      string          `json:"cursor"`
	SeenCursors []string        `json:"seen_cursors"`
	Phase       string          `json:"phase"`
	Result      *ds.QueryResult `json:"result,omitempty"`
	Epoch       int64           `json:"-"`
}
type Item struct {
	ID    string
	Scope l.Principal
}
type Repository interface {
	Create(context.Context, l.Principal, Input, ds.Type, string, string) (*Record, error)
	Get(context.Context, l.Principal, string) (*Record, error)
	List(context.Context, l.Principal, string, int) ([]Job, error)
	Runnable(context.Context, int) ([]Item, error)
	Claim(context.Context, l.Principal, string) (*Record, error)
	Change(context.Context, l.Principal, string, int64, func(*Record, time.Time) error) (*Record, error)
	Release(context.Context, l.Principal, string, int64) error
}
