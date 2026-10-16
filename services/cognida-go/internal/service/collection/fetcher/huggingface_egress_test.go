package fetcher

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHFOperatorEgressConfigurationAndNumericOnly(t *testing.T) {
	for _, addr := range []string{"localhost:7897", "192.168.1.1:7897", "8.8.8.8:7897", "http://127.0.0.1:7897", "user:pass@127.0.0.1:7897", "127.0.0.1:0", "127.0.0.1:65536", "127.0.0.1:07897", "127.0.0.1:7897 "} {
		_, err := NewHuggingFaceOperatorFetcher(HuggingFaceEgressConfig{SOCKS5Address: addr})
		require.ErrorIs(t, err, ErrHuggingFaceEgress)
	}
	for _, addr := range []string{"127.0.0.1:7897", "[::1]:7897"} {
		_, err := NewHuggingFaceOperatorFetcher(HuggingFaceEgressConfig{SOCKS5Address: addr})
		require.NoError(t, err)
	}
	direct, err := NewHuggingFaceOperatorFetcher(HuggingFaceEgressConfig{})
	require.NoError(t, err)
	require.Same(t, net.DefaultResolver, direct.resolver)
	require.Nil(t, direct.client.Transport.(*http.Transport).Proxy)
	dial, err := newHFNumericSOCKSDial("127.0.0.1:1")
	require.NoError(t, err)
	for _, target := range []string{"huggingface.co:443", "127.0.0.1:443", "10.0.0.1:443", "169.254.169.254:443", "93.184.216.34:80", "[::1]:443"} {
		_, err = dial(context.Background(), "tcp", target)
		require.ErrorIs(t, err, ErrUnsafeDestination)
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	_, err = dial(cancelled, "tcp", "93.184.216.34:443")
	require.ErrorIs(t, err, context.Canceled)
}

func hfAnswer(kind int, values ...string) []byte {
	items := make([]map[string]any, 0, len(values))
	for _, v := range values {
		items = append(items, map[string]any{"name": "huggingface.co.", "type": kind, "data": v, "TTL": 300})
	}
	b, _ := json.Marshal(map[string]any{"Status": 0, "TC": false, "Question": []any{map[string]any{"name": "huggingface.co.", "type": kind}}, "Answer": items})
	return b
}

func TestHFDoHCompleteAnswerValidation(t *testing.T) {
	good := hfAnswer(1, "93.184.216.34")
	cases := map[string][]byte{
		"private": hfAnswer(1, "127.0.0.1"), "mixed": hfAnswer(1, "93.184.216.34", "10.0.0.1"), "ipv6private": hfAnswer(28, "::1"),
		"truncated":      []byte(strings.Replace(string(good), `"TC":false`, `"TC":true`, 1)),
		"status":         []byte(strings.Replace(string(good), `"Status":0`, `"Status":2`, 1)),
		"missingstatus":  []byte(strings.Replace(string(good), `"Status":0,`, "", 1)),
		"wrongquestion":  []byte(strings.ReplaceAll(string(good), "huggingface.co.", "attacker.example.")),
		"invalidaddress": hfAnswer(1, "93.184.216.34.evil"), "wrongfamily": hfAnswer(1, "2606:4700::1111"),
		"trailing": append(append([]byte{}, good...), []byte(` {}`)...), "null": []byte(`null`),
	}
	tooMany := make([]string, 65)
	for i := range tooMany {
		tooMany[i] = "93.184.216.34"
	}
	cases["too_many"] = hfAnswer(1, tooMany...)
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			kind := 1
			if name == "ipv6private" {
				kind = 28
			}
			_, err := decodeHFDoHAnswer(body, "huggingface.co", kind)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "93.184")
		})
	}
	ips, err := decodeHFDoHAnswer(good, "huggingface.co", 1)
	require.NoError(t, err)
	require.Len(t, ips, 1)
	ips, err = decodeHFDoHAnswer(hfAnswer(28), "huggingface.co", 28)
	require.NoError(t, err)
	require.Empty(t, ips, "empty one family is legal; combined empty is rejected")
}

