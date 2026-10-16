package fetcher

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func twoPublicIPs(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("93.184.216.35")}}, nil
}

func TestApprovedDialSharesBudgetAndHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	calls := 0
	client, peer := net.Pipe()
	defer peer.Close()
	conn, err := dialApproved(ctx, resolverFunc(twoPublicIPs), func(ctx context.Context, _, address string) (net.Conn, error) {
		calls++
		if calls == 1 {
			require.Equal(t, "93.184.216.34:443", address)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		require.Equal(t, "93.184.216.35:443", address)
		return client, nil
	}, "tcp", "example.com:443", nil)
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.NoError(t, conn.Close())

	cancelled, stop := context.WithCancel(context.Background())
	stop()
	_, err = dialApproved(cancelled, resolverFunc(twoPublicIPs), func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("cancelled dial must not connect")
		return nil, nil
	}, "tcp", "example.com:443", nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestApprovedDialRejectsWholeMixedAnswerBeforeConnecting(t *testing.T) {
	resolve := resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("127.0.0.1")}}, nil
	})
	_, err := dialApproved(context.Background(), resolve, func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("no address may be dialed when any answer is private")
		return nil, nil
	}, "tcp", "example.com:443", &tls.Config{})
	require.ErrorIs(t, err, ErrUnsafeDestination)
}

func TestFetcherTLSFallbackPrecedesSingleHTTPRequest(t *testing.T) {
	for _, h2 := range []bool{false, true} {
		name := "http1"
		if h2 {
			name = "http2"
		}
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			var wrongIdentity atomic.Bool
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Host != "example.com" || r.TLS.ServerName != "example.com" || r.Header.Get("Authorization") != "Bearer synthetic-testing-token" || (h2 && r.ProtoMajor != 2) {
					wrongIdentity.Store(true)
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"items":[{"id":"00123"}]}`)
			}))
			server.EnableHTTP2 = h2
			server.StartTLS()
			defer server.Close()
			roots := x509.NewCertPool()
			roots.AddCert(server.Certificate())
			var dials atomic.Int32
			stalledClosed := make(chan struct{})
			f := New(Options{Resolver: resolverFunc(twoPublicIPs), TLSConfig: &tls.Config{RootCAs: roots}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				dials.Add(1)
				if address == "93.184.216.34:443" {
					client, peer := net.Pipe()
					go func() { defer close(stalledClosed); defer peer.Close(); _, _ = io.Copy(io.Discard, peer) }()
					return client, nil // TCP succeeds; TLS never replies.
				}
				if address != "93.184.216.35:443" {
					return nil, errors.New("unexpected dial target")
				}
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			}})
			defer f.client.CloseIdleConnections()
			result, err := f.FetchJSON(context.Background(), "https://example.com/items", JSONCredential{Origin: "https://example.com/", Header: "Authorization", Value: "Bearer synthetic-testing-token"}, 1024)
			require.NoError(t, err)
			require.Contains(t, string(result.Body), "00123")
			require.EqualValues(t, 2, dials.Load())
			require.EqualValues(t, 1, requests.Load(), "fallback must not replay the HTTP request")
			require.False(t, wrongIdentity.Load(), "TLS hostname, HTTP host, credentials and ALPN must be preserved")
			select {
			case <-stalledClosed:
			case <-time.After(time.Second):
				t.Fatal("failed TLS connection leaked")
			}
		})
	}
}

func TestFetcherDoesNotFallbackAfterHTTPResponseOrInvalidCertificate(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	for _, trusted := range []bool{true, false} {
		var dials atomic.Int32
		cfg := &tls.Config{RootCAs: x509.NewCertPool()}
		if trusted {
			cfg.RootCAs = roots
		}
		f := New(Options{Resolver: resolverFunc(twoPublicIPs), TLSConfig: cfg, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			dials.Add(1)
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		}})
		_, err := f.FetchJSON(context.Background(), "https://example.com/items", JSONCredential{Origin: "https://example.com/", Header: "Authorization", Value: "Bearer synthetic-testing-token"}, 1024)
		f.client.CloseIdleConnections()
		if trusted {
			require.ErrorIs(t, err, ErrUnexpectedStatus)
			require.EqualValues(t, 1, dials.Load())
		} else {
			require.Error(t, err)
			require.EqualValues(t, 2, dials.Load())
		}
		require.EqualValues(t, 1, calls.Load(), "no retry after HTTP status and no HTTP on untrusted certificates")
	}
}

func TestApprovedTLSCancellationClosesConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client, peer := net.Pipe()
	defer peer.Close()
	entered := make(chan struct{})
	returned := make(chan error, 1)
	go func() {
		_, err := dialApproved(ctx, resolverFunc(twoPublicIPs), func(context.Context, string, string) (net.Conn, error) {
			close(entered)
			return client, nil
		}, "tcp", "example.com:443", &tls.Config{})
		returned <- err
	}()
	<-entered
	cancel()
	select {
	case err := <-returned:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("TLS cancellation stuck")
	}
	_, err := peer.Write([]byte("x"))
	require.Error(t, err, "cancelled connection must close")
}

func TestFetcherRevalidatesDNSAtTLSConnection(t *testing.T) {
	calls := 0
	f := New(Options{Resolver: resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		calls++
		if calls == 1 {
			return twoPublicIPs(context.Background(), "")
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}), DialContext: func(context.Context, string, string) (net.Conn, error) {
		t.Error("rebound destination must not dial")
		return nil, errors.New("unsafe")
	}})
	defer f.client.CloseIdleConnections()
	_, err := f.FetchJSON(context.Background(), "https://example.com/items", JSONCredential{Origin: "https://example.com/", Header: "Authorization", Value: "Bearer synthetic-testing-token"}, 1024)
	require.ErrorIs(t, err, ErrUnsafeDestination)
	require.Equal(t, 2, calls)
}
