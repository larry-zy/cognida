package datasource

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "github.com/lib/pq" // 注册 database/sql 驱动 "postgres"

	model "link/internal/model/datasource"
)

// postgresDriver PostgreSQL 数据源驱动。
// 走 database/sql + lib/pq，只做 information_schema/pg_catalog 元数据查询，
// 不对外部库做任何写操作或 schema 迁移。
type postgresDriver struct{}

func (postgresDriver) Type() model.Type   { return model.TypePostgres }
func (postgresDriver) DriverName() string { return "postgres" }

// pgExtra 类型差异化参数（存 DataSource.Extra JSON），均可空。
type pgExtra struct {
	SSLMode string `json:"sslmode"` // 缺省 disable
	Schema  string `json:"schema"`  // 缺省 public
}

func parsePGExtra(raw []byte) pgExtra {
	e := pgExtra{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &e)
	}
	if strings.TrimSpace(e.SSLMode) == "" {
		e.SSLMode = "disable"
	}
	if strings.TrimSpace(e.Schema) == "" {
		e.Schema = "public"
	}
	return e
}

// BuildDSN 构建 lib/pq keyword/value DSN。查询用 schema 经 search_path 固化，
// 元数据查询（ListTables/DescribeTable）再按连接的 current_schema() 过滤，
// 从而绕开 Driver 接口不透传 Extra 的限制。
func (postgresDriver) BuildDSN(ds *model.DataSource, password string) (string, error) {
	e := parsePGExtra(ds.Extra)
	parts := []string{
		"host=" + pqQuote(ds.Host),
		fmt.Sprintf("port=%d", ds.Port),
		"user=" + pqQuote(ds.Username),
		"password=" + pqQuote(password),
		"dbname=" + pqQuote(ds.DatabaseName),
		"sslmode=" + pqQuote(e.SSLMode),
		"connect_timeout=5",
		// 固化默认 schema，未加 schema 前缀的查询命中配置 schema
		"search_path=" + pqQuote(e.Schema),
	}
	return strings.Join(parts, " "), nil
}

// pqQuote 按 lib/pq keyword/value DSN 规则转义取值：含空格/特殊字符时
// 用单引号包裹并转义反斜杠与单引号，空值也需显式空单引号。
func pqQuote(v string) string {
	if v == "" {
		return "''"
	}
	if !strings.ContainsAny(v, " '\\\t\n") {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	return "'" + r.Replace(v) + "'"
}

func (postgresDriver) Ping(ctx context.Context, db *sql.DB) error {
	return db.PingContext(ctx)
}

// QuoteIdent 用双引号包裹并转义内部双引号。
func (postgresDriver) QuoteIdent(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

// pgCurrentSchema 返回连接 search_path 中生效的首个 schema（缺省 public 兜底）。
// 元数据查询据此过滤，等价于数据源 Extra 在 DSN 里固化的 schema。
func pgCurrentSchema(ctx context.Context, db *sql.DB) string {
	var schema string
	// current_schema() 返回 search_path 中第一个存在的 schema
	if err := db.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&schema); err != nil || schema == "" {
		return "public"
	}
	return schema
}

func (postgresDriver) ListTables(ctx context.Context, db *sql.DB, dbName string) ([]TableInfo, error) {
	schema := pgCurrentSchema(ctx, db)
	rows, err := db.QueryContext(ctx, `
		SELECT c.relname AS name,
		       COALESCE(obj_description(c.oid), '') AS comment,
		       COALESCE(c.reltuples, 0)::bigint AS rows
		FROM pg_catalog.pg_class c
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind IN ('r', 'p') AND n.nspname = $1
		ORDER BY c.relname`, schema)
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
		if t.Rows < 0 { // reltuples 未 ANALYZE 时可能为 -1
			t.Rows = 0
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

func (postgresDriver) DescribeTable(ctx context.Context, db *sql.DB, dbName, table string) (*TableSchema, error) {
	schema := pgCurrentSchema(ctx, db)
	result := &TableSchema{Name: table}

	err := db.QueryRowContext(ctx, `
		SELECT COALESCE(obj_description(c.oid), '')
		FROM pg_catalog.pg_class c
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relname = $2 AND c.relkind IN ('r', 'p')`,
		schema, table).Scan(&result.Comment)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("datasource: 表 %q 不存在", table)
		}
		return nil, err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT a.attname AS name,
		       pg_catalog.format_type(a.atttypid, a.atttypmod) AS type,
		       CASE WHEN a.attnotnull THEN 'NO' ELSE 'YES' END AS nullable,
		       CASE WHEN pk.attnum IS NOT NULL THEN 'PRI' ELSE '' END AS key,
		       COALESCE(pg_catalog.col_description(a.attrelid, a.attnum), '') AS comment
		FROM pg_catalog.pg_attribute a
		JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN (
			SELECT conrelid, unnest(conkey) AS attnum
			FROM pg_catalog.pg_constraint WHERE contype = 'p'
		) pk ON pk.conrelid = a.attrelid AND pk.attnum = a.attnum
		WHERE n.nspname = $1 AND c.relname = $2 AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var col ColumnInfo
		var nullable string
		if err := rows.Scan(&col.Name, &col.Type, &nullable, &col.Key, &col.Comment); err != nil {
			return nil, err
		}
		col.Nullable = nullable == "YES"
		result.Columns = append(result.Columns, col)
	}
	return result, rows.Err()
}
