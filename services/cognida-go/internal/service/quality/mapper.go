package quality

import (
	"context"
	"encoding/json"
	"log"

	qualitypb "cognida/api/proto/quality"
	domain_quality "cognida/internal/model/quality"
)

// severityText 把 proto 严重级别枚举转为可读字符串。
func severityText(s qualitypb.SeverityLevel) string {
	switch s {
	case qualitypb.SeverityLevel_CRITICAL:
		return "critical"
	case qualitypb.SeverityLevel_WARNING:
		return "warning"
	case qualitypb.SeverityLevel_INFO:
		return "info"
	default:
		return "unknown"
	}
}

func mapIssue(p *qualitypb.QualityIssue) Issue {
	return Issue{
		Severity:    severityText(p.GetSeverity()),
		Dimension:   p.GetDimension(),
		Field:       p.GetField(),
		Description: p.GetDescription(),
		Count:       p.GetCount(),
		Sample:      p.GetSample(),
	}
}

func mapStructuredReport(p *qualitypb.QualityReport) *StructuredReport {
	if p == nil {
		return &StructuredReport{Dimensions: []DimensionScore{}, Issues: []Issue{}}
	}
	dims := make([]DimensionScore, 0, len(p.GetDimensions()))
	for _, d := range p.GetDimensions() {
		issues := make([]Issue, 0, len(d.GetIssues()))
		for _, iss := range d.GetIssues() {
			issues = append(issues, mapIssue(iss))
		}
		dims = append(dims, DimensionScore{
			Name:   d.GetName(),
			Score:  d.GetScore(),
			Passed: d.GetPassed(),
			Issues: issues,
		})
	}
	issues := make([]Issue, 0, len(p.GetIssues()))
	for _, iss := range p.GetIssues() {
		issues = append(issues, mapIssue(iss))
	}
	return &StructuredReport{
		OverallScore: p.GetOverallScore(),
		RecordCount:  p.GetRecordCount(),
		Dimensions:   dims,
		Issues:       issues,
		Metadata:     p.GetMetadata(),
	}
}

func mapUnstructuredReport(p *qualitypb.UnstructuredQualityReport) *UnstructuredReport {
	if p == nil {
		return &UnstructuredReport{Dimensions: []UnstructuredDimensionScore{}, Issues: []TextIssue{}}
	}
	dims := make([]UnstructuredDimensionScore, 0, len(p.GetDimensions()))
	for _, d := range p.GetDimensions() {
		dims = append(dims, UnstructuredDimensionScore{
			Name:   d.GetName(),
			Score:  d.GetScore(),
			Passed: d.GetPassed(),
		})
	}
	issues := make([]TextIssue, 0, len(p.GetIssues()))
	for _, iss := range p.GetIssues() {
		issues = append(issues, TextIssue{
			Type:        iss.GetType(),
			Severity:    severityText(iss.GetSeverity()),
			Description: iss.GetDescription(),
			Snippet:     iss.GetSnippet(),
			Suggestion:  iss.GetSuggestion(),
		})
	}
	return &UnstructuredReport{
		OverallScore: p.GetOverallScore(),
		Dimensions:   dims,
		TextStats:    p.GetTextStats(),
		Issues:       issues,
	}
}

func mapCleaningReport(p *qualitypb.CleaningResult) *CleaningReport {
	if p == nil {
		return &CleaningReport{Operations: []CleaningOperation{}}
	}
	ops := make([]CleaningOperation, 0, len(p.GetOperations()))
	for _, op := range p.GetOperations() {
		ops = append(ops, CleaningOperation{
			Type:        op.GetType(),
			Field:       op.GetField(),
			Count:       op.GetCount(),
			Description: op.GetDescription(),
		})
	}
	return &CleaningReport{
		OriginalCount: p.GetOriginalCount(),
		CleanedCount:  p.GetCleanedCount(),
		RemovedCount:  p.GetRemovedCount(),
		Operations:    ops,
	}
}

// persist 把一次质检/清洗结果写入历史记录。落库失败不影响主流程（仅告警）。
func (s *Service) persist(ctx context.Context, tenantID, userID int64, t domain_quality.CheckType, sourceName string, score float64, recordCount int32, summary string, report interface{}) {
	if s.repo == nil {
		return
	}
	raw, err := json.Marshal(report)
	if err != nil {
		log.Printf("[quality] 序列化报告失败: %v", err)
		raw = nil
	}
	record := &domain_quality.CheckRecord{
		ID:           s.idGen.Generate(),
		TenantID:     tenantID,
		UserID:       userID,
		Type:         t,
		SourceName:   sourceName,
		OverallScore: score,
		RecordCount:  recordCount,
		Summary:      summary,
		ReportJSON:   raw,
	}
	if err := s.repo.Create(ctx, record); err != nil {
		log.Printf("[quality] 质检记录落库失败: %v", err)
	}
}
