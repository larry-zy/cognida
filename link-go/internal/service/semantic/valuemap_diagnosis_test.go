package semantic

import (
	"context"
	"errors"
	"testing"

	"link/internal/model/dataprofile"
	model "link/internal/model/semantic"
)

// fakeProfileReader 内存版 ProfileReader：按 (数据源, 表) 存画像列表，供诊断回环。
type fakeProfileReader struct {
	byTable map[string][]*dataprofile.ColumnProfile
	err     error
}

func (r *fakeProfileReader) ListByDatasourceTable(_ context.Context, _ int64, datasourceID, table string) ([]*dataprofile.ColumnProfile, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.byTable[datasourceID+"\x00"+table], nil
}

func vf(pairs ...interface{}) []dataprofile.ValueFrequency {
	out := make([]dataprofile.ValueFrequency, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, dataprofile.ValueFrequency{Value: pairs[i].(string), Count: int64(pairs[i+1].(int))})
	}
	return out
}

// --- 纯比对逻辑 ------------------------------------------------------------

func TestDiagnoseValueMap_DeadAndGap(t *testing.T) {
	// 真实枚举：completed/shipped/paid；ValueMap 里 已退款→refunded 是死映射（真实无此值），
	// paid 无任何标签指向 → 覆盖缺口。
	vm := map[string]string{"已完成": "completed", "已发货": "shipped", "已退款": "refunded"}
	p := &dataprofile.ColumnProfile{
		RowCount: 10, Distinct: 3,
		TopValues: vf("completed", 6, "shipped", 3, "paid", 1),
	}
	dead, gaps, status := diagnoseValueMap(vm, p)
	if status != DiagStatusOK {
		t.Fatalf("状态应为 ok, got %s", status)
	}
	if len(dead) != 1 || dead[0].PhysicalValue != "refunded" || dead[0].Label != "已退款" {
		t.Errorf("死映射应为 已退款→refunded, got %+v", dead)
	}
	if len(gaps) != 1 || gaps[0].Value != "paid" || gaps[0].Count != 1 {
		t.Errorf("覆盖缺口应为 paid×1, got %+v", gaps)
	}
}

func TestDiagnoseValueMap_CaseInsensitive(t *testing.T) {
	// 纯大小写差异不应误判为死映射（_ci collation 口径，仅折叠大小写）。
	vm := map[string]string{"已完成": "Completed", "已发货": "SHIPPED"}
	p := &dataprofile.ColumnProfile{
		RowCount: 5, Distinct: 2, TopValues: vf("completed", 3, "shipped", 2),
	}
	dead, gaps, status := diagnoseValueMap(vm, p)
	if status != DiagStatusOK || len(dead) != 0 || len(gaps) != 0 {
		t.Errorf("纯大小写差异不应产生死映射或缺口: dead=%+v gaps=%+v status=%s", dead, gaps, status)
	}
}

func TestDiagnoseValueMap_WhitespaceIsDead(t *testing.T) {
	// 前导空白在运行时（MySQL `=` 从不左裁）恒匹配 0 行，应如实报为死映射——
	// 这正是旧版 TrimSpace 会漏报的一类真实失效。
	vm := map[string]string{"已完成": " completed"} // 前导空格笔误
	p := &dataprofile.ColumnProfile{RowCount: 5, Distinct: 1, TopValues: vf("completed", 5)}
	dead, gaps, status := diagnoseValueMap(vm, p)
	if status != DiagStatusOK {
		t.Fatalf("状态应为 ok, got %s", status)
	}
	if len(dead) != 1 || dead[0].PhysicalValue != " completed" {
		t.Errorf("带前导空白的映射应报死映射: %+v", dead)
	}
	// 真实 completed 无标签覆盖（映射侧带空白不匹配）→ 覆盖缺口。
	if len(gaps) != 1 || gaps[0].Value != "completed" {
		t.Errorf("真实 completed 应作覆盖缺口: %+v", gaps)
	}
}

