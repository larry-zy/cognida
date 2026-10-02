package mysql

import (
	"cognida/internal/model/semantic"
	"cognida/internal/pkg/pagination"
	"context"
	"fmt"
)

func (r *SemanticRepository) ListModels(ctx context.Context, tenantID int64, status, cursor string, limit int) ([]*semantic.SemanticModel, string, error) {
	if tenantID <= 0 {
		return nil, "", fmt.Errorf("invalid tenant")
	}
	c, err := pagination.Decode(cursor)
	if err != nil {
		return nil, "", err
	}
	limit = pagination.NormalizeLimit(limit)
	q := r.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID)
	if status != "all" {
		q = q.Where("status = ?", status)
	}
	if c.Sort != "" {
		q = q.Where("id > ?", c.Sort)
	}
	var rows []*SemanticModelModel
	if err := q.Order("id ASC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = (pagination.Cursor{Sort: rows[len(rows)-1].ID}).Encode()
	}
	items := make([]*semantic.SemanticModel, len(rows))
	for i, row := range rows {
		items[i] = row.ToDomain()
	}
	return items, next, nil
}
