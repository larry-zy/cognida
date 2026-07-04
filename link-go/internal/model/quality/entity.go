// Package quality 数据质量领域模型：质检记录实体与仓储接口。
package quality

import (
	"context"
	"time"
)

// CheckType 质检类型
type CheckType string

const (
	// CheckTypeStructured 结构化数据评估
	CheckTypeStructured CheckType = "structured"
	// CheckTypeUnstructured 非结构化文本质量
	CheckTypeUnstructured CheckType = "unstructured"
	// CheckTypeCleaning 数据清洗
	CheckTypeCleaning CheckType = "cleaning"
)

// CheckRecord 质检记录：一次质量评估/清洗的持久化结果，用于历史回看。
type CheckRecord struct {
	ID           string
	TenantID     int64
	UserID       int64
	Type         CheckType
	SourceName   string  // 数据来源名称（文件名或"粘贴文本"）
	OverallScore float64 // 综合得分（清洗类记录为 0）
	RecordCount  int32   // 参与评估/清洗的记录数
	Summary      string  // 一句话摘要（如"6 维度 / 3 项问题"）
	// ReportJSON 完整报告快照（结构化报告 / 文本报告 / 清洗结果），前端回看用。
	ReportJSON []byte
	CreatedAt  time.Time
}

// ListFilter 历史记录查询过滤
type ListFilter struct {
	TenantID int64
	Type     CheckType // 为空表示不限类型
	Limit    int
	Offset   int
}

// CheckRecordRepository 质检记录仓储接口
type CheckRecordRepository interface {
	Create(ctx context.Context, record *CheckRecord) error
	Get(ctx context.Context, id string, tenantID int64) (*CheckRecord, error)
	List(ctx context.Context, filter ListFilter) ([]*CheckRecord, int64, error)
	Delete(ctx context.Context, id string, tenantID int64) error
}
