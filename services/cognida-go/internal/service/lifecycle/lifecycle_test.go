package lifecycle

import (
	d "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContractsAndTemplate(t *testing.T) {
	c := Contracts()
	def := DefaultTemplate()
	require.NoError(t, def.Validate(c))
	for _, id := range []string{"data-analysis", "data-quality"} {
		for _, tool := range c[id].ToolAllowlist {
			require.NotContains(t, []string{"sql_mutate", "etl_run", "data_export", "asset.publish"}, tool)
		}
	}
	def.Stages[0].DependsOn = []string{"evaluation"}
	require.Error(t, def.Validate(c))
	def = DefaultTemplate()
	def.Stages[1].AgentID = "data-analysis"
	require.Error(t, def.Validate(c))
}
func TestRefRoundTrip(t *testing.T) {
	id := d.RefID{AssetID: uuid.NewString(), Version: 2}
	parsed, e := d.ParseRefID(id.String())
	require.NoError(t, e)
	require.Equal(t, id, parsed)
	_, e = d.ParseRefID("../../secret:1")
	require.Error(t, e)
	_, e = d.ParseRefID(id.AssetID + ":0")
	require.Error(t, e)
	raw, e := json.Marshal(d.DataAssetRef{RefID: id, ObjectKey: "must-not-leak"})
	require.NoError(t, e)
	require.NotContains(t, string(raw), "must-not-leak")
}
func TestHTTPComputeAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		require.Equal(t, "request-1", r.Header.Get("X-Request-ID"))
		require.Equal(t, "/api/v1/lifecycle/profile", r.URL.Path)
		_, _ = w.Write([]byte(`{"schema_version":"1","method":"rules","result":{"overall_score":90}}`))
	}))
	defer server.Close()
	c := NewHTTPCompute(server.URL, "test-token")
	result, e := c.Call(context.Background(), "profile", d.Input{Kind: "text", Text: "hello"}, nil, "request-1")
	require.NoError(t, e)
	require.Equal(t, json.Number("90"), result.Result["overall_score"])
	_, e = c.Call(context.Background(), "../../admin", d.Input{Kind: "text", Text: "hello"}, nil, "")
	require.Error(t, e)
}
func TestOptionsNoAuthority(t *testing.T) {
	for _, key := range []string{"approved", "is_admin", "tenant_id", "tools", "stages"} {
		require.Error(t, validateOptions(map[string]any{key: true}))
	}
}

func TestInputBoundsAndDescriptiveCalculation(t *testing.T) {
	require.Error(t, (d.Input{Kind: "table", Columns: []string{"a"}, Rows: [][]any{{map[string]any{"nested": true}}}}).Validate())
	require.Error(t, (d.Input{Kind: "table", Columns: []string{"a"}, Rows: [][]any{}}).Validate())
	out, e := AnalyzeInput(d.Input{Kind: "table", Columns: []string{"gmv"}, Rows: [][]any{{float64(100)}, {float64(200)}, {nil}}})
	require.NoError(t, e)
	columns := out["columns"].([]map[string]any)
	require.Equal(t, float64(150), columns[0]["mean"])
	require.Equal(t, 1, columns[0]["missing_count"])
	require.Equal(t, 2, columns[0]["numeric_count"])
}

func TestAuditOutputOptionalAcrossLifecycleStates(t *testing.T) {
	for _, status := range []string{d.StatusWaiting, d.StatusQueued, d.StatusRunning, d.StatusApproval, d.StatusFailed, d.StatusCancelled, d.StatusQuarantined} {
		t.Run(status, func(t *testing.T) {
			details := map[string]any{"status": status}
			require.NotPanics(t, func() { appendAuditOutput(details, nil) })
			require.NotContains(t, details, "gate")
			require.Equal(t, []string{}, details["output_refs"])
			require.Equal(t, status, details["status"])
		})
	}
	ref := d.DataAssetRef{RefID: d.RefID{AssetID: uuid.NewString(), Version: 1}}
	details := map[string]any{}
	appendAuditOutput(details, &d.Output{Gate: d.GateResult{Status: d.GatePassed}, Refs: []d.DataAssetRef{ref}})
	require.Equal(t, []string{ref.RefID.String()}, details["output_refs"])
}

func TestTableInputPreservesExactJSONNumbers(t *testing.T) {
	input := d.Input{Kind: "table", Columns: []string{"value"}, Rows: [][]any{{json.Number("9007199254740993")}}}
	require.NoError(t, input.Validate())
	raw, e := json.Marshal(input)
	require.NoError(t, e)
	require.Contains(t, string(raw), "9007199254740993")
	for _, value := range []json.Number{"NaN", "null", "1e999999999", "1/2"} {
		input.Rows[0][0] = value
		require.Error(t, input.Validate())
	}
}

func TestExactInputDecodeAndStatistics(t *testing.T) {
	var input d.Input
	require.NoError(t, json.Unmarshal([]byte(`{"kind":"table","columns":["id","value"],"rows":[[9007199254740993,10],[9007199254740995,20]]}`), &input))
	require.Equal(t, json.Number("9007199254740993"), input.Rows[0][0])
	output, err := AnalyzeInput(input)
	require.NoError(t, err)
	stats := output["columns"].([]map[string]any)
	require.Equal(t, 2, stats[1]["numeric_count"])
	require.Equal(t, float64(15), stats[1]["mean"])
	var computed ComputeResult
	require.NoError(t, json.Unmarshal([]byte(`{"result":{"data":{"kind":"table","columns":["id"],"rows":[[9007199254740993]]}}}`), &computed))
	raw, err := json.Marshal(computed)
	require.NoError(t, err)
	require.Contains(t, string(raw), "9007199254740993")
}
