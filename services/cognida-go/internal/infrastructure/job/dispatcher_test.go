package job

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type memoryQueue struct {
	jobs, retried, acked []Job
	recovered            int
}

func (q *memoryQueue) Enqueue(_ context.Context, j Job) error { q.jobs = append(q.jobs, j); return nil }
func (q *memoryQueue) Claim(_ context.Context, _ time.Duration) (*Job, error) {
	if len(q.jobs) == 0 {
		return nil, nil
	}
	j := q.jobs[0]
	q.jobs = q.jobs[1:]
	return &j, nil
}
func (q *memoryQueue) Ack(_ context.Context, j Job) error { q.acked = append(q.acked, j); return nil }
func (q *memoryQueue) Retry(_ context.Context, j Job, _ time.Duration) error {
	j.Attempt++
	q.retried = append(q.retried, j)
	return nil
}
func (q *memoryQueue) PromoteDue(context.Context, int) (int, error) { return 0, nil }
func (q *memoryQueue) Recover(context.Context) (int, error)         { return q.recovered, nil }

type handler struct {
	errs      []error
	exhausted int
}

func (h *handler) Handle(context.Context, Job) error           { e := h.errs[0]; h.errs = h.errs[1:]; return e }
func (h *handler) Exhausted(context.Context, Job, error) error { h.exhausted++; return nil }

type source struct{ jobs []Job }

func (s source) PendingJobs(context.Context, int) ([]Job, error) { return s.jobs, nil }

func TestDispatcherRetriesThenCallsDomainExhaustionHandler(t *testing.T) {
	q := &memoryQueue{jobs: []Job{{ID: "a", Type: "test"}}}
	h := &handler{errs: []error{errors.New("temp"), errors.New("fatal")}}
	d := NewDispatcher(q, RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond})
	d.Register("test", h, nil)
	ok, err := d.DispatchOnce(context.Background(), 0)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, q.retried, 1)
	q.jobs = append(q.jobs, q.retried[0])
	ok, err = d.DispatchOnce(context.Background(), 0)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 1, h.exhausted)
	require.Len(t, q.acked, 1)
}
func TestDispatcherRecoveryUsesGenericSources(t *testing.T) {
	q := &memoryQueue{recovered: 1}
	d := NewDispatcher(q, RetryPolicy{})
	d.Register("test", &handler{}, source{jobs: []Job{{ID: "a", Type: "test"}}})
	n, err := d.Recover(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, []Job{{ID: "a", Type: "test"}}, q.jobs)
}
func TestRetryPolicyCapsBackoff(t *testing.T) {
	p := RetryPolicy{BaseDelay: time.Second, MaxDelay: 3 * time.Second}
	require.Equal(t, 2*time.Second, p.delay(0))
	require.Equal(t, 3*time.Second, p.delay(4))
}

type panicHandler struct{}

func (panicHandler) Handle(context.Context, Job) error { panic("broken plugin") }
func TestDispatcherRetriesHandlerPanic(t *testing.T) {
	q := &memoryQueue{jobs: []Job{{ID: "panic", Type: "test"}}}
	d := NewDispatcher(q, RetryPolicy{})
	d.Register("test", panicHandler{}, nil)
	handled, err := d.DispatchOnce(context.Background(), 0)
	require.NoError(t, err)
	require.True(t, handled)
	require.Len(t, q.retried, 1)
	require.Empty(t, q.acked)
}

type lostLeaseQueue struct{ memoryQueue }

func (*lostLeaseQueue) Heartbeat(context.Context, Job) error { return errors.New("lease lost") }

type cancelledHandler struct{ exhausted bool }

func (*cancelledHandler) Handle(ctx context.Context, _ Job) error {
	<-ctx.Done()
	return errors.New("stale handler failure")
}
func (h *cancelledHandler) Exhausted(context.Context, Job, error) error {
	h.exhausted = true
	return nil
}
func TestDispatcherLeaseLossDoesNotFinalizeDomain(t *testing.T) {
	q := &lostLeaseQueue{memoryQueue{jobs: []Job{{ID: "old", Type: "test", Attempt: 2}}}}
	h := &cancelledHandler{}
	d := NewDispatcher(q, RetryPolicy{MaxAttempts: 3})
	d.heartbeatInterval = time.Millisecond
	d.Register("test", h, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := d.DispatchOnce(ctx, 0)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, h.exhausted)
	require.Empty(t, q.retried)
	require.Empty(t, q.acked)
}
