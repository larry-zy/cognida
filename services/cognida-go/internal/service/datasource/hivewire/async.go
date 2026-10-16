package hivewire

import (
	"context"
	"database/sql/driver"
	"fmt"
	h "github.com/beltran/gohive/v2/hiveserver"
	"time"
)

// Handles contain Hive session secrets. Callers must encrypt them at rest and
// never return them in API responses, traces or error messages.
type Handles struct {
	Session   *h.TSessionHandle   `json:"session"`
	Operation *h.TOperationHandle `json:"operation,omitempty"`
}

func Prepare(ctx context.Context, cfg Config) (*Handles, error) {
	c, e := (&connector{cfg}).Connect(ctx)
	if e != nil {
		return nil, e
	}
	conn := c.(*connection)
	if e = conn.transport.Close(); e != nil {
		return nil, fmt.Errorf("Hive session detach unavailable")
	}
	handles := &Handles{Session: conn.session}
	probe, e := restore(ctx, cfg, handles)
	if e != nil {
		return nil, e
	}
	defer probe.transport.Close()
	info, e := probe.client.GetInfo(ctx, &h.TGetInfoReq{SessionHandle: handles.Session, InfoType: h.TGetInfoType_CLI_DBMS_NAME})
	if e != nil || info == nil || check(info.Status) != nil {
		return nil, fmt.Errorf("Hive durable queries require hive.server2.close.session.on.disconnect=false and bounded idle session cleanup")
	}
	return handles, nil
}
func restore(ctx context.Context, cfg Config, handles *Handles) (*connection, error) {
	if handles == nil || handles.Session == nil || handles.Session.SessionId == nil {
		return nil, fmt.Errorf("Hive session handle invalid")
	}
	c, e := (&connector{cfg}).transport(ctx)
	if e != nil {
		return nil, e
	}
	c.session = handles.Session
	return c, nil
}
func Submit(ctx context.Context, cfg Config, handles *Handles, query string, timeout time.Duration) (*Handles, error) {
	if handles == nil || handles.Operation != nil || timeout <= 0 || timeout > 30*time.Second {
		return nil, fmt.Errorf("Hive prepared submission invalid")
	}
	c, e := restore(ctx, cfg, handles)
	if e != nil {
		return nil, e
	}
	defer c.transport.Close()
	r, e := c.client.ExecuteStatement(ctx, &h.TExecuteStatementReq{SessionHandle: handles.Session, Statement: query, RunAsync: true, QueryTimeout: int64((timeout + time.Second - 1) / time.Second)})
	if e != nil || r == nil || r.OperationHandle == nil || check(r.Status) != nil {
		return nil, fmt.Errorf("Hive submission outcome unknown; close prepared session")
	}
	return &Handles{Session: handles.Session, Operation: r.OperationHandle}, nil
}
func Status(ctx context.Context, cfg Config, handles *Handles) (string, error) {
	if handles == nil || handles.Operation == nil {
		return "", fmt.Errorf("Hive operation unavailable")
	}
	c, e := restore(ctx, cfg, handles)
	if e != nil {
		return "", e
	}
	defer c.transport.Close()
	r, e := c.client.GetOperationStatus(ctx, &h.TGetOperationStatusReq{OperationHandle: handles.Operation})
	if e != nil || r == nil || check(r.Status) != nil {
		return "", fmt.Errorf("Hive status unavailable")
	}
	switch r.GetOperationState() {
	case h.TOperationState_FINISHED_STATE:
		return "succeeded", nil
	case h.TOperationState_CANCELED_STATE:
		return "cancelled", nil
	case h.TOperationState_TIMEDOUT_STATE:
		return "timed_out", nil
	case h.TOperationState_INITIALIZED_STATE, h.TOperationState_RUNNING_STATE, h.TOperationState_PENDING_STATE:
		return "running", nil
	default:
		return "failed", nil
	}
}
func Close(ctx context.Context, cfg Config, handles *Handles) error {
	c, e := restore(ctx, cfg, handles)
	if e != nil {
		return e
	}
	defer c.transport.Close()
	if handles.Operation != nil {
		_, _ = c.client.CancelOperation(ctx, &h.TCancelOperationReq{OperationHandle: handles.Operation})
		_, _ = c.client.CloseOperation(ctx, &h.TCloseOperationReq{OperationHandle: handles.Operation})
	}
	r, e := c.client.CloseSession(ctx, &h.TCloseSessionReq{SessionHandle: handles.Session})
	if e != nil || r == nil {
		return fmt.Errorf("Hive session cleanup unavailable")
	}
	// Already-closed sessions are idempotent cleanup, but transport failures
	// remain observable so the caller can retry reconciliation.
	if r.Status != nil && r.Status.StatusCode == h.TStatusCode_INVALID_HANDLE_STATUS {
		return nil
	}
	return check(r.Status)
}

