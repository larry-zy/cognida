package governance

import (
	d "cognida/internal/model/governance"
	"cognida/internal/model/job"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type jobScope struct {
	TenantID int64  `json:"tenant_id"`
	UserID   int64  `json:"user_id"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
}

func (s *Service) PendingJobs(ctx context.Context, limit int) ([]job.Job, error) {
	rows, e := s.Repo.Pending(ctx, limit)
	if e != nil {
		return nil, e
	}
	jobs := []job.Job{}
	for _, r := range rows {
		j, e := job.New(r.ID, "governance", jobScope{r.TenantID, r.UserID, r.Kind, r.ID})
		if e != nil {
			return nil, e
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}
func (s *Service) Handle(ctx context.Context, j job.Job) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var scope jobScope
	if err := json.Unmarshal(j.Payload, &scope); err != nil {
		return err
	}
	if scope.Kind != "subscription_run" && scope.Kind != "tagging_task" && scope.Kind != d.GovernedKind {
		return d.ErrInvalid
	}
	p := l.Principal{TenantID: scope.TenantID, UserID: scope.UserID}
	if p.Validate() != nil {
		return d.ErrForbidden
	}
	var record *d.Record
	var err error
	if scope.Kind == d.GovernedKind {
		// Recovery must still reconcile committed receipts after source access
		// is revoked. New work validates full input authorization in runGoverned.
		record, err = s.Repo.Get(ctx, p.TenantID, scope.Kind, scope.ID)
		if err == nil && record.UserID != p.UserID {
			return d.ErrForbidden
		}
	} else {
		record, err = s.Get(ctx, p, scope.Kind, scope.ID)
	}
	if err != nil {
		return err
	}
	if record.Status == "completed" || record.Status == "failed" || record.Status == "cancelled" {
		return nil
	}
	if scope.Kind == d.GovernedKind {
		return s.runGoverned(ctx, record)
	}
	if scope.Kind == "subscription_run" {
		return s.runReport(ctx, record)
	}
	return s.runTagging(ctx, record)
}
func (s *Service) Exhausted(ctx context.Context, j job.Job, cause error) error {
	var scope jobScope
	if err := json.Unmarshal(j.Payload, &scope); err != nil {
		return err
	}
	record, err := s.Repo.Get(ctx, scope.TenantID, scope.Kind, scope.ID)
	if err != nil {
		return err
	}
	if record.UserID != scope.UserID || scope.UserID <= 0 {
		return d.ErrForbidden
	}
	if scope.Kind == d.GovernedKind {
		var state d.GovernedExecution
		if json.Unmarshal(record.Payload, &state) != nil {
			return d.ErrInvalid
		}
		if state.Invocation.Profile == d.SFTProfile {
			return s.failGovernedSFT(ctx, record, "candidate_generation_failed")
		}
		if state.Invocation.Profile == d.FeatureProfile {
			if jobs, err := s.featureJobs(); err == nil {
				j, err := jobs.FindFeatureJobByKey(ctx, scope.TenantID, scope.UserID, "governed-job:"+record.ID)
				if err != nil {
					return err
				}
				if j != nil {
					return s.finishGovernedFeatureJob(ctx, record, j, cause)
				}
			}
			return s.finishGovernedFeature(ctx, record, state, nil, cause)
		}
		r, err := s.governedRepo()
		if err != nil {
			return err
		}
		_, err = r.UpdateGoverned(ctx, l.Principal{TenantID: scope.TenantID, UserID: scope.UserID}, scope.ID, func(op *d.Record, st *d.GovernedExecution, repo d.Repository) error {
			if op.Status == "completed" || op.Status == "cancelled" || op.Status == "failed" {
				return nil
			}
			task, err := repo.Get(ctx, scope.TenantID, st.DomainKind, st.DomainID)
			if err != nil {
				return err
			}
			task.Status = "failed"
			if err = repo.Update(ctx, task, task.Version); err != nil {
				return err
			}
			op.Status, st.Error = "failed", "candidate_generation_failed"
			return nil
		})
		return err
	}
	if record.Status == "completed" || record.Status == "cancelled" {
		return nil
	}
	var value map[string]any
	if err = json.Unmarshal(record.Payload, &value); err != nil {
		return err
	}
	value["error"] = "执行失败，请检查资产可用性和处理服务"
	value["status"] = "failed"
	return s.update(ctx, record, value, "failed", record.Version)
}

type Worker struct {
	service    *Service
	worker     job.Runner
	dispatcher job.Recoverer
	mu         sync.Mutex
	cancel     context.CancelFunc
	done       chan struct{}
}

func NewWorker(service *Service, runner job.Runner, recovery job.Recoverer) *Worker {
	return &Worker{service: service, dispatcher: recovery, worker: runner}
}
func (w *Worker) Run() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		return nil
	}
	if err := w.worker.Run(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.done = make(chan struct{})
	go func() {
		defer close(w.done)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			if err := w.service.Tick(ctx); err != nil && ctx.Err() == nil {
				fmt.Printf("[Governance] schedule dispatch failed: %v\n", err)
			}
			if _, err := w.dispatcher.Recover(ctx, 100); err != nil && ctx.Err() == nil {
				fmt.Printf("[Governance] recovery failed: %v\n", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
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
	<-w.done
	w.worker.Stop()
	w.cancel = nil
}
