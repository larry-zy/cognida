package governance

import (
	"cognida/internal/infrastructure/job"
	d "cognida/internal/model/governance"
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
	if scope.Kind != "subscription_run" && scope.Kind != "tagging_task" {
		return d.ErrInvalid
	}
	record, err := s.Get(ctx, l.Principal{TenantID: scope.TenantID, UserID: scope.UserID}, scope.Kind, scope.ID)
	if err != nil {
		return err
	}
	if record.Status == "completed" || record.Status == "failed" {
		return nil
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
	if record.Status == "completed" {
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
	worker     *job.Worker
	dispatcher *job.Dispatcher
	mu         sync.Mutex
	cancel     context.CancelFunc
	done       chan struct{}
}

func NewWorker(service *Service) *Worker {
	d := job.NewDispatcher(service.Queue, job.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: time.Minute})
	d.Register("governance", service, service)
	return &Worker{service: service, dispatcher: d, worker: job.NewWorker(d, 2, time.Second)}
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
