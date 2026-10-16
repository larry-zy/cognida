package governance

import (
	ds "cognida/internal/model/datasource"
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
)

func normalizeDriftSampling(s d.DriftSampling) (d.DriftSampling, error) {
	if s.Limit == 0 {
		s.Limit = 1000
	}
	if s.Minimum == 0 {
		s.Minimum = 20
	}
	if s.Seed == "" {
		s.Seed = "cognida-drift-v1"
	}
	if s.Limit < 2 || s.Limit > 1000 || s.Minimum < 2 || s.Minimum > s.Limit || len(s.Seed) > 128 {
		return s, d.ErrInvalid
	}
	return s, nil
}

// Select real row indexes by a stable seeded hash. No synthetic distribution is generated.
func driftSample(in l.Input, s d.DriftSampling) [][]any {
	type rank struct {
		index int
		hash  string
	}
	ranked := make([]rank, len(in.Rows))
	for i := range in.Rows {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", s.Seed, i)))
		ranked[i] = rank{i, hex.EncodeToString(sum[:])}
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].hash < ranked[j].hash })
	if len(ranked) > s.Limit {
		ranked = ranked[:s.Limit]
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].index < ranked[j].index })
	out := make([][]any, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, in.Rows[r.index])
	}
	return out
}
func driftFieldKind(in l.Input, col int) string {
	if typ := in.ColumnTypes[in.Columns[col]]; typ != "" {
		return strings.ToUpper(strings.TrimSpace(typ))
	}
	types := map[string]bool{}
	for _, row := range in.Rows {
		switch row[col].(type) {
		case nil:
		case json.Number, float64, float32, int, int32, int64, uint, uint32, uint64:
			types["NUMBER"] = true
		case string:
			types["STRING"] = true
		case bool:
			types["BOOL"] = true
		}
	}
	names := []string{}
	for t := range types {
		names = append(names, t)
	}
	sort.Strings(names)
	return strings.Join(names, "|")
}
func driftIsNumeric(kind string) bool { return kind == "NUMBER" || ds.IsNumericSQLType(kind) }
func driftNumbers(rows [][]any, col int) ([]*big.Rat, error) {
	out := []*big.Rat{}
	for _, row := range rows {
		if row[col] == nil {
			continue
		}
		v := row[col]
		if text, ok := v.(string); ok {
			v = json.Number(text)
		}
		n, e := featureNumber(v)
		if e != nil {
			return nil, e
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Cmp(out[j]) < 0 })
	return out, nil
}
func empiricalKS(a, b []*big.Rat) float64 {
	i, j := 0, 0
	effect := 0.0
	for i < len(a) || j < len(b) {
		var point *big.Rat
		if j == len(b) || (i < len(a) && a[i].Cmp(b[j]) <= 0) {
			point = a[i]
		} else {
			point = b[j]
		}
		for i < len(a) && a[i].Cmp(point) <= 0 {
			i++
		}
		for j < len(b) && b[j].Cmp(point) <= 0 {
			j++
		}
		diff := math.Abs(float64(i)/float64(len(a)) - float64(j)/float64(len(b)))
		if diff > effect {
			effect = diff
		}
	}
	return effect
}
func categoryDistance(a, b [][]any, ca, cb int) (float64, int, int) {
	counts := func(rows [][]any, col int) (map[string]int, int) {
		m := map[string]int{}
		n := 0
		for _, row := range rows {
			if row[col] != nil {
				raw, _ := json.Marshal(row[col])
				m[string(raw)]++
				n++
			}
		}
		return m, n
	}
	x, n := counts(a, ca)
	y, m := counts(b, cb)
	if n == 0 || m == 0 {
		return 0, n, m
	}
	distance := 0.0
	for k, v := range x {
		distance += math.Abs(float64(v)/float64(n) - float64(y[k])/float64(m))
		delete(y, k)
	}
	for _, v := range y {
		distance += float64(v) / float64(m)
	}
	return distance / 2, n, m
}
func compareDrift(a, b l.Input, s d.DriftSampling, threshold float64) ([]d.DriftMetric, int, int, bool, error) {
	if a.Kind != "table" || b.Kind != "table" || a.Validate() != nil || b.Validate() != nil || threshold <= 0 || threshold > 1 {
		return nil, 0, 0, false, d.ErrInvalid
	}
	sa, sb := driftSample(a, s), driftSample(b, s)
	ac, bc := map[string]int{}, map[string]int{}
	for i, c := range a.Columns {
		ac[c] = i
	}
	for i, c := range b.Columns {
		bc[c] = i
	}
	fields := []string{}
	for c := range ac {
		fields = append(fields, c)
	}
	for c := range bc {
		if _, ok := ac[c]; !ok {
			fields = append(fields, c)
		}
	}
	sort.Strings(fields)
	metrics := []d.DriftMetric{}
	changed := false
	for _, field := range fields {
		ca, oka := ac[field]
		cb, okb := bc[field]
		metric := d.DriftMetric{Field: field, Method: "schema", Threshold: 0, Status: "stable"}
		if !oka || !okb {
			metric.Status = "changed"
			metric.Detail = "字段新增或删除"
			changed = true
			metrics = append(metrics, metric)
			continue
		}
		ka, kb := driftFieldKind(a, ca), driftFieldKind(b, cb)
		if ka != kb {
			metric.Status = "changed"
			metric.Detail = ka + " → " + kb
			changed = true
		}
		metrics = append(metrics, metric)
		na, nb := 0, 0
		for _, r := range sa {
			if r[ca] == nil {
				na++
			}
		}
		for _, r := range sb {
			if r[cb] == nil {
				nb++
			}
		}
		effect := math.Abs(float64(na)/float64(len(sa)) - float64(nb)/float64(len(sb)))
		missing := d.DriftMetric{Field: field, Method: "missing_rate_delta", BaselineCount: len(sa), CurrentCount: len(sb), Threshold: threshold, Effect: &effect, Status: "stable"}
		if len(sa) < s.Minimum || len(sb) < s.Minimum {
			missing.Status = "insufficient"
			missing.Effect = nil
		} else if effect >= threshold {
			missing.Status = "changed"
			changed = true
		}
		metrics = append(metrics, missing)
		if ka != kb {
			continue
		}
		metric = d.DriftMetric{Field: field, Threshold: threshold, Status: "stable"}
		if driftIsNumeric(ka) {
			metric.Method = "empirical_ks"
			x, e := driftNumbers(sa, ca)
			if e != nil {
				return nil, 0, 0, false, featureErr("numeric_schema_mismatch", "declared numeric field contains non-numeric data: "+field)
			}
			y, e := driftNumbers(sb, cb)
			if e != nil {
				return nil, 0, 0, false, featureErr("numeric_schema_mismatch", "declared numeric field contains non-numeric data: "+field)
			}
			metric.BaselineCount = len(x)
			metric.CurrentCount = len(y)
			if len(x) >= s.Minimum && len(y) >= s.Minimum {
				v := empiricalKS(x, y)
				metric.Effect = &v
			}
		} else {
			metric.Method = "categorical_total_variation"
			v, n, m := categoryDistance(sa, sb, ca, cb)
			metric.BaselineCount = n
			metric.CurrentCount = m
			if n >= s.Minimum && m >= s.Minimum {
				metric.Effect = &v
			}
		}
		if metric.Effect == nil {
			metric.Status = "insufficient"
		} else if *metric.Effect >= threshold {
			metric.Status = "changed"
			changed = true
		}
		metrics = append(metrics, metric)
	}
	return metrics, len(sa), len(sb), changed, nil
}
