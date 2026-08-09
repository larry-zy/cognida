package mysql

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"cognida/internal/model/user/rbac"
)

// ========================================
// 仓储类型别名
// ========================================
type (
	// RoleRepository 角色仓储接口
	RoleRepository = rbac.RoleRepository
	// UserRoleRepository 用户角色仓储接口
	UserRoleRepository = rbac.UserRoleRepository
	// PermissionRepository 权限仓储接口
	PermissionRepository = rbac.PermissionRepository
	// RolePermissionRepository 角色权限仓储接口
	RolePermissionRepository = rbac.RolePermissionRepository
	// ResourcePermissionRepository 资源权限仓储接口
	ResourcePermissionRepository = rbac.ResourcePermissionRepository
)

// ========================================
// 角色仓储实现
// ========================================

// roleRepository 角色仓储实现
type roleRepository struct {
	*BaseRepository
}

// NewRoleRepository 创建角色仓储
func NewRoleRepository(db *gorm.DB) RoleRepository {
	return &roleRepository{
		BaseRepository: NewBaseRepository(db, true),
	}
}

// Create 创建角色
func (r *roleRepository) Create(ctx context.Context, role *rbac.Role) error {
	return r.WithContext(ctx).Create(role).Error
}

// FindByID 根据ID查找角色
func (r *roleRepository) FindByID(ctx context.Context, id int64) (*rbac.Role, error) {
	var role rbac.Role
	// 按主键做管理级查找，跨租户合法：显式声明全局 Scope 以放行 fail-closed 守卫。
	err := r.WithGlobalAndSoftDeleteScope(ctx).Where("id = ?", id).First(&role).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("角色不存在")
		}
		return nil, fmt.Errorf("查询角色失败: %w", err)
	}
	return &role, nil
}

// FindByTenantID 根据租户ID查找角色列表
func (r *roleRepository) FindByTenantID(ctx context.Context, tenantID int64, page, pageSize int) ([]*rbac.Role, int64, error) {
	var roles []*rbac.Role
	var total int64

	query := r.WithTenantAndSoftDeleteScope(ctx, tenantID).Model(&rbac.Role{})

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("查询角色总数失败: %w", err)
	}

	offset := (page - 1) * pageSize
	if err := query.Order("level DESC, created_at ASC").Offset(offset).Limit(pageSize).Find(&roles).Error; err != nil {
		return nil, 0, fmt.Errorf("查询角色列表失败: %w", err)
	}

	return roles, total, nil
}

// FindByCode 根据租户ID和角色编码查找角色
func (r *roleRepository) FindByCode(ctx context.Context, tenantID int64, code string) (*rbac.Role, error) {
	var role rbac.Role
	err := r.WithTenantAndSoftDeleteScope(ctx, tenantID).
		Where("code = ?", code).
		First(&role).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("角色不存在")
		}
		return nil, fmt.Errorf("查询角色失败: %w", err)
	}
	return &role, nil
}

// Update 更新角色
func (r *roleRepository) Update(ctx context.Context, role *rbac.Role) error {
	role.UpdatedAt = time.Now()
	result := r.WithContext(ctx).Model(&rbac.Role{}).
		Where("id = ? AND deleted_at IS NULL", role.ID).
		Updates(map[string]interface{}{
			"name":        role.Name,
			"description": role.Description,
			"level":       role.Level,
			"status":      role.Status,
			"updated_at":  role.UpdatedAt,
		}).Error
	if result != nil {
		return result
	}
	return nil
}

// Delete 删除角色（软删除）
func (r *roleRepository) Delete(ctx context.Context, id int64) error {
	result := r.WithContext(ctx).Model(&rbac.Role{}).
		Where("id = ?", id).
		Update("deleted_at", time.Now()).Error
	if result != nil {
		return result
	}
	return nil
}

// List 分页查询角色列表
func (r *roleRepository) List(ctx context.Context, tenantID int64, page, pageSize int) ([]*rbac.Role, int64, error) {
	return r.FindByTenantID(ctx, tenantID, page, pageSize)
}

