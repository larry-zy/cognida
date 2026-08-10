// Package mysql 提供用户领域的 MySQL 持久化实现
package mysql

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"cognida/internal/model/tenant"
	"cognida/internal/model/user"
	"cognida/internal/pkg/pagination"
)

// ========================================
// UserRepository 用户仓储实现
// ========================================

// userRepository 用户仓储实现
type userRepository struct {
	db *gorm.DB
}

// NewUserRepository 创建用户仓储
func NewUserRepository(db *gorm.DB) user.UserRepository {
	return &userRepository{db: db}
}

// dbCtx 返回本次操作应使用的 *gorm.DB：优先复用 ctx 中携带的事务句柄，
// 否则回退到 r.db。仓储必须经此访问 DB，才能参与上层 WithTransaction 开启的事务。
func (r *userRepository) dbCtx(ctx context.Context) *gorm.DB {
	return DBFromContext(ctx, r.db)
}

// Create 创建用户
func (r *userRepository) Create(ctx context.Context, u *user.User) error {
	var model UserModel
	model.FromDomain(u)
	return r.dbCtx(ctx).Create(&model).Error
}

// FindByID 根据ID查找用户
func (r *userRepository) FindByID(ctx context.Context, id int64) (*user.User, error) {
	var model UserModel
	err := r.dbCtx(ctx).
		Where("id = ?", id).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("用户不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	return model.ToDomain(), nil
}

// FindByEmail 根据邮箱查找用户
func (r *userRepository) FindByEmail(ctx context.Context, tenantID int64, email string) (*user.User, error) {
	var model UserModel
	err := r.dbCtx(ctx).
		Where("tenant_id = ? AND email = ?", tenantID, email).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("用户不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	return model.ToDomain(), nil
}

// FindByEmailOnly 根据邮箱查找用户（跨所有租户）
func (r *userRepository) FindByEmailOnly(ctx context.Context, email string) (*user.User, error) {
	var model UserModel
	err := r.dbCtx(ctx).
		Where("email = ?", email).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("用户不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	return model.ToDomain(), nil
}

// FindByUsername 根据用户名查找用户
func (r *userRepository) FindByUsername(ctx context.Context, tenantID int64, username string) (*user.User, error) {
	var model UserModel
	err := r.dbCtx(ctx).
		Where("tenant_id = ? AND username = ?", tenantID, username).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("用户不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	return model.ToDomain(), nil
}

// FindByTenantID 根据租户ID查找用户列表
func (r *userRepository) FindByTenantID(ctx context.Context, tenantID int64, page, pageSize int) ([]*user.User, int64, error) {
	var models []*UserModel
	var total int64

	db := r.dbCtx(ctx).Model(&UserModel{}).Where("tenant_id = ?", tenantID)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计用户数量失败: %w", err)
	}

	offset := (page - 1) * pageSize
	err := db.Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("查询用户列表失败: %w", err)
	}

	result := make([]*user.User, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, total, nil
}

// Update 更新用户
func (r *userRepository) Update(ctx context.Context, u *user.User) error {
	var model UserModel
	model.FromDomain(u)
	return r.dbCtx(ctx).Save(&model).Error
}

// UpdatePassword 更新密码
func (r *userRepository) UpdatePassword(ctx context.Context, userID int64, passwordHash string) error {
	return r.dbCtx(ctx).Model(&UserModel{}).
		Where("id = ?", userID).
		Update("password_hash", passwordHash).Error
}

// UpdateAvatar 更新头像
func (r *userRepository) UpdateAvatar(ctx context.Context, userID int64, avatar string) error {
	return r.dbCtx(ctx).Model(&UserModel{}).
		Where("id = ?", userID).
		Update("avatar", avatar).Error
}

// UpdateLastLogin 更新最后登录时间
func (r *userRepository) UpdateLastLogin(ctx context.Context, userID int64) error {
	return r.dbCtx(ctx).Model(&UserModel{}).
		Where("id = ?", userID).
		Update("last_login_at", gorm.Expr("NOW()")).Error
}

// UpdateStatus 更新用户状态
func (r *userRepository) UpdateStatus(ctx context.Context, userID int64, status int8) error {
	return r.dbCtx(ctx).Model(&UserModel{}).
		Where("id = ?", userID).
		Update("status", status).Error
}

// Delete 删除用户（软删除）
func (r *userRepository) Delete(ctx context.Context, userID int64) error {
	return r.dbCtx(ctx).Where("id = ?", userID).Delete(&UserModel{}).Error
}

// Exists 检查用户是否存在
func (r *userRepository) Exists(ctx context.Context, userID int64) (bool, error) {
	var count int64
	err := r.dbCtx(ctx).
		Model(&UserModel{}).
		Where("id = ?", userID).
		Count(&count).Error

	return count > 0, err
}

// ExistsByEmail 检查邮箱是否已存在
func (r *userRepository) ExistsByEmail(ctx context.Context, tenantID int64, email string) (bool, error) {
	var count int64
	err := r.dbCtx(ctx).
		Model(&UserModel{}).
		Where("tenant_id = ? AND email = ?", tenantID, email).
		Count(&count).Error

	return count > 0, err
}

// ExistsByUsername 检查用户名是否已存在
func (r *userRepository) ExistsByUsername(ctx context.Context, tenantID int64, username string) (bool, error) {
	var count int64
	err := r.dbCtx(ctx).
		Model(&UserModel{}).
		Where("tenant_id = ? AND username = ?", tenantID, username).
		Count(&count).Error

	return count > 0, err
}

// CountByTenantID 统计租户的用户数量
func (r *userRepository) CountByTenantID(ctx context.Context, tenantID int64) (int64, error) {
	var count int64
	err := r.dbCtx(ctx).
		Model(&UserModel{}).
		Where("tenant_id = ?", tenantID).
		Count(&count).Error

	return count, err
}

// ========================================
// RefreshTokenRepository 刷新令牌仓储实现
// ========================================

// refreshTokenRepository 刷新令牌仓储实现
type refreshTokenRepository struct {
	db *gorm.DB
}

// NewRefreshTokenRepository 创建刷新令牌仓储
func NewRefreshTokenRepository(db *gorm.DB) user.RefreshTokenRepository {
	return &refreshTokenRepository{db: db}
}

// dbCtx 返回本次操作应使用的 *gorm.DB：优先复用 ctx 中携带的事务句柄，否则回退到 r.db。
func (r *refreshTokenRepository) dbCtx(ctx context.Context) *gorm.DB {
	return DBFromContext(ctx, r.db)
}

// Create 创建刷新令牌
func (r *refreshTokenRepository) Create(ctx context.Context, token *user.RefreshToken) error {
	var model RefreshTokenModel
	model.FromDomain(token)
	return r.dbCtx(ctx).Create(&model).Error
}

// FindByHash 根据哈希查找令牌
func (r *refreshTokenRepository) FindByHash(ctx context.Context, tokenHash string) (*user.RefreshToken, error) {
	var model RefreshTokenModel
	err := r.dbCtx(ctx).
		Where("token_hash = ?", tokenHash).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("令牌不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询令牌失败: %w", err)
	}

	return model.ToDomain(), nil
}

// FindByUserID 根据用户ID查找令牌列表
func (r *refreshTokenRepository) FindByUserID(ctx context.Context, userID int64) ([]*user.RefreshToken, error) {
	var models []*RefreshTokenModel
	err := r.dbCtx(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&models).Error

	if err != nil {
		return nil, fmt.Errorf("查询令牌列表失败: %w", err)
	}

	result := make([]*user.RefreshToken, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, nil
}

// Delete 删除令牌
func (r *refreshTokenRepository) Delete(ctx context.Context, id int64) error {
	return r.dbCtx(ctx).Where("id = ?", id).Delete(&RefreshTokenModel{}).Error
}

// DeleteByUserID 删除用户的所有令牌
func (r *refreshTokenRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	return r.dbCtx(ctx).Where("user_id = ?", userID).Delete(&RefreshTokenModel{}).Error
}

// DeleteExpired 删除过期令牌
func (r *refreshTokenRepository) DeleteExpired(ctx context.Context) error {
	return r.dbCtx(ctx).
		Where("expires_at < NOW()").
		Delete(&RefreshTokenModel{}).Error
}

// RevokeUserTokens 撤销用户的所有令牌
func (r *refreshTokenRepository) RevokeUserTokens(ctx context.Context, userID int64) error {
	return r.DeleteByUserID(ctx, userID)
}

// ========================================
// TenantRepository 租户仓储实现
// ========================================

// tenantRepository 租户仓储实现
type tenantRepository struct {
	db *gorm.DB
}

// NewTenantRepository 创建租户仓储
func NewTenantRepository(db *gorm.DB) tenant.TenantRepository {
	return &tenantRepository{db: db}
}

// dbCtx 返回本次操作应使用的 *gorm.DB：优先复用 ctx 中携带的事务句柄，否则回退到 r.db。
func (r *tenantRepository) dbCtx(ctx context.Context) *gorm.DB {
	return DBFromContext(ctx, r.db)
}

// Create 创建租户
func (r *tenantRepository) Create(ctx context.Context, t *tenant.Tenant) error {
	var model TenantModel
	model.FromDomainForCreate(t)
	return r.dbCtx(ctx).Create(&model).Error
}

// FindByID 根据ID查找租户
func (r *tenantRepository) FindByID(ctx context.Context, id int64) (*tenant.Tenant, error) {
	var model TenantModel
	err := r.dbCtx(ctx).
		Where("id = ?", id).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("租户不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询租户失败: %w", err)
	}

	return model.ToDomain(), nil
}

// FindByAPIKey 根据API密钥查找租户
func (r *tenantRepository) FindByAPIKey(ctx context.Context, apiKey string) (*tenant.Tenant, error) {
	var model TenantModel
	err := r.dbCtx(ctx).
		Where("api_key = ?", apiKey).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("租户不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询租户失败: %w", err)
	}

	return model.ToDomain(), nil
}

// FindAll 查找所有租户列表
func (r *tenantRepository) FindAll(ctx context.Context, page, pageSize int) ([]*tenant.Tenant, int64, error) {
	var models []*TenantModel
	var total int64

	db := r.dbCtx(ctx).Model(&TenantModel{})

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计租户数量失败: %w", err)
	}

	offset := (page - 1) * pageSize
	err := db.Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("查询租户列表失败: %w", err)
	}

	result := make([]*tenant.Tenant, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, total, nil
}

