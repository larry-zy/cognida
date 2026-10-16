// Package apisource implements bounded authenticated JSON snapshots. Outputs are
// candidates: neither the worker nor any recovery path publishes knowledge.
package apisource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"cognida/internal/infrastructure/job"
	d "cognida/internal/model/apisource"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/model/objectstore"
	"cognida/internal/service/collection/fetcher"
	"cognida/internal/service/datasource"
	"github.com/google/uuid"
)

type Assets interface {
	ImportDerived(context.Context, l.Principal, l.Kind, string, []l.RefID, json.RawMessage, l.PolicySnapshot) (l.DataAssetRef, error)
}
type JSONFetcher interface {
	FetchJSON(context.Context, string, fetcher.JSONCredential, int64) (*fetcher.Result, error)
}
type Authorizer func(context.Context, l.Principal, string) error
type Service struct {
	repo      d.Repository
	store     objectstore.Store
	assets    Assets
	queue     job.Queue
	cipher    *datasource.Cipher
	fetch     JSONFetcher
	authorize Authorizer
	mu        sync.Mutex
	active    map[string]context.CancelFunc
}

func New(repo d.Repository, store objectstore.Store, assets Assets, queue job.Queue, cipher *datasource.Cipher, fetch JSONFetcher, authorize Authorizer) (*Service, error) {
	if repo == nil || store == nil || assets == nil || queue == nil || cipher == nil || fetch == nil || authorize == nil {
		return nil, d.ErrInvalid
	}
	return &Service{repo: repo, store: store, assets: assets, queue: queue, cipher: cipher, fetch: fetch, authorize: authorize, active: map[string]context.CancelFunc{}}, nil
}
func (s *Service) auth(ctx context.Context, p l.Principal, action string) error {
	if p.Validate() != nil {
		return d.ErrForbidden
	}
	if e := s.authorize(ctx, p, action); e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return d.ErrForbidden
	}
	return nil
}
func hash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func (s *Service) CreateCredential(ctx context.Context, p l.Principal, in d.CredentialInput, key string) (*d.Credential, error) {
	if e := s.auth(ctx, p, "admin"); e != nil {
		return nil, e
	}
	if strings.TrimSpace(key) == "" || len(key) > 200 {
		return nil, d.ErrInvalid
	}
	origin, e := fetcher.NormalizeURL(in.Origin)
	if e != nil {
		return nil, d.ErrInvalid
	}
	u, _ := url.Parse(origin)
	if u.Path != "/" || u.RawQuery != "" || strings.TrimSpace(in.Name) == "" || len(in.Name) > 128 || (in.Kind != "bearer" && in.Kind != "api_key") || len(in.Secret) < 16 || len(in.Secret) > 4096 || strings.ContainsAny(in.Secret, "\r\n\x00") {
		return nil, d.ErrInvalid
	}
	ciphertext, e := s.cipher.Encrypt(in.Secret)
	if e != nil {
		return nil, d.ErrInvalid
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("api-credential/%d/%d/%s", p.TenantID, p.UserID, key))).String()
	created, e := s.repo.CreateCredential(ctx, p, d.Credential{ID: id, Name: strings.TrimSpace(in.Name), Origin: origin, Kind: in.Kind, Ciphertext: ciphertext})
	if e != nil {
		return nil, e
	}
	prior, e := s.cipher.Decrypt(created.Ciphertext)
	if e != nil || prior != in.Secret || created.Name != strings.TrimSpace(in.Name) || created.Origin != origin || created.Kind != in.Kind {
		return nil, d.ErrConflict
	}
	return created, nil
}
func (s *Service) ListCredentials(ctx context.Context, p l.Principal, cursor string, limit int) ([]d.Credential, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	return s.repo.ListCredentials(ctx, p, cursor, limit)
}
func (s *Service) DisableCredential(ctx context.Context, p l.Principal, id string) error {
	if e := s.auth(ctx, p, "admin"); e != nil {
		return e
	}
	return s.repo.DisableCredential(ctx, p, id)
}
func (s *Service) credential(ctx context.Context, p l.Principal, config d.Config) (fetcher.JSONCredential, error) {
	c, e := s.repo.Credential(ctx, p, config.CredentialID)
	if e != nil || !c.Enabled {
		return fetcher.JSONCredential{}, d.ErrForbidden
	}
	u, e := url.Parse(config.URL)
	if e != nil {
		return fetcher.JSONCredential{}, d.ErrInvalid
	}
	origin, _ := url.Parse(c.Origin)
	if u.Scheme != origin.Scheme || u.Host != origin.Host {
		return fetcher.JSONCredential{}, d.ErrForbidden
	}
	secret, e := s.cipher.Decrypt(c.Ciphertext)
	if e != nil {
		return fetcher.JSONCredential{}, d.ErrForbidden
	}
	header, value := "X-API-Key", secret
	if c.Kind == "bearer" {
		header = "Authorization"
		value = "Bearer " + secret
	} else if c.Kind != "api_key" {
		return fetcher.JSONCredential{}, d.ErrForbidden
	}
	return fetcher.JSONCredential{Origin: c.Origin, Header: header, Value: value}, nil
}
func (s *Service) PutSource(ctx context.Context, p l.Principal, id string, in d.SourceInput, key string) (*d.Source, error) {
	if e := s.auth(ctx, p, "admin"); e != nil {
		return nil, e
	}
	if in.Config.Validate() != nil || strings.TrimSpace(in.Name) == "" || len(in.Name) > 128 {
		return nil, d.ErrInvalid
	}
	normalized, e := fetcher.NormalizeURL(in.Config.URL)
	if e != nil {
		return nil, d.ErrInvalid
	}
	in.Config.URL = normalized
	if _, e = s.credential(ctx, p, in.Config); e != nil {
		return nil, e
	}
	if id == "" {
		if strings.TrimSpace(key) == "" || len(key) > 200 || in.ExpectedVersion != 0 {
			return nil, d.ErrInvalid
		}
		id = uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("api-source/%d/%d/%s", p.TenantID, p.UserID, key))).String()
	} else if in.ExpectedVersion < 1 {
		return nil, d.ErrInvalid
	}
	return s.repo.PutSource(ctx, p, id, in)
}
func (s *Service) ListSources(ctx context.Context, p l.Principal, cursor string, limit int) ([]d.Source, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	return s.repo.ListSources(ctx, p, cursor, limit)
}
func (s *Service) Source(ctx context.Context, p l.Principal, id string, version int) (*d.Source, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	return s.repo.Source(ctx, p, id, version)
}
func (s *Service) SetEnabled(ctx context.Context, p l.Principal, id string, version int, enabled bool) (*d.Source, error) {
	if e := s.auth(ctx, p, "admin"); e != nil {
		return nil, e
	}
	return s.repo.SetEnabled(ctx, p, id, version, enabled)
}
func (s *Service) check(ctx context.Context, p l.Principal, run *d.Run) (fetcher.JSONCredential, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return fetcher.JSONCredential{}, e
	}
	source, e := s.repo.Source(ctx, p, run.SourceID, run.SourceVersion)
	if e != nil || !source.Enabled {
		return fetcher.JSONCredential{}, d.ErrForbidden
	}
	return s.credential(ctx, p, run.Config)
}

