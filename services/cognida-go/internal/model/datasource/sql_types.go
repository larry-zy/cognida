package datasource

import "strings"

// IsNumericSQLType recognizes explicit SQL numeric metadata. It never infers a
// type from a cell's spelling, so VARCHAR identifiers such as 00123 stay text.
func IsNumericSQLType(databaseType string) bool {
	typ := strings.ToUpper(strings.TrimSpace(databaseType))
	for _, wrapper := range []string{"NULLABLE(", "LOWCARDINALITY("} {
		if strings.HasPrefix(typ, wrapper) && strings.HasSuffix(typ, ")") {
			inner := typ[len(wrapper) : len(typ)-1]
			if inner == typ {
				return false
			}
			return IsNumericSQLType(inner)
		}
	}
	typ = strings.TrimSpace(strings.TrimPrefix(typ, "UNSIGNED "))
	typ = strings.TrimSpace(strings.TrimSuffix(typ, " UNSIGNED"))
	if index := strings.IndexByte(typ, '('); index >= 0 {
		if !strings.HasSuffix(typ, ")") {
			return false
		}
		for _, ch := range typ[index+1 : len(typ)-1] {
			if (ch < '0' || ch > '9') && ch != ',' && ch != ' ' {
				return false
			}
		}
		typ = strings.TrimSpace(typ[:index])
	}
	switch typ {
	case "TINYINT", "SMALLINT", "MEDIUMINT", "INT", "INTEGER", "BIGINT", "INT2", "INT4", "INT8", "INT16", "INT32", "INT64", "INT128", "INT256", "UINT8", "UINT16", "UINT32", "UINT64", "UINT128", "UINT256", "SMALLSERIAL", "SERIAL", "BIGSERIAL", "DECIMAL", "DECIMAL32", "DECIMAL64", "DECIMAL128", "DECIMAL256", "NUMERIC", "BIGNUMERIC", "BIGDECIMAL", "FLOAT", "FLOAT32", "FLOAT64", "DOUBLE", "DOUBLE PRECISION", "REAL", "FLOAT4", "FLOAT8":
		return true
	default:
		return false
	}
}