func TestHFDoHPartialFailureNoFallbackAndBounds(t *testing.T) {
	for _, mode := range []string{"family_http_error", "family_invalid", "body_limit", "empty", "cancel", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			r := &hfDoHResolver{client: &http.Client{Transport: hfRoundTrip(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "cloudflare-dns.com", req.URL.Host)
				require.Empty(t, req.Header.Get("Authorization"))
				require.Equal(t, "application/dns-json", req.Header.Get("Accept"))
				kind, _ := strconv.Atoi(req.URL.Query().Get("type"))
				body := hfAnswer(kind)
				status := 200
				headers := http.Header{}
				if kind == 1 {
					body = hfAnswer(1, "93.184.216.34")
				}
				switch mode {
				case "family_http_error":
					if kind == 28 {
						status = 503
					}
				case "family_invalid":
					if kind == 28 {
						body = []byte(`{`)
					}
				case "body_limit":
					body = []byte(strings.Repeat("x", (64<<10)+1))
				case "empty":
					body = hfAnswer(kind)
				case "cancel":
					<-req.Context().Done()
					return nil, req.Context().Err()
				case "redirect":
					status = 302
					headers.Set("Location", "https://attacker.example/collect")
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(body))), Header: headers, Request: req}, nil
			}), CheckRedirect: func(*http.Request, []*http.Request) error { return errHFResolution }}}
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancel()
			_, err := r.LookupIPAddr(ctx, "huggingface.co")
			require.Error(t, err)
			require.LessOrEqual(t, calls, 2)
			before := calls
			_, err = r.LookupIPAddr(context.Background(), "attacker.example")
			require.ErrorIs(t, err, ErrUnsafeDestination)
			require.Equal(t, before, calls)
		})
	}
}

func hfTLSFixture(t *testing.T, h http.Handler) (*httptest.Server, *tls.Config) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "HF test only"}, DNSNames: []string{"cloudflare-dns.com", "huggingface.co", "cas-bridge.xethub.hf.co", "us.aws.cdn.hf.co"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	pk, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	cert, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: pk}))
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(h)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	root, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(root)
	return server, &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}

// Test-only proxy records the actual wire address type. Only numeric public
// destinations are forwarded into our local TLS fixture; no external network.
func hfSOCKSFixture(t *testing.T, endpoint string) (string, *atomic.Int32, func() []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	var mu sync.Mutex
	var targets []string
	var bad atomic.Int32
	var wg sync.WaitGroup
	var active sync.Map
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			active.Store(c, true)
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer active.Delete(c)
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(3 * time.Second))
				hello := make([]byte, 3)
				if _, err := io.ReadFull(c, hello); err != nil || hello[0] != 5 || hello[1] != 1 || hello[2] != 0 {
					bad.Add(1)
					return
				}
				if _, err := c.Write([]byte{5, 0}); err != nil {
					return
				}
				header := make([]byte, 4)
				if _, err := io.ReadFull(c, header); err != nil {
					return
				}
				size := 0
				switch header[3] {
				case 1:
					size = 4
				case 4:
					size = 16
				default:
					bad.Add(1)
					return
				}
				if header[0] != 5 || header[1] != 1 || header[2] != 0 {
					bad.Add(1)
					return
				}
				raw := make([]byte, size+2)
				if _, err := io.ReadFull(c, raw); err != nil {
					return
				}
				ip := net.IP(raw[:size])
				port := binary.BigEndian.Uint16(raw[size:])
				if !isPublicIP(ip) || port != 443 {
					bad.Add(1)
					return
				}
				target := net.JoinHostPort(ip.String(), strconv.Itoa(int(port)))
				mu.Lock()
				targets = append(targets, target)
				mu.Unlock()
				peer, err := net.DialTimeout("tcp", endpoint, time.Second)
				if err != nil {
					return
				}
				defer peer.Close()
				_, err = c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
				if err != nil {
					return
				}
				_ = c.SetDeadline(time.Time{})
				done := make(chan struct{})
				go func() { _, _ = io.Copy(peer, c); _ = peer.Close(); close(done) }()
				_, _ = io.Copy(c, peer)
				_ = c.Close()
				<-done
			}()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		active.Range(func(k, v any) bool { _ = k.(net.Conn).Close(); return true })
		wg.Wait()
	})
	return ln.Addr().String(), &bad, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string{}, targets...) }
}

