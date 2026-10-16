package fetcher

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// JSONCredential is assembled from a managed credential after authorization.
// It is never supplied by an ingestion request or persisted in task parameters.
// Origin binds credentials to the exact HTTPS host and port, including redirects.
type JSONCredential struct{ Origin, Header, Value string }

var ErrAuthentication = errors.New("API source authentication failed")
var ErrCursorExpired = errors.New("API snapshot cursor expired")

// FetchJSON uses the same DNS revalidation, public-address transport and request
// gate as Fetch. Cross-origin redirects are refused before headers can be sent.
// Provider errors, response headers and URLs are not included in returned errors.
func (f *Fetcher) FetchJSON(ctx context.Context, raw string, credential JSONCredential, maxBytes int64) (*Result, error) {
	normalized, err := f.validateURL(ctx, raw)
	if err != nil {
		return nil, publicFetchError(err)
	}
	target, _ := url.Parse(normalized)
	origin, err := NormalizeURL(credential.Origin)
	if err != nil {
		return nil, ErrInvalidURL
	}
	bound, _ := url.Parse(origin)
	if target.Scheme != bound.Scheme || target.Host != bound.Host || bound.Path != "/" || bound.RawQuery != "" {
		return nil, ErrUnsafeDestination
	}
	if credential.Header != "Authorization" && credential.Header != "X-API-Key" {
		return nil, ErrAuthentication
	}
	if len(credential.Value) < 16 || len(credential.Value) > 8192 || strings.ContainsAny(credential.Value, "\r\n\x00") {
		return nil, ErrAuthentication
	}
	if maxBytes < 1 || maxBytes > 8<<20 {
		return nil, ErrResponseTooLarge
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, normalized, nil)
	if err != nil {
		return nil, ErrInvalidURL
	}
	request.Header.Set("User-Agent", f.userAgent)
	request.Header.Set("Accept", "application/json")
	request.Header.Set(credential.Header, credential.Value)
	client := *f.client
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return ErrInvalidURL
		}
		if next.URL.Scheme != bound.Scheme || next.URL.Host != bound.Host {
			return ErrUnsafeDestination
		}
		_, err := f.validateURL(next.Context(), next.URL.String())
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, publicFetchError(err)
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return nil, ErrAuthentication
	}
	if response.StatusCode == 410 {
		return nil, ErrCursorExpired
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, ErrUnexpectedStatus
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	if contentType != "application/json" && !strings.HasSuffix(contentType, "+json") {
		return nil, ErrUnsupportedType
	}
	if response.ContentLength > maxBytes {
		return nil, ErrResponseTooLarge
	}
	body, err := readBounded(response.Body, maxBytes)
	if err != nil {
		return nil, publicFetchError(err)
	}
	// A provider reflecting the active credential must not make it an artifact.
	secret := strings.TrimPrefix(credential.Value, "Bearer ")
	if strings.Contains(string(body), secret) {
		return nil, ErrAuthentication
	}
	return &Result{URL: response.Request.URL.String(), StatusCode: response.StatusCode, ContentType: contentType, Body: body}, nil
}
func publicFetchError(err error) error {
	for _, known := range []error{ErrInvalidURL, ErrUnsafeDestination, ErrUnsupportedType, ErrResponseTooLarge, ErrUnexpectedStatus, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, known) {
			return known
		}
	}
	return errors.New("API source request failed")
}
