package governedops

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

var identifier = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
var decimal = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

func Identifier(s string) bool {
	return identifier.MatchString(s) && !strings.HasPrefix(strings.ToLower(s), "cognida_op_")
}
func Digest(v any) string {
	b, _ := json.Marshal(v)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func (c Cell) Validate() error {
	if len(c.Text) > 65535 || strings.ContainsRune(c.Text, 0) {
		return ErrInvalid
	}
	switch c.Type {
	case "null":
		if c.Text != "" {
			return ErrInvalid
		}
	case "text":
	case "number":
		if len(c.Text) > 512 || !decimal.MatchString(c.Text) {
			return ErrInvalid
		}
	case "boolean":
		if c.Text != "true" && c.Text != "false" {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
func (t Table) Validate() error {
	if len(t.Columns) < 1 || len(t.Columns) > 32 || len(t.Rows) > 1000 || len(t.Columns)*len(t.Rows) > 32000 || len(t.Types) != len(t.Columns) {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, c := range t.Columns {
		if !Identifier(c) || seen[strings.ToLower(c)] {
			return ErrInvalid
		}
		seen[strings.ToLower(c)] = true
		if t.Types[c] != "text" && t.Types[c] != "number" && t.Types[c] != "boolean" {
			return ErrInvalid
		}
	}
	for _, row := range t.Rows {
		if len(row) != len(t.Columns) {
			return ErrInvalid
		}
		for i, c := range row {
			if c.Validate() != nil || (c.Type != "null" && c.Type != t.Types[t.Columns[i]]) {
				return ErrInvalid
			}
		}
	}
	b, e := json.Marshal(t)
	if e != nil || len(b) > 4<<20 {
		return ErrInvalid
	}
	return nil
}
func (r Request) Validate() error {
	if r.SourceQueryHash != "" {
		if len(r.SourceQueryHash) != 64 {
			return ErrInvalid
		}
		if _, err := hex.DecodeString(r.SourceQueryHash); err != nil {
			return ErrInvalid
		}
	}
	if !Identifier(r.Target) || r.ExpectedVersion < 0 || len(r.Values) > 32 || len(r.Where) > 32 {
		return ErrInvalid
	}
	for _, m := range []map[string]Cell{r.Values, r.Where} {
		for k, v := range m {
			if !Identifier(k) || v.Validate() != nil {
				return ErrInvalid
			}
		}
	}
	switch r.Action {
	case "etl":
		if r.InputRef == nil || r.InputRef.Validate() != nil || len(r.Values) > 0 || len(r.Where) > 0 || r.RollbackVersion != 0 {
			return ErrInvalid
		}
	case "insert":
		if r.ExpectedVersion < 1 || r.InputRef != nil || len(r.Values) == 0 || len(r.Where) > 0 || r.RollbackVersion != 0 {
			return ErrInvalid
		}
	case "update":
		if r.ExpectedVersion < 1 || r.InputRef != nil || len(r.Values) == 0 || len(r.Where) == 0 || r.RollbackVersion != 0 {
			return ErrInvalid
		}
	case "delete":
		if r.ExpectedVersion < 1 || r.InputRef != nil || len(r.Values) > 0 || len(r.Where) == 0 || r.RollbackVersion != 0 {
			return ErrInvalid
		}
	case "rollback":
		if r.ExpectedVersion < 1 || r.RollbackVersion < 1 || r.InputRef != nil || len(r.Values) > 0 || len(r.Where) > 0 {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
func Quote(s string) string { return "`" + s + "`" }

// Compile applies explicit literal values and exact equality predicates only.
// No raw SQL, expressions, joins, subqueries, or target identifiers enter the SQL.
func Compile(before Table, r Request) (Table, string, []Cell, int64, error) {
	if r.Validate() != nil || before.Validate() != nil {
		return Table{}, "", nil, 0, ErrInvalid
	}
	raw, _ := json.Marshal(before)
	var after Table
	_ = json.Unmarshal(raw, &after)
	ix := map[string]int{}
	for i, c := range before.Columns {
		ix[c] = i
	}
	for _, m := range []map[string]Cell{r.Values, r.Where} {
		for c, v := range m {
			i, ok := ix[c]
			if !ok || (v.Type != "null" && v.Type != before.Types[before.Columns[i]]) {
				return Table{}, "", nil, 0, ErrInvalid
			}
		}
	}
	names := func(m map[string]Cell) []string {
		keys := []string{}
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	}
	params := []Cell{}
	predicate := []string{}
	for _, c := range names(r.Where) {
		if r.Where[c].Type == "null" {
			predicate = append(predicate, Quote(c)+" IS NULL")
		} else {
			predicate = append(predicate, "BINARY "+Quote(c)+" = BINARY ?")
			params = append(params, r.Where[c])
		}
	}
	sql := ""
	var affected int64
	match := func(row []Cell) bool {
		for c, v := range r.Where {
			if row[ix[c]] != v {
				return false
			}
		}
		return true
	}
	switch r.Action {
	case "insert":
		row := make([]Cell, len(before.Columns))
		marks := []string{}
		cols := []string{}
		params = nil
		for i, c := range before.Columns {
			v, ok := r.Values[c]
			if !ok {
				v = Cell{Type: "null"}
			}
			row[i] = v
			params = append(params, v)
			marks = append(marks, "?")
			cols = append(cols, Quote(c))
		}
		after.Rows = append(after.Rows, row)
		affected = 1
		sql = "INSERT INTO $TARGET (" + strings.Join(cols, ",") + ") VALUES (" + strings.Join(marks, ",") + ")"
	case "update":
		sets := []string{}
		values := []Cell{}
		for _, c := range names(r.Values) {
			sets = append(sets, Quote(c)+" = ?")
			values = append(values, r.Values[c])
		}
		params = append(values, params...)
		for i, row := range after.Rows {
			if match(row) {
				affected++
				for c, v := range r.Values {
					after.Rows[i][ix[c]] = v
				}
			}
		}
		sql = "UPDATE $TARGET SET " + strings.Join(sets, ",") + " WHERE " + strings.Join(predicate, " AND ")
	case "delete":
		after.Rows = [][]Cell{}
		for _, row := range before.Rows {
			if match(row) {
				affected++
			} else {
				after.Rows = append(after.Rows, row)
			}
		}
		sql = "DELETE FROM $TARGET WHERE " + strings.Join(predicate, " AND ")
	default:
		return Table{}, "", nil, 0, ErrInvalid
	}
	if after.Validate() != nil {
		return Table{}, "", nil, 0, ErrInvalid
	}
	return after, sql, params, affected, nil
}
