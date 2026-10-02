package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	agentctx "cognida/internal/model/agent"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/service/agent/resultstore"
)

// DecisionAssetResolver preserves tenant/member/sensitivity checks at the same
// immutable-reference boundary used by lifecycle HTTP reads.
type DecisionAssetResolver interface {
	Resolve(context.Context, int64, int64, l.RefID) (l.DataAssetRef, json.RawMessage, error)
}

type VerifyResultsRequest struct {
	LeftResultID      string   `json:"left_result_id" jsonschema:"required"`
	RightResultID     string   `json:"right_result_id" jsonschema:"required"`
	KeyColumns        []string `json:"key_columns,omitempty"`
	ValueColumns      []string `json:"value_columns" jsonschema:"required"`
	AbsoluteTolerance float64  `json:"absolute_tolerance,omitempty"`
	RelativeTolerance float64  `json:"relative_tolerance,omitempty"`
}
type ResultDifference struct {
	Key    string `json:"key"`
	Column string `json:"column,omitempty"`
	Reason string `json:"reason"`
}
type DecisionCard struct {
	EvidenceRefs       []string `json:"evidence_refs"`
	Method             string   `json:"method"`
	Assumptions        []string `json:"assumptions"`
	Risks              []string `json:"risks"`
	VerificationStatus string   `json:"verification_status"`
	Confidence         *float64 `json:"confidence"`
	Recommendation     string   `json:"recommendation"`
}
type VerifyResultsResult struct {
	Decision              DecisionCard       `json:"decision"`
	ComparedCells         int                `json:"compared_cells"`
	DifferenceCount       int                `json:"difference_count"`
	Differences           []ResultDifference `json:"differences"`
	RequiresClarification bool               `json:"requires_clarification"`
}