// Exists 检查角色是否存在
func (r *roleRepository) Exists(ctx context.Context, id int64) (bool, error) {
	var count int64
	err := r.WithContext(ctx).Model(&rbac.Role{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Count(&count).Error
	return count > 0, err
}

// CountByTenantID 统计租户的角色数量
func (r *roleRepository) CountByTenantID(ctx context.Context, tenantID int64) (int64, error) {
	var count int64
	err := r.WithTenantAndSoftDeleteScope(ctx, tenantID).
		Model(&rbac.Role{}).
		Count(&count).Error
	return count, err
}

// ========================================
// 用户角色仓储实现
// ========================================

// userRoleRepository 用户角色仓储实现
type userRoleRepository struct {
	*BaseRepository
}

// NewUserRoleRepository 创建用户角色仓储
func NewUserRoleRepository(db *gorm.DB) UserRoleRepository {
	return &userRoleRepository{
		BaseRepository: NewBaseRepository(db, true),
	}
}

// Create 创建用户角色关联
func (r *userRoleRepository) Create(ctx context.Context, userRole *rbac.UserRole) error {
	return r.WithContext(ctx).Create(userRole).Error
}

// FindByID 根据ID查找用户角色关联
func (r *userRoleRepository) FindByID(ctx context.Context, id int64) (*rbac.UserRole, error) {
	var userRole rbac.UserRole
	err := r.WithContext(ctx).
		Where("id = ?", id).
		Where("(expires_at IS NULL OR expires_at > ?)", time.Now()).
		First(&userRole).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("用户角色不存在")
		}
		return nil, fmt.Errorf("查询用户角色失败: %w", err)
	}
	return &userRole, nil
}

// FindByUserID 根据用户ID查找角色关联列表
func (r *userRoleRepository) FindByUserID(ctx context.Context, userID int64) ([]*rbac.UserRole, error) {
	var userRoles []*rbac.UserRole
	err := r.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("(expires_at IS NULL OR expires_at > ?)", time.Now()).
		Order("assigned_at DESC").
		Find(&userRoles).Error
	if err != nil {
		return nil, fmt.Errorf("查询用户角色列表失败: %w", err)
	}
	return userRoles, nil
}

// FindByTenantAndUserID 根据租户ID和用户ID查找其角色
func (r *userRoleRepository) FindByTenantAndUserID(ctx context.Context, tenantID, userID int64) (*rbac.UserRole, error) {
	var userRole rbac.UserRole
	err := r.WithTenantAndSoftDeleteScope(ctx, tenantID).
		Where("user_id = ?", userID).
		Where("(expires_at IS NULL OR expires_at > ?)", time.Now()).
		First(&userRole).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("用户角色不存在")
		}
		return nil, fmt.Errorf("查询用户角色失败: %w", err)
	}
	return &userRole, nil
}

// FindByUserAndRole 根据用户ID和角色ID查找关联
func (r *userRoleRepository) FindByUserAndRole(ctx context.Context, userID, roleID int64) (*rbac.UserRole, error) {
	var userRole rbac.UserRole
	err := r.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", userID, roleID).
		Where("(expires_at IS NULL OR expires_at > ?)", time.Now()).
		First(&userRole).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("用户角色不存在")
		}
		return nil, fmt.Errorf("查询用户角色失败: %w", err)
	}
	return &userRole, nil
}

// FindByRoleID 根据角色ID查找拥有该角色的用户列表
func (r *userRoleRepository) FindByRoleID(ctx context.Context, roleID int64, page, pageSize int) ([]*rbac.UserRole, int64, error) {
	var userRoles []*rbac.UserRole
	var total int64

	query := r.WithContext(ctx).Model(&rbac.UserRole{}).
		Where("role_id = ?", roleID).
		Where("(expires_at IS NULL OR expires_at > ?)", time.Now())

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("查询用户角色总数失败: %w", err)
	}

	offset := (page - 1) * pageSize
	if err := query.Order("assigned_at DESC").Offset(offset).Limit(pageSize).Find(&userRoles).Error; err != nil {
		return nil, 0, fmt.Errorf("查询用户角色列表失败: %w", err)
	}

	return userRoles, total, nil
}

