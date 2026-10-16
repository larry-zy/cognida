// Package researchrun persists bounded research operations independently of HTTP.
package researchrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	agentctx "cognida/internal/model/agent"
	l "cognida/internal/model/lifecycle"
	r "cognida/internal/model/researchrun"
	d "cognida/internal/model/taskcontrol"
	agent "cognida/internal/service/agent"
	"github.com/google/uuid"
)

type Engine interface {
	ExecuteGoverned(context.Context, *agent.DeepResearchRequest) (*agent.DeepResearchResponse, error)
	Available() bool
}

// Authorizer checks the entire explicit KB range and, for a stored report, every
// cited chunk/document. No filtering or silent scope widening is permitted.
type Authorizer func(context.Context, d.Scope, []string, *agent.DeepResearchResponse) error
type OutputStore func(context.Context, d.Scope, string, *agent.DeepResearchResponse) (l.RefID, error)
type Service struct {
	repo      r.Repository
	authorize Authorizer
	output    OutputStore
	mu        sync.RWMutex
	engine    Engine
	active    map[string]context.CancelFunc
}

func New(repo r.Repository, authorize Authorizer, output OutputStore) (*Service, error) {
	if repo == nil || authorize == nil || output == nil {
		return nil, d.ErrInvalid
	}
	return &Service{repo: repo, authorize: authorize, output: output, active: map[string]context.CancelFunc{}}, nil
}
func (s *Service) SetEngine(engine Engine) { s.mu.Lock(); defer s.mu.Unlock(); s.engine = engine }
func (s *Service) Available() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.engine != nil && s.engine.Available()
}
func (s *Service) Validate(ctx context.Context, p d.Scope, req r.Request) error {
	if p.Validate() != nil {
		return d.ErrForbidden
	}
	if err := req.Validate(); err != nil {
		return err
	}
	if !s.Available() {
		return fmt.Errorf("research engine unavailable: %w", d.ErrInvalid)
	}
	return s.authorize(ctx, p, req.KBIDs, nil)
}
func (s *Service) Submit(ctx context.Context, p d.Scope, in r.Submission) (*r.Run, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if err := s.Validate(ctx, p, in.Request); err != nil {
		return nil, err
	}
	return s.repo.Submit(ctx, p, in, false)
}
func (s *Service) Lookup(ctx context.Context, p d.Scope, in r.Submission) (*r.Run, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	row, err := s.repo.Lookup(ctx, p, in.OperationKey, in.PayloadHash)
	if err != nil || row == nil {
		return row, err
	}
	raw, _ := json.Marshal(in)
	actual, e := d.CanonicalJSON(row.Submission, '{', 64<<10)
	expected, f := d.CanonicalJSON(raw, '{', 64<<10)
	if e != nil || f != nil || string(actual) != string(expected) {
		return nil, d.ErrConflict
	}
	return row, nil
}
func (s *Service) CancelSubmission(ctx context.Context, p d.Scope, in r.Submission) (*r.Run, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	row, err := s.repo.Submit(ctx, p, in, true)
	if err == nil {
		s.stopActive(p, row.ID)
	}
	return row, err
}
func activeKey(p d.Scope, id string) string { return fmt.Sprintf("%d/%d/%s", p.TenantID, p.UserID, id) }
func (s *Service) stopActive(p d.Scope, id string) {
	s.mu.RLock()
	cancel := s.active[activeKey(p, id)]
	s.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
}
func (s *Service) Cancel(ctx context.Context, p d.Scope, id string) (*r.Run, error) {
	row, err := s.repo.Cancel(ctx, p, id)
	if err == nil {
		s.stopActive(p, id)
	}
	return row, err
}
func submission(row *r.Run) (r.Submission, error) {
	var in r.Submission
	err := json.Unmarshal(row.Submission, &in)
	if err == nil {
		err = in.Validate()
	}
	return in, err
}

