// Package politeness applies per-domain crawl limits at the Collection
// boundary. It is independent from queue and HTTP implementations, so every
// worker can share the same policy before invoking its underlying Fetcher.
package politeness

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	domain "cognida/internal/model/collection"
)

const defaultMaxConcurrentPerDomain = 1

var ErrInvalidDomain = errors.New("invalid collection fetch domain")

// Policy defines limits for one registrable host as represented by URL host
// names. A zero MaxConcurrentPerDomain uses the conservative value of one;
// a zero MinRequestInterval permits successive starts without a delay.
type Policy struct {
	MaxConcurrentPerDomain int
	MinRequestInterval     time.Duration
}

// Gate reserves permission to start one request. It is safe for concurrent
// use and may be shared by all Collection workers in a process.
type Gate struct {
	policy Policy
	now    func() time.Time
	wait   func(context.Context, time.Duration) error

	mu      sync.Mutex
	domains map[string]*domainGate
}

type domainGate struct {
	sem       chan struct{}
	mu        sync.Mutex
	nextStart time.Time
}

// NewGate constructs a gate with conservative defaults. A Gate has no
// dependency on the worker implementation and can be used by local or queued
// runners alike.
func NewGate(policy Policy) *Gate {
	if policy.MaxConcurrentPerDomain <= 0 {
		policy.MaxConcurrentPerDomain = defaultMaxConcurrentPerDomain
	}
	if policy.MinRequestInterval < 0 {
		policy.MinRequestInterval = 0
	}
	return &Gate{
		policy:  policy,
		now:     time.Now,
		wait:    waitContext,
		domains: make(map[string]*domainGate),
	}
}

// Acquire waits until a request for rawURL may start. Its release function
// must be called exactly once after the underlying fetch finishes. If the
// context is cancelled while waiting, no permit is retained.
func (g *Gate) Acquire(ctx context.Context, rawURL string) (release func(), err error) {
	domainName, err := Domain(rawURL)
	if err != nil {
		return nil, err
	}
	d := g.domain(domainName)
	select {
	case d.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	released := false
	release = func() {
		if released {
			return
		}
		released = true
		<-d.sem
	}

	delay := g.reserveStart(d)
	if delay > 0 {
		if err := g.wait(ctx, delay); err != nil {
			release()
			return nil, err
		}
	}
	return release, nil
}

func (g *Gate) domain(name string) *domainGate {
	g.mu.Lock()
	defer g.mu.Unlock()
	if d := g.domains[name]; d != nil {
		return d
	}
	d := &domainGate{sem: make(chan struct{}, g.policy.MaxConcurrentPerDomain)}
	g.domains[name] = d
	return d
}

func (g *Gate) reserveStart(d *domainGate) time.Duration {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := g.now()
	start := now
	if d.nextStart.After(start) {
		start = d.nextStart
	}
	d.nextStart = start.Add(g.policy.MinRequestInterval)
	return start.Sub(now)
}

// Domain extracts the stable hostname used as the policy key. Ports, paths,
// queries and case do not form separate crawl domains.
func Domain(rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme == "" || u.Hostname() == "" {
		return "", fmt.Errorf("%w: absolute URL required", ErrInvalidDomain)
	}
	return strings.TrimSuffix(strings.ToLower(u.Hostname()), "."), nil
}

// Fetcher wraps a Collection Fetcher with a shared Gate. It is intended for
// composition at a worker boundary, not for any specific queue technology.
type Fetcher struct {
	delegate domain.Fetcher
	gate     *Gate
}

func NewFetcher(delegate domain.Fetcher, gate *Gate) *Fetcher {
	if gate == nil {
		gate = NewGate(Policy{})
	}
	return &Fetcher{delegate: delegate, gate: gate}
}

func (f *Fetcher) Fetch(ctx context.Context, request domain.FetchRequest) (*domain.FetchResponse, error) {
	if f.delegate == nil {
		return nil, errors.New("collection polite fetcher delegate is nil")
	}
	release, err := f.gate.Acquire(ctx, request.URL)
	if err != nil {
		return nil, err
	}
	defer release()
	return f.delegate.Fetch(ctx, request)
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

var _ domain.Fetcher = (*Fetcher)(nil)
