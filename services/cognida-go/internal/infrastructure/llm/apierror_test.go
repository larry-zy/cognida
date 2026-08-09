package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainllm "cognida/internal/model/llm"
)

func newOpenAITestClient(t *testing.T, url string) domainllm.LLMClient {
	t.Helper()
	c, err := NewOpenAIChatRepo(&domainllm.ModelConfig{
		Provider:  domainllm.ProviderOpenAI,
		APIKey:    "sk-test-key-123456",
		BaseURL:   url,
		ModelName: "gpt-4",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return c
}

func TestOpenAIChat_RateLimitedAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	client := newOpenAITestClient(t, srv.URL)
	_, err := client.Chat(context.Background(), &domainllm.ChatRequest{})
	ae, ok := domainllm.AsAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if ae.Class != domainllm.ClassRateLimited {
		t.Errorf("class=%s want rate_limited", ae.Class)
	}
	if ae.StatusCode != 429 {
		t.Errorf("status=%d want 429", ae.StatusCode)
	}
	if ae.RetryAfter != 7*time.Second {
		t.Errorf("retryAfter=%v want 7s", ae.RetryAfter)
	}
	if ae.Provider != domainllm.ProviderOpenAI || ae.Model != "gpt-4" {
		t.Errorf("target mismatch: %s/%s", ae.Provider, ae.Model)
	}
}

func TestOpenAIChat_TerminalAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	client := newOpenAITestClient(t, srv.URL)
	_, err := client.Chat(context.Background(), &domainllm.ChatRequest{})
	ae, ok := domainllm.AsAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if ae.Class != domainllm.ClassTerminal {
		t.Errorf("class=%s want terminal", ae.Class)
	}
	if ae.Retryable() {
		t.Error("400 must not be retryable")
	}
}

func TestOpenAIChat_ServerErrorTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newOpenAITestClient(t, srv.URL)
	_, err := client.Chat(context.Background(), &domainllm.ChatRequest{})
	ae, ok := domainllm.AsAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if ae.Class != domainllm.ClassTransient || !ae.Retryable() {
		t.Errorf("500 should be transient+retryable, got %s", ae.Class)
	}
}

func TestOpenAIChat_TransportErrorTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // 立即关闭 → 连接被拒

	client := newOpenAITestClient(t, url)
	_, err := client.Chat(context.Background(), &domainllm.ChatRequest{})
	ae, ok := domainllm.AsAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if ae.Class != domainllm.ClassTransient {
		t.Errorf("connection refused should be transient, got %s", ae.Class)
	}
}

func TestOpenAIChat_CtxCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := newOpenAITestClient(t, srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	_, err := client.Chat(ctx, &domainllm.ChatRequest{})
	ae, ok := domainllm.AsAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if ae.Class != domainllm.ClassCanceled {
		t.Errorf("canceled ctx should classify canceled, got %s", ae.Class)
	}
}
