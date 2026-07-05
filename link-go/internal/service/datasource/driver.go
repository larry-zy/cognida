package datasource

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	gosqlmysql "github.com/go-sql-driver/mysql"

	model "link/internal/model/datasource"
)

// TableInfo 外部数据源表清单条目（轻量：不含列）
type TableInfo struct {
	Name    string `json:"name"`
	Comment string `json:"comment"`
	Rows    int64  `json:"rows"` // information_schema 估算行数
}

// ColumnInfo 列结构
type ColumnInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Key      string `json:"key"` // PRI/UNI/MUL/空
	Comment  string `json:"comment"`
}

// TableSchema 表结构
type TableSchema struct {
	Name    string       `json:"name"`
	Comment string       `json:"comment"`
	Columns []ColumnInfo `json:"columns"`
}

// Driver 数据源驱动策略接口：每种数据库类型一个实现。
// 所有查询走 database/sql 原生连接，不引入 GORM。
type Driver interface {
	// Type 对应的数据源类型
	Type() model.Type
	// DriverName database/sql 注册的驱动名
	DriverName() string
	// BuildDSN 由数据源配置与明文密码构建 DSN
	BuildDSN(ds *model.DataSource, password string) (string, error)
	// Ping 连通性检查
	Ping(ctx context.Context, db *sql.DB) error
	// ListTables 表清单（经 information_schema，不 SELECT 业务数据）
	ListTables(ctx context.Context, db *sql.DB, dbName string) ([]TableInfo, error)
	// DescribeTable 单表结构
	DescribeTable(ctx context.Context, db *sql.DB, dbName, table string) (*TableSchema, error)
}

// drivers 已注册驱动（Factory）。Phase 3 在此追加 PostgreSQL。
var drivers = map[model.Type]Driver{
	model.TypeMySQL: mysqlDriver{},
}

// DriverFor 按类型取驱动；不支持的类型返回错误
func DriverFor(t model.Type) (Driver, error) {
	d, ok := drivers[t]
	if !ok {
		return nil, fmt.Errorf("datasource: 不支持的数据源类型 %q", t)
	}
	return d, nil
}

// SupportedTypes 当前支持的数据源类型（供 API 校验与前端下拉）
func SupportedTypes() []model.Type {
	types := make([]model.Type, 0, len(drivers))
	for t := range drivers {
		types = append(types, t)
	}
	return types
}

// ========================================
// MySQL driver
// ========================================

type mysqlDriver struct{}

func (mysqlDriver) Type() model.Type   { return model.TypeMySQL }
func (mysqlDriver) DriverName() string { return "mysql" }

func (mysqlDriver) BuildDSN(ds *model.DataSource, password string) (string, error) {
	cfg := gosqlmysql.NewConfig()
	cfg.User = ds.Username
	cfg.Passwd = password
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", ds.Host, ds.Port)
	cfg.DBName = ds.DatabaseName
	cfg.ParseTime = true
	cfg.Timeout = 5 * time.Second // 连接建立超时；查询超时由调用方 context 控制
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	return cfg.FormatDSN(), nil
}

func (mysqlDriver) Ping(ctx context.Context, db *sql.DB) error {
	return db.PingContext(ctx)
}

func (mysqlDriver) ListTables(ctx context.Context, db *sql.DB, dbName string) ([]TableInfo, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT TABLE_NAME, IFNULL(TABLE_COMMENT, ''), IFNULL(TABLE_ROWS, 0)
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'
		ORDER BY TABLE_NAME`, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Name, &t.Comment, &t.Rows); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

func (mysqlDriver) DescribeTable(ctx context.Context, db *sql.DB, dbName, table string) (*TableSchema, error) {
	schema := &TableSchema{Name: table}

	if err := db.QueryRowContext(ctx, `
		SELECT IFNULL(TABLE_COMMENT, '')
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`, dbName, table).
		Scan(&schema.Comment); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("datasource: 表 %q 不存在", table)
		}
		return nil, err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, IFNULL(COLUMN_KEY, ''), IFNULL(COLUMN_COMMENT, '')
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION`, dbName, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var c ColumnInfo
		var nullable string
		if err := rows.Scan(&c.Name, &c.Type, &nullable, &c.Key, &c.Comment); err != nil {
			return nil, err
		}
		c.Nullable = nullable == "YES"
		schema.Columns = append(schema.Columns, c)
	}
	return schema, rows.Err()
}
