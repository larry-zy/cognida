//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	storage "cognida/internal/infrastructure/objectstore"
	domain "cognida/internal/model/collection"
	repository "cognida/internal/repository/mysql"
	svc "cognida/internal/service/collection"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type outcomeIntegrationFetch struct{}

func (outcomeIntegrationFetch) Fetch(_ context.Context, req domain.FetchRequest) (*domain.FetchResponse, error) {
	if strings.HasSuffix(req.URL, "/fail") {
		return nil, errors.New("fixture upstream unavailable")
	}
	body := "fixture source " + req.URL
	if strings.HasSuffix(req.URL, "/empty") {
		body = " "
	}
	return &domain.FetchResponse{CanonicalURL: req.URL, ContentType: "text/plain", StatusCode: 200, Body: []byte(body)}, nil
}

type outcomeIntegrationIngest struct {
	db    *gorm.DB
	calls int
}

func (i *outcomeIntegrationIngest) Ingest(ctx context.Context, req domain.IngestRequest) (*domain.IngestResult, error) {
	i.calls++
	if strings.HasSuffix(req.SourceURL, "/index-fail") {
		return nil, errors.New("fixture index unavailable")
	}
	id := uuid.NewString()
	err := i.db.WithContext(ctx).Exec("INSERT INTO knowledge (id, tenant_id, knowledge_base_id, user_id, type, title, source, parse_status) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", id, req.TenantID, req.KnowledgeBaseID, req.UserID, "text", "collection outcome fixture", "collection", "completed").Error
	return &domain.IngestResult{KnowledgeID: id}, err
}

func TestCollectionOutcomeMySQLAPI(t *testing.T) {
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
	for _, tc := range []struct {
		name    string
		paths   []string
		status  domain.RunStatus
		outcome string
		usable  int
		repeat  bool
	}{
		{"all fetch failures", []string{"fail", "fail"}, domain.RunStatusFailed, "failed", 0, false},
		{"all rejected", []string{"empty"}, domain.RunStatusFailed, "failed", 0, false},
		{"legacy ingestor is never called", []string{"index-fail"}, domain.RunStatusCompleted, "success", 1, false},
		{"partial keeps candidates", []string{"success", "fail"}, domain.RunStatusCompleted, "partial", 1, false},
		{"unchanged rerun remains candidate", []string{"success"}, domain.RunStatusCompleted, "success", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tenant := time.Now().UnixNano() & 0x1fffffffffffff
			taskID := "collection-outcome-" + uuid.NewString()
			t.Cleanup(func() {
				require.NoError(t, db.Where("id = ? AND tenant_id = ?", taskID, tenant).Delete(&repository.CollectionTaskModel{}).Error)
				require.NoError(t, db.Exec("DELETE FROM knowledge WHERE tenant_id = ? AND source = ?", tenant, "collection").Error)
			})
			objects, err := storage.NewLocal(t.TempDir())
			require.NoError(t, err)
			// Commit only terminal runs: live background workers must never see or
			// claim this test's intermediate running fixture.
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				repo := repository.NewCollectionRepository(tx)
				ingestor := &outcomeIntegrationIngest{db: tx}
				service := svc.NewService(repo, outcomeIntegrationFetch{}, ingestor, svc.TextNormalizer{})
				service.SetArtifactStore(objects)
				now := time.Now().UTC()
				task := &domain.Task{ID: taskID, TenantID: tenant, UserID: 1, KnowledgeBaseID: "outcome-fixture", IdempotencyKey: taskID, Status: domain.TaskStatusDraft, Limits: domain.Limits{MaxSources: len(tc.paths), MaxResponseBytes: 1024}, CreatedAt: now, UpdatedAt: now}
				sources := []*domain.Source{}
				for _, path := range tc.paths {
					sources = append(sources, &domain.Source{ID: uuid.NewString(), TaskID: taskID, URL: "https://example.com/" + path, CreatedAt: now})
				}
				if err := repo.CreateTask(ctx, task, sources); err != nil {
					return err
				}
				run := &domain.Run{ID: uuid.NewString(), TenantID: tenant, TaskID: taskID, Status: domain.RunStatusRunning, StartedAt: now, CreatedAt: now, UpdatedAt: now}
				if err := repo.CreateRun(ctx, run); err != nil {
					return err
				}
				if err := service.ExecuteRun(ctx, svc.RunJobPayload{RunID: run.ID, TaskID: taskID, TenantID: tenant}); err != nil {
					return err
				}
				if tc.repeat {
					run.ID, run.CreatedAt = uuid.NewString(), now.Add(time.Second)
					if err := repo.CreateRun(ctx, run); err != nil {
						return err
					}
					if err := service.ExecuteRun(ctx, svc.RunJobPayload{RunID: run.ID, TaskID: taskID, TenantID: tenant}); err != nil {
						return err
					}
					require.Zero(t, ingestor.calls, "a rerun cannot bypass candidate review by ingesting knowledge")
				}
				require.Zero(t, ingestor.calls, "collection workers must never call the legacy publishing port")
				return nil
			}))
			// Read back using a fresh repository/HTTP handler after transaction commit.
			repo := repository.NewCollectionRepository(db)
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("tenant_id", tenant); c.Set("user_id", int64(1)); c.Next() })
			h := NewCollectionHandler(nil, repo, nil)
			router.GET("/collection/:id", h.Get)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/collection/"+taskID, nil))
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var result struct {
				Data struct {
					Task    domain.Task
					Run     domain.Run
					Items   []*domain.Item
					Summary domain.OutcomeSummary
				}
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
			require.Equal(t, tc.status, result.Data.Run.Status)
			require.Equal(t, domain.TaskStatus(tc.status), result.Data.Task.Status)
			require.Equal(t, tc.outcome, result.Data.Summary.Outcome)
			require.Equal(t, tc.usable, result.Data.Summary.Usable)
			if tc.outcome != "success" {
				require.Contains(t, result.Data.Task.ErrorMessage, "failed=")
			}
			require.Equal(t, tc.usable, result.Data.Summary.Candidates)
			require.Zero(t, result.Data.Summary.Accepted)
			require.Zero(t, result.Data.Summary.Skipped)
			var documents int64
			require.NoError(t, db.Table("knowledge").Where("tenant_id = ?", tenant).Count(&documents).Error)
			require.Zero(t, documents, "candidate creation and reruns never publish knowledge")
			for _, item := range result.Data.Items {
				require.Empty(t, item.KnowledgeID)
				if item.NormalizedObjectKey != "" {
					r, _, err := objects.Get(ctx, item.NormalizedObjectKey)
					require.NoError(t, err)
					body, err := io.ReadAll(r)
					require.NoError(t, r.Close())
					require.NoError(t, err)
					require.NotEmpty(t, body, "normalized source survives an index or peer-source failure")
				}
			}
		})
	}
}
