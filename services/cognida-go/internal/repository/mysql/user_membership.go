package mysql

import (
	"cognida/internal/model/user"
	"cognida/internal/pkg/pagination"
	"context"
	"fmt"
	"gorm.io/gorm/clause"
	"time"
)

func (r *userRepository) ListMemberships(ctx context.Context, userID int64, cursor string, limit int) ([]*user.Membership, string, error) {
	c, err := pagination.Decode(cursor)
	if err != nil {
		return nil, "", err
	}
	limit = pagination.NormalizeLimit(limit)
	q := r.dbCtx(ctx).Table("tenant_users AS m").Select("t.id, t.name, m.role, t.status").Joins("JOIN tenants t ON t.id = m.tenant_id").Joins("JOIN users u ON u.id = m.user_id").Where("m.user_id = ? AND m.status = 'active' AND t.status = 'active' AND t.deleted_at IS NULL AND u.status = 1 AND u.deleted_at IS NULL", userID)
	if c.ID != 0 {
		q = q.Where("t.id > ?", c.ID)
	}
	rows := []*user.Membership{}
	if err := q.Order("t.id ASC").Limit(limit + 1).Scan(&rows).Error; err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = (pagination.Cursor{ID: rows[len(rows)-1].ID}).Encode()
	}
	return rows, next, nil
}
func (r *userRepository) FindMembership(ctx context.Context, userID, tenantID int64) (*user.Membership, error) {
	if userID <= 0 || tenantID <= 0 {
		return nil, fmt.Errorf("invalid membership identity")
	}
	var row user.Membership
	result := r.dbCtx(ctx).Table("tenant_users AS m").Select("t.id, t.name, m.role, t.status").Joins("JOIN tenants t ON t.id = m.tenant_id").Joins("JOIN users u ON u.id = m.user_id").Where("m.user_id = ? AND m.tenant_id = ? AND m.status = 'active' AND t.status = 'active' AND t.deleted_at IS NULL AND u.status = 1 AND u.deleted_at IS NULL", userID, tenantID).Take(&row)
	if result.Error != nil {
		return nil, result.Error
	}
	return &row, nil
}
func (r *userRepository) AddMembership(ctx context.Context, userID, tenantID int64, role string) error {
	if userID <= 0 || tenantID <= 0 || (role != "owner" && role != "admin" && role != "member") {
		return fmt.Errorf("invalid membership")
	}
	return r.dbCtx(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&TenantUserModel{TenantID: tenantID, UserID: userID, Role: role, Status: "active", CreatedAt: time.Now()}).Error
}
