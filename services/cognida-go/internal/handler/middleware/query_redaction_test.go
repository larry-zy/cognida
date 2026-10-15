package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRedactSensitiveQuery(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      url.Values
	}{
		{"ordinary parameters", "cursor=abc%2B123&page_size=20&status=completed", url.Values{"cursor": {"abc+123"}, "page_size": {"20"}, "status": {"completed"}}},
		{"case encoded and duplicate keys", "to%6Ben=fixture-one&token=fixture-two&ACCESS_TOKEN=fixture-three&cursor=2", url.Values{"token": {redactedQueryValue, redactedQueryValue}, "ACCESS_TOKEN": {redactedQueryValue}, "cursor": {"2"}}},
		{"credential aliases", "refresh-token=x&id_token=x&api_key=x&password=x&client_secret=x&authorization=x&signature=x", url.Values{"refresh-token": {redactedQueryValue}, "id_token": {redactedQueryValue}, "api_key": {redactedQueryValue}, "password": {redactedQueryValue}, "client_secret": {redactedQueryValue}, "authorization": {redactedQueryValue}, "signature": {redactedQueryValue}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := url.ParseQuery(redactSensitiveQuery(tc.raw))
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
	for _, malformed := range []string{"token=fixture-secret;cursor=1", "to%zzken=fixture-secret", "cursor=%zz&token=fixture-secret"} {
		require.Equal(t, redactedQueryValue, redactSensitiveQuery(malformed))
	}
	require.Empty(t, redactSensitiveQuery(""))
}

func TestQueryCredentialsRedactedAtHTTPLogAndAuditBoundary(t *testing.T) {
	var output bytes.Buffer
	previous := gin.DefaultWriter
	gin.DefaultWriter = &output
	t.Cleanup(func() { gin.DefaultWriter = previous })
	router := gin.New()
	router.Use(Logger()) // The same entry point registered by the production router.
	router.GET("/api/v1/lifecycle/runs/:id/events", func(c *gin.Context) {
		// Redaction must not mutate the live authentication input.
		require.Equal(t, "fixture-sse-credential", c.Query("token"))
		require.Equal(t, "41", c.Query("cursor"))
		entry := buildAuditLog(c, c.Request.URL.Path, 1)
		var details map[string]any
		require.NoError(t, json.Unmarshal([]byte(entry.Details), &details))
		query, err := url.ParseQuery(details["query"].(string))
		require.NoError(t, err)
		require.Equal(t, redactedQueryValue, query.Get("token"))
		require.Equal(t, "41", query.Get("cursor"))
		require.NotContains(t, entry.Details, "fixture-sse-credential")
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/lifecycle/runs/test/events?token=fixture-sse-credential&cursor=41", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, output.String(), "fixture-sse-credential")
	require.Contains(t, output.String(), "cursor=41")
	require.Contains(t, output.String(), "token=%5BREDACTED%5D")
}
