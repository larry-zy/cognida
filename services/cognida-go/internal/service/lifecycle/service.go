package lifecycle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"cognida/internal/infrastructure/job"
	d "cognida/internal/model/lifecycle"
	"cognida/internal/model/objectstore"
	"github.com/google/uuid"
)

type Authorizer func(context.Context, d.Principal, string) error
type PublishFunc func(context.Context, d.Principal, d.DataAssetRef, json.RawMessage, map[string]any) (map[string]any, error)
type PublicationValidator func(context.Context, d.Principal, d.Input, map[string]any) error
type Compute interface {
	Call(context.Context, string, d.Input, map[string]any, string) (ComputeResult, error)
}
type Config struct {
	// StageTimeout bounds one adapter attempt, including remote publication
	// and indexing. Zero uses five minutes; approvals wait outside this budget.
	StageTimeout        time.Duration
	Audit               AuditFunc
	Access              AccessFunc
	Compute             Compute
	Publish             PublishFunc
	ValidatePublication PublicationValidator
	Authorize           Authorizer
	Adapters            map[string]d.Adapter
	Templates           []d.PipelineDefinition
}
type Service struct {
	repo      d.Repository
	store     objectstore.Store
	queue     job.Queue
	config    Config
	contracts map[string]d.AgentContract
	templates map[string]d.PipelineDefinition
	adapters  map[string]d.Adapter
}

func NewService(repo d.Repository, store objectstore.Store, queue job.Queue, config Config) (*Service, error) {
	if repo == nil || store == nil || queue == nil {
		return nil, fmt.Errorf("lifecycle repository, object storage and queue are required")
	}
	if config.StageTimeout <= 0 {
		config.StageTimeout = 5 * time.Minute
	}
	s := &Service{repo: repo, store: store, queue: queue, config: config, contracts: Contracts(), templates: map[string]d.PipelineDefinition{}, adapters: map[string]d.Adapter{}}
	def := DefaultTemplate()
	if e := def.Validate(s.contracts); e != nil {
		return nil, e
	}
	s.templates[def.ID] = def
	for _, custom := range config.Templates {
		if custom.ID == def.ID {
			return nil, d.ErrConflict
		}
		if e := custom.Validate(s.contracts); e != nil {
			return nil, e
		}
		s.templates[custom.ID] = custom
	}
	for id := range s.contracts {
		s.adapters[id] = adapterFunc(func(ctx context.Context, r d.StageRequest) (d.Output, error) { return s.executeAdapter(ctx, r) })
	}
	for id, a := range config.Adapters {
		if _, ok := s.contracts[id]; !ok || a == nil {
			return nil, d.ErrInvalid
		}
		s.adapters[id] = a
	}
	return s, nil
}

type adapterFunc func(context.Context, d.StageRequest) (d.Output, error)