func TestHFOperatorSOCKSRealTLSNumericAndStickyCredentialBoundary(t *testing.T) {
	var dnsCalls, contentCalls atomic.Int32
	var violation atomic.Bool
	server, roots := hfTLSFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS.ServerName != r.Host || r.ProtoMajor != 2 {
			violation.Store(true)
		}
		if r.Host == "cloudflare-dns.com" {
			dnsCalls.Add(1)
			if r.Header.Get("Authorization") != "" || r.URL.Path != "/dns-query" {
				violation.Store(true)
			}
			kind, _ := strconv.Atoi(r.URL.Query().Get("type"))
			body := hfAnswer(kind)
			if kind == 1 {
				body = hfAnswer(1, "93.184.216.34")
			}
			body = []byte(strings.ReplaceAll(string(body), "huggingface.co.", r.URL.Query().Get("name")+"."))
			_, _ = w.Write(body)
			return
		}
		contentCalls.Add(1)
		switch r.URL.Path {
		case "/start":
			if r.Header.Get("Authorization") != "Bearer synthetic-private-token" {
				violation.Store(true)
			}
			http.Redirect(w, r, "https://us.aws.cdn.hf.co/signed", 302)
		case "/signed":
			if r.Header.Get("Authorization") != "" {
				violation.Store(true)
			}
			http.Redirect(w, r, "https://huggingface.co/end", 302)
		case "/end":
			if r.Header.Get("Authorization") != "" {
				violation.Store(true)
			}
			_, _ = w.Write([]byte(`{"id":"00123"}`))
		default:
			violation.Store(true)
			w.WriteHeader(500)
		}
	}))
	addr, bad, targets := hfSOCKSFixture(t, server.Listener.Addr().String())
	dial, err := newHFNumericSOCKSDial(addr)
	require.NoError(t, err)
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "socks5h://127.0.0.1:1")
	f := newHFOperatorFetcher(dial, roots)
	defer f.client.CloseIdleConnections()
	defer f.resolver.(*hfDoHResolver).client.CloseIdleConnections()
	result, err := f.FetchHuggingFace(context.Background(), "https://huggingface.co/start", JSONCredential{Origin: "https://huggingface.co/", Header: "Authorization", Value: "Bearer synthetic-private-token"}, 1024)
	require.NoError(t, err)
	require.Equal(t, `{"id":"00123"}`, string(result.Body))
	require.EqualValues(t, 3, contentCalls.Load())
	require.GreaterOrEqual(t, dnsCalls.Load(), int32(6))
	require.False(t, violation.Load())
	require.Zero(t, bad.Load())
	for _, target := range targets() {
		host, port, err := net.SplitHostPort(target)
		require.NoError(t, err)
		require.NotNil(t, net.ParseIP(host))
		require.Equal(t, "443", port)
	}
}

func TestHFOperatorSOCKSRebindingAndUntrustedTLSFailClosed(t *testing.T) {
	for _, mode := range []string{"rebind", "untrusted_doh", "untrusted_hf", "http503"} {
		t.Run(mode, func(t *testing.T) {
			var aQueries, content atomic.Int32
			server, roots := hfTLSFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Host == "cloudflare-dns.com" {
					kind, _ := strconv.Atoi(r.URL.Query().Get("type"))
					body := hfAnswer(kind)
					if kind == 1 {
						ip := "93.184.216.34"
						if aQueries.Add(1) > 1 && mode == "rebind" {
							ip = "127.0.0.1"
						}
						body = hfAnswer(1, ip)
					}
					_, _ = w.Write(body)
					return
				}
				content.Add(1)
				w.WriteHeader(503)
			}))
			addr, bad, _ := hfSOCKSFixture(t, server.Listener.Addr().String())
			dial, err := newHFNumericSOCKSDial(addr)
			require.NoError(t, err)
			f := newHFOperatorFetcher(dial, roots)
			if mode == "untrusted_doh" {
				f.resolver.(*hfDoHResolver).client.Transport = newHFDoHResolver(dial, &tls.Config{RootCAs: x509.NewCertPool()}).client.Transport
			}
			if mode == "untrusted_hf" {
				f.client.Transport = f.safeTransport(&tls.Config{RootCAs: x509.NewCertPool()}, dial)
			}
			defer f.client.CloseIdleConnections()
			defer f.resolver.(*hfDoHResolver).client.CloseIdleConnections()
			_, err = f.FetchHuggingFace(context.Background(), "https://huggingface.co/data", JSONCredential{}, 1024)
			require.Error(t, err)
			require.Zero(t, bad.Load())
			if mode == "http503" {
				require.ErrorIs(t, err, ErrUnexpectedStatus)
				require.EqualValues(t, 1, content.Load())
			} else {
				require.Zero(t, content.Load())
			}
			if mode == "rebind" {
				require.ErrorIs(t, err, ErrUnsafeDestination)
			}
		})
	}
}

