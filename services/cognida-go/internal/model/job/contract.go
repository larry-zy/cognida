// Package job defines transport-independent durable delivery contracts.
package job

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type Job struct {
	// Delivery is an opaque adapter-owned lease, never persisted in job payloads.
	Delivery any             `json:"-"`
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Payload  json.RawMessage `json:"payload"`
	Attempt  int             `json:"attempt"`
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

// Runner is the lifecycle of a configured delivery worker.
type Runner interface {
	Run() error
	Stop()
	IsRunning() bool
}

// Recoverer restores durable pending deliveries through the configured dispatcher.
type Recoverer interface {
	Recover(context.Context, int) (int, error)
}
