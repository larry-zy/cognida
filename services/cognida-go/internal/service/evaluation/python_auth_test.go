package evaluation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPythonCatalogUsesServiceCredential(t *testing.T) {
	t.Setenv("INTERNAL_HTTP_AUTH_TOKEN", "service-secret")
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Header.Get("Authorization") != "Bearer service-secret" {
			t.Error("missing service credential")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"graders":[]}`))
	}))
	defer server.Close()
	_, err := NewPythonEvaluationClient(server.URL).ListGraders(context.Background(), "rag")
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("request was not sent")
	}
}