// FindAllByCursor 游标（keyset）分页〔M5〕：与 FindAll 一致按 created_at DESC 排序，
// 追加 id DESC 作为唯一 tie-breaker（created_at 可能相同），保证翻页稳定；多取一条判定是否还有下一页。
func (r *tenantRepository) FindAllByCursor(ctx context.Context, cursor string, limit int) ([]*tenant.Tenant, string, error) {
	limit = pagination.NormalizeLimit(limit)

	db := r.dbCtx(ctx).Model(&TenantModel{})

	cur, err := pagination.Decode(cursor)
	if err != nil {
		return nil, "", fmt.Errorf("无效游标: %w", err)
	}
	if !cur.IsZero() {
		anchor, perr := time.Parse(time.RFC3339Nano, cur.Sort)
		if perr != nil {
			return nil, "", fmt.Errorf("无效游标锚点: %w", perr)
		}
		pred, args := pagination.KeysetPredicate("created_at", "id", anchor, cur.ID, pagination.Desc)
		db = db.Where(pred, args...)
	}

	var models []*TenantModel
	if err := db.Order("created_at DESC, id DESC").
		Limit(limit + 1).
		Find(&models).Error; err != nil {
		return nil, "", fmt.Errorf("查询租户列表失败: %w", err)
	}

	nextCursor := ""
	if len(models) > limit {
		last := models[limit-1]
		nextCursor = pagination.Cursor{Sort: last.CreatedAt.UTC().Format(time.RFC3339Nano), ID: last.ID}.Encode()
		models = models[:limit]
	}

	result := make([]*tenant.Tenant, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}
	return result, nextCursor, nil
}

