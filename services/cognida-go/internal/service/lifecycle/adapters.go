package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	d "cognida/internal/model/lifecycle"
)

func (s *Service) executeAdapter(ctx context.Context, r d.StageRequest) (d.Output, error) {
	switch r.Stage.AgentID {
	case "data-access":
		ref, payload, input, e := dataInput(r)
		if e != nil {
			return d.Output{}, e
		}
		if e = input.Validate(); e != nil {
			return d.Output{}, e
		}
		out, e := s.ImportDerived(ctx, r.Principal, d.RawAsset, r.Stage.ID, []d.RefID{ref.RefID}, payload, ref.Policy)
		return passedOutput("Previously acquired input validated", out), e
	case "data-quality":
		return s.quality(ctx, r)
	case "data-cleaning":
		return s.clean(ctx, r)
	case "data-enrichment":
		return s.enrich(ctx, r)
	case "data-asset":
		return s.publish(ctx, r)
	case "data-analysis":
		return s.analyze(ctx, r)
	case "data-evaluation":
		return s.evaluate(ctx, r)
	}
	return d.Output{}, d.ErrInvalid
}
func dataInput(r d.StageRequest) (d.DataAssetRef, json.RawMessage, d.Input, error) {
	for i, ref := range r.Inputs {
		if ref.Kind == d.RawAsset || ref.Kind == d.CleanedAsset || ref.Kind == d.EnrichedAsset || ref.Kind == d.PublishedAsset {
			var input d.Input
			if e := json.Unmarshal(r.Payloads[i], &input); e != nil {
				return ref, nil, input, e
			}
			if e := input.Validate(); e != nil {
				return ref, nil, input, e
			}
			return ref, r.Payloads[i], input, nil
		}
	}
	return d.DataAssetRef{}, nil, d.Input{}, fmt.Errorf("%w: data input required", d.ErrInvalid)
}
func passedOutput(summary string, refs ...d.DataAssetRef) d.Output {
	ids := []d.RefID{}
	for _, ref := range refs {
		ids = append(ids, ref.RefID)
	}
	return d.Output{Refs: refs, Summary: summary, Gate: d.GateResult{Status: d.GatePassed, Reason: summary, EvidenceRefs: ids}}
}
func (s *Service) compute(ctx context.Context, op string, r d.StageRequest, input d.Input, opts map[string]any) (ComputeResult, error) {
	producer := r.Stage.ID + ":compute:" + op
	id := derivedRefID(r.Principal.TenantID, d.Result, producer, r.Stage.InputRefs)
	_, body, e := s.Resolve(ctx, r.Principal.TenantID, r.Principal.UserID, id)
	if e == nil {
		var out ComputeResult
		e = json.Unmarshal(body, &out)
		return out, e
	}
	if !errors.Is(e, d.ErrNotFound) {
		return ComputeResult{}, e
	}
	if s.config.Compute == nil {
		return ComputeResult{}, fmt.Errorf("lifecycle compute service unavailable")
	}
	out, e := s.config.Compute.Call(ctx, op, input, opts, r.Stage.RequestID)
	if e != nil {
		return out, e
	}
	body, e = json.Marshal(out)
	if e != nil {
		return out, e
	}
	policy := d.PolicySnapshot{Sensitivity: "unknown", Scopes: []string{"read"}, Version: "1"}
	if len(r.Inputs) > 0 {
		policy = r.Inputs[0].Policy
	}
	_, e = s.ImportDerived(ctx, r.Principal, d.Result, producer, r.Stage.InputRefs, body, policy)
	return out, e
}