// Update 更新用户角色（实际是删除旧的，创建新的）
func (r *userRoleRepository) Update(ctx context.Context, userRole *rbac.UserRole) error {
	// 先删除旧的角色关联
	if err := r.WithContext(ctx).
		Where("tenant_id = ? AND user_id = ?", userRole.TenantID, userRole.UserID).
		Delete(&rbac.UserRole{}).Error; err != nil {
		return fmt.Errorf("删除旧的用户角色失败: %w", err)
	}

	// 创建新的角色关联
	return r.Create(ctx, userRole)
}

// Delete 删除用户角色关联
func (r *userRoleRepository) Delete(ctx context.Context, id int64) error {
	result := r.WithContext(ctx).
		Where("id = ?", id).
		Delete(&rbac.UserRole{})
	if result.Error != nil {
		return fmt.Errorf("删除用户角色失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("用户角色不存在")
	}
	return nil
}

// DeleteByTenantAndUserID 删除租户下的用户角色关联
func (r *userRoleRepository) DeleteByTenantAndUserID(ctx context.Context, tenantID, userID int64) error {
	result := r.WithTenantScope(ctx, tenantID).
		Where("user_id = ?", userID).
		Delete(&rbac.UserRole{})
	if result.Error != nil {
		return fmt.Errorf("删除用户角色失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("用户角色不存在")
	}
	return nil
}

// DeleteByUserID 删除用户的所有角色关联
func (r *userRoleRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	return r.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&rbac.UserRole{}).Error
}

// DeleteExpired 删除过期的角色关联
func (r *userRoleRepository) DeleteExpired(ctx context.Context) error {
	return r.WithContext(ctx).
		Where("expires_at IS NOT NULL AND expires_at <= ?", time.Now()).
		Delete(&rbac.UserRole{}).Error
}

// DeleteByRoleID 删除角色的所有用户关联
func (r *userRoleRepository) DeleteByRoleID(ctx context.Context, roleID int64) error {
	return r.WithContext(ctx).
		Where("role_id = ?", roleID).
		Delete(&rbac.UserRole{}).Error
}

// ========================================
// 权限仓储实现
// ========================================

// permissionRepository 权限仓储实现
type permissionRepository struct {
	*BaseRepository
}

// NewPermissionRepository 创建权限仓储
func NewPermissionRepository(db *gorm.DB) PermissionRepository {
	return &permissionRepository{
		BaseRepository: NewBaseRepository(db, false),
	}
}

// Create 创建权限
func (r *permissionRepository) Create(ctx context.Context, permission *rbac.Permission) error {
	return r.WithContext(ctx).Create(permission).Error
}

// FindByID 根据ID查找权限
func (r *permissionRepository) FindByID(ctx context.Context, id int64) (*rbac.Permission, error) {
	var permission rbac.Permission
	err := r.WithContext(ctx).Where("id = ?", id).First(&permission).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("权限不存在")
		}
		return nil, fmt.Errorf("查询权限失败: %w", err)
	}
	return &permission, nil
}

// FindAll 查找所有权限
func (r *permissionRepository) FindAll(ctx context.Context) ([]*rbac.Permission, error) {
	var permissions []*rbac.Permission
	err := r.WithContext(ctx).
		Order("resource_type, action").
		Find(&permissions).Error
	if err != nil {
		return nil, fmt.Errorf("查询所有权限失败: %w", err)
	}
	return permissions, nil
}

// FindByResourceType 根据资源类型查找权限
func (r *permissionRepository) FindByResourceType(ctx context.Context, resourceType string) ([]*rbac.Permission, error) {
	var permissions []*rbac.Permission
	err := r.WithContext(ctx).
		Where("resource_type = ?", resourceType).
		Order("action").
		Find(&permissions).Error
	if err != nil {
		return nil, fmt.Errorf("查询资源权限失败: %w", err)
	}
	return permissions, nil
}

// FindByRoleID 根据角色ID查找其拥有的所有权限
func (r *permissionRepository) FindByRoleID(ctx context.Context, roleID int64) ([]*rbac.Permission, error) {
	var permissions []*rbac.Permission
	err := r.WithContext(ctx).
		Table("permissions p").
		Joins("INNER JOIN role_permissions rp ON p.id = rp.permission_id").
		Where("rp.role_id = ?", roleID).
		Order("p.resource_type, p.action").
		Find(&permissions).Error
	if err != nil {
		return nil, fmt.Errorf("查询角色权限失败: %w", err)
	}
	return permissions, nil
}

