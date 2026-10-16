package governance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
)

const featureRowsLimit = 1000

func featureErr(code, message string) *d.FeatureError {
	return &d.FeatureError{Code: code, Message: message}
}
func rowErr(err error, row int) error {
	if e, ok := err.(*d.FeatureError); ok {
		copy := *e
		copy.Row = &row
		return &copy
	}
	return &d.FeatureError{Code: "invalid_value", Row: &row, Message: "value is incompatible with the declared feature type"}
}
func featureHash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func strictFeatureJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var walk func() error
	walk = func() error {
		token, e := decoder.Token()
		if e != nil {
			return e
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		if delim == '{' {
			seen := map[string]bool{}
			for decoder.More() {
				k, e := decoder.Token()
				if e != nil {
					return e
				}
				key, ok := k.(string)
				if !ok || seen[key] {
					return d.ErrInvalid
				}
				seen[key] = true
				if e = walk(); e != nil {
					return e
				}
			}
		} else if delim == '[' {
			for decoder.More() {
				if e := walk(); e != nil {
					return e
				}
			}
		} else {
			return d.ErrInvalid
		}
		_, e = decoder.Token()
		return e
	}
	if e := walk(); e != nil {
		return e
	}
	if _, e := decoder.Token(); e != io.EOF {
		return d.ErrInvalid
	}
	return nil
}
func exactFeatureKeys(raw json.RawMessage, expression bool) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false
	}
	allowed := map[string]bool{"kind": true, "field": true, "entity_field": true, "event_time_field": true, "expression": true, "window_seconds": true, "aggregate": true, "embedding": true}
	if expression {
		allowed = map[string]bool{"op": true, "field": true, "value": true, "args": true, "mapping": true, "default": true}
	}
	for key, value := range fields {
		if !allowed[key] {
			return false
		}
		if key == "embedding" {
			var model map[string]json.RawMessage
			if json.Unmarshal(value, &model) != nil || model == nil {
				return false
			}
			for k := range model {
				if k != "model_id" && k != "revision" && k != "dimension" {
					return false
				}
			}
		}
		if key == "expression" {
			if !exactFeatureKeys(value, true) {
				return false
			}
		}
		if key == "args" {
			var args []json.RawMessage
			if json.Unmarshal(value, &args) != nil {
				return false
			}
			for _, arg := range args {
				if !exactFeatureKeys(arg, true) {
					return false
				}
			}
		}
	}
	return true
}
func parseFeatureDefinition(raw json.RawMessage) (d.FeatureDefinition, error) {
	var def d.FeatureDefinition
	if len(raw) == 0 || len(raw) > 32768 || strictFeatureJSON(raw) != nil || !exactFeatureKeys(raw, false) {
		return def, featureErr("invalid_definition", "definition must be bounded JSON without duplicate keys")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&def) != nil {
		return def, featureErr("invalid_definition", "unknown or malformed definition fields")
	}
	for _, field := range []string{def.Field, def.EntityField, def.EventTimeField} {
		if len(field) > 128 || !utf8.ValidString(field) || strings.ContainsRune(field, 0) {
			return def, featureErr("invalid_field", "field names must be valid text up to 128 bytes")
		}
	}
	switch def.Kind {
	case "field":
		if def.Field == "" || def.Expression != nil || def.WindowSeconds != 0 || def.Aggregate != "" {
			return def, featureErr("invalid_definition", "field requires field and no expression or window options")
		}
	case "expression":
		if def.Expression == nil || def.Field != "" || def.WindowSeconds != 0 || def.Aggregate != "" {
			return def, featureErr("invalid_definition", "expression requires one expression AST")
		}
		count := 0
		if e := validateExpression(*def.Expression, 0, &count); e != nil {
			return def, e
		}
	case "window_aggregate":
		if def.Expression != nil || def.EntityField == "" || def.EventTimeField == "" || def.WindowSeconds < 1 || def.WindowSeconds > 31536000 {
			return def, featureErr("invalid_window", "window requires entity, event time and 1..31536000 seconds")
		}
		switch def.Aggregate {
		case "count":
		case "sum", "mean", "min", "max":
			if def.Field == "" {
				return def, featureErr("invalid_window", "numeric aggregate requires field")
			}
		default:
			return def, featureErr("invalid_window", "aggregate must be count, sum, mean, min or max")
		}
	case "embedding":
		if def.Embedding == nil || strings.TrimSpace(def.Embedding.ModelID) == "" || strings.TrimSpace(def.Embedding.Revision) == "" || len(def.Embedding.ModelID) > 256 || len(def.Embedding.Revision) > 256 || def.Embedding.Dimension < 1 || def.Embedding.Dimension > 4096 || def.Field == "" || def.Expression != nil || def.WindowSeconds != 0 || def.Aggregate != "" {
			return def, featureErr("invalid_embedding", "embedding requires a fixed model identity/revision, dimension 1..4096 and text field")
		}
	default:
		return def, featureErr("unsupported_kind", "supported kinds are field, expression and window_aggregate")
	}
	if def.Kind != "embedding" && def.Embedding != nil {
		return def, featureErr("invalid_definition", "embedding options require embedding kind")
	}
	return def, nil
}
func validateExpression(e d.FeatureExpression, depth int, count *int) error {
	*count++
	if depth > 8 || *count > 64 {
		return featureErr("expression_limit", "expression is limited to depth 8 and 64 nodes")
	}
	arity := -1
	switch e.Op {
	case "column":
		arity = 0
		if e.Field == "" || len(e.Field) > 128 || !utf8.ValidString(e.Field) {
			return featureErr("invalid_expression", "column requires a valid field")
		}
	case "literal":
		arity = 0
		if len(e.Value) == 0 || len(e.Value) > 4096 {
			return featureErr("invalid_expression", "literal requires a bounded value")
		}
		var value any
		decoder := json.NewDecoder(bytes.NewReader(e.Value))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			return featureErr("invalid_expression", "invalid literal")
		}
		switch value.(type) {
		case string, json.Number:
		default:
			return featureErr("invalid_expression", "literal must be text or a number")
		}
	case "add", "sub", "mul", "div", "round":
		arity = 2
	case "abs", "to_number", "lower", "upper", "trim", "category_map":
		arity = 1
	case "clamp":
		arity = 3
	case "concat":
		if len(e.Args) < 1 || len(e.Args) > 8 {
			return featureErr("invalid_expression", "concat accepts 1..8 text arguments")
		}
		arity = len(e.Args)
	default:
		return featureErr("unsupported_operation", "expression operation is not allowed")
	}
	if len(e.Args) != arity || (e.Op != "column" && e.Field != "") || (e.Op != "literal" && len(e.Value) > 0) || (e.Op != "category_map" && (e.Mapping != nil || e.Default != nil)) {
		return featureErr("invalid_expression", "operation fields or argument count do not match")
	}
	if e.Op == "category_map" {
		if len(e.Mapping) == 0 || len(e.Mapping) > 256 {
			return featureErr("invalid_expression", "category_map requires 1..256 declared mappings")
		}
		for key, value := range e.Mapping {
			if len(key) > 4096 || len(value) > 4096 {
				return featureErr("invalid_expression", "category map value is too long")
			}
		}
		if e.Default != nil && len(*e.Default) > 4096 {
			return featureErr("invalid_expression", "category default is too long")
		}
	}
	for _, arg := range e.Args {
		if err := validateExpression(arg, depth+1, count); err != nil {
			return err
		}
	}
	return nil
}
func featureNumber(value any) (*big.Rat, error) {
	var text string
	switch v := value.(type) {
	case json.Number:
		text = string(v)
	case string:
		text = v
	case int:
		text = strconv.Itoa(v)
	case int64:
		text = strconv.FormatInt(v, 10)
	case float64:
		text = strconv.FormatFloat(v, 'g', -1, 64)
	default:
		return nil, featureErr("numeric_required", "numeric operation requires a number or explicit decimal text")
	}
	if len(text) > 256 || !json.Valid([]byte(text)) || strings.TrimSpace(text) != text {
		return nil, featureErr("invalid_number", "invalid or oversized decimal value")
	}
	var decoded any
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	if decoder.Decode(&decoded) != nil {
		return nil, featureErr("invalid_number", "invalid decimal")
	}
	if _, ok := decoded.(json.Number); !ok {
		return nil, featureErr("numeric_required", "numeric operation requires a decimal value")
	}
	if pos := strings.IndexAny(text, "eE"); pos >= 0 {
		exponent, e := strconv.Atoi(text[pos+1:])
		if e != nil || exponent < -1024 || exponent > 1024 {
			return nil, featureErr("numeric_limit", "decimal exponent exceeds allowed bounds")
		}
	}
	r, ok := new(big.Rat).SetString(text)
	if !ok || r.Num().BitLen() > 4096 || r.Denom().BitLen() > 4096 {
		return nil, featureErr("numeric_limit", "decimal magnitude exceeds the bounded numeric profile")
	}
	return r, nil
}

