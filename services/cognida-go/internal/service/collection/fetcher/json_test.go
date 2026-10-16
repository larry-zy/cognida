package fetcher

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestFetchJSONCredentialOriginAndLimits(t *testing.T) {
	credential := JSONCredential{Origin: "https://example.com/", Header: "Authorization", Value: "Bearer synthetic-testing-token"}
	for _, test := range []struct {
		name, body, contentType string
		status                  int
		want                    error
	}{{"json", `{"items":[]}`, "application/json", 200, nil}, {"auth", `private-provider-detail`, "application/json", 401, ErrAuthentication}, {"expired", `private-provider-detail`, "application/json", 410, ErrCursorExpired}, {"type", `html`, "text/html", 200, ErrUnsupportedType}, {"too large", strings.Repeat("x", 1025), "application/json", 200, ErrResponseTooLarge}, {"reflection", `{"secret":"synthetic-testing-token"}`, "application/json", 200, ErrAuthentication}} {
		t.Run(test.name, func(t *testing.T) {
			f := New(Options{Resolver: resolverFunc(publicResolver), Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				if req.Header.Get("Authorization") != credential.Value {
					t.Fatal("managed credential missing")
				}
				return response(req, test.status, test.contentType, test.body), nil
			})})
			result, e := f.FetchJSON(context.Background(), "https://example.com/items", credential, 1024)
			if !errors.Is(e, test.want) {
				t.Fatalf("error %v", e)
			}
			if e == nil && (result.ContentType != "application/json") {
				t.Fatal("provider headers exposed")
			}
		})
	}
}
func TestFetchJSONCrossOriginRedirectNeverSendsCredential(t *testing.T) {
	calls := 0
	f := New(Options{Resolver: resolverFunc(publicResolver), Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Host != "example.com" {
			t.Fatal("credential sent to another origin")
		}
		out := response(req, 302, "application/json", "")
		out.Header.Set("Location", "https://other.example/private")
		return out, nil
	})})
	_, e := f.FetchJSON(context.Background(), "https://example.com/items", JSONCredential{Origin: "https://example.com/", Header: "X-API-Key", Value: "synthetic-testing-token"}, 1024)
	if !errors.Is(e, ErrUnsafeDestination) || calls != 1 {
		t.Fatalf("redirect not blocked: %d %v", calls, e)
	}
}
