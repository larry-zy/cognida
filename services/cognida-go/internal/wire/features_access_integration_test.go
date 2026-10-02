//go:build integration

package wire

import (
	c "cognida/internal/model/catalog"
	ds "cognida/internal/model/datasource"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/catalog"
	"cognida/internal/service/datasource"
	"cognida/internal/service/knowledge"
	"cognida/internal/service/lifecycle"
	"context"
	"encoding/json"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFeatureAccessIntegrationSensitiveCatalogBeforeSampling(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("MYSQL_DSN")
	require.NotEmpty(t, dsn)
	cfg, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sql.Close()) })
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	table := "access_scope_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS `"+table+"`").Error)
		for _, name := range []string{"asset_column", "asset_table", "data_sources", "tenant_users", "users"} {
			require.NoError(t, db.Table(name).Where("tenant_id=?", tenant).Delete(map[string]any{}).Error)
		}
		require.NoError(t, db.Where("id=?", tenant).Delete(&mysql.TenantModel{}).Error)
	})
	require.NoError(t, db.Create(&mysql.TenantModel{ID: tenant, Name: "access fixture", APIKey: uuid.NewString(), Status: "active", Business: "test"}).Error)
	principals := map[string]l.Principal{}
	for _, role := range []string{"member", "admin"} {
		user := mysql.UserModel{TenantID: tenant, Username: role, Email: role + "@fixture.invalid", PasswordHash: "unusable-test-hash", Status: 1}
		require.NoError(t, db.Create(&user).Error)
		require.NoError(t, db.Create(&mysql.TenantUserModel{TenantID: tenant, UserID: user.ID, Role: role, Status: "active"}).Error)
		principals[role] = l.Principal{TenantID: tenant, UserID: user.ID}
	}
	cipher, err := datasource.NewCipher(uuid.NewString())
	require.NoError(t, err)
	encrypted, err := cipher.Encrypt(cfg.Passwd)
	require.NoError(t, err)
	host, portText, err := net.SplitHostPort(cfg.Addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	sourceRepo := mysql.NewDataSourceRepository(db)
	source := &ds.DataSource{ID: uuid.NewString(), TenantID: tenant, Name: "test", Type: ds.TypeMySQL, Host: host, Port: port, DatabaseName: cfg.DBName, Username: cfg.User, PasswordEncrypted: encrypted, Status: ds.StatusActive, CreatedBy: principals["admin"].UserID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	require.NoError(t, sourceRepo.Create(ctx, source))
	manager := datasource.NewConnectionManager(sourceRepo, cipher, datasource.DefaultPoolOptions())
	t.Cleanup(manager.Close)
	service := datasource.NewService(sourceRepo, cipher, manager, nil)
	catalogRepo := mysql.NewCatalogRepository(db)
	_, err = catalogRepo.Snapshot(ctx, tenant, source.ID, []c.AssetTable{{Schema: cfg.DBName, TableName: table, Type: "table", LastSyncedAt: time.Now(), Governance: c.Governance{Classification: "secret"}, Columns: []c.AssetColumn{{Name: "amount", DataType: "decimal", Governance: c.Governance{Classification: "confidential"}}}}})
	require.NoError(t, err)
	assetID, err := catalogRepo.ResolveCoordinate(ctx, tenant, source.ID, cfg.DBName, table, "")
	require.NoError(t, err)
	asset, err := catalogRepo.Get(ctx, tenant, assetID)
	require.NoError(t, err)
	require.Len(t, asset.Columns, 1)
	access := featureAccess(db, service, catalog.NewService(catalogRepo, manager, service, nil), nil, nil, nil)
	req := lifecycle.SourceRequest{Kind: "datasource", DatasourceID: source.ID, Table: table} // deliberately omit CatalogAssetID
	// The physical table does not exist yet: ErrForbidden rather than a SQL error
	// proves authorization is performed before SampleRecords executes its SELECT.
	_, _, _, err = access(ctx, principals["member"], req)
	require.ErrorIs(t, err, l.ErrForbidden)
	wrong := req
	wrong.Schema = cfg.DBName + "_wrong"
	_, _, _, err = access(ctx, principals["admin"], wrong)
	require.ErrorIs(t, err, l.ErrInvalid)
	require.NoError(t, db.Exec("CREATE TABLE `"+table+"` (amount DECIMAL(12,2) NOT NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO `"+table+"` (amount) VALUES (?)", 123.45).Error)
	for _, scenario := range []struct{ name, table, column, want string }{{"table secret cannot be downgraded", "secret", "confidential", "secret"}, {"column secret inherited", "internal", "secret", "secret"}, {"table confidential inherited", "confidential", "internal", "confidential"}} {
		t.Run(scenario.name, func(t *testing.T) {
			require.NoError(t, catalogRepo.UpdateGovernance(ctx, tenant, assetID, c.Governance{Classification: scenario.table}))
			require.NoError(t, catalogRepo.UpdateGovernance(ctx, tenant, asset.Columns[0].ID, c.Governance{Classification: scenario.column}))
			_, _, _, e := access(ctx, principals["member"], req)
			require.ErrorIs(t, e, l.ErrForbidden)
			input, origin, policy, e := access(ctx, principals["admin"], req)
			require.NoError(t, e)
			require.Equal(t, "table", input.Kind)
			require.Equal(t, []string{"amount"}, input.Columns)
			require.Len(t, input.Rows, 1)
			require.Equal(t, scenario.want, policy.Sensitivity)
			require.Equal(t, assetID, origin.SourceID)
		})
	}
}

type accessPublicationProcessor struct {
	knowledge.DocumentProcessorService
	request *knowledge.ProcessDocumentRequest
}

func (p *accessPublicationProcessor) ProcessDocument(_ context.Context, _ int64, _ int64, in *knowledge.ProcessDocumentRequest) (*knowledge.ProcessDocumentResponse, error) {
	p.request = in
	return &knowledge.ProcessDocumentResponse{KnowledgeID: uuid.NewString(), ParseStatus: "completed"}, nil
}
func TestFeaturePublicationEmptyTitleFallback(t *testing.T) {
	processor := &accessPublicationProcessor{}
	ref := l.DataAssetRef{RefID: l.RefID{AssetID: uuid.NewString(), Version: 2}, Name: "  "}
	ref.AssetRefID = ref.RefID.String()
	body, err := json.Marshal(l.Input{Kind: "text", Text: "approved content"})
	require.NoError(t, err)
	receipt, err := featurePublication(processor, nil)(context.Background(), l.Principal{TenantID: 1, UserID: 2}, ref, body, map[string]any{"knowledge_base_id": uuid.NewString()})
	require.NoError(t, err)
	require.Equal(t, "published", receipt["publication_status"])
	require.NotNil(t, processor.request)
	require.Equal(t, "Data asset "+ref.AssetID, processor.request.Title)
	require.Equal(t, "lifecycle:"+ref.AssetRefID, processor.request.IdempotencyKey)
	require.Equal(t, []byte("approved content"), processor.request.Content)
}