func (f adapterFunc) Run(c context.Context, r d.StageRequest) (d.Output, error) { return f(c, r) }
func (s *Service) Repository() d.Repository                                     { return s.repo }
func (s *Service) Agents() []d.AgentContract {
	out := []d.AgentContract{}
	for _, id := range agentOrder {
		out = append(out, s.contracts[id])
	}
	return out
}
func (s *Service) Templates() []d.PipelineDefinition {
	out := []d.PipelineDefinition{s.templates["web-knowledge-to-insight"]}
	for id, def := range s.templates {
		if id != "web-knowledge-to-insight" {
			out = append(out, def)
		}
	}
	return out
}
func (s *Service) authorize(ctx context.Context, p d.Principal, scope string) error {
	if e := p.Validate(); e != nil {
		return e
	}
	if s.config.Authorize == nil {
		if scope != "read" {
			return d.ErrForbidden
		}
		return nil
	}
	return s.config.Authorize(ctx, p, scope)
}
func (s *Service) Import(ctx context.Context, p d.Principal, input d.Input, name string) (d.DataAssetRef, error) {
	if e := s.authorize(ctx, p, "read"); e != nil {
		return d.DataAssetRef{}, e
	}
	if e := input.Validate(); e != nil {
		return d.DataAssetRef{}, e
	}
	payload, e := json.Marshal(input)
	if e != nil {
		return d.DataAssetRef{}, e
	}
	return s.importPayload(ctx, p, d.RawAsset, uuid.NewString(), nil, payload, d.PolicySnapshot{Sensitivity: "unknown", Scopes: []string{"read"}, Version: "1"}, name)
}
func (s *Service) ImportDerived(ctx context.Context, p d.Principal, kind d.Kind, producer string, parents []d.RefID, payload json.RawMessage, policy d.PolicySnapshot) (d.DataAssetRef, error) {
	if e := s.authorize(ctx, p, "read"); e != nil {
		return d.DataAssetRef{}, e
	}
	name := ""
	for _, id := range parents {
		parent, _, e := s.Resolve(ctx, p.TenantID, p.UserID, id)
		if e != nil {
			return d.DataAssetRef{}, e
		}
		if name == "" {
			name = parent.Name
		}
	}
	return s.importPayload(ctx, p, kind, producer, parents, payload, policy, name)
}
func (s *Service) importPayload(ctx context.Context, p d.Principal, kind d.Kind, producer string, parents []d.RefID, payload json.RawMessage, policy d.PolicySnapshot, name string, origins ...d.Provenance) (d.DataAssetRef, error) {
	if producer == "" || !json.Valid(payload) || len(payload) > 8<<20 {
		return d.DataAssetRef{}, d.ErrInvalid
	}
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	refID := derivedRefID(p.TenantID, kind, producer, parents)
	id := refID.AssetID
	if existing, e := s.repo.FindAsset(ctx, p.TenantID, refID); e == nil {
		if existing.Provenance.Digest != digest {
			return d.DataAssetRef{}, d.ErrConflict
		}
		return existing, nil
	} else if !errors.Is(e, d.ErrNotFound) {
		return d.DataAssetRef{}, e
	}
	ref := d.DataAssetRef{RefID: refID, AssetRefID: refID.String(), SchemaVersion: d.SchemaVersion, TenantID: p.TenantID, Kind: kind, Status: "available", Name: name, ProducerID: producer, Parents: parents, Policy: policy, Provenance: d.Provenance{SourceType: "lifecycle", SourceID: producer, Digest: digest}, CreatedAt: time.Now().UTC(), ObjectKey: fmt.Sprintf("lifecycle/%d/%s/%d/%s.json", p.TenantID, id, refID.Version, digest)}
	if len(origins) > 0 {
		ref.Provenance = origins[0]
		ref.Provenance.Digest = digest
	}
	if _, e := s.store.Put(ctx, objectstore.PutRequest{Key: ref.ObjectKey, Body: bytes.NewReader(payload), Size: int64(len(payload)), ContentType: "application/json"}); e != nil {
		return ref, e
	}
	if e := s.repo.PutAsset(ctx, ref); e != nil {
		existing, findErr := s.repo.FindAsset(ctx, p.TenantID, refID)
		if findErr == nil {
			if existing.Provenance.Digest != digest {
				return d.DataAssetRef{}, d.ErrConflict
			}
			return existing, nil
		}
		return ref, e
	}
	return ref, nil
}
func (s *Service) resolve(ctx context.Context, tenantID, userID int64, id d.RefID) (d.DataAssetRef, json.RawMessage, error) {
	if e := s.authorize(ctx, d.Principal{TenantID: tenantID, UserID: userID}, "read"); e != nil {
		return d.DataAssetRef{}, nil, e
	}
	if e := id.Validate(); e != nil {
		return d.DataAssetRef{}, nil, e
	}
	ref, e := s.repo.FindAsset(ctx, tenantID, id)
	if e != nil {
		return ref, nil, e
	}
	if ref.ExpiresAt != nil && !ref.ExpiresAt.After(time.Now()) {
		return ref, nil, d.ErrExpired
	}
	if ref.Policy.Sensitivity == "restricted" || ref.Policy.Sensitivity == "confidential" || ref.Policy.Sensitivity == "secret" {
		if e := s.authorize(ctx, d.Principal{TenantID: tenantID, UserID: userID}, "sensitive"); e != nil {
			return ref, nil, e
		}
	}
	if ref.Status != "available" {
		return ref, nil, d.ErrConflict
	}
	reader, _, e := s.store.Get(ctx, ref.ObjectKey)
	if e != nil {
		return ref, nil, e
	}
	defer reader.Close()
	body, e := io.ReadAll(io.LimitReader(reader, (8<<20)+1))
	if e != nil {
		return ref, nil, e
	}
	sum := sha256.Sum256(body)
	if len(body) > 8<<20 || hex.EncodeToString(sum[:]) != ref.Provenance.Digest {
		return ref, nil, fmt.Errorf("asset integrity check failed")
	}
	return ref, body, nil
}
func validateKey(key string) error {
	if strings.TrimSpace(key) == "" || len(key) > 100 {
		return fmt.Errorf("%w: Idempotency-Key must be 1-100 characters", d.ErrInvalid)
	}
	return nil
}
func (s *Service) validateInputs(ctx context.Context, p d.Principal, agent string, refs []d.RefID) error {
	c, ok := s.contracts[agent]
	if !ok || len(refs) == 0 || len(refs) > 16 {
		return d.ErrInvalid
	}
	accepted := false
	for _, id := range refs {
		r, _, e := s.Resolve(ctx, p.TenantID, p.UserID, id)
		if e != nil {
			return e
		}
		if !c.Accepts(r.Kind) {
			return fmt.Errorf("%w: unsupported input kind %s for %s", d.ErrInvalid, r.Kind, agent)
		}
		accepted = true
	}
	if !accepted {
		return fmt.Errorf("%w: unsupported input kinds for %s", d.ErrInvalid, agent)
	}
	return nil
}
func (s *Service) Invoke(ctx context.Context, p d.Principal, agent string, refs []d.RefID, options map[string]any, key, requestID string) (*d.StageRun, error) {
	if e := validateKey(key); e != nil {
		return nil, e
	}
	if e := s.validateInputs(ctx, p, agent, refs); e != nil {
		return nil, e
	}
	if e := validateOptions(options); e != nil {
		return nil, e
	}
	if e := s.preflight(ctx, p, agent, refs, options); e != nil {
		return nil, e
	}
	stage := newStage(p, "", agent, agent, refs, options, "invoke:"+key, requestID, 3)
	if e := s.repo.CreateStage(ctx, &stage); e != nil {
		old, findErr := s.repo.FindStageByKey(ctx, p.TenantID, stage.IdempotencyKey)
		if findErr != nil {
			return nil, e
		}
		if old.AgentID != agent || !sameJSON(old.InputRefs, refs) || !sameJSON(old.Options, options) {
			return nil, d.ErrConflict
		}
		stage = *old
	}
	if stage.Status == d.StatusQueued {
		if e := s.enqueue(ctx, stage); e != nil {
			return &stage, e
		}
	}
	return &stage, nil
}
func newStage(p d.Principal, run, key, agent string, refs []d.RefID, opts map[string]any, idem, requestID string, max int) d.StageRun {
	now := time.Now().UTC()
	return d.StageRun{ID: uuid.NewString(), RunID: run, TenantID: p.TenantID, UserID: p.UserID, StageKey: key, AgentID: agent, Status: d.StatusQueued, MaxAttempts: max, InputRefs: refs, Options: opts, IdempotencyKey: idem, RequestID: requestID, CreatedAt: now, UpdatedAt: now}
}
func sameJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
func validateOptions(options map[string]any) error {
	body, e := json.Marshal(options)
	if e != nil || len(body) > 64<<10 {
		return d.ErrInvalid
	}
	for k := range options {
		if k == "approved" || k == "is_admin" || k == "tenant_id" || k == "user_id" || k == "tools" || k == "stages" {
			return fmt.Errorf("%w: protected option %s", d.ErrInvalid, k)
		}
	}
	return nil
}
func (s *Service) StartRun(ctx context.Context, p d.Principal, template string, refs []d.RefID, opts map[string]any, key, requestID string) (*d.PipelineRun, error) {
	if e := validateKey(key); e != nil {
		return nil, e
	}
	def, ok := s.templates[template]
	if !ok || !def.Enabled {
		return nil, d.ErrInvalid
	}
	if e := validateOptions(opts); e != nil {
		return nil, e
	}
	if e := s.validateInputs(ctx, p, "data-access", refs); e != nil {
		return nil, e
	}
	if e := validatePipelineOptions(def, opts); e != nil {
		return nil, e
	}
	for _, st := range def.Stages {
		if e := s.preflight(ctx, p, st.AgentID, refs, stageOptions(opts, st.ID)); e != nil {
			return nil, fmt.Errorf("stage %s: %w", st.ID, e)
		}
	}
	if e := s.repo.SaveDefinition(ctx, def); e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	run := &d.PipelineRun{ID: uuid.NewString(), TenantID: p.TenantID, UserID: p.UserID, TemplateID: template, TemplateVersion: def.Version, Status: d.StatusRunning, InputRefs: refs, Options: opts, IdempotencyKey: key, RequestID: requestID, CreatedAt: now, UpdatedAt: now}
	stages := []d.StageRun{}
	for _, v := range def.Stages {
		st := newStage(p, run.ID, v.ID, v.AgentID, refs, stageOptions(opts, v.ID), run.ID+":"+v.ID, requestID, v.MaxAttempts)
		st.ApprovalRequired = v.Approval
		if len(v.DependsOn) > 0 {
			st.Status = d.StatusWaiting
			st.InputRefs = nil
		}
		stages = append(stages, st)
	}
	if e := s.repo.CreateRun(ctx, run, stages); e != nil {
		old, findErr := s.repo.FindRunByKey(ctx, p.TenantID, key)
		if findErr != nil {
			return nil, e
		}
		if old.TemplateID != template || !sameJSON(old.InputRefs, refs) || !sameJSON(old.Options, opts) {
			return nil, d.ErrConflict
		}
		run = old
		stages, e = s.repo.ListStages(ctx, p.TenantID, run.ID)
		if e != nil {
			return nil, e
		}
	}
	for _, st := range stages {
		if st.Status == d.StatusQueued {
			if e := s.enqueue(ctx, st); e != nil {
				return run, e
			}
		}
	}
	return run, nil
}
func stageOptions(opts map[string]any, key string) map[string]any {
	if stages, ok := opts["stage_options"].(map[string]any); ok {
		if v, ok := stages[key].(map[string]any); ok {
			return v
		}
	}
	return map[string]any{}
}

// DerivedRefID identifies an immutable output for retry recovery. Reading that
// output must still go through Resolve to recheck the caller's current access.
func DerivedRefID(tenant int64, kind d.Kind, producer string, parents []d.RefID) d.RefID {
	return derivedRefID(tenant, kind, producer, parents)
}
func derivedRefID(tenant int64, kind d.Kind, producer string, parents []d.RefID) d.RefID {
	parentKey, _ := json.Marshal(parents)
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%d/%s/%s/%s", tenant, producer, kind, parentKey))).String()
	ref := d.RefID{AssetID: id, Version: 1}
	for _, parent := range parents {
		if parent.Version >= ref.Version {
			ref.Version = parent.Version + 1
		}
	}
	return ref
}
