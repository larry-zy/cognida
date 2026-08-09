package datasource

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	model "cognida/internal/model/datasource"
)

// ========================================
// 可查询的假 driver：按 SQL 文本返回预置行，覆盖 ListTables 与抽样 SELECT
// ========================================

var registerSampleOnce sync.Once

func registerSampleDriver() {
	registerSampleOnce.Do(func() { sql.Register("sample_fake", sampleDrv{}) })
}

type sampleDrv struct{}

func (sampleDrv) Open(string) (driver.Conn, error) { return sampleConn{}, nil }

type sampleConn struct{}

func (sampleConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not implemented") }
func (sampleConn) Close() error                        { return nil }
func (sampleConn) Begin() (driver.Tx, error)           { return nil, errors.New("not implemented") }

func (sampleConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "information_schema.TABLES"):
		// mysqlDriver.ListTables
		return &sampleRows{
			cols: []string{"TABLE_NAME", "TABLE_COMMENT", "TABLE_ROWS"},
			data: [][]driver.Value{{[]byte("orders"), []byte("订单表"), int64(100)}},
		}, nil
	case strings.Contains(query, "SELECT * FROM"):
		// SampleCSV 抽样查询：含 NULL、中文，验证 RawBytes → CSV
		return &sampleRows{
			cols: []string{"id", "name", "note"},
			data: [][]driver.Value{
				{[]byte("1"), []byte("alice"), nil},
				{[]byte("2"), []byte("bob"), []byte("你好")},
			},
		}, nil
	}
	return nil, errors.New("unexpected query: " + query)
}

type sampleRows struct {
	cols []string
	data [][]driver.Value
	pos  int
}

func (r *sampleRows) Columns() []string { return r.cols }
func (r *sampleRows) Close() error      { return nil }
func (r *sampleRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.pos])
	r.pos++
	return nil
}

func newSampleService(t *testing.T) (*Service, *memRepo) {
	t.Helper()
	registerSampleDriver()
	repo := newMemRepo()
	cipher, err := NewCipher("cm-test-secret")
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	m := NewConnectionManager(repo, cipher, DefaultPoolOptions())
	m.openFunc = func(_, dsn string) (*sql.DB, error) { return sql.Open("sample_fake", dsn) }
	m.skipPing = true
	return NewService(repo, cipher, m, &seqIDGen{}), repo
}

func TestSampleCSV(t *testing.T) {
	s, repo := newSampleService(t)
	seedHealthDS(t, s, repo, "ds1", model.TypeMySQL, model.StatusActive)

	data, n, err := s.SampleCSV(context.Background(), 1, "ds1", "orders", 10)
	if err != nil {
		t.Fatalf("SampleCSV: %v", err)
	}
	if n != 2 {
		t.Fatalf("抽样行数 = %d, want 2", n)
	}

	records, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil {
		t.Fatalf("解析 CSV: %v", err)
	}
	want := [][]string{
		{"id", "name", "note"},
		{"1", "alice", ""}, // NULL → 空串
		{"2", "bob", "你好"},
	}
	if len(records) != len(want) {
		t.Fatalf("CSV 行数 = %d, want %d: %v", len(records), len(want), records)
	}
	for i := range want {
		if strings.Join(records[i], ",") != strings.Join(want[i], ",") {
			t.Errorf("第 %d 行 = %v, want %v", i, records[i], want[i])
		}
	}
}

func TestSampleCSVRejectsUnknownTable(t *testing.T) {
	s, repo := newSampleService(t)
	seedHealthDS(t, s, repo, "ds1", model.TypeMySQL, model.StatusActive)

	// 未在真实表清单中的表名必须被白名单拦截，不落到 SELECT
	_, n, err := s.SampleCSV(context.Background(), 1, "ds1", "secret", 10)
	if err == nil {
		t.Fatalf("未知表应报错")
	}
	if n != 0 {
		t.Fatalf("拒绝时行数应为 0, got %d", n)
	}
	if !strings.Contains(err.Error(), "不存在或不可访问") {
		t.Fatalf("错误信息应指明表不可访问: %v", err)
	}
}

func TestSampleCSVLimitClamped(t *testing.T) {
	s, repo := newSampleService(t)
	seedHealthDS(t, s, repo, "ds1", model.TypeMySQL, model.StatusActive)

	// limit<=0 走默认；超上限被收敛——此处仅验证不报错且返回样本
	if _, _, err := s.SampleCSV(context.Background(), 1, "ds1", "orders", 0); err != nil {
		t.Fatalf("默认 limit 应成功: %v", err)
	}
	if _, _, err := s.SampleCSV(context.Background(), 1, "ds1", "orders", maxSampleSize+1); err != nil {
		t.Fatalf("超上限 limit 应被收敛并成功: %v", err)
	}
}
