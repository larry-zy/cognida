package metricsql

import (
	"cognida/internal/model/semantic"
	"encoding/json"
	"regexp"
	"strings"
)

type Dependency struct {
	Output         string `json:"output"`
	LogicalTableID string `json:"logical_table_id"`
	Column         string `json:"column"`
}
type Observer func(*semantic.ModelBundle, Query, *Result)

var observerSlots = make(chan struct{}, 8)

// Optional observers receive immutable copies after successful compilation.
// Slow graph storage cannot block SQL compilation or spawn unbounded goroutines.
func dispatchObservers(bundle *semantic.ModelBundle, q Query, result *Result, observers []Observer) {
	if len(observers) == 0 {
		return
	}
	select {
	case observerSlots <- struct{}{}:
	default:
		return
	}
	b, _ := json.Marshal(bundle)
	r, _ := json.Marshal(result)
	query, _ := json.Marshal(q)
	go func() {
		defer func() { <-observerSlots; _ = recover() }()
		for _, fn := range observers {
			if fn == nil {
				continue
			}
			var copy semantic.ModelBundle
			var res Result
			var cq Query
			if json.Unmarshal(b, &copy) != nil || json.Unmarshal(r, &res) != nil || json.Unmarshal(query, &cq) != nil {
				continue
			}
			fn(&copy, cq, &res)
		}
	}()
}

// Compiled expressions have already expanded governed metric references and
// qualified columns. Ignore string literals before reading identifier pairs.
var qualifiedColumn = regexp.MustCompile("([A-Za-z_][A-Za-z0-9_]*|`[^`]+`|\"[^\"]+\")\\s*\\.\\s*([A-Za-z_][A-Za-z0-9_]*|`[^`]+`|\"[^\"]+\")")
var lineageLiteral = regexp.MustCompile(`'(?:''|[^'])*'`)

func physicalDependencies(idx *index, metrics, dims []selectItem, filters []genericFilter, from string) []Dependency {
	aliases := map[string]string{}
	for id, a := range idx.aliasByID {
		aliases[a] = id
	}
	for _, t := range idx.bundle.LogicalTables {
		aliases[t.PhysicalTable] = t.ID
	}
	shared := " " + from
	for _, d := range dims {
		shared += " " + d.expr
	}
	for _, f := range filters {
		shared += " " + f.expr
	}
	items := metrics
	if len(items) == 0 {
		items = dims
	}
	out := []Dependency{}
	seen := map[string]bool{}
	for _, item := range items {
		start := len(out)
		expr := lineageLiteral.ReplaceAllString(item.expr+shared, " ")
		for _, m := range qualifiedColumn.FindAllStringSubmatch(expr, -1) {
			alias := strings.Trim(m[1], "`\"")
			id := aliases[alias]
			if id == "" {
				continue
			}
			col := strings.Trim(m[2], "`\"")
			key := item.alias + "\x00" + id + "\x00" + col
			if !seen[key] {
				seen[key] = true
				out = append(out, Dependency{item.alias, id, col})
			}
		}
		// COUNT(*) is a table dependency, not a fabricated column.
		if len(out) == start && item.tableID != "" {
			out = append(out, Dependency{item.alias, item.tableID, ""})
		}
	}
	return out
}
