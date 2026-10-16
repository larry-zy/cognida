package fetcher

import (
	"context"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

type hfRoundTrip func(*http.Request) (*http.Response, error)

func (f hfRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestHuggingFaceRedirectNeverForwardsBearerAndDoesNotRetryHTTP(t *testing.T) {
	for _, target := range []string{"https://cas-bridge.xethub.hf.co/signed?x=opaque", "https://attacker.example/signed", "https://127.0.0.1/file"} {
		t.Run(target, func(t *testing.T) {
			calls := 0
			f := New(Options{Resolver: resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
			}), Transport: hfRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					require.Equal(t, "Bearer synthetic-private-token", r.Header.Get("Authorization"))
					return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{target}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
				}
				require.Empty(t, r.Header.Get("Authorization"))
				return &http.Response{StatusCode: 503, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("upstream unavailable")), Request: r}, nil
			})})
			_, e := f.FetchHuggingFace(context.Background(), "https://huggingface.co/datasets/org/repo/resolve/commit/data.jsonl", JSONCredential{Origin: "https://huggingface.co/", Header: "Authorization", Value: "Bearer synthetic-private-token"}, 1024)
			if strings.Contains(target, "cas-bridge") {
				require.ErrorIs(t, e, ErrUnexpectedStatus)
				require.Equal(t, 2, calls)
			} else {
				require.ErrorIs(t, e, ErrUnsafeDestination)
				require.Equal(t, 1, calls)
			}
		})
	}
}
func TestHuggingFaceBoundsReflectionAndHost(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		limit      int64
		want       error
	}{{"limit", strings.Repeat("x", 30), 10, ErrResponseTooLarge}, {"reflection", "synthetic-private-token", 1024, ErrAuthentication}, {"accepted", "{\"id\":1}", 1024, nil}} {
		t.Run(tc.name, func(t *testing.T) {
			f := New(Options{Resolver: resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
			}), Transport: hfRoundTrip(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body)), Request: r}, nil
			})})
			out, e := f.FetchHuggingFace(context.Background(), "https://huggingface.co/api/datasets", JSONCredential{Origin: "https://huggingface.co/", Header: "Authorization", Value: "Bearer synthetic-private-token"}, tc.limit)
			if tc.want != nil {
				require.ErrorIs(t, e, tc.want)
			} else {
				require.NoError(t, e)
				require.Equal(t, tc.body, string(out.Body))
			}
		})
	}
	f := New(Options{})
	_, e := f.FetchHuggingFace(context.Background(), "https://127.0.0.1/", JSONCredential{}, 1024)
	require.ErrorIs(t, e, ErrUnsafeDestination)
}