func threshold(options map[string]any) (float64, error) {
	v, ok := options["min_score"]
	if !ok {
		return 70, nil
	}
	n, ok := v.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 100 {
		return 0, fmt.Errorf("%w: min_score must be 0-100", d.ErrInvalid)
	}
	return n, nil
}
func (s *Service) profile(ctx context.Context, r d.StageRequest, ref d.DataAssetRef, input d.Input) (d.DataAssetRef, d.GateResult, error) {
	min, e := threshold(r.Stage.Options)
	if e != nil {
		return d.DataAssetRef{}, d.GateResult{}, e
	}
	options := map[string]any{}
	for _, k := range []string{"dimensions", "reference_text"} {
		if v, ok := r.Stage.Options[k]; ok {
			options[k] = v
		}
	}
	result, e := s.compute(ctx, "profile", r, input, options)
	if e != nil {
		return d.DataAssetRef{}, d.GateResult{}, e
	}
	score, ok := number(result.Result["overall_score"])
	if !ok || score < 0 || score > 100 || math.IsNaN(score) {
		return d.DataAssetRef{}, d.GateResult{}, fmt.Errorf("quality returned invalid score")
	}
	raw, e := json.Marshal(result)
	if e != nil {
		return d.DataAssetRef{}, d.GateResult{}, e
	}
	report, e := s.ImportDerived(ctx, r.Principal, d.QualityReport, r.Stage.ID, []d.RefID{ref.RefID}, raw, ref.Policy)
	gate := d.GateResult{Status: d.GatePassed, Reason: "quality threshold met", Score: &score, Threshold: &min, EvidenceRefs: []d.RefID{report.RefID}}
	if score < min {
		gate.Status = d.GateFailed
		gate.Reason = "quality score below threshold"
	}
	return report, gate, e
}
func (s *Service) quality(ctx context.Context, r d.StageRequest) (d.Output, error) {
	ref, _, input, e := dataInput(r)
	if e != nil {
		return d.Output{}, e
	}
	report, gate, e := s.profile(ctx, r, ref, input)
	return d.Output{Refs: []d.DataAssetRef{ref, report}, Summary: gate.Reason, Gate: gate}, e
}
func (s *Service) clean(ctx context.Context, r d.StageRequest) (d.Output, error) {
	ref, _, input, e := dataInput(r)
	if e != nil {
		return d.Output{}, e
	}
	opts := map[string]any{}
	for _, k := range []string{"operations", "columns", "keep"} {
		if v, ok := r.Stage.Options[k]; ok {
			opts[k] = v
		}
	}
	result, e := s.compute(ctx, "clean", r, input, opts)
	if e != nil {
		return d.Output{}, e
	}
	body, e := json.Marshal(result.Result["data"])
	if e != nil {
		return d.Output{}, e
	}
	var cleaned d.Input
	if e = json.Unmarshal(body, &cleaned); e != nil {
		return d.Output{}, e
	}
	if e = cleaned.Validate(); e != nil {
		return d.Output{Refs: []d.DataAssetRef{ref}, Summary: "Cleaning produced no usable data", Gate: d.GateResult{Status: d.GateQuarantined, Reason: "cleaned data is empty or violates input bounds", EvidenceRefs: []d.RefID{ref.RefID}}}, nil
	}
	out, e := s.ImportDerived(ctx, r.Principal, d.CleanedAsset, r.Stage.ID, []d.RefID{ref.RefID}, body, ref.Policy)
	if e != nil {
		return d.Output{}, e
	}
	report, gate, e := s.profile(ctx, r, out, cleaned)
	if gate.Status == d.GateFailed {
		gate.Status = d.GateQuarantined
		gate.Reason = "cleaned data still fails quality threshold"
	}
	return d.Output{Refs: []d.DataAssetRef{out, report}, Summary: "Created cleaned version and rechecked quality", Gate: gate, Metadata: map[string]any{"method": result.Method, "operations": result.Result["operations"], "removed_count": result.Result["removed_count"], "source_row_indices": result.Result["source_row_indices"]}}, e
}
func (s *Service) enrich(ctx context.Context, r d.StageRequest) (d.Output, error) {
	ref, payload, input, e := dataInput(r)
	if e != nil {
		return d.Output{}, e
	}
	opts := map[string]any{}
	for _, k := range []string{"method", "labels", "max_keywords", "max_summary_chars", "features"} {
		if v, ok := r.Stage.Options[k]; ok {
			opts[k] = v
		}
	}
	result, e := s.compute(ctx, "enrich", r, input, opts)
	if e != nil {
		return d.Output{}, e
	}
	var content map[string]json.RawMessage
	if e = json.Unmarshal(payload, &content); e != nil {
		return d.Output{}, e
	}
	content["enrichment"], e = json.Marshal(result)
	if e != nil {
		return d.Output{}, e
	}
	body, e := json.Marshal(content)
	if e != nil {
		return d.Output{}, e
	}
	out, e := s.ImportDerived(ctx, r.Principal, d.EnrichedAsset, r.Stage.ID, []d.RefID{ref.RefID}, body, ref.Policy)
	output := passedOutput("Metadata recommendations generated; publication policy unchanged", out)
	output.Metadata = map[string]any{"recommendations": result.Result, "method": result.Method, "publication_action": "recommendation_only"}
	return output, e
}
func (s *Service) publish(ctx context.Context, r d.StageRequest) (d.Output, error) {
	if !r.Stage.Approved {
		return d.Output{}, d.ErrForbidden
	}
	ref, payload, input, e := dataInput(r)
	if e != nil {
		return d.Output{}, e
	}
	if s.config.Publish == nil {
		return d.Output{}, fmt.Errorf("asset publication adapter unavailable")
	}
	if s.config.ValidatePublication != nil {
		if e = s.config.ValidatePublication(ctx, r.Principal, input, r.Stage.Options); e != nil {
			return d.Output{}, e
		}
	}
	receipt, e := s.config.Publish(ctx, r.Principal, ref, payload, r.Stage.Options)
	if e != nil {
		return d.Output{}, e
	}
	if len(receipt) == 0 {
		return d.Output{}, fmt.Errorf("publication adapter returned no durable receipt")
	}
	var content map[string]json.RawMessage
	if e = json.Unmarshal(payload, &content); e != nil {
		return d.Output{}, e
	}
	content["publication"], e = json.Marshal(receipt)
	if e != nil {
		return d.Output{}, e
	}
	body, e := json.Marshal(content)
	if e != nil {
		return d.Output{}, e
	}
	policy := ref.Policy
	policy.Approved = true
	out, e := s.ImportDerived(ctx, r.Principal, d.PublishedAsset, r.Stage.ID, []d.RefID{ref.RefID}, body, policy)
	output := passedOutput("Approved asset published", out)
	output.Metadata = receipt
	return output, e
}
func (s *Service) analyze(ctx context.Context, r d.StageRequest) (d.Output, error) {
	ref, _, input, e := dataInput(r)
	if e != nil {
		return d.Output{}, e
	}
	if ref.Kind != d.PublishedAsset || !ref.Policy.Approved {
		return d.Output{}, d.ErrForbidden
	}
	prompt, _ := r.Stage.Options["prompt"].(string)
	analysis, e := AnalyzeGoal(ctx, stageAnalysisCompute{s, r}, input, prompt, ref.AssetRefID, r.Stage.RequestID)
	if e != nil {
		return d.Output{}, e
	}
	analysis["source_ref"] = ref.AssetRefID
	decision := decisionFor([]d.RefID{ref.RefID}, "descriptive_statistics", "verified_computation")
	if strings.TrimSpace(prompt) != "" {
		decision = decisionFor([]d.RefID{ref.RefID}, "goal-directed-llm-v1", "citations_checked_interpretation_unverified")
	}
	analysis["decision"] = decision
	body, e := json.Marshal(analysis)
	if e != nil {
		return d.Output{}, e
	}
	out, e := s.ImportDerived(ctx, r.Principal, d.Insight, r.Stage.ID, []d.RefID{ref.RefID}, body, ref.Policy)
	result := passedOutput("Descriptive analysis completed with source evidence", out)
	result.Decision = decision
	return result, e
}

