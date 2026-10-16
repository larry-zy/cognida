package queryjob

import (
	ds "cognida/internal/model/datasource"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/queryjob"
	sources "cognida/internal/service/datasource"
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type Cipher interface {
	Encrypt(string) (string, error)
	Decrypt(string) (string, error)
}

// Authorizer rechecks current membership and the exact registered source. It
// must not resolve credential paths supplied by clients.
type Authorizer func(context.Context, l.Principal, string) (*ds.DataSource, error)
type Service struct {
	repo      d.Repository
	provider  ds.QueryProvider
	authorize Authorizer
	cipher    Cipher
}

func New(repo d.Repository, provider ds.QueryProvider, authorize Authorizer, cipher Cipher) (*Service, error) {
	if repo == nil || provider == nil || authorize == nil || cipher == nil {
		return nil, d.ErrInvalid
	}
	return &Service{repo, provider, authorize, cipher}, nil
}
func sourceVersion(s *ds.DataSource) string {
	return d.Digest(struct {
		ID                           string
		Tenant                       int64
		Type                         ds.Type
		Host                         string
		Port                         int
		Database, Username, Password string
		Extra                        []byte
		Updated                      time.Time
	}{s.ID, s.TenantID, s.Type, s.Host, s.Port, s.DatabaseName, s.Username, s.PasswordEncrypted, s.Extra, s.UpdatedAt})
}
func (s *Service) source(ctx context.Context, p l.Principal, id string) (*ds.DataSource, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	v, e := s.authorize(ctx, p, id)
	if e != nil || v == nil || v.TenantID != p.TenantID || v.ID != id || v.Status == "disabled" || v.Status == "inactive" || v.Status == ds.StatusNeedCredentials {
		return nil, d.ErrForbidden
	}
	return v, nil
}
func (s *Service) authorized(ctx context.Context, p l.Principal, r *d.Record) error {
	v, e := s.source(ctx, p, r.Job.Input.DataSourceID)
	if e != nil {
		return e
	}
	if sourceVersion(v) != r.Job.SourceVersion {
		return d.ErrForbidden
	}
	return nil
}
func (s *Service) Create(ctx context.Context, p l.Principal, in d.Input, key string) (*d.Job, error) {
	if in.Validate() != nil || sources.ValidateReadQuery(in.SQL) != nil || !d.ValidKey(key) {
		return nil, d.ErrInvalid
	}
	v, e := s.source(ctx, p, in.DataSourceID)
	if e != nil {
		return nil, e
	}
	a, actual, e := s.provider.AcquireQuery(ctx, p.TenantID, in.DataSourceID)
	if e != nil || actual == nil || sourceVersion(actual) != sourceVersion(v) {
		return nil, d.ErrForbidden
	}
	if _, ok := a.(ds.AsyncQueryAdapter); !ok || !a.Capabilities().Async {
		return nil, d.ErrInvalid
	}
	if v.Type != ds.TypeBigQuery && v.Type != ds.TypeHive {
		return nil, d.ErrInvalid
	}
	if v.Type == ds.TypeBigQuery && in.MaxScanBytes < 1 || v.Type != ds.TypeBigQuery && in.MaxScanBytes != 0 {
		return nil, d.ErrInvalid
	}
	r, e := s.repo.Create(ctx, p, in, v.Type, sourceVersion(v), key)
	if e != nil {
		return nil, e
	}
	return &r.Job, nil
}
func (s *Service) Get(ctx context.Context, p l.Principal, id string) (*d.Job, error) {
	r, e := s.repo.Get(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if e = s.authorized(ctx, p, r); e != nil {
		return nil, e
	}
	return &r.Job, nil
}

type JobPage struct {
	Items      []d.Job `json:"items"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

func (s *Service) List(ctx context.Context, p l.Principal, cursor string, limit int) (JobPage, error) {
	out := JobPage{Items: []d.Job{}}
	rows, e := s.repo.List(ctx, p, cursor, limit)
	if e != nil {
		return out, e
	}
	for _, v := range rows {
		if s.authorized(ctx, p, &d.Record{Job: v}) == nil {
			out.Items = append(out.Items, v)
		}
	}
	if len(rows) == limit {
		out.NextCursor = rows[len(rows)-1].ID
	}
	return out, nil
}

// Cancellation is an owner command and remains available after source access is
// revoked. It does not claim the remote query was stopped before reconciliation.
func (s *Service) Cancel(ctx context.Context, p l.Principal, id string) (*d.Job, error) {
	r, e := s.repo.Change(ctx, p, id, 0, func(r *d.Record, _ time.Time) error {
		if !r.Job.Terminal() {
			r.Job.CancelRequested = true
			r.Job.Status = "cancel_requested"
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	out := r.Job
	out.Input = d.Input{DataSourceID: r.Job.Input.DataSourceID}
	return &out, nil
}

type Results struct {
	Columns     []string          `json:"columns"`
	ColumnTypes map[string]string `json:"column_types"`
	Rows        []map[string]any  `json:"rows"`
	NextCursor  string            `json:"next_cursor,omitempty"`
	Truncated   bool              `json:"truncated"`
}

func (s *Service) Results(ctx context.Context, p l.Principal, id, cursor string, limit int) (Results, error) {
	out := Results{}
	if limit < 1 || limit > 1000 {
		return out, d.ErrInvalid
	}
	r, e := s.repo.Get(ctx, p, id)
	if e != nil {
		return out, e
	}
	if e = s.authorized(ctx, p, r); e != nil {
		return out, e
	}
	if r.Job.Status != "succeeded" || r.Result == nil {
		return out, d.ErrConflict
	}
	offset := 0
	if cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(cursor)
		parts := strings.Split(string(raw), ":")
		if e != nil || len(parts) != 2 || parts[0] != id {
			return out, d.ErrInvalid
		}
		offset, e = strconv.Atoi(parts[1])
		if e != nil || offset < 0 || strconv.Itoa(offset) != parts[1] {
			return out, d.ErrInvalid
		}
	}
	if offset > len(r.Result.Rows) {
		return out, d.ErrInvalid
	}
	end := min(offset+limit, len(r.Result.Rows))
	out = Results{Columns: r.Result.Columns, ColumnTypes: r.Result.ColumnTypes, Rows: r.Result.Rows[offset:end], Truncated: r.Job.Truncated}
	if end < len(r.Result.Rows) {
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(id + ":" + strconv.Itoa(end)))
	}
	return out, nil
}
func (s *Service) seal(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	return s.cipher.Encrypt(v)
}
func (s *Service) open(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	return s.cipher.Decrypt(v)
}
func resultBytes(v any) int { b, _ := json.Marshal(v); return len(b) }
