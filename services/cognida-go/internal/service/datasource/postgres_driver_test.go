package datasource

import (
	"strings"
	"testing"

	model "cognida/internal/model/datasource"
)

func TestPostgresDriverRegistered(t *testing.T) {
	drv, err := DriverFor(model.TypePostgres)
	if err != nil {
		t.Fatalf("postgres 驱动应已注册: %v", err)
	}
	if drv.Type() != model.TypePostgres {
		t.Errorf("Type() = %q, want %q", drv.Type(), model.TypePostgres)
	}
	if drv.DriverName() != "postgres" {
		t.Errorf("DriverName() = %q, want postgres", drv.DriverName())
	}
}

func TestPostgresBuildDSN(t *testing.T) {
	ds := &model.DataSource{
		Host:         "db.internal",
		Port:         5432,
		Username:     "reader",
		DatabaseName: "shop",
	}
	dsn, err := postgresDriver{}.BuildDSN(ds, "s3cr3t")
	if err != nil {
		t.Fatalf("BuildDSN error: %v", err)
	}
	for _, want := range []string{
		"host=db.internal", "port=5432", "user=reader",
		"password=s3cr3t", "dbname=shop",
		"sslmode=disable", "search_path=public", "connect_timeout=5",
	} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN 缺少 %q，实际: %s", want, dsn)
		}
	}
}

func TestPostgresBuildDSNExtraOverride(t *testing.T) {
	ds := &model.DataSource{
		Host:         "db",
		Port:         5432,
		Username:     "u",
		DatabaseName: "d",
		Extra:        []byte(`{"sslmode":"require","schema":"analytics"}`),
	}
	dsn, err := postgresDriver{}.BuildDSN(ds, "p")
	if err != nil {
		t.Fatalf("BuildDSN error: %v", err)
	}
	if !strings.Contains(dsn, "sslmode=require") {
		t.Errorf("应使用 Extra 的 sslmode=require，实际: %s", dsn)
	}
	if !strings.Contains(dsn, "search_path=analytics") {
		t.Errorf("应使用 Extra 的 schema=analytics，实际: %s", dsn)
	}
}

func TestPqQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "''"},
		{"simple", "simple"},
		{"has space", "'has space'"},
		{`it's`, `'it\'s'`},
		{`back\slash`, `'back\\slash'`},
	}
	for _, c := range cases {
		if got := pqQuote(c.in); got != c.want {
			t.Errorf("pqQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParsePGExtraDefaults(t *testing.T) {
	e := parsePGExtra(nil)
	if e.SSLMode != "disable" || e.Schema != "public" {
		t.Errorf("默认应为 disable/public，实际 %+v", e)
	}
	e = parsePGExtra([]byte(`{"sslmode":"verify-full"}`))
	if e.SSLMode != "verify-full" || e.Schema != "public" {
		t.Errorf("部分覆盖失败，实际 %+v", e)
	}
	// 非法 JSON 不 panic，回落默认
	e = parsePGExtra([]byte(`not-json`))
	if e.SSLMode != "disable" || e.Schema != "public" {
		t.Errorf("非法 JSON 应回落默认，实际 %+v", e)
	}
}

func TestSupportedTypesIncludesPostgres(t *testing.T) {
	found := false
	for _, tp := range SupportedTypes() {
		if tp == model.TypePostgres {
			found = true
		}
	}
	if !found {
		t.Error("SupportedTypes 应包含 postgres")
	}
}
