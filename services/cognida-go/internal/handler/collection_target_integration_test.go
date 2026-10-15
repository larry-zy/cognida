//go:build integration

package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	domain "cognida/internal/model/collection"
	kbmodel "cognida/internal/model/knowledge"
	repository "cognida/internal/repository/mysql"
	collectionsvc "cognida/internal/service/collection"
	knowledge "cognida/internal/service/knowledge"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCollectionInactiveKnowledgeBaseMySQLAPI(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	cfg, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"), "isolated review database required")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	ctx := context.Background()
	for _, mode := range []string{"disabled", "deleted", "document_disabled"} {
		t.Run(mode, func(t *testing.T) {
			tenant, kbID := time.Now().UnixNano()&0x1fffffffffffff, uuid.NewString()
			t.Cleanup(func() {
				require.NoError(t, db.Where("tenant_id = ? AND knowledge_base_id = ?", tenant, kbID).Delete(&repository.CollectionTaskModel{}).Error)
				require.NoError(t, db.Exec("DELETE FROM knowledge WHERE tenant_id = ? AND knowledge_base_id = ?", tenant, kbID).Error)
				require.NoError(t, db.Where("id = ? AND tenant_id = ?", kbID, tenant).Delete(&repository.KnowledgeBaseModel{}).Error)
			})
			require.NoError(t, db.Create(&repository.KnowledgeBaseModel{ID: kbID, TenantID: tenant, UserID: 1, Name: "collection target fixture", Status: 1}).Error)
			kbRepo := repository.NewKnowledgeBaseRepository(db, true)
			kbService := knowledge.NewKnowledgeBaseService(kbRepo, nil, nil, nil, nil, nil, nil)
			processor := knowledge.NewDocumentProcessorService(kbRepo, nil, repository.NewKnowledgeRepository(db), nil, nil, nil, nil, nil, nil, nil)
			repo := repository.NewCollectionRepository(db)
			service := collectionsvc.NewService(repo, outcomeIntegrationFetch{}, collectionsvc.NewKnowledgeIngestorAdapter(processor), collectionsvc.TextNormalizer{})
			h := NewCollectionHandler(service, repo, kbService)
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("tenant_id", tenant); c.Set("user_id", int64(1)); c.Next() })
			router.POST("/collection", h.Create)
			router.GET("/collection/:id", h.Get)
			create := func() *httptest.ResponseRecorder {
				body := fmt.Sprintf(`{"knowledge_base_id":%q,"sources":["https://example.com/success"]}`, kbID)
				req := httptest.NewRequest(http.MethodPost, "/collection", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", uuid.NewString())
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				return w
			}
			created := create()
			require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
			var taskResponse struct{ Data domain.Task }
			require.NoError(t, json.Unmarshal(created.Body.Bytes(), &taskResponse))
			// A completed ingestion receipt exists while the target is active.
			content := []byte("fixture source https://example.com/success")
			hash := sha256.Sum256(content)
			key := hex.EncodeToString(hash[:]) // same stable key as the production adapter
			digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s", tenant, kbID, key)))
			docID := uuid.NewSHA1(uuid.NameSpaceURL, digest[:]).String()
			require.NoError(t, db.Exec("INSERT INTO knowledge (id, tenant_id, knowledge_base_id, user_id, type, title, source, parse_status, file_hash) VALUES (?, ?, ?, 1, 'text', 'target fixture', 'collection', 'completed', ?)", docID, tenant, kbID, hex.EncodeToString(hash[:])).Error)
			input := &knowledge.ProcessDocumentRequest{KnowledgeBaseID: kbID, Title: "target fixture", Content: content, IdempotencyKey: key}
			result, err := processor.ProcessDocument(ctx, tenant, 1, input)
			require.NoError(t, err)
			require.Equal(t, docID, result.KnowledgeID)
			// Seed a previous accepted run for the identical source. A future run
			// must validate the target before reusing this deduplication receipt.
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				repo := repository.NewCollectionRepository(tx)
				sources, err := repo.ListSources(ctx, tenant, taskResponse.Data.ID)
				if err != nil {
					return err
				}
				now := time.Now().UTC().Add(-time.Second)
				run := &domain.Run{ID: uuid.NewString(), TaskID: taskResponse.Data.ID, TenantID: tenant, Status: domain.RunStatusRunning, StartedAt: now, CreatedAt: now, UpdatedAt: now}
				if err := repo.CreateRun(ctx, run); err != nil {
					return err
				}
				if err := repo.CreateItem(ctx, &domain.Item{ID: uuid.NewString(), TenantID: tenant, TaskID: run.TaskID, RunID: run.ID, SourceID: sources[0].ID, SourceURL: sources[0].URL, Status: domain.ItemStatusAccepted, KnowledgeID: docID, ContentHash: hex.EncodeToString(hash[:]), CreatedAt: now, UpdatedAt: now}); err != nil {
					return err
				}
				run.Status, run.CompletedAt = domain.RunStatusCompleted, &now
				return repo.FinishRun(ctx, run)
			}))
			updates := map[string]any{"status": 0}
			if mode == "deleted" {
				updates = map[string]any{"deleted_at": time.Now().UTC()}
			}
			expectedErr := kbmodel.ErrKnowledgeBaseNotFound
			replayKeys := []string{key, ""}
			if mode == "document_disabled" {
				require.NoError(t, db.Exec("UPDATE knowledge SET enable_status = 'disabled' WHERE id = ? AND tenant_id = ?", docID, tenant).Error)
				expectedErr, replayKeys = kbmodel.ErrKnowledgeNotFound, []string{key}
			} else {
				require.NoError(t, db.Model(&repository.KnowledgeBaseModel{}).Where("id = ? AND tenant_id = ?", kbID, tenant).Updates(updates).Error)
				require.Equal(t, http.StatusNotFound, create().Code, "creation must reject an unavailable target")
			}
			_, err = repo.FindAcceptedContent(ctx, tenant, taskResponse.Data.ID, key)
			if mode == "document_disabled" {
				require.ErrorIs(t, err, domain.ErrItemNotFound, "disabled receipts cannot deduplicate a new run")
			}
			for _, replayKey := range replayKeys {
				input.IdempotencyKey = replayKey
				result, err := processor.ProcessDocument(ctx, tenant, 1, input)
				require.ErrorIs(t, err, expectedErr)
				require.Nil(t, result, "neither new processing nor replay may bypass target state")
			}
			// Execute the previously valid draft after target removal. The real
			// processor rejects before parser/embedder invocation; only failed state commits.
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				repo := repository.NewCollectionRepository(tx)
				now := time.Now().UTC()
				run := &domain.Run{ID: uuid.NewString(), TaskID: taskResponse.Data.ID, TenantID: tenant, Status: domain.RunStatusRunning, StartedAt: now, CreatedAt: now, UpdatedAt: now}
				if err := repo.CreateRun(ctx, run); err != nil {
					return err
				}
				worker := collectionsvc.NewService(repo, outcomeIntegrationFetch{}, collectionsvc.NewKnowledgeIngestorAdapter(processor), collectionsvc.TextNormalizer{})
				return worker.ExecuteRun(ctx, collectionsvc.RunJobPayload{RunID: run.ID, TaskID: run.TaskID, TenantID: tenant})
			}))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/collection/"+taskResponse.Data.ID, nil))
			require.Equal(t, http.StatusOK, w.Code)
			var detail struct {
				Data struct {
					Task    domain.Task
					Run     domain.Run
					Items   []*domain.Item
					Summary domain.OutcomeSummary
				}
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detail))
			require.Equal(t, domain.TaskStatusFailed, detail.Data.Task.Status)
			require.Equal(t, domain.RunStatusFailed, detail.Data.Run.Status)
			require.Equal(t, 1, detail.Data.Summary.Failed)
			require.Empty(t, detail.Data.Items[0].KnowledgeID)
			var documents, tasks int64
			require.NoError(t, db.Table("knowledge").Where("tenant_id = ? AND knowledge_base_id = ?", tenant, kbID).Count(&documents).Error)
			require.NoError(t, db.Model(&repository.CollectionTaskModel{}).Where("tenant_id = ?", tenant).Count(&tasks).Error)
			require.EqualValues(t, 1, documents, "the pre-existing receipt is retained and no document is added")
			require.EqualValues(t, 1, tasks, "invalid creation writes no task")
			if mode == "document_disabled" {
				var enabled string
				require.NoError(t, db.Raw("SELECT enable_status FROM knowledge WHERE id = ? AND tenant_id = ?", docID, tenant).Scan(&enabled).Error)
				require.Equal(t, "disabled", enabled, "collection must not re-enable the prior document")
			}
		})
	}
}
