// Package apisource defines credential-bound, candidate-only JSON ingestion.
package apisource

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	l "cognida/internal/model/lifecycle"
)

var (
	ErrInvalid   = errors.New("invalid API ingestion request")
	ErrForbidden = errors.New("API source access denied")
	ErrConflict  = errors.New("API ingestion state conflict")
	ErrNotFound  = errors.New("API source not found")
)

const JobType = "api.ingest"
const MaxNormalizedBytes = 4 << 20

var Identifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.-]{0,63}$`)

type Credential struct {
	ID         string    `json:"id"`
	TenantID   int64     `json:"-"`
	UserID     int64     `json:"-"`
	Name       string    `json:"name"`
	Origin     string    `json:"origin"`
	Kind       string    `json:"kind"`
	Ciphertext string    `json:"-"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
}
type CredentialInput struct {
	Name   string `json:"name"`
	Origin string `json:"origin"`
	Kind   string `json:"kind"`
	Secret string `json:"secret"`
}
type Limits struct {
	MaxPages       int   `json:"max_pages"`
	MaxRecords     int   `json:"max_records"`
	MaxBytes       int64 `json:"max_bytes"`
	MaxPageBytes   int64 `json:"max_page_bytes"`
	MaxRequests    int   `json:"max_requests"`
	TimeoutSeconds int   `json:"timeout_seconds"`
}
type Pagination struct {
	Mode              string `json:"mode"`
	Parameter         string `json:"parameter,omitempty"`
	PageSizeParameter string `json:"page_size_parameter,omitempty"`
	PageSize          int    `json:"page_size,omitempty"`
	NextPointer       string `json:"next_pointer,omitempty"`
	SnapshotPointer   string `json:"snapshot_pointer,omitempty"`
	SnapshotParameter string `json:"snapshot_parameter,omitempty"`
}
type Config struct {
	URL                string     `json:"url"`
	CredentialID       string     `json:"credential_id"`
	ItemsPointer       string     `json:"items_pointer"`
	RecordKey          string     `json:"record_key"`
	WatermarkField     string     `json:"watermark_field,omitempty"`
	WatermarkParameter string     `json:"watermark_parameter,omitempty"`
	DeletedField       string     `json:"deleted_field,omitempty"`
	Pagination         Pagination `json:"pagination"`
	Limits             Limits     `json:"limits"`
}
type Source struct {
	ID        string    `json:"id"`
	TenantID  int64     `json:"-"`
	UserID    int64     `json:"-"`
	Name      string    `json:"name"`
	Version   int       `json:"version"`
	Enabled   bool      `json:"enabled"`
	Config    Config    `json:"config"`
	Watermark string    `json:"watermark,omitempty"`
	ActiveRun string    `json:"active_run,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type SourceInput struct {
	Name            string `json:"name"`
	ExpectedVersion int    `json:"expected_version,omitempty"`
	Config          Config `json:"config"`
}
type PageReceipt struct {
	Number     int       `json:"number"`
	Digest     string    `json:"digest"`
	Bytes      int64     `json:"bytes"`
	Records    int       `json:"records"`
	ObjectKey  string    `json:"-"`
	AcquiredAt time.Time `json:"acquired_at"`
}
type Run struct {
	ID              string        `json:"id"`
	TenantID        int64         `json:"-"`
	UserID          int64         `json:"-"`
	SourceID        string        `json:"source_id"`
	SourceVersion   int           `json:"source_version"`
	Version         int64         `json:"version"`
	OperationKey    string        `json:"-"`
	RequestHash     string        `json:"-"`
	Status          string        `json:"status"`
	Error           string        `json:"error,omitempty"`
	Consistency     string        `json:"consistency"`
	SnapshotDigest  string        `json:"snapshot_digest,omitempty"`
	Config          Config        `json:"config"`
	Pages           []PageReceipt `json:"pages"`
	RecordCount     int           `json:"record_count"`
	DuplicateCount  int           `json:"duplicate_count"`
	DeletedCount    int           `json:"deleted_count"`
	Bytes           int64         `json:"bytes"`
	Requests        int           `json:"requests"`
	WatermarkBefore string        `json:"watermark_before,omitempty"`
	WatermarkAfter  string        `json:"watermark_after,omitempty"`
	OutputRefs      []l.RefID     `json:"output_refs"`
	Deadline        time.Time     `json:"deadline"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
	// Internal state is encoded separately by the repository and never returned.
	CursorCiphertext   string            `json:"-"`
	SnapshotCiphertext string            `json:"-"`
	Records            []json.RawMessage `json:"-"`
	Keys               map[string]string `json:"-"`
	Epoch              int64             `json:"-"`
	LeaseUntil         time.Time         `json:"-"`
	Retryable          bool              `json:"retryable"`
}

