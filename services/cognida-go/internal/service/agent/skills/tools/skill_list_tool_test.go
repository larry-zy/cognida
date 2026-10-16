package tools

import (
	"context"
	"encoding/json"
	"testing"

	"cognida/internal/service/agent/skills"
	toolregistry "cognida/internal/service/agent/tools"
	"github.com/cloudwego/eino/components/tool"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDocumentSkillListMetadataFilters(t *testing.T) {
	manager := skills.NewSkillManager()
	for _, skill := range []*skills.Skill{
		{Name: "sql-guide", Description: "SQL guidance", Category: "data", Tags: []string{"sql", "analytics"}},
		{Name: "query-guide", Description: "Query guidance", Category: "data", Tags: []string{"sql"}},
		{Name: "analysis-guide", Description: "Analysis guidance", Category: "research", Tags: []string{"analytics"}},
	} {
		require.NoError(t, manager.GetRegistry().Register(skill))
	}
	l := &skillListTool{skillManager: manager}
	cases := []struct {
		input string
		names []string
	}{
		{`{"tags":"sql,analytics"}`, []string{"analysis-guide", "query-guide", "sql-guide"}},
		{`{"tags":"sql, analytics","tag_match":"all"}`, []string{"sql-guide"}},
		{`{"category":"data","tags":"analytics"}`, []string{"sql-guide"}},
		{`{"category":"research","tags":"sql"}`, []string{}},
		{`{"tags":"absent"}`, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			out, err := l.InvokableRun(context.Background(), tc.input)
			require.NoError(t, err)
			var result struct {
				Count  int                `json:"count"`
				Skills []skills.SkillInfo `json:"skills"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &result))
			names := make([]string, 0, len(result.Skills))
			for _, info := range result.Skills {
				names = append(names, info.Name)
				require.NotEmpty(t, info.Tags)
			}
			require.Equal(t, tc.names, names)
			require.Equal(t, len(tc.names), result.Count)
		})
	}
	for _, input := range []string{`null`, `{"tags":12}`, `{"category":false}`, `{"tags":"sql,,analytics"}`, `{"tag_match":"OR"}`, `{"tags":"sql","tags":"analytics"}`} {
		out, err := l.InvokableRun(context.Background(), input)
		require.Error(t, err)
		require.Empty(t, out)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := l.InvokableRun(ctx, `{}`)
	require.ErrorIs(t, err, context.Canceled)
}

// Mirror main's actual registration order: document Skill tools replace tool
// discovery entries. The inert GORM handle is construction-only; no SQL is run.
func TestProductionSkillRegistrationFiltersAndInvokes(t *testing.T) {
	reg, err := toolregistry.NewToolRegistry(toolregistry.ToolDeps{SQLDB: &gorm.DB{}})
	require.NoError(t, err)
	registerSkill(t, &skills.Skill{
		Name: "production-filtered-guide", Description: "Fixture guidance", Category: "test-filter",
		Tags: []string{"specific", "guide"}, Content: "Use the registered guidance.",
	})
	require.NoError(t, RegisterSkillTools(reg))
	require.NoError(t, RegisterSkillTools(reg))
	names := reg.ListToolsByGroup("skill")
	require.ElementsMatch(t, []string{"skill_list", "skill_invoke", "skill_match"}, names)
	list, ok := reg.Get("skill_list")
	require.True(t, ok)
	out, err := list.(tool.InvokableTool).InvokableRun(context.Background(), `{"category":"test-filter","tags":"specific,guide","tag_match":"all"}`)
	require.NoError(t, err)
	var result struct {
		Count  int                `json:"count"`
		Skills []skills.SkillInfo `json:"skills"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Count)
	require.Equal(t, "production-filtered-guide", result.Skills[0].Name)
	invoke, ok := reg.Get("skill_invoke")
	require.True(t, ok)
	out, err = invoke.(tool.InvokableTool).InvokableRun(context.Background(), `{"skill_name":"production-filtered-guide"}`)
	require.NoError(t, err)
	require.Contains(t, out, "Use the registered guidance.")
}
