package datasource

import (
	"cognida/internal/service/datasource/hivewire"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	model "cognida/internal/model/datasource"
)

type sqlQueryAdapter struct {
	db     *sql.DB
	source *model.DataSource
}

func (s *Service) AcquireQuery(ctx context.Context, tenant int64, id string) (model.QueryAdapter, *model.DataSource, error) {
	remote, _, err := s.bigQuery(ctx, tenant, id)
	if err != nil {
		return nil, nil, err
	}
	if remote != nil {
		return remote, remote.source, nil
	}
	db, source, err := s.Acquire(ctx, tenant, id)
	if err != nil {
		return nil, nil, err
	}
	adapter := &sqlQueryAdapter{db: db, source: source}
	if source.Type == model.TypeHive {
		password, e := s.cipher.Decrypt(source.PasswordEncrypted)
		if e != nil {
			return nil, nil, e
		}
		dsn, e := (hiveDriver{}).BuildDSN(source, password)
		if e != nil {
			return nil, nil, e
		}
		var config hivewire.Config
		if json.Unmarshal([]byte(dsn), &config) != nil {
			return nil, nil, fmt.Errorf("Hive configuration invalid")
		}
		return &hiveAsyncAdapter{sqlQueryAdapter: adapter, config: config, cipher: s.cipher}, source, nil
	}
	return adapter, source, nil
}
func (a *sqlQueryAdapter) Capabilities() model.QueryCapabilities {
	client := "go-sql-driver/mysql@1.9.3"
	if a.source.Type == model.TypePostgres {
		client = "lib/pq@1.10.9"
	}
	if a.source.Type == model.TypeClickHouse {
		client = "clickhouse-go/v2@2.40.3"
	}
	if a.source.Type == model.TypeHive {
		return model.QueryCapabilities{Engine: model.TypeHive, Client: "gohive/v2@2.1.0 protocol + cognida context adapter", Dialect: "hive", Parameters: false, Cancellation: "hiveserver2_cancel_operation", Pagination: "bounded_result_snapshot"}
	}
	return model.QueryCapabilities{Engine: a.source.Type, Client: client, Dialect: string(a.source.Type), Parameters: true, Cancellation: "context_cancel", Pagination: "bounded_result_snapshot"}
}

