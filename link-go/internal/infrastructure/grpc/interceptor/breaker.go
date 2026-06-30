package interceptor

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
)

type CircuitState int32

const (
	StateClosed CircuitState = iota
	StateHalfOpen
	StateOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateHalfOpen:
		return "half-open"
	case StateOpen:
		return "open"
	default:
		return "unknown"
	}
}

type Counts struct {
	Requests             uint32
	TotalSuccesses       uint32
	TotalFailures        uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
}

type BreakerConfig struct {
	MaxRequests    uint32
	Interval       time.Duration
	Timeout        time.Duration
	ReadyToTrip    TripConditionFunc
	OnStateChange  StateChangeCallback
	IsSuccess      func(error) bool
}

type TripConditionFunc func(counts Counts) bool
type StateChangeCallback func(name string, from, to CircuitState)

func DefaultBreakerConfig() *BreakerConfig {
	return &BreakerConfig{
		MaxRequests: 3,
		Interval:    10 * time.Second,
		Timeout:     60 * time.Second,
		ReadyToTrip: DefaultTripCondition,
		IsSuccess:   func(err error) bool { return err == nil },
	}
}

func DefaultTripCondition(counts Counts) bool {
	failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
	return counts.Requests >= 5 && failureRatio > 0.5
}

type Breaker struct {
	name      string
	cfg       *BreakerConfig
	state     atomic.Value // CircuitState
	generation atomic.Uint64
	counts    atomic.Value // Counts
	expiry    atomic.Value // time.Time
	mu        sync.Mutex
}

func NewBreaker(name string, cfg *BreakerConfig) *Breaker {
	if cfg == nil {
		cfg = DefaultBreakerConfig()
	}
	b := &Breaker{name: name, cfg: cfg}
	b.state.Store(StateClosed)
	b.counts.Store(Counts{})
	b.expiry.Store(time.Now().Add(cfg.Interval))
	return b
}

func (b *Breaker) Name() string { return b.name }
func (b *Breaker) State() CircuitState { return b.state.Load().(CircuitState) }

func (b *Breaker) Allow() error {
	state := b.State()
	if state == StateOpen {
		if b.expiry.Load().(time.Time).Before(time.Now()) {
			b.changeState(StateHalfOpen)
			return nil
		}
		return fmt.Errorf("circuit breaker is open")
	}
	if state == StateHalfOpen && b.cfg.MaxRequests > 0 {
		counts := b.counts.Load().(Counts)
		if counts.Requests >= b.cfg.MaxRequests {
			return fmt.Errorf("circuit breaker is half-open and max requests reached")
		}
	}
	return nil
}

func (b *Breaker) onSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	counts := b.counts.Load().(Counts)
	counts.Requests++
	counts.TotalSuccesses++
	counts.ConsecutiveSuccesses++
	counts.ConsecutiveFailures = 0
	b.counts.Store(counts)
	if b.State() == StateHalfOpen && counts.ConsecutiveSuccesses >= b.cfg.MaxRequests {
		b.changeState(StateClosed)
	}
}

func (b *Breaker) onFail() {
	b.mu.Lock()
	defer b.mu.Unlock()
	counts := b.counts.Load().(Counts)
	counts.Requests++
	counts.TotalFailures++
	counts.ConsecutiveFailures++
	counts.ConsecutiveSuccesses = 0
	b.counts.Store(counts)
	state := b.State()
	if state == StateHalfOpen {
		b.changeState(StateOpen)
	} else if state == StateClosed && b.cfg.ReadyToTrip(counts) {
		b.changeState(StateOpen)
	}
}

func (b *Breaker) changeState(newState CircuitState) {
	oldState := b.State()
	if oldState == newState {
		return
	}
	b.state.Store(newState)
	b.generation.Add(1)
	if newState == StateClosed {
		b.counts.Store(Counts{})
	}
	if newState == StateOpen {
		b.expiry.Store(time.Now().Add(b.cfg.Timeout))
	} else {
		b.expiry.Store(time.Now().Add(b.cfg.Interval))
	}
	if b.cfg.OnStateChange != nil {
		b.cfg.OnStateChange(b.name, oldState, newState)
	}
}

func (b *Breaker) do(ctx context.Context, req func() error) error {
	if err := b.Allow(); err != nil {
		return err
	}
	err := req()
	if b.cfg.IsSuccess(err) {
		b.onSuccess()
	} else {
		b.onFail()
	}
	return err
}

type BreakerGroup struct {
	mu      sync.RWMutex
	breaker map[string]*Breaker
	cfg     *BreakerConfig
}

func NewBreakerGroup(cfg *BreakerConfig) *BreakerGroup {
	return &BreakerGroup{breaker: make(map[string]*Breaker), cfg: cfg}
}

func (g *BreakerGroup) Get(name string) *Breaker {
	g.mu.RLock()
	b, ok := g.breaker[name]
	g.mu.RUnlock()
	if ok {
		return b
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if b, ok = g.breaker[name]; ok {
		return b
	}
	b = NewBreaker(name, g.cfg)
	g.breaker[name] = b
	return b
}

func UnaryBreaker(group *BreakerGroup) grpc.UnaryClientInterceptor {
	if group == nil {
		group = NewBreakerGroup(nil)
	}
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		breaker := group.Get(method)
		return breaker.do(ctx, func() error {
			return invoker(ctx, method, req, reply, cc, opts...)
		})
	}
}

func StreamBreaker(group *BreakerGroup) grpc.StreamClientInterceptor {
	if group == nil {
		group = NewBreakerGroup(nil)
	}
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		breaker := group.Get(method)
		var stream grpc.ClientStream
		err := breaker.do(ctx, func() error {
			var err error
			stream, err = streamer(ctx, desc, cc, method, opts...)
			return err
		})
		return stream, err
	}
}
