package lifecycle

import (
	"encoding/json"
	"strings"
	"testing"

	d "cognida/internal/model/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestKnowledgeProjectionSelectsOnlyExplicitDocumentsAndPreservesSource(t *testing.T) {
	input := d.Input{Kind: "table", Columns: []string{"id", "contexts", "question", "answer"}, Rows: [][]any{
		{"doc-001", `[{"text":" line one\r\nline two\n","score":0.9},{"text":"Second document","answer":"DO NOT COPY"}]`, "held-out question", "held-out answer"},
		{json.Number("9007199254740993"), `[{"text":"Last document"}]`, "second question", "second answer"},
	}}
	opts := d.PreparationOptions{NormalizeNewlines: true, TextProjection: &d.PreparationTextProjection{Column: "contexts", TextField: "text", IDColumn: "id"}}
	out, err := StandardizeInput(input, opts)
	require.NoError(t, err)
	require.Equal(t, "text", out.Kind)
	require.Contains(t, out.Text, " line one\nline two\n")
	require.Contains(t, out.Text, `id "9007199254740993"`)
	require.Equal(t, 3, strings.Count(out.Text, "[Source row "))
	for _, excluded := range []string{"held-out", "second question", "second answer", "DO NOT COPY", "score"} {
		require.NotContains(t, out.Text, excluded)
	}
	require.Empty(t, out.Rows)
	require.Contains(t, input.Rows[0][1], `\r\n`, "input remains immutable")
}

func TestKnowledgeProjectionFailsWholeInputOnAmbiguousOrMissingDocuments(t *testing.T) {
	opts := d.PreparationOptions{TextProjection: &d.PreparationTextProjection{Column: "contexts", TextField: "text", IDColumn: "id"}}
	for _, raw := range []string{`null`, `[]`, `{}`, `["not an object"]`, `[{"text":null}]`, `[{"text":" "}]`, `[{"text":8}]`, `[{"other":"text"}]`, `[{"text":"first","text":"second"}]`, `[{"text":"ok"},{}]`, `[{"text":"ok"}] trailing`} {
		input := d.Input{Kind: "table", Columns: []string{"id", "contexts"}, Rows: [][]any{{"doc", raw}}}
		out, err := StandardizeInput(input, opts)
		require.ErrorIs(t, err, d.ErrInvalid, raw)
		require.Empty(t, out.Text, "never return a partial document corpus")
	}
	input := d.Input{Kind: "table", Columns: []string{"id", "contexts"}, Rows: [][]any{{"", `[{"text":"ok"}]`}}}
	_, err := StandardizeInput(input, opts)
	require.ErrorIs(t, err, d.ErrInvalid)
	opts.Fields = []d.PreparationField{{Source: "id", Name: "id", Type: "text"}}
	_, err = StandardizeInput(input, opts)
	require.ErrorIs(t, err, d.ErrInvalid, "table field mapping cannot silently be ignored")
}

func TestKnowledgeProjectionEnforcesDocumentAndOutputLimits(t *testing.T) {
	opts := d.PreparationOptions{TextProjection: &d.PreparationTextProjection{Column: "contexts", TextField: "text"}}
	items := make([]map[string]string, 257)
	for i := range items {
		items[i] = map[string]string{"text": "valid"}
	}
	encoded, _ := json.Marshal(items)
	input := d.Input{Kind: "table", Columns: []string{"contexts"}, Rows: [][]any{{string(encoded)}}}
	_, err := StandardizeInput(input, opts)
	require.ErrorIs(t, err, d.ErrInvalid)
	encoded, _ = json.Marshal([]map[string]string{{"text": strings.Repeat("x", 19000)}})
	input.Rows = nil
	for i := 0; i < 12; i++ {
		input.Rows = append(input.Rows, []any{string(encoded)})
	}
	_, err = StandardizeInput(input, opts)
	require.ErrorIs(t, err, d.ErrInvalid, "merged documents must fit the ordinary text-asset bound")
}