var forbiddenQueryToken = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|MERGE|DROP|ALTER|CREATE|TRUNCATE|GRANT|REVOKE|CALL|EXECUTE|COPY|INTO|OUTFILE|DUMPFILE|LOAD_FILE|SETTINGS|SYSTEM|ATTACH|DETACH|KILL|PG_SLEEP|PG_READ_FILE|PG_WRITE_FILE|DBLINK|SLEEP|BENCHMARK|GET_LOCK|RELEASE_LOCK|URL|S3|REMOTE|REMOTESecure|FILE|MYSQL|POSTGRESQL|INPUT)\b`)

// Conservative statement boundary. Server-side read-only users remain required:
// a SELECT can invoke installed functions, so parsing is never a privilege grant.
func ValidateReadQuery(query string) error {
	s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(query), ";"))
	upper := strings.ToUpper(s)
	if len(s) == 0 || len(s) > 64<<10 || (!strings.HasPrefix(upper, "SELECT ") && !strings.HasPrefix(upper, "SELECT\n") && !strings.HasPrefix(upper, "WITH ") && !strings.HasPrefix(upper, "WITH\n")) || strings.ContainsAny(s, ";\x00") || strings.Contains(s, "--") || strings.Contains(s, "/*") || strings.Contains(s, "#") || forbiddenQueryToken.MatchString(s) {
		return fmt.Errorf("datasource: only bounded read-only SELECT queries are allowed")
	}
	return nil
}
func (a *sqlQueryAdapter) Query(ctx context.Context, in model.QueryRequest) (*model.QueryResult, error) {
	if err := ValidateReadQuery(in.SQL); err != nil {
		return nil, err
	}
	if a.source.Type == model.TypeHive {
		if len(in.Parameters) > 0 {
			return nil, fmt.Errorf("Hive query parameters unsupported")
		}
		if regexp.MustCompile(`(?i)\b(TRANSFORM|SCRIPT|REFLECT|JAVA_METHOD|ADD|JAR|DFS)\b`).MatchString(in.SQL) {
			return nil, fmt.Errorf("Hive executable functions are forbidden")
		}
	}
	if in.MaxRows < 1 || in.MaxRows > 10000 || in.MaxBytes < 1 || in.MaxBytes > 8<<20 || in.Timeout <= 0 || in.Timeout > 30*time.Second || in.MaxScanBytes != 0 {
		return nil, fmt.Errorf("datasource: unsupported query budget")
	}
	ctx, cancel := context.WithTimeout(ctx, in.Timeout)
	defer cancel()
	// The wrapper bounds the whole result, including UNION/CTEs and inner LIMITs.
	query := fmt.Sprintf("SELECT * FROM (%s) AS cognida_bounded LIMIT %d", strings.TrimSuffix(strings.TrimSpace(in.SQL), ";"), in.MaxRows+1)
	var rows *sql.Rows
	var err error
	if a.source.Type == model.TypeMySQL || a.source.Type == model.TypePostgres {
		tx, e := a.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if e != nil {
			return nil, e
		}
		defer tx.Rollback()
		if a.source.Type == model.TypePostgres {
			if _, e = tx.ExecContext(ctx, "SET LOCAL statement_timeout = '30000ms'"); e != nil {
				return nil, e
			}
		}
		rows, err = tx.QueryContext(ctx, query, in.Parameters...)
	} else {
		rows, err = a.db.QueryContext(ctx, query, in.Parameters...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	if len(columns) > 512 {
		return nil, fmt.Errorf("datasource: column limit exceeded")
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	out := &model.QueryResult{Columns: columns, ColumnTypes: map[string]string{}, Rows: []map[string]any{}, ExecutedSQL: query, SourceVersion: a.source.UpdatedAt.UTC().Format(time.RFC3339Nano)}
	for i, col := range columns {
		if _, ok := out.ColumnTypes[col]; ok {
			return nil, fmt.Errorf("datasource: duplicate column name; use aliases")
		}
		out.ColumnTypes[col] = types[i].DatabaseTypeName()
	}
	size := 0
	for rows.Next() {
		if len(out.Rows) == in.MaxRows {
			out.Truncated = true
			break
		}
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for i := range dest {
			dest[i] = &values[i]
		}
		if err = rows.Scan(dest...); err != nil {
			return nil, err
		}
		row := map[string]any{}
		for i, v := range values {
			switch n := v.(type) {
			case []byte:
				row[columns[i]] = string(n)
			case int64:
				row[columns[i]] = fmt.Sprint(n)
			case uint64:
				row[columns[i]] = fmt.Sprint(n)
			case fmt.Stringer:
				row[columns[i]] = n.String()
			default:
				row[columns[i]] = v
			}
		}
		encoded, e := json.Marshal(row)
		if e != nil {
			return nil, e
		}
		size += len(encoded)
		if size > in.MaxBytes {
			return nil, fmt.Errorf("datasource: result exceeds byte budget")
		}
		out.Rows = append(out.Rows, row)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func trimQuery(query string) string { return strings.TrimSuffix(strings.TrimSpace(query), ";") }
func validateHiveRead(in model.QueryRequest) error {
	if e := ValidateReadQuery(in.SQL); e != nil {
		return e
	}
	if len(in.Parameters) > 0 || regexp.MustCompile(`(?i)\b(TRANSFORM|SCRIPT|REFLECT|JAVA_METHOD|ADD|JAR|DFS)\b`).MatchString(in.SQL) {
		return fmt.Errorf("Hive executable functions or parameters unsupported")
	}
	if in.MaxRows < 1 || in.MaxRows > 10000 || in.MaxBytes < 1 || in.MaxBytes > 8<<20 || in.Timeout <= 0 || in.Timeout > 30*time.Second || in.MaxScanBytes != 0 {
		return fmt.Errorf("Hive query budget invalid")
	}
	return nil
}