func TestDiagnoseValueMap_Unreliable(t *testing.T) {
	cases := map[string]*dataprofile.ColumnProfile{
		"空表":     {RowCount: 0, Distinct: 0},
		"高基数无枚举": {RowCount: 100, Distinct: 100},
		"枚举未采全":  {RowCount: 100, Distinct: 30, TopValues: vf("a", 10, "b", 8)}, // len(top)<distinct
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			dead, gaps, status := diagnoseValueMap(map[string]string{"x": "y"}, p)
			if status != DiagStatusNotEnum || dead != nil || gaps != nil {
				t.Errorf("不可靠画像应返回 not_enum 且不产出条目: dead=%+v gaps=%+v status=%s", dead, gaps, status)
			}
		})
	}
}

func TestDiagnoseValueMap_NoValueMapAllGaps(t *testing.T) {
	// 无 ValueMap 的枚举列：全部真实值都是覆盖缺口，无死映射。
	p := &dataprofile.ColumnProfile{RowCount: 3, Distinct: 2, TopValues: vf("a", 2, "b", 1)}
	dead, gaps, status := diagnoseValueMap(nil, p)
	if status != DiagStatusOK || len(dead) != 0 || len(gaps) != 2 {
		t.Errorf("无映射枚举列应全为缺口: dead=%+v gaps=%+v status=%s", dead, gaps, status)
	}
}

func TestIsBareColumn(t *testing.T) {
	bare := []string{"status", "order_status", "col1", "`status`", "  status  ", "_x", "订单状态", "amount$"}
	notBare := []string{"", "LOWER(status)", "a+b", "t.status", "  ", "1col", "a-b"}
	for _, s := range bare {
		if !isBareColumn(s) {
			t.Errorf("%q 应判为裸列名", s)
		}
	}
	for _, s := range notBare {
		if isBareColumn(s) {
			t.Errorf("%q 不应判为裸列名", s)
		}
	}
}

func TestPickColumnProfile(t *testing.T) {
	profs := []*dataprofile.ColumnProfile{
		{SchemaName: "db1", ColumnName: "status"},
		{SchemaName: "db1", ColumnName: "amount"},
	}
	got, n := pickColumnProfile(profs, "STATUS") // 列名大小写不敏感
	if got == nil || n != 1 {
		t.Errorf("应命中单 schema 的 status 列: got=%v schemas=%d", got, n)
	}
	// 跨库同名表 → 多义。
	amb := append(profs, &dataprofile.ColumnProfile{SchemaName: "db2", ColumnName: "status"})
	if _, n := pickColumnProfile(amb, "status"); n != 2 {
		t.Errorf("跨 schema 同名列应报多义(2), got %d", n)
	}
	if got, _ := pickColumnProfile(profs, "missing"); got != nil {
		t.Error("不存在的列应返回 nil")
	}
}

// --- 服务级编排 ------------------------------------------------------------

func newDiagBundle() *model.ModelBundle {
	return &model.ModelBundle{
		Model: &model.SemanticModel{ID: "m1", TenantID: 1, Name: "订单", Status: model.ModelStatusActive},
		LogicalTables: []*model.LogicalTable{
			{ID: "lt1", Name: "订单表", PhysicalTable: "orders", DatabaseID: ""},
		},
		Dimensions: []*model.Dimension{
			{ID: "d1", LogicalTableID: "lt1", Name: "订单状态", Expr: "status",
				ValueMap: map[string]string{"已完成": "completed", "已退款": "refunded"}},
			{ID: "d2", LogicalTableID: "lt1", Name: "渠道", Expr: "channel"}, // 无映射的枚举列 → 应作缺口上报
			{ID: "d3", LogicalTableID: "lt1", Name: "金额", Expr: "amount"},  // 无映射非枚举 → 不上报
			{ID: "d4", LogicalTableID: "lt1", Name: "表达式", Expr: "LOWER(status)", // 表达式 + 有映射 → not_column
				ValueMap: map[string]string{"x": "y"}},
			{ID: "d5", LogicalTableID: "ltX", Name: "悬挂", Expr: "foo", // 逻辑表缺失 + 有映射 → no_table
				ValueMap: map[string]string{"x": "y"}},
		},
	}
}

