// Package user 提供用户领域的 DTO 定义
package user

import "time"

// ========================================
// 请求/响应 DTO
// ========================================

// RegisterRequest 用户注册请求
type RegisterRequest struct {
	TenantID int64  `json:"tenant_id"` // 租户ID，可选（为空时自动创建）
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6,max=100"`
}

// LoginRequest 用户登录请求
type LoginRequest struct {
	TenantID int64  `json:"tenant_id"` // 租户ID，可选（为空时自动查找）
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// RefreshTokenRequest 刷新Token请求
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// AuthResponse 认证响应
type AuthResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresAt    int64    `json:"expires_at"`
	User         UserInfo `json:"user"`
	TenantID     int64    `json:"tenant_id,omitempty"`
}

// UpdateUserRequest 更新用户请求
type UpdateUserRequest struct {
	Username string `json:"username" binding:"omitempty,min=3,max=50"`
	Email    string `json:"email" binding:"omitempty,email"`
	Avatar   string `json:"avatar" binding:"omitempty,max=500"`
}

// UserResponse 用户响应
type UserResponse struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Avatar    string    `json:"avatar"`
	Status    int8      `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	TenantID  int64     `json:"tenant_id,omitempty"`
}

// ========================================
// 权限模块 (RBAC)
// ========================================

// Permission 权限实体
type Permission struct {
	ID          int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	ResourceType string   `json:"resource_type" gorm:"type:varchar(50);not null;index:idx_resource_type"` // kb/session/document/user/role/tenant
	Action      string    `json:"action" gorm:"type:varchar(50);not null;index:idx_action"`               // create/read/update/delete/assign
	Description string    `json:"description" gorm:"type:varchar(255)"`
	IsSystem    bool      `json:"is_system" gorm:"default:false"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}


// Role 角色实体
type Role struct {
	ID          int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID    int64      `json:"tenant_id" gorm:"not null;index:idx_tenant_id"`
	Name        string     `json:"name" gorm:"type:varchar(100);not null"`
	Code        string     `json:"code" gorm:"type:varchar(50);not null;index:idx_code"`         // owner/admin/user
	Description string     `json:"description" gorm:"type:varchar(255)"`
	IsSystem    bool       `json:"is_system" gorm:"default:false"`
	IsDefault   bool       `json:"is_default" gorm:"default:false"`
	Level       int        `json:"level" gorm:"default:0"`                       // 角色层级，数字越大权限越高
	Status      string     `json:"status" gorm:"type:varchar(20);default:'active'"` // active/inactive
	CreatedAt   time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty" gorm:"index"`
}


// UserRole 用户角色关联
type UserRole struct {
	ID         int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID   int64      `json:"tenant_id" gorm:"not null;index:idx_tenant_user;uniqueIndex:uk_tenant_user"`
	UserID     int64      `json:"user_id" gorm:"not null;index:idx_user_id;uniqueIndex:uk_tenant_user"`
	RoleID     int64      `json:"role_id" gorm:"not null;index:idx_role_id"`
	AssignedBy *int64     `json:"assigned_by,omitempty"`
	AssignedAt time.Time  `json:"assigned_at" gorm:"autoCreateTime"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}


// RolePermission 角色权限关联
type RolePermission struct {
	ID           int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	RoleID       int64     `json:"role_id" gorm:"not null;index:idx_role_id"`
	PermissionID int64     `json:"permission_id" gorm:"not null;index:idx_permission_id"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
}


// ResourcePermission 资源级权限
type ResourcePermission struct {
	ID             int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID       int64      `json:"tenant_id" gorm:"not null;index:idx_tenant_id"`
	UserID         int64      `json:"user_id" gorm:"not null;index:idx_user_id"`
	ResourceType   string     `json:"resource_type" gorm:"type:varchar(50);not null;index:idx_resource_type"` // kb/session/document
	ResourceID     string     `json:"resource_id" gorm:"type:varchar(100);not null;index:idx_resource_id"`
	PermissionType string     `json:"permission_type" gorm:"type:varchar(20);not null"` // read/write/delete/admin
	GrantedBy      *int64     `json:"granted_by,omitempty"`
	GrantedAt      time.Time  `json:"granted_at" gorm:"autoCreateTime"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}


// PermissionAuditLog 权限变更审计日志
type PermissionAuditLog struct {
	ID            int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID      *int64    `json:"tenant_id,omitempty" gorm:"index"`
	UserID        *int64    `json:"user_id,omitempty" gorm:"index"`
	OperatorID    int64     `json:"operator_id" gorm:"not null"`
	OperationType string    `json:"operation_type" gorm:"type:varchar(50);not null"` // grant_role/revoke_role/modify_role
	TargetType    string    `json:"target_type" gorm:"type:varchar(50);not null"`        // role/resource
	TargetID      string    `json:"target_id" gorm:"type:varchar(100);not null"`
	BeforeValue   string    `json:"before_value,omitempty" gorm:"type:text"` // JSON
	AfterValue    string    `json:"after_value,omitempty" gorm:"type:text"`   // JSON
	Reason        string    `json:"reason,omitempty" gorm:"type:text"`
	IPAddress     string    `json:"ip_address,omitempty" gorm:"type:varchar(50)"`
	CreatedAt     time.Time `json:"created_at" gorm:"autoCreateTime"`
}


// ========================================
// RBAC DTO
// ========================================

// CreateRoleRequest 创建角色请求
type CreateRoleRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=100"`
	Code        string `json:"code" binding:"required,min=1,max=50"`
	Description string `json:"description" binding:"max:500"`
	Level       int    `json:"level" binding:"min=0,max=100"`
}

// UpdateRoleRequest 更新角色请求
type UpdateRoleRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=100"`
	Description string `json:"description" binding:"max:500"`
	Level       int    `json:"level" binding:"min=0,max=100"`
	Status      string `json:"status" binding:"oneof=active inactive"`
}

// AssignRoleRequest 分配角色请求
type AssignRoleRequest struct {
	TenantID  int64  `json:"tenant_id" binding:"required"` // 租户ID
	UserID    int64  `json:"user_id" binding:"required"`
	RoleID    int64  `json:"role_id" binding:"required"`
	Reason    string `json:"reason" binding:"max:500"`
	ExpiresAt *int64 `json:"expires_at,omitempty"` // Unix timestamp
}

// RoleListResponse 角色列表响应
type RoleListResponse struct {
	Roles []*RoleInfo `json:"roles"`
	Total int64       `json:"total"`
}

// RoleInfo 角色信息（不含敏感信息）
type RoleInfo struct {
	ID          int64  `json:"id"`
	TenantID    int64  `json:"tenant_id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description"`
	IsSystem    bool   `json:"is_system"`
	IsDefault   bool   `json:"is_default"`
	Level       int    `json:"level"`
	Status      string `json:"status"`
	UserCount   int    `json:"user_count,omitempty"` // 拥有此角色的用户数量
}

// UserPermissionCheckRequest 用户权限检查请求
type UserPermissionCheckRequest struct {
	ResourceType string `json:"resource_type" binding:"required"`
	Action       string `json:"action" binding:"required"`
}

// CheckPermissionResponse 权限检查响应
type CheckPermissionResponse struct {
	HasPermission bool     `json:"has_permission"`
	Permissions   []string `json:"permissions,omitempty"` // 用户拥有的所有权限列表
}
