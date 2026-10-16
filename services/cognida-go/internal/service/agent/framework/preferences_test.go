package framework

import (
	d "cognida/internal/model/agent"
	"context"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPrivateIterationCapControlsActualLoopWithoutMutatingAgent(t *testing.T) {
	for _, tc := range []struct{ cap, configured, want int }{{1, 4, 1}, {20, 2, 2}} {
		calls := []string{}
		m := &scriptedToolModel{script: []*schema.Message{toolCallMsg("a", "query"), toolCallMsg("b", "query"), toolCallMsg("c", "query")}}
		a := &agentImpl{name: "bounded", toolModel: m, maxIter: tc.configured, tools: []tool.BaseTool{&recordingTool{name: "query", calls: &calls}}}
		ctx := d.WithIterationLimit(context.Background(), tc.cap)
		response, err := a.Chat(ctx, "question")
		require.NoError(t, err)
		require.Len(t, calls, tc.want)
		require.NotNil(t, response)
		require.Equal(t, tc.configured, a.maxIter)
	}
}
