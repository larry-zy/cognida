package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/service/agent/tools"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

const researchExcerpt = "The verified source contains at least eight matching characters."

type researchTestModel struct {
	model.BaseChatModel
	run func(context.Context, []*schema.Message) (*schema.Message, error)
}

func (m researchTestModel) Generate(ctx context.Context, msgs []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return m.run(ctx, msgs)
}

type researchTestRegistry map[string]tool.BaseTool

func (r researchTestRegistry) Get(name string) (tool.BaseTool, bool) { v, ok := r[name]; return v, ok }
func researchTestContext() context.Context {
	return agentctx.WithAllowedKBIDs(agentctx.NewAgentContext(context.Background(), "", 7, 9, "question"), []string{"allowed"})
}
func researchGoodDraft() string {
	b, _ := json.Marshal(researchDraft{Summary: "Summary [S1]", Analysis: "Analysis [S1]", Findings: []researchFinding{{Statement: "Supported finding", Citations: []researchCitation{{SourceID: "S1", Quote: researchExcerpt}}}}})
	return string(b)
}
func newResearchTestService(t *testing.T, empty bool, draft string) *ResearchService {
	t.Helper()
	llm := researchTestModel{run: func(ctx context.Context, msgs []*schema.Message) (*schema.Message, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		content := draft
		if strings.Contains(msgs[1].Content, "max_questions") {
			content = `{"questions":[{"question":"specific question","strategy":"hybrid"}]}`
		}
		return schema.AssistantMessage(content, nil), nil
	}}
	rag := tools.NewTypedBaseTool("rag_query", "retrieval", func(ctx context.Context, req *tools.RAGQueryRequest) (*tools.RAGQueryResult, error) {
		require.Equal(t, int64(7), agentctx.MustGetTenantID(ctx))
		require.Equal(t, []string{"allowed"}, agentctx.MustGetAllowedKBIDs(ctx))
		out := &tools.RAGQueryResult{}
		if !empty {
			out.Chunks = []tools.DocumentChunk{{Content: researchExcerpt, ChunkID: "c1", KnowledgeID: "d1", KnowledgeBaseID: "allowed", Source: "Source"}}
		}
		return out, nil
	})
	return NewResearchService(nil, nil).WithDependencies(llm, researchTestRegistry{"rag_query": rag})
}
func TestResearchReportAndSSETerminalEvidence(t *testing.T) {
	svc := newResearchTestService(t, false, researchGoodDraft())
	ch, err := svc.ExecuteStreamWithProgress(researchTestContext(), &DeepResearchRequest{Query: "question"})
	require.NoError(t, err)
	var last *ResearchProgressDTO
	stages := []string{}
	for p := range ch {
		last = p
		stages = append(stages, p.Stage)
	}
	require.Equal(t, []string{"decomposing", "researching", "evidence_collected", "synthesizing", "validating", "complete"}, stages)
	require.NotNil(t, last.Report)
	require.True(t, last.Report.Success)
	require.True(t, last.Report.Metadata.CitationChecked)
	require.False(t, last.Report.Metadata.FactChecked)
	require.Equal(t, "c1", last.Report.Sources[0].ChunkID)
	require.Equal(t, "allowed", last.Report.Sources[0].KnowledgeBaseID)
	require.Equal(t, "specific question", last.Report.SubQuestions[0])
	require.True(t, last.Report.CitationChecks[0].QuoteMatched)
}
func TestResearchRejectsNoEvidenceAndFakeCitations(t *testing.T) {
	report, err := newResearchTestService(t, true, researchGoodDraft()).Execute(researchTestContext(), &DeepResearchRequest{Query: "question"})
	require.ErrorIs(t, err, ErrResearchNoEvidence)
	require.False(t, report.Success)
	for _, draft := range []string{strings.ReplaceAll(researchGoodDraft(), "S1", "S404"), strings.ReplaceAll(researchGoodDraft(), researchExcerpt, "Invented evidence quote"), strings.ReplaceAll(researchGoodDraft(), "Supported finding", "Finding [S404]")} {
		report, err = newResearchTestService(t, false, draft).Execute(researchTestContext(), &DeepResearchRequest{Query: "question"})
		require.Error(t, err)
		require.False(t, report.Success)
		require.False(t, report.Metadata.CitationChecked)
	}
}
func TestResearchExcludesFallbackAndReportsPartialFailure(t *testing.T) {
	svc := newResearchTestService(t, false, researchGoodDraft())
	registry := svc.tools.(researchTestRegistry)
	registry["web_search"] = tools.NewTypedBaseTool("web_search", "search", func(context.Context, *map[string]any) (*map[string]any, error) {
		return &map[string]any{"is_fallback": true, "items": []any{map[string]any{"title": "Fake", "url": "https://example.com", "snippet": "Fabricated source"}}}, nil
	})
	report, err := svc.Execute(researchTestContext(), &DeepResearchRequest{Query: "question", Options: &DeepResearchOptions{EnableWebSearch: true}})
	require.NoError(t, err)
	require.True(t, report.Metadata.Partial)
	require.Len(t, report.Sources, 1)
	require.NotEmpty(t, report.Warnings)
	svc = newResearchTestService(t, true, researchGoodDraft())
	svc.tools.(researchTestRegistry)["web_search"] = registry["web_search"]
	_, err = svc.Execute(researchTestContext(), &DeepResearchRequest{Query: "question", Options: &DeepResearchOptions{EnableWebSearch: true}})
	require.ErrorIs(t, err, ErrResearchNoEvidence)
}
func TestResearchLimitsAuthCancellationAndTimeout(t *testing.T) {
	svc := newResearchTestService(t, false, researchGoodDraft())
	_, err := svc.Execute(context.Background(), &DeepResearchRequest{Query: "q"})
	require.ErrorIs(t, err, ErrResearchInvalid)
	for _, opts := range []*DeepResearchOptions{{MaxSubQuestions: 6}, {MaxConcurrency: 4}, {MaxSources: 31}, {TimeoutSeconds: 301}} {
		_, err = svc.Execute(researchTestContext(), &DeepResearchRequest{Query: "q", Options: opts})
		require.ErrorIs(t, err, ErrResearchInvalid)
	}
	svc.model = researchTestModel{run: func(ctx context.Context, _ []*schema.Message) (*schema.Message, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	ctx, cancel := context.WithCancel(researchTestContext())
	cancel()
	_, err = svc.Execute(ctx, &DeepResearchRequest{Query: "q"})
	require.ErrorIs(t, err, context.Canceled)
	ctx, cancel = context.WithTimeout(researchTestContext(), 10*time.Millisecond)
	defer cancel()
	_, err = svc.Execute(ctx, &DeepResearchRequest{Query: "q"})
	require.True(t, errors.Is(err, context.DeadlineExceeded))
}