func (r Run) Terminal() bool {
	return r.Status == "completed" || r.Status == "partial" || r.Status == "failed" || r.Status == "cancelled" || r.Status == "timed_out"
}
func (c Config) Validate() error {
	u, e := url.Parse(c.URL)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || c.CredentialID == "" || len(c.URL) > 4096 {
		return ErrInvalid
	}
	for key := range u.Query() {
		switch strings.ToLower(key) {
		case "token", "access_token", "api_key", "apikey", "authorization", "password", "secret":
			return ErrInvalid
		}
	}
	if !validPointer(c.ItemsPointer) || !Identifier.MatchString(c.RecordKey) {
		return ErrInvalid
	}
	if (c.WatermarkField == "") != (c.WatermarkParameter == "") || c.WatermarkField != "" && (!Identifier.MatchString(c.WatermarkField) || !Identifier.MatchString(c.WatermarkParameter)) {
		return ErrInvalid
	}
	if c.DeletedField != "" && !Identifier.MatchString(c.DeletedField) {
		return ErrInvalid
	}
	p := c.Pagination
	if p.Mode != "none" && p.Mode != "page" && p.Mode != "cursor" {
		return ErrInvalid
	}
	if p.Mode != "none" && (!Identifier.MatchString(p.Parameter) || !validPointer(p.NextPointer)) {
		return ErrInvalid
	}
	if p.PageSizeParameter != "" && (!Identifier.MatchString(p.PageSizeParameter) || p.PageSize < 1 || p.PageSize > 1000) {
		return ErrInvalid
	}
	if (p.SnapshotPointer == "") != (p.SnapshotParameter == "") || p.SnapshotPointer != "" && (!validPointer(p.SnapshotPointer) || !Identifier.MatchString(p.SnapshotParameter)) {
		return ErrInvalid
	}
	names := map[string]bool{}
	for _, name := range []string{p.Parameter, p.PageSizeParameter, p.SnapshotParameter, c.WatermarkParameter} {
		if name != "" {
			if names[name] || u.Query().Has(name) {
				return ErrInvalid
			}
			names[name] = true
		}
	}
	b := c.Limits
	if b.MaxPages < 1 || b.MaxPages > 100 || b.MaxRecords < 1 || b.MaxRecords > 10000 || b.MaxPageBytes < 1024 || b.MaxPageBytes > 8<<20 || b.MaxBytes < b.MaxPageBytes || b.MaxBytes > 32<<20 || b.MaxRequests < b.MaxPages || b.MaxRequests > 200 || b.TimeoutSeconds < 1 || b.TimeoutSeconds > 300 {
		return ErrInvalid
	}
	return nil
}
func validPointer(s string) bool {
	if s == "" {
		return true
	}
	if !strings.HasPrefix(s, "/") || len(s) > 512 {
		return false
	}
	for _, p := range strings.Split(s[1:], "/") {
		if p == "" || strings.Contains(p, "~") {
			return false
		}
	}
	return true
}

type Checkpoint struct {
	ExpectedIndex                                                        int
	Page                                                                 PageReceipt
	Records                                                              []json.RawMessage
	Keys                                                                 map[string]string
	DuplicateCount, DeletedCount                                         int
	CursorCiphertext, SnapshotCiphertext, SnapshotDigest, WatermarkAfter string
	Complete                                                             bool
}
type Repository interface {
	CreateCredential(context.Context, l.Principal, Credential) (*Credential, error)
	Credential(context.Context, l.Principal, string) (*Credential, error)
	DisableCredential(context.Context, l.Principal, string) error
	ListCredentials(context.Context, l.Principal, string, int) ([]Credential, error)
	PutSource(context.Context, l.Principal, string, SourceInput) (*Source, error)
	Source(context.Context, l.Principal, string, int) (*Source, error)
	ListSources(context.Context, l.Principal, string, int) ([]Source, error)
	SetEnabled(context.Context, l.Principal, string, int, bool) (*Source, error)
	Start(context.Context, l.Principal, string, int, string, string) (*Run, error)
	GetRun(context.Context, l.Principal, string) (*Run, error)
	Pending(context.Context, int) ([]Run, error)
	Claim(context.Context, l.Principal, string, time.Duration) (*Run, error)
	ReserveRequest(context.Context, l.Principal, string, int64) (*Run, error)
	Checkpoint(context.Context, l.Principal, string, int64, Checkpoint) (*Run, error)
	Finish(context.Context, l.Principal, string, int64, string, string, bool, []l.RefID) (*Run, error)
	Cancel(context.Context, l.Principal, string) (*Run, error)
	Resume(context.Context, l.Principal, string) (*Run, error)
}
