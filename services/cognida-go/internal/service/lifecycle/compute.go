package lifecycle

import (
	"bytes"
	d "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ComputeResult struct {
	SchemaVersion string         `json:"schema_version"`
	InputDigest   string         `json:"input_digest"`
	Method        string         `json:"method"`
	Result        map[string]any `json:"result"`
	Warnings      []string       `json:"warnings"`
}

// Keep computed table identifiers exact both on HTTP responses and checkpoint replay.
func (r *ComputeResult) UnmarshalJSON(raw []byte) error {
	type plain ComputeResult
	var value plain
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	*r = ComputeResult(value)
	return nil
}

type HTTPCompute struct {
	baseURL, token string
	client         *http.Client
}

func NewHTTPCompute(baseURL, token string) *HTTPCompute {
	return &HTTPCompute{baseURL: strings.TrimRight(baseURL, "/"), token: token, client: &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *HTTPCompute) Call(ctx context.Context, operation string, input d.Input, options map[string]any, requestID string) (ComputeResult, error) {
	var out ComputeResult
	if operation != "profile" && operation != "clean" && operation != "enrich" && operation != "analyze" {
		return out, d.ErrInvalid
	}
	if e := input.Validate(); e != nil {
		return out, e
	}
	u, e := url.Parse(c.baseURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return out, fmt.Errorf("lifecycle compute URL is not configured")
	}
	if c.token == "" {
		return out, fmt.Errorf("lifecycle internal HTTP token is not configured")
	}
	if options == nil {
		options = map[string]any{}
	}
	body, e := json.Marshal(map[string]any{"input": input, "options": options})
	if e != nil {
		return out, e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/lifecycle/"+operation, bytes.NewReader(body))
	if e != nil {
		return out, e
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", requestID)
	resp, e := c.client.Do(req)
	if e != nil {
		return out, e
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if e != nil {
		return out, e
	}
	if len(raw) > 8<<20 {
		return out, fmt.Errorf("compute response exceeds limit")
	}
	if resp.StatusCode == 422 {
		return out, fmt.Errorf("%w: lifecycle %s rejected its input/options", d.ErrInvalid, operation)
	}
	if resp.StatusCode != 200 {
		return out, fmt.Errorf("lifecycle %s returned HTTP %d", operation, resp.StatusCode)
	}
	if e = json.Unmarshal(raw, &out); e != nil {
		return out, e
	}
	if out.SchemaVersion != "1" || out.Method == "" || len(out.Result) == 0 {
		return out, fmt.Errorf("invalid lifecycle compute envelope")
	}
	return out, nil
}
