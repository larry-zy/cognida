package lifecycle

import (
	d "cognida/internal/model/lifecycle"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type CleaningChange struct {
	SourceRow  int      `json:"source_row"`
	Operations []string `json:"operations"`
	RemovedBy  string   `json:"removed_by"`
}
type CleaningReport struct {
	SourceRef        string          `json:"source_ref"`
	PlanDigest       string          `json:"plan_digest"`
	Diagnostics      json.RawMessage `json:"diagnostics"`
	SourceRowIndices []int           `json:"source_row_indices"`
}
type CleaningDifference struct {
	SourceRow  int      `json:"source_row"`
	OutputRow  *int     `json:"output_row,omitempty"`
	Before     []any    `json:"before"`
	After      []any    `json:"after,omitempty"`
	Operations []string `json:"operations"`
	RemovedBy  string   `json:"removed_by,omitempty"`
}
type CleaningDiffPage struct {
	Items      []CleaningDifference `json:"items"`
	NextCursor string               `json:"next_cursor"`
	Total      int                  `json:"total"`
	Columns    []string             `json:"columns"`
}
type CleaningPreview struct {
	SourceRef   string           `json:"source_ref"`
	PlanDigest  string           `json:"plan_digest"`
	Diagnostics json.RawMessage  `json:"diagnostics"`
	Differences CleaningDiffPage `json:"differences"`
	Saved       bool             `json:"saved"`
}

func cleanOptions(options map[string]any) (map[string]any, error) {
	for key := range options {
		switch key {
		case "operations", "columns", "keep", "min_score", "dimensions", "reference_text", "preview_digest", governedOption, strategyOption:
		default:
			return nil, fmt.Errorf("%w: unsupported cleaning option %s; custom code requires sandbox", d.ErrInvalid, key)
		}
	}
	out := map[string]any{"operations": []string{"trim", "dedup"}, "keep": "first"}
	for _, key := range []string{"operations", "columns", "keep"} {
		if v, ok := options[key]; ok {
			out[key] = v
		}
	}
	raw, _ := json.Marshal(out)
	var shape struct {
		Operations []string `json:"operations"`
		Columns    []string `json:"columns"`
		Keep       string   `json:"keep"`
	}
	if json.Unmarshal(raw, &shape) != nil || len(shape.Operations) < 1 || len(shape.Operations) > 12 || len(shape.Columns) > 200 || shape.Keep != "first" && shape.Keep != "last" {
		return nil, d.ErrInvalid
	}
	allowed := map[string]bool{"trim": true, "normalize_ws": true, "drop_empty": true, "dedup": true, "text_cleaner": true, "pii_masker": true}
	for _, op := range shape.Operations {
		if !allowed[op] {
			return nil, d.ErrInvalid
		}
	}
	return out, nil
}
func cleaningDigest(ref d.DataAssetRef, options map[string]any) string {
	copy := map[string]any{}
	for k, v := range options {
		if k != "preview_digest" && k != strategyOption {
			copy[k] = v
		}
	}
	raw, _ := json.Marshal(struct {
		Source, Digest string
		Options        map[string]any
	}{ref.RefID.String(), ref.Provenance.Digest, copy})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func cleaningReport(ref d.DataAssetRef, options map[string]any, result ComputeResult) (CleaningReport, error) {
	if result.Result["diagnostics"] == nil {
		return CleaningReport{}, fmt.Errorf("cleaning diagnostics unavailable")
	}
	out := CleaningReport{SourceRef: ref.RefID.String(), PlanDigest: cleaningDigest(ref, options)}
	var e error
	out.Diagnostics, e = json.Marshal(result.Result["diagnostics"])
	if e != nil {
		return out, e
	}
	raw, e := json.Marshal(result.Result["source_row_indices"])
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(raw, &out.SourceRowIndices)
	return out, e
}
func cleaningRows(in d.Input) ([]string, [][]any) {
	if in.Kind == "table" {
		return in.Columns, in.Rows
	}
	text := strings.ReplaceAll(in.Text, "\r\n", "\n")
	for _, delimiter := range []string{"\r", "\v", "\f", "\x1c", "\x1d", "\x1e", "\u0085", "\u2028", "\u2029"} {
		text = strings.ReplaceAll(text, delimiter, "\n")
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	rows := make([][]any, len(lines))
	for i, s := range lines {
		rows[i] = []any{s}
	}
	return []string{"text"}, rows
}
func cleaningDiff(before, after d.Input, report CleaningReport, cursor string, limit int) (CleaningDiffPage, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	start := 0
	if cursor != "" {
		var e error
		start, e = strconv.Atoi(cursor)
		if e != nil || start < 0 {
			return CleaningDiffPage{}, d.ErrInvalid
		}
	}
	var diagnostics struct {
		Changes []CleaningChange `json:"row_changes"`
	}
	if json.Unmarshal(report.Diagnostics, &diagnostics) != nil || len(diagnostics.Changes) > 10000 {
		return CleaningDiffPage{}, d.ErrInvalid
	}
	columns, original := cleaningRows(before)
	_, cleaned := cleaningRows(after)
	if after.Kind == "text" && after.Text == "" && len(report.SourceRowIndices) == 0 {
		cleaned = nil
	}
	if len(cleaned) != len(report.SourceRowIndices) {
		return CleaningDiffPage{}, d.ErrConflict
	}
	mapping := map[int]int{}
	for i, index := range report.SourceRowIndices {
		if index < 0 || index >= len(original) {
			return CleaningDiffPage{}, d.ErrConflict
		}
		if _, ok := mapping[index]; ok {
			return CleaningDiffPage{}, d.ErrConflict
		}
		mapping[index] = i
	}
	out := CleaningDiffPage{Items: []CleaningDifference{}, Total: len(diagnostics.Changes), Columns: columns}
	if start > out.Total {
		return out, d.ErrInvalid
	}
	end := start + limit
	if end > out.Total {
		end = out.Total
	}
	for _, change := range diagnostics.Changes[start:end] {
		if change.SourceRow < 0 || change.SourceRow >= len(original) {
			return out, d.ErrConflict
		}
		item := CleaningDifference{SourceRow: change.SourceRow, Before: original[change.SourceRow], Operations: change.Operations, RemovedBy: change.RemovedBy}
		if i, ok := mapping[change.SourceRow]; ok {
			item.OutputRow = &i
			item.After = cleaned[i]
		}
		out.Items = append(out.Items, item)
	}
	if end < out.Total {
		out.NextCursor = strconv.Itoa(end)
	}
	return out, nil
}
func (s *Service) PreviewCleaning(ctx context.Context, p d.Principal, id d.RefID, options map[string]any, cursor string) (CleaningPreview, error) {
	var out CleaningPreview
	if e := s.authorize(ctx, p, "read"); e != nil {
		return out, e
	}
	if e := validateExpertOptions("data-cleaning", options); e != nil {
		return out, e
	}
	options, strategyVersion, e := s.previewStrategyOptions(ctx, p, options)
	if e != nil {
		return out, e
	}
	opts, e := cleanOptions(options)
	if e != nil {
		return out, e
	}
	ref, raw, e := s.Resolve(ctx, p.TenantID, p.UserID, id)
	if e != nil {
		return out, e
	}
	var input d.Input
	if json.Unmarshal(raw, &input) != nil || input.Validate() != nil {
		return out, d.ErrInvalid
	}
	if s.config.Compute == nil {
		return out, fmt.Errorf("cleaning compute unavailable")
	}
	result, e := s.config.Compute.Call(ctx, "clean", input, opts, "cleaning-preview")
	if e != nil {
		return out, e
	}
	body, _ := json.Marshal(result.Result["data"])
	var after d.Input
	if json.Unmarshal(body, &after) != nil {
		return out, d.ErrInvalid
	}
	report, e := cleaningReport(ref, options, result)
	if e != nil {
		return out, e
	}
	diff, e := cleaningDiff(input, after, report, cursor, 50)
	if e != nil {
		return out, e
	}
	if _, _, e = s.Resolve(ctx, p.TenantID, p.UserID, id); e != nil {
		return out, e
	}
	if strategyVersion != nil {
		current, e := s.strategies.Active(ctx, p, "cleaning.default")
		if e != nil {
			return out, e
		}
		if current.ID != strategyVersion.ID {
			return out, d.ErrConflict
		}
	}
	return CleaningPreview{SourceRef: id.String(), PlanDigest: report.PlanDigest, Diagnostics: report.Diagnostics, Differences: diff}, nil
}
func (s *Service) CleaningDifferences(ctx context.Context, p d.Principal, id d.RefID, cursor string) (CleaningDiffPage, error) {
	ref, raw, e := s.Resolve(ctx, p.TenantID, p.UserID, id)
	if e != nil {
		return CleaningDiffPage{}, e
	}
	if ref.Kind != d.CleanedAsset {
		return CleaningDiffPage{}, d.ErrInvalid
	}
	var stored struct {
		Report CleaningReport `json:"cleaning_report"`
	}
	if json.Unmarshal(raw, &stored) != nil || stored.Report.SourceRef == "" {
		return CleaningDiffPage{}, d.ErrNotFound
	}
	source, e := d.ParseRefID(stored.Report.SourceRef)
	if e != nil {
		return CleaningDiffPage{}, d.ErrInvalid
	}
	bound := false
	for _, parent := range ref.Parents {
		if parent == source {
			bound = true
		}
	}
	if !bound {
		return CleaningDiffPage{}, d.ErrForbidden
	}
	_, original, e := s.Resolve(ctx, p.TenantID, p.UserID, source)
	if e != nil {
		return CleaningDiffPage{}, e
	}
	var before, after d.Input
	if json.Unmarshal(original, &before) != nil || json.Unmarshal(raw, &after) != nil {
		return CleaningDiffPage{}, d.ErrInvalid
	}
	return cleaningDiff(before, after, stored.Report, cursor, 50)
}

// CleaningComputeOptions is the same closed normalization used by real lifecycle
// cleaning. Policy quality gates and internal receipts never reach Python clean.
func CleaningComputeOptions(options map[string]any) (map[string]any, error) {
	return cleanOptions(options)
}
func DefaultCleaningEvaluationOptions() json.RawMessage {
	opts, _ := effectiveCleaningOptions(nil)
	raw, _ := json.Marshal(opts)
	return raw
}
