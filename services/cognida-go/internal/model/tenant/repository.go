// Package tenant 提供租户领域的仓储接口定义
package tenant

import "context"

// ========================================
// TenantRepository 租户仓储接口
// ========================================

// TenantRepository 租户仓储接口
type TenantRepository interface {
	// Create 创建租户
	Create(ctx context.Context, tenant *Tenant) error

	// FindByID 根据ID查找租户
	FindByID(ctx context.Context, id int64) (*Tenant, error)

	// FindByAPIKey 根据API密钥查找租户
	FindByAPIKey(ctx context.Context, apiKey string) (*Tenant, error)

	// FindAll 查找所有租户列表
	FindAll(ctx context.Context, page, pageSize int) ([]*Tenant, int64, error)

	// FindAllByCursor 游标（keyset）分页〔M5〕：按 created_at DESC, id DESC 稳定排序，
	// 以 cursor 为锚点取下一页。返回本页租户与「下一页游标」；nextCursor 为空表示已到末页。不返回总数。
	FindAllByCursor(ctx context.Context, cursor string, limit int) (tenants []*Tenant, nextCursor string, err error)

	// FindByStatus 根据状态查找租户列表
	FindByStatus(ctx context.Context, status string, page, pageSize int) ([]*Tenant, int64, error)

	// Update 更新租户
	Update(ctx context.Context, tenant *Tenant) error

	// UpdateName 更新租户名称
	UpdateName(ctx context.Context, id int64, name string) error

	// UpdateDescription 更新租户描述
	UpdateDescription(ctx context.Context, id int64, description string) error

	// UpdateStatus 更新租户状态
	UpdateStatus(ctx context.Context, id int64, status string) error

	// UpdateStorage 更新存储使用量
	UpdateStorage(ctx context.Context, id int64, storageUsed int64) error

	// UpdateAPIKey 更新API密钥
	UpdateAPIKey(ctx context.Context, id int64, apiKey string) error

	// Delete 删除租户（软删除）
	Delete(ctx context.Context, id int64) error

	// HardDelete 硬删除租户
	HardDelete(ctx context.Context, id int64) error

	// Exists 检查租户是否存在
	Exists(ctx context.Context, id int64) (bool, error)

	// ExistsByName 检查租户名称是否已存在
	ExistsByName(ctx context.Context, name string) (bool, error)

	// Count 统计租户总数
	Count(ctx context.Context) (int64, error)

	// CountByStatus 统计指定状态的租户数量
	CountByStatus(ctx context.Context, status string) (int64, error)

	// GetStorageStats 获取存储统计
	GetStorageStats(ctx context.Context, tenantID int64) (*StorageStats, error)
}

// ========================================
// TenantUserRepository 租户用户关联仓储接口
// ========================================

// TenantUserRepository 租户用户关联仓储接口
type TenantUserRepository interface {
	// Create 创建租户用户关联
	Create(ctx context.Context, tenantUser *TenantUser) error

	// FindByTenantID 根据租户ID查找用户列表
	FindByTenantID(ctx context.Context, tenantID int64) ([]*TenantUser, error)

	// FindByUserID 根据用户ID查找租户列表
	FindByUserID(ctx context.Context, userID int64) ([]*TenantUser, error)

	// Find 查找特定的租户用户关联
	Find(ctx context.Context, tenantID, userID int64) (*TenantUser, error)

	// Update 更新租户用户关联
	Update(ctx context.Context, tenantUser *TenantUser) error

	// UpdateRole 更新用户角色
	UpdateRole(ctx context.Context, tenantID, userID int64, role string) error

	// UpdateStatus 更新用户状态
	UpdateStatus(ctx context.Context, tenantID, userID int64, status string) error

	// Delete 删除租户用户关联
	Delete(ctx context.Context, tenantID, userID int64) error

	// DeleteByTenantID 删除租户的所有用户关联
	DeleteByTenantID(ctx context.Context, tenantID int64) error

	// DeleteByUserID 删除用户的所有租户关联
	DeleteByUserID(ctx context.Context, userID int64) error

	// Exists 检查租户用户关联是否存在
	Exists(ctx context.Context, tenantID, userID int64) (bool, error)

	// CountByTenantID 统计租户的用户数量
	CountByTenantID(ctx context.Context, tenantID int64) (int64, error)

	// GetOwners 获取租户的所有者
	GetOwners(ctx context.Context, tenantID int64) ([]*TenantUser, error)

	// GetAdmins 获取租户的管理员
	GetAdmins(ctx context.Context, tenantID int64) ([]*TenantUser, error)
}