func TestHFOperatorSOCKSCancelClosesHandshake(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	entered := make(chan struct{})
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(time.Second))
		buf := make([]byte, 3)
		_, _ = io.ReadFull(c, buf)
		close(entered)
		_, _ = io.Copy(io.Discard, c)
	}()
	dial, err := newHFNumericSOCKSDial(ln.Addr().String())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := make(chan error, 1)
	go func() { _, err := dial(ctx, "tcp", "93.184.216.34:443"); out <- err }()
	<-entered
	cancel()
	select {
	case err := <-out:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("SOCKS handshake ignored cancellation")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("SOCKS connection leaked")
	}
}

func TestHFOperatorPublicReadDiagnostic(t *testing.T) {
	if os.Getenv("COGNIDA_HF_SOCKS_READ_DIAGNOSTIC") != "1" {
		t.Skip("explicit operator read-only diagnostic")
	}
	f, err := NewHuggingFaceOperatorFetcher(HuggingFaceEgressConfig{SOCKS5Address: os.Getenv("HUGGINGFACE_SOCKS5_ADDRESS")})
	require.NoError(t, err)
	require.IsType(t, &hfDoHResolver{}, f.resolver, "diagnostic must use explicit trusted egress")
	defer f.client.CloseIdleConnections()
	defer f.resolver.(*hfDoHResolver).client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	started := time.Now()
	out, err := f.FetchHuggingFace(ctx, "https://huggingface.co/api/datasets/mteb/banking77/revision/18072d2685ea682290f7b8924d94c62acc19c0b2", JSONCredential{}, 2<<20)
	if err != nil {
		t.Fatalf("public read failed; timeout=%t", errors.Is(err, context.DeadlineExceeded))
	}
	var body struct {
		ID  string `json:"id"`
		SHA string `json:"sha"`
	}
	require.NoError(t, json.Unmarshal(out.Body, &body))
	require.Equal(t, "mteb/banking77", body.ID)
	require.Equal(t, "18072d2685ea682290f7b8924d94c62acc19c0b2", body.SHA)
	t.Logf("public fixed metadata verified: status=%d bytes=%d elapsed_ms=%d credentials=false import=false", out.StatusCode, len(out.Body), time.Since(started).Milliseconds())
}

func TestHFAWSEdgeExactAllowlist(t *testing.T) {
	require.True(t, hfAllowedHost("us.aws.cdn.hf.co"))
	for _, host := range []string{"us.aws.cdn.hf.co.evil.example", "evil.us.aws.cdn.hf.co", "us.aws.cdn.hf.co.", "attacker.hf.co"} {
		require.False(t, hfAllowedHost(host))
		r := &hfDoHResolver{}
		_, err := r.LookupIPAddr(context.Background(), host)
		require.ErrorIs(t, err, ErrUnsafeDestination)
	}
}

func TestHFOperatorPublicFileDiagnostic(t *testing.T) {
	if os.Getenv("COGNIDA_HF_SOCKS_FILE_DIAGNOSTIC") != "1" {
		t.Skip("explicit operator read-only file diagnostic")
	}
	f, err := NewHuggingFaceOperatorFetcher(HuggingFaceEgressConfig{SOCKS5Address: os.Getenv("HUGGINGFACE_SOCKS5_ADDRESS")})
	require.NoError(t, err)
	require.IsType(t, &hfDoHResolver{}, f.resolver)
	defer f.client.CloseIdleConnections()
	defer f.resolver.(*hfDoHResolver).client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	started := time.Now()
	out, err := f.FetchHuggingFace(ctx, "https://huggingface.co/datasets/mteb/banking77/resolve/18072d2685ea682290f7b8924d94c62acc19c0b2/data/train-00000-of-00001.parquet", JSONCredential{}, 8<<20)
	if err != nil {
		t.Fatalf("public file read failed; timeout=%t unsafe=%t", errors.Is(err, context.DeadlineExceeded), errors.Is(err, ErrUnsafeDestination))
	}
	require.GreaterOrEqual(t, len(out.Body), 8)
	require.Equal(t, "PAR1", string(out.Body[:4]))
	require.Equal(t, "PAR1", string(out.Body[len(out.Body)-4:]))
	require.Equal(t, 294377, len(out.Body))
	t.Logf("public fixed parquet verified: status=%d bytes=%d sha256=%x elapsed_ms=%d credentials=false import=false", out.StatusCode, len(out.Body), sha256.Sum256(out.Body), time.Since(started).Milliseconds())
}
