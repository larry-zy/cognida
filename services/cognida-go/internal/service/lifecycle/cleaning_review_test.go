package lifecycle

import (
	d "cognida/internal/model/lifecycle"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCleaningDifferencesKeepExactValuesAndReasons(t *testing.T) {
	before := d.Input{Kind: "table", Columns: []string{"id", "amount"}, Rows: [][]any{{"00123", "9007199254740993.123456789012345678"}, {" a ", json.Number("9007199254740993")}}}
	after := d.Input{Kind: "table", Columns: before.Columns, Rows: [][]any{{"a", json.Number("9007199254740993")}}}
	report := CleaningReport{SourceRowIndices: []int{1}, Diagnostics: json.RawMessage(`{"row_changes":[{"source_row":0,"operations":["dedup"],"removed_by":"dedup"},{"source_row":1,"operations":["trim"]}]}`)}
	page, e := cleaningDiff(before, after, report, "", 1)
	require.NoError(t, e)
	require.Equal(t, "1", page.NextCursor)
	require.Equal(t, before.Rows[0], page.Items[0].Before)
	require.Nil(t, page.Items[0].After)
	require.Equal(t, "dedup", page.Items[0].RemovedBy)
	page, e = cleaningDiff(before, after, report, "1", 1)
	require.NoError(t, e)
	require.Equal(t, after.Rows[0], page.Items[0].After)
	require.Equal(t, 0, *page.Items[0].OutputRow)
	_, e = cleaningDiff(before, after, report, "-1", 50)
	require.Error(t, e)
	_, e = cleanOptions(map[string]any{"script": "print('not allowed')"})
	require.Error(t, e)
	_, e = cleanOptions(map[string]any{governedOption: "internal"})
	require.NoError(t, e)
	ref := d.DataAssetRef{RefID: d.RefID{AssetID: "original", Version: 1}, Provenance: d.Provenance{Digest: "source"}}
	one := cleaningDigest(ref, map[string]any{"keep": "first"})
	require.NotEqual(t, one, cleaningDigest(ref, map[string]any{"keep": "last"}))
	require.Equal(t, one, cleaningDigest(ref, map[string]any{"keep": "first", "preview_digest": one, strategyOption: "server-pin"}))
}
func TestCleaningTextLineageMatchesPythonSplitLines(t *testing.T) {
	_, rows := cleaningRows(d.Input{Kind: "text", Text: "一\r\n二\u2028三\n"})
	require.Equal(t, [][]any{{"一"}, {"二"}, {"三"}}, rows)
	_, e := cleaningDiff(d.Input{Kind: "text", Text: " "}, d.Input{Kind: "text", Text: ""}, CleaningReport{Diagnostics: json.RawMessage(`{"row_changes":[{"source_row":0,"operations":["drop_empty"],"removed_by":"drop_empty"}]}`)}, "", 50)
	require.NoError(t, e)
}
