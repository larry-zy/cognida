package lifecycle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	d "cognida/internal/model/lifecycle"
)

func preparationDigest(v any) string {
	raw, _ := json.Marshal(v)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

func preparationOptions(options map[string]any) (d.PreparationOptions, error) {
	var out d.PreparationOptions
	clean := map[string]any{}
	for k, v := range options {
		if k != governedOption {
			clean[k] = v
		}
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		return out, d.ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil || len(out.Fields) > 200 {
		return out, d.ErrInvalid
	}
	if projection := out.TextProjection; projection != nil {
		if len(out.Fields) != 0 || !projectionField(projection.Column) || !projectionField(projection.TextField) || (projection.IDColumn != "" && (!projectionField(projection.IDColumn) || projection.IDColumn == projection.Column)) {
			return out, fmt.Errorf("%w: text projection requires explicit columns and cannot combine with field mapping", d.ErrInvalid)
		}
	}
	seen, names := map[string]bool{}, map[string]bool{}
	for _, f := range out.Fields {
		if strings.TrimSpace(f.Source) == "" || f.Name != strings.TrimSpace(f.Name) || f.Name == "" || utf8.RuneCountInString(f.Name) > 200 || seen[f.Source] || names[f.Name] {
			return out, d.ErrInvalid
		}
		seen[f.Source], names[f.Name] = true, true
		switch f.Type {
		case "any", "text", "number", "boolean", "json":
		default:
			return out, fmt.Errorf("%w: unsupported preparation field type", d.ErrInvalid)
		}
	}
	return out, nil
}

// StandardizeInput never guesses types, invents values or silently drops rows.
func StandardizeInput(input d.Input, opts d.PreparationOptions) (d.Input, error) {
	if input.Validate() != nil {
		return d.Input{}, d.ErrInvalid
	}
	raw, _ := json.Marshal(opts)
	var options map[string]any
	_ = json.Unmarshal(raw, &options)
	if _, err := preparationOptions(options); err != nil {
		return d.Input{}, err
	}
	normalize := func(text string) string {
		if opts.NormalizeNewlines {
			return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
		}
		return text
	}
	if input.Kind == "text" {
		if len(opts.Fields) != 0 || opts.TextProjection != nil {
			return d.Input{}, fmt.Errorf("%w: field mapping requires a table", d.ErrInvalid)
		}
		out := d.Input{Kind: "text", Text: normalize(input.Text)}
		return out, out.Validate()
	}
	if opts.TextProjection != nil {
		return projectKnowledgeText(input, *opts.TextProjection, normalize)
	}
	indices := map[string]int{}
	for i, name := range input.Columns {
		indices[name] = i
	}
	fields := opts.Fields
	if len(fields) == 0 {
		for _, name := range input.Columns {
			fields = append(fields, d.PreparationField{Source: name, Name: name, Type: "any"})
		}
	}
	out := d.Input{Kind: "table", Columns: []string{}, Rows: make([][]any, len(input.Rows)), ColumnTypes: map[string]string{}}
	for _, f := range fields {
		if _, ok := indices[f.Source]; !ok {
			return d.Input{}, fmt.Errorf("%w: source column %q does not exist", d.ErrInvalid, f.Source)
		}
		out.Columns = append(out.Columns, f.Name)
		if declared := input.ColumnTypes[f.Source]; declared != "" {
			out.ColumnTypes[f.Name] = declared
		}
	}
	for i, row := range input.Rows {
		out.Rows[i] = make([]any, len(fields))
		for j, field := range fields {
			value := row[indices[field.Source]]
			if text, ok := value.(string); ok {
				value = normalize(text)
			}
			missing := value == nil
			if text, ok := value.(string); ok {
				missing = strings.TrimSpace(text) == ""
			}
			if field.Required && missing {
				return d.Input{}, fmt.Errorf("%w: row %d field %q is required", d.ErrInvalid, i+1, field.Name)
			}
			if value != nil && !preparationType(value, field.Type) {
				return d.Input{}, fmt.Errorf("%w: row %d field %q does not match %s", d.ErrInvalid, i+1, field.Name, field.Type)
			}
			out.Rows[i][j] = value
		}
	}
	return out, out.Validate()
}

func preparationType(value any, kind string) bool {
	switch kind {
	case "any":
		return true
	case "text":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		switch value.(type) {
		case json.Number, float64, float32, int, int64, int32, uint, uint64, uint32:
			return true
		}
	case "json":
		text, ok := value.(string)
		if !ok {
			return false
		}
		decoder := json.NewDecoder(strings.NewReader(text))
		decoder.UseNumber()
		var parsed any
		return decoder.Decode(&parsed) == nil && decoder.Decode(&parsed) == io.EOF
	}
	return false
}

func (s *Service) prepare(ctx context.Context, request d.StageRequest) (d.Output, error) {
	source, _, input, err := dataInput(request)
	if err != nil {
		return d.Output{}, err
	}
	if source.Kind != d.CleanedAsset {
		return d.Output{}, fmt.Errorf("%w: complete data cleaning before standardization", d.ErrInvalid)
	}
	opts, err := preparationOptions(request.Stage.Options)
	if err != nil {
		return d.Output{}, err
	}
	out, err := StandardizeInput(input, opts)
	if err != nil {
		return d.Output{}, err
	}
	body, err := json.Marshal(out)
	if err != nil {
		return d.Output{}, err
	}
	rows := len(out.Rows)
	if out.Kind == "text" {
		rows = 1
	}
	proof := &d.Preparation{Version: 1, SourceRefID: source.RefID.String(), SourceDigest: source.Provenance.Digest, OutputDigest: preparationDigest(out), SchemaDigest: preparationDigest([]any{out.Kind, out.Columns, out.ColumnTypes, opts}), Rows: rows, Fields: opts.Fields, NormalizeNewlines: opts.NormalizeNewlines, TextProjection: opts.TextProjection}
	if _, _, err = s.Resolve(ctx, request.Principal.TenantID, request.Principal.UserID, source.RefID); err != nil {
		return d.Output{}, err
	}
	origin := d.Provenance{SourceType: "lifecycle", SourceID: request.Stage.ID, RequestID: request.Stage.RequestID, Preparation: proof}
	ref, err := s.importPayload(ctx, request.Principal, d.CleanedAsset, request.Stage.ID, []d.RefID{source.RefID}, body, source.Policy, source.Name, origin)
	if err != nil {
		return d.Output{}, err
	}
	return passedOutput("Cleaned data standardized and validated; ready for labeling", ref), nil
}

// PrepareAsset uses the durable lifecycle invocation ledger for idempotency,
// cancellation, recovery and an immutable operation-to-source binding.
func (s *Service) PrepareAsset(ctx context.Context, p d.Principal, source d.RefID, opts d.PreparationOptions, key, requestID string) (*d.StageRun, error) {
	if validateKey(key) != nil {
		return nil, d.ErrInvalid
	}
	raw, _ := json.Marshal(opts)
	var options map[string]any
	_ = json.Unmarshal(raw, &options)
	return s.Invoke(ctx, p, "data-preparation", []d.RefID{source}, options, "prepare:"+preparationDigest([]any{p.UserID, key}), requestID)
}
