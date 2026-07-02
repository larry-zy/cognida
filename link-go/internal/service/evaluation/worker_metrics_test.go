// Package evaluation 检索指标映射测试
//
// 回归用例：确保喂给 Python 指标服务的 retrieved_pids 是“检索到的分块ID”，
// 而非分块正文内容。历史 bug 曾把 RetrievedChunks(正文) 塞进 retrieved_pids 字段，
// 导致检索指标(precision/recall/ndcg/mrr)与 relevant_pids(分块ID) 对比恒不命中。
package evaluation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	domeval "link/internal/model/evaluation"
)

// TestComputeMetrics_RetrievedPIDsUsesChunkIDs 验证 computeMetrics 组装的
// ComputeItem.retrieved_pids 使用 RetrievedPIDs(分块ID)，且不等于 RetrievedChunks(正文)。
func TestComputeMetrics_RetrievedPIDsUsesChunkIDs(t *testing.T) {
	var captured *ComputeMetricsRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ComputeMetricsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request failed: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		captured = &req
		// 返回一个最小可用响应
		_ = json.NewEncoder(w).Encode(ComputeMetricsResponse{
			Items:     []*ComputeItemResult{{Index: 0}},
			Aggregate: map[string]float64{"precision": 1.0},
		})
	}))
	defer server.Close()

	worker := &EvaluationWorker{
		pythonClient: NewPythonEvaluationClient(server.URL),
	}

	qaResults := []*QAResult{
		{
			Question:        "谁提出了相对论？",
			ReferenceAnswer: "爱因斯坦",
			GeneratedAnswer: "爱因斯坦提出了相对论。",
			RetrievedChunks: []string{"爱因斯坦于1905年提出狭义相对论……(一大段正文)", "相对论改变了物理学……"},
			RetrievedPIDs:   []string{"chunk-001", "chunk-042"},
			RelevantPIDs:    []string{"chunk-001"},
		},
	}
	config := &DomainEvaluationTaskConfig{DatasetID: "default", Type: domeval.EvaluationTypeRAG}

	if _, err := worker.computeMetrics(context.Background(), config, qaResults); err != nil {
		t.Fatalf("computeMetrics failed: %v", err)
	}

	if captured == nil || len(captured.Items) != 1 {
		t.Fatalf("expected 1 captured item, got %+v", captured)
	}
	got := captured.Items[0].RetrievedPIDs
	want := []string{"chunk-001", "chunk-042"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("retrieved_pids = %v, want %v (应为分块ID而非正文)", got, want)
	}
	// 显式反向断言：不得等于正文内容
	if reflect.DeepEqual(got, qaResults[0].RetrievedChunks) {
		t.Errorf("retrieved_pids 不应等于 RetrievedChunks(正文)：%v", got)
	}
}

// TestComputeMetrics_ForwardsRetrievedContexts 验证 computeMetrics 把 RetrievedChunks(正文)
// 作为 ComputeItem.retrieved_contexts 转发给 Python，供忠实度/上下文相关性/噪声比计算。
func TestComputeMetrics_ForwardsRetrievedContexts(t *testing.T) {
	var captured *ComputeMetricsRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ComputeMetricsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		captured = &req
		_ = json.NewEncoder(w).Encode(ComputeMetricsResponse{
			Items:     []*ComputeItemResult{{Index: 0}},
			Aggregate: map[string]float64{},
		})
	}))
	defer server.Close()

	worker := &EvaluationWorker{pythonClient: NewPythonEvaluationClient(server.URL)}

	chunks := []string{"爱因斯坦于1905年提出狭义相对论……", "相对论改变了物理学……"}
	qaResults := []*QAResult{
		{
			Question:        "谁提出了相对论？",
			GeneratedAnswer: "爱因斯坦。",
			RetrievedChunks: chunks,
			RetrievedPIDs:   []string{"chunk-001", "chunk-042"},
			RelevantPIDs:    []string{"chunk-001"},
		},
	}
	config := &DomainEvaluationTaskConfig{DatasetID: "default", Type: domeval.EvaluationTypeRAG}

	if _, err := worker.computeMetrics(context.Background(), config, qaResults); err != nil {
		t.Fatalf("computeMetrics failed: %v", err)
	}
	if captured == nil || len(captured.Items) != 1 {
		t.Fatalf("expected 1 captured item, got %+v", captured)
	}
	if !reflect.DeepEqual(captured.Items[0].RetrievedContexts, chunks) {
		t.Errorf("retrieved_contexts = %v, want %v(分块正文)", captured.Items[0].RetrievedContexts, chunks)
	}
}

