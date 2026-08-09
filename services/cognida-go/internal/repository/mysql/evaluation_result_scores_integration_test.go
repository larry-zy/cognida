//go:build integration
// +build integration

// Package mysql: 评测结果动态指标载体（scores JSON 列）真实 DB 集成测试。
//
// 针对真实 MySQL 运行，受 `integration` 构建标签门控（见 CLAUDE.md 集成测试）。
// 通过 MYSQL_DSN 提供 DSN，例如：
//
//	MYSQL_DSN='root:password@tcp(localhost:3306)/cognida?charset=utf8mb4&parseTime=True&loc=Local' \
//	  go test -tags=integration ./internal/repository/mysql/ -run TestEvaluationResultScores -v
//
// 覆盖：注册表驱动的动态 scores{name:value} 写入 JSON 列→回读往返，
// 固定指标列（precision/rouge_1/…）仍与写入一致；NULL/空 scores 读侧兼容为空 map。
package mysql

import (
	"context"
	"testing"

	"cognida/internal/model/evaluation"
)

func f64(v float64) *float64 { return &v }

// TestEvaluationResultScoresRoundTrip 动态 scores 与固定字段一并落库并原样回读。
func TestEvaluationResultScoresRoundTrip(t *testing.T) {
	db := newIntegrationDB(t)
	if err := db.AutoMigrate(&EvaluationResultModel{}); err != nil {
		t.Fatalf("automigrate evaluation_qa_results: %v", err)
	}

	repo := NewEvaluationResultRepository(db)
	ctx := context.Background()
	const taskID = "it_scores_task_0001"

	clean := func() { _ = repo.DeleteByTaskID(ctx, taskID) }
	clean()
	t.Cleanup(clean)

	want := &evaluation.EvaluationResult{
		TaskID:          taskID,
		Question:        "地球到月球多远?",
		ReferenceAnswer: "约 38 万公里",
		GeneratedAnswer: "大约 384400 公里",
		Success:         true,
		// 本条 QA 的子 request_id（供前端深链到 trace 瀑布图）
		RequestID: taskID + "-abcd1234#1",
		// 固定指标列
		Precision: f64(0.75),
		ROUGE1:    f64(0.42),
		// 动态指标载体：注册表驱动的 name->value（含扩展指标 exact_match/llm_factual）
		Scores: map[string]float64{
			"precision":   0.75,
			"rouge_1":     0.42,
			"exact_match": 100,
			"llm_factual": 88.5,
		},
	}

	if err := repo.Create(ctx, want); err != nil {
		t.Fatalf("create result: %v", err)
	}

	got, err := repo.FindByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("find by task id: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	r := got[0]

	// 动态 scores 原样回读
	if len(r.Scores) != len(want.Scores) {
		t.Fatalf("scores size mismatch: want %v, got %v", want.Scores, r.Scores)
	}
	for k, v := range want.Scores {
		if gv, ok := r.Scores[k]; !ok || gv != v {
			t.Errorf("scores[%q]: want %v, got %v (present=%v)", k, v, gv, ok)
		}
	}

	// 固定字段仍与写入一致
	if r.Precision == nil || *r.Precision != 0.75 {
		t.Errorf("precision fixed column: want 0.75, got %v", r.Precision)
	}
	if r.ROUGE1 == nil || *r.ROUGE1 != 0.42 {
		t.Errorf("rouge_1 fixed column: want 0.42, got %v", r.ROUGE1)
	}
	if r.GeneratedAnswer != want.GeneratedAnswer {
		t.Errorf("generated_answer: want %q, got %q", want.GeneratedAnswer, r.GeneratedAnswer)
	}
	// 子 request_id 原样回读，供前端深链 trace 瀑布图
	if r.RequestID != want.RequestID {
		t.Errorf("request_id: want %q, got %q", want.RequestID, r.RequestID)
	}
}

// TestEvaluationResultScoresNullCompatible 未写 scores 时 NULL 列读侧兼容为空 map（非 nil）。
func TestEvaluationResultScoresNullCompatible(t *testing.T) {
	db := newIntegrationDB(t)
	if err := db.AutoMigrate(&EvaluationResultModel{}); err != nil {
		t.Fatalf("automigrate evaluation_qa_results: %v", err)
	}

	repo := NewEvaluationResultRepository(db)
	ctx := context.Background()
	const taskID = "it_scores_task_null_0001"

	clean := func() { _ = repo.DeleteByTaskID(ctx, taskID) }
	clean()
	t.Cleanup(clean)

	// 不设置 Scores（模拟旧数据/纯固定字段路径）
	if err := repo.Create(ctx, &evaluation.EvaluationResult{
		TaskID:          taskID,
		Question:        "无动态指标",
		GeneratedAnswer: "answer",
		Success:         true,
		Precision:       f64(0.5),
	}); err != nil {
		t.Fatalf("create result: %v", err)
	}

	got, err := repo.FindByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("find by task id: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if got[0].Scores == nil {
		t.Errorf("scores should be non-nil empty map on NULL column, got nil")
	}
	if len(got[0].Scores) != 0 {
		t.Errorf("scores should be empty on NULL column, got %v", got[0].Scores)
	}
}
