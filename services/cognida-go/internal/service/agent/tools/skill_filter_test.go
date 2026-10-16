package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSkillListFiltersRegisteredMetadata(t *testing.T) {
	r := newSkillTestRegistry()
	require.NoError(t, r.RegisterWithTags("data", &mockInvokableTool{name: "sql_one"}, "sql", "analytics"))
	require.NoError(t, r.RegisterWithTags("data", &mockInvokableTool{name: "sql_two"}, "sql"))
	require.NoError(t, r.RegisterWithTags("other", &mockInvokableTool{name: "other"}, "analytics"))
	l, err := NewSkillListTool(r)
	require.NoError(t, err)
	cases := []struct {
		input string
		names []string
	}{
		{`{}`, []string{"other", "sql_one", "sql_two"}},
		{`{"category":"data"}`, []string{"sql_one", "sql_two"}},
		{`{"tags":"sql,analytics"}`, []string{"other", "sql_one", "sql_two"}},
		{`{"tags":"sql,analytics","tag_match":"all"}`, []string{"sql_one"}},
		{`{"category":"data","tags":"analytics","tag_match":"any"}`, []string{"sql_one"}},
		{`{"tags":" data, sql,sql ","tag_match":"all"}`, []string{"sql_one", "sql_two"}},
		{`{"tags":"SQL"}`, []string{}}, {`{"category":"absent"}`, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			out, err := l.InvokableRun(context.Background(), tc.input)
			require.NoError(t, err)
			var result struct {
				Count int `json:"count"`
				Tools []struct {
					Name     string   `json:"name"`
					Category string   `json:"category"`
					Tags     []string `json:"tags"`
				} `json:"tools"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &result))
			names := make([]string, 0, len(result.Tools))
			for _, entry := range result.Tools {
				names = append(names, entry.Name)
				require.Contains(t, entry.Tags, entry.Category)
			}
			require.Equal(t, tc.names, names)
			require.Equal(t, len(tc.names), result.Count)
		})
	}
	metadata, ok := r.ToolMetadata("sql_one")
	require.True(t, ok)
	metadata.Tags[0] = "mutated"
	metadata, _ = r.ToolMetadata("sql_one")
	require.NotContains(t, metadata.Tags, "mutated")
}

func TestSkillListRejectsIllegalFilters(t *testing.T) {
	invalid := []string{
		``, `null`, `[]`, `{"tags":`, `{} {}`, `{"unknown":"value"}`, `{"Tags":"sql"}`,
		`{"tags":null}`, `{"tags":["sql"]}`, `{"category":12}`, `{"tag_match":false}`,
		`{"tag_match":"or"}`, `{"tag_match":""}`, `{"tags":",sql"}`, `{"tags":"sql,"}`,
		`{"tags":"sql,,analytics"}`, `{"tags":"sql\nanalytics"}`, `{"tags":"sql","tags":"rag"}`,
		`{"tags":"sql","\u0074ags":"rag"}`, `{"category":{}}`,
		`{"tags":"` + strings.Repeat("x", 129) + `"}`, `{"tags":"` + strings.Repeat("x,", 32) + `x"}`,
		`{"category":"` + strings.Repeat("x", 16*1024) + `"}`,
	}
	l, err := NewSkillListTool(newSkillTestRegistry())
	require.NoError(t, err)
	for _, input := range invalid {
		out, err := l.InvokableRun(context.Background(), input)
		require.Error(t, err, "must reject invalid filter")
		require.Empty(t, out)
	}
	for _, input := range []string{`{}`, `{"tags":""}`, `{"category":""}`, `{"tags":"sql, analytics"}`, `{"tags":"质量,数据","tag_match":"all"}`} {
		_, err := ParseSkillListFilter(input)
		require.NoError(t, err)
	}
}

func TestToolRegistryMetadataReplacementAndInvocation(t *testing.T) {
	r := newSkillTestRegistry()
	require.NoError(t, r.RegisterWithTags("old", &mockInvokableTool{name: "registered", result: "first"}, "old-tag"))
	require.NoError(t, r.RegisterWithTags("new", &mockInvokableTool{name: "registered", result: "second"}, "new-tag"))
	require.NoError(t, r.RegisterWithTags("new", &mockInvokableTool{name: "registered", result: "second"}, "new-tag"))
	require.Empty(t, r.ListToolsByGroup("old"))
	require.Equal(t, []string{"registered"}, r.ListToolsByGroup("new"))
	metadata, _ := r.ToolMetadata("REGISTERED")
	require.Equal(t, []string{"new", "new-tag"}, metadata.Tags)
	invoke, err := NewSkillInvokeTool(r)
	require.NoError(t, err)
	out, err := invoke.InvokableRun(context.Background(), `{"task":"execute","tool_name":"registered"}`)
	require.NoError(t, err)
	require.Equal(t, "second", out)
	require.Error(t, r.RegisterWithTags("bad", &mockInvokableTool{name: "registered"}, "bad,tag"))
	metadata, _ = r.ToolMetadata("registered")
	require.Equal(t, "new", metadata.Category)
	require.NoError(t, r.Unregister("REGISTERED"))
	_, ok := r.ToolMetadata("registered")
	require.False(t, ok)
	require.Empty(t, r.ListGroups())
}
