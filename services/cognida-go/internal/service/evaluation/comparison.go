package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"

	d "cognida/internal/model/evaluation"
)

type ComparisonRequest struct {
	BaselineID     string  `json:"baseline_id"`
	CandidateID    string  `json:"candidate_id"`
	Metric         string  `json:"metric"`
	HigherIsBetter bool    `json:"higher_is_better"`
	MinPairs       int     `json:"min_pairs"`
	MaxRegression  float64 `json:"max_regression"`
}
type Comparison struct {
	BaselineID     string   `json:"baseline_id"`
	CandidateID    string   `json:"candidate_id"`
	Metric         string   `json:"metric"`
	Pairs          int      `json:"pairs"`
	MissingPairs   int      `json:"missing_pairs"`
	BaselineMean   float64  `json:"baseline_mean"`
	CandidateMean  float64  `json:"candidate_mean"`
	Improvement    float64  `json:"improvement"`
	Lower95        *float64 `json:"lower_95"`
	Upper95        *float64 `json:"upper_95"`
	Gate           string   `json:"gate"`
	Recommendation string   `json:"recommendation"`
	Method         string   `json:"method"`
	Limitations    []string `json:"limitations"`
}

func (s *Service) Compare(ctx context.Context, tenant, user int64, in ComparisonRequest) (*Comparison, error) {
	if tenant <= 0 || user <= 0 || in.BaselineID == "" || in.BaselineID == in.CandidateID || in.Metric == "" || len(in.Metric) > 100 || in.MinPairs < 0 || in.MinPairs > 10000 || in.MaxRegression < 0 || math.IsNaN(in.MaxRegression) || math.IsInf(in.MaxRegression, 0) {
		return nil, d.ErrInvalidConfig
	}
	tasks := []*d.EvaluationTask{}
	for _, id := range []string{in.BaselineID, in.CandidateID} {
		task, e := s.taskRepo.FindByID(ctx, id)
		if e != nil {
			return nil, e
		}
		if task == nil || task.TenantID != tenant || task.UserID != user || task.DeletedAt != nil {
			return nil, d.ErrTaskNotFound
		}
		if task.Status != d.TaskStatusCompleted {
			return nil, fmt.Errorf("%w: both tasks must be completed", d.ErrInvalidConfig)
		}
		tasks = append(tasks, task)
	}
	if tasks[0].DatasetID != tasks[1].DatasetID || tasks[0].Type.Normalize() != tasks[1].Type.Normalize() {
		return nil, fmt.Errorf("%w: dataset and evaluation type must match", d.ErrInvalidConfig)
	}
	sets := [][]*d.EvaluationResult{}
	for _, task := range tasks {
		rows := []*d.EvaluationResult{}
		cursor := ""
		for {
			page, next, e := s.resultRepo.FindByTaskIDByCursor(ctx, task.ID, cursor, 200)
			if e != nil {
				return nil, e
			}
			rows = append(rows, page...)
			if len(rows) > 10000 {
				return nil, fmt.Errorf("%w: comparison exceeds 10000 cases", d.ErrInvalidConfig)
			}
			if next == "" {
				break
			}
			cursor = next
		}
		sets = append(sets, rows)
	}
	return compareResults(in, sets[0], sets[1])
}
func resultKey(r *d.EvaluationResult) string {
	body, _ := json.Marshal([]string{r.Question, r.ReferenceAnswer})
	return fmt.Sprintf("%x", sha256.Sum256(body))
}
func resultMetric(r *d.EvaluationResult, name string) (float64, bool) {
	if name == "success_rate" {
		if r.Success {
			return 1, true
		}
		return 0, true
	}
	if !r.Success {
		return 0, false
	}
	if value, ok := r.Scores[name]; ok {
		return value, !math.IsNaN(value) && !math.IsInf(value, 0)
	}
	switch name {
	case "precision", "recall", "ndcg", "rr", "rouge_1", "rouge_2", "rouge_l", "bleu_1", "bleu_2", "bleu_4", "llm_score", "semantic_similarity":
	default:
		return 0, false
	}
	body, _ := json.Marshal(r)
	values := map[string]any{}
	_ = json.Unmarshal(body, &values)
	value, ok := values[name].(float64)
	return value, ok && !math.IsNaN(value) && !math.IsInf(value, 0)
}
func compareResults(in ComparisonRequest, baseline, candidate []*d.EvaluationResult) (*Comparison, error) {
	if in.MinPairs == 0 {
		in.MinPairs = 20
	}
	out := &Comparison{BaselineID: in.BaselineID, CandidateID: in.CandidateID, Metric: in.Metric, Method: "paired_mean_delta_normal_95_v1", Gate: "insufficient_evidence", Limitations: []string{"按问题和参考答案配对；仅在相同数据集、评分器与运行条件下解释增益。", "区间为配对均值的正态近似；小样本或相关样本不能证明统计显著性。", "结果不自动修改生产配置；推广和回滚均需人工审核。"}}
	b := map[string]*d.EvaluationResult{}
	a := map[string]*d.EvaluationResult{}
	for _, set := range []struct {
		rows  []*d.EvaluationResult
		index map[string]*d.EvaluationResult
	}{{baseline, b}, {candidate, a}} {
		for _, r := range set.rows {
			key := resultKey(r)
			if set.index[key] != nil {
				return nil, fmt.Errorf("%w: duplicate question/reference pairs", d.ErrInvalidConfig)
			}
			set.index[key] = r
		}
	}
	deltas := []float64{}
	for key, base := range b {
		next := a[key]
		if next == nil {
			out.MissingPairs++
			continue
		}
		left, lok := resultMetric(base, in.Metric)
		right, rok := resultMetric(next, in.Metric)
		if !lok || !rok {
			out.MissingPairs++
			continue
		}
		out.Pairs++
		out.BaselineMean += left
		out.CandidateMean += right
		delta := right - left
		if !in.HigherIsBetter {
			delta = -delta
		}
		deltas = append(deltas, delta)
	}
	for key := range a {
		if b[key] == nil {
			out.MissingPairs++
		}
	}
	out.Recommendation = "保留基线，补齐配对样本后再评估。"
	if out.Pairs == 0 {
		return out, nil
	}
	out.BaselineMean /= float64(out.Pairs)
	out.CandidateMean /= float64(out.Pairs)
	for _, delta := range deltas {
		out.Improvement += delta
	}
	out.Improvement /= float64(out.Pairs)
	if out.Pairs > 1 {
		variance := 0.0
		for _, delta := range deltas {
			variance += math.Pow(delta-out.Improvement, 2)
		}
		se := math.Sqrt(variance / float64(out.Pairs-1) / float64(out.Pairs))
		lower, upper := out.Improvement-1.96*se, out.Improvement+1.96*se
		out.Lower95 = &lower
		out.Upper95 = &upper
	}
	if out.MissingPairs == 0 && out.Pairs >= in.MinPairs && out.Lower95 != nil {
		if *out.Upper95 < -in.MaxRegression {
			out.Gate = "regression"
			out.Recommendation = "候选存在退化证据：保持或恢复基线配置。"
		} else if *out.Lower95 >= -in.MaxRegression && out.Improvement > 0 {
			out.Gate = "eligible_for_review"
			out.Recommendation = "候选通过本次增益门槛，可进入人工审核和限量灰度。"
		} else {
			out.Gate = "inconclusive"
			out.Recommendation = "尚无明确增益，保持基线并扩大评测样本。"
		}
	}
	return out, nil
}
