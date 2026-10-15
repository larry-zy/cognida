//go:build integration

package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	localstore "cognida/internal/infrastructure/objectstore"
	collectionmodel "cognida/internal/model/collection"
	dsmodel "cognida/internal/model/datasource"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/model/objectstore"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/agent/resultstore"
	"cognida/internal/service/catalog"
	"cognida/internal/service/collection/fetcher"
	"cognida/internal/service/datasource"
	"cognida/internal/service/lifecycle"

	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This test covers the real HTTP handler, authorization, MySQL repositories,
// datasource SQL, local object storage and Redis result storage. Web HTML is a
// controlled Fetcher response: this is deliberately NOT a public-network E2E
// test, nor an upstream model/evaluation/collection-worker execution test.
type sourceHTTPFetcher struct{}

func (sourceHTTPFetcher) Fetch(_ context.Context, req collectionmodel.FetchRequest) (*collectionmodel.FetchResponse, error) {
	if req.URL != "https://source.example/fixture" {
		return nil, fetcher.ErrInvalidURL
	}
	return &collectionmodel.FetchResponse{CanonicalURL: req.URL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<html><body><h1>Fixture report</h1><p>Revenue 123.45</p><script>private_script()</script></body></html>`)}, nil
}

type sourceHTTPFixture struct {
	db      *gorm.DB
	router  *gin.Engine
	objects *localstore.LocalStore
	results *resultstore.RedisStore
	redis   *redis.Client
	owner   l.Principal
	peer    l.Principal
	other   l.Principal
	dsID    string
	table   string
}

func newSourceHTTPFixture(t *testing.T) *sourceHTTPFixture {
	t.Helper()
	dsn, redisAddr := os.Getenv("MYSQL_DSN"), os.Getenv("REDIS_ADDR")
	require.NotEmpty(t, dsn, "MYSQL_DSN must target a migrated isolated test database")
	require.NotEmpty(t, redisAddr, "REDIS_ADDR is required; do not default to a shared Redis")
	cfg, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	redisDB, _ := strconv.Atoi(os.Getenv("REDIS_DB"))
	client := redis.NewClient(&redis.Options{Addr: redisAddr, Password: os.Getenv("REDIS_PASSWORD"), DB: redisDB})
	require.NoError(t, client.Ping(context.Background()).Err())
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	objects, err := localstore.NewLocal(t.TempDir())
	require.NoError(t, err)
	f := &sourceHTTPFixture{db: db, objects: objects, results: resultstore.NewRedisStore(client), redis: client}
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	f.table = "source_http_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS `"+f.table+"`").Error)
		var taskIDs []string
		require.NoError(t, db.Model(&mysql.EvaluationTaskModel{}).Where("tenant_id IN ?", []int64{tenant, tenant + 1}).Pluck("id", &taskIDs).Error)
		if len(taskIDs) > 0 {
			require.NoError(t, db.Where("task_id IN ?", taskIDs).Delete(&mysql.EvaluationResultModel{}).Error)
		}
		for _, table := range []string{"lifecycle_assets", "evaluation_tasks", "collection_items", "collection_runs", "collection_tasks", "sessions", "chunks", "knowledge", "knowledge_bases", "data_sources", "tenant_users", "users"} {
			require.NoError(t, db.Table(table).Where("tenant_id IN ?", []int64{tenant, tenant + 1}).Delete(map[string]any{}).Error)
		}
		require.NoError(t, db.Where("id IN ?", []int64{tenant, tenant + 1}).Delete(&mysql.TenantModel{}).Error)
	})
	for _, id := range []int64{tenant, tenant + 1} {
		require.NoError(t, db.Create(&mysql.TenantModel{ID: id, Name: "source HTTP fixture", APIKey: uuid.NewString(), Status: "active", Business: "test"}).Error)
	}
	makeUser := func(id int64) l.Principal {
		u := mysql.UserModel{TenantID: id, Username: uuid.NewString(), Email: uuid.NewString() + "@fixture.invalid", PasswordHash: "unusable-test-hash", Status: 1}
		require.NoError(t, db.Create(&u).Error)
		require.NoError(t, db.Create(&mysql.TenantUserModel{TenantID: id, UserID: u.ID, Role: "admin", Status: "active"}).Error)
		return l.Principal{TenantID: id, UserID: u.ID}
	}
	f.owner, f.peer, f.other = makeUser(tenant), makeUser(tenant), makeUser(tenant+1)
	cipher, err := datasource.NewCipher(uuid.NewString())
	require.NoError(t, err)
	password, err := cipher.Encrypt(cfg.Passwd)
	require.NoError(t, err)
	host, portText, err := net.SplitHostPort(cfg.Addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	f.dsID = uuid.NewString()
	sourceRepo := mysql.NewDataSourceRepository(db)
	require.NoError(t, sourceRepo.Create(context.Background(), &dsmodel.DataSource{ID: f.dsID, TenantID: tenant, Name: "source HTTP database", Type: dsmodel.TypeMySQL, Host: host, Port: port, DatabaseName: cfg.DBName, Username: cfg.User, PasswordEncrypted: password, Status: dsmodel.StatusActive, CreatedBy: f.owner.UserID, CreatedAt: time.Now(), UpdatedAt: time.Now()}))
	manager := datasource.NewConnectionManager(sourceRepo, cipher, datasource.DefaultPoolOptions())
	t.Cleanup(manager.Close)
	sources := datasource.NewService(sourceRepo, cipher, manager, nil)
	catalogService := catalog.NewService(mysql.NewCatalogRepository(db), manager, sources, nil)
	require.NoError(t, db.Exec("CREATE TABLE `"+f.table+"` (region VARCHAR(32) NOT NULL, amount DECIMAL(12,2) NOT NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO `"+f.table+"` (region, amount) VALUES (?, ?)", "north", "123.45").Error)
	// No run/invocation is enqueued, but isolate even the queue namespace from a
	// concurrently running test application worker.
	queue := job.NewRedisQueueWithPrefix(client, "source-http:"+uuid.NewString())
	service, err := lifecycle.NewService(mysql.NewLifecycleRepository(db), objects, queue, lifecycle.Config{Authorize: featureAuthorizer(db), Access: featureAccess(db, sources, catalogService, sourceHTTPFetcher{}, objects, f.results)})
	require.NoError(t, err)
	f.router = gin.New()
	f.router.Use(func(c *gin.Context) {
		p := f.owner
		switch c.GetHeader("X-Test-Principal") {
		case "peer":
			p = f.peer
		case "other":
			p = f.other
		}
		c.Set("tenant_id", p.TenantID)
		c.Set("user_id", p.UserID)
		c.Next()
	})
	handler.NewLifecycleHandler(service).Register(f.router.Group("/lifecycle"))
	return f
}

func (f *sourceHTTPFixture) request(t *testing.T, method, path string, source *lifecycle.SourceRequest, key, principal string) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte("null")
	if source != nil {
		var err error
		body, err = json.Marshal(map[string]any{"source": source, "name": "source HTTP fixture"})
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("X-Test-Principal", principal)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}

func (f *sourceHTTPFixture) acquire(t *testing.T, source lifecycle.SourceRequest, sourceID string) l.Input {
	t.Helper()
	key := uuid.NewString()
	w := f.request(t, http.MethodPost, "/lifecycle/assets", &source, key, "")
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		Data l.DataAssetRef `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.Equal(t, f.owner.TenantID, created.Data.TenantID)
	require.Equal(t, l.RawAsset, created.Data.Kind)
	require.Equal(t, source.Kind, created.Data.Provenance.SourceType)
	require.Equal(t, sourceID, created.Data.Provenance.SourceID)
	require.NotEmpty(t, created.Data.Provenance.Digest)
	w = f.request(t, http.MethodPost, "/lifecycle/assets", &source, key, "")
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var replay struct {
		Data l.DataAssetRef `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &replay))
	require.Equal(t, created.Data.AssetRefID, replay.Data.AssetRefID)
	path := "/lifecycle/assets/" + created.Data.AssetID
	require.Equal(t, http.StatusNotFound, f.request(t, http.MethodGet, path, nil, "", "other").Code)
	w = f.request(t, http.MethodGet, path, nil, "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var stored struct {
		Data struct {
			Content l.Input `json:"content"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stored))
	return stored.Data.Content
}

