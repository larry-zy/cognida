//go:build integration

package handler

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestLabelStandardMySQLHTTPCASBindingAndIndependentUse(t *testing.T) {
	f := newReviewedFixture(t)
	NewLabelStandardHandler(f.gs).Register(f.router.Group("/api"))
	ctx := context.Background()
	in := d.LabelStandardInput{Name: "Retail", TaskType: "classification", Guidelines: "Only sales facts.", Labels: []d.StandardLabel{{Label: d.Label{ID: "sales", Name: "Sales", Keywords: []string{"sales"}}, Definition: "Sales facts", ExcludeKeywords: []string{"not sales"}}}, Constraints: d.StandardConstraints{MaxLabels: 1}}
	code, raw := f.request(t, "POST", "/label-standards", "create", in)
	require.Equal(t, 200, code)
	var head d.Meta
	require.NoError(t, json.Unmarshal(raw, &head))
	code, replay := f.request(t, "POST", "/label-standards", "create", in)
	require.Equal(t, 200, code)
	var same d.Meta
	require.NoError(t, json.Unmarshal(replay, &same))
	require.Equal(t, head.ID, same.ID)
	changed := in
	changed.Name = "Other"
	code, _ = f.request(t, "POST", "/label-standards", "create", changed)
	require.Equal(t, 409, code)
	code, raw = f.request(t, "POST", "/label-standards/"+head.ID+"/publish", "publish", map[string]any{"expected_version": 1})
	require.Equal(t, 200, code)
	var published d.LabelStandardVersion
	require.NoError(t, json.Unmarshal(raw, &published))
	require.NotEqual(t, head.ID, published.ID)
	code, raw = f.request(t, "POST", "/label-standards/"+head.ID+"/publish", "publish", map[string]any{"expected_version": 1})
	require.Equal(t, 200, code)
	require.Contains(t, string(raw), published.ID)
	code, _ = f.request(t, "PUT", "/label-standards/"+head.ID, "", map[string]any{"expected_version": 1, "standard": changed})
	require.Equal(t, 409, code)
	code, _ = f.request(t, "PUT", "/label-standards/"+head.ID, "", map[string]any{"expected_version": 2, "standard": changed})
	require.Equal(t, 200, code)
	code, raw = f.request(t, "GET", "/label-standard-versions/"+published.ID, "", nil)
	require.Equal(t, 200, code)
	require.Contains(t, string(raw), `"name":"Retail"`)
	code, _ = f.request(t, "GET", "/label-standard-versions/"+published.ID, "", nil, "X-Other-User")
	require.Equal(t, 404, code)
	f.assets.input = l.Input{Kind: "text", Text: "not sales"}
	taskInput := d.AnnotationTask{Name: "independent", Type: "classification", SourceRefID: f.assets.ref.AssetRefID, StandardVersionID: published.ID, StandardSnapshot: &d.LabelStandardInput{Name: "forged"}}
	task, e := f.gs.CreateAnnotation(ctx, f.p, taskInput, "annotation")
	require.NoError(t, e)
	var stored d.AnnotationTask
	require.NoError(t, json.Unmarshal(task.Payload, &stored))
	require.Equal(t, "Retail", stored.StandardSnapshot.Name)
	require.NoError(t, f.gs.Preannotate(ctx, f.p, task.ID))
	items, e := f.gs.List(ctx, f.p, d.Filter{Kind: "annotation_item", ParentID: task.ID, Limit: 10})
	require.NoError(t, e)
	require.Len(t, items.Items, 1)
	item := items.Items[0]
	var value d.AnnotationItem
	require.NoError(t, json.Unmarshal(item.Payload, &value))
	require.Empty(t, value.Annotation.Labels)
	require.Equal(t, "uncertain", value.Provenance.DecisionStatus)
	_, e = f.gs.ReviewAnnotation(ctx, f.p, task.ID, item.ID, "approved", "", item.Version)
	require.ErrorIs(t, e, d.ErrInvalid)
	_, e = f.gs.UpdateAnnotation(ctx, f.p, task.ID, item.ID, d.Annotation{Labels: []string{"sales"}}, item.Version)
	require.ErrorIs(t, e, d.ErrInvalid)
	taskInput.StandardVersionID = ""
	_, e = f.gs.CreateAnnotation(ctx, f.p, taskInput, "no-standard")
	require.ErrorIs(t, e, d.ErrInvalid)
	taskInput.StandardVersionID = published.ID
	f.assets.ref.Provenance.Preparation = nil
	_, e = f.gs.CreateAnnotation(ctx, f.p, taskInput, "not-prepared")
	require.ErrorIs(t, e, d.ErrInvalid)
}

