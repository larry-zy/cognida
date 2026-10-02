package job

import (
	"context"
	"log"
	"sync"
	"time"
)

// Worker owns the lifecycle and concurrency of a generic Dispatcher. It has
// no knowledge of domain task state; applications supply handlers instead.
type Worker struct {
	dispatcher  *Dispatcher
	concurrency int
	poll        time.Duration

	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.Mutex
	running bool
}

func NewWorker(dispatcher *Dispatcher, concurrency int, poll time.Duration) *Worker {
	if concurrency <= 0 {
		concurrency = 1
	}
	if poll <= 0 {
		poll = 5 * time.Second
	}
	return &Worker{dispatcher: dispatcher, concurrency: concurrency, poll: poll}
}

func (w *Worker) Run() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.running = true
	if w.dispatcher == nil {
		w.running = false
		cancel()
		return nil
	}
	if _, err := w.dispatcher.Recover(ctx, 1000); err != nil {
		log.Printf("[Job] recovery failed: %v", err)
	}
	for i := 0; i < w.concurrency; i++ {
		w.wg.Add(1)
		go w.loop(ctx)
	}
	return nil
}

func (w *Worker) loop(ctx context.Context) {
	defer w.wg.Done()
	for ctx.Err() == nil {
		_, err := w.dispatcher.DispatchOnce(ctx, w.poll)
		if err != nil && ctx.Err() == nil {
			log.Printf("[Job] dispatch failed: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}

func (w *Worker) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	// Serialize Stop/Run until the old goroutines have exited; otherwise a
	// concurrent Run can add to the WaitGroup while Stop is waiting.
	w.cancel()
	w.wg.Wait()
	w.running = false
	w.mu.Unlock()
}

func (w *Worker) IsRunning() bool { w.mu.Lock(); defer w.mu.Unlock(); return w.running }
