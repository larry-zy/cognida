package datasource

import "strings"

// IsNumericSQLType recognizes explicit SQL numeric metadata. It never infers a
// type from a cell's spelling, so VARCHAR identifiers such as 00123 stay text.
func IsNumericSQLType(databaseType string) bool {
	typ := strings.ToUpper(strings.TrimSpace(databaseType))
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
	case "TINYINT", "SMALLINT", "MEDIUMINT", "INT", "INTEGER", "BIGINT", "INT2", "INT4", "INT8", "SMALLSERIAL", "SERIAL", "BIGSERIAL", "DECIMAL", "NUMERIC", "FLOAT", "DOUBLE", "DOUBLE PRECISION", "REAL", "FLOAT4", "FLOAT8":
		return true
	default:
		return false
	}
}
