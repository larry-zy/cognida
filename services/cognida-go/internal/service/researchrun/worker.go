package researchrun

import (
	"context"
	"log"
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
func (w *Worker) Run() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.done = make(chan struct{})
	go func() {
		defer close(w.done)
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			if err := w.s.Tick(ctx); err != nil && ctx.Err() == nil {
				log.Print("[Research] durable execution reconciliation incomplete")
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return nil
}
func (w *Worker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		<-w.done
		w.cancel = nil
	}
}
