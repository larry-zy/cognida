package embedding

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"cognida/internal/infrastructure/config"
)

func TestOpenAIEmbedder_EmbedStrings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("expected auth header, got %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		var req openAIEmbeddingRequest
		_ = json.Unmarshal(body, &req)
		if req.Model != "text-embedding-3-small" {
			t.Errorf("unexpected model: %s", req.Model)
		}
		// 故意乱序返回，验证按 index 排序
		resp := openAIEmbeddingResponse{
			Data: []openAIEmbeddingData{
				{Index: 1, Embedding: []float64{0.3, 0.4}},
				{Index: 0, Embedding: []float64{0.1, 0.2}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewOpenAIEmbedder(&config.EmbeddingConfig{
		Provider: "openai",
		APIKey:   "sk-test",
		Model:    "text-embedding-3-small",
		BaseURL:  srv.URL,
	})

	vecs, err := e.EmbedStrings(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("EmbedStrings failed: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}
	if vecs[0][0] != 0.1 || vecs[1][0] != 0.3 {
		t.Errorf("vectors not ordered by index: %+v", vecs)
	}
}

func TestOpenAIEmbedder_EmptyInput(t *testing.T) {
	e := NewOpenAIEmbedder(&config.EmbeddingConfig{APIKey: "sk-test"})
	if _, err := e.EmbedStrings(context.Background(), nil); err == nil {
		t.Error("expected error for empty input")
	}
}

func TestOllamaEmbedder_EmbedStrings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req ollamaEmbeddingRequest
		_ = json.Unmarshal(body, &req)
		if len(req.Input) != 2 {
			t.Errorf("expected 2 inputs, got %d", len(req.Input))
		}
		resp := ollamaEmbeddingResponse{
			Model:      req.Model,
			Embeddings: [][]float64{{0.1, 0.2}, {0.3, 0.4}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(&config.EmbeddingConfig{Provider: "ollama", BaseURL: srv.URL, Model: "nomic-embed-text"})
	vecs, err := e.EmbedStrings(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("EmbedStrings failed: %v", err)
	}
	if len(vecs) != 2 || vecs[1][1] != 0.4 {
		t.Errorf("unexpected vectors: %+v", vecs)
	}
}

func TestOllamaEmbedder_Defaults(t *testing.T) {
	e := NewOllamaEmbedder(&config.EmbeddingConfig{})
	if e.baseURL != "http://localhost:11434" {
		t.Errorf("expected default baseURL, got %s", e.baseURL)
	}
	if e.model != "nomic-embed-text" {
		t.Errorf("expected default model, got %s", e.model)
	}
}

func TestNewEmbedder_Routing(t *testing.T) {
	// openai 需要 APIKey
	if _, err := NewEmbedder(&config.EmbeddingConfig{Provider: "openai", APIKey: "k"}); err != nil {
		t.Errorf("openai routing failed: %v", err)
	}
	// ollama 无需 APIKey
	if _, err := NewEmbedder(&config.EmbeddingConfig{Provider: "ollama"}); err != nil {
		t.Errorf("ollama routing failed: %v", err)
	}
}
