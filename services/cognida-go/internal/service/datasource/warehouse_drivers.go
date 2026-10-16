package datasource

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	model "cognida/internal/model/datasource"
	_ "github.com/ClickHouse/clickhouse-go/v2"
)

type clickHouseDriver struct{}

func (clickHouseDriver) Type() model.Type   { return model.TypeClickHouse }
func (clickHouseDriver) DriverName() string { return "clickhouse" }
func (clickHouseDriver) BuildDSN(ds *model.DataSource, password string) (string, error) {
	var extra struct {
		TLS bool `json:"tls"`
	}
	if len(ds.Extra) > 0 {
		if err := json.Unmarshal(ds.Extra, &extra); err != nil {
			return "", fmt.Errorf("invalid ClickHouse configuration")
		}
	}
	u := url.URL{Scheme: "clickhouse", Host: net.JoinHostPort(ds.Host, strconv.Itoa(ds.Port)), Path: "/" + ds.DatabaseName, User: url.UserPassword(ds.Username, password)}
	q := url.Values{"dial_timeout": {"5s"}, "read_timeout": {"30s"}, "readonly": {"1"}, "max_execution_time": {"30"}, "max_memory_usage": {"268435456"}, "max_result_bytes": {"8388608"}, "result_overflow_mode": {"throw"}}
	if extra.TLS {
		q.Set("secure", "true")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func (clickHouseDriver) Ping(ctx context.Context, db *sql.DB) error { return db.PingContext(ctx) }
func (clickHouseDriver) QuoteIdent(v string) string {
	return "`" + strings.ReplaceAll(v, "`", "``") + "`"
}
func (clickHouseDriver) ListTables(ctx context.Context, db *sql.DB, name string) ([]TableInfo, error) {
	rows, err := db.QueryContext(ctx, "SELECT name, comment, ifNull(total_rows, 0), if(engine IN ('View','MaterializedView'), 'view', 'table') FROM system.tables WHERE database = ? ORDER BY name", name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TableInfo{}
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Name, &t.Comment, &t.Rows, &t.Type); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (clickHouseDriver) DescribeTable(ctx context.Context, db *sql.DB, name, table string) (*TableSchema, error) {
	out := &TableSchema{Name: table}
	if err := db.QueryRowContext(ctx, "SELECT comment FROM system.tables WHERE database = ? AND name = ?", name, table).Scan(&out.Comment); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, "SELECT name, type, comment, is_in_primary_key FROM system.columns WHERE database = ? AND table = ? ORDER BY position", name, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c ColumnInfo
		var primary uint8
		if err := rows.Scan(&c.Name, &c.Type, &c.Comment, &primary); err != nil {
			return nil, err
		}
		c.Nullable = strings.HasPrefix(c.Type, "Nullable(")
		if primary != 0 {
			c.Key = "PRI"
		}
		out.Columns = append(out.Columns, c)
	}
	return out, rows.Err()
}

// These engines share the MySQL wire protocol, but have their own metadata
// contract. In particular, the MySQL foreign-key metadata query is not used.
type warehouseMySQLDriver struct{ engine model.Type }

func (d warehouseMySQLDriver) Type() model.Type { return d.engine }
func (warehouseMySQLDriver) DriverName() string { return "mysql" }
func (warehouseMySQLDriver) BuildDSN(ds *model.DataSource, p string) (string, error) {
	return (mysqlDriver{}).BuildDSN(ds, p)
}
func (warehouseMySQLDriver) Ping(ctx context.Context, db *sql.DB) error { return db.PingContext(ctx) }
func (warehouseMySQLDriver) QuoteIdent(s string) string                 { return (mysqlDriver{}).QuoteIdent(s) }
func (warehouseMySQLDriver) ListTables(ctx context.Context, db *sql.DB, name string) ([]TableInfo, error) {
	return (mysqlDriver{}).ListTables(ctx, db, name)
}
func (warehouseMySQLDriver) DescribeTable(ctx context.Context, db *sql.DB, name, table string) (*TableSchema, error) {
	out := &TableSchema{Name: table}
	if err := db.QueryRowContext(ctx, "SELECT IFNULL(TABLE_COMMENT, '') FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?", name, table).Scan(&out.Comment); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, "SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, IFNULL(COLUMN_KEY, ''), IFNULL(COLUMN_COMMENT, '') FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION", name, table)
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
		out.Columns = append(out.Columns, c)
	}
	return out, rows.Err()
}
