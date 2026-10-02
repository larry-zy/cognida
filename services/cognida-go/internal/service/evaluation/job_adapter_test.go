package evaluation

import (
	"context"
	"testing"
	"time"

	jobinfra "cognida/internal/infrastructure/job"
)

type jobQueueStub struct{ jobs []jobinfra.Job }

func (q *jobQueueStub) Enqueue(_ context.Context, j jobinfra.Job) error {
	q.jobs = append(q.jobs, j)
	return nil
}
func (q *jobQueueStub) Claim(context.Context, time.Duration) (*jobinfra.Job, error) { return nil, nil }
func (q *jobQueueStub) Ack(context.Context, jobinfra.Job) error                     { return nil }
func (q *jobQueueStub) Retry(context.Context, jobinfra.Job, time.Duration) error    { return nil }
func (q *jobQueueStub) PromoteDue(context.Context, int) (int, error)                { return 0, nil }
func (q *jobQueueStub) Recover(context.Context) (int, error)                        { return 0, nil }

func TestJobEnqueuerCreatesTypedEvaluationJob(t *testing.T) {
	queue := &jobQueueStub{}
	if err := (JobEnqueuer{Queue: queue}).Enqueue(context.Background(), "eval-123"); err != nil {
		t.Fatal(err)
	}
	if len(queue.jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(queue.jobs))
	}
	payload, err := decodeRunJob(queue.jobs[0])
	if err != nil {
		t.Fatal(err)
	}
	if payload.TaskID != "eval-123" {
		t.Fatalf("task id = %q", payload.TaskID)
	}
}

func TestDecodeRunJobRejectsWrongType(t *testing.T) {
	if _, err := decodeRunJob(jobinfra.Job{Type: "other", Payload: []byte(`{}`)}); err == nil {
		t.Fatal("expected error")
	}
}
