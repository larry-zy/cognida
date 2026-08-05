package tools

import (
	"testing"
	"time"

	"link/internal/model/dataprofile"
)

func TestQuoteMySQLIdent(t *testing.T) {
	cases := map[string]string{
		"orders":       "`orders`",
		"order_status": "`order_status`",
		"we`ird":       "`we``ird`", // 内部反引号翻倍转义，防注入
		"":             "``",
	}
	for in, want := range cases {
		if got := quoteMySQLIdent(in); got != want {
			t.Errorf("quoteMySQLIdent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsEnumCandidate(t *testing.T) {
	tests := []struct {
		name     string
		col      ColumnSchema
		distinct int64
		rowCount int64
		want     bool
	}{
		{"低基数枚举varchar", ColumnSchema{Type: "varchar(32)"}, 5, 60000, true},
		{"数值型状态码", ColumnSchema{Type: "tinyint"}, 4, 1000, true},
		{"基数超阈值", ColumnSchema{Type: "varchar(64)"}, 51, 60000, false},
		{"唯一标识列(distinct==row)", ColumnSchema{Type: "bigint"}, 1000, 1000, false},
		{"自由文本text", ColumnSchema{Type: "text"}, 3, 100, false},
		{"时间列datetime", ColumnSchema{Type: "datetime"}, 5, 100, false},
		{"连续decimal", ColumnSchema{Type: "decimal(10,2)"}, 10, 100, false},
		{"json列", ColumnSchema{Type: "json"}, 2, 100, false},
		{"基数为0(空表列)", ColumnSchema{Type: "varchar(32)"}, 0, 0, false},
		{"大小写与空格容错", ColumnSchema{Type: "  VARCHAR(16) "}, 3, 100, true},
		{"几何列point", ColumnSchema{Type: "point"}, 3, 100, false},
		{"几何列geometry", ColumnSchema{Type: "geometry"}, 2, 100, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEnumCandidate(tt.col, tt.distinct, tt.rowCount); got != tt.want {
				t.Errorf("isEnumCandidate(%+v, d=%d, n=%d) = %v, want %v",
					tt.col, tt.distinct, tt.rowCount, got, tt.want)
			}
		})
	}
}

func TestProfilesStale(t *testing.T) {
	now := time.Now()
	fresh := []*dataprofile.ColumnProfile{{ProfiledAt: now.Add(-time.Hour)}}
	stale := []*dataprofile.ColumnProfile{{ProfiledAt: now.Add(-profileTTL - time.Hour)}}
	// 多条取最新一条判定：一新一旧应视为新鲜。
	mixed := []*dataprofile.ColumnProfile{
		{ProfiledAt: now.Add(-profileTTL - time.Hour)},
		{ProfiledAt: now.Add(-time.Minute)},
	}

	if !profilesStale(nil) {
		t.Error("空画像应视为过期")
	}
	if profilesStale(fresh) {
		t.Error("TTL 内画像不应过期")
	}
	if !profilesStale(stale) {
		t.Error("超 TTL 画像应过期")
	}
	if profilesStale(mixed) {
		t.Error("按最新一条判定，含新条目不应过期")
	}
}

func TestAttachColumnFacts(t *testing.T) {
	table := &TableSchema{
		TableName: "orders",
		Columns: []ColumnSchema{
			{Name: "id"},
			{Name: "status"},
			{Name: "note"}, // 无画像的列应保持 Facts=nil
		},
	}
	profiles := []*dataprofile.ColumnProfile{
		{ColumnName: "id", RowCount: 100, Distinct: 100, NullRate: 0},
		{ColumnName: "status", RowCount: 100, Distinct: 3, NullRate: 0.1,
			TopValues: []dataprofile.ValueFrequency{{Value: "completed", Count: 55}}},
	}
	attachColumnFacts(table, profiles, true)

	if table.Columns[0].Facts == nil || table.Columns[0].Facts.Distinct != 100 {
		t.Fatalf("id 列事实未正确挂载: %+v", table.Columns[0].Facts)
	}
	sf := table.Columns[1].Facts
	if sf == nil || sf.NullRate != 0.1 || len(sf.TopValues) != 1 || sf.TopValues[0].Value != "completed" {
		t.Fatalf("status 列事实未正确挂载: %+v", sf)
	}
	if !sf.Stale {
		t.Error("stale 标记未透传")
	}
	if table.Columns[2].Facts != nil {
		t.Error("无画像的列不应有 Facts")
	}
}

func TestAttachColumnFactsEmpty(t *testing.T) {
	table := &TableSchema{Columns: []ColumnSchema{{Name: "a"}}}
	attachColumnFacts(table, nil, false) // 空画像不应 panic，也不挂载
	if table.Columns[0].Facts != nil {
		t.Error("空画像不应挂载 Facts")
	}
}

func TestProfileKeyDistinct(t *testing.T) {
	// 不同物理坐标必须产生不同键，避免后台画像去重误判为同一任务。
	base := profileKey(1, "ds", "db", "orders")
	diffs := []string{
		profileKey(2, "ds", "db", "orders"),   // 租户
		profileKey(1, "ds2", "db", "orders"),  // 数据源
		profileKey(1, "ds", "db2", "orders"),  // 库
		profileKey(1, "ds", "db", "customers"), // 表
	}
	for _, d := range diffs {
		if d == base {
			t.Errorf("不同坐标产生了相同键: %q", d)
		}
	}
	if profileKey(1, "ds", "db", "orders") != base {
		t.Error("相同坐标应产生相同键（幂等）")
	}
}
