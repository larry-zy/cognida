package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	d "cognida/internal/model/lifecycle"
)

// AnalyzeGoal preserves reproducible statistics and adds explicitly unverified
// model interpretation only when the caller supplies an actual analysis goal.
func AnalyzeGoal(ctx context.Context, compute Compute, input d.Input, prompt, sourceRef, requestID string) (map[string]any, error) {
	stats, err := AnalyzeInput(input)
	if err != nil {
		return nil, err
	}
	prompt = strings.TrimSpace(prompt)
	if utf8.RuneCountInString(prompt) > 4000 {
		return nil, fmt.Errorf("%w: analysis goal exceeds 4000 characters", d.ErrInvalid)
	}
	stats["source_ref"] = sourceRef
	if prompt == "" {
		return stats, nil
	}
	if compute == nil {
		return nil, fmt.Errorf("goal-directed analysis compute is unavailable")
	}
	result, err := compute.Call(ctx, "analyze", input, map[string]any{"prompt": prompt, "source_ref": sourceRef, "statistics": stats}, requestID)
	if err != nil {
		return nil, err
	}
	narrative, _ := result.Result["narrative"].(string)
	if strings.TrimSpace(narrative) == "" || result.Result["verification_status"] != "citations_checked_interpretation_unverified" {
		return nil, fmt.Errorf("analysis compute returned an invalid interpretation")
	}
	// Do not let model output replace deterministic numeric statistics.
	for _, key := range []string{"narrative", "findings", "evidence", "limitations", "verification_status", "model"} {
		stats[key] = result.Result[key]
	}
	stats["method"] = result.Method
	stats["prompt"] = prompt
	return stats, nil
}

// stageAnalysisCompute reuses the durable compute checkpoint on worker retries.
type stageAnalysisCompute struct {
	service *Service
	request d.StageRequest
}

func (c stageAnalysisCompute) Call(ctx context.Context, operation string, input d.Input, options map[string]any, _ string) (ComputeResult, error) {
	return c.service.compute(ctx, operation, c.request, input, options)
}
