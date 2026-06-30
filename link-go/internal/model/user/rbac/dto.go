// Package rbac provides RBAC (Role-Based Access Control) DTO types
package rbac

// ========================================
// 请求/响应 DTO
// ========================================

// CreateRoleRequest 创建角色请求
type CreateRoleRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=100"`
	Code        string `json:"code" binding:"required,min=1,max=50"`
	Description string `json:"description" binding:"max=500"`
	Level       int    `json:"level" binding:"min=0,max=100"`
}

// UpdateRoleRequest 更新角色请求
type UpdateRoleRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=100"`
	Description string `json:"description" binding:"max=500"`
	Level       int    `json:"level" binding:"min=0,max=100"`
	Status      string `json:"status" binding:"oneof=active inactive"`
}

// AssignRoleRequest 分配角色请求
type AssignRoleRequest struct {
	TenantID  int64  `json:"tenant_id" binding:"required"` // 租户ID
	UserID    int64  `json:"user_id" binding:"required"`
	RoleID    int64  `json:"role_id" binding:"required"`
	Reason    string `json:"reason" binding:"max=500"`
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
