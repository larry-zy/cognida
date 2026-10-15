package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"cognida/internal/infrastructure/job"
	d "cognida/internal/model/lifecycle"
	"github.com/google/uuid"
)

const JobType = "lifecycle.stage"

type delivery struct {
	TenantID int64  `json:"tenant_id"`
	StageID  string `json:"stage_id"`
}

func stageJob(st d.StageRun) (job.Job, error) {
	return job.New("lifecycle:"+st.ID, JobType, delivery{st.TenantID, st.ID})
}
func (s *Service) enqueue(ctx context.Context, st d.StageRun) error {
	j, e := stageJob(st)
	if e != nil {
		return e
	}
	return s.queue.Enqueue(ctx, j)
}

type Worker struct {
	base    *job.Worker
	service *Service
	cancel  context.CancelFunc
	mu      sync.Mutex
	wg      sync.WaitGroup
}

func NewWorker(s *Service) *Worker {
	dispatcher := job.NewDispatcher(s.queue, job.RetryPolicy{MaxAttempts: 5, BaseDelay: time.Second, MaxDelay: 30 * time.Second})
	dispatcher.Register(JobType, s, s)
	return &Worker{base: job.NewWorker(dispatcher, 2, time.Second), service: s}
}
func (w *Worker) Run() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	if e := w.base.Run(); e != nil {
		cancel()
		w.cancel = nil
		return e
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				jobs, e := w.service.PendingJobs(ctx, 1000)
				if e == nil {
					for _, j := range jobs {
						if e = w.service.queue.Enqueue(ctx, j); e != nil {
							break
						}
					}
				}
			}
		}
	}()
	return nil
}
func (w *Worker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel == nil {
		return
	}
	w.cancel()
	w.base.Stop()
	w.wg.Wait()
	w.cancel = nil
}
func (w *Worker) IsRunning() bool { return w.base.IsRunning() }
func (s *Service) PendingJobs(ctx context.Context, n int) ([]job.Job, error) {
	if e := s.recoverRuns(ctx, n); e != nil {
		return nil, e
	}
	stages, e := s.repo.PendingStages(ctx, n)
	if e != nil {
		return nil, e
	}
	out := []job.Job{}
	for _, st := range stages {
		j, e := stageJob(st)
		if e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, nil
}
func (s *Service) event(ctx context.Context, repo d.Repository, st *d.StageRun, typ, detail string) error {
	return repo.AppendEvent(ctx, &d.Event{TenantID: st.TenantID, RunID: st.RunID, StageID: st.ID, Type: typ, Detail: detail, RequestID: st.RequestID, CreatedAt: time.Now().UTC()})
}
func (s *Service) handle(ctx context.Context, j job.Job) error {
	var msg delivery
	if e := json.Unmarshal(j.Payload, &msg); e != nil {
		return e
	}
	var stage *d.StageRun
	execute := false
	err := s.repo.Transaction(ctx, func(repo d.Repository) error {
		st, e := repo.FindStage(ctx, msg.TenantID, msg.StageID, true)
		if e != nil {
			return e
		}
		stage = st
		if d.Terminal(st.Status) || st.Status == d.StatusApproval || st.Status == d.StatusWaiting {
			return nil
		}
		if st.Attempt >= st.MaxAttempts {
			st.Status = d.StatusFailed
			st.Error = "stage retry budget exhausted"
			st.UpdatedAt = time.Now().UTC()
			return repo.UpdateStage(ctx, st)
		}
		// The job receipt grants a lease. A new delivery advances the attempt fence,
		// so a worker whose lease expired can no longer commit its result.
		st.Attempt++
		st.Status = d.StatusRunning
		st.Error = ""
		st.UpdatedAt = time.Now().UTC()
		execute = true
		if e = repo.UpdateStage(ctx, st); e != nil {
			return e
		}
		return s.event(ctx, repo, st, "stage.started", fmt.Sprintf("attempt=%d", st.Attempt))
	})
	if err != nil {
		return err
	}
	if !execute {
		return s.reconcile(ctx, stage.TenantID, stage.RunID)
	}
	workCtx, cancel, governed, contextErr := s.governedStageContext(ctx, stage)
	if contextErr != nil {
		return s.finish(ctx, stage, d.Output{}, contextErr)
	}
	defer cancel()
	if governed {
		if err := s.ValidateGovernedInputs(workCtx, d.Principal{TenantID: stage.TenantID, UserID: stage.UserID}, stage.InputRefs); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				err = governedTimeoutError()
			}
			return s.finish(ctx, stage, d.Output{}, err)
		}
	}
	request := d.StageRequest{Principal: d.Principal{TenantID: stage.TenantID, UserID: stage.UserID}, Stage: *stage}
	for _, id := range stage.InputRefs {
		ref, body, e := s.Resolve(workCtx, stage.TenantID, stage.UserID, id)
		if e != nil {
			return s.finish(ctx, stage, d.Output{}, e)
		}
		request.Inputs = append(request.Inputs, ref)
		request.Payloads = append(request.Payloads, body)
	}
	if s.requiresApproval(request) && !stage.Approved {
		return s.pause(ctx, stage)
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-workCtx.Done():
				return
			case <-ticker.C:
				current, e := s.repo.FindStage(workCtx, stage.TenantID, stage.ID, false)
				if e == nil && (current.Status != d.StatusRunning || current.Attempt != stage.Attempt) {
					cancel()
					return
				}
			}
		}
	}()
	var out d.Output
	var runErr error
	if governed {
		out, runErr = s.executeAdapter(workCtx, request)
	} else {
		out, runErr = s.adapters[stage.AgentID].Run(workCtx, request)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(workCtx.Err(), context.DeadlineExceeded) {
		// Use the still-live delivery context to persist the failure and apply
		// the existing retry budget. A local deadline must not leave the stage
		// running as lease loss or explicit cancellation would.
		if governed {
			return s.finish(ctx, stage, d.Output{}, governedTimeoutError())
		}
		return s.finish(ctx, stage, d.Output{}, fmt.Errorf("stage execution exceeded %s: %w", s.config.StageTimeout, context.DeadlineExceeded))
	}
	if workCtx.Err() != nil {
		return s.reconcile(ctx, stage.TenantID, stage.RunID)
	}
	return s.finish(ctx, stage, out, runErr)
}
func (s *Service) requiresApproval(r d.StageRequest) bool {
	if r.Stage.ApprovalRequired {
		return true
	}
	if r.Stage.AgentID == "data-asset" {
		return true
	} // Publication is a durable shared write.
	if method, _ := r.Stage.Options["method"].(string); method == "llm" {
		return true
	}
	if high, _ := r.Stage.Options["high_cost"].(bool); high {
		return true
	}
	return false
}
func (s *Service) pause(ctx context.Context, st *d.StageRun) error {
	e := s.repo.Transaction(ctx, func(repo d.Repository) error {
		cur, e := repo.FindStage(ctx, st.TenantID, st.ID, true)
		if e != nil {
			return e
		}
		if cur.Status != d.StatusRunning || cur.Attempt != st.Attempt {
			return nil
		}
		now := time.Now().UTC()
		approval := &d.Approval{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("approval:"+st.ID)).String(), TenantID: st.TenantID, RunID: st.RunID, StageID: st.ID, Status: "pending", Reason: "administrator approval required for publication or model cost", ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now}
		if e = repo.CreateApproval(ctx, approval); e != nil {
			return e
		}
		cur.Status = d.StatusApproval
		cur.Attempt--
		cur.UpdatedAt = now
		if e = repo.UpdateStage(ctx, cur); e != nil {
			return e
		}
		return s.event(ctx, repo, cur, "approval.requested", approval.ID)
	})
	if e != nil {
		return e
	}
	return s.reconcile(ctx, st.TenantID, st.RunID)
}
func (s *Service) finish(ctx context.Context, st *d.StageRun, out d.Output, runErr error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	retry := false
	e := s.repo.Transaction(ctx, func(repo d.Repository) error {
		cur, e := repo.FindStage(ctx, st.TenantID, st.ID, true)
		if e != nil {
			return e
		}
		if cur.Status != d.StatusRunning || cur.Attempt != st.Attempt {
			return nil
		}
		cur.UpdatedAt = time.Now().UTC()
		if runErr != nil {
			cur.Error = runErr.Error()
			if cur.Attempt < cur.MaxAttempts && !errors.Is(runErr, d.ErrInvalid) && !errors.Is(runErr, d.ErrForbidden) && !errors.Is(runErr, d.ErrExpired) && !errors.Is(runErr, d.ErrNotFound) {
				cur.Status = d.StatusQueued
				retry = true
			} else {
				cur.Status = d.StatusFailed
			}
		} else {
			if out.Gate.Status == "" {
				return fmt.Errorf("adapter returned no gate")
			}
			for _, ref := range out.Refs {
				canonical, e := repo.FindAsset(ctx, cur.TenantID, ref.RefID)
				if e != nil {
					return e
				}
				if canonical.Kind != ref.Kind {
					return d.ErrInvalid
				}
			}
			cur.Output = &out
			cur.Status = d.StatusSucceeded
			if out.Gate.Status == d.GateQuarantined || out.Gate.Status == d.GateApprovalRequired {
				cur.Status = d.StatusQuarantined
			}
		}
		if e = repo.UpdateStage(ctx, cur); e != nil {
			return e
		}
		return s.event(ctx, repo, cur, "stage."+cur.Status, cur.Error)
	})
	if e != nil {
		return e
	}
	if e = s.reconcile(ctx, st.TenantID, st.RunID); e != nil {
		return e
	}
	if retry {
		return runErr
	}
	return nil
}
func (s *Service) Exhausted(ctx context.Context, j job.Job, cause error) error {
	var msg delivery
	if e := json.Unmarshal(j.Payload, &msg); e != nil {
		return e
	}
	st, e := s.repo.FindStage(ctx, msg.TenantID, msg.StageID, false)
	if e != nil {
		return e
	}
	if !d.Terminal(st.Status) && st.Status != d.StatusApproval {
		e = s.repo.Transaction(ctx, func(repo d.Repository) error {
			cur, e := repo.FindStage(ctx, msg.TenantID, msg.StageID, true)
			if e != nil {
				return e
			}
			if d.Terminal(cur.Status) || cur.Status == d.StatusApproval {
				return nil
			}
			cur.Status = d.StatusFailed
			cur.Error = cause.Error()
			cur.UpdatedAt = time.Now().UTC()
			return repo.UpdateStage(ctx, cur)
		})
	}
	if e != nil {
		return e
	}
	return s.reconcile(ctx, st.TenantID, st.RunID)
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func (s *Service) reconcile(ctx context.Context, tenant int64, id string) error {
	if id == "" {
		return nil
	}
	ready := []d.StageRun{}
	e := s.repo.Transaction(ctx, func(repo d.Repository) error {
		run, e := repo.FindRun(ctx, tenant, id, true)
		if e != nil {
			return e
		}
		if d.Terminal(run.Status) {
			return nil
		}
		// Runs bind to the immutable persisted definition from their creation,
		// including across deployments that register a newer template version.
		def, e := repo.FindDefinition(ctx, run.TemplateID, run.TemplateVersion)
		if errors.Is(e, d.ErrNotFound) || errors.Is(e, d.ErrConflict) || errors.Is(e, d.ErrInvalid) {
			return s.quarantineDefinition(ctx, repo, run, "persisted pipeline definition is missing or invalid")
		}
		if e != nil {
			return e
		}
		if e = def.Validate(s.contracts); e != nil {
			return s.quarantineDefinition(ctx, repo, run, "persisted pipeline definition is incompatible with registered agent contracts")
		}
		stages, e := repo.LockStages(ctx, tenant, id)
		if e != nil {
			return e
		}
		byKey := map[string]*d.StageRun{}
		for i := range stages {
			byKey[stages[i].StageKey] = &stages[i]
		}
		for _, sd := range def.Stages {
			st := byKey[sd.ID]
			if st == nil {
				return d.ErrConflict
			}
			if st.Status == d.StatusQueued {
				ready = append(ready, *st)
				continue
			}
			if st.Status != d.StatusWaiting {
				continue
			}
			all := true
			blocked := false
			refs := []d.RefID{}
			seen := map[string]bool{}
			for _, parent := range sd.DependsOn {
				prev := byKey[parent]
				if !d.Terminal(prev.Status) {
					all = false
					continue
				}
				if prev.Status != d.StatusSucceeded || prev.Output == nil || !contains(sd.AllowedGates, prev.Output.Gate.Status) {
					blocked = true
					continue
				}
				for _, ref := range prev.Output.Refs {
					if s.contracts[st.AgentID].Accepts(ref.Kind) && !seen[ref.AssetRefID] {
						refs = append(refs, ref.RefID)
						seen[ref.AssetRefID] = true
					}
				}
			}
			if blocked {
				st.Status = d.StatusQuarantined
				st.Error = "upstream stage or quality gate did not permit this transition"
			} else if all {
				if len(refs) == 0 {
					st.Status = d.StatusQuarantined
					st.Error = "upstream produced no compatible reference"
				} else {
					st.Status = d.StatusQueued
					st.InputRefs = refs
					ready = append(ready, *st)
				}
			} else {
				continue
			}
			st.UpdatedAt = time.Now().UTC()
			if e = repo.UpdateStage(ctx, st); e != nil {
				return e
			}
			if e = s.event(ctx, repo, st, "stage."+st.Status, st.Error); e != nil {
				return e
			}
		}
		allDone := true
		failed := false
		quarantined := false
		approval := false
		for _, st := range stages {
			if !d.Terminal(st.Status) {
				allDone = false
			}
			failed = failed || st.Status == d.StatusFailed
			quarantined = quarantined || st.Status == d.StatusQuarantined
			approval = approval || st.Status == d.StatusApproval
		}
		run.Status = d.StatusRunning
		if approval {
			run.Status = d.StatusApproval
		}
		if allDone {
			run.Status = d.StatusSucceeded
			if quarantined {
				run.Status = d.StatusQuarantined
			}
			if failed {
				run.Status = d.StatusFailed
			}
		}
		run.UpdatedAt = time.Now().UTC()
		return repo.UpdateRun(ctx, run)
	})
	if e != nil {
		return e
	}
	for _, st := range ready {
		if e = s.enqueue(ctx, st); e != nil {
			return e
		}
	}
	return nil
}

// A permanently unusable template quarantines only its own run. Retrying it
// forever from PendingJobs would prevent healthy runs from being recovered.
// Transient repository errors are returned by the caller and never quarantined.
func (s *Service) quarantineDefinition(ctx context.Context, repo d.Repository, run *d.PipelineRun, reason string) error {
	stages, err := repo.LockStages(ctx, run.TenantID, run.ID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for i := range stages {
		st := &stages[i]
		if d.Terminal(st.Status) {
			continue
		}
		st.Status = d.StatusQuarantined
		st.Error = reason
		st.UpdatedAt = now
		if err = repo.UpdateStage(ctx, st); err != nil {
			return err
		}
		if err = s.event(ctx, repo, st, "stage.quarantined", reason); err != nil {
			return err
		}
	}
	run.Status = d.StatusQuarantined
	run.Error = reason
	run.UpdatedAt = now
	return repo.UpdateRun(ctx, run)
}
func (s *Service) CancelStage(ctx context.Context, p d.Principal, id string) error {
	if e := s.authorize(ctx, p, "read"); e != nil {
		return e
	}
	var runID string
	e := s.repo.Transaction(ctx, func(repo d.Repository) error {
		st, e := repo.FindStage(ctx, p.TenantID, id, true)
		if e != nil {
			return e
		}
		runID = st.RunID
		if d.Terminal(st.Status) {
			return nil
		}
		st.Status = d.StatusCancelled
		st.UpdatedAt = time.Now().UTC()
		if e = repo.UpdateStage(ctx, st); e != nil {
			return e
		}
		return s.event(ctx, repo, st, "stage.cancelled", "")
	})
	if e != nil {
		return e
	}
	return s.reconcile(ctx, p.TenantID, runID)
}
func (s *Service) CancelRun(ctx context.Context, p d.Principal, id string) error {
	if e := s.authorize(ctx, p, "read"); e != nil {
		return e
	}
	return s.repo.Transaction(ctx, func(repo d.Repository) error {
		run, e := repo.FindRun(ctx, p.TenantID, id, true)
		if e != nil {
			return e
		}
		if d.Terminal(run.Status) {
			return nil
		}
		stages, e := repo.LockStages(ctx, p.TenantID, id)
		if e != nil {
			return e
		}
		for i := range stages {
			st := &stages[i]
			if !d.Terminal(st.Status) {
				st.Status = d.StatusCancelled
				st.UpdatedAt = time.Now().UTC()
				if e = repo.UpdateStage(ctx, st); e != nil {
					return e
				}
				if e = s.event(ctx, repo, st, "stage.cancelled", ""); e != nil {
					return e
				}
			}
		}
		run.Status = d.StatusCancelled
		run.UpdatedAt = time.Now().UTC()
		return repo.UpdateRun(ctx, run)
	})
}
func (s *Service) resolveApproval(ctx context.Context, p d.Principal, id, decision, reason string) error {
	if e := s.authorize(ctx, p, "admin"); e != nil {
		return e
	}
	if decision != "approved" && decision != "rejected" {
		return d.ErrInvalid
	}
	var stage *d.StageRun
	expired := false
	e := s.repo.Transaction(ctx, func(repo d.Repository) error {
		a, e := repo.FindApproval(ctx, p.TenantID, id, true)
		if e != nil {
			return e
		}
		if a.Status != "pending" {
			return d.ErrConflict
		}
		st, e := repo.FindStage(ctx, p.TenantID, a.StageID, true)
		if e != nil {
			return e
		}
		stage = st
		if st.Status != d.StatusApproval {
			return d.ErrConflict
		}
		now := time.Now().UTC()
		if !a.ExpiresAt.After(now) {
			decision = "expired"
			expired = true
		}
		a.Status = decision
		a.Reason = reason
		a.ResolvedBy = p.UserID
		a.ResolvedAt = &now
		st.UpdatedAt = now
		if decision == "approved" {
			st.Status = d.StatusQueued
			st.Approved = true
		} else {
			st.Status = d.StatusQuarantined
			st.Error = "approval " + decision
		}
		if e = repo.UpdateApproval(ctx, a); e != nil {
			return e
		}
		if e = repo.UpdateStage(ctx, st); e != nil {
			return e
		}
		return s.event(ctx, repo, st, "approval."+decision, reason)
	})
	if e != nil {
		return e
	}
	if e = s.reconcile(ctx, p.TenantID, stage.RunID); e != nil {
		return e
	}
	if stage.Status == d.StatusQueued {
		if e = s.enqueue(ctx, *stage); e != nil {
			return e
		}
	}
	if expired {
		return d.ErrExpired
	}
	return nil
}
func (s *Service) RetryRun(ctx context.Context, p d.Principal, id string) error {
	if e := s.authorize(ctx, p, "read"); e != nil {
		return e
	}
	e := s.repo.Transaction(ctx, func(repo d.Repository) error {
		run, e := repo.FindRun(ctx, p.TenantID, id, true)
		if e != nil {
			return e
		}
		if run.Status != d.StatusFailed {
			return d.ErrConflict
		}
		stages, e := repo.LockStages(ctx, p.TenantID, id)
		if e != nil {
			return e
		}
		for i := range stages {
			st := &stages[i]
			if st.Status == d.StatusFailed {
				st.Status = d.StatusQueued
				st.Attempt = 0
				st.Error = ""
			} else if st.Status == d.StatusQuarantined {
				st.Status = d.StatusWaiting
				st.Error = ""
			} else {
				continue
			}
			st.UpdatedAt = time.Now().UTC()
			if e = repo.UpdateStage(ctx, st); e != nil {
				return e
			}
		}
		run.Status = d.StatusRunning
		run.Error = ""
		run.UpdatedAt = time.Now().UTC()
		return repo.UpdateRun(ctx, run)
	})
	if e != nil {
		return e
	}
	return s.reconcile(ctx, p.TenantID, id)
}

var _ job.Handler = (*Service)(nil)

func (s *Service) recoverRuns(ctx context.Context, n int) error {
	approvals, e := s.repo.ExpiredApprovals(ctx, time.Now().UTC(), n)
	if e != nil {
		return e
	}
	for _, a := range approvals {
		e = s.repo.Transaction(ctx, func(repo d.Repository) error {
			current, e := repo.FindApproval(ctx, a.TenantID, a.ID, true)
			if e != nil {
				return e
			}
			if current.Status != "pending" {
				return nil
			}
			st, e := repo.FindStage(ctx, a.TenantID, a.StageID, true)
			if e != nil {
				return e
			}
			now := time.Now().UTC()
			current.Status = "expired"
			current.ResolvedAt = &now
			if e = repo.UpdateApproval(ctx, current); e != nil {
				return e
			}
			if st.Status == d.StatusApproval {
				st.Status = d.StatusQuarantined
				st.Error = "approval expired"
				st.UpdatedAt = now
				if e = repo.UpdateStage(ctx, st); e != nil {
					return e
				}
			}
			return s.event(ctx, repo, st, "approval.expired", a.ID)
		})
		if e != nil {
			return e
		}
	}
	runs, e := s.repo.RecoverableRuns(ctx, n)
	if e != nil {
		return e
	}
	for _, run := range runs {
		if e = s.reconcile(ctx, run.TenantID, run.ID); e != nil {
			return e
		}
	}
	return nil
}