type delivery struct {
	TenantID int64  `json:"tenant_id"`
	UserID   int64  `json:"user_id"`
	ID       string `json:"id"`
}

func makeJob(run *d.Run) (job.Job, error) {
	return job.New(run.ID, d.JobType, delivery{run.TenantID, run.UserID, run.ID})
}
func (s *Service) Start(ctx context.Context, p l.Principal, id string, version int, key string) (*d.Run, error) {
	if e := s.auth(ctx, p, "admin"); e != nil {
		return nil, e
	}
	if strings.TrimSpace(key) == "" || len(key) > 200 || version < 1 {
		return nil, d.ErrInvalid
	}
	source, e := s.repo.Source(ctx, p, id, version)
	if e != nil {
		return nil, e
	}
	if !source.Enabled {
		return nil, d.ErrForbidden
	}
	if _, e = s.credential(ctx, p, source.Config); e != nil {
		return nil, e
	}
	raw, _ := json.Marshal(struct {
		ID      string
		Version int
	}{id, version})
	run, e := s.repo.Start(ctx, p, id, version, hash([]byte(key)), hash(raw))
	if e != nil {
		return nil, e
	}
	if !run.Terminal() {
		j, _ := makeJob(run)
		if e = s.queue.Enqueue(ctx, j); e != nil {
			return run, errors.New("API ingestion persisted; delivery pending")
		}
	}
	return run, nil
}
func (s *Service) Read(ctx context.Context, p l.Principal, id string) (*d.Run, error) {
	run, e := s.repo.GetRun(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if _, e = s.check(ctx, p, run); e != nil {
		return nil, e
	}
	copy := *run
	if copy.Error == "pages_complete" {
		copy.Error = ""
	}
	return &copy, nil
}
func (s *Service) Cancel(ctx context.Context, p l.Principal, id string) (*d.Run, error) {
	run, e := s.repo.Cancel(ctx, p, id)
	if e != nil {
		return nil, e
	}
	s.mu.Lock()
	cancel := s.active[id]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if _, e = s.check(ctx, p, run); e != nil {
		copy := *run
		copy.OutputRefs = nil
		copy.Pages = nil
		copy.Config = d.Config{}
		return &copy, nil
	}
	return run, nil
}
func (s *Service) Resume(ctx context.Context, p l.Principal, id string) (*d.Run, error) {
	run, e := s.Read(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if e = s.auth(ctx, p, "admin"); e != nil {
		return nil, e
	}
	run, e = s.repo.Resume(ctx, p, id)
	if e != nil {
		return nil, e
	}
	j, _ := makeJob(run)
	if e = s.queue.Enqueue(ctx, j); e != nil {
		return run, errors.New("API ingestion persisted; delivery pending")
	}
	return run, nil
}
func (s *Service) PendingJobs(ctx context.Context, limit int) ([]job.Job, error) {
	rows, e := s.repo.Pending(ctx, limit)
	if e != nil {
		return nil, e
	}
	out := []job.Job{}
	for i := range rows {
		j, e := makeJob(&rows[i])
		if e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, nil
}
func (s *Service) ValidateGovernedOrigin(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
	if p.Validate() != nil || ref.TenantID != p.TenantID || !strings.HasPrefix(ref.ProducerID, "api-ingestion:") || len(ref.Parents) != 0 {
		return l.ErrForbidden
	}
	id := strings.TrimPrefix(ref.ProducerID, "api-ingestion:")
	run, e := s.Read(ctx, p, id)
	if e != nil || !run.Terminal() {
		return l.ErrForbidden
	}
	for _, output := range run.OutputRefs {
		if output == ref.RefID && ref.Provenance.SourceID == ref.ProducerID && ref.Provenance.SourceType == "lifecycle" {
			return nil
		}
	}
	return l.ErrForbidden
}

// Capability describes actual supported behavior rather than inferring source guarantees.
func Capabilities() map[string]any {
	return map[string]any{"method": "GET", "format": "JSON", "authentication": []string{"bearer", "api_key"}, "pagination": []string{"none", "page", "cursor"}, "snapshot": "requires explicit source token; otherwise best effort; unsafe recovery refused", "deletion": "explicit boolean tombstone field only; absence never implies deletion", "candidate_only": true, "publication_requires_approval": true, "max_timeout_seconds": 300, "max_pages": 100, "max_records": 10000}
}
