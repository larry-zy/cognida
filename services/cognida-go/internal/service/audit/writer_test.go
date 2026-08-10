package audit

import (
	"context"
	"sync"
	"testing"
	"time"

	auditmodel "cognida/internal/model/audit"
)

// fakeRepo 记录落库条数，仅用于验证写入器行为。
type fakeRepo struct {
	mu      sync.Mutex
	created []*auditmodel.AuditLog
}

func (r *fakeRepo) Create(ctx context.Context, log *auditmodel.AuditLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created = append(r.created, log)
	return nil
}
func (r *fakeRepo) FindByID(ctx context.Context, id int64) (*auditmodel.AuditLog, error) {
	return nil, nil
}
func (r *fakeRepo) Query(ctx context.Context, q *auditmodel.Query) ([]*auditmodel.AuditLog, int64, error) {
	return nil, 0, nil
}
func (r *fakeRepo) QueryByCursor(ctx context.Context, q *auditmodel.Query) ([]*auditmodel.AuditLog, string, error) {
	return nil, "", nil
}
func (r *fakeRepo) GetStats(ctx context.Context, tenantID *int64, days int) (*auditmodel.Stats, error) {
	return nil, nil
}
func (r *fakeRepo) DeleteBefore(ctx context.Context, date time.Time) (int64, error) {
	return 0, nil
}

func (r *fakeRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.created)
}

// TestWriterFlushOnClose 验证：入队的条目在 Close 时全部 flush 落库。
func TestWriterFlushOnClose(t *testing.T) {
	repo := &fakeRepo{}
	w := NewWriter(repo, WriterConfig{BufferSize: 100, BatchSize: 10, FlushInterval: time.Hour})

	const n = 25
	for i := 0; i < n; i++ {
		w.Enqueue(auditmodel.NewAuditLog(nil, nil, "test", "act"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := w.Close(ctx); err != nil {
		t.Fatalf("close returned error: %v", err)
	}

	if got := repo.count(); got != n {
		t.Fatalf("expected %d flushed, got %d", n, got)
	}
}

// TestWriterBatchFlush 验证：达到批量阈值即触发 flush（无需等待定时器）。
func TestWriterBatchFlush(t *testing.T) {
	repo := &fakeRepo{}
	w := NewWriter(repo, WriterConfig{BufferSize: 100, BatchSize: 5, FlushInterval: time.Hour})
	defer w.Close(context.Background())

	for i := 0; i < 5; i++ {
		w.Enqueue(auditmodel.NewAuditLog(nil, nil, "test", "act"))
	}

	// 轮询等待后台 goroutine 处理完这一批。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if repo.count() == 5 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("batch not flushed on threshold, got %d", repo.count())
}

// TestWriterEnqueueAfterClose 验证：Close 后入队被安全忽略，不 panic。
func TestWriterEnqueueAfterClose(t *testing.T) {
	repo := &fakeRepo{}
	w := NewWriter(repo, DefaultWriterConfig())
	_ = w.Close(context.Background())
	// 不应 panic，也不应落库。
	w.Enqueue(auditmodel.NewAuditLog(nil, nil, "test", "act"))
	if got := repo.count(); got != 0 {
		t.Fatalf("expected no writes after close, got %d", got)
	}
}
