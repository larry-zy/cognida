package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFixtureSeedFailsClosedBeforeOpeningResources(t *testing.T) {
	for _, tc := range []struct {
		name, dsn, isolated, api, redis, message string
	}{
		{"ordinary database", "fixture@tcp(127.0.0.1:3306)/cognida", "true", "", "", "isolated cognida_review_"},
		{"missing explicit isolation", "fixture@tcp(127.0.0.1:3306)/cognida_review_guard", "", "", "", "ISOLATED=true"},
		{"remote API", "fixture@tcp(127.0.0.1:3306)/cognida_review_guard", "true", "https://production.example", "", "GO_SERVICE_URL must use loopback"},
		{"remote Redis", "fixture@tcp(127.0.0.1:3306)/cognida_review_guard", "true", "http://127.0.0.1:18588", "production.example:6379", "REDIS_ADDR must use loopback"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MYSQL_DSN", tc.dsn)
			t.Setenv("ISOLATED", tc.isolated)
			t.Setenv("GO_SERVICE_URL", tc.api)
			t.Setenv("REDIS_ADDR", tc.redis)
			for _, cleanup := range []bool{false, true} {
				require.ErrorContains(t, run(t.TempDir(), false, cleanup), tc.message)
			}
		})
	}
}

func TestFixtureRejectsMismatchedAuthenticatedIdentityBeforeWrites(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/user/profile", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":901015972,"tenant_id":42}}`))
	}))
	defer server.Close()
	t.Setenv("MYSQL_DSN", "fixture@tcp(127.0.0.1:3306)/cognida_review_guard")
	t.Setenv("ISOLATED", "true")
	t.Setenv("GO_SERVICE_URL", server.URL)
	t.Setenv("REDIS_ADDR", "127.0.0.1:18589")
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "account.json"), []byte(`{"auth":{"data":{"access_token":"fixture","tenant_id":901015972,"user":{"id":901015972}}}}`), 0600))
	require.ErrorContains(t, run(dir, false, false), "does not match the authenticated profile")
	require.Equal(t, 1, requests)
}

func TestFixtureStatusAcceptsDynamicIdentityAndRejectsForeignManifest(t *testing.T) {
	for _, tc := range []struct {
		name             string
		tenantID, userID int64
		wantError        bool
	}{
		{"matching dynamic owner", 901015972, 901015972, false},
		{"foreign tenant", 1, 901015972, true},
		{"foreign user", 901015972, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "Bearer fixture", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/user/profile":
					_, _ = w.Write([]byte(`{"data":{"id":901015972,"tenant_id":901015972}}`))
				case "/api/v1/evaluation/tasks/owned-task":
					_, _ = w.Write([]byte(`{"data":{"status":"completed"}}`))
				default:
					t.Errorf("unexpected API path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			t.Setenv("MYSQL_DSN", "fixture@tcp(127.0.0.1:3306)/cognida_review_guard")
			t.Setenv("ISOLATED", "true")
			t.Setenv("GO_SERVICE_URL", server.URL)
			t.Setenv("REDIS_ADDR", "127.0.0.1:18589")
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "account.json"), []byte(`{"auth":{"data":{"access_token":"fixture","tenant_id":901015972,"user":{"id":901015972}}}}`), 0600))
			manifest := fixture{TenantID: tc.tenantID, UserID: tc.userID, Schema: "cognida_review_guard", EvaluationTaskID: "owned-task"}
			path := filepath.Join(dir, "source-fixtures.json")
			require.NoError(t, save(path, &manifest))
			err := run(dir, true, false)
			if tc.wantError {
				require.ErrorContains(t, err, "different environment")
				require.Equal(t, 1, requests)
				return
			}
			require.NoError(t, err)
			require.Equal(t, 2, requests)
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(raw, &manifest))
			require.Equal(t, int64(901015972), manifest.TenantID)
			require.Equal(t, int64(901015972), manifest.UserID)
			require.Equal(t, "completed", manifest.Evaluation["status"])
		})
	}
}