// Update 更新权限
func (r *permissionRepository) Update(ctx context.Context, permission *rbac.Permission) error {
	permission.UpdatedAt = time.Now()
	return r.WithContext(ctx).Save(permission).Error
}

// Delete 删除权限
func (r *permissionRepository) Delete(ctx context.Context, id int64) error {
	return r.WithContext(ctx).
		Where("id = ?", id).
		Delete(&rbac.Permission{}).Error
}

// Exists 检查权限是否存在
func (r *permissionRepository) Exists(ctx context.Context, id int64) (bool, error) {
	var count int64
	err := r.WithContext(ctx).Model(&rbac.Permission{}).
		Where("id = ?", id).
		Count(&count).Error
	return count > 0, err
}

// ========================================
// 角色权限仓储实现
// ========================================

// rolePermissionRepository 角色权限仓储实现
type rolePermissionRepository struct {
	*BaseRepository
}

// NewRolePermissionRepository 创建角色权限仓储
func NewRolePermissionRepository(db *gorm.DB) RolePermissionRepository {
	return &rolePermissionRepository{
		BaseRepository: NewBaseRepository(db, false),
	}
}

// Create 创建角色权限关联
func (r *rolePermissionRepository) Create(ctx context.Context, rolePermission *rbac.RolePermission) error {
	return r.WithContext(ctx).Create(rolePermission).Error
}

// FindByRoleID 根据角色ID查找权限关联列表
func (r *rolePermissionRepository) FindByRoleID(ctx context.Context, roleID int64) ([]*rbac.RolePermission, error) {
	var rolePermissions []*rbac.RolePermission
	err := r.WithContext(ctx).
		Where("role_id = ?", roleID).
		Find(&rolePermissions).Error
	if err != nil {
		return nil, fmt.Errorf("查询角色权限列表失败: %w", err)
	}
	return rolePermissions, nil
}

// Delete 删除角色权限关联
func (r *rolePermissionRepository) Delete(ctx context.Context, id int64) error {
	return r.WithContext(ctx).
		Where("id = ?", id).
		Delete(&rbac.RolePermission{}).Error
}

// DeleteByRoleIDAndPermissionID 删除角色的某个权限
func (r *rolePermissionRepository) DeleteByRoleIDAndPermissionID(ctx context.Context, roleID, permissionID int64) error {
	return r.WithContext(ctx).
		Where("role_id = ? AND permission_id = ?", roleID, permissionID).
		Delete(&rbac.RolePermission{}).Error
}

// DeleteByRoleID 删除角色的所有权限
func (r *rolePermissionRepository) DeleteByRoleID(ctx context.Context, roleID int64) error {
	return r.WithContext(ctx).
		Where("role_id = ?", roleID).
		Delete(&rbac.RolePermission{}).Error
}

// BatchCreate 批量创建角色权限关联
func (r *rolePermissionRepository) BatchCreate(ctx context.Context, rolePermissions []*rbac.RolePermission) error {
	if len(rolePermissions) == 0 {
		return nil
	}
	return r.WithContext(ctx).Create(rolePermissions).Error
}

// ========================================
// 资源级权限仓储实现
// ========================================

// resourcePermissionRepository 资源级权限仓储实现
type resourcePermissionRepository struct {
	*BaseRepository
}

// NewResourcePermissionRepository 创建资源级权限仓储
func NewResourcePermissionRepository(db *gorm.DB) ResourcePermissionRepository {
	return &resourcePermissionRepository{
		BaseRepository: NewBaseRepository(db, true),
	}
}

// Create 创建资源级权限
func (r *resourcePermissionRepository) Create(ctx context.Context, resourcePermission *rbac.ResourcePermission) error {
	return r.WithContext(ctx).Create(resourcePermission).Error
}

