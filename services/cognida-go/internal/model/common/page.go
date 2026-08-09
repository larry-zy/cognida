package common

import (
	"github.com/gin-gonic/gin"
)

// ========================================
// 分页请求
// ========================================

// Req 分页请求
type Req struct {
	Page     int `json:"page" form:"page" binding:"omitempty,min=1"`           // 页码
	PageSize int `json:"page_size" form:"page_size" binding:"omitempty,min=1"` // 每页数量
}

const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// NewReq 创建分页请求
func NewReq(page, pageSize int) *Req {
	if page <= 0 {
		page = DefaultPage
	}
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	return &Req{
		Page:     page,
		PageSize: pageSize,
	}
}

// GetFromGin 从 Gin 获取分页参数
func GetFromGin(c *gin.Context) *Req {
	var req Req
	_ = c.ShouldBindQuery(&req) // 绑定失败保持零值，由 Normalize 兜底默认值
	return req.Normalize()
}

// Normalize 标准化分页参数
func (r *Req) Normalize() *Req {
	if r.Page <= 0 {
		r.Page = DefaultPage
	}
	if r.PageSize <= 0 {
		r.PageSize = DefaultPageSize
	}
	if r.PageSize > MaxPageSize {
		r.PageSize = MaxPageSize
	}
	return r
}

// Offset 计算偏移量
func (r *Req) Offset() int {
	return (r.Page - 1) * r.PageSize
}

// Limit 获取限制数量
func (r *Req) Limit() int {
	return r.PageSize
}

// ========================================
// 分页响应
// ========================================

// Resp 分页响应
type Resp struct {
	Total int64       `json:"total"` // 总数
	List  interface{} `json:"list"`  // 列表数据
	Page  int         `json:"page"`  // 当前页
	Size  int         `json:"size"`  // 每页数量
	Pages int         `json:"pages"` // 总页数
}

// NewResp 创建分页响应
func NewResp(total int64, list interface{}, page, size int) *Resp {
	pages := int(total) / size
	if int(total)%size > 0 {
		pages++
	}
	return &Resp{
		Total: total,
		List:  list,
		Page:  page,
		Size:  size,
		Pages: pages,
	}
}

// NewRespFromReq 根据请求创建分页响应
func NewRespFromReq(req *Req, total int64, list interface{}) *Resp {
	return NewResp(total, list, req.Page, req.PageSize)
}

// ========================================
// 空列表响应
// ========================================

// EmptyResp 空分页响应
func EmptyResp(page, size int) *Resp {
	return &Resp{
		Total: 0,
		List:  []interface{}{},
		Page:  page,
		Size:  size,
		Pages: 0,
	}
}

// ========================================
// 分页信息
// ========================================

// Info 分页信息（不含列表数据）
type Info struct {
	Total int64 `json:"total"` // 总数
	Page  int   `json:"page"`  // 当前页
	Size  int   `json:"size"`  // 每页数量
	Pages int   `json:"pages"` // 总页数
}

// NewInfo 创建分页信息
func NewInfo(total int64, page, size int) *Info {
	pages := int(total) / size
	if int(total)%size > 0 {
		pages++
	}
	return &Info{
		Total: total,
		Page:  page,
		Size:  size,
		Pages: pages,
	}
}

// ========================================
// 辅助函数
// ========================================

// TotalPages 计算总页数
func TotalPages(total int64, pageSize int) int {
	if pageSize <= 0 {
		return 0
	}
	pages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		pages++
	}
	return pages
}

// HasNext 是否有下一页
func (r *Resp) HasNext() bool {
	return r.Page < r.Pages
}

// HasPrev 是否有上一页
func (r *Resp) HasPrev() bool {
	return r.Page > 1
}

// IsFirst 是否是第一页
func (r *Resp) IsFirst() bool {
	return r.Page == 1
}

// IsLast 是否是最后一页
func (r *Resp) IsLast() bool {
	return r.Page >= r.Pages
}

// ========================================
// 数据库查询辅助
// ========================================

// BuildPage 构建分页响应（适用于 GORM）
func BuildPage(list interface{}, total int64, req *Req) *Resp {
	return NewRespFromReq(req, total, list)
}

// BuildPageWithList 构建分页响应（指定列表和总数）
func BuildPageWithList(list interface{}, total int64, page, size int) *Resp {
	return NewResp(total, list, page, size)
}
