//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	m "cognida/internal/model/datasource"
	"cognida/internal/model/user"
	repo "cognida/internal/repository/mysql"
	"cognida/internal/service/account"
	ds "cognida/internal/service/datasource"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type sourceTestIDs struct{}

func (sourceTestIDs) Generate() string { return uuid.NewString() }

// Authentication identities are injected; current roles, credential storage,
// source mutations and the successful connection test use real migrated MySQL.
func TestDataSourceManagementMySQLHTTP(t *testing.T) {
	f := newReviewedFixture(t)
	ctx := context.Background()
	tid := f.p.TenantID
	require.NoError(t, f.db.Table("tenants").Create(map[string]any{"id": tid, "name": "source-management-fixture", "api_key": uuid.NewString(), "business": "test", "status": "active"}).Error)
	users := repo.NewUserRepository(f.db)
	memberships := users.(user.MembershipRepository)
	ids := map[string]int64{}
	t.Cleanup(func() {
		require.NoError(t, f.db.Table("data_sources").Where("tenant_id=?", tid).Delete(map[string]any{}).Error)
		require.NoError(t, f.db.Table("tenant_users").Where("tenant_id=?", tid).Delete(map[string]any{}).Error)
		require.NoError(t, f.db.Table("users").Where("tenant_id=?", tid).Delete(map[string]any{}).Error)
		require.NoError(t, f.db.Table("tenants").Where("id=?", tid).Delete(map[string]any{}).Error)
	})
	for _, role := range []string{"owner", "admin", "member"} {
		u := &user.User{TenantID: tid, Username: role, Email: role + "@fixture.invalid", PasswordHash: "unused-fixture", Status: 1}
		require.NoError(t, users.Create(ctx, u))
		ids[role] = u.ID
		require.NoError(t, memberships.AddMembership(ctx, u.ID, tid, role))
	}
	cipher, err := ds.NewCipher("synthetic-source-management-key")
	require.NoError(t, err)
	sources := repo.NewDataSourceRepository(f.db)
	manager := ds.NewConnectionManager(sources, cipher, ds.DefaultPoolOptions())
	t.Cleanup(manager.Close)
	service := ds.NewService(sources, cipher, manager, sourceTestIDs{})
	authorizer := account.NewAccountService(users, nil, nil, repo.NewTenantRepository(f.db), nil)
	h := NewDataSourceHandler(service, authorizer)
	var acting int64
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", acting); c.Set("tenant_id", tid); c.Next() })
	router.POST("/datasources", h.Create)
	router.PUT("/datasources/:id", h.Update)
	router.DELETE("/datasources/:id", h.Delete)
	router.POST("/datasources/test", h.TestConnection)
	router.GET("/datasources", h.List)
	request := func(role, method, path string, body any) (int, json.RawMessage) {
		acting = ids[role]
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var response struct {
			Data json.RawMessage `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		return w.Code, response.Data
	}
	cfg, err := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	if err != nil {
		t.Fatal("invalid isolated MySQL configuration")
	}
	host, portstr, err := net.SplitHostPort(cfg.Addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portstr)
	require.NoError(t, err)
	in := ds.SaveInput{Name: "management-fixture", Type: "mysql", Host: host, Port: port, DatabaseName: cfg.DBName, Username: cfg.User, Password: cfg.Passwd}
	code, raw := request("owner", "POST", "/datasources", in)
	require.Equal(t, 200, code)
	var created ds.Sanitized
	require.NoError(t, json.Unmarshal(raw, &created))
	require.NotEmpty(t, created.ID)
	stored, err := sources.Get(ctx, created.ID, tid)
	require.NoError(t, err)
	for _, action := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/datasources", in}, {"PUT", "/datasources/" + created.ID, in}, {"DELETE", "/datasources/" + created.ID, nil}, {"POST", "/datasources/test", ds.TestInput{DatasourceID: created.ID}},
	} {
		code, _ = request("member", action.method, action.path, action.body)
		require.Equal(t, 403, code)
	}
	code, _ = request("member", "GET", "/datasources", nil)
	require.Equal(t, 200, code, "member keeps read-only source listing")
	changed := in
	changed.Password = ""
	changed.Host = "attacker.invalid"
	code, _ = request("owner", "POST", "/datasources/test", ds.TestInput{DatasourceID: created.ID, SaveInput: changed})
	require.Equal(t, 400, code)
	code, _ = request("admin", "PUT", "/datasources/"+created.ID, changed)
	require.Equal(t, 400, code)
	after, err := sources.Get(ctx, created.ID, tid)
	require.NoError(t, err)
	require.Equal(t, stored.PasswordEncrypted, after.PasswordEncrypted)
	require.Equal(t, stored.Host, after.Host)
	code, raw = request("admin", "POST", "/datasources/test", ds.TestInput{DatasourceID: created.ID})
	require.Equal(t, 200, code)
	var result struct {
		OK bool `json:"ok"`
	}
	require.NoError(t, json.Unmarshal(raw, &result))
	require.True(t, result.OK, "approved stored-target test must connect to the actual isolated database")
	changed = in
	changed.Password = ""
	changed.Name = "renamed"
	changed.Description = "display only"
	code, _ = request("admin", "PUT", "/datasources/"+created.ID, changed)
	require.Equal(t, 200, code)
	after, err = sources.Get(ctx, created.ID, tid)
	require.NoError(t, err)
	require.Equal(t, m.QueryConnectionVersion(stored), m.QueryConnectionVersion(after))
	require.NoError(t, f.db.Table("tenant_users").Where("tenant_id=? AND user_id=?", tid, ids["admin"]).Update("role", "member").Error)
	code, _ = request("admin", "POST", "/datasources/test", ds.TestInput{DatasourceID: created.ID})
	require.Equal(t, 403, code, "revoked management is checked on every request")
	code, _ = request("owner", "DELETE", "/datasources/"+created.ID, nil)
	require.Equal(t, 200, code)
}
