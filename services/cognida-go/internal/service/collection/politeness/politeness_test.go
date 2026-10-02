package politeness

import (
	"context"
	"sync"
	"testing"
	"time"

	domain "cognida/internal/model/collection"
)

type fetchFunc func(context.Context, domain.FetchRequest) (*domain.FetchResponse, error)

func (f fetchFunc) Fetch(ctx context.Context, request domain.FetchRequest) (*domain.FetchResponse, error) {
	return f(ctx, request)
}

func TestFetcherLimitsConcurrentRequestsForSameDomain(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var mu sync.Mutex
	active, peak := 0, 0
	delegate := fetchFunc(func(_ context.Context, _ domain.FetchRequest) (*domain.FetchResponse, error) {
		mu.Lock()
		active++
		if active > peak {
			peak = active
		}
		mu.Unlock()
		started <- struct{}{}
		<-release
		mu.Lock()
		active--
		mu.Unlock()
		return &domain.FetchResponse{}, nil
	})
	f := NewFetcher(delegate, NewGate(Policy{MaxConcurrentPerDomain: 1}))
	done := make(chan error, 2)
	go func() {
		_, err := f.Fetch(context.Background(), domain.FetchRequest{URL: "https://example.com/one"})
		done <- err
	}()
	<-started
	go func() {
		_, err := f.Fetch(context.Background(), domain.FetchRequest{URL: "https://example.com/two"})
		done <- err
	}()

	select {
	case <-started:
		t.Fatal("second request started before first same-domain request released")
	case <-time.After(40 * time.Millisecond):
	}
	close(release)
	<-started
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if peak != 1 {
		t.Fatalf("peak concurrent requests = %d, want 1", peak)
	}
}

func TestFetcherAllowsDifferentDomainsConcurrently(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	delegate := fetchFunc(func(_ context.Context, _ domain.FetchRequest) (*domain.FetchResponse, error) {
		started <- struct{}{}
		<-release
		return &domain.FetchResponse{}, nil
	})
	f := NewFetcher(delegate, NewGate(Policy{MaxConcurrentPerDomain: 1}))
	done := make(chan error, 2)
	for _, rawURL := range []string{"https://one.example/path", "https://two.example/path"} {
		go func(u string) { _, err := f.Fetch(context.Background(), domain.FetchRequest{URL: u}); done <- err }(rawURL)
	}
	<-started
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("different domains should not share a concurrency permit")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestFetcherSpacesStartsForSameDomain(t *testing.T) {
	var mu sync.Mutex
	var starts []time.Time
	delegate := fetchFunc(func(_ context.Context, _ domain.FetchRequest) (*domain.FetchResponse, error) {
		mu.Lock()
		starts = append(starts, time.Now())
		mu.Unlock()
		return &domain.FetchResponse{}, nil
	})
	f := NewFetcher(delegate, NewGate(Policy{MinRequestInterval: 50 * time.Millisecond}))
	for _, rawURL := range []string{"https://example.com/one", "https://EXAMPLE.com:8443/two"} {
		if _, err := f.Fetch(context.Background(), domain.FetchRequest{URL: rawURL}); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if got := starts[1].Sub(starts[0]); got < 45*time.Millisecond {
		t.Fatalf("request start interval = %s, want at least 50ms", got)
	}
}

func TestGateReleasesPermitWhenContextCancelledWhileWaiting(t *testing.T) {
	gate := NewGate(Policy{MaxConcurrentPerDomain: 1})
	release, err := gate.Acquire(context.Background(), "https://example.com/one")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := gate.Acquire(ctx, "https://example.com/two"); err == nil {
		t.Fatal("Acquire unexpectedly succeeded")
	}
	release()
	secondRelease, err := gate.Acquire(context.Background(), "https://example.com/three")
	if err != nil {
		t.Fatalf("permit was not released after cancellation: %v", err)
	}
	secondRelease()
}

func TestDomain(t *testing.T) {
	got, err := Domain(" HTTPS://Example.COM.:8443/docs?q=1 ")
	if err != nil || got != "example.com" {
		t.Fatalf("Domain() = %q, %v", got, err)
	}
	if _, err := Domain("/relative"); err == nil {
		t.Fatal("relative URL accepted")
	}
}