// Inspect exposes metadata only. Read is mandatory before returning report bytes.
func (s *Service) Inspect(ctx context.Context, p d.Scope, id string) (*r.Run, error) {
	row, err := s.repo.Get(ctx, p, id)
	if err != nil {
		return nil, err
	}
	row.Report = nil
	return row, nil
}
func (s *Service) ValidateInputs(ctx context.Context, p d.Scope, refs []l.RefID) error {
	if p.Validate() != nil || len(refs) > 64 {
		return d.ErrInvalid
	}
	for _, ref := range refs {
		row, err := s.repo.FindOutput(ctx, p, ref)
		if err != nil {
			return err
		}
		if _, err = s.Read(ctx, p, row.ID); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) Read(ctx context.Context, p d.Scope, id string) (*r.Run, error) {
	row, err := s.repo.Get(ctx, p, id)
	if err != nil {
		return nil, err
	}
	in, err := submission(row)
	if err != nil {
		return nil, err
	}
	var report *agent.DeepResearchResponse
	if len(row.Report) > 0 {
		report = &agent.DeepResearchResponse{}
		if json.Unmarshal(row.Report, report) != nil {
			return nil, d.ErrConflict
		}
	}
	if err = s.authorize(ctx, p, in.Request.KBIDs, report); err != nil {
		return nil, err
	}
	return row, nil
}

// ValidateGovernedOrigin never resolves its own output recursively. It checks
// the persisted run, current sources, output binding and exact report digest.
func (s *Service) ValidateGovernedOrigin(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
	const prefix = "research-governed:"
	if !strings.HasPrefix(ref.ProducerID, prefix) || ref.Kind != l.Insight {
		return d.ErrForbidden
	}
	row, err := s.Read(ctx, d.Scope{TenantID: p.TenantID, UserID: p.UserID}, strings.TrimPrefix(ref.ProducerID, prefix))
	if err != nil {
		return err
	}
	if row.Status != "succeeded" && row.Status != "partial" {
		return d.ErrForbidden
	}
	var report agent.DeepResearchResponse
	if json.Unmarshal(row.Report, &report) != nil {
		return d.ErrForbidden
	}
	body, e := json.Marshal(&report)
	if e != nil {
		return e
	}
	var refs []l.RefID
	if json.Unmarshal(row.OutputRefs, &refs) != nil || len(refs) != 1 || refs[0] != ref.RefID || d.Digest(body) != ref.Provenance.Digest {
		return d.ErrForbidden
	}
	return nil
}
func (s *Service) Tick(ctx context.Context) error {
	rows, err := s.repo.Pending(ctx, 16)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err = s.Process(ctx, row.Scope, row.ID); err != nil && !errors.Is(err, d.ErrConflict) {
			return err
		}
	}
	return nil
}

