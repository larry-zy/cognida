// Package hivewire provides the context-aware database/sql bridge used by
// Cognida. The upstream v2.1 driver has legacy []driver.Value QueryContext
// signatures, so it cannot provide database/sql's cancellation contract.
package hivewire

import (
	"context"
	"crypto/tls"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"github.com/apache/thrift/lib/go/thrift"
	gohive "github.com/beltran/gohive/v2"
	h "github.com/beltran/gohive/v2/hiveserver"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host                               string
	Port                               int
	Username, Password, Database, Auth string
	TLS                                bool
}
type Driver struct{}
type connector struct{ config Config }
type connection struct {
	client    *h.TCLIServiceClient
	transport thrift.TTransport
	session   *h.TSessionHandle
}

func init() { sql.Register("cognida_hive", Driver{}) }
func (Driver) Open(dsn string) (driver.Conn, error) {
	c, e := (Driver{}).OpenConnector(dsn)
	if e != nil {
		return nil, e
	}
	return c.Connect(context.Background())
}
func (Driver) OpenConnector(dsn string) (driver.Connector, error) {
	var c Config
	if json.Unmarshal([]byte(dsn), &c) != nil {
		return nil, fmt.Errorf("invalid Hive connection")
	}
	return &connector{c}, nil
}
func (*connector) Driver() driver.Driver { return Driver{} }
func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.transport(ctx)
	if err != nil {
		return nil, err
	}
	cfg := c.config
	response, e := conn.client.OpenSession(ctx, &h.TOpenSessionReq{ClientProtocol: h.TProtocolVersion_HIVE_CLI_SERVICE_PROTOCOL_V6, Username: &cfg.Username, Password: &cfg.Password, Configuration: map[string]string{"use:database": cfg.Database, "hive.resultset.use.unique.column.names": "false"}})
	if e != nil || response == nil || check(response.Status) != nil || response.SessionHandle == nil {
		conn.transport.Close()
		return nil, fmt.Errorf("Hive session unavailable")
	}
	conn.session = response.SessionHandle
	return conn, nil
}
func (c *connector) transport(ctx context.Context) (*connection, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	cfg := c.config
	if cfg.Host == "" || cfg.Port < 1 || cfg.Port > 65535 || cfg.Username == "" || cfg.Database == "" {
		return nil, fmt.Errorf("Hive identity and database required")
	}
	if cfg.Auth != "NONE" && cfg.Auth != "NOSASL" && cfg.Auth != "LDAP" && cfg.Auth != "CUSTOM" {
		return nil, fmt.Errorf("unsupported Hive authentication")
	}
	if (cfg.Auth == "LDAP" || cfg.Auth == "CUSTOM") && !cfg.TLS {
		return nil, fmt.Errorf("Hive password authentication requires TLS")
	}
	conf := &thrift.TConfiguration{ConnectTimeout: 5 * time.Second, SocketTimeout: 3 * time.Second, MaxMessageSize: 16 << 20, MaxFrameSize: 16 << 20}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	var socket thrift.TTransport
	if cfg.TLS {
		conf.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: cfg.Host}
		socket = thrift.NewTSSLSocketConf(addr, conf)
	} else {
		socket = thrift.NewTSocketConf(addr, conf)
	}
	if e := socket.Open(); e != nil {
		return nil, fmt.Errorf("Hive connection unavailable")
	}
	var transport thrift.TTransport
	if cfg.Auth == "NOSASL" {
		transport = thrift.NewTBufferedTransport(socket, 4096)
	} else {
		password := cfg.Password
		if password == "" {
			password = "x"
		}
		var e error
		transport, e = gohive.NewTSaslTransport(socket, cfg.Host, "PLAIN", map[string]string{"username": cfg.Username, "password": password}, 16<<20)
		if e != nil {
			socket.Close()
			return nil, fmt.Errorf("Hive SASL authentication failed")
		}
	}
	if !transport.IsOpen() {
		if e := transport.Open(); e != nil {
			transport.Close()
			return nil, fmt.Errorf("Hive transport unavailable")
		}
	}
	client := h.NewTCLIServiceClientFactory(transport, thrift.NewTBinaryProtocolFactoryConf(conf))
	return &connection{client: client, transport: transport}, nil
}

