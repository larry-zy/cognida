package metricsql

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestBigQueryCalendarGrainsAndEscapes(t *testing.T) {
	for grain, format := range map[TimeGrain]string{GrainDay: "%Y-%m-%d", GrainWeek: "%G-W%V", GrainMonth: "%Y-%m", GrainQuarter: "%Y-Q%Q", GrainYear: "%Y"} {
		for _, typ := range []string{"date", "datetime", "timestamp"} {
			b := timeBundle()
			b.Dimensions[1].DataType = typ
			b.LogicalTables[0].PhysicalTable = "project-id.dataset.orders"
			got, err := Build(b, Query{Dialect: DialectBigQuery, TimeGrain: grain, Metrics: []string{"营收"}, Filters: []Filter{{Field: "状态", Op: OpEq, Values: []string{"O'Reilly\\path\nnext"}}}})
			require.NoError(t, err)
			require.True(t, got.Coverage.Covered)
			bucket := "FORMAT_DATE('" + format + "', CAST(orders.created_at AS DATE))"
			require.Contains(t, got.SQL, bucket+" AS `下单日期`")
			require.Contains(t, got.SQL, "GROUP BY "+bucket)
			require.Contains(t, got.SQL, "FROM `project-id.dataset.orders` orders")
			require.Contains(t, got.SQL, "'O\\'Reilly\\\\path\\x0anext'")
			require.NotContains(t, got.SQL, "DATE_FORMAT")
		}
	}
	require.Equal(t, "`a\\`b\\\\c`", quoteIdent(DialectBigQuery, "a`b\\c"))
	b := timeBundle()
	b.Dimensions[1].DataType = "time"
	_, err := Build(b, Query{Dialect: DialectBigQuery, TimeGrain: GrainDay, Metrics: []string{"营收"}})
	require.ErrorContains(t, err, "not TIME")
	// Never emit a literal newline or unescaped delimiter in a single-quoted value.
	require.False(t, strings.Contains(bigQueryLiteral("a\nb"), "\n"))
}
