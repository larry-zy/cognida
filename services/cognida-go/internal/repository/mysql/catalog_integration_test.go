//go:build integration

package mysql

import (
	c "cognida/internal/model/catalog"
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCatalogSnapshotGovernanceSearchAndTenantIsolation(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	tenant := time.Now().UnixNano()
	id := uuid.NewString()
	source := DataSourceModel{ID: id, TenantID: tenant, Name: id, Type: "mysql", Host: "localhost", Port: 3306, DatabaseName: "fixture", Username: "reader", PasswordEncrypted: "fixture", Status: "active", CreatedBy: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	require.NoError(t, db.Create(&source).Error)
	t.Cleanup(func() {
		db.Where("tenant_id=?", tenant).Delete(&glossaryRow{})
		db.Where("tenant_id=?", tenant).Delete(&catalogColumnRow{})
		db.Where("tenant_id=?", tenant).Delete(&catalogTableRow{})
		db.Where("id=?", id).Delete(&DataSourceModel{})
	})
	repo := NewCatalogRepository(db)
	snapshot := []c.AssetTable{{Schema: "fixture", TableName: "orders", Type: "table", LastSyncedAt: time.Now(), Governance: c.Governance{Tags: []string{}, Classification: "internal"}, Columns: []c.AssetColumn{{Name: "amount", DataType: "decimal", Governance: c.Governance{Classification: "internal"}}, {Name: "obsolete", DataType: "text"}}}}
	first, err := repo.Snapshot(ctx, tenant, id, snapshot)
	require.NoError(t, err)
	require.Equal(t, 1, first.Created)
	page, err := repo.List(ctx, tenant, c.Filter{})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	asset := page.Items[0].ID
	table, err := repo.Get(ctx, tenant, asset)
	require.NoError(t, err)
	require.Len(t, table.Columns, 2)
	columnID := table.Columns[0].ID
	require.NoError(t, repo.UpdateGovernance(ctx, tenant, asset, c.Governance{Owner: "analyst", Tags: []string{"core"}, Classification: "confidential"}))
	_, err = repo.SaveGlossary(ctx, tenant, c.Glossary{Term: "营收", Synonyms: []string{"GMV"}, AssetIDs: []string{columnID}})
	require.NoError(t, err)
	for _, q := range []string{"营收", "GMV"} {
		p, e := repo.List(ctx, tenant, c.Filter{Query: q})
		require.NoError(t, e)
		require.Len(t, p.Items, 1)
		require.Equal(t, "glossary", p.Items[0].MatchSource)
	}
	snapshot[0].Columns = snapshot[0].Columns[:1]
	second, err := repo.Snapshot(ctx, tenant, id, snapshot)
	require.NoError(t, err)
	require.Equal(t, 0, second.Created)
	require.Equal(t, 1, second.Deprecated)
	table, err = repo.Get(ctx, tenant, asset)
	require.NoError(t, err)
	require.Equal(t, "analyst", table.Owner)
	require.Equal(t, columnID, table.Columns[0].ID)
	require.True(t, table.Columns[1].Deprecated)
	names, err := repo.ActiveColumnNames(ctx, tenant, []string{asset})
	require.NoError(t, err)
	require.Equal(t, []string{"amount"}, names[asset])
	names, err = repo.ActiveColumnNames(ctx, tenant+1, []string{asset})
	require.NoError(t, err)
	require.Empty(t, names)
	_, err = repo.ResolveCoordinate(ctx, tenant, id, "fixture", "orders", "obsolete")
	require.NoError(t, err, "historical coordinates remain resolvable for access-policy checks")
	_, err = repo.Get(ctx, tenant+1, asset)
	require.ErrorIs(t, err, c.ErrNotFound)
	require.ErrorIs(t, repo.UpdateGovernance(ctx, tenant+1, asset, c.Governance{}), c.ErrNotFound)
	_, err = repo.SaveGlossary(ctx, tenant+1, c.Glossary{Term: "attack", AssetIDs: []string{columnID}})
	require.ErrorIs(t, err, c.ErrNotFound)
	_, err = repo.Snapshot(ctx, tenant, id, nil)
	require.NoError(t, err)
	_, err = repo.ResolveCoordinate(ctx, tenant, id, "fixture", "orders", "")
	require.NoError(t, err, "retired coordinates retain their governance classification")
	names, err = repo.ActiveColumnNames(ctx, tenant, []string{asset})
	require.NoError(t, err)
	require.Empty(t, names)
}
