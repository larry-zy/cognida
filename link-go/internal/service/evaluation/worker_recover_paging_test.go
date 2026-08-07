// Package evaluation：孤儿任务恢复分页回归测试（#4）。
//
// 背景：recoverStuckTasks 原以 PageSize:1000 单页拉取待恢复任务，但 taskRepo.List 会把
// PageSize 收敛到 100，第 101 个及以后的孤儿任务永远拉不到、卡死不恢复。修复后按 100 一页
// 翻页直至覆盖 total。本测试构造 250 个 pending 孤儿，断言全部被重新入队。
package evaluation

import (
	"context"
	"testing"

	domeval "link/internal/model/evaluation"
)

// pagingTaskRepo 模拟带 PageSize 收敛（≤100）的分页仓储：只对 pending 状态返回 total 条任务。
type pagingTaskRepo struct {
	total int // pending 任务总数
}

func (r *pagingTaskRepo) List(ctx context.Context, filter *domeval.TaskFilter) ([]*domeval.EvaluationTask, int64, error) {
	// 仅 pending 有数据，running 返回空（与被测分支一致）。
	if filter.Status == nil || *filter.Status != domeval.TaskStatusPending {
		return nil, 0, nil
	}
	pageSize := filter.PageSize
	if pageSize > 100 { // 复刻仓储层收敛：单页最多 100
		pageSize = 100
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	offset := (filter.Page - 1) * pageSize
	if offset >= r.total {
		return nil, int64(r.total), nil
	}
	end := offset + pageSize
	if end > r.total {
		end = r.total
	}
	tasks := make([]*domeval.EvaluationTask, 0, end-offset)
	for i := offset; i < end; i++ {
		tasks = append(tasks, &domeval.EvaluationTask{
			ID:     taskIDForIndex(i),
			Status: domeval.TaskStatusPending,
		})
	}
	return tasks, int64(r.total), nil
}

func (r *pagingTaskRepo) Create(ctx context.Context, task *domeval.EvaluationTask) error { return nil }
func (r *pagingTaskRepo) FindByID(ctx context.Context, id string) (*domeval.EvaluationTask, error) {
	return nil, nil
}
func (r *pagingTaskRepo) UpdateStatus(ctx context.Context, id string, status domeval.TaskStatus) error {
	return nil
}
func (r *pagingTaskRepo) UpdateProgress(ctx context.Context, id string, success, failure int) error {
	return nil
}
func (r *pagingTaskRepo) UpdateError(ctx context.Context, id string, errorMsg string) error {
	return nil
}
func (r *pagingTaskRepo) UpdateMetrics(ctx context.Context, id string, metrics *domeval.TaskMetrics) error {
	return nil
}
func (r *pagingTaskRepo) SoftDelete(ctx context.Context, id string) error { return nil }
func (r *pagingTaskRepo) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func taskIDForIndex(i int) string {
	return "orphan-" + itoa(i)
}

// itoa 避免为测试引入 strconv 的极小工具（保持文件自足）。
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// recordingQueue 记录被重新入队的任务 ID；其余队列方法为 no-op。
type recordingQueue struct {
	enqueued map[string]struct{}
}

func (q *recordingQueue) Enqueue(ctx context.Context, taskID string) error {
	q.enqueued[taskID] = struct{}{}
	return nil
}
func (q *recordingQueue) Dequeue(ctx context.Context) (string, error)   { return "", nil }
func (q *recordingQueue) AcquireSlot(ctx context.Context) (bool, error) { return true, nil }
func (q *recordingQueue) ReleaseSlot(ctx context.Context) error         { return nil }
func (q *recordingQueue) PendingIDs(ctx context.Context) ([]string, error) {
	return nil, nil // 队列空 → 全部视为孤儿，应全部重新入队
}
func (q *recordingQueue) ResetSlots(ctx context.Context) error { return nil }

// TestRecoverStuckTasks_PaginatesBeyondPageCap 验证：>100 的 pending 孤儿全部被恢复入队，
// 不因单页上限（100）而漏掉第 101 个及以后的任务。
func TestRecoverStuckTasks_PaginatesBeyondPageCap(t *testing.T) {
	const total = 250
	repo := &pagingTaskRepo{total: total}
	q := &recordingQueue{enqueued: make(map[string]struct{})}
	w := &EvaluationWorker{taskRepo: repo, queue: q}

	w.recoverStuckTasks(context.Background())

	if len(q.enqueued) != total {
		t.Fatalf("恢复入队 %d 条，期望 %d 条（分页未翻完，超页上限的孤儿被漏恢复）", len(q.enqueued), total)
	}
	// 抽查首、跨页边界、尾三个 ID 都在。
	for _, i := range []int{0, 99, 100, 199, 249} {
		if _, ok := q.enqueued[taskIDForIndex(i)]; !ok {
			t.Errorf("孤儿 %s 未被恢复入队", taskIDForIndex(i))
		}
	}
}
