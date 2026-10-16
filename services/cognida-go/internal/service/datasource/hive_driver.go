package datasource

import (
	m "cognida/internal/model/datasource"
	"cognida/internal/service/datasource/hivewire"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type hiveDriver struct{}

func (hiveDriver) Type() m.Type               { return m.TypeHive }
func (hiveDriver) DriverName() string         { return "cognida_hive" }
func (hiveDriver) QuoteIdent(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }
func (hiveDriver) BuildDSN(ds *m.DataSource, password string) (string, error) {
	var extra struct {
		Auth string `json:"auth"`
		TLS  bool   `json:"tls"`
	}
	if json.Unmarshal(ds.Extra, &extra) != nil {
		return "", fmt.Errorf("Hive auth/TLS configuration required")
	}
	if extra.Auth != "NONE" && extra.Auth != "NOSASL" && extra.Auth != "LDAP" && extra.Auth != "CUSTOM" {
		return "", fmt.Errorf("Hive auth must be NONE, NOSASL, LDAP or CUSTOM; Kerberos is not configured")
	}
	if (extra.Auth == "LDAP" || extra.Auth == "CUSTOM") && !extra.TLS {
		return "", fmt.Errorf("Hive password authentication requires verified TLS")
	}
	if ds.Username == "" || ds.DatabaseName == "" {
		return "", fmt.Errorf("Hive requires an explicit service identity and database")
	}
	b, e := json.Marshal(hivewire.Config{Host: ds.Host, Port: ds.Port, Username: ds.Username, Password: password, Database: ds.DatabaseName, Auth: extra.Auth, TLS: extra.TLS})
	return string(b), e
}
func (hiveDriver) Ping(ctx context.Context, db *sql.DB) error { return db.PingContext(ctx) }
func (d hiveDriver) ListTables(ctx context.Context, db *sql.DB, name string) ([]TableInfo, error) {
	rows, e := db.QueryContext(ctx, "SHOW TABLES IN "+d.QuoteIdent(name))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []TableInfo{}
	for rows.Next() {
		var item TableInfo
		if e = rows.Scan(&item.Name); e != nil {
			return nil, e
		}
		item.Type = "table"
		out = append(out, item)
		if len(out) > 10000 {
			return nil, fmt.Errorf("Hive table metadata limit exceeded")
		}
	}
	return out, rows.Err()
}
func (d hiveDriver) DescribeTable(ctx context.Context, db *sql.DB, name, table string) (*TableSchema, error) {
	rows, e := db.QueryContext(ctx, "DESCRIBE "+d.QuoteIdent(name)+"."+d.QuoteIdent(table))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := &TableSchema{Name: table}
	seen := map[string]bool{}
	for rows.Next() {
		var col ColumnInfo
		var comment sql.NullString
		if e = rows.Scan(&col.Name, &col.Type, &comment); e != nil {
			return nil, e
		}
		if col.Name == "" || strings.HasPrefix(col.Name, "#") || seen[col.Name] {
			continue
		}
		seen[col.Name] = true
		col.Comment = comment.String
		col.Nullable = true
		out.Columns = append(out.Columns, col)
		if len(out.Columns) > 512 {
			return nil, fmt.Errorf("Hive column metadata limit exceeded")
		}
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out.Columns) == 0 {
		return nil, fmt.Errorf("Hive table not found or empty schema")
	}
	return out, nil
}
