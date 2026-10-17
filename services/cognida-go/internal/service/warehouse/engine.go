package warehouse

import (
	ds "cognida/internal/model/datasource"
	g "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/warehouse"
	"cognida/internal/service/governance"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const maxRows = 10000

var ident = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func problem(node, code, message string) error {
	return &d.Diagnostic{Node: node, Code: code, Message: message}
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func validNames(v []string) bool {
	if len(v) == 0 || len(v) > 32 {
		return false
	}
	seen := map[string]bool{}
	for _, x := range v {
		if x == "" || utf8.RuneCountInString(x) > 100 || seen[x] {
			return false
		}
		seen[x] = true
	}
	return true
}
func validate(def d.Definition) ([]d.Node, error) {
	if strings.TrimSpace(def.Name) == "" || len(def.Name) > 200 || strings.TrimSpace(def.Subject) == "" || len(def.Subject) > 200 || strings.TrimSpace(def.Grain) == "" || len(def.Grain) > 1000 || !validNames(def.GrainKeys) || !validNames(def.FactKeys) || len(def.Nodes) < 1 || len(def.Nodes) > 8 || def.UpdatePolicy != "upsert" || def.DeletePolicy != "unsupported" || def.LatePolicy != "reject_incremental_use_backfill" {
		return nil, problem("", "definition", "需声明主题域、事实粒度、键、upsert 更新及显式删除/迟到策略（最多 8 个节点）")
	}
	if (def.SemanticModelID == "") != (def.SemanticVersion == 0) {
		return nil, d.ErrInvalid
	}
	nodes := map[string]d.Node{}
	layer := map[string]int{"raw": 0, "detail": 1, "summary": 2, "application": 3}
	for _, n := range def.Nodes {
		if !ident.MatchString(n.ID) {
			return nil, d.ErrInvalid
		}
		if _, ok := nodes[n.ID]; ok {
			return nil, problem(n.ID, "duplicate_node", "节点 ID 重复")
		}
		if _, ok := layer[n.Layer]; !ok {
			return nil, d.ErrInvalid
		}
		nodes[n.ID] = n
		switch n.Kind {
		case "source":
			if len(n.Inputs) != 0 || n.SourceRef == "" || len(n.Columns)+len(n.GroupBy)+len(n.Metrics) > 0 || n.Join != nil || n.Layer != "raw" {
				return nil, d.ErrInvalid
			}
			if _, e := l.ParseRefID(n.SourceRef); e != nil {
				return nil, e
			}
		case "projection":
			if len(n.Inputs) != 1 || n.SourceRef != "" || !validProjection(n.Columns) || n.Join != nil || len(n.GroupBy)+len(n.Metrics) > 0 {
				return nil, d.ErrInvalid
			}
		case "left_join_many_to_one":
			if len(n.Inputs) != 2 || n.Inputs[0] == n.Inputs[1] || n.Join == nil || !validNames(n.Join.LeftKeys) || !validNames(n.Join.RightKeys) || len(n.Join.LeftKeys) != len(n.Join.RightKeys) || !validNames(n.Join.RightColumns) || !ident.MatchString(n.Join.Prefix) || n.SourceRef != "" || len(n.Columns)+len(n.Metrics)+len(n.GroupBy) > 0 {
				return nil, d.ErrInvalid
			}
		case "aggregate":
			if len(n.Inputs) != 1 || !validNames(n.GroupBy) || len(n.Metrics) < 1 || len(n.Metrics) > 32 || n.SourceRef != "" || len(n.Columns) > 0 || n.Join != nil {
				return nil, d.ErrInvalid
			}
			names := append([]string{}, n.GroupBy...)
			for _, m := range n.Metrics {
				names = append(names, m.Name)
				if m.Caliber == "" || len(m.Caliber) > 1000 {
					return nil, problem(n.ID, "metric_caliber", "每项度量必须声明口径")
				}
				switch m.Aggregate {
				case "sum", "mean", "min", "max":
					if m.Field == "" {
						return nil, d.ErrInvalid
					}
				case "count":
					if m.Field != "" {
						return nil, d.ErrInvalid
					}
				default:
					return nil, d.ErrInvalid
				}
			}
			if !validNames(names) {
				return nil, d.ErrInvalid
			}
		default:
			return nil, problem(n.ID, "unsupported_operator", "仅支持快照、受限投影、多对一左关联和分组聚合；不接受 SQL、DDL 或脚本")
		}
	}
	fact, ok := nodes[def.FactNode]
	if !ok || fact.Kind != "source" {
		return nil, problem("", "fact_source", "必须指定唯一事实来源节点")
	}
	if _, ok := nodes[def.OutputNode]; !ok {
		return nil, d.ErrInvalid
	}
	state := map[string]int{}
	order := []d.Node{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return problem(id, "cycle", "转换 DAG 存在依赖环")
		}
		if state[id] == 2 {
			return nil
		}
		n, ok := nodes[id]
		if !ok {
			return problem(id, "unknown_dependency", "输入节点不存在")
		}
		state[id] = 1
		for _, dep := range n.Inputs {
			if e := visit(dep); e != nil {
				return e
			}
			if layer[nodes[dep].Layer] > layer[n.Layer] {
				return problem(id, "layer_order", "分层转换不能反向依赖后层")
			}
		}
		state[id] = 2
		order = append(order, n)
		return nil
	}
	if e := visit(def.OutputNode); e != nil {
		return nil, e
	}
	if len(order) != len(nodes) {
		return nil, problem("", "unused_node", "每个节点必须参与最终输出，禁止隐藏未执行节点")
	}
	return order, nil
}
func validProjection(cols []d.Projection) bool {
	names := []string{}
	for _, c := range cols {
		names = append(names, c.Name)
		if c.Type != "text" && c.Type != "number" && c.Type != "boolean" {
			return false
		}
	}
	return validNames(names)
}
func indexes(t l.Input) (map[string]int, error) {
	if t.Kind != "table" || len(t.Rows) > maxRows || len(t.Columns)*len(t.Rows) > 100000 || !validNames(t.Columns) {
		return nil, problem("", "table_limit", "仅支持最多 10,000 行、100,000 单元格、32 列的完整表快照")
	}
	ix := map[string]int{}
	for i, c := range t.Columns {
		ix[c] = i
	}
	for _, row := range t.Rows {
		if len(row) != len(t.Columns) {
			return nil, d.ErrInvalid
		}
		for _, cell := range row {
			switch v := cell.(type) {
			case nil, bool, int, int64, int32, uint, uint64, uint32, float32, float64, json.Number:
			case string:
				if len(v) > 80000 {
					return nil, d.ErrInvalid
				}
			default:
				return nil, d.ErrInvalid
			}
		}
	}
	return ix, nil
}
func key(row []any, ix map[string]int, cols []string, types map[string]string) (string, error) {
	vals := []any{}
	for _, c := range cols {
		i, ok := ix[c]
		if !ok {
			return "", problem("", "missing_column", "缺少键列 "+c)
		}
		if row[i] == nil || row[i] == "" {
			return "", problem("", "null_key", "键列不允许为空："+c)
		}
		value := row[i]
		if ds.IsNumericSQLType(types[c]) || types[c] == "NUMBER" {
			number, e := numeric(context.Background(), value)
			if e != nil {
				return "", e
			}
			value = []string{"numeric", number.RatString()}
		}
		vals = append(vals, value)
	}
	raw, _ := json.Marshal(vals)
	return string(raw), nil
}
func unique(t l.Input, cols []string, node string) error {
	ix, e := indexes(t)
	if e != nil {
		return e
	}
	seen := map[string]bool{}
	for _, r := range t.Rows {
		k, e := key(r, ix, cols, t.ColumnTypes)
		if e != nil {
			return e
		}
		if seen[k] {
			return problem(node, "duplicate_grain", "重复键违反声明粒度；未发布重复放大的结果")
		}
		seen[k] = true
	}
	return nil
}
func expressionFields(e g.FeatureExpression) []string {
	if e.Op == "column" {
		return []string{e.Field}
	}
	out := []string{}
	for _, a := range e.Args {
		out = append(out, expressionFields(a)...)
	}
	return out
}
func project(ctx context.Context, n d.Node, in l.Input) (l.Input, []d.ColumnLineage, error) {
	out := l.Input{Kind: "table", Rows: [][]any{}, ColumnTypes: map[string]string{}}
	lines := []d.ColumnLineage{}
	ix, _ := indexes(in)
	for _, c := range n.Columns {
		out.Columns = append(out.Columns, c.Name)
		out.ColumnTypes[c.Name] = map[string]string{"text": "STRING", "number": "DECIMAL", "boolean": "BOOLEAN"}[c.Type]
		for _, f := range expressionFields(c.Expression) {
			if _, ok := ix[f]; !ok {
				return out, nil, problem(n.ID, "missing_column", "投影引用缺少列 "+f)
			}
			lines = append(lines, d.ColumnLineage{Node: n.ID, Column: c.Name, InputNode: n.Inputs[0], InputColumn: f})
		}
	}
	for _, r := range in.Rows {
		row := []any{}
		for _, c := range n.Columns {
			v, e := governance.EvaluateBoundedExpression(ctx, c.Expression, in.Columns, r)
			if e != nil {
				return out, nil, e
			}
			if v != nil {
				switch c.Type {
				case "text":
					if _, ok := v.(string); !ok {
						return out, nil, problem(n.ID, "column_type", "文本投影必须返回字符串")
					}
				case "boolean":
					if _, ok := v.(bool); !ok {
						return out, nil, d.ErrInvalid
					}
				case "number":
					v, e = governance.EvaluateBoundedExpression(ctx, g.FeatureExpression{Op: "to_number", Args: []g.FeatureExpression{{Op: "column", Field: "value"}}}, []string{"value"}, []any{v})
					if e != nil {
						return out, nil, e
					}
				}
			}
			row = append(row, v)
		}
		out.Rows = append(out.Rows, row)
	}
	return out, lines, nil
}
func join(n d.Node, left, right l.Input) (l.Input, []d.ColumnLineage, error) {
	if e := unique(right, n.Join.RightKeys, n.ID); e != nil {
		return l.Input{}, nil, e
	}
	li, _ := indexes(left)
	ri, _ := indexes(right)
	lookup := map[string][]any{}
	for _, r := range right.Rows {
		k, e := key(r, ri, n.Join.RightKeys, right.ColumnTypes)
		if e != nil {
			return l.Input{}, nil, e
		}
		lookup[k] = r
	}
	out := l.Input{Kind: "table", Columns: append([]string{}, left.Columns...), Rows: [][]any{}, ColumnTypes: map[string]string{}}
	lines := []d.ColumnLineage{}
	for _, c := range left.Columns {
		if typ := left.ColumnTypes[c]; typ != "" {
			out.ColumnTypes[c] = typ
		}
		lines = append(lines, d.ColumnLineage{Node: n.ID, Column: c, InputNode: n.Inputs[0], InputColumn: c})
	}
	for _, c := range n.Join.RightColumns {
		if _, ok := ri[c]; !ok {
			return out, nil, problem(n.ID, "missing_column", "维度缺少列 "+c)
		}
		name := n.Join.Prefix + c
		if _, ok := li[name]; ok {
			return out, nil, problem(n.ID, "column_collision", "关联输出列重名")
		}
		out.Columns = append(out.Columns, name)
		if typ := right.ColumnTypes[c]; typ != "" {
			out.ColumnTypes[name] = typ
		}
		lines = append(lines, d.ColumnLineage{Node: n.ID, Column: name, InputNode: n.Inputs[1], InputColumn: c})
		for _, k := range n.Join.LeftKeys {
			lines = append(lines, d.ColumnLineage{Node: n.ID, Column: name, InputNode: n.Inputs[0], InputColumn: k})
		}
		for _, k := range n.Join.RightKeys {
			lines = append(lines, d.ColumnLineage{Node: n.ID, Column: name, InputNode: n.Inputs[1], InputColumn: k})
		}
	}
	for _, r := range left.Rows {
		k, e := key(r, li, n.Join.LeftKeys, left.ColumnTypes)
		if e != nil {
			return out, nil, e
		}
		row := append([]any{}, r...)
		match := lookup[k]
		for _, c := range n.Join.RightColumns {
			var v any
			if match != nil {
				v = match[ri[c]]
			}
			row = append(row, v)
		}
		out.Rows = append(out.Rows, row)
	}
	return out, lines, nil
}
func numeric(ctx context.Context, v any) (*big.Rat, error) {
	out, e := governance.EvaluateBoundedExpression(ctx, g.FeatureExpression{Op: "to_number", Args: []g.FeatureExpression{{Op: "column", Field: "value"}}}, []string{"value"}, []any{v})
	if e != nil {
		return nil, e
	}
	n, ok := out.(json.Number)
	if !ok {
		return nil, d.ErrInvalid
	}
	r, ok := new(big.Rat).SetString(n.String()) // #nosec G113 -- Only the bounded to_number conversion above supplies this token (<=4096-bit magnitude, <=256 fractional digits); never raw input.
	if !ok {
		return nil, d.ErrInvalid
	}
	return r, nil
}
func decimal(r *big.Rat) (json.Number, error) {
	den := new(big.Int).Set(r.Denom())
	scale := 0
	for _, factor := range []int64{2, 5} {
		n := 0
		for new(big.Int).Mod(den, big.NewInt(factor)).Sign() == 0 {
			den.Quo(den, big.NewInt(factor))
			n++
		}
		if n > scale {
			scale = n
		}
	}
	if den.Cmp(big.NewInt(1)) != 0 {
		scale = 18
	}
	if scale > 256 {
		return "", d.ErrInvalid
	}
	v := r.FloatString(scale)
	if strings.Contains(v, ".") {
		v = strings.TrimRight(strings.TrimRight(v, "0"), ".")
	}
	return json.Number(v), nil
}
func aggregate(ctx context.Context, n d.Node, in l.Input) (l.Input, []d.ColumnLineage, error) {
	ix, _ := indexes(in)
	groups := map[string][][]any{}
	keys := []string{}
	for _, r := range in.Rows {
		k, e := key(r, ix, n.GroupBy, in.ColumnTypes)
		if e != nil {
			return l.Input{}, nil, e
		}
		if _, ok := groups[k]; !ok {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], r)
	}
	sort.Strings(keys)
	out := l.Input{Kind: "table", Columns: append([]string{}, n.GroupBy...), Rows: [][]any{}, ColumnTypes: map[string]string{}}
	lines := []d.ColumnLineage{}
	for _, c := range n.GroupBy {
		if _, ok := ix[c]; !ok {
			return out, nil, d.ErrInvalid
		}
		if typ := in.ColumnTypes[c]; typ != "" {
			out.ColumnTypes[c] = typ
		}
		lines = append(lines, d.ColumnLineage{Node: n.ID, Column: c, InputNode: n.Inputs[0], InputColumn: c})
	}
	for _, m := range n.Metrics {
		if m.Field != "" {
			if _, ok := ix[m.Field]; !ok {
				return out, nil, problem(n.ID, "missing_column", "度量引用缺少列 "+m.Field)
			}
		}
		out.Columns = append(out.Columns, m.Name)
		out.ColumnTypes[m.Name] = "DECIMAL"
		if m.Aggregate == "count" {
			// count(*) depends on the actual input row set. Record its real columns
			// with an explicit row-set relation, never invent an empty-name column.
			for _, c := range in.Columns {
				lines = append(lines, d.ColumnLineage{Node: n.ID, Column: m.Name, InputNode: n.Inputs[0], InputColumn: c, Dependency: "row_set"})
			}
		} else {
			lines = append(lines, d.ColumnLineage{Node: n.ID, Column: m.Name, InputNode: n.Inputs[0], InputColumn: m.Field})
		}
		for _, g := range n.GroupBy {
			lines = append(lines, d.ColumnLineage{Node: n.ID, Column: m.Name, InputNode: n.Inputs[0], InputColumn: g})
		}
	}
	for _, k := range keys {
		rows := groups[k]
		row := []any{}
		for _, c := range n.GroupBy {
			row = append(row, rows[0][ix[c]])
		}
		for _, m := range n.Metrics {
			var v any
			if m.Aggregate == "count" {
				v = json.Number(fmt.Sprint(len(rows)))
			} else {
				var sum *big.Rat
				count := 0
				for _, r := range rows {
					cell := r[ix[m.Field]]
					if cell == nil {
						continue
					}
					x, e := numeric(ctx, cell)
					if e != nil {
						return out, nil, e
					}
					count++
					if sum == nil {
						sum = new(big.Rat).Set(x)
					} else {
						switch m.Aggregate {
						case "sum", "mean":
							sum.Add(sum, x)
						case "min":
							if x.Cmp(sum) < 0 {
								sum.Set(x)
							}
						case "max":
							if x.Cmp(sum) > 0 {
								sum.Set(x)
							}
						}
					}
				}
				if count > 0 {
					if m.Aggregate == "mean" {
						sum.Quo(sum, big.NewRat(int64(count), 1))
					}
					var e error
					v, e = decimal(sum)
					if e != nil {
						return out, nil, e
					}
				}
			}
			row = append(row, v)
		}
		out.Rows = append(out.Rows, row)
	}
	return out, lines, nil
}
func execute(ctx context.Context, def d.Definition, sources map[string]l.Input) (*d.Preview, error) {
	order, e := validate(def)
	if e != nil {
		return nil, e
	}
	tables := map[string]l.Input{}
	preview := &d.Preview{DefinitionDigest: digest(def), SourceDigests: map[string]string{}, Lineage: []d.ColumnLineage{}, Stages: []d.Stage{}, Limitations: []string{"外部来源为固定只读快照，不承诺删除检测或持续同步。", "非终止小数在 18 位小数处四舍五入；原始有限小数按精确十进制处理。", "未登记的外部消费依赖未知，不代表没有影响。"}}
	for _, n := range order {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		var out l.Input
		var lines []d.ColumnLineage
		inputRows := 0
		switch n.Kind {
		case "source":
			var ok bool
			out, ok = sources[n.ID]
			if !ok {
				return nil, d.ErrInvalid
			}
			if _, e = indexes(out); e != nil {
				return nil, e
			}
			for _, c := range out.Columns {
				lines = append(lines, d.ColumnLineage{Node: n.ID, Column: c, SourceRef: n.SourceRef, InputColumn: c})
			}
		case "projection":
			inputRows = len(tables[n.Inputs[0]].Rows)
			out, lines, e = project(ctx, n, tables[n.Inputs[0]])
		case "left_join_many_to_one":
			inputRows = len(tables[n.Inputs[0]].Rows)
			out, lines, e = join(n, tables[n.Inputs[0]], tables[n.Inputs[1]])
		case "aggregate":
			inputRows = len(tables[n.Inputs[0]].Rows)
			out, lines, e = aggregate(ctx, n, tables[n.Inputs[0]])
		}
		if e != nil {
			return nil, e
		}
		if _, e = indexes(out); e != nil {
			return nil, e
		}
		tables[n.ID] = out
		preview.Lineage = append(preview.Lineage, lines...)
		preview.Stages = append(preview.Stages, d.Stage{Node: n.ID, Kind: n.Kind, Layer: n.Layer, InputRows: inputRows, OutputRows: len(out.Rows), Digest: digest(out)})
	}
	if e = unique(tables[def.FactNode], def.FactKeys, def.FactNode); e != nil {
		return nil, e
	}
	out := tables[def.OutputNode]
	if e = unique(out, def.GrainKeys, def.OutputNode); e != nil {
		return nil, e
	}
	if len(out.Rows) == 0 {
		return nil, problem(def.OutputNode, "empty_output", "当前快照没有候选数据，不切换活动版本")
	}
	preview.Output = out
	preview.TotalRows = len(out.Rows)
	return preview, nil
}
func event(t l.Input, row []any, field string) (time.Time, error) {
	ix, e := indexes(t)
	if e != nil {
		return time.Time{}, e
	}
	i, ok := ix[field]
	if !ok {
		return time.Time{}, problem("", "event_field", "缺少事件时间字段")
	}
	text, ok := row[i].(string)
	if !ok {
		return time.Time{}, d.ErrInvalid
	}
	at, e := time.Parse(time.RFC3339Nano, text)
	if e != nil {
		return at, problem("", "event_value", "事件时间必须是带时区的 RFC3339 字符串")
	}
	return at.UTC(), nil
}
func mergeFacts(def d.Definition, base, in l.Input, req d.RunRequest, watermark *time.Time) (l.Input, *time.Time, error) {
	if req.Mode == "full" {
		var latest *time.Time
		if def.EventField != "" {
			for _, r := range in.Rows {
				at, e := event(in, r, def.EventField)
				if e != nil {
					return in, nil, e
				}
				if latest == nil || at.After(*latest) {
					copy := at
					latest = &copy
				}
			}
		}
		return in, latest, nil
	}
	if def.EventField == "" || digest(base.Columns) != digest(in.Columns) || digest(base.ColumnTypes) != digest(in.ColumnTypes) {
		return in, nil, problem("", "incremental_schema", "增量/回填必须有固定事件字段，且与活动事实快照列类型完全一致")
	}
	if e := unique(in, def.FactKeys, def.FactNode); e != nil {
		return in, nil, e
	}
	ix, _ := indexes(in)
	rows := map[string][]any{}
	order := []string{}
	for _, r := range base.Rows {
		k, e := key(r, ix, def.FactKeys, in.ColumnTypes)
		if e != nil {
			return in, nil, e
		}
		rows[k] = r
		order = append(order, k)
	}
	latest := watermark
	for _, r := range in.Rows {
		at, e := event(in, r, def.EventField)
		if e != nil {
			return in, nil, e
		}
		if req.Mode == "incremental" && (watermark == nil || !at.After(*watermark)) {
			return in, nil, problem("", "late_record", "增量记录未超过活动水位；迟到或历史修正必须显式回填")
		}
		if req.Mode == "backfill" && (req.EventFrom == nil || req.EventTo == nil || at.Before(*req.EventFrom) || !at.Before(*req.EventTo)) {
			return in, nil, problem("", "backfill_range", "回填只接收固定半开时间范围内的记录")
		}
		k, e := key(r, ix, def.FactKeys, in.ColumnTypes)
		if e != nil {
			return in, nil, e
		}
		if _, ok := rows[k]; !ok {
			order = append(order, k)
		}
		rows[k] = r
		if req.Mode != "backfill" && (latest == nil || at.After(*latest)) {
			copy := at
			latest = &copy
		}
	}
	out := in
	out.Rows = [][]any{}
	for _, k := range order {
		out.Rows = append(out.Rows, rows[k])
	}
	if _, e := indexes(out); e != nil {
		return out, nil, e
	}
	return out, latest, nil
}

