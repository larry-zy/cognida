package fetcher

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

type resolverFunc func(context.Context, string) ([]net.IPAddr, error)

func (f resolverFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return f(ctx, host)
}

func publicResolver(_ context.Context, _ string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func response(req *http.Request, status int, contentType string, body string) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Status:        http.StatusText(status),
		Header:        http.Header{"Content-Type": []string{contentType}},
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		err  error
	}{
		{"canonicalizes host fragment and root", " HTTPS://Example.COM./docs#part ", "https://example.com/docs", nil},
		{"preserves query", "https://example.com?q=one", "https://example.com/?q=one", nil},
		{"preserves valid port", "https://Example.com:8443/docs", "https://example.com:8443/docs", nil},
		{"preserves IPv6 brackets", "https://[2001:4860:4860::8888]/docs", "https://[2001:4860:4860::8888]/docs", nil},
		{"rejects HTTP", "http://example.com", "", ErrInvalidURL},
		{"rejects relative", "/docs", "", ErrInvalidURL},
		{"rejects credentials", "https://user@example.com", "", ErrInvalidURL},
		{"rejects invalid port", "https://example.com:99999", "", ErrInvalidURL},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeURL(tc.in)
			if !errors.Is(err, tc.err) || got != tc.want {
				t.Fatalf("NormalizeURL(%q) = (%q, %v), want (%q, %v)", tc.in, got, err, tc.want, tc.err)
			}
		})
	}
}

func TestFetcherRejectsNonPublicDestinations(t *testing.T) {
	tests := []struct {
		name string
		url  string
		ip   string
	}{
		{"literal loopback", "https://127.0.0.1/", ""},
		{"private DNS", "https://internal.example/", "10.0.0.8"},
		{"link local DNS", "https://metadata.example/", "169.254.169.254"},
		{"mixed DNS answers", "https://mixed.example/", "93.184.216.34,::1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolver := resolverFunc(func(_ context.Context, _ string) ([]net.IPAddr, error) {
				var out []net.IPAddr
				for _, raw := range strings.Split(tc.ip, ",") {
					if raw != "" {
						out = append(out, net.IPAddr{IP: net.ParseIP(raw)})
					}
				}
				return out, nil
			})
			f := New(Options{Resolver: resolver, Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				t.Fatalf("transport must not run for unsafe destination")
				return nil, nil
			})})
			_, err := f.Fetch(context.Background(), tc.url)
			if !errors.Is(err, ErrUnsafeDestination) {
				t.Fatalf("Fetch error = %v, want ErrUnsafeDestination", err)
			}
		})
	}
}

func TestFetcherBoundsAndValidatesResponses(t *testing.T) {
	tests := []struct {
		name        string
		maxBytes    int64
		status      int
		contentType string
		body        string
		wantErr     error
	}{
		{"accepts HTML", 32, http.StatusOK, "text/html; charset=utf-8", "<h1>ok</h1>", nil},
		{"rejects non textual content", 32, http.StatusOK, "application/pdf", "data", ErrUnsupportedType},
		{"rejects non success", 32, http.StatusNotFound, "text/html", "missing", ErrUnexpectedStatus},
		{"rejects body over cap", 3, http.StatusOK, "text/plain", "four", ErrResponseTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := New(Options{MaxBytes: tc.maxBytes, Resolver: resolverFunc(publicResolver), Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				if got := req.Header.Get("User-Agent"); !strings.Contains(got, "CognidaCollector/") {
					t.Fatalf("missing product User-Agent: %q", got)
				}
				return response(req, tc.status, tc.contentType, tc.body), nil
			})})
			got, err := f.Fetch(context.Background(), "https://example.com")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Fetch error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && (string(got.Body) != tc.body || got.URL != "https://example.com/") {
				t.Fatalf("Fetch result = %#v", got)
			}
		})
	}
}

func TestFetcherValidatesRedirectTarget(t *testing.T) {
	requests := 0
	f := New(Options{Resolver: resolverFunc(func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "private.example" {
			return []net.IPAddr{{IP: net.ParseIP("192.168.1.1")}}, nil
		}
		return publicResolver(context.Background(), host)
	}), Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.URL.Hostname() == "example.com" {
			resp := response(req, http.StatusFound, "text/html", "")
			resp.Header.Set("Location", "https://private.example/secret")
			return resp, nil
		}
		t.Fatalf("private redirect target must not be requested")
		return nil, nil
	})})
	_, err := f.Fetch(context.Background(), "https://example.com/")
	if !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("Fetch error = %v, want ErrUnsafeDestination", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestReadBounded(t *testing.T) {
	got, err := readBounded(bytes.NewBufferString("abc"), 3)
	if err != nil || string(got) != "abc" {
		t.Fatalf("readBounded exact limit = %q, %v", got, err)
	}
	_, err = readBounded(bytes.NewBufferString("abcd"), 3)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("readBounded overflow error = %v", err)
	}
}

type recordingGate struct {
	hosts  []string
	active int
	t      *testing.T
}

func (g *recordingGate) Acquire(_ context.Context, url string) (func(), error) {
	if g.active != 0 {
		g.t.Fatal("previous HTTP hop must release before acquiring next hop")
	}
	g.active++
	g.hosts = append(g.hosts, url)
	return func() { g.active-- }, nil
}
func TestFetcherPolitenessCoversRedirectHopsAndReleasesBodies(t *testing.T) {
	gate := &recordingGate{t: t}
	client := New(Options{Resolver: resolverFunc(publicResolver), Gate: gate, Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if gate.active != 1 {
			t.Fatal("request ran without a permit")
		}
		if req.URL.Host == "example.com" {
			resp := response(req, 302, "text/plain", "")
			resp.Header.Set("Location", "https://other.example/final")
			return resp, nil
		}
		return response(req, 200, "text/plain", "done"), nil
	})})
	result, err := client.Fetch(context.Background(), "https://example.com/start")
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Body) != "done" || len(gate.hosts) != 2 || gate.active != 0 {
		t.Fatalf("gate state: %+v", gate)
	}
}
