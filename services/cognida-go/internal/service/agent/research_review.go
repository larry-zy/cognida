package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"reflect"
	"strings"

	"cognida/internal/infrastructure/llm/chat"
)

var ErrResearchSupplementIncomplete = errors.New("optional research evidence review incomplete")

type researchSupplementError struct{ cause error }

func (e *researchSupplementError) Error() string { return ErrResearchSupplementIncomplete.Error() }
func (e *researchSupplementError) Unwrap() error { return e.cause }
func (e *researchSupplementError) Is(target error) bool {
	return target == ErrResearchSupplementIncomplete
}

// ResearchReviewSnapshot preserves the exact scope of a completed review. The
// digest includes source excerpts and their then-current independence groups.
// Failed model responses never become snapshots or overwrite completed reviews.
type ResearchReviewSnapshot struct {
	Round        int               `json:"round"`
	SourceIDs    []string          `json:"source_ids"`
	SourceDigest string            `json:"source_digest"`
	SourceGroups map[string]string `json:"source_groups"`
	Claims       []ResearchClaim   `json:"claims"`
}

func validateResearchClaims(claims, expected []ResearchClaim, sources []ReportSourceDTO) ([]ResearchClaim, error) {
	fail := func(code string) ([]ResearchClaim, error) { return nil, &researchValidationError{code: code} }
	if len(claims) != len(expected) || len(claims) < researchMinFindings || len(claims) > researchMaxFindings {
		return fail("review_claim_count")
	}
	byID := map[string]ResearchClaim{}
	for _, c := range expected {
		if c.ID == "" || strings.TrimSpace(c.Statement) == "" {
			return fail("review_claim_identity")
		}
		if _, exists := byID[c.ID]; exists {
			return fail("review_claim_identity")
		}
		byID[c.ID] = c
	}
	bySource := map[string]ReportSourceDTO{}
	for _, source := range sources {
		bySource[source.ID] = source
	}
	out := cloneResearchClaims(claims)
	seen := map[string]bool{}
	for i := range out {
		c := &out[i]
		original, ok := byID[c.ID]
		if !ok || seen[c.ID] {
			return fail("review_claim_identity")
		}
		seen[c.ID] = true
		if strings.TrimSpace(c.Reason) == "" || len([]rune(c.Reason)) > 2000 {
			return fail("review_reason_bounds")
		}
		if len(c.Evidence) > 8 {
			return fail("review_evidence_bounds")
		}
		c.Statement = original.Statement
		if c.Status != "supported" && c.Status != "conflict" && c.Status != "insufficient" {
			return fail("review_status_invalid")
		}
		support, conflict := false, false
		for j := range c.Evidence {
			e := &c.Evidence[j]
			source, ok := bySource[e.SourceID]
			if !ok {
				return fail("review_source_unknown")
			}
			if len([]rune(e.Quote)) < 8 {
				return fail("review_quote_bounds")
			}
			if !strings.Contains(source.Excerpt, e.Quote) {
				return fail("review_quote_mismatch")
			}
			if e.Relation != "supports" && e.Relation != "contradicts" && e.Relation != "context" {
				return fail("review_relation_invalid")
			}
			e.QuoteMatched = true
			support = support || e.Relation == "supports"
			conflict = conflict || e.Relation == "contradicts"
		}
		if (c.Status == "supported" && (!support || conflict)) || (c.Status == "conflict" && !conflict) {
			return fail("review_status_evidence_conflict")
		}
	}
	refreshResearchClaimGroups(out, sources)
	return out, nil
}

func cloneResearchClaims(claims []ResearchClaim) []ResearchClaim {
	out := append([]ResearchClaim(nil), claims...)
	for i := range out {
		out[i].Evidence = append([]ResearchClaimEvidence(nil), claims[i].Evidence...)
	}
	return out
}

func refreshResearchClaimGroups(claims []ResearchClaim, sources []ReportSourceDTO) {
	byID := map[string]string{}
	for _, source := range sources {
		byID[source.ID] = source.IndependenceGroup
	}
	for i := range claims {
		groups := map[string]bool{}
		for _, e := range claims[i].Evidence {
			if group := byID[e.SourceID]; group != "" && e.Relation != "context" {
				groups[group] = true
			}
		}
		claims[i].IndependentSources = len(groups)
	}
}

func snapshotResearchReview(round int, sources []ReportSourceDTO, claims []ResearchClaim) ResearchReviewSnapshot {
	snapshot := ResearchReviewSnapshot{Round: round, SourceGroups: map[string]string{}, Claims: cloneResearchClaims(claims)}
	for _, source := range sources {
		snapshot.SourceIDs = append(snapshot.SourceIDs, source.ID)
		snapshot.SourceGroups[source.ID] = source.IndependenceGroup
	}
	body, _ := json.Marshal(sources)
	digest := sha256.Sum256(body)
	snapshot.SourceDigest = hex.EncodeToString(digest[:])
	return snapshot
}

func unreviewedResearchSources(sources []ReportSourceDTO, reviewed []string) []string {
	known := map[string]bool{}
	for _, id := range reviewed {
		known[id] = true
	}
	var pending []string
	for _, source := range sources {
		if !known[source.ID] {
			pending = append(pending, source.ID)
		}
	}
	return pending
}

