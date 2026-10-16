package sandbox

import (
	"bytes"
	d "cognida/internal/model/sandbox"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The local Engine socket is used only by the trusted controller. Persisting
// the Engine exec ID lets a replacement controller observe the real exit code;
// a file written by the guest is never execution evidence.
func (h *DockerHost) engine(ctx context.Context, method, path string, input any, output any) error {
	h.mu.Lock()
	if h.client == nil {
		endpoint := os.Getenv("DOCKER_HOST")
		if endpoint == "" {
			b, e := boundedCommand(ctx, 4096, "context", "inspect", "--format", "{{.Endpoints.docker.Host}}")
			if e != nil {
				h.mu.Unlock()
				return e
			}
			endpoint = strings.TrimSpace(string(b))
		}
		if !strings.HasPrefix(endpoint, "unix:///") {
			h.mu.Unlock()
			return d.ErrUnsupported
		}
		socket := strings.TrimPrefix(endpoint, "unix://")
		h.client = &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		}}}
	}
	client := h.client
	h.mu.Unlock()
	var body []byte
	if input != nil {
		var e error
		body, e = json.Marshal(input)
		if e != nil {
			return d.ErrInvalid
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, "http://docker"+path, bytes.NewReader(body))
	if e != nil {
		return d.ErrInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	resp, e := client.Do(req)
	if e != nil {
		return d.ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return d.ErrConflict
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if e != nil || len(b) > 64<<10 || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return d.ErrUnavailable
	}
	if output != nil && json.Unmarshal(b, output) != nil {
		return d.ErrConflict
	}
	return nil
}

func (h *DockerHost) api(ctx context.Context) (string, error) {
	var v struct {
		APIVersion string `json:"ApiVersion"`
	}
	if e := h.engine(ctx, "GET", "/version", nil, &v); e != nil {
		return "", e
	}
	if !regexp.MustCompile(`^1\.[0-9]{2,3}$`).MatchString(v.APIVersion) {
		return "", d.ErrUnsupported
	}
	minor, _ := strconv.Atoi(strings.TrimPrefix(v.APIVersion, "1."))
	if minor < 41 {
		return "", d.ErrUnsupported
	}
	return "/v" + v.APIVersion, nil
}

func (h *DockerHost) CreateExec(ctx context.Context, v ValidatedSubmission) (string, error) {
	api, e := h.api(ctx)
	if e != nil {
		return "", e
	}
	var result struct {
		ID string `json:"Id"`
	}
	e = h.engine(ctx, "POST", api+"/containers/"+sandboxContainer(v.Binding.ID)+"/exec", map[string]any{"User": "65532:65532", "Privileged": false, "AttachStdout": false, "AttachStderr": false, "Cmd": []string{"python", "-I", "-S", "/inputs/runner.py", strconv.FormatInt(v.Request.Resources.TimeoutSeconds, 10), strconv.FormatInt(v.Request.Resources.LogBytes, 10)}}, &result)
	if e != nil {
		return "", e
	}
	if !engineID.MatchString(result.ID) {
		return "", d.ErrConflict
	}
	return result.ID, nil
}

var engineID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (h *DockerHost) StartExec(ctx context.Context, id string) error {
	if !engineID.MatchString(id) {
		return d.ErrInvalid
	}
	api, e := h.api(ctx)
	if e != nil {
		return e
	}
	return h.engine(ctx, "POST", api+"/exec/"+id+"/start", map[string]any{"Detach": true, "Tty": false}, nil)
}
func (h *DockerHost) ReceiptExec(ctx context.Context, v ValidatedSubmission, id string) (bool, int, error) {
	if !engineID.MatchString(id) {
		return false, 0, d.ErrInvalid
	}
	api, e := h.api(ctx)
	if e != nil {
		return false, 0, e
	}
	var status struct {
		Running     bool
		ExitCode    *int
		ContainerID string
		ID          string `json:"ID"`
	}
	if e = h.engine(ctx, "GET", api+"/exec/"+id+"/json", nil, &status); e != nil {
		return false, 0, e
	}
	var container struct {
		ID string `json:"Id"`
	}
	if e = h.engine(ctx, "GET", api+"/containers/"+sandboxContainer(v.Binding.ID)+"/json", nil, &container); e != nil {
		return false, 0, e
	}
	if status.ID != id || status.ContainerID != container.ID {
		return false, 0, d.ErrConflict
	}
	if status.Running || status.ExitCode == nil {
		return false, 0, nil
	}
	return true, *status.ExitCode, nil
}
