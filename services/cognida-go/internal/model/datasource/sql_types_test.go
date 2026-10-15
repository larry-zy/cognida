package datasource

import "testing"

func TestIsNumericSQLTypeUsesDeclaredType(t *testing.T) {
	for _, typ := range []string{"DECIMAL", "numeric", "DECIMAL(38,18)", "UNSIGNED BIGINT", "INT UNSIGNED", "INT8", "FLOAT4", "DOUBLE PRECISION"} {
		if !IsNumericSQLType(typ) {
			t.Errorf("numeric type %q was not recognized", typ)
		}
	}
	for _, typ := range []string{"", "VARCHAR", "CHAR(12)", "TEXT", "JSON", "BIT", "BOOLEAN", "TIMESTAMP", "YEAR", "00123", "DECIMAL_ARRAY", "DECIMAL(foo)"} {
		if IsNumericSQLType(typ) {
			t.Errorf("non-numeric type %q was guessed numeric", typ)
		}
	}
}
