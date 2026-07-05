// Package datasource 数据源领域模型：用户注册的外部数据库连接的实体与接口定义。
//
// 外部数据源是"只读外部资源"而非第二个业务库：连接一律走 database/sql 原生查询，
// 不做 GORM 映射与 schema 迁移；密码只以密文形态出现在领域内。
package datasource

import (
	"context"
	"database/sql"
	"time"
)

// Type 数据源类型
type Type string

const (
	// TypeMySQL MySQL 数据源（Phase 1 唯一支持类型）
	TypeMySQL Type = "mysql"
)

// 数据源状态
const (
	// StatusActive 正常可用
	StatusActive = "active"
	// StatusError 最近一次连接/健康检查失败
	StatusError = "error"
	// StatusNeedCredentials 凭证不可解（如加密密钥变更），需重新录入密码
	StatusNeedCredentials = "need_credentials"
)

// DataSource 外部数据源实体。
// PasswordEncrypted 为 AES-GCM 密文（base64），领域内不出现明文密码；
// API 层不得回显本字段（含密文）。
type DataSource struct {
	ID                string
	TenantID          int64
	Name              string // 租户内唯一
	Type              Type
	Host              string
	Port              int
	DatabaseName      string
	Username          string
	PasswordEncrypted string
	Extra             []byte // 类型差异化参数（charset/SSL 等）JSON，可空
	Status            string
	LastHealthCheckAt *time.Time
	CreatedBy         int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ListFilter 数据源列表查询过滤
type ListFilter struct {
	TenantID int64
	Limit    int
	Offset   int
}

// Repository 数据源仓储接口
type Repository interface {
	Create(ctx context.Context, ds *DataSource) error
	Get(ctx context.Context, id string, tenantID int64) (*DataSource, error)
	GetByName(ctx context.Context, name string, tenantID int64) (*DataSource, error)
	List(ctx context.Context, filter ListFilter) ([]*DataSource, int64, error)
	Update(ctx context.Context, ds *DataSource) error
	// UpdateStatus 仅更新状态与健康检查时间（不触碰 updated_at 之外的配置字段）。
	UpdateStatus(ctx context.Context, id string, tenantID int64, status string, checkedAt time.Time) error
	Delete(ctx context.Context, id string, tenantID int64) error
}

// ConnectionProvider 按数据源 ID 提供只读连接（实现在 service 层，供 agent 工具层消费）。
// 返回的 *sql.DB 是受管连接池：调用方只使用、不 Close。
type ConnectionProvider interface {
	// Acquire 返回数据源的连接池与元数据。
	// 数据源不存在、被禁用或不可连接时返回错误——调用方不得静默回落到业务库。
	Acquire(ctx context.Context, tenantID int64, datasourceID string) (*sql.DB, *DataSource, error)
}
