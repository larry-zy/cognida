package governance

import (
	"context"
	"sync"
	"time"
)

// FeatureOperationsWorker has no schema side effects. Two bounded job slots run
// independently of scheduled comparisons so model latency cannot starve alerts.
// Shutdown waits for cancelled operations; unconfirmed model leases reconcile.
type FeatureOperationsWorker struct {
	service *Service
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	active  map[string]bool
	workers sync.WaitGroup
}

func NewFeatureOperationsWorker(s *Service) *FeatureOperationsWorker {
	return &FeatureOperationsWorker{service: s, active: map[string]bool{}}
}
func (w *FeatureOperationsWorker) Run(ctx context.Context) {
	w.mu.Lock()
	if w.done != nil {
		w.mu.Unlock()
		return
	}
	ctx, w.cancel = context.WithCancel(ctx)
	w.done = make(chan struct{})
	done := w.done
	w.mu.Unlock()
	defer func() { w.workers.Wait(); close(done) }()
	tick := time.NewTicker(4 * time.Second)
	defer tick.Stop()
	for {
		w.step(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (w *FeatureOperationsWorker) Stop() {
	w.mu.Lock()
	cancel, done := w.cancel, w.done
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}
func (w *FeatureOperationsWorker) step(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	if r, e := w.service.driftRepo(); e == nil {
		rows, e := r.ClaimDriftMonitors(ctx, 10)
		if e == nil {
			for _, row := range rows {
				if ctx.Err() != nil {
					return
				}
				_ = w.service.RunDriftMonitor(ctx, row)
			}
		}
	}
	if r, e := w.service.featureJobs(); e == nil {
		jobs, e := r.PendingFeatureJobs(ctx, 20)
		if e != nil {
			return
		}
		for _, j := range jobs {
			if ctx.Err() != nil {
				return
			}
			w.mu.Lock()
			if len(w.active) >= 2 {
				w.mu.Unlock()
				return
			}
			if w.active[j.ID] {
				w.mu.Unlock()
				continue
			}
			w.active[j.ID] = true
			w.workers.Add(1)
			w.mu.Unlock()
			go func() {
				defer w.workers.Done()
				defer func() { w.mu.Lock(); delete(w.active, j.ID); w.mu.Unlock() }()
				_ = w.service.RunFeatureJob(ctx, j.TenantID, j.UserID, j.ID)
			}()
		}
	}
}