// Process claims once. Expired running claims become failed, never queued.
// An unconfirmed paid generation is not replayed on a restart or lease loss.
func (s *Service) Process(parent context.Context, p d.Scope, id string) error {
	owner := uuid.NewString()
	row, err := s.repo.Claim(parent, p, id, owner, 15*time.Second)
	if err != nil {
		return err
	}
	if row.Status != "running" {
		return nil
	}
	in, err := submission(row)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(time.Duration(in.TimeoutSeconds) * time.Second)
	if in.Deadline.Before(deadline) {
		deadline = in.Deadline
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	defer cancel()
	key := activeKey(p, id)
	s.mu.Lock()
	s.active[key] = cancel
	engine := s.engine
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.active, key); s.mu.Unlock() }()
	done := make(chan struct{})
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				updated, e := s.repo.Renew(ctx, p, id, owner, 15*time.Second)
				if e != nil || updated.Status == "cancel_requested" {
					cancel()
					return
				}
			}
		}
	}()
	finish := func(out r.Completion) error {
		close(done)
		<-heartbeatDone
		finishCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_, e := s.repo.Complete(finishCtx, p, id, owner, out)
		return e
	}
	fail := func(message string) error { return finish(r.Completion{Status: "failed", Error: message}) }
	if err = s.authorize(ctx, p, in.Request.KBIDs, nil); err != nil {
		return fail("research source authorization failed")
	}
	if engine == nil || !engine.Available() {
		return fail("research engine unavailable")
	}
	agentContext := agentctx.NewAgentContext(ctx, "", p.TenantID, p.UserID, in.Request.Query)
	requestID := in.RequestID
	if requestID == "" {
		// HTTP-created and older queued runs may not carry a request ID. The
		// durable run identity keeps retrieval strategy pins stable across all
		// subquestions without borrowing a worker/reconnect request's identity.
		requestID = "research:" + row.ID
	}
	agentContext = agentctx.WithRequestID(agentContext, requestID)
	agentContext = agentctx.WithAllowedKBIDs(agentContext, append([]string(nil), in.Request.KBIDs...))
	agentContext = agentctx.WithKBScopeMode(agentContext, "manual")
	agentContext = agentctx.WithGraphEnabled(agentContext, in.Request.GraphEnabled)
	request := &agent.DeepResearchRequest{Query: in.Request.Query, KBIDs: append([]string(nil), in.Request.KBIDs...), KBScopeMode: "manual", GraphEnabled: in.Request.GraphEnabled, Options: &agent.DeepResearchOptions{EnableWebSearch: in.Request.EnableWebSearch}}
	checkpoint := func(event *agent.ResearchProgressDTO) {
		if event == nil || event.Report == nil {
			return
		}
		body, e := json.Marshal(event.Report)
		if e != nil {
			cancel()
			return
		}
		checkpointCtx, c := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer c()
		if _, e = s.repo.Checkpoint(checkpointCtx, p, id, owner, event.Stage, event.Progress, body); e != nil {
			cancel()
		}
	}
	report, runErr := executeSafelyProgress(engine, agentContext, request, checkpoint)
	var raw json.RawMessage
	if report != nil {
		raw, _ = json.Marshal(report)
		if len(raw) > 1<<20 {
			raw = nil
		}
	}
	// Keep collected evidence on model failures/timeouts; never replay paid work.
	usablePartial := agent.UsableResearchPartial(report, runErr)
	if (runErr != nil && !usablePartial) || ctx.Err() != nil {
		status, message := "failed", "research execution incomplete; existing evidence retained; no automatic paid-call replay"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status, message = "timed_out", "research deadline exceeded"
		}
		return finish(r.Completion{Status: status, Report: raw, Error: message})
	}
	if report == nil || (!report.Success && !usablePartial) || !report.Metadata.CitationChecked || len(report.Sources) == 0 {
		return finish(r.Completion{Status: "failed", Report: raw, Error: "research returned no usable citation report"})
	}
	if err = s.authorize(ctx, p, in.Request.KBIDs, report); err != nil {
		return fail("research source authorization changed before completion")
	}
	if len(raw) == 0 {
		return fail("research report exceeds bounded output")
	}
	ref, err := s.output(ctx, p, id, report)
	if err != nil || ref.Validate() != nil {
		return finish(r.Completion{Status: "failed", Report: raw, Error: "research report storage failed; evidence retained"})
	}
	refs, _ := json.Marshal([]l.RefID{ref})
	status := "succeeded"
	if report.Metadata.Partial {
		status = "partial"
	}
	message := ""
	if usablePartial {
		message = "optional supplemental review incomplete; retained first reviewed snapshot only"
	}
	return finish(r.Completion{Status: status, Report: raw, OutputRefs: refs, Error: message})
}
func executeSafely(engine Engine, ctx context.Context, req *agent.DeepResearchRequest) (report *agent.DeepResearchResponse, err error) {
	defer func() {
		if recover() != nil {
			report = nil
			err = fmt.Errorf("research engine failed")
		}
	}()
	return engine.ExecuteGoverned(ctx, req)
}

func executeSafelyProgress(engine Engine, ctx context.Context, req *agent.DeepResearchRequest, progress func(*agent.ResearchProgressDTO)) (report *agent.DeepResearchResponse, err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("research engine failed")
		}
	}()
	if enhanced, ok := engine.(interface {
		ExecuteGovernedWithProgress(context.Context, *agent.DeepResearchRequest, func(*agent.ResearchProgressDTO)) (*agent.DeepResearchResponse, error)
	}); ok {
		return enhanced.ExecuteGovernedWithProgress(ctx, req, progress)
	}
	return engine.ExecuteGoverned(ctx, req)
}

// Create is the HTTP idempotent entry. Reconnects use Read, never Create.
func (s *Service) Create(ctx context.Context, p d.Scope, req r.Request, key string) (*r.Run, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	if len(key) < 1 || len(key) > 128 || strings.TrimSpace(key) != key {
		return nil, d.ErrInvalid
	}
	if err := s.Validate(ctx, p, req); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(req)
	operation := d.Digest([]byte(fmt.Sprintf("http/%d/%d/%s", p.TenantID, p.UserID, key)))
	hash := d.Digest(raw)
	previous, err := s.repo.Lookup(ctx, p, operation, hash)
	if err != nil {
		return nil, err
	}
	if previous != nil {
		return s.Read(ctx, p, previous.ID)
	}
	in := r.Submission{OperationKey: operation, PayloadHash: hash, Profile: r.Profile, Request: req, Deadline: time.Now().UTC().Add(r.MaxTimeoutSeconds * time.Second), TimeoutSeconds: r.MaxTimeoutSeconds, MaxCalls: r.MaxCalls}
	row, err := s.repo.Submit(ctx, p, in, false)
	if errors.Is(err, d.ErrConflict) {
		existing, lookupErr := s.repo.Lookup(ctx, p, operation, hash)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if existing != nil {
			return s.Read(ctx, p, existing.ID)
		}
	}
	return row, err
}
