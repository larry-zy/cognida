//go:build integration
// +build integration

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentuc "link/internal/service/agent"
	infraagent "link/internal/infrastructure/adapter/agent"
	"link/internal/service/agent/tools"
	"link/internal/model/common"
	"link/internal/infrastructure/llm/chat"
)

// mockRAGQueryTool 模拟 RAG 查询工具。
type mockRAGQueryTool struct{}

func (m *mockRAGQueryTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "rag_query",
		Desc: "RAG 知识库查询工具",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "查询内容",
				Required: true,
			},
			"top_k": {
				Type:     schema.Integer,
				Desc:     "返回结果数量",
				Required: false,
			},
		}),
	}, nil
}

func (m *mockRAGQueryTool) InvokableRun(ctx context.Context, argsIn string) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsIn), &args); err != nil {
		return "", err
	}

	query := args["query"].(string)
	return fmt.Sprintf("RAG 查询结果：关于 '%s' 的知识库内容", query), nil
}

// mockGraphQueryTool 模拟图谱查询工具。
type mockGraphQueryTool struct{}

func (m *mockGraphQueryTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "graph_query",
		Desc: "知识图谱查询工具",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "图谱查询语句",
				Required: true,
			},
		}),
	}, nil
}

func (m *mockGraphQueryTool) InvokableRun(ctx context.Context, argsIn string) (string, error) {
	return "图谱查询结果：找到 3 个相关节点", nil
}

// mockWebSearchTool 模拟网络搜索工具。
type mockWebSearchTool struct{}

func (m *mockWebSearchTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "web_search",
		Desc: "网络搜索工具",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "搜索关键词",
				Required: true,
			},
		}),
	}, nil
}

func (m *mockWebSearchTool) InvokableRun(ctx context.Context, argsIn string) (string, error) {
	return "网络搜索结果：找到 5 条相关内容", nil
}

// TestRAG_RegisterTools tests manually registering RAG tools.
func TestRAG_RegisterTools(t *testing.T) {
	registry := tools.GetDefaultRegistry()

	// 手动注册模拟工具
	mockRAG := &mockRAGQueryTool{}
	mockGraph := &mockGraphQueryTool{}
	mockSearch := &mockWebSearchTool{}

	registry.Register("rag_query", mockRAG)
	registry.Register("graph_query", mockGraph)
	registry.Register("web_search", mockSearch)

	// 验证工具已注册
	assert.Equal(t, 3, registry.Count())

	// 列出工具
	names := registry.List()
	assert.Len(t, names, 3)
	t.Logf("已注册工具: %v", names)

	// 获取工具
	tool, ok := registry.Get("rag_query")
	assert.True(t, ok)
	assert.NotNil(t, tool)

	// 获取工具信息
	info, err := tool.Info(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "rag_query", info.Name)
	t.Logf("工具信息: %s - %s", info.Name, info.Desc)
}

// TestRAG_WithMockTools tests agent with mock RAG tools.
func TestRAG_WithMockTools(t *testing.T) {
	registry := tools.GetDefaultRegistry()

	// 注册模拟工具
	registry.Register("rag_query", &mockRAGQueryTool{})
	registry.Register("web_search", &mockWebSearchTool{})

	// 使用集成层的适配器
	adapter := 	infraagent.NewToolRegistryAdapter(registry)

	// 获取所有工具
	allTools := adapter.GetTools()
	t.Logf("适配器获取工具数量: %d", len(allTools))
	assert.GreaterOrEqual(t, len(allTools), 2)

	// 验证工具信息
	for _, baseTool := range allTools {
		info, err := baseTool.Info(context.Background())
		require.NoError(t, err)
		t.Logf("工具: %s - %s", info.Name, info.Desc)

		// 测试工具调用 - mock tools implement InvokableRun
		args := fmt.Sprintf(`{"query": "test query"}`)
		mockTool, ok := baseTool.(interface {
			InvokableRun(ctx context.Context, argsIn string) (string, error)
		})
		if ok {
			output, err := mockTool.InvokableRun(context.Background(), args)
			require.NoError(t, err)
			t.Logf("  调用结果: %s", output)
		}
	}
}