// Numeric constants are persisted in the existing explicit decimal-text AST
// form, preserving precision when a browser reads, edits and resaves a model.
func canonicalDefinition(in d.Definition) (d.Definition, error) {
	raw, e := json.Marshal(in)
	if e != nil {
		return in, e
	}
	var out d.Definition
	if e = json.Unmarshal(raw, &out); e != nil {
		return in, e
	}
	var normalize func(*g.FeatureExpression) error
	normalize = func(expr *g.FeatureExpression) error {
		for i := range expr.Args {
			if e := normalize(&expr.Args[i]); e != nil {
				return e
			}
		}
		if expr.Op != "literal" || len(expr.Value) == 0 {
			return nil
		}
		var value any
		decoder := json.NewDecoder(strings.NewReader(string(expr.Value)))
		decoder.UseNumber()
		if e := decoder.Decode(&value); e != nil {
			return e
		}
		if number, ok := value.(json.Number); ok {
			text, _ := json.Marshal(number.String())
			*expr = g.FeatureExpression{Op: "to_number", Args: []g.FeatureExpression{{Op: "literal", Value: text}}}
		}
		return nil
	}
	for i := range out.Nodes {
		for j := range out.Nodes[i].Columns {
			if e = normalize(&out.Nodes[i].Columns[j].Expression); e != nil {
				return out, e
			}
		}
	}
	return out, nil
}