func recoverableSupplementFailure(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var validation *researchValidationError
	var output *researchOutputError
	var status *chat.HTTPStatusError
	var network net.Error
	// Only known failures from the optional model call are recoverable. Generic
	// errors and authorization failures do not become usable partial reports.
	return errors.As(err, &validation) || errors.As(err, &output) ||
		(errors.As(err, &status) && (status.StatusCode == 402 || status.StatusCode == 429 || status.StatusCode >= 500 && status.StatusCode <= 599)) ||
		errors.As(err, &network) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

func refreshResearchUnverified(report *DeepResearchResponse) {
	report.Unverified = nil
	for _, claim := range report.Claims {
		if claim.Status != "supported" {
			report.Unverified = append(report.Unverified, claim.Statement+": "+claim.Reason)
		}
	}
	if report.Metadata.SupplementalReviewIncomplete {
		report.Unverified = append(report.Unverified, "补充证据的语义审查未完成；新增来源可能包含反证，不能以首轮结论替代完整审查。")
	}
}

func retainResearchPartial(report *DeepResearchResponse, cause error) {
	report.Metadata.SupplementalReviewIncomplete = true
	report.Metadata.Partial = true
	report.Metadata.Failure = researchFailure("supplement_reviewing", cause)
	markResearchClaimsPending(report)
	report.Warnings = append(report.Warnings, "补充审查失败（"+report.Metadata.Failure.Code+"）；仅保留已完成的首轮审查快照，当前报告为部分结果，不是完整成功。")
}

func markResearchClaimsPending(report *DeepResearchResponse) {
	for i := range report.Claims {
		if report.Claims[i].Status == "supported" {
			report.Claims[i].Status = "insufficient"
			report.Claims[i].Reason = "补充审查未完成，首轮支持意见仅见历史快照；新增证据可能改变结论。"
		}
	}
	refreshResearchClaimGroups(report.Claims, report.Sources)
	refreshResearchUnverified(report)
}

// UsableResearchPartial is a strict persistence gate, not permission to ignore
// an arbitrary provider error. The domain service must still reauthorize every
// current source and enforce its deadline/cancellation fence before publishing.
func UsableResearchPartial(report *DeepResearchResponse, err error) bool {
	if !errors.Is(err, ErrResearchSupplementIncomplete) || !recoverableSupplementFailure(err) || report == nil || report.Success ||
		!report.Metadata.Partial || !report.Metadata.CitationChecked || !report.Metadata.SemanticReview || !report.Metadata.SupplementalReviewIncomplete ||
		report.Metadata.ReviewRounds != 1 || len(report.ReviewHistory) != 1 || report.Metadata.Failure == nil || report.Metadata.Failure.Stage != "supplement_reviewing" ||
		len(report.Sources) == 0 || len(report.Sources) > 30 || len(report.Claims) == 0 {
		return false
	}
	snapshot := report.ReviewHistory[0]
	if snapshot.Round != 1 || len(snapshot.SourceIDs) == 0 || len(snapshot.SourceGroups) != len(snapshot.SourceIDs) || !reflect.DeepEqual(snapshot.SourceIDs, report.Metadata.ReviewedSourceIDs) || !reflect.DeepEqual(report.Metadata.Failure, researchFailure("supplement_reviewing", err)) {
		return false
	}
	byID := map[string]ReportSourceDTO{}
	for _, source := range report.Sources {
		if source.ID == "" || source.Excerpt == "" {
			return false
		}
		if _, exists := byID[source.ID]; exists {
			return false
		}
		byID[source.ID] = source
	}
	var reviewed []ReportSourceDTO
	seen := map[string]bool{}
	for _, id := range snapshot.SourceIDs {
		source, ok := byID[id]
		if !ok || seen[id] || snapshot.SourceGroups[id] == "" {
			return false
		}
		seen[id] = true
		source.IndependenceGroup = snapshot.SourceGroups[id]
		reviewed = append(reviewed, source)
	}
	if snapshotResearchReview(1, reviewed, nil).SourceDigest != snapshot.SourceDigest || !reflect.DeepEqual(unreviewedResearchSources(report.Sources, snapshot.SourceIDs), report.Metadata.UnreviewedSourceIDs) {
		return false
	}
	validated, validationErr := validateResearchClaims(snapshot.Claims, report.Claims, reviewed)
	if validationErr != nil || !reflect.DeepEqual(validated, snapshot.Claims) {
		return false
	}
	for _, claim := range report.Claims {
		if claim.Status == "supported" {
			return false
		}
	}
	current, validationErr := validateResearchClaims(report.Claims, snapshot.Claims, report.Sources)
	if validationErr != nil || !reflect.DeepEqual(current, report.Claims) {
		return false
	}
	expected := &DeepResearchResponse{Claims: cloneResearchClaims(snapshot.Claims), Sources: report.Sources}
	markResearchClaimsPending(expected)
	if !reflect.DeepEqual(expected.Claims, report.Claims) {
		return false
	}
	// Revalidate the original writer's citations as well as the review snapshot.
	draft := researchDraft{Summary: report.ExecutiveSummary, Analysis: report.DetailedAnalysis}
	byStatement := map[string][]researchCitation{}
	for _, check := range report.CitationChecks {
		if !check.QuoteMatched {
			return false
		}
		byStatement[check.Statement] = append(byStatement[check.Statement], researchCitation{SourceID: check.SourceID, Quote: check.Quote})
	}
	for _, claim := range snapshot.Claims {
		draft.Findings = append(draft.Findings, researchFinding{Statement: claim.Statement, Citations: byStatement[claim.Statement]})
	}
	return validateResearchDraft(&draft, report.Sources) == nil
}