type PageResult struct {
	Columns []string
	Types   map[string]string
	Rows    []map[string]any
	More    bool
}

// Each request starts at the result beginning and reads a bounded prefix, so
// retrying a page cannot consume the following page. Servers may return less
// than MaxRows; HasMoreRows then requires another bounded FETCH_NEXT.
func Page(ctx context.Context, cfg Config, handles *Handles, offset, limit int) (*PageResult, error) {
	if handles == nil || handles.Operation == nil || offset < 0 || offset > 10000 || limit < 1 || limit > 1000 || offset+limit > 10001 {
		return nil, fmt.Errorf("Hive page budget invalid")
	}
	c, e := restore(ctx, cfg, handles)
	if e != nil {
		return nil, e
	}
	defer c.transport.Close()
	meta, e := c.client.GetResultSetMetadata(ctx, &h.TGetResultSetMetadataReq{OperationHandle: handles.Operation})
	if e != nil || meta == nil || meta.Schema == nil || check(meta.Status) != nil || len(meta.Schema.Columns) > 512 {
		return nil, fmt.Errorf("Hive schema unavailable")
	}
	out := &PageResult{Columns: []string{}, Types: map[string]string{}, Rows: []map[string]any{}}
	names := &rows{columns: meta.Schema.Columns}
	for i, col := range meta.Schema.Columns {
		if _, exists := out.Types[col.ColumnName]; exists {
			return nil, fmt.Errorf("Hive duplicate column")
		}
		out.Columns = append(out.Columns, col.ColumnName)
		out.Types[col.ColumnName] = names.ColumnTypeDatabaseTypeName(i)
	}
	values, e := fetchPrefix(ctx, c.client, handles.Operation, len(out.Columns), offset+limit+1)
	if e != nil {
		return nil, e
	}
	out.More = len(values) > offset+limit
	for i := offset; i < len(values) && i < offset+limit; i++ {
		row := map[string]any{}
		for col, v := range values[i] {
			if n, ok := v.(int64); ok {
				row[out.Columns[col]] = fmt.Sprint(n)
			} else {
				row[out.Columns[col]] = v
			}
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

type resultFetcher interface {
	FetchResults(context.Context, *h.TFetchResultsReq) (*h.TFetchResultsResp, error)
}

func fetchPrefix(ctx context.Context, client resultFetcher, operation *h.TOperationHandle, columns, want int) ([][]driver.Value, error) {
	values := [][]driver.Value{}
	orientation := h.TFetchOrientation_FETCH_FIRST
	for calls := 0; calls < 128; calls++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		r, err := client.FetchResults(ctx, &h.TFetchResultsReq{OperationHandle: operation, Orientation: orientation, MaxRows: int64(want - len(values))})
		if err != nil || r == nil || r.Results == nil || check(r.Status) != nil {
			return nil, fmt.Errorf("Hive page unavailable")
		}
		if r.Results.StartRowOffset != int64(len(values)) {
			return nil, fmt.Errorf("Hive result offset mismatch")
		}
		batch, err := decodeColumns(r.Results.Columns, columns)
		if err != nil {
			return nil, err
		}
		if len(batch) > want-len(values) {
			return nil, fmt.Errorf("Hive fetch exceeded row budget")
		}
		values = append(values, batch...)
		if len(values) >= want || !r.GetHasMoreRows() {
			return values, nil
		}
		if len(batch) == 0 {
			return nil, fmt.Errorf("Hive pagination did not advance")
		}
		orientation = h.TFetchOrientation_FETCH_NEXT
	}
	return nil, fmt.Errorf("Hive pagination exceeded call budget")
}
