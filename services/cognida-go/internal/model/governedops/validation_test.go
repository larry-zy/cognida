package governedops

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCompileExactManagedDML(t *testing.T) {
	table := Table{Columns: []string{"code", "amount"}, Types: map[string]string{"code": "text", "amount": "number"}, Rows: [][]Cell{{{Type: "text", Text: "00123"}, {Type: "number", Text: "9007199254740993.123456789012345678"}}, {{Type: "text", Text: "ABC"}, {Type: "number", Text: "1.00"}}}}
	req := Request{Action: "update", Target: "agent_etl_orders", ExpectedVersion: 1, Values: map[string]Cell{"amount": {Type: "number", Text: "2.50"}}, Where: map[string]Cell{"code": {Type: "text", Text: "00123"}}}
	result, sql, params, count, e := Compile(table, req)
	require.NoError(t, e)
	require.EqualValues(t, 1, count)
	require.Equal(t, "2.50", result.Rows[0][1].Text)
	require.Contains(t, sql, "BINARY `code` = BINARY ?")
	require.Equal(t, "00123", params[1].Text)
	require.Equal(t, "9007199254740993.123456789012345678", table.Rows[0][1].Text)
	req.Where["code"] = Cell{Type: "text", Text: "abc"}
	_, _, _, count, e = Compile(table, req)
	require.NoError(t, e)
	require.Zero(t, count)
}
func TestRejectUnboundedOrAmbiguousWrites(t *testing.T) {
	for _, r := range []Request{{Action: "delete", Target: "orders", ExpectedVersion: 1}, {Action: "update", Target: "orders", ExpectedVersion: 1, Values: map[string]Cell{"x": {Type: "text", Text: "1"}}}, {Action: "insert", Target: "orders;DROP TABLE users", ExpectedVersion: 1, Values: map[string]Cell{"x": {Type: "text", Text: "x"}}}, {Action: "insert", Target: "cognida_op_hidden", ExpectedVersion: 1, Values: map[string]Cell{"x": {Type: "text", Text: "x"}}}} {
		require.ErrorIs(t, r.Validate(), ErrInvalid)
	}
	require.Error(t, (Cell{Type: "number", Text: "1;delete"}).Validate())
	require.Error(t, (Cell{Type: "boolean", Text: "1"}).Validate())
}
