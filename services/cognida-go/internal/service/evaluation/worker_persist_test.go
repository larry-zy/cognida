// Package evaluation：逐条结果落库幂等性回归测试。
//
// 背景（执行链缺陷之一）：saveResults 曾直接 CreateBatch 而不先清旧行，任务重跑
// （启动恢复重入队 / 手动重跑）或「打分前落原始产物 + 打分后落富结果」两次落库会
// 累积重复行，使前端逐条列表出现重复、成功/失败计数虚高。persistResults 改为
// delete+insert 幂等替换后，多次落库同一 taskID 只保留最后一批。
package evaluation

import (
	"context"
	"testing"

	domeval "cognida/internal/model/evaluation"
)

// fakeResultRepo 内存版评测结果仓储，按 taskID 保存行，供幂等性断言。
type fakeResultRepo struct {
	rows       map[string][]*domeval.EvaluationResult
	deletes    int
	createCall int
}

func newFakeResultRepo() *fakeResultRepo {
	return &fakeResultRepo{rows: map[string][]*domeval.EvaluationResult{}}
}

func (f *fakeResultRepo) Create(ctx context.Context, result *domeval.EvaluationResult) error {
	f.rows[result.TaskID] = append(f.rows[result.TaskID], result)
	return nil
}

func (f *fakeResultRepo) CreateBatch(ctx context.Context, results []*domeval.EvaluationResult) error {
	f.createCall++
	for _, r := range results {
		f.rows[r.TaskID] = append(f.rows[r.TaskID], r)
	}
	return nil
}

func (f *fakeResultRepo) FindByTaskID(ctx context.Context, taskID string) ([]*domeval.EvaluationResult, error) {
	return f.rows[taskID], nil
}

func (f *fakeResultRepo) FindByTaskIDWithPagination(ctx context.Context, taskID string, page, pageSize int) ([]*domeval.EvaluationResult, int64, error) {
	rows := f.rows[taskID]
	return rows, int64(len(rows)), nil
}

func (f *fakeResultRepo) FindByTaskIDByCursor(ctx context.Context, taskID string, cursor string, limit int) ([]*domeval.EvaluationResult, string, error) {
	return f.rows[taskID], "", nil
}

func (f *fakeResultRepo) DeleteByTaskID(ctx context.Context, taskID string) error {
	f.deletes++
	delete(f.rows, taskID)
	return nil
}

// ReplaceByTaskID 模拟事务内「先删后插」的幂等替换（与真实仓储语义一致）。
func (f *fakeResultRepo) ReplaceByTaskID(ctx context.Context, taskID string, results []*domeval.EvaluationResult) error {
	f.deletes++
	delete(f.rows, taskID)
	f.createCall++
	for _, r := range results {
		f.rows[r.TaskID] = append(f.rows[r.TaskID], r)
	}
	return nil
}

// TestPersistResults_IdempotentOnRerun 验证同一 taskID 多次 persistResults 不累积行：
// 每次落库前先按 taskID 清旧，最终只剩最后一批。
func TestPersistResults_IdempotentOnRerun(t *testing.T) {
	repo := newFakeResultRepo()
	w := &EvaluationWorker{resultRepo: repo}
	ctx := context.Background()
	const taskID = "task-rerun-1"

	// 首次落库：原始产物 2 条。
	if err := w.persistResults(ctx, taskID, []*QAResult{
		{Question: "q1", GeneratedAnswer: "a1", Success: true},
		{Question: "q2", GeneratedAnswer: "a2", Success: false},
	}); err != nil {
		t.Fatalf("first persist: %v", err)
	}
	if got := len(repo.rows[taskID]); got != 2 {
		t.Fatalf("首次落库应为 2 行，实得 %d", got)
	}

	// 二次落库（同一任务，模拟打分后富结果 / 重跑）：仍应为 2 行而非累积到 4 行。
	if err := w.persistResults(ctx, taskID, []*QAResult{
		{Question: "q1", GeneratedAnswer: "a1", Success: true},
		{Question: "q2", GeneratedAnswer: "a2", Success: true},
	}); err != nil {
		t.Fatalf("second persist: %v", err)
	}
	if got := len(repo.rows[taskID]); got != 2 {
		t.Errorf("重跑后应仍为 2 行（幂等替换），实得 %d（重复插行）", got)
	}
	// 两次落库应各清一次旧行。
	if repo.deletes != 2 {
		t.Errorf("应先删后插，DeleteByTaskID 期望调用 2 次，实得 %d", repo.deletes)
	}
	// 断言写入的是最后一批（q2 已变为 success）。
	for _, r := range repo.rows[taskID] {
		if r.Question == "q2" && !r.Success {
			t.Error("第二批未覆盖第一批：q2 仍为失败态")
		}
	}
}
