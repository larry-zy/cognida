package taskcontrol

import (
	"context"
	"log"
	"sync"
	"time"
)

// RuntimeWorker owns polling only. Durable leases and domain operation keys
// remain authoritative when this process stops or another worker takes over.
type RuntimeWorker struct {
	runtime *Runtime
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
}

func NewRuntimeWorker(runtime *Runtime) *RuntimeWorker { return &RuntimeWorker{runtime: runtime} }
func (w *RuntimeWorker) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.done = make(chan struct{})
	go func() {
		defer close(w.done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			if err := w.runtime.Tick(ctx); err != nil && ctx.Err() == nil {
				// Domain errors can include private input details; keep worker logs generic.
				log.Print("[TaskControl] reconciliation incomplete; durable records retained for retry")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
func (w *RuntimeWorker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		<-w.done
		w.cancel = nil
	}
}