func TestSFTMySQLHTTPDeclaredSchemaAndExactSnapshot(t *testing.T) {
	f := newReviewedFixture(t)
	NewSFTHandler(f.gs).Register(f.router.Group("/api"))
	standard := f.standard(t, "qa", nil)
	const amount = "9007199254740993.123456789012345678"
	const validArgs = `{"amount":9007199254740993.123456789012345678,"code":"00123","items":[{"name":"中文"}]}`
	const schema = `{"type":"object","properties":{"amount":{"type":"number","minimum":9007199254740993.123456789012345678,"maximum":9007199254740993.123456789012345679},"code":{"type":"string","enum":["00123"]},"items":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}}},"required":["amount","code","items"],"additionalProperties":false}`
	q, answer := "Recorded amount", amount
	setSource := func(args, parameters string) {
		messages := []d.SFTMessage{{Role: "user", Content: &q}, {Role: "assistant", ToolCalls: []d.SFTToolCall{{ID: "one", Type: "function", Function: d.SFTFunctionCall{Name: "lookup", Arguments: args}}}}, {Role: "tool", ToolCallID: "one", Content: &answer}, {Role: "assistant", Content: &answer}}
		tools := []d.SFTTool{{Type: "function", Function: d.SFTToolFunction{Name: "lookup", Parameters: json.RawMessage(parameters)}}}
		m, err := json.Marshal(messages)
		require.NoError(t, err)
		toolsJSON, err := json.Marshal(tools)
		require.NoError(t, err)
		f.assets.input = l.Input{Kind: "table", Columns: []string{"messages", "tools"}, Rows: [][]any{{string(m), string(toolsJSON)}}}
	}
	input := d.SFTInput{Name: "Schema-validated recorded tool calls", SourceRefID: f.assets.ref.AssetRefID, StandardVersionID: standard, Format: "messages", Columns: d.SFTColumns{Messages: "messages", Tools: "tools"}}
	setSource(validArgs, schema)
	code, raw := f.request(t, "POST", "/sft-datasets", "precise-schema", input)
	require.Equal(t, 200, code)
	require.NotContains(t, string(raw), "rows_exact")
	var head d.Meta
	require.NoError(t, json.Unmarshal(raw, &head))
	code, raw = f.request(t, "GET", "/sft-datasets/"+head.ID+"/rows", "", nil)
	require.Equal(t, 200, code)
	var rows d.Page[d.SFTRow]
	require.NoError(t, json.Unmarshal(raw, &rows))
	require.Equal(t, validArgs, rows.Items[0].Messages[1].ToolCalls[0].Function.Arguments)
	require.JSONEq(t, schema, string(rows.Items[0].Tools[0].Function.Parameters))
	require.Contains(t, string(rows.Items[0].Tools[0].Function.Parameters), amount)
	code, _ = f.request(t, "POST", "/sft-datasets/"+head.ID+"/review", "", map[string]any{"expected_version": 1, "decision": "approved"})
	require.Equal(t, 200, code)
	code, raw = f.request(t, "POST", "/sft-datasets/"+head.ID+"/publish", "precise-publish", map[string]any{"expected_version": 2})
	require.Equal(t, 200, code)
	var version d.Meta
	require.NoError(t, json.Unmarshal(raw, &version))
	exported, err := f.gs.ExportSFTDataset(context.Background(), f.p, version.ID)
	require.NoError(t, err)
	require.Contains(t, string(exported), `"minimum":`+amount)
	require.NotContains(t, string(exported), "rows_exact")
	for i, tc := range []struct{ args, schema string }{
		{strings.Replace(validArgs, `"amount":`+amount, `"amount":"private-value"`, 1), schema},
		{`{"code":"00123","items":[]}`, schema},
		{strings.Replace(validArgs, `"00123"`, `"other"`, 1), schema},
		{strings.Replace(validArgs, `"name":"中文"`, `"name":123`, 1), schema},
		{strings.Replace(validArgs, `"name":"中文"`, `"name":"中文","unexpected":true`, 1), schema},
		{validArgs, `{"type":"object","$ref":"https://private.invalid/schema"}`},
	} {
		setSource(tc.args, tc.schema)
		code, raw = f.request(t, "POST", "/sft-datasets", fmt.Sprintf("invalid-schema-%d", i), input)
		require.Equal(t, 400, code)
		require.NotContains(t, string(raw), "private-value")
		require.NotContains(t, string(raw), "private.invalid")
	}
	var count int64
	require.NoError(t, f.db.Table("governance_records").Where("tenant_id=? AND kind=?", f.p.TenantID, d.SFTDatasetKind).Count(&count).Error)
	require.EqualValues(t, 1, count)
	// A real stored snapshot mutation must still invalidate all reads.
	require.NoError(t, f.db.Exec("UPDATE governance_records SET payload=JSON_SET(payload,'$.rows_exact',?) WHERE tenant_id=? AND id=?", "[]", f.p.TenantID, head.ID).Error)
	code, _ = f.request(t, "GET", "/sft-datasets/"+head.ID, "", nil)
	require.Equal(t, 403, code)
}
func TestSFTMySQLHTTPExactReviewImmutableExportAndRevocation(t *testing.T) {
	f := newReviewedFixture(t)
	NewLabelStandardHandler(f.gs).Register(f.router.Group("/api"))
	NewSFTHandler(f.gs).Register(f.router.Group("/api"))
	standard := f.standard(t, "qa", nil)
	f.assets.input = l.Input{Kind: "table", Columns: []string{"q", "a", "split", "group"}, Rows: [][]any{{"金额？", "9007199254740993.123456789012345678", "train", "one"}, {"代码？", "00123", "test", "two"}}}
	in := d.SFTInput{Name: "source QA", SourceRefID: f.assets.ref.AssetRefID, StandardVersionID: standard, Format: "qa", Columns: d.SFTColumns{Question: "q", ReferenceAnswer: "a", Split: "split", Group: "group"}}
	code, raw := f.request(t, "POST", "/sft-datasets", "sft", in)
	require.Equal(t, 200, code)
	require.NotContains(t, string(raw), `"rows"`)
	var head d.Meta
	require.NoError(t, json.Unmarshal(raw, &head))
	code, replay := f.request(t, "POST", "/sft-datasets", "sft", in)
	require.Equal(t, 200, code)
	require.Contains(t, string(replay), head.ID)
	code, _ = f.request(t, "POST", "/sft-datasets/"+head.ID+"/publish", "early", map[string]any{"expected_version": 1})
	require.Equal(t, 409, code)
	code, raw = f.request(t, "GET", "/sft-datasets/"+head.ID+"/rows?page_size=1", "", nil)
	require.Equal(t, 200, code)
	var page d.Page[d.SFTRow]
	require.NoError(t, json.Unmarshal(raw, &page))
	require.True(t, page.HasMore)
	require.Equal(t, "9007199254740993.123456789012345678", *page.Items[0].Messages[1].Content)
	code, raw = f.request(t, "GET", "/sft-datasets/"+head.ID+"/rows?cursor="+page.NextCursor, "", nil)
	require.Equal(t, 200, code)
	require.NoError(t, json.Unmarshal(raw, &page))
	require.False(t, page.HasMore)
	require.Equal(t, "00123", *page.Items[0].Messages[1].Content)
	code, _ = f.request(t, "POST", "/sft-datasets/"+head.ID+"/review", "", map[string]any{"expected_version": 1, "decision": "approved", "comment": "Verified against source"})
	require.Equal(t, 200, code)
	code, raw = f.request(t, "POST", "/sft-datasets/"+head.ID+"/publish", "version", map[string]any{"expected_version": 2})
	require.Equal(t, 200, code)
	var version d.Meta
	require.NoError(t, json.Unmarshal(raw, &version))
	exported, e := f.gs.ExportSFTDataset(context.Background(), f.p, version.ID)
	require.NoError(t, e)
	require.Contains(t, string(exported), "9007199254740993.123456789012345678")
	require.Contains(t, string(exported), "00123")
	require.Contains(t, string(exported), `"split":"test"`)
	code, _ = f.request(t, "GET", "/sft-dataset-versions/"+version.ID+"/export", "", nil)
	require.Equal(t, 200, code)
	code, _ = f.request(t, "GET", "/sft-datasets/"+head.ID, "", nil, "X-Other-User")
	require.Equal(t, 404, code)
	f.assets.deny = true
	code, _ = f.request(t, "GET", "/sft-dataset-versions/"+version.ID+"/export", "", nil)
	require.Equal(t, 403, code)
	code, _ = f.request(t, "POST", "/sft-datasets", "sft", in)
	require.Equal(t, 403, code)
	f.assets.deny = false
	f.assets.input.Rows[1][0] = "金额？"
	code, _ = f.request(t, "POST", "/sft-datasets", "leak", in)
	require.Equal(t, 400, code)
	var count int64
	require.NoError(t, f.db.Table("governance_records").Where("tenant_id=? AND kind=?", f.p.TenantID, d.SFTDatasetKind).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestSFTMySQLHTTPToolMessagesValidationAndAdminBoundary(t *testing.T) {
	f := newReviewedFixture(t)
	NewSFTHandler(f.gs).Register(f.router.Group("/api"))
	NewLabelStandardHandler(f.gs).Register(f.router.Group("/api"))
	standard := f.standard(t, "qa", nil)
	messages := `[{"role":"user","content":"查00123"},{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"lookup","arguments":"{\"code\":\"00123\"}"}}]},{"role":"tool","tool_call_id":"c1","content":"9007199254740993.123456789012345678"},{"role":"assistant","content":"9007199254740993.123456789012345678"}]`
	tools := `[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"code":{"type":"string"}},"required":["code"],"additionalProperties":false}}}]`
	f.assets.input = l.Input{Kind: "table", Columns: []string{"messages", "tools"}, Rows: [][]any{{messages, tools}}}
	input := d.SFTInput{Name: "Recorded tool conversation", SourceRefID: f.assets.ref.AssetRefID, StandardVersionID: standard, Format: "messages", Columns: d.SFTColumns{Messages: "messages", Tools: "tools"}}
	code, raw := f.request(t, "POST", "/sft-datasets", "tool-source", input)
	require.Equal(t, 200, code)
	var head d.Meta
	require.NoError(t, json.Unmarshal(raw, &head))
	code, raw = f.request(t, "GET", "/sft-datasets/"+head.ID+"/rows", "", nil)
	require.Equal(t, 200, code)
	var rows d.Page[d.SFTRow]
	require.NoError(t, json.Unmarshal(raw, &rows))
	require.Equal(t, `{"code":"00123"}`, rows.Items[0].Messages[1].ToolCalls[0].Function.Arguments)
	before := f.gs.Authorize
	f.gs.Authorize = func(ctx context.Context, p l.Principal, scope string) error {
		if scope == "admin" {
			return l.ErrForbidden
		}
		return before(ctx, p, scope)
	}
	code, _ = f.request(t, "POST", "/sft-datasets/"+head.ID+"/review", "", map[string]any{"expected_version": 1, "decision": "approved"})
	require.Equal(t, 403, code)
	code, _ = f.request(t, "POST", "/sft-datasets/"+head.ID+"/publish", "unauthorized", map[string]any{"expected_version": 1})
	require.Equal(t, 403, code)
	f.gs.Authorize = before
	for i, bad := range []string{`null`, `[{"role":"user","Role":"assistant","content":"bad"}]`, `[{"role":"user","content":"q"},{"role":"tool","content":"a","tool_call_id":"unknown"}]`, `[{"role":"user","content":"q"},{"role":"assistant","content":"a","approved":true}]`} {
		f.assets.input.Rows[0][0] = bad
		code, _ = f.request(t, "POST", "/sft-datasets", fmt.Sprintf("bad-%d", i), input)
		require.Equal(t, 400, code)
	}
	var count int64
	require.NoError(t, f.db.Table("governance_records").Where("tenant_id=? AND kind=?", f.p.TenantID, d.SFTDatasetKind).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
