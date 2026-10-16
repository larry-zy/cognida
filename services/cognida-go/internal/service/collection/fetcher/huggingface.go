package fetcher

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// FetchHuggingFace is deliberately narrower than a generic authenticated file
// downloader. Only Hub URLs may start a request. Credentials never follow CDN
// redirects; each destination still goes through DNS/public-address validation.
func (f *Fetcher) FetchHuggingFace(ctx context.Context, raw string, credential JSONCredential, maxBytes int64) (*Result, error) {
	if maxBytes < 1 || maxBytes > 16<<20 {
		return nil, ErrResponseTooLarge
	}
	initial, parseErr := url.Parse(raw)
	if parseErr != nil || initial.Scheme != "https" || initial.Host != "huggingface.co" || initial.User != nil {
		return nil, ErrUnsafeDestination
	}
	normalized, e := f.validateURL(ctx, raw)
	if e != nil {
		return nil, publicFetchError(e)
	}
	u, _ := url.Parse(normalized)
	if u.Host != "huggingface.co" || u.Scheme != "https" {
		return nil, ErrUnsafeDestination
	}
	if credential.Value != "" && (credential.Origin != "https://huggingface.co/" || credential.Header != "Authorization" || !strings.HasPrefix(credential.Value, "Bearer ") || len(credential.Value) < 16 || len(credential.Value) > 8192 || strings.ContainsAny(credential.Value, "\r\n\x00")) {
		return nil, ErrAuthentication
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, normalized, nil)
	if e != nil {
		return nil, ErrInvalidURL
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "application/json, application/octet-stream, text/plain")
	if credential.Value != "" {
		req.Header.Set("Authorization", credential.Value)
	}
	client := *f.client
	credentialStripped := false
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return ErrInvalidURL
		}
		host := next.URL.Hostname()
		if next.URL.Scheme != "https" || next.URL.Port() != "" || !hfAllowedHost(host) {
			return ErrUnsafeDestination
		}
		// After the first redirect credentials are never reintroduced, even if a
		// later redirect returns to the Hub. Signed CDN query values stay internal.
		if next.URL.Host != "huggingface.co" || via[len(via)-1].URL.Host != "huggingface.co" {
			credentialStripped = true
		}
		if credentialStripped {
			next.Header.Del("Authorization")
		}
		_, e := f.validateURL(next.Context(), next.URL.String())
		return e
	}
	response, e := client.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, publicFetchError(e)
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return nil, ErrAuthentication
	}
	if response.StatusCode == 404 {
		return nil, ErrNotFound
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, ErrUnexpectedStatus
	}
	if response.ContentLength > maxBytes {
		return nil, ErrResponseTooLarge
	}
	body, e := readBounded(response.Body, maxBytes)
	if e != nil {
		return nil, publicFetchError(e)
	}
	if credential.Value != "" && strings.Contains(string(body), strings.TrimPrefix(credential.Value, "Bearer ")) {
		return nil, ErrAuthentication
	}
	// Do not expose the final signed CDN URL or provider headers to callers.
	return &Result{URL: normalized, StatusCode: response.StatusCode, Body: body, ContentType: response.Header.Get("Content-Type"), Header: http.Header{"Link": response.Header.Values("Link")}}, nil
}

var ErrNotFound = errors.New("remote resource not found")
