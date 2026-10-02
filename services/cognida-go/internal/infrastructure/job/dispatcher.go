package job

import (
	"context"
	"fmt"
	"time"
)

// Dispatcher is a generic single-message scheduler. Applications can run it
// in multiple processes; Queue's processing list makes delivery at-least-once.
type Dispatcher struct {
	queue             Queue
	handlers          map[string]Handler
	recovery          []RecoverySource
	retry             RetryPolicy
	heartbeatInterval time.Duration
}

func NewDispatcher(queue Queue, retry RetryPolicy) *Dispatcher {
	return &Dispatcher{queue: queue, handlers: map[string]Handler{}, retry: retry.normalize(), heartbeatInterval: jobLeaseDuration / 3}
}
func (d *Dispatcher) Register(typ string, handler Handler, source RecoverySource) {
	d.handlers[typ] = handler
	if source != nil {
		d.recovery = append(d.recovery, source)
	}
}
func (d *Dispatcher) Recover(ctx context.Context, limit int) (int, error) {
	n, err := d.queue.Recover(ctx)
	if err != nil {
		return 0, err
	}
	for _, source := range d.recovery {
		jobs, err := source.PendingJobs(ctx, limit)
		if err != nil {
			return n, err
		}
		for _, j := range jobs {
			if err := d.queue.Enqueue(ctx, j); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}
func (d *Dispatcher) DispatchOnce(ctx context.Context, wait time.Duration) (bool, error) {
	if _, err := d.queue.Recover(ctx); err != nil {
		return false, err
	}
	if _, err := d.queue.PromoteDue(ctx, 100); err != nil {
		return false, err
	}
	j, err := d.queue.Claim(ctx, wait)
	if err != nil || j == nil {
		return false, err
	}
	h, ok := d.handlers[j.Type]
	if !ok {
		return true, d.queue.Ack(ctx, *j)
	}
	handlerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if leased, ok := d.queue.(interface {
		Heartbeat(context.Context, Job) error
	}); ok {
		go func() {
			ticker := time.NewTicker(d.heartbeatInterval)
			defer ticker.Stop()
			for {
				select {
				case <-handlerCtx.Done():
					return
				case <-ticker.C:
					if leased.Heartbeat(handlerCtx, *j) != nil {
						cancel()
						return
					}
				}
			}
		}()
	}
	if err = callHandler(handlerCtx, h, *j); handlerCtx.Err() != nil {
		// Shutdown or lease loss leaves delivery recovery to its owner. A
		// stale worker must not retry, acknowledge, or finalize domain state.
		return true, handlerCtx.Err()
	} else if err == nil {
		return true, d.queue.Ack(ctx, *j)
	} else if j.Attempt+1 >= d.retry.MaxAttempts {
		if terminal, ok := h.(ExhaustionHandler); ok {
			if e := terminal.Exhausted(ctx, *j, err); e != nil {
				return true, fmt.Errorf("finalize exhausted job: %w", e)
			}
		}
		return true, d.queue.Ack(ctx, *j)
	}
	return true, d.queue.Retry(ctx, *j, d.retry.delay(j.Attempt))
}

// A bad handler must not crash the entire API process hosting the worker.
func callHandler(ctx context.Context, h Handler, j Job) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("job handler panic: %v", p)
		}
	}()
	return h.Handle(ctx, j)
}
