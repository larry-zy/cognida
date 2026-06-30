// Package rbac provides RBAC repository interfaces
package rbac

import "context"

// ========================================
// RoleRepository 角色仓储接口
// ========================================

// RoleRepository 角色仓储接口
type RoleRepository interface {
	// Create 创建角色
	Create(ctx context.Context, role *Role) error

	// FindByID 根据ID查找角色
	FindByID(ctx context.Context, id int64) (*Role, error)

	// FindByCode 根据代码查找角色
	FindByCode(ctx context.Context, tenantID int64, code string) (*Role, error)

	// FindByTenantID 根据租户ID查找角色列表
	FindByTenantID(ctx context.Context, tenantID int64, page, pageSize int) ([]*Role, int64, error)

	// Update 更新角色
	Update(ctx context.Context, role *Role) error

	// Delete 删除角色（软删除）
	Delete(ctx context.Context, id int64) error

	// Exists 检查角色是否存在
	Exists(ctx context.Context, id int64) (bool, error)

	// CountByTenantID 统计租户的角色数量
	CountByTenantID(ctx context.Context, tenantID int64) (int64, error)
}

// ========================================
// UserRoleRepository 用户角色仓储接口
// ========================================

// UserRoleRepository 用户角色仓储接口
type UserRoleRepository interface {
	// Create 创建用户角色关联
	Create(ctx context.Context, userRole *UserRole) error

	// FindByID 根据ID查找用户角色关联
	FindByID(ctx context.Context, id int64) (*UserRole, error)

	// FindByUserID 根据用户ID查找角色关联列表
	FindByUserID(ctx context.Context, userID int64) ([]*UserRole, error)

	// FindByUserAndRole 根据用户ID和角色ID查找关联
	FindByUserAndRole(ctx context.Context, userID, roleID int64) (*UserRole, error)

	// Update 更新用户角色关联
	Update(ctx context.Context, userRole *UserRole) error

	// Delete 删除用户角色关联
	Delete(ctx context.Context, id int64) error

	// DeleteByUserID 删除用户的所有角色关联
	DeleteByUserID(ctx context.Context, userID int64) error

	// DeleteExpired 删除过期的角色关联
	DeleteExpired(ctx context.Context) error
}

// ========================================
// PermissionRepository 权限仓储接口
// ========================================

// PermissionRepository 权限仓储接口
type PermissionRepository interface {
	// Create 创建权限
	Create(ctx context.Context, permission *Permission) error

	// FindByID 根据ID查找权限
	FindByID(ctx context.Context, id int64) (*Permission, error)

	// FindAll 查找所有权限
	FindAll(ctx context.Context) ([]*Permission, error)

	// FindByResourceType 根据资源类型查找权限列表
	FindByResourceType(ctx context.Context, resourceType string) ([]*Permission, error)

	// Update 更新权限
	Update(ctx context.Context, permission *Permission) error

	// Delete 删除权限
	Delete(ctx context.Context, id int64) error

	// Exists 检查权限是否存在
	Exists(ctx context.Context, id int64) (bool, error)
}

// ========================================
// RolePermissionRepository 角色权限仓储接口
// ========================================

// RolePermissionRepository 角色权限仓储接口
type RolePermissionRepository interface {
	// Create 创建角色权限关联
	Create(ctx context.Context, rolePermission *RolePermission) error

	// FindByRoleID 根据角色ID查找权限ID列表
	FindByRoleID(ctx context.Context, roleID int64) ([]*RolePermission, error)

	// DeleteByRoleID 删除角色的所有权限关联
	DeleteByRoleID(ctx context.Context, roleID int64) error

	// Delete 删除角色权限关联
	Delete(ctx context.Context, id int64) error
}

// ========================================
// ResourcePermissionRepository 资源权限仓储接口
// ========================================

// ResourcePermissionRepository 资源权限仓储接口
type ResourcePermissionRepository interface {
	// Create 创建资源权限
	Create(ctx context.Context, resourcePermission *ResourcePermission) error

	// FindByID 根据ID查找资源权限
	FindByID(ctx context.Context, id int64) (*ResourcePermission, error)

	// FindByUserID 根据用户ID查找资源权限列表
	FindByUserID(ctx context.Context, userID int64) ([]*ResourcePermission, error)

	// FindByResource 根据资源类型和资源ID查找权限列表
	FindByResource(ctx context.Context, resourceType, resourceID string) ([]*ResourcePermission, error)

	// Update 更新资源权限
	Update(ctx context.Context, resourcePermission *ResourcePermission) error

	// Delete 删除资源权限
	Delete(ctx context.Context, id int64) error

	// DeleteByUserID 删除用户的所有资源权限
	DeleteByUserID(ctx context.Context, userID int64) error

	// DeleteExpired 删除过期的资源权限
	DeleteExpired(ctx context.Context) error
}
