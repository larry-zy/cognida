package tools

import (
	"context"
	"testing"

	agentctx "link/internal/model/agent"
	"link/internal/model/dataprofile"
)

// fakeProfileStore 仅实现 columnFactsHint 用到的 ListByTable；其余端口置为无操作。
type fakeProfileStore struct {
	profiles  []*dataprofile.ColumnProfile
	err       error
	gotTenant int64
	gotDsID   string
	gotSchema string
	gotTable  string
}

func (f *fakeProfileStore) Upsert(context.Context, []*dataprofile.ColumnProfile) error { return nil }

func (f *fakeProfileStore) ListByTable(_ context.Context, tenantID int64, dsID, schema, table string) ([]*dataprofile.ColumnProfile, error) {
	f.gotTenant, f.gotDsID, f.gotSchema, f.gotTable = tenantID, dsID, schema, table
	return f.profiles, f.err
}

func (f *fakeProfileStore) ListByDatasourceTable(context.Context, int64, string, string) ([]*dataprofile.ColumnProfile, error) {
	return nil, nil
}

func ctxWithIdentity(tenantID int64, datasourceID string) context.Context {
	ctx := agentctx.WithTenantID(context.Background(), tenantID)
	if datasourceID != "" {
		ctx = agentctx.WithDatasourceID(ctx, datasourceID)
	}
	return ctx
}

// TestColumnFactsHint_OnlyEnumColumns 校验只回带 TopValues 的枚举列，且取值裁到上限。
func TestColumnFactsHint_OnlyEnumColumns(t *testing.T) {
	many := make([]dataprofile.ValueFrequency, 0, maxHintEnumValues+5)
	for i := 0; i < maxHintEnumValues+5; i++ {
		many = append(many, dataprofile.ValueFrequency{Value: string(rune('a' + i)), Count: int64(i)})
	}
	store := &fakeProfileStore{profiles: []*dataprofile.ColumnProfile{
		{ColumnName: "status", Distinct: 3, TopValues: []dataprofile.ValueFrequency{
			{Value: "completed", Count: 10}, {Value: "shipped", Count: 5},
		}},
		{ColumnName: "id", Distinct: 1000}, // 高基数无 TopValues → 应被剔除
		{ColumnName: "tag", Distinct: 30, TopValues: many},
	}}
	target := &queryTarget{dbName: "shop"}

	out := columnFactsHint(ctxWithIdentity(7, "ds-1"), store, target, "", "orders")
	if out == nil {
		t.Fatal("应返回枚举列事实")
	}
	if _, ok := out["id"]; ok {
		t.Fatal("无 TopValues 的高基数列不应出现在 column_facts")
	}
	status, ok := out["status"].(map[string]interface{})
	if !ok {
		t.Fatalf("status 事实类型异常: %T", out["status"])
	}
	if vals, _ := status["top_values"].([]string); len(vals) != 2 || vals[0] != "completed" {
		t.Fatalf("status 取值分布异常: %+v", status["top_values"])
	}
	if status["distinct"].(int64) != 3 {
		t.Fatalf("status distinct 应透传: %+v", status["distinct"])
	}
	tag := out["tag"].(map[string]interface{})
	if vals := tag["top_values"].([]string); len(vals) != maxHintEnumValues {
		t.Fatalf("枚举取值应裁到 %d, got %d", maxHintEnumValues, len(vals))
	}

	// 坐标透传校验：schema=target.dbName、dsID 显式 database_id 优先。
	if store.gotSchema != "shop" || store.gotTable != "orders" || store.gotTenant != 7 || store.gotDsID != "ds-1" {
		t.Fatalf("坐标透传异常: tenant=%d ds=%q schema=%q table=%q",
			store.gotTenant, store.gotDsID, store.gotSchema, store.gotTable)
	}
}

// TestColumnFactsHint_DatasourceFallback 校验 database_id 为空时回落会话选定数据源。
func TestColumnFactsHint_DatasourceFallback(t *testing.T) {
	store := &fakeProfileStore{profiles: []*dataprofile.ColumnProfile{
		{ColumnName: "status", TopValues: []dataprofile.ValueFrequency{{Value: "x", Count: 1}}},
	}}
	target := &queryTarget{dbName: "shop"}

	columnFactsHint(ctxWithIdentity(7, "sess-ds"), store, target, "", "orders")
	if store.gotDsID != "sess-ds" {
		t.Fatalf("database_id 为空应回落会话数据源, got %q", store.gotDsID)
	}
	// 显式 database_id 覆盖会话数据源。
	columnFactsHint(ctxWithIdentity(7, "sess-ds"), store, target, "explicit", "orders")
	if store.gotDsID != "explicit" {
		t.Fatalf("显式 database_id 应优先, got %q", store.gotDsID)
	}
}

// TestColumnFactsHint_Degrades 校验 store 缺失/无租户/无枚举列时静默返回 nil（不阻断 available_columns）。
func TestColumnFactsHint_Degrades(t *testing.T) {
	target := &queryTarget{dbName: "shop"}
	if columnFactsHint(ctxWithIdentity(7, "ds"), nil, target, "", "orders") != nil {
		t.Fatal("store 为 nil 应返回 nil")
	}
	empty := &fakeProfileStore{}
	if columnFactsHint(context.Background(), empty, target, "", "orders") != nil {
		t.Fatal("缺租户应返回 nil")
	}
	nonEnum := &fakeProfileStore{profiles: []*dataprofile.ColumnProfile{{ColumnName: "amount", Distinct: 9999}}}
	if columnFactsHint(ctxWithIdentity(7, "ds"), nonEnum, target, "", "orders") != nil {
		t.Fatal("无枚举列（无 TopValues）应返回 nil")
	}
	failing := &fakeProfileStore{err: context.DeadlineExceeded}
	if columnFactsHint(ctxWithIdentity(7, "ds"), failing, target, "", "orders") != nil {
		t.Fatal("画像读取出错应静默返回 nil，不阻断 available_columns")
	}
}

// TestColumnFactsHint_CapsEnumColumns 校验单表枚举列数裁到上限，防宽表存量画像灌爆观察。
func TestColumnFactsHint_CapsEnumColumns(t *testing.T) {
	profiles := make([]*dataprofile.ColumnProfile, 0, maxHintFactColumns+5)
	for i := 0; i < maxHintFactColumns+5; i++ {
		profiles = append(profiles, &dataprofile.ColumnProfile{
			ColumnName: "c" + string(rune('A'+i)),
			TopValues:  []dataprofile.ValueFrequency{{Value: "v", Count: 1}},
		})
	}
	store := &fakeProfileStore{profiles: profiles}
	out := columnFactsHint(ctxWithIdentity(7, "ds"), store, &queryTarget{dbName: "shop"}, "", "orders")
	if len(out) != maxHintFactColumns {
		t.Fatalf("枚举列应裁到 %d, got %d", maxHintFactColumns, len(out))
	}
}
