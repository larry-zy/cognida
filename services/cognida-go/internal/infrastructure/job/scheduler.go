// Package job provides reusable polling and dispatch primitives for durable
// background jobs. Domain packages provide the Tick implementation.
package job

import (
	"context"
	"time"
)

// Ticker performs one bounded unit of scheduler work.
type Ticker interface {
	Tick(ctx context.Context) error
}

// Scheduler owns only timer lifecycle. It has no knowledge of a particular
// domain, storage engine, queue, or job payload.
type Scheduler struct {
	ticker Ticker
}

func NewScheduler(ticker Ticker) *Scheduler { return &Scheduler{ticker: ticker} }

// Run ticks once immediately, then at cadence until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context, cadence time.Duration) error {
	if cadence <= 0 {
		cadence = time.Minute
	}
	if err := s.ticker.Tick(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(cadence)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.ticker.Tick(ctx); err != nil {
				return err
			}
		}
	}
}
