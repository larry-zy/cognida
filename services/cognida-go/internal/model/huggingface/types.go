// Package huggingface defines bounded, static Hub snapshots. No dataset code runs.
package huggingface

import (
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid_request")
	ErrForbidden   = errors.New("forbidden")
	ErrNotFound    = errors.New("not_found")
	ErrConflict    = errors.New("conflict")
	ErrUpstream    = errors.New("upstream_error")
	ErrDecode      = errors.New("decode_error")
	ErrLimit       = errors.New("limit_exceeded")
	ErrUnavailable = errors.New("unavailable")
	repoPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,95}/[A-Za-z0-9][A-Za-z0-9_.-]{0,95}$`)
	commitPattern  = regexp.MustCompile(`^[a-f0-9]{40}$`)
)

const MaxDownloadBytes int64 = 16 << 20
const MaxFileBytes int64 = 8 << 20

type ColumnMapping struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Encoding string `json:"encoding,omitempty"`
}
type FieldMapping struct {
	TextField string          `json:"text_field,omitempty"`
	IDField   string          `json:"id_field,omitempty"`
	Columns   []ColumnMapping `json:"columns,omitempty"`
}
type Request struct {
	RepoID       string       `json:"repo_id"`
	Revision     string       `json:"revision"`
	Config       string       `json:"config"`
	Split        string       `json:"split"`
	Files        []string     `json:"files"`
	FieldMapping FieldMapping `json:"field_mapping"`
	MaxRows      int          `json:"max_rows"`
	MaxBytes     int64        `json:"max_bytes"`
	CredentialID string       `json:"credential_id,omitempty"`
}

func ValidRepo(s string) bool   { return repoPattern.MatchString(s) && !strings.Contains(s, "..") }
func ValidCommit(s string) bool { return commitPattern.MatchString(s) }
func ValidPath(s string) bool {
	return len(s) > 0 && len(s) <= 512 && path.Clean(s) == s && !strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "../") && s != ".." && !strings.ContainsAny(s, "\\\x00\r\n?#%")
}
func Format(s string) string {
	s = strings.ToLower(s)
	gz := ""
	if strings.HasSuffix(s, ".gz") {
		gz = ".gz"
		s = strings.TrimSuffix(s, ".gz")
	}
	for _, ext := range []string{"jsonl", "json", "csv", "parquet"} {
		if strings.HasSuffix(s, "."+ext) && !(ext == "parquet" && gz != "") {
			return ext + gz
		}
	}
	return ""
}
func field(s string) bool {
	return len(s) > 0 && len(s) <= 128 && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "\x00\r\n")
}
func (r Request) Validate() error {
	if !ValidRepo(r.RepoID) || !ValidCommit(r.Revision) || !field(r.Config) || !field(r.Split) || len(r.Files) < 1 || len(r.Files) > 4 || r.MaxRows < 1 || r.MaxRows > 1000 || r.MaxBytes < 1 || r.MaxBytes > MaxDownloadBytes || len(r.CredentialID) > 128 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, f := range r.Files {
		if !ValidPath(f) || Format(f) == "" || seen[f] {
			return ErrInvalid
		}
		seen[f] = true
	}
	m := r.FieldMapping
	if len(m.Columns) > 0 {
		if m.TextField != "" || m.IDField != "" || len(m.Columns) > 32 {
			return ErrInvalid
		}
		seen = map[string]bool{}
		for _, c := range m.Columns {
			if !field(c.Source) || !field(c.Target) || seen[c.Target] || (c.Encoding != "" && c.Encoding != "json") {
				return ErrInvalid
			}
			seen[c.Target] = true
		}
	} else if !field(m.TextField) || (m.IDField != "" && !field(m.IDField)) {
		return ErrInvalid
	}
	return nil
}

type SearchItem struct {
	RepoID    string `json:"repo_id"`
	SHA       string `json:"sha,omitempty"`
	Downloads int64  `json:"downloads"`
	Likes     int64  `json:"likes"`
	Private   bool   `json:"private"`
	Gated     bool   `json:"gated"`
}
type SearchResult struct {
	Items      []SearchItem `json:"items"`
	NextCursor string       `json:"next_cursor"`
}
type File struct {
	Path      string `json:"path"`
	Size      *int64 `json:"size"`
	Format    string `json:"format"`
	Supported bool   `json:"supported"`
	SHA256    string `json:"sha256,omitempty"`
}
type Split struct {
	Name  string   `json:"name"`
	Files []string `json:"files"`
}
type Config struct {
	Name   string  `json:"name"`
	Splits []Split `json:"splits"`
}
type Dataset struct {
	RepoID   string   `json:"repo_id"`
	Revision string   `json:"revision"`
	Card     string   `json:"card"`
	License  string   `json:"license"`
	Files    []File   `json:"files"`
	Configs  []Config `json:"configs"`
	Warnings []string `json:"warnings"`
}
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
type FileReceipt struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	Bytes     int64  `json:"bytes"`
	Rows      int    `json:"rows"`
	TotalRows *int64 `json:"total_rows"`
	Truncated bool   `json:"truncated"`
}
type Manifest struct {
	RepoID          string        `json:"repo_id"`
	Revision        string        `json:"revision"`
	Config          string        `json:"config"`
	Split           string        `json:"split"`
	Files           []FileReceipt `json:"files"`
	Mapping         FieldMapping  `json:"field_mapping"`
	License         string        `json:"license"`
	Rows            int           `json:"rows"`
	Bytes           int64         `json:"bytes"`
	TotalRows       *int64        `json:"total_rows"`
	Sampled         bool          `json:"sampled"`
	Truncated       bool          `json:"truncated"`
	ExcludedColumns []string      `json:"excluded_columns"`
	AcquiredAt      time.Time     `json:"acquired_at"`
	RequestHash     string        `json:"request_hash"`
}
type Preview struct {
	NumberEncoding string            `json:"number_encoding"`
	Manifest       Manifest          `json:"manifest"`
	Columns        []string          `json:"columns"`
	ColumnTypes    map[string]string `json:"column_types"`
	SourceColumns  []Column          `json:"source_columns"`
	Rows           [][]any           `json:"rows"`
	PreviewRows    int               `json:"preview_rows"`
}
type Import struct {
	ID          string    `json:"id"`
	TenantID    int64     `json:"-"`
	UserID      int64     `json:"-"`
	Request     Request   `json:"request"`
	RequestHash string    `json:"request_hash"`
	Status      string    `json:"status"`
	Manifest    *Manifest `json:"manifest,omitempty"`
	OutputRefs  []l.RefID `json:"output_refs"`
	ErrorCode   string    `json:"error_code,omitempty"`
	Deadline    time.Time `json:"deadline"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Repository interface {
	Reserve(context.Context, l.Principal, string, string, Request) (*Import, bool, error)
	Get(context.Context, l.Principal, string) (*Import, error)
	Finish(context.Context, l.Principal, string, *Manifest, []l.RefID, string) (*Import, error)
}

// DecodeResponse transports exact JSON number tokens in RowsJSON; Go uses UseNumber.
type DecodeResponse struct {
	Columns       []string          `json:"columns"`
	ColumnTypes   map[string]string `json:"column_types"`
	SourceColumns []Column          `json:"source_columns"`
	RowsJSON      json.RawMessage   `json:"-"`
	RowsEncoded   string            `json:"rows_json"`
	TotalRows     *int64            `json:"total_rows"`
	Truncated     bool              `json:"truncated"`
}
