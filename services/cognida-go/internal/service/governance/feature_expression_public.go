package governance

import (
	d "cognida/internal/model/governance"
	"context"
)

// EvaluateBoundedExpression shares the closed, exact-decimal expression contract
// with managed warehouse projections. It performs no I/O or model calls.
func EvaluateBoundedExpression(ctx context.Context, e d.FeatureExpression, columns []string, row []any) (any, error) {
	count := 0
	if err := validateExpression(e, 0, &count); err != nil {
		return nil, err
	}
	if len(row) != len(columns) {
		return nil, d.ErrInvalid
	}
	indexes := map[string]int{}
	for i, c := range columns {
		if _, ok := indexes[c]; ok {
			return nil, d.ErrInvalid
		}
		indexes[c] = i
	}
	return evalFeatureExpression(ctx, e, row, indexes)
}