func NewVerifyResultsTool(store resultstore.Store) *TypedBaseTool[VerifyResultsRequest, VerifyResultsResult] {
	return NewTypedBaseTool("verify_results", "Compare two explicitly supplied same-session result references by keys and selected numeric columns with absolute/relative tolerance. This checks agreement only, not independent provenance or business correctness. Never runs SQL.", func(ctx context.Context, req *VerifyResultsRequest) (*VerifyResultsResult, error) {
		return verifyResults(ctx, store, req)
	})
}
func verifyResults(ctx context.Context, store resultstore.Store, req *VerifyResultsRequest) (*VerifyResultsResult, error) {
	tenant, session := agentctx.MustGetTenantID(ctx), agentctx.MustGetSessionID(ctx)
	if tenant <= 0 || session == "" || store == nil {
		return nil, fmt.Errorf("authorized session and result store required")
	}
	if req.LeftResultID == "" || req.RightResultID == "" || req.LeftResultID == req.RightResultID || len(req.ValueColumns) == 0 || len(req.ValueColumns) > 100 || len(req.KeyColumns) > 20 || !finite(req.AbsoluteTolerance) || !finite(req.RelativeTolerance) || req.AbsoluteTolerance < 0 || req.RelativeTolerance < 0 || req.RelativeTolerance > 1 {
		return nil, fmt.Errorf("invalid comparison inputs or tolerance")
	}
	owner := resultstore.OwnerKey(tenant, session)
	get := store.Get
	if exact, ok := store.(interface {
		GetExact(context.Context, string, string) (*resultstore.Result, error)
	}); ok {
		get = exact.GetExact
	}
	left, e := get(ctx, owner, req.LeftResultID)
	if e != nil {
		return nil, e
	}
	right, e := get(ctx, owner, req.RightResultID)
	if e != nil {
		return nil, e
	}
	if (len(left.Rows)+len(right.Rows))*len(req.ValueColumns) > 200000 {
		return nil, fmt.Errorf("comparison exceeds 200000 cells")
	}
	for _, r := range []*resultstore.Result{left, right} {
		cols := map[string]bool{}
		for _, v := range r.Columns {
			cols[v] = true
		}
		seen := map[string]bool{}
		for _, col := range append(append([]string{}, req.KeyColumns...), req.ValueColumns...) {
			if col == "" || !cols[col] || seen[col] {
				return nil, fmt.Errorf("missing or duplicate comparison column %q", col)
			}
			seen[col] = true
		}
	}
	index := func(r *resultstore.Result) (map[string]map[string]any, []string, error) {
		out := map[string]map[string]any{}
		order := []string{}
		for i, row := range r.Rows {
			key := strconv.Itoa(i)
			if len(req.KeyColumns) > 0 {
				values := make([]any, len(req.KeyColumns))
				for j, col := range req.KeyColumns {
					v, ok := row[col]
					if !ok || v == nil {
						return nil, nil, fmt.Errorf("null/missing key column %s", col)
					}
					switch v.(type) {
					case string, bool:
						values[j] = v
					default:
						n, ok := comparisonNumber(v)
						if !ok {
							return nil, nil, fmt.Errorf("key must be an exact scalar")
						}
						values[j] = map[string]string{"number": n.RatString()}
					}
				}
				b, err := json.Marshal(values)
				if err != nil {
					return nil, nil, err
				}
				key = string(b)
			}
			if _, ok := out[key]; ok {
				return nil, nil, fmt.Errorf("duplicate comparison key")
			}
			out[key] = row
			order = append(order, key)
		}
		return out, order, nil
	}
	lm, order, e := index(left)
	if e != nil {
		return nil, e
	}
	rm, rightOrder, e := index(right)
	if e != nil {
		return nil, e
	}
	out := &VerifyResultsResult{Decision: DecisionCard{EvidenceRefs: []string{req.LeftResultID, req.RightResultID}, Method: "deterministic_keyed_numeric_comparison", Assumptions: []string{"Both paths use the same period, filters, aggregation, units and null semantics; caller must confirm this caliber.", "Result references do not prove independent execution paths."}, Risks: []string{"Agreement can preserve shared upstream errors; this is not causal or semantic validation."}, VerificationStatus: "numeric_agreement_only", Recommendation: "Confirm the metric definition and independent provenance before acting."}, Differences: []ResultDifference{}}
	if len(req.KeyColumns) == 0 {
		out.Decision.Assumptions = append(out.Decision.Assumptions, "Rows are paired by position because no key columns were supplied.")
	}
	add := func(key, col, reason string) {
		out.DifferenceCount++
		if len(out.Differences) < 100 {
			if len(key) > 256 {
				key = key[:256]
			}
			out.Differences = append(out.Differences, ResultDifference{key, col, reason})
		}
	}
	for _, key := range order {
		a := lm[key]
		b, ok := rm[key]
		if !ok {
			add(key, "", "missing_right_row")
			continue
		}
		for _, col := range req.ValueColumns {
			out.ComparedCells++
			av, aok := a[col]
			bv, bok := b[col]
			if !aok || !bok || av == nil || bv == nil {
				add(key, col, "missing_or_null_value")
				continue
			}
			x, xok := comparisonNumber(av)
			y, yok := comparisonNumber(bv)
			if !xok || !yok {
				add(key, col, "non_numeric_or_unsafe_precision")
				continue
			}
			diff := new(big.Rat).Abs(new(big.Rat).Sub(x, y))
			scale := new(big.Rat).Abs(x)
			if yy := new(big.Rat).Abs(y); yy.Cmp(scale) > 0 {
				scale = yy
			}
			absTol, _ := decimalNumber(strconv.FormatFloat(req.AbsoluteTolerance, 'g', -1, 64))
			relTol, _ := decimalNumber(strconv.FormatFloat(req.RelativeTolerance, 'g', -1, 64))
			relativeBound := new(big.Rat).Mul(scale, relTol)
			if diff.Cmp(absTol) > 0 && diff.Cmp(relativeBound) > 0 {
				add(key, col, "numeric_mismatch")
			}
		}
	}
	for _, key := range rightOrder {
		if _, ok := lm[key]; !ok {
			add(key, "", "missing_left_row")
		}
	}
	if out.DifferenceCount > 0 || out.ComparedCells == 0 {
		out.RequiresClarification = true
		out.Decision.VerificationStatus = "requires_clarification"
		out.Decision.Recommendation = "Reconcile missing values, units, time windows, filters and source paths before making a decision."
		out.Decision.Risks = append(out.Decision.Risks, "Numeric support is inconsistent or absent.")
		zero := 0.0
		out.Decision.Confidence = &zero
	}
	return out, nil
}
func finite(v float64) bool { return !math.IsInf(v, 0) && !math.IsNaN(v) }

var decimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

func decimalNumber(raw string) (*big.Rat, bool) {
	if len(raw) == 0 || len(raw) > 128 || !decimalPattern.MatchString(raw) {
		return nil, false
	}
	if at := strings.IndexAny(raw, "eE"); at >= 0 {
		exponent, e := strconv.Atoi(raw[at+1:])
		if e != nil || exponent < -512 || exponent > 512 {
			return nil, false
		}
	}
	// #nosec G113 -- Decimal grammar excludes binary/hex exponents; length <=128 and decimal exponent bounded to [-512,512] above.
	n, ok := new(big.Rat).SetString(raw)
	return n, ok
}
func comparisonNumber(v any) (*big.Rat, bool) {
	if n, ok := v.(json.Number); ok {
		return decimalNumber(string(n))
	}
	rv := reflect.ValueOf(v)
	var raw string
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		raw = strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		raw = strconv.FormatUint(rv.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		x := rv.Float()
		if !finite(x) || math.Abs(x) >= 1<<53 {
			return nil, false
		}
		raw = strconv.FormatFloat(x, 'g', -1, 64)
	default:
		return nil, false
	}
	return decimalNumber(raw)
}

type DecisionRequest struct {
	EvidenceRefIDs []string              `json:"evidence_ref_ids" jsonschema:"required"`
	Goal           string                `json:"goal" jsonschema:"required"`
	Assumptions    []string              `json:"assumptions,omitempty"`
	Risks          []string              `json:"risks,omitempty"`
	Actions        []string              `json:"actions,omitempty"`
	KnowledgeQuery string                `json:"knowledge_query,omitempty"`
	Verification   *VerifyResultsRequest `json:"verification,omitempty"`
}
type DecisionResult struct {
	Decision        DecisionCard         `json:"decision"`
	Goal            string               `json:"goal"`
	Insights        []json.RawMessage    `json:"insights"`
	Knowledge       []DocumentChunk      `json:"knowledge"`
	ProposedActions []string             `json:"proposed_actions"`
	ActionStatus    string               `json:"action_status"`
	Verification    *VerifyResultsResult `json:"verification,omitempty"`
}

