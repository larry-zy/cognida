package warehouse

import (
	l "cognida/internal/model/lifecycle"
	"context"
	"sync"
	"time"
)

type Worker struct {
	s      *Service
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func NewWorker(s *Service) *Worker { return &Worker{s: s} }
func (w *Worker) Run(ctx context.Context) {
	w.mu.Lock()
	if w.done != nil {
		w.mu.Unlock()
		return
	}
	ctx, w.cancel = context.WithCancel(ctx)
	w.done = make(chan struct{})
	done := w.done
	w.mu.Unlock()
	defer close(done)
	tick := time.NewTicker(4 * time.Second)
	defer tick.Stop()
	for {
		rows, e := w.s.Repo.Pending(ctx)
		if e == nil {
			for _, r := range rows {
				if ctx.Err() != nil {
					return
				}
				_ = w.s.Process(ctx, l.Principal{TenantID: r.TenantID, UserID: r.UserID}, r.ID)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (w *Worker) Stop() {
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
