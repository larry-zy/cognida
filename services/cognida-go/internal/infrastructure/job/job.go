// Package job implements durable delivery adapters for the model/job contracts.
package job

import (
	contract "cognida/internal/model/job"
	"time"
)

type Job = contract.Job
type Queue = contract.Queue
type RecoverySource = contract.RecoverySource
type Handler = contract.Handler
type ExhaustionHandler = contract.ExhaustionHandler

var New = contract.New

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
