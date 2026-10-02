//go:build integration

package account_test

import (
	"cognida/internal/config"
	"cognida/internal/handler"
	"cognida/internal/handler/middleware"
	repo "cognida/internal/repository/mysql"
	"cognida/internal/service/account"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestMembershipHTTPAndRefreshWithRealDatabase(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	require.NotEmpty(t, dsn)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	svc := account.NewAccountService(repo.NewUserRepository(db), repo.NewRefreshTokenRepository(db), &config.JWTConfig{Secret: strings.Repeat("f10-integration-", 3), AccessTokenExpire: 900, RefreshTokenExpire: 3600}, repo.NewTenantRepository(db), repo.NewTransactionManager(db))
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	registered, err := svc.Register(ctx, &account.RegisterRequest{Username: "f10-" + suffix, Email: suffix + "@review.example", Password: "test-password-123"})
	require.NoError(t, err)
	require.Positive(t, registered.User.ID)
	require.Positive(t, registered.TenantID)
	tenantIDs := []int64{registered.TenantID}
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DELETE FROM refresh_tokens WHERE user_id = ?", registered.User.ID).Error)
		require.NoError(t, db.Exec("DELETE FROM tenant_users WHERE user_id = ?", registered.User.ID).Error)
		require.NoError(t, db.Exec("DELETE FROM users WHERE id = ?", registered.User.ID).Error)
		require.NoError(t, db.Exec("DELETE FROM tenants WHERE id IN ?", tenantIDs).Error)
	})
	router := gin.New()
	auth := handler.NewAuthHandler(svc)
	secured := router.Group("/api/v1", middleware.NewAuthMiddleware(svc).Apply())
	secured.GET("/user/tenants", auth.MemberTenants)
	secured.POST("/user/tenants", auth.CreateWorkspace)
	secured.POST("/auth/switch-tenant", auth.SwitchTenant)
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		return w
	}
	created := request(http.MethodPost, "/api/v1/user/tenants", `{"name":"Review workspace","business":"integration","description":"HTTP validation regression"}`, registered.AccessToken)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var workspaceResponse struct {
		Data account.TenantResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &workspaceResponse))
	workspace := workspaceResponse.Data
	require.Positive(t, workspace.ID)
	tenantIDs = append(tenantIDs, workspace.ID)
	listed := request(http.MethodGet, "/api/v1/user/tenants", "", registered.AccessToken)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	var page struct {
		Data struct {
			Items []struct {
				ID int64 `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listed.Body.Bytes(), &page))
	require.Len(t, page.Data.Items, 2)
	payload, _ := json.Marshal(map[string]int64{"tenant_id": workspace.ID})
	switched := request(http.MethodPost, "/api/v1/auth/switch-tenant", string(payload), registered.AccessToken)
	require.Equal(t, http.StatusOK, switched.Code, switched.Body.String())
	var response struct {
		Data account.AuthResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(switched.Body.Bytes(), &response))
	require.Equal(t, workspace.ID, response.Data.TenantID)
	// Two requests racing on one refresh token must not mint two independent sessions.
	type refreshResult struct {
		response *account.AuthResponse
		err      error
	}
	results := make(chan refreshResult, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			res, err := svc.RefreshToken(ctx, &account.RefreshTokenRequest{RefreshToken: response.Data.RefreshToken})
			results <- refreshResult{res, err}
		}()
	}
	close(start)
	var refreshed *account.AuthResponse
	successes := 0
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err == nil {
			successes++
			refreshed = result.response
		}
	}
	require.Equal(t, 1, successes)
	require.NotNil(t, refreshed)
	require.Equal(t, workspace.ID, refreshed.TenantID)
	_, err = svc.RefreshToken(ctx, &account.RefreshTokenRequest{RefreshToken: response.Data.RefreshToken})
	require.Error(t, err)
	denied := request(http.MethodPost, "/api/v1/auth/switch-tenant", `{"tenant_id":9223372036854775806}`, refreshed.AccessToken)
	require.Equal(t, http.StatusForbidden, denied.Code)
	require.NoError(t, db.Exec("UPDATE tenant_users SET status = 'inactive' WHERE user_id = ? AND tenant_id = ?", registered.User.ID, workspace.ID).Error)
	revoked := request(http.MethodGet, "/api/v1/user/tenants", "", refreshed.AccessToken)
	require.Equal(t, http.StatusForbidden, revoked.Code)
	_, err = svc.RefreshToken(ctx, &account.RefreshTokenRequest{RefreshToken: refreshed.RefreshToken})
	require.ErrorIs(t, err, account.ErrTenantAccessDenied)
}