func (f *sourceHTTPFixture) knowledge(t *testing.T) (string, string, string) {
	t.Helper()
	kbID, docID, chunkID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	require.NoError(t, f.db.Create(&mysql.KnowledgeBaseModel{ID: kbID, TenantID: f.owner.TenantID, UserID: f.owner.UserID, Name: "source fixture", Status: 1}).Error)
	require.NoError(t, f.db.Create(&mysql.KnowledgeModel{ID: docID, TenantID: f.owner.TenantID, UserID: f.owner.UserID, KnowledgeBaseID: kbID, Title: "fixture", Type: "text", Source: "fixture", ParseStatus: "completed", EnableStatus: "enabled"}).Error)
	require.NoError(t, f.db.Create(&mysql.ChunkModel{ID: chunkID, TenantID: f.owner.TenantID, KnowledgeBaseID: kbID, KnowledgeID: docID, Content: "Authorized knowledge text", ChunkIndex: 0, IsEnabled: true, ChunkType: "text"}).Error)
	return kbID, docID, chunkID
}

func TestFeatureSourcesHTTPIntegration(t *testing.T) {
	f := newSourceHTTPFixture(t)
	t.Run("web controlled response normalization", func(t *testing.T) {
		in := f.acquire(t, lifecycle.SourceRequest{Kind: "web", URL: "https://source.example/fixture"}, "https://source.example/fixture")
		require.Equal(t, "text", in.Kind)
		require.Contains(t, in.Text, "Revenue 123.45")
		require.NotContains(t, in.Text, "private_script")
	})
	t.Run("web invalid URL has client status", func(t *testing.T) {
		w := f.request(t, http.MethodPost, "/lifecycle/assets", &lifecycle.SourceRequest{Kind: "web", URL: "http://localhost/"}, uuid.NewString(), "")
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})
	t.Run("datasource real SQL", func(t *testing.T) {
		cfg, err := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
		require.NoError(t, err)
		src := lifecycle.SourceRequest{Kind: "datasource", DatasourceID: f.dsID, Table: f.table}
		in := f.acquire(t, src, f.dsID+"/"+cfg.DBName+"/"+f.table)
		require.Equal(t, []string{"region", "amount"}, in.Columns)
		require.Len(t, in.Rows, 1)
		require.Equal(t, []any{"north", "123.45"}, in.Rows[0])
		require.Equal(t, "DECIMAL", in.ColumnTypes["amount"])
		stats, err := lifecycle.AnalyzeInput(in)
		require.NoError(t, err)
		require.Equal(t, 1, stats["columns"].([]map[string]any)[1]["numeric_count"])
		w := f.request(t, http.MethodPost, "/lifecycle/assets", &src, uuid.NewString(), "other")
		require.Contains(t, []int{http.StatusNotFound, http.StatusForbidden}, w.Code, w.Body.String())
	})
	t.Run("knowledge and deleted chunk exclusion", func(t *testing.T) {
		kbID, docID, _ := f.knowledge(t)
		deletedAt := time.Now()
		require.NoError(t, f.db.Create(&mysql.ChunkModel{ID: uuid.NewString(), TenantID: f.owner.TenantID, KnowledgeBaseID: kbID, KnowledgeID: docID, Content: "Deleted secret chunk", ChunkIndex: 1, IsEnabled: true, DeletedAt: &deletedAt}).Error)
		src := lifecycle.SourceRequest{Kind: "knowledge", KnowledgeID: docID}
		in := f.acquire(t, src, docID)
		require.Equal(t, "Authorized knowledge text\n", in.Text)
		require.Equal(t, http.StatusNotFound, f.request(t, http.MethodPost, "/lifecycle/assets", &src, uuid.NewString(), "other").Code)
	})
	for _, field := range []string{"disabled document", "deleted document", "disabled knowledge base", "deleted knowledge base", "deleted only chunk"} {
		t.Run("knowledge rejects "+field, func(t *testing.T) {
			kbID, docID, chunkID := f.knowledge(t)
			switch field {
			case "disabled document":
				require.NoError(t, f.db.Model(&mysql.KnowledgeModel{}).Where("id=?", docID).Update("enable_status", "disabled").Error)
			case "deleted document":
				require.NoError(t, f.db.Model(&mysql.KnowledgeModel{}).Where("id=?", docID).Update("deleted_at", time.Now()).Error)
			case "disabled knowledge base":
				require.NoError(t, f.db.Model(&mysql.KnowledgeBaseModel{}).Where("id=?", kbID).Update("status", 0).Error)
			case "deleted knowledge base":
				require.NoError(t, f.db.Model(&mysql.KnowledgeBaseModel{}).Where("id=?", kbID).Update("deleted_at", time.Now()).Error)
			case "deleted only chunk":
				require.NoError(t, f.db.Model(&mysql.ChunkModel{}).Where("id=?", chunkID).Update("deleted_at", time.Now()).Error)
			}
			w := f.request(t, http.MethodPost, "/lifecycle/assets", &lifecycle.SourceRequest{Kind: "knowledge", KnowledgeID: docID}, uuid.NewString(), "")
			require.Contains(t, []int{http.StatusNotFound, http.StatusBadRequest}, w.Code, w.Body.String())
		})
	}
	t.Run("collection persisted normalized object", func(t *testing.T) {
		taskID, runID, itemID, sourceID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
		body, key := "Collected normalized text\nRevenue 123.45", "source-fixture/"+itemID+".txt"
		_, err := f.objects.Put(context.Background(), objectstore.PutRequest{Key: key, Body: strings.NewReader(body), Size: int64(len(body)), ContentType: "text/plain"})
		require.NoError(t, err)
		require.NoError(t, f.db.Create(&mysql.CollectionTaskModel{ID: taskID, TenantID: f.owner.TenantID, UserID: f.owner.UserID, IdempotencyKey: uuid.NewString(), Status: "completed"}).Error)
		require.NoError(t, f.db.Create(&mysql.CollectionSourceModel{ID: sourceID, TaskID: taskID, URL: "https://source.example/collected"}).Error)
		require.NoError(t, f.db.Create(&mysql.CollectionRunModel{ID: runID, TenantID: f.owner.TenantID, TaskID: taskID, Status: "completed", StartedAt: time.Now()}).Error)
		require.NoError(t, f.db.Create(&mysql.CollectionItemModel{ID: itemID, TenantID: f.owner.TenantID, TaskID: taskID, RunID: runID, SourceID: sourceID, Status: "succeeded", CanonicalURL: "https://source.example/collected", NormalizedObjectKey: key, NormalizedSize: int64(len(body))}).Error)
		src := lifecycle.SourceRequest{Kind: "collection", RunID: runID, ItemID: itemID}
		in := f.acquire(t, src, itemID)
		require.Equal(t, body, in.Text)
		require.Equal(t, http.StatusNotFound, f.request(t, http.MethodPost, "/lifecycle/assets", &src, uuid.NewString(), "other").Code)
	})
	t.Run("result exact values and session owner", func(t *testing.T) {
		sessionID := uuid.NewString()
		require.NoError(t, f.db.Create(&mysql.SessionModel{ID: sessionID, TenantID: f.owner.TenantID, UserID: f.owner.UserID, Title: "source fixture", Status: 1}).Error)
		id, err := f.results.Put(context.Background(), &resultstore.Result{Owner: resultstore.OwnerKey(f.owner.TenantID, sessionID), Columns: []string{"id", "amount", "missing"}, Rows: []map[string]any{{"id": json.Number("9007199254740993"), "amount": json.Number("123.4567890123456789")}}}, time.Minute)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, f.redis.Del(context.Background(), "agent:resultstore:"+id).Err()) })
		src := lifecycle.SourceRequest{Kind: "result", SessionID: sessionID, ResultID: id}
		in := f.acquire(t, src, id)
		require.Equal(t, []string{"id", "amount", "missing"}, in.Columns)
		require.Equal(t, [][]any{{json.Number("9007199254740993"), json.Number("123.4567890123456789"), nil}}, in.Rows)
		for _, principal := range []string{"peer", "other"} {
			require.Equal(t, http.StatusNotFound, f.request(t, http.MethodPost, "/lifecycle/assets", &src, uuid.NewString(), principal).Code)
		}
	})
	t.Run("result missing handle is not internal error", func(t *testing.T) {
		sessionID := uuid.NewString()
		require.NoError(t, f.db.Create(&mysql.SessionModel{ID: sessionID, TenantID: f.owner.TenantID, UserID: f.owner.UserID, Title: "empty source fixture", Status: 1}).Error)
		w := f.request(t, http.MethodPost, "/lifecycle/assets", &lifecycle.SourceRequest{Kind: "result", SessionID: sessionID, ResultID: resultstore.NewResultID()}, uuid.NewString(), "")
		require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	})
	t.Run("evaluation traverses result pages and owns task", func(t *testing.T) {
		taskID := uuid.NewString()
		require.NoError(t, f.db.Create(&mysql.EvaluationTaskModel{ID: taskID, TenantID: f.owner.TenantID, UserID: f.owner.UserID, DatasetID: uuid.NewString(), Type: "agent", Status: "completed", TotalCount: 101, SuccessCount: 101}).Error)
		rows := make([]mysql.EvaluationResultModel, 101)
		for i := range rows {
			rows[i] = mysql.EvaluationResultModel{TaskID: taskID, Question: fmt.Sprintf("question %03d", i), ReferenceAnswer: "reference", GeneratedAnswer: "generated", RetrievedPIDs: "[]", RelevantPIDs: "[]", Success: true, CreatedAt: time.Now()}
		}
		require.NoError(t, f.db.Create(&rows).Error)
		src := lifecycle.SourceRequest{Kind: "evaluation", RunID: taskID}
		in := f.acquire(t, src, taskID)
		require.Equal(t, []string{"question", "reference_answer", "generated_answer", "success"}, in.Columns)
		require.Len(t, in.Rows, 101)
		require.Equal(t, []any{"question 100", "reference", "generated", true}, in.Rows[100])
		for _, principal := range []string{"peer", "other"} {
			require.Equal(t, http.StatusNotFound, f.request(t, http.MethodPost, "/lifecycle/assets", &src, uuid.NewString(), principal).Code)
		}
	})
	t.Run("evaluation refuses oversize instead of truncating", func(t *testing.T) {
		taskID := uuid.NewString()
		require.NoError(t, f.db.Create(&mysql.EvaluationTaskModel{ID: taskID, TenantID: f.owner.TenantID, UserID: f.owner.UserID, DatasetID: uuid.NewString(), Type: "agent", Status: "completed", TotalCount: 10001, SuccessCount: 10001}).Error)
		rows := make([]mysql.EvaluationResultModel, 10001)
		for i := range rows {
			rows[i] = mysql.EvaluationResultModel{TaskID: taskID, Question: fmt.Sprintf("large question %05d", i), ReferenceAnswer: "reference", GeneratedAnswer: "generated", RetrievedPIDs: "[]", RelevantPIDs: "[]", Success: true, CreatedAt: time.Now()}
		}
		require.NoError(t, f.db.CreateInBatches(&rows, 250).Error)
		var before, after int64
		require.NoError(t, f.db.Table("lifecycle_assets").Where("tenant_id=?", f.owner.TenantID).Count(&before).Error)
		w := f.request(t, http.MethodPost, "/lifecycle/assets", &lifecycle.SourceRequest{Kind: "evaluation", RunID: taskID}, uuid.NewString(), "")
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "10000")
		require.NoError(t, f.db.Table("lifecycle_assets").Where("tenant_id=?", f.owner.TenantID).Count(&after).Error)
		require.Equal(t, before, after, "rejected oversize acquisition must not leave a partial snapshot")
	})
}
