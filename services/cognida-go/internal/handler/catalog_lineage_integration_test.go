//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	c "cognida/internal/model/catalog"
	d "cognida/internal/model/datasource"
	l "cognida/internal/model/lineage"
	repo "cognida/internal/repository/mysql"
	graphrepo "cognida/internal/repository/neo4j"
	catalogsvc "cognida/internal/service/catalog"
	lineagesvc "cognida/internal/service/lineage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	graph "github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCatalogLineageHTTPRealStores(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("MYSQL_DSN")
	require.NotEmpty(t, dsn)
	db, e := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	t.Cleanup(func() { sql.Close() })
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	t.Cleanup(func() {
		for _, table := range []string{"business_glossary", "asset_column", "asset_table", "data_sources"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", tenant).Delete(map[string]any{}).Error)
		}
	})
	source := &d.DataSource{ID: uuid.NewString(), TenantID: tenant, Name: "api-test", Type: d.TypeMySQL, Status: d.StatusActive, CreatedBy: 1}
	require.NoError(t, repo.NewDataSourceRepository(db).Create(ctx, source))
	cr := repo.NewCatalogRepository(db)
	_, e = cr.Snapshot(ctx, tenant, source.ID, []c.AssetTable{{LastSyncedAt: time.Now().UTC(), Schema: "fixture", TableName: "orders", Type: "table", Governance: c.Governance{Classification: "secret"}, Columns: []c.AssetColumn{{Name: "amount", DataType: "decimal", Governance: c.Governance{Classification: "internal"}, SampleValues: []string{"private-value"}}}}})
	require.NoError(t, e)
	page, e := cr.List(ctx, tenant, c.Filter{})
	require.NoError(t, e)
	require.Len(t, page.Items, 1)
	asset, e := cr.Get(ctx, tenant, page.Items[0].ID)
	require.NoError(t, e)
	uri := os.Getenv("NEO4J_URI")
	require.NotEmpty(t, uri)
	driver, e := graph.NewDriverWithContext(uri, graph.BasicAuth(os.Getenv("NEO4J_USERNAME"), os.Getenv("NEO4J_PASSWORD"), ""))
	require.NoError(t, e)
	t.Cleanup(func() { driver.Close(ctx) })
	database := os.Getenv("NEO4J_DATABASE")
	if database == "" {
		database = "neo4j"
	}
	gr := graphrepo.NewLineageRepository(driver, database)
	t.Cleanup(func() {
		_, err := graph.ExecuteQuery(ctx, driver, "MATCH(n:lineage_node {tenant_id:$tenant}) DETACH DELETE n", map[string]any{"tenant": tenant}, graph.EagerResultTransformer, graph.ExecuteQueryWithDatabase(database))
		require.NoError(t, err)
	})
	require.NoError(t, gr.Upsert(ctx, tenant, "http-projection", l.Graph{Nodes: []l.Node{{ID: "column", Kind: "PhysicalColumn", AssetID: asset.Columns[0].ID}, {ID: "metric", Kind: "Metric", AssetID: "metric-id"}}, Edges: []l.Edge{{From: "column", To: "metric", Kind: "depends"}}}))
	api := gin.New()
	api.Use(func(g *gin.Context) {
		if g.GetHeader("X-Test-Tenant") == "" {
			g.AbortWithStatus(401)
			return
		}
		id, _ := strconv.ParseInt(g.GetHeader("X-Test-Tenant"), 10, 64)
		g.Set("tenant_id", id)
		g.Set("user_id", int64(1))
	})
	ch := NewCatalogHandler(catalogsvc.NewService(cr, nil, nil, nil))
	ch.Authorize = func(_ context.Context, _, _ int64, _ string) error { return c.ErrInvalid }
	ch.Register(api.Group(""))
	NewLineageHandler(lineagesvc.NewService(gr, cr, repo.NewSemanticRepository(db))).Register(api.Group(""))
	request := func(method, path string, body any, who int64) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if who > 0 {
			r.Header.Set("X-Test-Tenant", strconv.FormatInt(who, 10))
		}
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		return w
	}
	require.Equal(t, 401, request("GET", "/catalog/tables", nil, 0).Code)
	for _, path := range []string{"/catalog/tables", "/catalog/tables/" + asset.ID} {
		w := request("GET", path, nil, tenant)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), "private-value")
	}
	require.Equal(t, 404, request("GET", "/catalog/tables/"+asset.ID, nil, tenant+1).Code)
	require.Equal(t, 403, request("PUT", "/catalog/tables/"+asset.ID, c.Governance{Classification: "public"}, tenant).Code)
	require.Equal(t, 403, request("POST", "/catalog/sync", map[string]any{"datasource_id": source.ID}, tenant).Code)
	ch.Authorize = nil
	w := request("POST", "/catalog/glossary", map[string]any{"term": "Revenue", "synonyms": []string{"GMV"}, "asset_ids": []string{asset.ID}}, tenant)
	require.Equal(t, 200, w.Code, w.Body.String())
	w = request("GET", "/catalog/tables?q=GMV&page_size=1", nil, tenant)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), asset.ID)
	var envelope struct {
		Data c.Glossary `json:"data"`
	}
	require.NoError(t, json.Unmarshal(request("POST", "/catalog/glossary", map[string]any{"term": "Sales", "asset_ids": []string{asset.ID}}, tenant).Body.Bytes(), &envelope))
	require.NotEmpty(t, envelope.Data.ID)
	require.Equal(t, 404, request("DELETE", "/catalog/glossary/"+envelope.Data.ID, nil, tenant+1).Code)
	require.Equal(t, 200, request("DELETE", "/catalog/glossary/"+envelope.Data.ID, nil, tenant).Code)
	w = request("GET", "/lineage/"+asset.Columns[0].ID+"/impact", nil, tenant)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "metric-id")
	w = request("GET", "/lineage/"+asset.Columns[0].ID, nil, tenant+1)
	require.Equal(t, 200, w.Code)
	require.NotContains(t, w.Body.String(), "metric-id")
	require.Equal(t, 400, request("GET", "/lineage/metric-id?depth=9", nil, tenant).Code)
	require.Equal(t, 400, request("GET", "/lineage/metric-id?direction=invalid", nil, tenant).Code)
	require.Equal(t, 404, request("POST", "/lineage/rebuild", map[string]any{"model_id": uuid.NewString()}, tenant).Code)
}