func NewDecisionTool(assets DecisionAssetResolver, store resultstore.Store, rag RAGQueryService) *TypedBaseTool[DecisionRequest, DecisionResult] {
	return NewTypedBaseTool("decision_synthesize", "Compose a reviewable decision card from authorized immutable Insight refs, optionally retrieve real selected-KB evidence and recompute a two-result numeric check. Proposed actions are hypotheses, never verified causal conclusions or automatic writes.", func(ctx context.Context, req *DecisionRequest) (*DecisionResult, error) {
		return synthesizeDecision(ctx, assets, store, rag, req)
	})
}
func synthesizeDecision(ctx context.Context, assets DecisionAssetResolver, store resultstore.Store, rag RAGQueryService, req *DecisionRequest) (*DecisionResult, error) {
	tenant, user := agentctx.MustGetTenantID(ctx), agentctx.MustGetUserID(ctx)
	if assets == nil || tenant <= 0 || user <= 0 || len(req.EvidenceRefIDs) == 0 || len(req.EvidenceRefIDs) > 10 || strings.TrimSpace(req.Goal) == "" || len(req.Goal) > 4000 || len(req.KnowledgeQuery) > 4000 {
		return nil, fmt.Errorf("authorized insight evidence and bounded goal required")
	}
	for _, list := range [][]string{req.Assumptions, req.Risks, req.Actions} {
		if len(list) > 20 {
			return nil, fmt.Errorf("too many decision statements")
		}
		for _, v := range list {
			if len(v) > 4000 {
				return nil, fmt.Errorf("decision statement too long")
			}
		}
	}
	out := &DecisionResult{Goal: req.Goal, Insights: []json.RawMessage{}, Knowledge: []DocumentChunk{}, ProposedActions: req.Actions, ActionStatus: "proposed_requires_human_review", Decision: DecisionCard{EvidenceRefs: append([]string{}, req.EvidenceRefIDs...), Method: "authorized_insight_and_retrieved_knowledge_synthesis", Assumptions: append([]string{"Caller-proposed actions, assumptions and risks are hypotheses, not conclusions inferred or verified by this tool."}, req.Assumptions...), Risks: append([]string{"Descriptive data and retrieved text do not establish causality or predict action outcomes."}, req.Risks...), VerificationStatus: "evidence_resolved_actions_unverified", Recommendation: "Review the cited evidence and validate each action in a bounded experiment before adoption."}}
	total := 0
	for _, raw := range req.EvidenceRefIDs {
		ref, e := l.ParseRefID(raw)
		if e != nil {
			return nil, e
		}
		meta, body, e := assets.Resolve(ctx, tenant, user, ref)
		if e != nil {
			return nil, e
		}
		if meta.Kind != l.Insight {
			return nil, fmt.Errorf("decision evidence must be an insight ref")
		}
		total += len(body)
		if total > 256<<10 {
			return nil, fmt.Errorf("insight evidence exceeds output budget")
		}
		out.Insights = append(out.Insights, body)
	}
	if req.Verification != nil {
		v, e := verifyResults(ctx, store, req.Verification)
		if e != nil {
			return nil, e
		}
		out.Verification = v
		out.Decision.EvidenceRefs = append(out.Decision.EvidenceRefs, v.Decision.EvidenceRefs...)
		out.Decision.Assumptions = append(out.Decision.Assumptions, v.Decision.Assumptions...)
		out.Decision.Assumptions = append(out.Decision.Assumptions, "Numeric verification covers only the supplied result references; their linkage to these insights has not been verified.")
		if v.RequiresClarification {
			out.Decision.VerificationStatus = "requires_clarification"
			out.Decision.Confidence = v.Decision.Confidence
			out.Decision.Recommendation = v.Decision.Recommendation
			out.ActionStatus = "blocked_pending_clarification"
		} else {
			out.Decision.VerificationStatus = "numeric_agreement_actions_unverified"
		}
	}
	if strings.TrimSpace(req.KnowledgeQuery) != "" {
		r, e := ragQuery(ctx, &RAGQueryRequest{Query: req.KnowledgeQuery, TopK: 5}, rag)
		if e != nil {
			return nil, e
		}
		if r == nil || len(r.Chunks) == 0 {
			out.Decision.Risks = append(out.Decision.Risks, "No supporting knowledge was retrieved; no domain support is claimed.")
		} else {
			for i, chunk := range r.Chunks {
				if i >= 5 {
					break
				}
				if len(chunk.Content) > 8000 {
					chunk.Content = chunk.Content[:8000]
				}
				out.Knowledge = append(out.Knowledge, chunk)
				out.Decision.EvidenceRefs = append(out.Decision.EvidenceRefs, fmt.Sprintf("kb:%s:document:%d:chunk:%d", r.KnowledgeBaseID, chunk.DocumentID, chunk.ChunkIndex))
			}
			out.Decision.Risks = append(out.Decision.Risks, "Retrieved passages are source evidence; their relevance to proposed actions remains subject to review.")
		}
	}
	return out, nil
}
func (r *ToolRegistry) registerDecisionTools() error {
	if e := r.Register("analytics", NewVerifyResultsTool(r.deps.ResultStore)); e != nil {
		return e
	}
	return r.Register("analytics", NewDecisionTool(r.deps.DecisionAssets, r.deps.ResultStore, r.deps.RAGService))
}