// FindByID 根据ID查找资源权限
func (r *resourcePermissionRepository) FindByID(ctx context.Context, id int64) (*rbac.ResourcePermission, error) {
	var resourcePermission rbac.ResourcePermission
	err := r.WithContext(ctx).
		Where("id = ?", id).
		Where("(expires_at IS NULL OR expires_at > ?)", time.Now()).
		First(&resourcePermission).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("资源权限不存在")
		}
		return nil, fmt.Errorf("查询资源权限失败: %w", err)
	}
	return &resourcePermission, nil
}

// Update 更新资源权限
func (r *resourcePermissionRepository) Update(ctx context.Context, resourcePermission *rbac.ResourcePermission) error {
	return r.WithContext(ctx).Save(resourcePermission).Error
}

// FindByUserID 根据用户ID查找其资源权限
func (r *resourcePermissionRepository) FindByUserID(ctx context.Context, userID int64) ([]*rbac.ResourcePermission, error) {
	var permissions []*rbac.ResourcePermission
	err := r.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("(expires_at IS NULL OR expires_at > ?)", time.Now()).
		Order("resource_type, resource_id").
		Find(&permissions).Error
	if err != nil {
		return nil, fmt.Errorf("查询用户资源权限失败: %w", err)
	}
	return permissions, nil
}

// FindByResource 根据资源查找权限列表
func (r *resourcePermissionRepository) FindByResource(ctx context.Context, resourceType, resourceID string) ([]*rbac.ResourcePermission, error) {
	var permissions []*rbac.ResourcePermission
	err := r.WithContext(ctx).
		Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
		Where("(expires_at IS NULL OR expires_at > ?)", time.Now()).
		Order("permission_type").
		Find(&permissions).Error
	if err != nil {
		return nil, fmt.Errorf("查询资源权限失败: %w", err)
	}
	return permissions, nil
}

// FindByResourceAndTenant 根据资源和租户查找权限列表
func (r *resourcePermissionRepository) FindByResourceAndTenant(ctx context.Context, tenantID int64, resourceType, resourceID string) ([]*rbac.ResourcePermission, error) {
	var permissions []*rbac.ResourcePermission
	err := r.WithTenantScope(ctx, tenantID).
		Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
		Where("(expires_at IS NULL OR expires_at > ?)", time.Now()).
		Order("permission_type").
		Find(&permissions).Error
	if err != nil {
		return nil, fmt.Errorf("查询资源权限失败: %w", err)
	}
	return permissions, nil
}

// CheckPermission 检查用户对资源是否有指定权限
func (r *resourcePermissionRepository) CheckPermission(ctx context.Context, tenantID, userID int64, resourceType, resourceID, permissionType string) (bool, error) {
	var count int64
	err := r.WithTenantScope(ctx, tenantID).
		Model(&rbac.ResourcePermission{}).
		Where("user_id = ? AND resource_type = ? AND resource_id = ? AND permission_type = ?", userID, resourceType, resourceID, permissionType).
		Where("(expires_at IS NULL OR expires_at > ?)", time.Now()).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("检查资源权限失败: %w", err)
	}
	return count > 0, nil
}

// Delete 删除资源级权限
func (r *resourcePermissionRepository) Delete(ctx context.Context, id int64) error {
	result := r.WithContext(ctx).
		Where("id = ?", id).
		Delete(&rbac.ResourcePermission{})
	if result.Error != nil {
		return fmt.Errorf("删除资源级权限失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("资源级权限不存在")
	}
	return nil
}

// DeleteByResource 删除资源的所有权限
func (r *resourcePermissionRepository) DeleteByResource(ctx context.Context, tenantID int64, resourceType, resourceID string) error {
	return r.WithTenantScope(ctx, tenantID).
		Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
		Delete(&rbac.ResourcePermission{}).Error
}

// DeleteByUserID 删除用户的所有资源权限
func (r *resourcePermissionRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	return r.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&rbac.ResourcePermission{}).Error
}

// DeleteExpired 删除过期的资源权限
func (r *resourcePermissionRepository) DeleteExpired(ctx context.Context) error {
	return r.WithContext(ctx).
		Where("expires_at IS NOT NULL AND expires_at <= ?", time.Now()).
		Delete(&rbac.ResourcePermission{}).Error
}