func TestDiagnoseValueMaps_Service(t *testing.T) {
	repo := newFakeRepo()
	repo.bundles["m1"] = newDiagBundle()
	reader := &fakeProfileReader{byTable: map[string][]*dataprofile.ColumnProfile{
		"\x00orders": {
			{SchemaName: "link", ColumnName: "status", RowCount: 10, Distinct: 3,
				TopValues: vf("completed", 6, "shipped", 3, "paid", 1)},
			{SchemaName: "link", ColumnName: "channel", RowCount: 10, Distinct: 2,
				TopValues: vf("web", 7, "app", 3)},
			{SchemaName: "link", ColumnName: "amount", RowCount: 10, Distinct: 10}, // 高基数非枚举
		},
	}}
	svc := NewService(repo, &seqIDGen{}, nil, reader)

	report, err := svc.DiagnoseValueMaps(context.Background(), 1, "m1")
	if err != nil {
		t.Fatalf("DiagnoseValueMaps: %v", err)
	}
	byDim := map[string]DimensionValueMapDiagnosis{}
	for _, d := range report.Dimensions {
		byDim[d.DimensionID] = d
	}

	// d1：refunded 死映射 + shipped/paid 覆盖缺口。
	d1 := byDim["d1"]
	if d1.Status != DiagStatusOK || len(d1.DeadMappings) != 1 || d1.DeadMappings[0].PhysicalValue != "refunded" {
		t.Errorf("d1 死映射不符: %+v", d1)
	}
	if len(d1.CoverageGaps) != 2 {
		t.Errorf("d1 应有 2 个覆盖缺口(shipped/paid): %+v", d1.CoverageGaps)
	}
	// d2：无映射枚举列 → 全量缺口上报。
	d2 := byDim["d2"]
	if d2.Status != DiagStatusOK || len(d2.CoverageGaps) != 2 || len(d2.DeadMappings) != 0 {
		t.Errorf("d2 应作无映射枚举列上报缺口: %+v", d2)
	}
	// d3：无映射非枚举 → 不上报。
	if _, ok := byDim["d3"]; ok {
		t.Error("d3(无映射非枚举)不应入报告")
	}
	// d4：表达式 + 有映射 → not_column。
	if byDim["d4"].Status != DiagStatusNotColumn {
		t.Errorf("d4 应为 not_column: %+v", byDim["d4"])
	}
	// d5：逻辑表缺失 → no_table。
	if byDim["d5"].Status != DiagStatusNoTable {
		t.Errorf("d5 应为 no_table: %+v", byDim["d5"])
	}
}

func TestDiagnoseValueMaps_Disabled(t *testing.T) {
	repo := newFakeRepo()
	repo.bundles["m1"] = newDiagBundle()
	svc := NewService(repo, &seqIDGen{}, nil, nil) // profiles 未接线
	if _, err := svc.DiagnoseValueMaps(context.Background(), 1, "m1"); !errors.Is(err, ErrProfileDisabled) {
		t.Errorf("未接线画像应返回 ErrProfileDisabled, got %v", err)
	}
}

func TestDiagnoseValueMaps_NoProfileAndTenantIsolation(t *testing.T) {
	repo := newFakeRepo()
	repo.bundles["m1"] = newDiagBundle()
	reader := &fakeProfileReader{byTable: map[string][]*dataprofile.ColumnProfile{}} // 空：无任何画像
	svc := NewService(repo, &seqIDGen{}, nil, reader)

	// 有映射但无画像 → d1 状态 no_profile；d2(无映射)不上报。
	report, err := svc.DiagnoseValueMaps(context.Background(), 1, "m1")
	if err != nil {
		t.Fatalf("DiagnoseValueMaps: %v", err)
	}
	var d1 *DimensionValueMapDiagnosis
	for i := range report.Dimensions {
		if report.Dimensions[i].DimensionID == "d1" {
			d1 = &report.Dimensions[i]
		}
	}
	if d1 == nil || d1.Status != DiagStatusNoProfile {
		t.Errorf("无画像时 d1 应为 no_profile: %+v", d1)
	}

	// 跨租户不可见 → ErrModelNotFound。
	if _, err := svc.DiagnoseValueMaps(context.Background(), 999, "m1"); !errors.Is(err, model.ErrModelNotFound) {
		t.Errorf("跨租户应不可见, got %v", err)
	}
}