// TestRAG_ToolWithRealLLM tests RAG tool with real LLM.
func TestRAG_ToolWithRealLLM(t *testing.T) {
	// 创建模型
	model := createToolModelForTest(t)
	registry := tools.GetDefaultRegistry()

	// 注册模拟 RAG 工具
	registry.Register("rag_query", &mockRAGQueryTool{})

	// 获取工具
	ragTool, ok := registry.Get("rag_query")
	require.True(t, ok, "RAG 工具应该已注册")

	// 创建带工具的 Agent
	agent, err := agentuc.New(nil).
		Name("rag-agent").
		Prompt("你是一个助手，可以使用 RAG 查询工具获取知识库信息。").
		WithToolModel(model).
		Tools(ragTool).
		Build(context.Background())
	require.NoError(t, err)

	ctx := context.Background()
	resp, err := agent.Chat(ctx, "查询一下 LinkGo 的架构信息")

	require.NoError(t, err)
	t.Logf("RAG Agent 响应: %s", resp.Content)

	if len(resp.ToolCalls) > 0 {
		t.Logf("工具调用: %s", resp.ToolCalls[0].Name)
		t.Logf("工具输出: %s", resp.ToolCalls[0].Output)
	}
}

// TestRAG_MultipleRAGTools tests multiple RAG tools with real LLM.
func TestRAG_MultipleRAGTools(t *testing.T) {
	model := createToolModelForTest(t)
	registry := tools.GetDefaultRegistry()

	// 注册多个 RAG 相关工具
	registry.Register("rag_query", &mockRAGQueryTool{})
	registry.Register("graph_query", &mockGraphQueryTool{})
	registry.Register("web_search", &mockWebSearchTool{})

	// 获取所有工具
	allTools := registry.GetTools()

	// 创建带多个工具的 Agent
	agent, err := agentuc.New(nil).
		Name("multi-rag-agent").
		Prompt("你是一个助手，可以使用各种 RAG 工具获取信息。").
		WithToolModel(model).
		Tools(allTools...).
		Build(context.Background())
	require.NoError(t, err)

	ctx := context.Background()
	resp, err := agent.Chat(ctx, "搜索人工智能相关信息")

	require.NoError(t, err)
	t.Logf("多工具 RAG Agent 响应: %s", resp.Content)
	t.Logf("工具调用次数: %d", len(resp.ToolCalls))
}

// TestRAG_ToolInfoStructure tests tool information structure.
func TestRAG_ToolInfoStructure(t *testing.T) {
	mockTool := &mockRAGQueryTool{}

	info, err := mockTool.Info(context.Background())
	require.NoError(t, err)

	// 验证必需字段
	assert.NotEmpty(t, info.Name)
	assert.NotEmpty(t, info.Desc)

	t.Logf("工具信息:")
	t.Logf("  名称: %s", info.Name)
	t.Logf("  描述: %s", info.Desc)

	// 检查参数信息
	if info.ParamsOneOf != nil {
		jsonSchema, err := info.ParamsOneOf.ToJSONSchema()
		require.NoError(t, err)
		t.Logf("  参数 JSON Schema: %+v", jsonSchema)
	}
}

// TestRAG_ToolAdapter tests tool adapter with real registry.
func TestRAG_ToolAdapter(t *testing.T) {
	registry := tools.GetDefaultRegistry()

	// 注册模拟工具
	registry.Register("rag_query", &mockRAGQueryTool{})

	// 创建适配器
	adapter := 	infraagent.NewToolRegistryAdapter(registry)

	// 使用 WithRegistry
	agent, err := agentuc.New(nil).
		Name("adapter-agent").
		Prompt("你是一个助手。").
		WithToolModel(createToolModelForTest(t)).
		WithRegistry(adapter).
		ToolsAutoSelect().
		Build(context.Background())
	require.NoError(t, err)

	t.Logf("使用工具注册表适配器的 Agent 创建成功")
	assert.NotNil(t, agent)
}

// createToolModelForTest 创建测试用的模型。
func createToolModelForTest(t *testing.T) model.ToolCallingChatModel {
	apiKey := "sk-S0T25bBKxUgWSFtqB0C08462E4Ce4801A5399a2eD7B84a39"
	baseURL := "https://api.deepseek.com/v1"

	config := &chat.ChatConfig{
		Source:    common.ModelSourceRemote,
		APIKey:    apiKey,
		ModelName: "gpt-3.5-turbo",
		Provider:  "openai",
		BaseURL:   baseURL,
	}

	model, err := chat.NewToolCallingChatModel(context.Background(), config)
	require.NoError(t, err)
	return model
}
