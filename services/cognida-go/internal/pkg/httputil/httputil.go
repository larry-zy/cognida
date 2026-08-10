// Package httputil 汇集 HTTP 入站处理的通用小工具：分页规范化、租户/用户 ID 兜底。
//
// 这些函数原先杂居在 config 包，但它们与「配置加载」无关，属于 handler 层反复用到的
// 请求参数归一逻辑，故独立成包，避免 config 包承担非配置职责。
package httputil

// 默认值常量（开发环境兜底 + 分页边界）。
const (
	// DefaultTenantID 默认租户ID（用于开发环境）。
	DefaultTenantID = int64(1)
	// DefaultUserID 默认用户ID（用于开发环境）。
	DefaultUserID = int64(1)
	// DefaultPageSize 默认分页大小。
	DefaultPageSize = 20
	// MaxPageSize 最大分页大小。
	MaxPageSize = 100
)

// GetTenantIDWithDefault 获取租户ID，如果为0则返回默认值。
func GetTenantIDWithDefault(tenantID int64) int64 {
	if tenantID == 0 {
		return DefaultTenantID
	}
	return tenantID
}

// GetUserIDWithDefault 获取用户ID，如果为0则返回默认值。
func GetUserIDWithDefault(userID int64) int64 {
	if userID == 0 {
		return DefaultUserID
	}
	return userID
}

// NormalizePageSize 规范化分页大小，钳制到 [DefaultPageSize, MaxPageSize]。
func NormalizePageSize(pageSize int) int {
	if pageSize <= 0 {
		return DefaultPageSize
	}
	if pageSize > MaxPageSize {
		return MaxPageSize
	}
	return pageSize
}

// NormalizePage 规范化页码，非正数归一为 1。
func NormalizePage(page int) int {
	if page <= 0 {
		return 1
	}
	return page
}