// TestFillMetrics_RAGAggregates 验证 fillMetrics 把 RAG 专属聚合指标写入 EvaluationResult。
func TestFillMetrics_RAGAggregates(t *testing.T) {
	worker := &EvaluationWorker{}
	evalResult := &EvaluationResult{QAResults: []*QAResult{{}}}
	resp := &ComputeMetricsResponse{
		Items: []*ComputeItemResult{{Index: 0}},
		Aggregate: map[string]float64{
			"faithfulness":      0.83,
			"context_relevance": 0.71,
			"noise_ratio":       0.25,
		},
	}

	worker.fillMetrics(evalResult, resp)

	if evalResult.Faithfulness == nil || *evalResult.Faithfulness != 0.83 {
		t.Errorf("faithfulness = %v, want 0.83", evalResult.Faithfulness)
	}
	if evalResult.ContextRelevance == nil || *evalResult.ContextRelevance != 0.71 {
		t.Errorf("context_relevance = %v, want 0.71", evalResult.ContextRelevance)
	}
	if evalResult.NoiseRatio == nil || *evalResult.NoiseRatio != 0.25 {
		t.Errorf("noise_ratio = %v, want 0.25", evalResult.NoiseRatio)
	}
}

// TestBuildTaskMetrics_MapsAllFields 验证聚合结果到任务级 TaskMetrics 的映射（含 RAG 专属指标）。
func TestBuildTaskMetrics_MapsAllFields(t *testing.T) {
	fp := func(v float64) *float64 { return &v }
	r := &EvaluationResult{
		Precision:          fp(0.9),
		Recall:             fp(0.8),
		NDCG:               fp(0.7),
		MRR:                fp(0.6),
		MAP:                fp(0.5),
		ROUGE1:             fp(0.11),
		ROUGE2:             fp(0.12),
		ROUGEL:             fp(0.13),
		BLEU1:              fp(0.21),
		BLEU2:              fp(0.22),
		BLEU4:              fp(0.24),
		LLMJudgeScore:      fp(4.2),
		SemanticSimilarity: fp(0.88),
		Faithfulness:       fp(0.83),
		ContextRelevance:   fp(0.71),
		NoiseRatio:         fp(0.25),
	}

	m := buildTaskMetrics(r)
	if m == nil {
		t.Fatal("buildTaskMetrics returned nil")
	}
	checks := map[string]struct{ got, want *float64 }{
		"precision":           {m.Precision, r.Precision},
		"recall":              {m.Recall, r.Recall},
		"ndcg":                {m.NDCG, r.NDCG},
		"mrr":                 {m.MRR, r.MRR},
		"map":                 {m.MAP, r.MAP},
		"llm_judge_score":     {m.LLMJudgeScore, r.LLMJudgeScore},
		"semantic_similarity": {m.SemanticSimilarity, r.SemanticSimilarity},
		"faithfulness":        {m.Faithfulness, r.Faithfulness},
		"context_relevance":   {m.ContextRelevance, r.ContextRelevance},
		"noise_ratio":         {m.NoiseRatio, r.NoiseRatio},
	}
	for name, c := range checks {
		if c.got == nil || c.want == nil || *c.got != *c.want {
			t.Errorf("%s: got %v, want %v", name, c.got, c.want)
		}
	}

	// nil 入参应返回 nil
	if buildTaskMetrics(nil) != nil {
		t.Error("buildTaskMetrics(nil) 应返回 nil")
	}
}

// TestConvertQAResultsToApp_CarriesRetrievedPIDs 验证 domain→app 转换保留 RetrievedPIDs。
func TestConvertQAResultsToApp_CarriesRetrievedPIDs(t *testing.T) {
	domainResults := []*domeval.QAResult{
		{
			Question:        "Q",
			RetrievedChunks: []string{"正文A", "正文B"},
			RetrievedPIDs:   []string{"pid-A", "pid-B"},
			RelevantPIDs:    []string{"pid-A"},
		},
	}

	app := convertQAResultsToApp(domainResults)
	if len(app) != 1 {
		t.Fatalf("expected 1 app result, got %d", len(app))
	}
	if !reflect.DeepEqual(app[0].RetrievedPIDs, []string{"pid-A", "pid-B"}) {
		t.Errorf("RetrievedPIDs 未透传：got %v", app[0].RetrievedPIDs)
	}
	if !reflect.DeepEqual(app[0].RetrievedChunks, []string{"正文A", "正文B"}) {
		t.Errorf("RetrievedChunks 未透传：got %v", app[0].RetrievedChunks)
	}
}
