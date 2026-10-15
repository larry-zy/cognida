package tools

import (
	agentctx "cognida/internal/model/agent"
	"context"
	"encoding/json"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
	"testing"
)

type optimizationTestModel struct{ model.BaseChatModel }

func (optimizationTestModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return schema.AssistantMessage(`{"intermediate_answer":"partial","should_continue":true,"next_query":"second question"}`, nil), nil
}

type optimizationTestRAG struct {
	t     *testing.T
	calls int
}

func (s *optimizationTestRAG) Query(ctxAny interface{}, req *RAGQueryRequest) (*RAGQueryResult, error) {
	ctx := ctxAny.(context.Context)
	require.Equal(s.t, int64(7), agentctx.MustGetTenantID(ctx))
	require.Equal(s.t, int64(9), agentctx.MustGetUserID(ctx))
	require.Equal(s.t, []string{"allowed"}, agentctx.MustGetAllowedKBIDs(ctx))
	s.calls++
	return &RAGQueryResult{Chunks: []DocumentChunk{{Content: "Actual source evidence", KnowledgeBaseID: "allowed", ChunkID: "c1"}}, Count: 1, HasAnswer: true, Warnings: []string{"one unavailable source"}}, nil
}
func TestRAGOptimizationEveryRetrievalRetainsTrustedScope(t *testing.T) {
	for _, index := range []int{0, 1, 2} {
		svc := &optimizationTestRAG{t: t}
		opt := NewRAGOptimizationTools(svc, optimizationTestModel{})[index].(tool.InvokableTool)
		_, err := opt.InvokableRun(context.Background(), `{"query":"q"}`)
		require.Error(t, err)
		require.Zero(t, svc.calls)
		ctx := agentctx.WithAllowedKBIDs(agentctx.NewAgentContext(context.Background(), "", 7, 9, "q"), []string{"allowed"})
		raw, err := opt.InvokableRun(ctx, `{"query":"q","max_hops":2,"tenant_id":999,"kb_ids":["foreign"]}`)
		require.NoError(t, err)
		var result RAGOptimizationResult
		require.NoError(t, json.Unmarshal([]byte(raw), &result))
		require.Len(t, result.Chunks, 1)
		require.Equal(t, "allowed", result.Chunks[0].KnowledgeBaseID)
		require.NotEmpty(t, result.Warnings)
		if index == 2 {
			require.Equal(t, 2, svc.calls)
		} else {
			require.Equal(t, 1, svc.calls)
		}
	}
}

func TestRAGOptimizationPublishesCallableSchema(t *testing.T) {
	for _, base := range NewRAGOptimizationTools(nil, nil) {
		info, err := base.Info(context.Background())
		require.NoError(t, err)
		require.NotNil(t, info.ParamsOneOf)
		parameters, err := info.ParamsOneOf.ToJSONSchema()
		require.NoError(t, err)
		raw, err := json.Marshal(parameters)
		require.NoError(t, err)
		for _, field := range []string{`"query"`, `"top_k"`, `"max_hops"`} {
			require.Contains(t, string(raw), field)
		}
		require.NotContains(t, string(raw), `"tenant_id"`)
		require.NotContains(t, string(raw), `"kb_ids"`)
	}
}
