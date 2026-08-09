// Package evaluation：任务失败落库不受调用方 ctx 取消影响的回归测试。
//
// 背景（B4 回归）：executeTask 的根 ctx 叠加了 MaxTaskTimeout 且随 Stop() 可取消。
// 任务超时/停机后该 ctx 已 Done，若 handleTaskError 复用它写库，db.WithContext(ctx)
// 会在执行 UPDATE 前短路返回 context.Canceled，失败状态永远写不进库、任务卡在 running。
// 修复后 handleTaskError 用 context.WithoutCancel 剥离取消信号再落库。
package evaluation

import (
	"context"
	"testing"

	domeval "cognida/internal/model/evaluation"
)

// ctxRecordingTaskRepo 记录 UpdateStatus/UpdateError 收到的 ctx 是否已被取消。
type ctxRecordingTaskRepo struct {
	statusCanceled bool
	statusWritten  bool
	errWritten     bool
	lastStatus     domeval.TaskStatus
}

func (r *ctxRecordingTaskRepo) Create(ctx context.Context, task *domeval.EvaluationTask) error {
	return nil
}
func (r *ctxRecordingTaskRepo) FindByID(ctx context.Context, id string) (*domeval.EvaluationTask, error) {
	return nil, nil
}
func (r *ctxRecordingTaskRepo) UpdateStatus(ctx context.Context, id string, status domeval.TaskStatus) error {
	r.statusWritten = true
	r.statusCanceled = ctx.Err() != nil
	r.lastStatus = status
	return nil
}
func (r *ctxRecordingTaskRepo) UpdateProgress(ctx context.Context, id string, success, failure int) error {
	return nil
}
func (r *ctxRecordingTaskRepo) UpdateError(ctx context.Context, id string, errorMsg string) error {
	r.errWritten = true
	return ctx.Err()
}
func (r *ctxRecordingTaskRepo) UpdateMetrics(ctx context.Context, id string, metrics *domeval.TaskMetrics) error {
	return nil
}
func (r *ctxRecordingTaskRepo) SoftDelete(ctx context.Context, id string) error { return nil }
func (r *ctxRecordingTaskRepo) List(ctx context.Context, filter *domeval.TaskFilter) ([]*domeval.EvaluationTask, int64, error) {
	return nil, 0, nil
}
func (r *ctxRecordingTaskRepo) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

// noopProgressWriter 满足 ProgressWriter 接口，不做任何事。
type noopProgressWriter struct{}

func (noopProgressWriter) SetProgress(ctx context.Context, taskID string, p *domeval.Progress) error {
	return nil
}
func (noopProgressWriter) SetError(ctx context.Context, taskID, errMsg string, retryCount int) error {
	return nil
}

// TestHandleTaskError_PersistsUnderCanceledCtx 验证：即便传入已取消的 ctx，
// 失败状态与错误信息仍能落库（handleTaskError 内部剥离取消信号）。
func TestHandleTaskError_PersistsUnderCanceledCtx(t *testing.T) {
	repo := &ctxRecordingTaskRepo{}
	w := &EvaluationWorker{taskRepo: repo, progressCache: noopProgressWriter{}}

	// 模拟任务超时/停机：ctx 已取消。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w.handleTaskError(ctx, "task-timeout-1", context.DeadlineExceeded, 0)

	if !repo.statusWritten {
		t.Fatal("失败状态未落库")
	}
	if repo.statusCanceled {
		t.Error("UpdateStatus 仍收到已取消的 ctx——落库会被 db.WithContext 短路")
	}
	if repo.lastStatus != domeval.TaskStatusFailed {
		t.Errorf("期望状态 failed，实得 %v", repo.lastStatus)
	}
	if !repo.errWritten {
		t.Error("错误信息未落库")
	}
}
