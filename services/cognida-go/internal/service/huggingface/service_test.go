package huggingface

import (
	a "cognida/internal/model/agent"
	d "cognida/internal/model/huggingface"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/service/collection/fetcher"
	"context"
	"encoding/json"
	"github.com/cloudwego/eino/components/tool"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type hubFunc func(context.Context, string, fetcher.JSONCredential, int64) (*fetcher.Result, error)

func (f hubFunc) FetchHuggingFace(c context.Context, u string, k fetcher.JSONCredential, n int64) (*fetcher.Result, error) {
	return f(c, u, k, n)
}
func TestMetadataCommitConfigGlobsAndIndependentSearch(t *testing.T) {
	p := l.Principal{TenantID: 1, UserID: 2}
	commit := strings.Repeat("a", 40)
	calls := 0
	service := &Service{authorize: func(context.Context, l.Principal, string) error { return nil }, fetch: hubFunc(func(_ context.Context, endpoint string, _ fetcher.JSONCredential, _ int64) (*fetcher.Result, error) {
		calls++
		var body string
		header := http.Header{}
		switch {
		case strings.Contains(endpoint, "/revision/"):
			body = `{"id":"org/data","sha":"` + commit + `","cardData":{"license":"cc-by-4.0","configs":[{"config_name":"zh-CN","data_files":[{"split":"test","path":"test/**/*.jsonl"}]}]},"siblings":[{"rfilename":"README.md"},{"rfilename":"test/zh.jsonl"},{"rfilename":"test/parts/b.jsonl"},{"rfilename":"test/loader.py"}]}`
		case strings.Contains(endpoint, "README.md"):
			body = "untrusted data card"
		default:
			body = `[{"id":"org/data","sha":"` + commit + `","gated":false}]`
			header.Set("Link", `<https://huggingface.co/api/datasets?cursor=next-token>; rel="next"`)
		}
		return &fetcher.Result{Body: []byte(body), Header: header}, nil
	})}
	info, e := service.Dataset(context.Background(), p, "org/data", "main", "")
	require.NoError(t, e)
	require.Equal(t, commit, info.Revision)
	require.Equal(t, []string{"test/parts/b.jsonl", "test/zh.jsonl"}, info.Configs[0].Splits[0].Files)
	require.False(t, info.Files[1].Supported)
	found, e := service.Search(context.Background(), p, "data", "", 10, "")
	require.NoError(t, e)
	require.Len(t, found.Items, 1)
	require.Equal(t, "next-token", found.NextCursor)
	require.False(t, service.Capabilities().(map[string]any)["decoder_ready"].(bool))
	require.True(t, service.Capabilities().(map[string]any)["search"].(bool))
	_, e = service.Preview(context.Background(), p, d.Request{})
	require.ErrorIs(t, e, d.ErrUnavailable)
	before := calls
	_, e = service.Dataset(context.Background(), l.Principal{}, "org/data", commit, "")
	require.ErrorIs(t, e, d.ErrForbidden)
	require.Equal(t, before, calls)
}
func TestHTTPDecoderPreservesNumbersAndUsesInternalAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/connectors/huggingface/decode", r.URL.Path)
		require.Equal(t, "Bearer synthetic-internal-token", r.Header.Get("Authorization"))
		var in map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&in))
		require.NotContains(t, in, "url")
		require.NotContains(t, in, "path")
		_ = json.NewEncoder(w).Encode(map[string]any{"columns": []string{"value"}, "column_types": map[string]string{"value": "DECIMAL"}, "source_columns": []any{map[string]string{"name": "value", "type": "DECIMAL"}}, "rows_json": "[[9007199254740993.123456789012345678]]", "total_rows": 1, "truncated": false})
	}))
	defer server.Close()
	decoder, e := NewHTTPDecoder(server.URL, "synthetic-internal-token")
	require.NoError(t, e)
	out, e := decoder.Decode(context.Background(), []byte(`{"value":1}`), "jsonl", "test.jsonl", 1, d.FieldMapping{Columns: []d.ColumnMapping{{Source: "value", Target: "value"}}})
	require.NoError(t, e)
	var rows [][]any
	require.NoError(t, decodeJSON([]byte(out.RowsEncoded), &rows))
	require.Equal(t, json.Number("9007199254740993.123456789012345678"), rows[0][0])
	_, e = NewHTTPDecoder(server.URL, "")
	require.ErrorIs(t, e, d.ErrUnavailable)
}
func TestIndependentToolsHaveSchemaAndRejectUnscopedExecution(t *testing.T) {
	calls := 0
	s := &Service{authorize: func(context.Context, l.Principal, string) error { calls++; return nil }, fetch: hubFunc(func(context.Context, string, fetcher.JSONCredential, int64) (*fetcher.Result, error) {
		return &fetcher.Result{Body: []byte("[]"), Header: http.Header{}}, nil
	})}
	all := NewTools(s)
	require.Len(t, all, 5)
	for _, t1 := range all {
		info, e := t1.Info(context.Background())
		require.NoError(t, e)
		require.NotNil(t, info.ParamsOneOf)
		_, e = t1.(tool.InvokableTool).InvokableRun(context.Background(), `{}`)
		require.ErrorIs(t, e, d.ErrForbidden)
	}
	require.Zero(t, calls)
	ctx := a.WithUserID(a.WithTenantID(context.Background(), 1), 2)
	response, e := all[0].(tool.InvokableTool).InvokableRun(ctx, `{"query":"data","limit":2}`)
	require.NoError(t, e)
	require.Contains(t, response, `"items":[]`)
	_, e = all[0].(tool.InvokableTool).InvokableRun(ctx, `{"query":"data","limit":2,"token":"not-accepted"}`)
	require.ErrorIs(t, e, d.ErrInvalid)
}