func check(s *h.TStatus) error {
	if s == nil || (s.StatusCode != h.TStatusCode_SUCCESS_STATUS && s.StatusCode != h.TStatusCode_SUCCESS_WITH_INFO_STATUS) {
		return fmt.Errorf("Hive operation rejected")
	}
	return nil
}
func (c *connection) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = c.client.CloseSession(ctx, &h.TCloseSessionReq{SessionHandle: c.session})
	return c.transport.Close()
}
func (*connection) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("Hive prepared parameters unsupported")
}
func (*connection) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("Hive transactions unsupported")
}
func (c *connection) Ping(ctx context.Context) error {
	r, e := c.QueryContext(ctx, "SELECT 1", nil)
	if e != nil {
		return e
	}
	return r.Close()
}
func (c *connection) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	upper := strings.ToUpper(strings.TrimSpace(query))
	if !strings.HasPrefix(upper, "SELECT ") && !strings.HasPrefix(upper, "SELECT\n") && !strings.HasPrefix(upper, "WITH ") && !strings.HasPrefix(upper, "SHOW TABLES IN ") && !strings.HasPrefix(upper, "DESCRIBE ") {
		return nil, fmt.Errorf("Hive connection only accepts reads")
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if len(args) > 0 {
		return nil, fmt.Errorf("Hive bound parameters unsupported")
	}
	response, e := c.client.ExecuteStatement(ctx, &h.TExecuteStatementReq{SessionHandle: c.session, Statement: query, RunAsync: true, QueryTimeout: 30})
	if e != nil || response == nil {
		return nil, fmt.Errorf("Hive submission unavailable")
	}
	if e = check(response.Status); e != nil {
		return nil, e
	}
	if response.OperationHandle == nil {
		return nil, fmt.Errorf("missing Hive operation handle")
	}
	op := response.OperationHandle
	cleanup := func() {
		clean, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = c.client.CancelOperation(clean, &h.TCancelOperationReq{OperationHandle: op})
		_, _ = c.client.CloseOperation(clean, &h.TCloseOperationReq{OperationHandle: op})
	}
	for {
		if e = ctx.Err(); e != nil {
			cleanup()
			return nil, e
		}
		status, err := c.client.GetOperationStatus(ctx, &h.TGetOperationStatusReq{OperationHandle: op})
		if err != nil || status == nil {
			cleanup()
			return nil, fmt.Errorf("Hive status unavailable")
		}
		if e = check(status.Status); e != nil {
			cleanup()
			return nil, e
		}
		switch status.GetOperationState() {
		case h.TOperationState_FINISHED_STATE:
			meta, err := c.client.GetResultSetMetadata(ctx, &h.TGetResultSetMetadataReq{OperationHandle: op})
			if err != nil || meta == nil || meta.Schema == nil || check(meta.Status) != nil {
				cleanup()
				return nil, fmt.Errorf("Hive schema unavailable")
			}
			if len(meta.Schema.Columns) > 512 {
				cleanup()
				return nil, fmt.Errorf("Hive column limit exceeded")
			}
			return &rows{conn: c, ctx: ctx, op: op, columns: meta.Schema.Columns}, nil
		case h.TOperationState_INITIALIZED_STATE, h.TOperationState_RUNNING_STATE, h.TOperationState_PENDING_STATE:
		default:
			cleanup()
			return nil, fmt.Errorf("Hive query did not complete")
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			cleanup()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

type rows struct {
	conn    *connection
	ctx     context.Context
	op      *h.TOperationHandle
	columns []*h.TColumnDesc
	batch   [][]driver.Value
	index   int
	closed  bool
}

func (r *rows) Columns() []string {
	out := make([]string, len(r.columns))
	for i, c := range r.columns {
		out[i] = c.ColumnName
	}
	return out
}
func (r *rows) ColumnTypeDatabaseTypeName(i int) string {
	c := r.columns[i]
	if c.TypeDesc == nil || len(c.TypeDesc.Types) != 1 || c.TypeDesc.Types[0].PrimitiveEntry == nil {
		return "UNSUPPORTED"
	}
	return strings.TrimSuffix(c.TypeDesc.Types[0].PrimitiveEntry.Type.String(), "_TYPE")
}
func (r *rows) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if r.ctx.Err() != nil {
		_, _ = r.conn.client.CancelOperation(ctx, &h.TCancelOperationReq{OperationHandle: r.op})
	}
	_, e := r.conn.client.CloseOperation(ctx, &h.TCloseOperationReq{OperationHandle: r.op})
	return e
}
func (r *rows) Next(dest []driver.Value) error {
	if e := r.ctx.Err(); e != nil {
		return e
	}
	if r.closed {
		return io.EOF
	}
	if r.index >= len(r.batch) {
		result, e := r.conn.client.FetchResults(r.ctx, &h.TFetchResultsReq{OperationHandle: r.op, Orientation: h.TFetchOrientation_FETCH_NEXT, MaxRows: 256})
		if e != nil || result == nil || result.Results == nil {
			return fmt.Errorf("Hive page unavailable")
		}
		if e = check(result.Status); e != nil {
			return e
		}
		r.batch, e = decodeColumns(result.Results.Columns, len(r.columns))
		if e != nil {
			return e
		}
		r.index = 0
		if len(r.batch) == 0 {
			return io.EOF
		}
	}
	copy(dest, r.batch[r.index])
	r.index++
	return nil
}
func decodeColumns(columns []*h.TColumn, width int) ([][]driver.Value, error) {
	if len(columns) == 0 {
		return nil, nil
	}
	if len(columns) != width {
		return nil, fmt.Errorf("Hive page schema mismatch")
	}
	var out [][]driver.Value
	for index, c := range columns {
		var values []driver.Value
		var nulls []byte
		switch {
		case c.StringVal != nil:
			nulls = c.StringVal.Nulls
			for _, v := range c.StringVal.Values {
				values = append(values, v)
			}
		case c.BoolVal != nil:
			nulls = c.BoolVal.Nulls
			for _, v := range c.BoolVal.Values {
				values = append(values, v)
			}
		case c.ByteVal != nil:
			nulls = c.ByteVal.Nulls
			for _, v := range c.ByteVal.Values {
				values = append(values, int64(v))
			}
		case c.I16Val != nil:
			nulls = c.I16Val.Nulls
			for _, v := range c.I16Val.Values {
				values = append(values, int64(v))
			}
		case c.I32Val != nil:
			nulls = c.I32Val.Nulls
			for _, v := range c.I32Val.Values {
				values = append(values, int64(v))
			}
		case c.I64Val != nil:
			nulls = c.I64Val.Nulls
			for _, v := range c.I64Val.Values {
				values = append(values, v)
			}
		case c.DoubleVal != nil:
			nulls = c.DoubleVal.Nulls
			for _, v := range c.DoubleVal.Values {
				values = append(values, v)
			}
		default:
			return nil, fmt.Errorf("unsupported Hive result type")
		}
		if len(values) > 256 {
			return nil, fmt.Errorf("Hive page limit exceeded")
		}
		if index == 0 {
			out = make([][]driver.Value, len(values))
			for i := range out {
				out[i] = make([]driver.Value, width)
			}
		} else if len(out) != len(values) {
			return nil, fmt.Errorf("Hive inconsistent column length")
		}
		for i, v := range values {
			if i/8 < len(nulls) && (nulls[i/8]&(1<<uint(i%8))) != 0 {
				v = nil
			}
			out[i][index] = v
		}
	}
	return out, nil
}

var _ driver.DriverContext = Driver{}
var _ driver.QueryerContext = (*connection)(nil)
var _ driver.Pinger = (*connection)(nil)