// AnalyzeInput performs bounded read-only descriptive statistics. It does not
// infer causality or claim predictive accuracy, and is shared by subscriptions.
func AnalyzeInput(input d.Input) (map[string]any, error) {
	if e := input.Validate(); e != nil {
		return nil, e
	}
	analysis := map[string]any{"method": "descriptive_statistics", "limitations": []string{"Descriptive observations only; no causal inference or predictive accuracy is claimed."}}
	if input.Kind == "text" {
		analysis["character_count"] = len([]rune(input.Text))
		analysis["word_count"] = len(strings.Fields(input.Text))
		summary := []rune(input.Text)
		if len(summary) > 500 {
			summary = summary[:500]
		}
		analysis["excerpt"] = string(summary)
	} else {
		analysis["record_count"] = len(input.Rows)
		columns := []map[string]any{}
		for index, name := range input.Columns {
			numeric := []float64{}
			missing := 0
			for _, row := range input.Rows {
				v := row[index]
				if v == nil || v == "" {
					missing++
					continue
				}
				if f, ok := number(v); ok && !math.IsNaN(f) && !math.IsInf(f, 0) {
					numeric = append(numeric, f)
				}
			}
			stats := map[string]any{"name": name, "missing_count": missing, "numeric_count": len(numeric)}
			if len(numeric) > 0 {
				sum, min, max := 0.0, numeric[0], numeric[0]
				for _, v := range numeric {
					sum += v / float64(len(numeric))
					if v < min {
						min = v
					}
					if v > max {
						max = v
					}
				}
				stats["mean"] = sum
				stats["min"] = min
				stats["max"] = max
			}
			columns = append(columns, stats)
		}
		analysis["columns"] = columns
	}
	return analysis, nil
}
func decisionFor(refs []d.RefID, method, status string) map[string]any {
	return map[string]any{"evidence_refs": refs, "method": method, "assumptions": []string{"Input snapshot is representative and source semantics have been checked by the data owner."}, "risks": []string{"Missing or stale inputs can change the result; descriptive statistics do not establish causality."}, "verification_status": status, "confidence": nil, "recommendation": "Review source definitions and freshness before taking action."}
}
func (s *Service) evaluate(ctx context.Context, r d.StageRequest) (d.Output, error) {
	if len(r.Inputs) == 0 {
		return d.Output{}, d.ErrInvalid
	}
	checks := []map[string]any{}
	parents := []d.RefID{}
	passed := true
	for _, ref := range r.Inputs {
		valid := ref.Provenance.Digest != "" && len(ref.Parents) > 0
		checks = append(checks, map[string]any{"asset_ref_id": ref.AssetRefID, "provenance_present": valid, "policy_approved": ref.Policy.Approved})
		passed = passed && valid
		parents = append(parents, ref.RefID)
	}
	body, _ := json.Marshal(map[string]any{"method": "artifact_integrity_and_provenance", "passed": passed, "checks": checks, "limitations": []string{"This evaluation verifies artifacts and evidence links; semantic or model accuracy requires a labelled evaluation dataset."}})
	out, e := s.ImportDerived(ctx, r.Principal, d.Evaluation, r.Stage.ID, parents, body, r.Inputs[0].Policy)
	result := passedOutput("Artifact integrity and evidence evaluation completed", out)
	result.Decision = decisionFor(parents, "artifact_integrity_and_provenance", "integrity_checked_semantics_unverified")
	if !passed {
		result.Gate.Status = d.GateFailed
		result.Gate.Reason = "missing source evidence"
	}
	return result, e
}

func number(value any) (float64, bool) {
	switch v := value.(type) {
	case json.Number:
		f, err := v.Float64()
		return f, err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	}
	return 0, false
}
