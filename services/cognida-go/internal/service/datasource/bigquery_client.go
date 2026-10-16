package datasource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	m "cognida/internal/model/datasource"
	"golang.org/x/oauth2/google"
)

// Credential files are chosen by the operator, never by a datasource request.
// Each reference is restricted to an exact tenant/project/location/dataset.
type BigQueryCredential struct {
	File         string `json:"file"`
	TenantID     int64  `json:"tenant_id"`
	Project      string `json:"project"`
	Location     string `json:"location"`
	Dataset      string `json:"dataset"`
	MaxScanBytes int64  `json:"max_scan_bytes"`
}
type BigQueryOptions struct {
	CredentialRef string `json:"credential_ref"`
	Project       string `json:"project"`
	Location      string `json:"location"`
	MaxScanBytes  int64  `json:"max_scan_bytes"`
}
type bigQueryAdapter struct {
	source   *m.DataSource
	options  BigQueryOptions
	client   *http.Client
	endpoint string
	prefix   string
}

var bqName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

func parseBigQueryOptions(ds *m.DataSource) (BigQueryOptions, error) {
	var v BigQueryOptions
	dec := json.NewDecoder(bytes.NewReader(ds.Extra))
	dec.DisallowUnknownFields()
	if dec.Decode(&v) != nil || !bqName.MatchString(v.CredentialRef) || !bqName.MatchString(v.Project) || !bqName.MatchString(v.Location) || !bqName.MatchString(ds.DatabaseName) || v.MaxScanBytes < 1 || ds.Host != "bigquery.googleapis.com" || ds.Port != 443 || ds.Username != v.CredentialRef {
		return v, fmt.Errorf("BigQuery requires a managed credential reference, project, location, dataset and positive scan limit; endpoint is fixed")
	}
	return v, nil
}
func newBigQueryAdapter(ctx context.Context, ds *m.DataSource) (*bigQueryAdapter, error) {
	options, e := parseBigQueryOptions(ds)
	if e != nil {
		return nil, e
	}
	var refs map[string]BigQueryCredential
	if json.Unmarshal([]byte(os.Getenv("COGNIDA_BIGQUERY_CREDENTIALS")), &refs) != nil {
		return nil, fmt.Errorf("BigQuery managed credentials are not configured")
	}
	ref, ok := refs[options.CredentialRef]
	if !ok || ref.TenantID != ds.TenantID || ref.Project != options.Project || ref.Location != options.Location || ref.Dataset != ds.DatabaseName || ref.MaxScanBytes < 1 || options.MaxScanBytes > ref.MaxScanBytes {
		return nil, fmt.Errorf("BigQuery credential scope or budget denied")
	}
	f, e := os.Open(ref.File)
	if e != nil {
		return nil, fmt.Errorf("BigQuery credential unavailable")
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if e != nil || len(raw) > 1<<20 {
		return nil, fmt.Errorf("BigQuery credential unavailable")
	}
	var metadata struct {
		Type     string `json:"type"`
		TokenURI string `json:"token_uri"`
		Project  string `json:"project_id"`
	}
	if json.Unmarshal(raw, &metadata) != nil || metadata.Type != "service_account" || metadata.TokenURI != "https://oauth2.googleapis.com/token" || metadata.Project != ref.Project {
		return nil, fmt.Errorf("BigQuery requires a scoped service-account credential")
	}
	cfg, e := google.JWTConfigFromJSON(raw, "https://www.googleapis.com/auth/bigquery")
	if e != nil {
		return nil, fmt.Errorf("BigQuery credential invalid")
	}
	client := cfg.Client(ctx)
	client.Timeout = 35 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s", ds.TenantID, ds.ID, ds.UpdatedAt.UTC().Format(time.RFC3339Nano))))
	return &bigQueryAdapter{source: ds, options: options, client: client, endpoint: "https://bigquery.googleapis.com/bigquery/v2", prefix: "cognida_" + hex.EncodeToString(sum[:12]) + "_"}, nil
}
func (a *bigQueryAdapter) Capabilities() m.QueryCapabilities {
	return m.QueryCapabilities{Engine: m.TypeBigQuery, Client: "BigQuery REST v2 / oauth2", Dialect: "bigquery", Parameters: false, Cancellation: "jobs.cancel", Pagination: "page_token", ScanBudget: true, Async: true}
}

type bqJob struct {
	JobReference struct {
		ProjectID string `json:"projectId"`
		JobID     string `json:"jobId"`
		Location  string `json:"location"`
	} `json:"jobReference"`
	Configuration struct {
		DryRun bool              `json:"dryRun"`
		Labels map[string]string `json:"labels"`
		Query  *bqQuery          `json:"query"`
	} `json:"configuration"`
	Status struct {
		State       string `json:"state"`
		ErrorResult *struct {
			Reason string `json:"reason"`
		} `json:"errorResult"`
	} `json:"status"`
	Statistics struct {
		Query struct {
			TotalBytesProcessed string `json:"totalBytesProcessed"`
			StatementType       string `json:"statementType"`
		} `json:"query"`
	} `json:"statistics"`
}
type bqQuery struct {
	Query              string            `json:"query"`
	UseLegacySQL       bool              `json:"useLegacySql"`
	MaximumBytesBilled string            `json:"maximumBytesBilled"`
	DefaultDataset     map[string]string `json:"defaultDataset"`
	UseQueryCache      bool              `json:"useQueryCache"`
	Priority           string            `json:"priority"`
}

var errBQMissing = errors.New("BigQuery job not found")
var errBQConflict = errors.New("BigQuery job already exists")

func (a *bigQueryAdapter) call(ctx context.Context, method, path string, in, out any) error {
	var raw []byte
	var e error
	if in != nil {
		raw, e = json.Marshal(in)
		if e != nil {
			return e
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, a.endpoint+path, bytes.NewReader(raw))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	resp, e := a.client.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("BigQuery transport unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return errBQMissing
	}
	if resp.StatusCode == 409 {
		return errBQConflict
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("BigQuery request rejected (HTTP %d)", resp.StatusCode)
	}
	body, e := io.ReadAll(io.LimitReader(resp.Body, 8<<20+1))
	if e != nil || len(body) > 8<<20 {
		return fmt.Errorf("BigQuery response exceeds byte budget")
	}
	if out != nil && json.Unmarshal(body, out) != nil {
		return fmt.Errorf("BigQuery response invalid")
	}
	return nil
}
func (a *bigQueryAdapter) jobsPath() string {
	return "/projects/" + url.PathEscape(a.options.Project) + "/jobs"
}
func (a *bigQueryAdapter) jobPath(id string) string {
	return a.jobsPath() + "/" + url.PathEscape(id) + "?location=" + url.QueryEscape(a.options.Location)
}
func (a *bigQueryAdapter) validID(id string) bool {
	return strings.HasPrefix(id, a.prefix) && len(id) == len(a.prefix)+64 && regexp.MustCompile(`^[a-f0-9]+$`).MatchString(strings.TrimPrefix(id, a.prefix))
}
func (a *bigQueryAdapter) get(ctx context.Context, id string) (*bqJob, error) {
	if !a.validID(id) {
		return nil, fmt.Errorf("BigQuery job scope denied")
	}
	var job bqJob
	if e := a.call(ctx, "GET", a.jobPath(id), nil, &job); e != nil {
		return nil, e
	}
	if job.JobReference.ProjectID != a.options.Project || job.JobReference.Location != a.options.Location || job.JobReference.JobID != id || job.Configuration.Query == nil || job.Configuration.DryRun {
		return nil, fmt.Errorf("BigQuery job binding mismatch")
	}
	return &job, nil
}
func (a *bigQueryAdapter) prepare(in m.QueryRequest) (*bqQuery, string, error) {
	if e := ValidateReadQuery(in.SQL); e != nil {
		return nil, "", e
	}
	if len(in.Parameters) > 0 || in.MaxRows < 1 || in.MaxRows > 10000 || in.MaxBytes < 1 || in.MaxBytes > 8<<20 || in.Timeout <= 0 || in.Timeout > 30*time.Second || len(in.OperationID) < 8 || len(in.OperationID) > 200 {
		return nil, "", fmt.Errorf("BigQuery query requires bounded output, timeout and a persisted operation ID")
	}
	if regexp.MustCompile(`(?i)\b(EXTERNAL_QUERY|EXTERNAL_OBJECT_TRANSFORM|ML\.|AI\.|REMOTE_FUNCTION)`).MatchString(in.SQL) {
		return nil, "", fmt.Errorf("BigQuery external or model functions are not supported")
	}
	limit := in.MaxScanBytes
	if limit == 0 {
		limit = a.options.MaxScanBytes
	}
	if limit < 1 || limit > a.options.MaxScanBytes {
		return nil, "", fmt.Errorf("BigQuery scan budget denied")
	}
	q := &bqQuery{Query: fmt.Sprintf("SELECT * FROM (%s) AS cognida_bounded LIMIT %d", strings.TrimSuffix(strings.TrimSpace(in.SQL), ";"), in.MaxRows+1), MaximumBytesBilled: strconv.FormatInt(limit, 10), DefaultDataset: map[string]string{"projectId": a.options.Project, "datasetId": a.source.DatabaseName}, Priority: "INTERACTIVE"}
	// The operation ID, not SQL, identifies a submission. A changed payload
	// under the same operation therefore conflicts instead of billing twice.
	hash := sha256.Sum256([]byte(in.OperationID))
	return q, a.prefix + hex.EncodeToString(hash[:]), nil
}
func sameBQQuery(a, b *bqQuery) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func (a *bigQueryAdapter) Submit(ctx context.Context, in m.QueryRequest) (string, error) {
	q, id, e := a.prepare(in)
	raw, _ := json.Marshal(struct {
		Request       m.QueryRequest
		TimeoutMillis int64
	}{in, in.Timeout.Milliseconds()})
	digest := sha256.Sum256(raw)
	requestDigest := hex.EncodeToString(digest[:])[:63]
	if e != nil {
		return "", e
	}
	if old, e := a.get(ctx, id); e == nil {
		if !sameBQQuery(q, old.Configuration.Query) || old.Configuration.Labels["cognida_request"] != requestDigest {
			return "", fmt.Errorf("BigQuery operation payload conflict")
		}
		return id, nil
	} else if !errors.Is(e, errBQMissing) {
		return "", e
	}
	request := map[string]any{"jobReference": map[string]string{"projectId": a.options.Project, "jobId": id, "location": a.options.Location}, "configuration": map[string]any{"dryRun": true, "query": q, "labels": map[string]string{"cognida_request": requestDigest}, "jobTimeoutMs": strconv.FormatInt(in.Timeout.Milliseconds(), 10)}}
	var dry bqJob
	if e = a.call(ctx, "POST", a.jobsPath(), request, &dry); e != nil {
		return "", e
	}
	bytes, e := strconv.ParseInt(dry.Statistics.Query.TotalBytesProcessed, 10, 64)
	limit, _ := strconv.ParseInt(q.MaximumBytesBilled, 10, 64)
	if e != nil || bytes < 0 || bytes > limit || dry.Status.ErrorResult != nil || dry.Statistics.Query.StatementType != "SELECT" {
		return "", fmt.Errorf("BigQuery dry-run rejected statement or scan budget")
	}
	request["configuration"].(map[string]any)["dryRun"] = false
	var actual bqJob
	if e = a.call(ctx, "POST", a.jobsPath(), request, &actual); e != nil {
		// The deterministic ID remains recoverable after a lost response. Never
		// retry with a fresh ID or downgrade the original billing ceiling.
		if !errors.Is(e, errBQConflict) {
			return id, e
		}
		old, readErr := a.get(ctx, id)
		if readErr != nil {
			return id, readErr
		}
		actual = *old
	}
	if actual.JobReference.JobID != id || actual.JobReference.ProjectID != a.options.Project || actual.JobReference.Location != a.options.Location || (!sameBQQuery(q, actual.Configuration.Query) || actual.Configuration.Labels["cognida_request"] != requestDigest) {
		return id, fmt.Errorf("BigQuery submitted job binding mismatch")
	}
	return id, nil
}
func (a *bigQueryAdapter) Status(ctx context.Context, id string) (string, error) {
	j, e := a.get(ctx, id)
	if e != nil {
		return "", e
	}
	if j.Status.ErrorResult != nil {
		return "failed", nil
	}
	switch j.Status.State {
	case "DONE":
		return "succeeded", nil
	case "RUNNING", "PENDING":
		return "running", nil
	}
	return "", fmt.Errorf("BigQuery unknown job state")
}
func (a *bigQueryAdapter) LookupSubmission(ctx context.Context, in m.QueryRequest) (string, string, bool, error) {
	q, id, err := a.prepare(in)
	if err != nil {
		return "", "", false, err
	}
	job, err := a.get(ctx, id)
	if errors.Is(err, errBQMissing) {
		return id, "", false, nil
	}
	if err != nil {
		return id, "", false, err
	}
	raw, _ := json.Marshal(struct {
		Request       m.QueryRequest
		TimeoutMillis int64
	}{in, in.Timeout.Milliseconds()})
	digest := sha256.Sum256(raw)
	if !sameBQQuery(q, job.Configuration.Query) || job.Configuration.Labels["cognida_request"] != hex.EncodeToString(digest[:])[:63] {
		return id, "", true, fmt.Errorf("BigQuery operation payload conflict")
	}
	state, err := a.Status(ctx, id)
	return id, state, true, err
}
func (a *bigQueryAdapter) Cancel(ctx context.Context, id string) error {
	if _, e := a.get(ctx, id); e != nil {
		return e
	}
	return a.call(ctx, "POST", a.jobsPath()+"/"+url.PathEscape(id)+"/cancel?location="+url.QueryEscape(a.options.Location), struct{}{}, nil)
}

type bqField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Mode        string `json:"mode"`
	Description string `json:"description"`
}
type bqPage struct {
	JobComplete bool   `json:"jobComplete"`
	PageToken   string `json:"pageToken"`
	Schema      struct {
		Fields []bqField `json:"fields"`
	} `json:"schema"`
	Rows []struct {
		F []struct {
			V json.RawMessage `json:"v"`
		} `json:"f"`
	} `json:"rows"`
	Errors []json.RawMessage `json:"errors"`
}

func (a *bigQueryAdapter) Page(ctx context.Context, id, token string, limit int) (*m.QueryResult, string, error) {
	if limit < 1 || limit > 1000 || len(token) > 4096 {
		return nil, "", fmt.Errorf("BigQuery page limit invalid")
	}
	j, e := a.get(ctx, id)
	if e != nil {
		return nil, "", e
	}
	if j.Status.State != "DONE" || j.Status.ErrorResult != nil {
		return nil, "", fmt.Errorf("BigQuery job has no completed result")
	}
	path := "/projects/" + url.PathEscape(a.options.Project) + "/queries/" + url.PathEscape(id) + "?location=" + url.QueryEscape(a.options.Location) + "&maxResults=" + strconv.Itoa(limit) + "&pageToken=" + url.QueryEscape(token) + "&formatOptions.useInt64Timestamp=true"
	var page bqPage
	if e = a.call(ctx, "GET", path, nil, &page); e != nil {
		return nil, "", e
	}
	if !page.JobComplete || len(page.Errors) > 0 || len(page.Rows) > limit || len(page.Schema.Fields) < 1 || len(page.Schema.Fields) > 512 {
		return nil, "", fmt.Errorf("BigQuery result unavailable or invalid")
	}
	out := &m.QueryResult{Columns: []string{}, ColumnTypes: map[string]string{}, Rows: []map[string]any{}, RemoteID: id, ExecutedSQL: j.Configuration.Query.Query, SourceVersion: a.source.UpdatedAt.UTC().Format(time.RFC3339Nano)}
	for _, f := range page.Schema.Fields {
		if f.Mode == "REPEATED" || f.Type == "RECORD" || f.Type == "STRUCT" || f.Type == "BYTES" || f.Name == "" {
			return nil, "", fmt.Errorf("BigQuery nested or binary results unsupported")
		}
		if _, exists := out.ColumnTypes[f.Name]; exists {
			return nil, "", fmt.Errorf("BigQuery duplicate column")
		}
		out.Columns = append(out.Columns, f.Name)
		out.ColumnTypes[f.Name] = f.Type
	}
	for _, row := range page.Rows {
		if len(row.F) != len(out.Columns) {
			return nil, "", fmt.Errorf("BigQuery row schema mismatch")
		}
		values := map[string]any{}
		for i, cell := range row.F {
			var v any
			if string(cell.V) == "null" || len(cell.V) == 0 {
				v = nil
			} else {
				var text string
				if json.Unmarshal(cell.V, &text) != nil {
					return nil, "", fmt.Errorf("BigQuery scalar invalid")
				}
				v = text
				if out.ColumnTypes[out.Columns[i]] == "BOOLEAN" || out.ColumnTypes[out.Columns[i]] == "BOOL" {
					b, e := strconv.ParseBool(text)
					if e != nil {
						return nil, "", e
					}
					v = b
				}
			}
			values[out.Columns[i]] = v
		}
		out.Rows = append(out.Rows, values)
	}
	return out, page.PageToken, nil
}
func (a *bigQueryAdapter) Query(ctx context.Context, in m.QueryRequest) (*m.QueryResult, error) {
	ctx, cancel := context.WithTimeout(ctx, in.Timeout)
	defer cancel()
	id, e := a.Submit(ctx, in)
	if e != nil {
		return nil, e
	}
	complete := false
	defer func() {
		if !complete {
			cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			_ = a.Cancel(cleanup, id)
		}
	}()
	for {
		state, e := a.Status(ctx, id)
		if e != nil {
			return nil, e
		}
		if state == "failed" {
			return nil, fmt.Errorf("BigQuery query failed")
		}
		if state == "succeeded" {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	var out *m.QueryResult
	token := ""
	size := 0
	seen := map[string]bool{}
	for {
		part, next, e := a.Page(ctx, id, token, min(1000, in.MaxRows+1))
		if e != nil {
			return nil, e
		}
		if out == nil {
			copy := *part
			out = &copy
			out.Rows = []map[string]any{}
		}
		for _, row := range part.Rows {
			if len(out.Rows) == in.MaxRows {
				out.Truncated = true
				break
			}
			raw, _ := json.Marshal(row)
			size += len(raw)
			if size > in.MaxBytes {
				return nil, fmt.Errorf("BigQuery result exceeds byte budget")
			}
			out.Rows = append(out.Rows, row)
		}
		if out.Truncated || next == "" {
			break
		}
		if seen[next] {
			return nil, fmt.Errorf("BigQuery cursor repeated")
		}
		seen[next] = true
		token = next
	}
	complete = true
	return out, nil
}

var _ m.AsyncQueryAdapter = (*bigQueryAdapter)(nil)
