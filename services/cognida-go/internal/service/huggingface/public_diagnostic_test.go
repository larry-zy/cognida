package huggingface

import (
	d "cognida/internal/model/huggingface"
	l "cognida/internal/model/lifecycle"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"net/http/httptrace"
	"os"
	"sync"
	"testing"
	"time"

	"cognida/internal/service/collection/fetcher"
)

// An opt-in, read-only network diagnostic. It never imports assets, sends a
// credential or logs request/response bodies, resolved addresses or headers.
func TestHuggingFacePublicReadDiagnostic(t *testing.T) {
	if os.Getenv("COGNIDA_HF_PUBLIC_READ_DIAGNOSTIC") != "1" {
		t.Skip("explicit public metadata diagnostic only")
	}
	for _, item := range []struct{ phase, endpoint string }{
		{"metadata", "https://huggingface.co/api/datasets/mteb/banking77/revision/18072d2685ea682290f7b8924d94c62acc19c0b2?blobs=true"},
		{"card", "https://huggingface.co/datasets/mteb/banking77/resolve/18072d2685ea682290f7b8924d94c62acc19c0b2/README.md"},
	} {
		started := time.Now()
		var mu sync.Mutex
		events := []map[string]any{}
		add := func(event string, failed bool) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, map[string]any{"event": event, "elapsed_ms": time.Since(started).Milliseconds(), "failed": failed})
		}
		trace := &httptrace.ClientTrace{
			DNSStart:             func(httptrace.DNSStartInfo) { add("dns_start", false) },
			DNSDone:              func(x httptrace.DNSDoneInfo) { add("dns_done", x.Err != nil) },
			ConnectStart:         func(string, string) { add("tcp_start", false) },
			ConnectDone:          func(_, _ string, err error) { add("tcp_done", err != nil) },
			TLSHandshakeStart:    func() { add("tls_start", false) },
			TLSHandshakeDone:     func(_ tls.ConnectionState, err error) { add("tls_done", err != nil) },
			GotConn:              func(httptrace.GotConnInfo) { add("connection_ready", false) },
			WroteRequest:         func(x httptrace.WroteRequestInfo) { add("request_written", x.Err != nil) },
			GotFirstResponseByte: func() { add("first_response_byte", false) },
		}
		ctx, cancel := context.WithTimeout(httptrace.WithClientTrace(context.Background(), trace), 30*time.Second)
		result, err := fetcher.New(fetcher.Options{}).FetchHuggingFace(ctx, item.endpoint, fetcher.JSONCredential{}, 2<<20)
		cancel()
		code, status, bytes := "", 0, 0
		if err != nil {
			code = "upstream_error"
			if errors.Is(err, context.DeadlineExceeded) {
				code = "deadline_exceeded"
			}
		} else {
			status, bytes = result.StatusCode, len(result.Body)
		}
		mu.Lock()
		body, _ := json.Marshal(map[string]any{"phase": item.phase, "elapsed_ms": time.Since(started).Milliseconds(), "status": status, "bytes": bytes, "error_code": code, "events": events})
		mu.Unlock()
		t.Log(string(body))
		if err != nil {
			t.Errorf("public %s failed: %s", item.phase, code)
		}
	}
}

// Executes one bounded public preview using the production fetcher and the
// operator's running authenticated Python decoder. No repository/assets are
// attached, so this diagnostic cannot persist an import or create an asset.
func TestHuggingFacePublicBoundedPreviewDiagnostic(t *testing.T) {
	if os.Getenv("COGNIDA_HF_PUBLIC_PREVIEW_DIAGNOSTIC") != "1" {
		t.Skip("explicit read-only public preview diagnostic")
	}
	f, err := fetcher.NewHuggingFaceOperatorFetcher(fetcher.HuggingFaceEgressConfig{SOCKS5Address: os.Getenv("HUGGINGFACE_SOCKS5_ADDRESS")})
	require.NoError(t, err)
	require.NotEmpty(t, os.Getenv("HUGGINGFACE_SOCKS5_ADDRESS"))
	decoder, err := NewHTTPDecoder(os.Getenv("PYTHON_EVALUATION_ENDPOINT"), os.Getenv("INTERNAL_HTTP_AUTH_TOKEN"))
	require.NoError(t, err)
	s := &Service{fetch: f, decoder: decoder, authorize: func(context.Context, l.Principal, string) error { return nil }}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	started := time.Now()
	out, err := s.Preview(ctx, l.Principal{TenantID: 1, UserID: 1}, d.Request{RepoID: "mteb/banking77", Revision: "18072d2685ea682290f7b8924d94c62acc19c0b2", Config: "default", Split: "train", Files: []string{"data/train-00000-of-00001.parquet"}, FieldMapping: d.FieldMapping{TextField: "text"}, MaxRows: 3, MaxBytes: 16 << 20})
	require.NoError(t, err)
	require.Equal(t, []string{"source_id", "text"}, out.Columns)
	require.Len(t, out.Rows, 3)
	require.Equal(t, 3, out.Manifest.Rows)
	require.Contains(t, out.Manifest.ExcludedColumns, "label")
	require.Contains(t, out.Manifest.ExcludedColumns, "label_text")
	require.True(t, out.Manifest.Sampled)
	require.True(t, out.Manifest.Truncated)
	require.Equal(t, int64(294377), out.Manifest.Bytes)
	require.Len(t, out.Manifest.Files, 1)
	t.Logf("public fixed parquet preview verified: rows=%d bytes=%d sha256=%s sampled=%t truncated=%t excluded_columns=%d elapsed_ms=%d credential=false import=false", out.Manifest.Rows, out.Manifest.Bytes, out.Manifest.Files[0].SHA256, out.Manifest.Sampled, out.Manifest.Truncated, len(out.Manifest.ExcludedColumns), time.Since(started).Milliseconds())
}
