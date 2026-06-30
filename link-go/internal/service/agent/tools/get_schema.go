// Package tools 提供 Schema 获取工具
package tools

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// ========================================
// Get Schema Tool
// ========================================

// GetSchemaRequest Schema 获取请求
type GetSchemaRequest struct {
	// DatabaseID 数据库ID（可选，默认使用当前数据库）
	DatabaseID string `json:"database_id" jsonschema:"description=数据库ID，可选"`

	// TableName 表名（可选，不指定则返回所有表）
	TableName string `json:"table_name" jsonschema:"description=表名，可选，不指定则返回所有表"`
}

// TableSchema 表结构定义
type TableSchema struct {
	TableName  string         `json:"table_name"`
	Columns    []ColumnSchema `json:"columns"`
	PrimaryKey string         `json:"primary_key,omitempty"`
}

// ColumnSchema 列结构定义
type ColumnSchema struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Nullable    bool   `json:"nullable"`
	Description string `json:"description,omitempty"`
}

// GetSchemaResult Schema 获取结果
type GetSchemaResult struct {
	// Tables 表结构列表
	Tables []TableSchema `json:"tables"`

	// Database 数据库名
	Database string `json:"database"`
}

// 全局 DB（通过 init 设置）
var getSchemaDB *gorm.DB

// InitGetSchemaTool 初始化 Schema 获取工具
func InitGetSchemaTool(db *gorm.DB) {
	getSchemaDB = db
}

// NewGetSchemaTool 创建 Schema 获取工具
// 使用基类 TypedBaseTool 实现类型安全
func NewGetSchemaTool() *TypedBaseTool[GetSchemaRequest, GetSchemaResult] {
	return NewTypedBaseTool("get_schema",
		`获取数据库表结构信息。

返回数据库的表和列信息，用于生成 SQL 查询。

参数：
- database_id: 数据库ID（可选）
- table_name: 表名（可选，不指定则返回所有表）`,
		getSchema,
	)
}

// FetchSchema 导出的 Schema 查询入口，供 HTTP handler 等外部调用方复用。
// 复用与 get_schema 工具相同的查询逻辑，避免重复实现。
func FetchSchema(ctx context.Context, databaseID, tableName string) (*GetSchemaResult, error) {
	return getSchema(ctx, &GetSchemaRequest{
		DatabaseID: databaseID,
		TableName:  tableName,
	})
}

// getSchema 获取数据库结构
func getSchema(ctx context.Context, req *GetSchemaRequest) (*GetSchemaResult, error) {
	if getSchemaDB == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}

	// 获取数据库名
	databaseName := req.DatabaseID
	if databaseName == "" {
		databaseName = getSchemaDB.Migrator().CurrentDatabase()
	}

	// 查询表
	var tables []struct {
		TableName string `gorm:"column:table_name"`
	}

	tableQuery := getSchemaDB.WithContext(ctx).Raw(`
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = ? AND table_type = 'BASE TABLE'
	`, databaseName)

	if req.TableName != "" {
		tableQuery = getSchemaDB.WithContext(ctx).Raw(`
			SELECT table_name
			FROM information_schema.tables
			WHERE table_schema = ? AND table_type = 'BASE TABLE' AND table_name = ?
		`, databaseName, req.TableName)
	}

	if err := tableQuery.Scan(&tables).Error; err != nil {
		return nil, fmt.Errorf("查询表列表失败: %w", err)
	}

	var result []TableSchema

	for _, table := range tables {
		// 查询列信息
		var columns []struct {
			ColumnName    string `gorm:"column:column_name"`
			DataType      string `gorm:"column:data_type"`
			IsNullable     string `gorm:"column:is_nullable"`
			ColumnComment string `gorm:"column:column_comment"`
		}

		if err := getSchemaDB.WithContext(ctx).Raw(`
			SELECT column_name, data_type, is_nullable, column_comment
			FROM information_schema.columns
			WHERE table_schema = ? AND table_name = ?
			ORDER BY ordinal_position
		`, databaseName, table.TableName).Scan(&columns).Error; err != nil {
			return nil, fmt.Errorf("查询列信息失败: %w", err)
		}

		var colSchemas []ColumnSchema
		for _, col := range columns {
			colSchemas = append(colSchemas, ColumnSchema{
				Name:        col.ColumnName,
				Type:        col.DataType,
				Nullable:    col.IsNullable == "YES",
				Description: col.ColumnComment,
			})
		}

		// 查询主键
		var primaryKeys []string
		getSchemaDB.WithContext(ctx).Raw(`
			SELECT column_name
			FROM information_schema.key_column_usage
			WHERE table_schema = ? AND table_name = ? AND constraint_name = 'PRIMARY'
		`, databaseName, table.TableName).Scan(&primaryKeys)

		pk := ""
		if len(primaryKeys) > 0 {
			pk = primaryKeys[0]
		}

		result = append(result, TableSchema{
			TableName:  table.TableName,
			Columns:    colSchemas,
			PrimaryKey: pk,
		})
	}

	return &GetSchemaResult{
		Tables:   result,
		Database: databaseName,
	}, nil
}
