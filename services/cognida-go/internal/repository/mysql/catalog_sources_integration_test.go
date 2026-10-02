//go:build integration

package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	c "cognida/internal/model/catalog"
	d "cognida/internal/model/datasource"
	"cognida/internal/model/semantic"
	graphrepo "cognida/internal/repository/neo4j"
	catalogsvc "cognida/internal/service/catalog"
	dssvc "cognida/internal/service/datasource"
	lineagesvc "cognida/internal/service/lineage"
	"github.com/google/uuid"
	graph "github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/stretchr/testify/require"
)

type catalogTestIDs struct{}

func (catalogTestIDs) Generate() string { return uuid.NewString() }
func TestCatalogRealSourcesAndLineage(t *testing.T) {
	for _, kind := range []d.Type{d.TypeMySQL, d.TypePostgres} {
		t.Run(string(kind), func(t *testing.T) {
			db := newIntegrationDB(t)
			ctx := context.Background()
			tenant := time.Now().UnixNano()
			fixture := "cat_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			var external *sql.DB
			var source *d.DataSource
			if kind == d.TypeMySQL {
				sqlDB, e := db.DB()
				require.NoError(t, e)
				_, e = sqlDB.Exec("CREATE DATABASE `" + fixture + "`")
				require.NoError(t, e)
				t.Cleanup(func() { sqlDB.Exec("DROP DATABASE `" + fixture + "`") })
				port, _ := strconv.Atoi(os.Getenv("DB_PORT"))
				source = &d.DataSource{Host: os.Getenv("DB_HOST"), Port: port, Username: os.Getenv("DB_USER"), DatabaseName: fixture, Type: kind}
				drv, _ := dssvc.DriverFor(kind)
				dsn, e := drv.BuildDSN(source, os.Getenv("DB_PASSWORD"))
				require.NoError(t, e)
				external, e = sql.Open("mysql", dsn)
				require.NoError(t, e)
			} else {
				dsn := os.Getenv("CATALOG_TEST_PG_DSN")
				require.NotEmpty(t, dsn, "set CATALOG_TEST_PG_DSN for a dedicated local PG test connection")
				admin, e := sql.Open("postgres", dsn)
				require.NoError(t, e)
				t.Cleanup(func() { admin.Close() })
				_, e = admin.Exec(`CREATE SCHEMA "` + fixture + `"`)
				require.NoError(t, e)
				t.Cleanup(func() { admin.Exec(`DROP SCHEMA "` + fixture + `" CASCADE`) })
				extra, _ := json.Marshal(map[string]string{"schema": fixture, "sslmode": "disable"})
				source = &d.DataSource{Host: "127.0.0.1", Port: 5432, Username: os.Getenv("CATALOG_TEST_PG_USER"), DatabaseName: "postgres", Extra: extra, Type: kind}
				drv, _ := dssvc.DriverFor(kind)
				dsn, e = drv.BuildDSN(source, os.Getenv("CATALOG_TEST_PG_PASSWORD"))
				require.NoError(t, e)
				external, e = sql.Open("postgres", dsn)
				require.NoError(t, e)
			}
			t.Cleanup(func() { external.Close() })
			_, e := external.Exec("CREATE TABLE orders (id INTEGER PRIMARY KEY, amount DECIMAL(12,2), obsolete VARCHAR(20))")
			require.NoError(t, e)
			_, e = external.Exec("INSERT INTO orders VALUES(1,10,'a'),(2,NULL,'b'),(3,10,'c')")
			require.NoError(t, e)
			_, e = external.Exec("CREATE TABLE order_items (id INTEGER PRIMARY KEY, order_id INTEGER REFERENCES orders(id))")
			require.NoError(t, e)
			// MySQL requires the table-level syntax for an enforced FK.
			if kind == d.TypeMySQL {
				_, e = external.Exec("ALTER TABLE order_items ADD FOREIGN KEY (order_id) REFERENCES orders(id)")
				require.NoError(t, e)
			}
			_, e = external.Exec("CREATE VIEW order_totals AS SELECT id, amount FROM orders")
			require.NoError(t, e)
			repo := NewDataSourceRepository(db)
			cipher, e := dssvc.NewCipher(uuid.NewString())
			require.NoError(t, e)
			password := os.Getenv("DB_PASSWORD")
			if kind == d.TypePostgres {
				password = os.Getenv("CATALOG_TEST_PG_PASSWORD")
			}
			source.PasswordEncrypted, e = cipher.Encrypt(password)
			require.NoError(t, e)
			source.ID = uuid.NewString()
			source.Name = source.ID
			source.TenantID = tenant
			source.Status = d.StatusActive
			source.CreatedBy = 1
			require.NoError(t, repo.Create(ctx, source))
			t.Cleanup(func() {
				db.Where("tenant_id=?", tenant).Delete(&glossaryRow{})
				db.Where("tenant_id=?", tenant).Delete(&catalogColumnRow{})
				db.Where("tenant_id=?", tenant).Delete(&catalogTableRow{})
				repo.Delete(ctx, source.ID, tenant)
			})
			cm := dssvc.NewConnectionManager(repo, cipher, dssvc.DefaultPoolOptions())
			t.Cleanup(cm.Close)
			dsService := dssvc.NewService(repo, cipher, cm, catalogTestIDs{})
			catalogRepo := NewCatalogRepository(db)
			service := catalogsvc.NewService(catalogRepo, cm, dsService, nil)
			result, e := service.Sync(ctx, tenant, 1, source.ID, true)
			require.NoError(t, e)
			require.Empty(t, result.Warnings)
			require.Equal(t, 3, result.Created)
			p, e := service.List(ctx, tenant, c.Filter{DatasourceID: source.ID})
			require.NoError(t, e)
			require.Len(t, p.Items, 3)
			var ordersID string
			for _, item := range p.Items {
				switch item.TableName {
				case "orders":
					ordersID = item.ID
				case "order_totals":
					require.Equal(t, "view", item.Type)
				case "order_items":
					detail, err := service.Get(ctx, tenant, item.ID)
					require.NoError(t, err)
					found := false
					for _, col := range detail.Columns {
						if col.Name == "order_id" {
							found = col.IsFK
						}
					}
					require.True(t, found, "actual FK metadata")
				}
			}
			require.NotEmpty(t, ordersID)
			asset, e := service.Get(ctx, tenant, ordersID)
			require.NoError(t, e)
			var amount c.AssetColumn
			for _, col := range asset.Columns {
				if col.Name == "amount" {
					amount = col
				}
			}
			require.True(t, amount.Profiled)
			require.InDelta(t, 1.0/3.0, *amount.NullRate, .0001)
			require.Equal(t, int64(1), *amount.DistinctCount)
			_, e = service.SaveGlossary(ctx, tenant, 1, c.Glossary{Term: "GMV", Synonyms: []string{"营收"}, AssetIDs: []string{amount.ID}})
			require.NoError(t, e)
			contextText, e := service.Context(ctx, tenant, "营收")
			require.NoError(t, e)
			require.Contains(t, contextText, "orders")
			require.Contains(t, contextText, source.ID)
			_, e = external.Exec("ALTER TABLE orders DROP COLUMN obsolete")
			require.NoError(t, e)
			_, e = service.Sync(ctx, tenant, 1, source.ID, false)
			require.NoError(t, e)
			asset, e = service.Get(ctx, tenant, asset.ID)
			require.NoError(t, e)
			require.Len(t, asset.Columns, 3)
			if kind == d.TypeMySQL {
				uri := os.Getenv("NEO4J_URI")
				require.NotEmpty(t, uri)
				driver, e := graph.NewDriverWithContext(uri, graph.BasicAuth(os.Getenv("NEO4J_USERNAME"), os.Getenv("NEO4J_PASSWORD"), ""))
				require.NoError(t, e)
				t.Cleanup(func() { driver.Close(ctx) })
				database := os.Getenv("NEO4J_DATABASE")
				if database == "" {
					database = "neo4j"
				}
				graphRepo := graphrepo.NewLineageRepository(driver, database)
				t.Cleanup(func() {
					_, err := graph.ExecuteQuery(ctx, driver, "MATCH(n:lineage_node {tenant_id:$tenant}) DETACH DELETE n", map[string]any{"tenant": tenant}, graph.EagerResultTransformer, graph.ExecuteQueryWithDatabase(database))
					require.NoError(t, err)
				})
				modelRepo := NewSemanticRepository(db)
				bundle := &semantic.ModelBundle{Model: &semantic.SemanticModel{ID: uuid.NewString(), TenantID: tenant, Name: fixture, Status: semantic.ModelStatusActive, Version: 1}, LogicalTables: []*semantic.LogicalTable{{ID: uuid.NewString(), Name: "orders", PhysicalTable: "orders", DatabaseID: source.ID}}, Metrics: []*semantic.Metric{{ID: uuid.NewString(), Name: "Revenue", Expr: "SUM(orders.amount)"}}}
				bundle.LogicalTables[0].ModelID = bundle.Model.ID
				bundle.Metrics[0].ModelID = bundle.Model.ID
				require.NoError(t, modelRepo.UpsertBundle(ctx, bundle))
				t.Cleanup(func() {
					for _, table := range []string{"agent_semantic_dimensions", "agent_semantic_measures", "agent_semantic_metrics", "agent_semantic_relations", "agent_semantic_logical_tables"} {
						db.Exec(fmt.Sprintf("DELETE FROM %s WHERE model_id=?", table), bundle.Model.ID)
					}
					db.Exec("DELETE FROM agent_semantic_models WHERE id=?", bundle.Model.ID)
				})
				lineage := lineagesvc.NewService(graphRepo, catalogRepo, modelRepo)
				g, e := lineage.Rebuild(ctx, tenant, bundle.Model.ID)
				require.NoError(t, e)
				require.NotEmpty(t, g.Edges)
				_, e = lineage.Rebuild(ctx, tenant, bundle.Model.ID)
				require.NoError(t, e)
				impact, e := lineage.Query(ctx, tenant, amount.ID, "downstream", 3)
				require.NoError(t, e)
				found := false
				for _, n := range impact.Nodes {
					if n.Kind == "Metric" && n.Name == "Revenue" {
						found = true
					}
				}
				require.True(t, found)
				upstream, e := lineage.Query(ctx, tenant, bundle.Metrics[0].ID, "upstream", 3)
				require.NoError(t, e)
				require.NotEmpty(t, upstream.Edges, "public semantic metric ID resolves lineage")
				other, e := lineage.Query(ctx, tenant+1, amount.ID, "both", 3)
				require.NoError(t, e)
				require.Empty(t, other.Nodes)
			}
		})
	}
}
