// Package job provides reusable durable job delivery primitives. Domain
// packages supply typed payload adapters and handlers; this package owns no
// business status or persistence semantics.
package job

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type Job struct {
	// receipt/raw identify one leased delivery; they are never serialized.
	receipt string
	raw     string
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	Attempt int             `json:"attempt"`
}

func New(id, typ string, payload any) (Job, error) {
	if id == "" || typ == "" {
		return Job{}, fmt.Errorf("job id and type are required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Job{}, fmt.Errorf("marshal job payload: %w", err)
	}
	return Job{ID: id, Type: typ, Payload: body}, nil
}

type Queue interface {
	Enqueue(context.Context, Job) error
	Claim(context.Context, time.Duration) (*Job, error)
	Ack(context.Context, Job) error
	Retry(context.Context, Job, time.Duration) error
	PromoteDue(context.Context, int) (int, error)
	Recover(context.Context) (int, error)
}

// RecoverySource makes a domain's durable pending work visible to the generic
// dispatcher without coupling the dispatcher to any repository model.
type RecoverySource interface {
	PendingJobs(context.Context, int) ([]Job, error)
}

type Handler interface {
	Handle(context.Context, Job) error
}
type ExhaustionHandler interface {
	Exhausted(context.Context, Job, error) error
}

type RetryPolicy struct {
	MaxAttempts         int
	BaseDelay, MaxDelay time.Duration
}

func (p RetryPolicy) normalize() RetryPolicy {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 3
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = time.Second
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = time.Minute
	}
	if p.MaxDelay < p.BaseDelay {
		p.MaxDelay = p.BaseDelay
	}
	return p
}
func (p RetryPolicy) delay(attempt int) time.Duration {
	p = p.normalize()
	delay := p.BaseDelay
	for i := 0; i <= attempt && delay < p.MaxDelay; i++ {
		delay *= 2
		if delay > p.MaxDelay {
			return p.MaxDelay
		}
	}
	return delay
}
