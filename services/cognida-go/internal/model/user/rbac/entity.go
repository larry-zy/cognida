// Package rbac provides RBAC (Role-Based Access Control) domain entities
package rbac

import "time"

// ========================================
// Permission 权限实体
// ========================================

// Permission 权限实体
type Permission struct {
	ID           int64     `json:"id" db:"id"`
	ResourceType string    `json:"resource_type" db:"resource_type"` // kb/session/document/user/role/tenant
	Action       string    `json:"action" db:"action"`               // create/read/update/delete/assign
	Description  string    `json:"description" db:"description"`
	IsSystem     bool      `json:"is_system" db:"is_system"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// ========================================
// Role 角色实体
// ========================================

// Role 角色实体
type Role struct {
	ID          int64      `json:"id" db:"id"`
	TenantID    int64      `json:"tenant_id" db:"tenant_id"`
	Name        string     `json:"name" db:"name"`
	Code        string     `json:"code" db:"code"`         // owner/admin/user
	Description string     `json:"description" db:"description"`
	IsSystem    bool       `json:"is_system" db:"is_system"`
	IsDefault   bool       `json:"is_default" db:"is_default"`
	Level       int        `json:"level" db:"level"`     // 角色层级，数字越大权限越高
	Status      string     `json:"status" db:"status"`   // active/inactive
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

// IsActive checks if the role is active
func (r *Role) IsActive() bool {
	return r.Status == "active"
}

// IsSystemRole checks if this is a system role
func (r *Role) IsSystemRole() bool {
	return r.IsSystem
}

// ========================================
// UserRole 用户角色关联
// ========================================

// UserRole 用户角色关联
type UserRole struct {
	ID         int64      `json:"id" db:"id"`
	TenantID   int64      `json:"tenant_id" db:"tenant_id"`
	UserID     int64      `json:"user_id" db:"user_id"`
	RoleID     int64      `json:"role_id" db:"role_id"`
	AssignedBy *int64     `json:"assigned_by,omitempty" db:"assigned_by"`
	AssignedAt time.Time  `json:"assigned_at" db:"assigned_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty" db:"expires_at"`
}

// IsExpired checks if the role assignment has expired
func (ur *UserRole) IsExpired() bool {
	if ur.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*ur.ExpiresAt)
}

// ========================================
// RolePermission 角色权限关联
// ========================================

// RolePermission 角色权限关联
type RolePermission struct {
	ID           int64     `json:"id" db:"id"`
	RoleID       int64     `json:"role_id" db:"role_id"`
	PermissionID int64     `json:"permission_id" db:"permission_id"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// ========================================
// ResourcePermission 资源级权限
// ========================================

// ResourcePermission 资源级权限
type ResourcePermission struct {
	ID             int64      `json:"id" db:"id"`
	TenantID       int64      `json:"tenant_id" db:"tenant_id"`
	UserID         int64      `json:"user_id" db:"user_id"`
	ResourceType   string     `json:"resource_type" db:"resource_type"`     // kb/session/document
	ResourceID     string     `json:"resource_id" db:"resource_id"`         // 资源ID
	PermissionType string     `json:"permission_type" db:"permission_type"` // read/write/delete/admin
	GrantedBy      *int64     `json:"granted_by,omitempty" db:"granted_by"`
	GrantedAt      time.Time  `json:"granted_at" db:"granted_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty" db:"expires_at"`
}

// IsExpired checks if the permission has expired
func (rp *ResourcePermission) IsExpired() bool {
	if rp.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*rp.ExpiresAt)
}

// ========================================
// View Models (视图模型)
// ========================================

// UserRolesView 用户角色视图数据
type UserRolesView struct {
	UserID      int64      `json:"user_id" db:"user_id"`
	TenantID    int64      `json:"tenant_id" db:"tenant_id"`
	Username    string     `json:"username" db:"username"`
	Email       string     `json:"email" db:"email"`
	Status      int8       `json:"status" db:"status"`
	RoleID      *int64     `json:"role_id,omitempty" db:"role_id"`
	RoleName    *string    `json:"role_name,omitempty" db:"role_name"`
	RoleCode    *string    `json:"role_code,omitempty" db:"role_code"`
	RoleLevel   *int       `json:"role_level,omitempty" db:"role_level"`
	AssignedAt  *time.Time `json:"assigned_at,omitempty" db:"assigned_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty" db:"expires_at"`
}

// UserPermissionsView 用户权限视图数据
type UserPermissionsView struct {
	UserID       int64  `json:"user_id" db:"user_id"`
	TenantID     int64  `json:"tenant_id" db:"tenant_id"`
	Username     string `json:"username" db:"username"`
	ResourceType string `json:"resource_type" db:"resource_type"`
	Action       string `json:"action" db:"action"`
	RoleCode     string `json:"role_code" db:"role_code"`
	RoleLevel    int    `json:"role_level" db:"role_level"`
}
