package huggingface

import (
	"bytes"
	d "cognida/internal/model/huggingface"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPDecoder struct {
	endpoint, token string
	client          *http.Client
}

// The endpoint is trusted operator configuration, never an HTTP request field.
// This adapter sends only already validated bytes; Python cannot follow a URL.
func NewHTTPDecoder(baseURL, internalToken string) (*HTTPDecoder, error) {
	u, e := url.Parse(baseURL)
	if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(internalToken, "\r\n") {
		return nil, d.ErrInvalid
	}
	if internalToken == "" {
		return nil, d.ErrUnavailable
	}
	return &HTTPDecoder{endpoint: strings.TrimRight(baseURL, "/") + "/api/v1/connectors/huggingface/decode", token: internalToken, client: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (h *HTTPDecoder) Decode(ctx context.Context, body []byte, format, file string, limit int, mapping d.FieldMapping) (d.DecodeResponse, error) {
	var result d.DecodeResponse
	if len(body) == 0 || int64(len(body)) > d.MaxFileBytes {
		return result, d.ErrLimit
	}
	raw, e := json.Marshal(map[string]any{"format": format, "content_base64": base64.StdEncoding.EncodeToString(body), "source_file": file, "max_rows": limit, "field_mapping": mapping})
	if e != nil {
		return result, d.ErrInvalid
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, bytes.NewReader(raw))
	if e != nil {
		return result, d.ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")
	response, e := h.client.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, d.ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == 413 {
		return result, d.ErrLimit
	}
	if response.StatusCode == 422 {
		return result, d.ErrDecode
	}
	if response.StatusCode != 200 {
		return result, d.ErrUnavailable
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, 8<<20+1))
	if e != nil {
		return result, d.ErrUnavailable
	}
	if len(data) > 8<<20 {
		return result, d.ErrLimit
	}
	if e = decodeJSON(data, &result); e != nil {
		return result, e
	}
	return result, nil
}