// Rat arithmetic preserves decimal spelling accuracy. Non-terminating division
// is explicitly rounded to 18 fractional digits (half away from zero).
func featureDecimal(r *big.Rat) (json.Number, error) {
	den := new(big.Int).Set(r.Denom())
	two, five := 0, 0
	rem := new(big.Int)
	for {
		q := new(big.Int)
		q.QuoRem(den, big.NewInt(2), rem)
		if rem.Sign() != 0 {
			break
		}
		den = q
		two++
	}
	for {
		q := new(big.Int)
		q.QuoRem(den, big.NewInt(5), rem)
		if rem.Sign() != 0 {
			break
		}
		den = q
		five++
	}
	scale := 18
	if den.Cmp(big.NewInt(1)) == 0 {
		scale = max(two, five)
	}
	if scale > 256 {
		return "", featureErr("numeric_limit", "exact decimal result exceeds 256 fractional digits")
	}
	text := r.FloatString(scale)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	if text == "-0" {
		text = "0"
	}
	return json.Number(text), nil
}
func evalFeatureExpression(ctx context.Context, e d.FeatureExpression, row []any, indexes map[string]int) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if e.Op == "column" {
		i, ok := indexes[e.Field]
		if !ok {
			return nil, &d.FeatureError{Code: "missing_field", Field: e.Field, Message: "column is absent in source"}
		}
		return row[i], nil
	}
	if e.Op == "literal" {
		var v any
		decoder := json.NewDecoder(bytes.NewReader(e.Value))
		decoder.UseNumber()
		err := decoder.Decode(&v)
		return v, err
	}
	args := make([]any, len(e.Args))
	for i, arg := range e.Args {
		v, err := evalFeatureExpression(ctx, arg, row, indexes)
		if err != nil {
			return nil, err
		}
		args[i] = v
	}
	switch e.Op {
	case "lower", "upper", "trim", "concat", "category_map":
		texts := make([]string, len(args))
		for i, arg := range args {
			v, ok := arg.(string)
			if !ok {
				return nil, featureErr("text_required", "text operation requires text and does not coerce identifiers")
			}
			texts[i] = v
		}
		var out string
		switch e.Op {
		case "lower":
			out = strings.ToLower(texts[0])
		case "upper":
			out = strings.ToUpper(texts[0])
		case "trim":
			out = strings.TrimSpace(texts[0])
		case "concat":
			out = strings.Join(texts, "")
		case "category_map":
			v, ok := e.Mapping[texts[0]]
			if !ok {
				if e.Default == nil {
					return nil, featureErr("unmapped_category", "category has no declared mapping or default")
				}
				v = *e.Default
			}
			out = v
		}
		if len(out) > 4096 {
			return nil, featureErr("text_limit", "text feature exceeds 4096 bytes")
		}
		return out, nil
	}
	nums := make([]*big.Rat, len(args))
	for i, arg := range args {
		n, err := featureNumber(arg)
		if err != nil {
			return nil, err
		}
		nums[i] = n
	}
	n := new(big.Rat)
	switch e.Op {
	case "to_number":
		n.Set(nums[0])
	case "add":
		n.Add(nums[0], nums[1])
	case "sub":
		n.Sub(nums[0], nums[1])
	case "mul":
		n.Mul(nums[0], nums[1])
	case "div":
		if nums[1].Sign() == 0 {
			return nil, featureErr("division_by_zero", "division denominator must be nonzero")
		}
		n.Quo(nums[0], nums[1])
	case "abs":
		n.Abs(nums[0])
	case "clamp":
		if nums[1].Cmp(nums[2]) > 0 {
			return nil, featureErr("invalid_bounds", "clamp lower bound exceeds upper bound")
		}
		n.Set(nums[0])
		if n.Cmp(nums[1]) < 0 {
			n.Set(nums[1])
		}
		if n.Cmp(nums[2]) > 0 {
			n.Set(nums[2])
		}
	case "round":
		if !nums[1].IsInt() || !nums[1].Num().IsInt64() {
			return nil, featureErr("invalid_precision", "round precision must be an integer 0..18")
		}
		scale := nums[1].Num().Int64()
		if scale < 0 || scale > 18 {
			return nil, featureErr("invalid_precision", "round precision must be an integer 0..18")
		}
		n.SetString(nums[0].FloatString(int(scale)))
	}
	if n.Num().BitLen() > 4096 || n.Denom().BitLen() > 4096 {
		return nil, featureErr("numeric_limit", "arithmetic result exceeds numeric bounds")
	}
	return featureDecimal(n)
}
func featureEvent(value any) (time.Time, error) {
	v, ok := value.(string)
	if !ok {
		return time.Time{}, featureErr("invalid_event_time", "event time must be RFC3339 text with timezone")
	}
	at, e := time.Parse(time.RFC3339Nano, v)
	if e != nil || at.Year() < 1970 || at.Year() > 9999 {
		return time.Time{}, featureErr("invalid_event_time", "event time must be RFC3339 text with timezone")
	}
	return at.UTC().Truncate(time.Microsecond), nil
}
func featureEntity(value any) (string, error) {
	switch v := value.(type) {
	case string:
		if v != "" && len(v) <= 128 {
			return v, nil
		}
	case json.Number:
		if len(v) <= 128 {
			return string(v), nil
		}
	case int:
		return strconv.Itoa(v), nil
	}
	return "", featureErr("invalid_entity", "entity must be nonempty text or an exact integer identifier up to 128 bytes")
}
func computeFeatureValues(ctx context.Context, f d.Feature, ref l.DataAssetRef, input l.Input) ([]d.FeatureValue, []string, error) {
	return computeFeatureValuesLimit(ctx, f, ref, input, featureRowsLimit)
}
func computeFeatureValuesLimit(ctx context.Context, f d.Feature, ref l.DataAssetRef, input l.Input, limit int) ([]d.FeatureValue, []string, error) {
	def, err := parseFeatureDefinition(f.Definition)
	if err != nil {
		return nil, nil, err
	}
	if def.Kind == "embedding" {
		return nil, nil, featureErr("embedding_unavailable", "embedding requires the configured real model execution path")
	}
	if input.Kind != "table" || input.Validate() != nil || len(input.Rows) > limit {
		return nil, nil, featureErr("input_limit", "feature input must be a valid table of 1..1000 rows")
	}
	indexes := map[string]int{}
	for i, col := range input.Columns {
		if _, ok := indexes[col]; ok {
			return nil, nil, featureErr("duplicate_column", "source columns must be unique")
		}
		indexes[col] = i
	}
	for _, field := range []string{def.Field, def.EntityField, def.EventTimeField} {
		if field != "" {
			if _, ok := indexes[field]; !ok {
				return nil, nil, &d.FeatureError{Code: "missing_field", Field: field, Message: "column is absent in source"}
			}
		}
	}
	if def.Kind == "window_aggregate" && f.Type != "numerical" {
		return nil, nil, featureErr("invalid_type", "window aggregates produce numerical features")
	}
	out := make([]d.FeatureValue, 0, len(input.Rows))
	warnings := []string{}
	skipped := 0
	for i, row := range input.Rows {
		if err = ctx.Err(); err != nil {
			return nil, nil, err
		}
		entity := fmt.Sprintf("%s:%d", ref.AssetID, i)
		if def.EntityField != "" {
			entity, err = featureEntity(row[indexes[def.EntityField]])
			if err != nil {
				return nil, nil, rowErr(err, i)
			}
		}
		at := ref.CreatedAt.UTC().Truncate(time.Microsecond)
		if def.EventTimeField != "" {
			at, err = featureEvent(row[indexes[def.EventTimeField]])
			if err != nil {
				return nil, nil, rowErr(err, i)
			}
		}
		var value any
		if def.Kind == "expression" {
			value, err = evalFeatureExpression(ctx, *def.Expression, row, indexes)
		} else if def.Aggregate == "count" {
			value = json.Number("1")
		} else {
			value = row[indexes[def.Field]]
		}
		if err != nil {
			return nil, nil, rowErr(err, i)
		}
		if value == nil && def.Kind == "field" {
			skipped++
			continue
		}
		if f.Type == "numerical" {
			n, e := featureNumber(value)
			if e != nil {
				return nil, nil, rowErr(e, i)
			}
			value, e = featureDecimal(n)
			if e != nil {
				return nil, nil, rowErr(e, i)
			}
		}
		if f.Type == "vector" {
			if text, ok := value.(string); ok {
				decoder := json.NewDecoder(strings.NewReader(text))
				decoder.UseNumber()
				if !json.Valid([]byte(text)) || decoder.Decode(&value) != nil {
					return nil, nil, rowErr(featureErr("invalid_vector", "vector must be a JSON array of finite numbers"), i)
				}
			}
		}
		raw, e := json.Marshal(value)
		if e != nil || validateFeatureValue(f.Type, raw) != nil {
			return nil, nil, rowErr(featureErr("invalid_value", "value is incompatible with feature type"), i)
		}
		out = append(out, d.FeatureValue{FeatureID: f.ID, Version: f.Version, EntityID: entity, Timestamp: at, Value: raw, ValueExact: string(raw), SourceRefID: f.SourceRefID})
	}
	if skipped > 0 {
		warnings = append(warnings, fmt.Sprintf("field extraction skipped %d null rows", skipped))
	}
	if def.Kind == "window_aggregate" {
		out, err = aggregateFeatureWindow(ctx, out, def)
		if err != nil {
			return nil, nil, err
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].EntityID == out[j].EntityID {
			return out[i].Timestamp.Before(out[j].Timestamp)
		}
		return out[i].EntityID < out[j].EntityID
	})
	seen := map[string]bool{}
	for _, v := range out {
		key := v.EntityID + "\x00" + v.Timestamp.Format(time.RFC3339Nano)
		if seen[key] {
			return nil, nil, featureErr("duplicate_entity_time", "multiple values for one entity and event time require a window aggregate or distinct event times")
		}
		seen[key] = true
	}
	return out, warnings, nil
}
func aggregateFeatureWindow(ctx context.Context, values []d.FeatureValue, def d.FeatureDefinition) ([]d.FeatureValue, error) {
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].EntityID == values[j].EntityID {
			return values[i].Timestamp.Before(values[j].Timestamp)
		}
		return values[i].EntityID < values[j].EntityID
	})
	out := []d.FeatureValue{}
	for begin := 0; begin < len(values); {
		end := begin + 1
		for end < len(values) && values[end].EntityID == values[begin].EntityID {
			end++
		}
		nums := make([]*big.Rat, end-begin)
		for i := begin; i < end; i++ {
			n, e := featureNumber(json.Number(values[i].Value))
			if e != nil {
				return nil, e
			}
			nums[i-begin] = n
		}
		sum := new(big.Rat)
		left, right := begin, begin
		minQ, maxQ := []int{}, []int{}
		for point := begin; point < end; {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			at := values[point].Timestamp
			for right < end && !values[right].Timestamp.After(at) {
				n := nums[right-begin]
				sum.Add(sum, n)
				for len(minQ) > 0 && nums[minQ[len(minQ)-1]-begin].Cmp(n) >= 0 {
					minQ = minQ[:len(minQ)-1]
				}
				for len(maxQ) > 0 && nums[maxQ[len(maxQ)-1]-begin].Cmp(n) <= 0 {
					maxQ = maxQ[:len(maxQ)-1]
				}
				minQ = append(minQ, right)
				maxQ = append(maxQ, right)
				right++
			}
			lower := at.Add(-time.Duration(def.WindowSeconds) * time.Second)
			for left < right && !values[left].Timestamp.After(lower) {
				sum.Sub(sum, nums[left-begin])
				left++
			}
			for len(minQ) > 0 && minQ[0] < left {
				minQ = minQ[1:]
			}
			for len(maxQ) > 0 && maxQ[0] < left {
				maxQ = maxQ[1:]
			}
			count := right - left
			var result *big.Rat
			switch def.Aggregate {
			case "count":
				result = new(big.Rat).SetInt64(int64(count))
			case "sum":
				result = new(big.Rat).Set(sum)
			case "mean":
				result = new(big.Rat).Quo(sum, new(big.Rat).SetInt64(int64(count)))
			case "min":
				result = nums[minQ[0]-begin]
			case "max":
				result = nums[maxQ[0]-begin]
			}
			number, e := featureDecimal(result)
			if e != nil {
				return nil, e
			}
			value := values[point]
			value.Value = []byte(number)
			value.ValueExact = string(value.Value)
			out = append(out, value)
			point = right
		}
		begin = end
	}
	return out, nil
}
