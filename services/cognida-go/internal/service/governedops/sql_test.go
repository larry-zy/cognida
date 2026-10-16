package governedops

import (
	d "cognida/internal/model/governedops"
	l "cognida/internal/model/lifecycle"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLiteralSQLProducesTypedExactPlan(t *testing.T) {
	for _, q := range []string{"INSERT INTO agent_etl_sales(code,amount) VALUES ('00123',9007199254740993.123456789012345678)", "UPDATE agent_etl_sales SET amount=2.50 WHERE code='00123' AND active=TRUE;", "DELETE FROM agent_etl_sales WHERE code='it''s exact'"} {
		r, e := parseLiteralSQL(q)
		require.NoError(t, e)
		require.NoError(t, r.Validate())
	}
	r, e := parseLiteralSQL("INSERT INTO agent_etl_sales(code,amount) VALUES ('00123',9007199254740993.123456789012345678)")
	require.NoError(t, e)
	require.Equal(t, d.Cell{Type: "text", Text: "00123"}, r.Values["code"])
	require.Equal(t, "9007199254740993.123456789012345678", r.Values["amount"].Text)
	for _, q := range []string{"UPDATE t SET x=1", "DELETE FROM t", "UPDATE t SET x=x+1 WHERE id=1", "UPDATE t SET x=1 WHERE id=1 OR id=2", "INSERT INTO t(x) SELECT x FROM s", "INSERT INTO t(x,x) VALUES(1,2)", "DELETE FROM t WHERE id=1; DELETE FROM t WHERE id=2", "DELETE FROM db.t WHERE id=1", "DELETE FROM t WHERE id=1 -- comment", "UPDATE t SET x=1 WHERE y='broken", "UPDATE t SET x=1 WHERE y='a\\b'", "UPDATE t SET x=1 WHERE y='ok' trailing"} {
		_, e := parseLiteralSQL(q)
		require.Error(t, e, q)
	}
}
func TestInputColumnInferenceIgnoresNullButNeverGuessesStrings(t *testing.T) {
	in := l.Input{Kind: "table", Columns: []string{"amount", "code"}, Rows: [][]any{{nil, "00123"}, {json.Number("9007199254740993.123456789012345678"), "4"}}}
	table, e := tableFromInput(in)
	require.NoError(t, e)
	require.Equal(t, "number", table.Types["amount"])
	require.Equal(t, "text", table.Types["code"])
	require.Equal(t, "9007199254740993.123456789012345678", table.Rows[1][0].Text)
	in.Rows = append(in.Rows, []any{"4", "005"})
	_, e = tableFromInput(in)
	require.Error(t, e)
}
