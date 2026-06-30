// Package tenant 提供租户领域的 DTO 定义
package tenant

// ========================================
// 请求/响应 DTO
// ========================================

// CreateTenantRequest 创建租户请求
type CreateTenantRequest struct {
	Name         string `json:"name" binding:"required,min=2,max=255"`
	Description  string `json:"description" binding:"max:500"`
	Business     string `json:"business" binding:"required,max=255"`
	StorageQuota int64  `json:"storage_quota" binding:"min=0"`
}

// UpdateTenantRequest 更新租户请求
type UpdateTenantRequest struct {
	Name        string `json:"name" binding:"omitempty,min=2,max=255"`
	Description string `json:"description" binding:"omitempty,max:500"`
	Business    string `json:"business" binding:"omitempty,max=255"`
	Status      string `json:"status" binding:"omitempty,oneof=active suspended"`
}
