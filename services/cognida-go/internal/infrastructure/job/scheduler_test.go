package job

import (
	"context"
	"errors"
	"testing"
	"time"
)

type tickerStub struct {
	calls int
	err   error
}

func (s *tickerStub) Tick(context.Context) error { s.calls++; return s.err }

func TestSchedulerReturnsInitialTickError(t *testing.T) {
	want := errors.New("unavailable")
	scheduler := NewScheduler(&tickerStub{err: want})
	if err := scheduler.Run(context.Background(), time.Hour); !errors.Is(err, want) {
		t.Fatalf("Run() error = %v, want %v", err, want)
	}
}

func TestSchedulerStopsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stub := &tickerStub{}
	scheduler := NewScheduler(stub)
	if err := scheduler.Run(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context cancelled", err)
	}
	if stub.calls != 1 {
		t.Fatalf("initial calls = %d, want 1", stub.calls)
	}
}
