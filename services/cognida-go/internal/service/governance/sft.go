package governance

import (
	"bytes"
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"strconv"
	"strings"
	"unicode/utf8"
)

func sftDecode(raw string, v any) error {
	if strings.TrimSpace(raw) == "null" || len(raw) > 1<<20 || strictFeatureJSON([]byte(raw)) != nil {
		return d.ErrInvalid
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if dec.Decode(v) != nil {
		return d.ErrInvalid
	}
	return nil
}
func validateSFTMessages(messages []d.SFTMessage, tools []d.SFTTool) error {
	if len(messages) < 2 || len(messages) > 100 || len(tools) > 30 {
		return d.ErrInvalid
	}
	toolNames := map[string]*sftSchema{}
	for _, t := range tools {
		if t.Type != "function" || t.Function.Name == "" || len(t.Function.Name) > 128 || toolNames[t.Function.Name] != nil || len(t.Function.Description) > 4000 {
			return d.ErrInvalid
		}
		schema, err := compileSFTSchema(t.Function.Parameters)
		if err != nil {
			return err
		}
		toolNames[t.Function.Name] = schema
	}
	pending, used := map[string]bool{}, map[string]bool{}
	users := 0
	last := ""
	for i, m := range messages {
		if m.Content != nil && utf8.RuneCountInString(*m.Content) > 100000 {
			return d.ErrInvalid
		}
		content := m.Content != nil && strings.TrimSpace(*m.Content) != ""
		switch m.Role {
		case "system":
			if i != 0 || !content || len(m.ToolCalls) > 0 || m.ToolCallID != "" {
				return d.ErrInvalid
			}
		case "user":
			if len(pending) > 0 || !content || len(m.ToolCalls) > 0 || m.ToolCallID != "" || (last != "" && last != "system" && last != "assistant") {
				return d.ErrInvalid
			}
			users++
		case "assistant":
			if users == 0 || len(pending) > 0 || m.ToolCallID != "" || (!content && len(m.ToolCalls) == 0) || len(m.ToolCalls) > 30 || (last != "user" && last != "tool") {
				return d.ErrInvalid
			}
			for _, call := range m.ToolCalls {
				var args map[string]any
				if call.ID == "" || len(call.ID) > 128 || used[call.ID] || call.Type != "function" || toolNames[call.Function.Name] == nil || sftDecode(call.Function.Arguments, &args) != nil || args == nil {
					return d.ErrInvalid
				}
				nodes := 0
				if !boundedSFTArguments(args, 0, &nodes) || !toolNames[call.Function.Name].validate(args) {
					return sftArgumentsError()
				}
				pending[call.ID] = true
				used[call.ID] = true
			}
		case "tool":
			if !pending[m.ToolCallID] || !content || len(m.ToolCalls) > 0 {
				return d.ErrInvalid
			}
			delete(pending, m.ToolCallID)
		default:
			return d.ErrInvalid
		}
		last = m.Role
	}
	if users == 0 || len(pending) > 0 || last != "assistant" || messages[len(messages)-1].Content == nil || strings.TrimSpace(*messages[len(messages)-1].Content) == "" {
		return d.ErrInvalid
	}
	return nil
}
func buildSFTRows(in d.SFTInput, input l.Input) ([]d.SFTRow, map[string]int, error) {
	if strings.TrimSpace(in.Name) == "" || utf8.RuneCountInString(in.Name) > 128 || input.Kind != "table" || len(input.Rows) < 1 || len(input.Rows) > 1000 {
		return nil, nil, d.ErrInvalid
	}
	indexes := map[string]int{}
	for i, c := range input.Columns {
		indexes[c] = i
	}
	c := in.Columns
	if in.Format == "qa" {
		if c.Question == "" || c.ReferenceAnswer == "" || c.Question == c.ReferenceAnswer || c.Messages != "" || c.Tools != "" {
			return nil, nil, d.ErrInvalid
		}
	} else if in.Format == "messages" {
		if c.Messages == "" || c.Question != "" || c.ReferenceAnswer != "" {
			return nil, nil, d.ErrInvalid
		}
	} else {
		return nil, nil, d.ErrInvalid
	}
	for _, col := range []string{c.Question, c.ReferenceAnswer, c.Messages, c.Tools, c.Split, c.Group} {
		if col != "" {
			if _, ok := indexes[col]; !ok {
				return nil, nil, d.ErrInvalid
			}
		}
	}
	out := []d.SFTRow{}
	counts := map[string]int{"train": 0, "validation": 0, "test": 0}
	prompts, groups := map[string]string{}, map[string]string{}
	for i, values := range input.Rows {
		cell := func(col string) (string, error) {
			idx, ok := indexes[col]
			if !ok || idx >= len(values) {
				return "", d.ErrInvalid
			}
			v, ok := values[idx].(string)
			if !ok {
				return "", d.ErrInvalid
			}
			return v, nil
		}
		row := d.SFTRow{RowID: fmt.Sprintf("row-%06d", i+1), SourceRowIndex: i, Split: "train"}
		if c.Split != "" {
			v, e := cell(c.Split)
			if e != nil {
				return nil, nil, e
			}
			if v != "train" && v != "validation" && v != "test" {
				return nil, nil, d.ErrInvalid
			}
			row.Split = v
		}
		if c.Group != "" {
			v, e := cell(c.Group)
			if e != nil || strings.TrimSpace(v) == "" || len(v) > 512 {
				return nil, nil, d.ErrInvalid
			}
			row.Group = v
			if split, ok := groups[v]; ok && split != row.Split {
				return nil, nil, fmt.Errorf("%w: group crosses dataset splits", d.ErrInvalid)
			}
			groups[v] = row.Split
		}
		if in.Format == "qa" {
			q, e := cell(c.Question)
			if e != nil {
				return nil, nil, e
			}
			a, e := cell(c.ReferenceAnswer)
			if e != nil {
				return nil, nil, e
			}
			row.Messages = []d.SFTMessage{{Role: "user", Content: &q}, {Role: "assistant", Content: &a}}
		} else {
			raw, e := cell(c.Messages)
			if e != nil || exactSFTKeys(raw, false) != nil || sftDecode(raw, &row.Messages) != nil {
				return nil, nil, d.ErrInvalid
			}
			if c.Tools != "" {
				raw, e = cell(c.Tools)
				if e != nil || exactSFTKeys(raw, true) != nil || sftDecode(raw, &row.Tools) != nil {
					return nil, nil, d.ErrInvalid
				}
			}
		}
		if e := validateSFTMessages(row.Messages, row.Tools); e != nil {
			return nil, nil, e
		}
		// Holdout isolation keys only inputs, never answers. Different answers must
		// not hide a duplicate prompt leaking between training and evaluation.
		questions := []string{}
		for _, m := range row.Messages {
			if m.Role == "user" {
				questions = append(questions, strings.TrimSpace(*m.Content))
			}
		}
		key := d.LabelContentDigest(string(mustLabelJSON(questions)))
		if split, ok := prompts[key]; ok {
			if split != row.Split {
				return nil, nil, fmt.Errorf("%w: duplicate input crosses dataset splits", d.ErrInvalid)
			}
			return nil, nil, fmt.Errorf("%w: duplicate input within dataset split", d.ErrInvalid)
		}
		prompts[key] = row.Split
		counts[row.Split]++
		out = append(out, row)
	}
	if len(mustLabelJSON(out)) > 6<<20 {
		return nil, nil, d.ErrInvalid
	}
	return out, counts, nil
}
func (s *Service) CreateSFTDataset(ctx context.Context, p l.Principal, in d.SFTInput, key string) (*d.Record, error) {
	if !validFeatureKey(key) {
		return nil, d.ErrInvalid
	}
	standard, e := s.GetLabelStandardVersion(ctx, p, in.StandardVersionID)
	if e != nil {
		return nil, e
	}
	if standard.TaskType != "qa" {
		return nil, d.ErrInvalid
	}
	ref, input, e := s.preparedInput(ctx, p, in.SourceRefID)
	if e != nil {
		return nil, e
	}
	rows, counts, e := buildSFTRows(in, input)
	if e != nil {
		return nil, e
	}
	// Store the immutable row snapshot as a JSON string. MySQL JSON objects
	// reorder raw schema keys and can round schema numeric bounds; neither may
	// change the content that was validated and approved by the user.
	exactRows := string(mustLabelJSON(rows))
	value := d.SFTDataset{SFTInput: in, SourceDigest: ref.Provenance.Digest, StandardDigest: standard.ContentDigest, RowsExact: exactRows, RowCount: len(rows), SplitCounts: counts, ContentDigest: d.LabelContentDigest(exactRows)}
	r, e := s.create(ctx, p, d.SFTDatasetKind, "", "draft", key, value, nil)
	if e != nil {
		return nil, e
	}
	var old d.SFTDataset
	if json.Unmarshal(r.Payload, &old) != nil {
		return nil, d.ErrInvalid
	}
	if !d.JSONEqual(mustLabelJSON(in), mustLabelJSON(old.SFTInput)) || old.SourceDigest != value.SourceDigest || old.StandardDigest != value.StandardDigest || old.ContentDigest != value.ContentDigest {
		return nil, d.ErrConflict
	}
	return r, nil
}
func (s *Service) authorizeSFT(ctx context.Context, p l.Principal, r *d.Record) error {
	if r.UserID != p.UserID {
		return d.ErrNotFound
	}
	var value d.SFTDataset
	if json.Unmarshal(r.Payload, &value) != nil {
		return d.ErrInvalid
	}
	ref, _, e := s.preparedInput(ctx, p, value.SourceRefID)
	if e != nil {
		return recordSourceError(e)
	}
	if ref.Provenance.Digest != value.SourceDigest {
		return d.ErrForbidden
	}
	standard, e := s.GetLabelStandardVersion(ctx, p, value.StandardVersionID)
	if e != nil {
		return e
	}
	if standard.TaskType != "qa" || standard.ContentDigest != value.StandardDigest {
		return d.ErrForbidden
	}
	if _, e = exactSFTRows(value); e != nil {
		return e
	}
	return nil
}

func exactSFTRows(value d.SFTDataset) ([]d.SFTRow, error) {
	rows := value.Rows
	raw := string(mustLabelJSON(rows))
	if value.RowsExact != "" {
		if len(rows) != 0 || len(value.RowsExact) > 6<<20 {
			return nil, d.ErrForbidden
		}
		raw = value.RowsExact
		if json.Unmarshal([]byte(raw), &rows) != nil {
			return nil, d.ErrForbidden
		}
	}
	if len(rows) < 1 || len(rows) > 1000 || len(rows) != value.RowCount || value.ContentDigest != d.LabelContentDigest(raw) {
		return nil, d.ErrForbidden
	}
	return rows, nil
}
func (s *Service) ReviewSFTDataset(ctx context.Context, p l.Principal, id, decision, comment string, expected int) (*d.Record, error) {
	if e := s.authorize(ctx, p, "admin"); e != nil {
		return nil, e
	}
	if decision != "approved" && decision != "rejected" || utf8.RuneCountInString(comment) > 2000 {
		return nil, d.ErrInvalid
	}
	r, e := s.Get(ctx, p, d.SFTDatasetKind, id)
	if e != nil {
		return nil, e
	}
	var v d.SFTDataset
	if json.Unmarshal(r.Payload, &v) != nil {
		return nil, d.ErrInvalid
	}
	v.ReviewedBy = p.UserID
	v.ReviewComment = comment
	if e = s.update(ctx, r, v, decision, expected); e != nil {
		return nil, e
	}
	return r, nil
}
func (s *Service) PublishSFTDataset(ctx context.Context, p l.Principal, id string, expected int, key string) (*d.Record, error) {
	if e := s.authorize(ctx, p, "admin"); e != nil {
		return nil, e
	}
	if !validFeatureKey(key) || expected < 1 {
		return nil, d.ErrInvalid
	}
	// Authorization is checked even for idempotent replays.
	if _, e := s.Get(ctx, p, d.SFTDatasetKind, id); e != nil {
		return nil, e
	}
	versionID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("sft:%d:%d:%s:%d", p.TenantID, p.UserID, id, expected))).String()
	var result *d.Record
	e := s.Repo.Transaction(ctx, func(tx d.Repository) error {
		boundKey := fmt.Sprintf("%d:%s", p.UserID, key)
		if r, err := tx.Get(ctx, p.TenantID, d.SFTDatasetVersionKind, versionID); err == nil {
			if r.UserID != p.UserID || r.IdempotencyKey != boundKey {
				return d.ErrConflict
			}
			result = r
			return nil
		} else if !errors.Is(err, d.ErrNotFound) {
			return err
		}
		head, err := tx.Get(ctx, p.TenantID, d.SFTDatasetKind, id)
		if err != nil {
			return err
		}
		if head.UserID != p.UserID {
			return d.ErrNotFound
		}
		if head.Version != expected || head.Status != "approved" {
			return d.ErrConflict
		}
		var v d.SFTDataset
		if json.Unmarshal(head.Payload, &v) != nil || v.ReviewedBy == 0 {
			return d.ErrInvalid
		}
		copy := d.SFTDatasetVersion{SFTDataset: v, DatasetID: id, DatasetRevision: expected}
		copy.LatestVersionID = ""
		created, err := tx.Create(ctx, &d.Record{Meta: d.Meta{ID: versionID, TenantID: p.TenantID, UserID: p.UserID}, Kind: d.SFTDatasetVersionKind, ParentID: id, Status: "published", IdempotencyKey: boundKey, Payload: mustLabelJSON(copy)})
		if err != nil {
			return err
		}
		if created.ID != versionID || !d.JSONEqual(created.Payload, mustLabelJSON(copy)) {
			return d.ErrConflict
		}
		v.LatestVersionID = versionID
		head.Payload = mustLabelJSON(v)
		if err = tx.Update(ctx, head, expected); err != nil {
			return err
		}
		result = created
		return nil
	})
	return result, e
}
func (s *Service) SFTDatasetRows(ctx context.Context, p l.Principal, id, cursor string, limit int) (*d.Page[d.SFTRow], error) {
	r, e := s.Get(ctx, p, d.SFTDatasetKind, id)
	if e != nil {
		return nil, e
	}
	var v d.SFTDataset
	if json.Unmarshal(r.Payload, &v) != nil {
		return nil, d.ErrInvalid
	}
	rows, e := exactSFTRows(v)
	if e != nil {
		return nil, e
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	start := 0
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return nil, d.ErrInvalid
		}
		parts := strings.Split(string(raw), ":")
		if len(parts) != 3 || parts[0] != r.ID || parts[1] != v.ContentDigest {
			return nil, d.ErrInvalid
		}
		start, err = strconv.Atoi(parts[2])
		if err != nil || start < 0 || start > len(rows) {
			return nil, d.ErrInvalid
		}
	}
	end := min(start+limit, len(rows))
	out := &d.Page[d.SFTRow]{Items: rows[start:end], HasMore: end < len(rows)}
	if out.HasMore {
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s:%d", r.ID, v.ContentDigest, end)))
	}
	return out, nil
}
func (s *Service) ExportSFTDataset(ctx context.Context, p l.Principal, id string) ([]byte, error) {
	r, e := s.Get(ctx, p, d.SFTDatasetVersionKind, id)
	if e != nil {
		return nil, e
	}
	if r.Status != "published" {
		return nil, d.ErrConflict
	}
	var v d.SFTDatasetVersion
	if json.Unmarshal(r.Payload, &v) != nil {
		return nil, d.ErrInvalid
	}
	rows, e := exactSFTRows(v.SFTDataset)
	if e != nil {
		return nil, e
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	for _, row := range rows {
		if e = enc.Encode(struct {
			Messages []d.SFTMessage `json:"messages"`
			Tools    []d.SFTTool    `json:"tools,omitempty"`
			Split    string         `json:"split"`
		}{row.Messages, row.Tools, row.Split}); e != nil {
			return nil, e
		}
	}
	return out.Bytes(), nil
}

// Validate wire keys before encoding/json's case-insensitive struct matching.
// Original messages must be unambiguous rather than silently rewritten.
func exactSFTKeys(raw string, tools bool) error {
	object := func(raw json.RawMessage, allowed string) (map[string]json.RawMessage, error) {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return nil, d.ErrInvalid
		}
		keys := map[string]bool{}
		for _, key := range strings.Fields(allowed) {
			keys[key] = true
		}
		for key := range fields {
			if !keys[key] {
				return nil, d.ErrInvalid
			}
		}
		return fields, nil
	}
	var items []json.RawMessage
	if json.Unmarshal([]byte(raw), &items) != nil || items == nil {
		return d.ErrInvalid
	}
	for _, item := range items {
		if tools {
			fields, e := object(item, "type function")
			if e != nil {
				return e
			}
			if _, e = object(fields["function"], "name description parameters"); e != nil {
				return e
			}
			continue
		}
		fields, e := object(item, "role content tool_calls tool_call_id")
		if e != nil {
			return e
		}
		if calls, ok := fields["tool_calls"]; ok && string(calls) != "null" {
			var values []json.RawMessage
			if json.Unmarshal(calls, &values) != nil {
				return d.ErrInvalid
			}
			for _, call := range values {
				fields, e = object(call, "id type function")
				if e != nil {
					return e
				}
				if _, e = object(fields["function"], "name arguments"); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
