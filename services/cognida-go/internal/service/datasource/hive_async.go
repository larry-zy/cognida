package datasource

import (
	m "cognida/internal/model/datasource"
	"cognida/internal/service/datasource/hivewire"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

type hiveAsyncAdapter struct {
	*sqlQueryAdapter
	config hivewire.Config
	cipher *Cipher
}
type hiveEnvelope struct {
	Tenant   int64             `json:"tenant"`
	Source   string            `json:"source"`
	Version  string            `json:"version"`
	Digest   string            `json:"digest"`
	Query    string            `json:"query"`
	MaxRows  int               `json:"max_rows"`
	MaxBytes int               `json:"max_bytes"`
	Deadline time.Time         `json:"deadline"`
	Handles  *hivewire.Handles `json:"handles"`
}

func hiveRequestDigest(req m.QueryRequest) string {
	raw, _ := json.Marshal(struct {
		Request m.QueryRequest
		Timeout int64
	}{req, req.Timeout.Nanoseconds()})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (a *hiveAsyncAdapter) Capabilities() m.QueryCapabilities {
	out := a.sqlQueryAdapter.Capabilities()
	out.Async = true
	out.Cancellation = "hiveserver2_cancel_operation_and_session"
	out.Pagination = "bounded_fetch_first_offset"
	return out
}
func (a *hiveAsyncAdapter) seal(h *hiveEnvelope) (string, error) {
	raw, e := json.Marshal(h)
	if e != nil {
		return "", e
	}
	return a.cipher.Encrypt(string(raw))
}
func (a *hiveAsyncAdapter) open(id string) (*hiveEnvelope, error) {
	if len(id) > 32768 {
		return nil, fmt.Errorf("Hive handle invalid")
	}
	raw, e := a.cipher.Decrypt(id)
	if e != nil {
		return nil, fmt.Errorf("Hive handle invalid")
	}
	var h hiveEnvelope
	if json.Unmarshal([]byte(raw), &h) != nil || h.Handles == nil || h.Handles.Session == nil || h.Tenant != a.source.TenantID || h.Source != a.source.ID || h.Version != a.source.UpdatedAt.UTC().Format(time.RFC3339Nano) || h.MaxRows < 1 || h.MaxRows > 10000 || h.MaxBytes < 1 || h.MaxBytes > 8<<20 {
		return nil, fmt.Errorf("Hive handle binding invalid")
	}
	return &h, nil
}
func (a *hiveAsyncAdapter) Prepare(ctx context.Context, req m.QueryRequest) (string, error) {
	if e := validateHiveRead(req); e != nil {
		return "", e
	}
	if len(req.OperationID) < 8 || len(req.OperationID) > 200 {
		return "", fmt.Errorf("Hive prepared query requires a durable operation ID")
	}
	handles, e := hivewire.Prepare(ctx, a.config)
	if e != nil {
		return "", e
	}
	query := fmt.Sprintf("SELECT * FROM (%s) AS cognida_bounded LIMIT %d", trimQuery(req.SQL), req.MaxRows+1)
	h := &hiveEnvelope{Tenant: a.source.TenantID, Source: a.source.ID, Version: a.source.UpdatedAt.UTC().Format(time.RFC3339Nano), Digest: hiveRequestDigest(req), Query: query, MaxRows: req.MaxRows, MaxBytes: req.MaxBytes, Deadline: time.Now().Add(req.Timeout), Handles: handles}
	id, e := a.seal(h)
	if e != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hivewire.Close(cleanup, a.config, handles)
	}
	return id, e
}
func (a *hiveAsyncAdapter) SubmitPrepared(ctx context.Context, prepared string, req m.QueryRequest) (string, error) {
	h, e := a.open(prepared)
	if e != nil {
		return "", e
	}
	if h.Handles.Operation != nil || h.Digest != hiveRequestDigest(req) || time.Now().After(h.Deadline) {
		return "", fmt.Errorf("Hive prepared request expired or differs")
	}
	ctx, cancel := context.WithDeadline(ctx, h.Deadline)
	defer cancel()
	handles, e := hivewire.Submit(ctx, a.config, h.Handles, h.Query, time.Until(h.Deadline))
	if e != nil {
		return "", e
	}
	h.Handles = handles
	return a.seal(h)
}

// Direct callers must use Prepare + durable fencing + SubmitPrepared. There is
// no honest idempotent Submit for HiveServer2 without a persisted session.
func (a *hiveAsyncAdapter) Submit(context.Context, m.QueryRequest) (string, error) {
	return "", fmt.Errorf("Hive requires persisted Prepare and SubmitPrepared")
}
func (a *hiveAsyncAdapter) Status(ctx context.Context, id string) (string, error) {
	h, e := a.open(id)
	if e != nil {
		return "", e
	}
	return hivewire.Status(ctx, a.config, h.Handles)
}
func (a *hiveAsyncAdapter) AbandonPrepared(ctx context.Context, id string) error {
	h, e := a.open(id)
	if e != nil {
		return e
	}
	return hivewire.Close(ctx, a.config, h.Handles)
}
func (a *hiveAsyncAdapter) Cancel(ctx context.Context, id string) error {
	return a.AbandonPrepared(ctx, id)
}
func (a *hiveAsyncAdapter) Page(ctx context.Context, id, cursor string, limit int) (*m.QueryResult, string, error) {
	h, e := a.open(id)
	if e != nil {
		return nil, "", e
	}
	offset := 0
	if cursor != "" {
		offset, e = strconv.Atoi(cursor)
		if e != nil || strconv.Itoa(offset) != cursor {
			return nil, "", fmt.Errorf("Hive cursor invalid")
		}
	}
	if offset < 0 || offset > h.MaxRows || limit < 1 || limit > 1000 {
		return nil, "", fmt.Errorf("Hive page outside budget")
	}
	limit = min(limit, h.MaxRows+1-offset)
	part, e := hivewire.Page(ctx, a.config, h.Handles, offset, limit)
	if e != nil {
		return nil, "", e
	}
	size := 0
	for _, row := range part.Rows {
		raw, _ := json.Marshal(row)
		size += len(raw)
		if size > h.MaxBytes {
			return nil, "", fmt.Errorf("Hive page exceeds byte budget")
		}
	}
	next := ""
	if part.More && offset+len(part.Rows) < h.MaxRows+1 {
		next = strconv.Itoa(offset + len(part.Rows))
	}
	out := &m.QueryResult{Columns: part.Columns, ColumnTypes: part.Types, Rows: part.Rows, ExecutedSQL: h.Query, SourceVersion: h.Version, Truncated: part.More && next == ""}
	return out, next, nil
}

var _ m.PreparedAsyncQueryAdapter = (*hiveAsyncAdapter)(nil)
