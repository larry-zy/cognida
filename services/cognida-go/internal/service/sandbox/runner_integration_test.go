//go:build integration

package sandbox_test

import (
	"bytes"
	"cognida/internal/handler"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/sandbox"
	repo "cognida/internal/repository/mysql"
	svc "cognida/internal/service/sandbox"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/gin-gonic/gin"
	mysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSandboxMySQLHTTPRecoveryIdempotencyAndCancellation(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	cfg, e := mysql.ParseDSN(dsn)
	require.NoError(t, e)
	if cfg.DBName != "cognida_review_sandbox_20261016" {
		t.Skip("dedicated sandbox review schema required")
	}
	image := os.Getenv("COGNIDA_TEST_SANDBOX_IMAGE")
	require.NotEmpty(t, image)
	db, e := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sqlDB, e := db.DB()
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	p := l.Principal{TenantID: time.Now().UnixNano()%1e12 + 100, UserID: 11}
	t.Cleanup(func() { require.NoError(t, db.Where("tenant_id=?", p.TenantID).Delete(&d.Record{}).Error) })
	ctx := context.Background()
	host := &svc.DockerHost{Root: t.TempDir()}
	storage := repo.NewSandboxRepository(db)
	allowed := true
	body, e := json.Marshal(l.Input{Kind: "text", Text: "import json;json.dump({'kind':'text','text':'actual sandbox output'},open('/output/result.json','w'))"})
	require.NoError(t, e)
	sum := sha256.Sum256(body)
	prof := profile()
	prof.Image = image
	prof.MaxResources.TimeoutSeconds = 20
	prof.AllowedMediaTypes = []string{"application/json", "text/plain"}
	in := request()
	in.IdempotencyKey = uuid.NewString()
	in.Inputs = nil
	in.Resources = prof.MaxResources
	in.ImageSHA256 = image[strings.LastIndex(image, "@")+1:]
	in.Code.SizeBytes = int64(len(body))
	in.Code.SHA256 = "sha256:" + hex.EncodeToString(sum[:])
	auth := func(_ context.Context, who l.Principal) error {
		if who != p {
			return d.ErrForbidden
		}
		return nil
	}
	loader := func(_ context.Context, who l.Principal, id l.RefID) ([]byte, error) {
		if who != p || id != in.Code.Ref || !allowed {
			return nil, d.ErrForbidden
		}
		return body, nil
	}
	validator, e := svc.NewValidator([]d.Profile{prof}, func(ctx context.Context, who l.Principal, id l.RefID) (d.InputMetadata, error) {
		if _, e := loader(ctx, who, id); e != nil {
			return d.InputMetadata{}, e
		}
		return d.InputMetadata{TenantID: p.TenantID, Ref: id, SHA256: in.Code.SHA256, SizeBytes: int64(len(body))}, nil
	})
	require.NoError(t, e)
	output := l.RefID{AssetID: uuid.NewString(), Version: 1}
	writes := 0
	writer := func(_ context.Context, _ l.Principal, _ string, _ []l.RefID, b []byte) (l.RefID, error) {
		require.Contains(t, string(b), "actual sandbox output")
		writes++
		return output, nil
	}
	runner, e := svc.NewDockerRunner(storage, validator, host, loader, writer, auth)
	require.NoError(t, e)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", p.TenantID); c.Set("user_id", p.UserID); c.Next() })
	handler.NewSandboxHandler(runner, &prof).Register(r.Group("/api"))
	post := func(input d.SubmitRequest) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(input)
		req := httptest.NewRequest("POST", "/api/sandbox/runs", bytes.NewReader(raw))
		req.Header.Set("Idempotency-Key", input.IdempotencyKey)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	response := post(in)
	require.Equal(t, 200, response.Code, response.Body.String())
	var envelope struct {
		Data d.Execution `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	execution := envelope.Data
	t.Cleanup(func() { require.NoError(t, host.Remove(ctx, execution.ID)) })
	response = post(in)
	require.Equal(t, 200, response.Code)
	var count int64
	require.NoError(t, db.Model(&d.Record{}).Where("tenant_id=?", p.TenantID).Count(&count).Error)
	require.EqualValues(t, 1, count)
	changed := in
	changed.Resources.TimeoutSeconds--
	require.Equal(t, 409, post(changed).Code)
	worked, e := runner.Tick(ctx)
	require.NoError(t, e)
	require.True(t, worked)
	// Recreate the entire controller after actual launch; the persisted phase
	// prevents executing the same code again on the new controller.
	runner, e = svc.NewDockerRunner(storage, validator, host, loader, writer, auth)
	require.NoError(t, e)
	require.Eventually(t, func() bool {
		_, e := runner.Tick(ctx)
		require.NoError(t, e)
		row, e := runner.Get(ctx, p, execution.ID)
		require.NoError(t, e)
		if d.Terminal(row.State) {
			require.Equal(t, d.StateSucceeded, row.State, row.ErrorCode)
			return true
		}
		return false
	}, 20*time.Second, 150*time.Millisecond)
	require.Equal(t, 1, writes)
	m, e := runner.Collect(ctx, p, execution.ExecutionRef)
	require.NoError(t, e)
	require.Len(t, m.Artifacts, 1)
	allowed = false
	_, e = runner.Get(ctx, p, execution.ID)
	require.ErrorIs(t, e, d.ErrForbidden)
	allowed = true
	pending := in
	pending.IdempotencyKey = uuid.NewString()
	cancelled, e := runner.Submit(ctx, p, pending)
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, host.Remove(ctx, cancelled.ID)) })
	_, e = runner.Cancel(ctx, p, cancelled.ExecutionRef)
	require.NoError(t, e)
	_, e = runner.Tick(ctx)
	require.NoError(t, e)
	row, e := runner.Get(ctx, p, cancelled.ID)
	require.NoError(t, e)
	require.Equal(t, d.StateCancelled, row.State)
	require.Empty(t, row.OutputRefs)
	require.Equal(t, 1, writes)
	// Host preparation may finish after the deadline without cancelling the
	// context. Database time must fence launch intent as well as publication.
	expired := in
	expired.IdempotencyKey = uuid.NewString()
	late, e := runner.Submit(ctx, p, expired)
	require.NoError(t, e)
	require.NoError(t, db.Model(&d.Record{}).Where("id=?", late.ID).Update("deadline", time.Now().Add(-time.Second)).Error)
	claim, e := storage.Claim(ctx, time.Minute)
	require.NoError(t, e)
	require.NotNil(t, claim)
	require.Equal(t, late.ID, claim.ID)
	claim.Phase = "launch_committed"
	claim.State = d.StateRunning
	require.ErrorIs(t, storage.Save(ctx, claim), d.ErrConflict)
	stored, e := storage.Get(ctx, p, late.ID)
	require.NoError(t, e)
	require.Empty(t, stored.Phase)
	require.Empty(t, stored.OutputRefs)
	_, e = runner.Tick(ctx)
	require.NoError(t, e)
	stored, e = storage.Get(ctx, p, late.ID)
	require.NoError(t, e)
	require.Equal(t, d.StateTimedOut, stored.State)
}