// FindByStatus 根据状态查找租户列表
func (r *tenantRepository) FindByStatus(ctx context.Context, status string, page, pageSize int) ([]*tenant.Tenant, int64, error) {
	var models []*TenantModel
	var total int64

	db := r.dbCtx(ctx).Model(&TenantModel{}).Where("status = ?", status)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计租户数量失败: %w", err)
	}

	offset := (page - 1) * pageSize
	err := db.Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("查询租户列表失败: %w", err)
	}

	result := make([]*tenant.Tenant, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, total, nil
}

// Update 更新租户
func (r *tenantRepository) Update(ctx context.Context, t *tenant.Tenant) error {
	var model TenantModel
	model.FromDomain(t)
	return r.dbCtx(ctx).Save(&model).Error
}

// UpdateName 更新租户名称
func (r *tenantRepository) UpdateName(ctx context.Context, id int64, name string) error {
	return r.dbCtx(ctx).Model(&TenantModel{}).
		Where("id = ?", id).
		Update("name", name).Error
}

// UpdateDescription 更新租户描述
func (r *tenantRepository) UpdateDescription(ctx context.Context, id int64, description string) error {
	return r.dbCtx(ctx).Model(&TenantModel{}).
		Where("id = ?", id).
		Update("description", description).Error
}

// UpdateStatus 更新租户状态
func (r *tenantRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	return r.dbCtx(ctx).Model(&TenantModel{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// UpdateStorage 更新存储使用量
func (r *tenantRepository) UpdateStorage(ctx context.Context, id int64, storageUsed int64) error {
	return r.dbCtx(ctx).Model(&TenantModel{}).
		Where("id = ?", id).
		Update("storage_used", storageUsed).Error
}

// UpdateAPIKey 更新API密钥
func (r *tenantRepository) UpdateAPIKey(ctx context.Context, id int64, apiKey string) error {
	return r.dbCtx(ctx).Model(&TenantModel{}).
		Where("id = ?", id).
		Update("api_key", apiKey).Error
}

// Delete 删除租户（软删除）
func (r *tenantRepository) Delete(ctx context.Context, id int64) error {
	return r.dbCtx(ctx).Where("id = ?", id).Delete(&TenantModel{}).Error
}

// HardDelete 硬删除租户
func (r *tenantRepository) HardDelete(ctx context.Context, id int64) error {
	return r.dbCtx(ctx).Unscoped().Where("id = ?", id).Delete(&TenantModel{}).Error
}

// Exists 检查租户是否存在
func (r *tenantRepository) Exists(ctx context.Context, id int64) (bool, error) {
	var count int64
	err := r.dbCtx(ctx).
		Model(&TenantModel{}).
		Where("id = ?", id).
		Count(&count).Error

	return count > 0, err
}

// ExistsByName 检查租户名称是否已存在
func (r *tenantRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	var count int64
	err := r.dbCtx(ctx).
		Model(&TenantModel{}).
		Where("name = ?", name).
		Count(&count).Error

	return count > 0, err
}

// Count 统计租户总数
func (r *tenantRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.dbCtx(ctx).Model(&TenantModel{}).Count(&count).Error
	return count, err
}

// CountByStatus 统计指定状态的租户数量
func (r *tenantRepository) CountByStatus(ctx context.Context, status string) (int64, error) {
	var count int64
	err := r.dbCtx(ctx).
		Model(&TenantModel{}).
		Where("status = ?", status).
		Count(&count).Error
	return count, err
}

// GetStorageStats 获取存储统计
func (r *tenantRepository) GetStorageStats(ctx context.Context, tenantID int64) (*tenant.StorageStats, error) {
	stats := &tenant.StorageStats{}
	err := r.dbCtx(ctx).
		Model(&TenantModel{}).
		Where("id = ?", tenantID).
		Select("storage_used as total_size, 0 as kb_count").
		First(stats).Error

	if err != nil {
		return nil, fmt.Errorf("查询存储统计失败: %w", err)
	}

	return stats, nil
}

// ========================================
// TenantUserRepository 租户用户关联仓储实现
// ========================================

// tenantUserRepository 租户用户关联仓储实现
type tenantUserRepository struct {
	db *gorm.DB
}

// NewTenantUserRepository 创建租户用户关联仓储
func NewTenantUserRepository(db *gorm.DB) tenant.TenantUserRepository {
	return &tenantUserRepository{db: db}
}

// dbCtx 返回本次操作应使用的 *gorm.DB：优先复用 ctx 中携带的事务句柄，否则回退到 r.db。
func (r *tenantUserRepository) dbCtx(ctx context.Context) *gorm.DB {
	return DBFromContext(ctx, r.db)
}

// Create 创建租户用户关联
func (r *tenantUserRepository) Create(ctx context.Context, tenantUser *tenant.TenantUser) error {
	var model TenantUserModel
	model.FromDomain(tenantUser)
	return r.dbCtx(ctx).Create(&model).Error
}

// FindByTenantID 根据租户ID查找用户列表
func (r *tenantUserRepository) FindByTenantID(ctx context.Context, tenantID int64) ([]*tenant.TenantUser, error) {
	var models []*TenantUserModel
	err := r.dbCtx(ctx).
		Where("tenant_id = ?", tenantID).
		Find(&models).Error

	if err != nil {
		return nil, err
	}

	result := make([]*tenant.TenantUser, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, nil
}

// FindByUserID 根据用户ID查找租户列表
func (r *tenantUserRepository) FindByUserID(ctx context.Context, userID int64) ([]*tenant.TenantUser, error) {
	var models []*TenantUserModel
	err := r.dbCtx(ctx).
		Where("user_id = ?", userID).
		Find(&models).Error

	if err != nil {
		return nil, err
	}

	result := make([]*tenant.TenantUser, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, nil
}

// Find 查找特定的租户用户关联
func (r *tenantUserRepository) Find(ctx context.Context, tenantID, userID int64) (*tenant.TenantUser, error) {
	var model TenantUserModel
	err := r.dbCtx(ctx).
		Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("租户用户关联不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询租户用户关联失败: %w", err)
	}

	return model.ToDomain(), nil
}

// Update 更新租户用户关联
func (r *tenantUserRepository) Update(ctx context.Context, tenantUser *tenant.TenantUser) error {
	var model TenantUserModel
	model.FromDomain(tenantUser)
	return r.dbCtx(ctx).Save(&model).Error
}

// UpdateRole 更新用户角色
func (r *tenantUserRepository) UpdateRole(ctx context.Context, tenantID, userID int64, role string) error {
	return r.dbCtx(ctx).
		Model(&TenantUserModel{}).
		Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Update("role", role).Error
}

// UpdateStatus 更新用户状态
func (r *tenantUserRepository) UpdateStatus(ctx context.Context, tenantID, userID int64, status string) error {
	return r.dbCtx(ctx).
		Model(&TenantUserModel{}).
		Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Update("status", status).Error
}

// Delete 删除租户用户关联
func (r *tenantUserRepository) Delete(ctx context.Context, tenantID, userID int64) error {
	return r.dbCtx(ctx).
		Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Delete(&TenantUserModel{}).Error
}

// DeleteByTenantID 删除租户的所有用户关联
func (r *tenantUserRepository) DeleteByTenantID(ctx context.Context, tenantID int64) error {
	return r.dbCtx(ctx).
		Where("tenant_id = ?", tenantID).
		Delete(&TenantUserModel{}).Error
}

// DeleteByUserID 删除用户的所有租户关联
func (r *tenantUserRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	return r.dbCtx(ctx).
		Where("user_id = ?", userID).
		Delete(&TenantUserModel{}).Error
}

// Exists 检查租户用户关联是否存在
func (r *tenantUserRepository) Exists(ctx context.Context, tenantID, userID int64) (bool, error) {
	var count int64
	err := r.dbCtx(ctx).
		Model(&TenantUserModel{}).
		Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Count(&count).Error
	return count > 0, err
}

// CountByTenantID 统计租户的用户数量
func (r *tenantUserRepository) CountByTenantID(ctx context.Context, tenantID int64) (int64, error) {
	var count int64
	err := r.dbCtx(ctx).
		Model(&TenantUserModel{}).
		Where("tenant_id = ?", tenantID).
		Count(&count).Error
	return count, err
}

// GetOwners 获取租户的所有者
func (r *tenantUserRepository) GetOwners(ctx context.Context, tenantID int64) ([]*tenant.TenantUser, error) {
	var models []*TenantUserModel
	err := r.dbCtx(ctx).
		Where("tenant_id = ? AND role = ?", tenantID, "owner").
		Find(&models).Error

	if err != nil {
		return nil, err
	}

	result := make([]*tenant.TenantUser, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, nil
}

// GetAdmins 获取租户的管理员
func (r *tenantUserRepository) GetAdmins(ctx context.Context, tenantID int64) ([]*tenant.TenantUser, error) {
	var models []*TenantUserModel
	err := r.dbCtx(ctx).
		Where("tenant_id = ? AND role = ?", tenantID, "admin").
		Find(&models).Error

	if err != nil {
		return nil, err
	}

	result := make([]*tenant.TenantUser, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, nil
}
