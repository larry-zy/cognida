//go:build integration
// +build integration

// Package dataagent —— Group 9：自我修复 + 触发式再规划的真实 DB 端到端验收。
//
// 受 `integration` 构建标签门控。只需真实 MySQL（.env 的 DB_* 或 MYSQL_* 变量），
// 不需要 LLM——用「脚本化 ToolCallingChatModel」精确编排每一步工具调用，使自愈链路可确定断言：
// 工具层对真实库检索到的 available_columns/available_tables 线索确实回灌，且指挥官据此一次改对。
//
// 覆盖 tasks.md：
//   - 9.1 列名错一次自愈 / 表名错一次自愈（真实 information_schema 线索）
//   - 9.2 连续同错触发再规划（注入失败签名提示）并在无法完成时诚实部分收尾、不耗尽 maxIter
//
// 说明：瞬时错自动重试（9.1 第三子句）由 queryWithTransientRetry 在工具内闭环，难以对真实库确定性触发，
// 已在 Group 2 工具级单测覆盖；连续同错的护栏计数/收尾在 framework 层单测（eino_self_repair_test.go）
// 亦有精确断言。本文件补齐「真实库 schema 线索 + 单一 ReAct 内核」的端到端闭环。
package dataagent

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentctx "cognida/internal/model/agent"
	infraagent "cognida/internal/service/agent/framework"
	ragtool "cognida/internal/service/agent/tools"
)

// scriptedToolModel 是脚本化的 ToolCallingChatModel：按 Generate 调用序号回放预设消息，
// 越界后（steps 用尽）若设了 loop 则恒回 loop（用于「模型永不主动收尾、交护栏兜底」的收尾用例），
// 否则自然收尾（Content="done"）。每次 Generate 的输入快照留存，供断言「可修复观察是否回灌」。
type scriptedToolModel struct {
	steps  []*schema.Message   // 第 i 次 Generate 回放 steps[i]
	loop   *schema.Message     // steps 用尽后的兜底（nil 则自然收尾）
	inputs [][]*schema.Message // 每次 Generate 收到的输入快照（浅拷贝切片头，防被循环追加覆盖）
	calls  int
}

func (m *scriptedToolModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.inputs = append(m.inputs, append([]*schema.Message(nil), input...))
	idx := m.calls
	m.calls++
	if idx < len(m.steps) {
		return m.steps[idx], nil
	}
	if m.loop != nil {
		return m.loop, nil
	}
	return &schema.Message{Role: schema.Assistant, Content: "done"}, nil
}

func (m *scriptedToolModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](1)
	writer.Close()
	return reader, nil
}

func (m *scriptedToolModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

// sqlToolCall 构造一次 sql_execute 工具调用消息（Arguments 为合法 SQLExecuteRequest JSON）。
func sqlToolCall(sql string) *schema.Message {
	return &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID:       "call-1",
			Function: schema.FunctionCall{Name: "sql_execute", Arguments: `{"sql":` + jsonQuote(sql) + `}`},
		}},
	}
}

// jsonQuote 把字符串安全编码成 JSON 字面量（此处 SQL 无特殊转义需求，仅加引号）。
func jsonQuote(s string) string {
	b := strings.Builder{}
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// lastToolContent 取输入消息里最后一条 Tool 观察内容（即上一步工具回灌）。
func lastToolContent(msgs []*schema.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == schema.Tool {
			return msgs[i].Content
		}
	}
	return ""
}

// buildSelfRepairAgent 构造裸 ReAct 内核（仅 sql_execute + 脚本模型），
// 绕过 Data Agent 预设的意图路由/playbook 注入 Before 钩子，隔离自我修复主循环以便确定性断言。
func buildSelfRepairAgent(t *testing.T, reg *ragtool.ToolRegistry, tm model.ToolCallingChatModel, maxIter int) infraagent.Agent {
	sqlTool, ok := reg.Get("sql_execute")
	require.True(t, ok, "sql_execute 工具应已注册")

	agent, err := infraagent.New(nil).
		Name("SelfRepairE2E").
		Prompt("你是取数助手；工具失败时请阅读观察里的 error_kind/hint 定向改写。").
		WithToolModel(tm).
		Tools([]tool.BaseTool{sqlTool}...).
		WithMaxIterations(maxIter).
		Build(context.Background())
	require.NoError(t, err, "构建自我修复 E2E agent 失败")
	return agent
}

// selfRepairCtx 携租户/会话，供结果存储归属键与审计透传（结果存储未启用亦不影响取数）。
func selfRepairCtx() context.Context {
	ctx := agentctx.WithTenantID(context.Background(), 1)
	return agentctx.WithSessionID(ctx, "sess-self-repair-e2e")
}

// TestSelfRepairE2E_ColumnErrorOneShotHeal 校验（9.1 列名错一次自愈）：
// 模型先写错列名 → 工具对真实库检索 available_columns 回灌 → 模型据真实列名一次改对 → 端到端成功。
func TestSelfRepairE2E_ColumnErrorOneShotHeal(t *testing.T) {
	loadEnvForTest(t)
	db := setupDB(t)
	defer func() { sqlDB, _ := db.DB(); sqlDB.Close() }()
	setupSalesSchema(t, db)

	reg, err := ragtool.NewToolRegistry(ragtool.ToolDeps{SQLDB: db})
	require.NoError(t, err, "构造工具注册表失败")

	scripted := &scriptedToolModel{steps: []*schema.Message{
		sqlToolCall("SELECT nonexist_col FROM dataagent_sales LIMIT 5"), // 列名错
		sqlToolCall("SELECT region, amount FROM dataagent_sales LIMIT 5"), // 据线索改对
	}}
	agent := buildSelfRepairAgent(t, reg, scripted, 8)

	resp, err := agent.Chat(selfRepairCtx(), "查一下销售数据")
	require.NoError(t, err, "自愈闭环应成功收尾")
	assert.Equal(t, "done", resp.Content, "应自然收尾而非提前 wind-down")
	assert.NotEqual(t, infraagent.TerminatedByRepairExhausted, resp.Metadata["terminated_by"],
		"一次即改对，不应触发反复失败收尾")

	// 第 2 次 Generate 的输入应含首个失败的可修复观察：分级 unknown_column + 真实库列线索。
	require.GreaterOrEqual(t, len(scripted.inputs), 3, "应至少三次 Generate（错→观察→改对→收尾）")
	obs := lastToolContent(scripted.inputs[1])
	assert.Contains(t, obs, `"error_kind":"unknown_column"`, "应回灌 unknown_column 分级")
	assert.Contains(t, obs, "available_columns", "应携带真实库检索到的可用列线索")
	assert.Contains(t, obs, "region", "线索应含真实列 region")
	assert.Contains(t, obs, "amount", "线索应含真实列 amount")

	// 第 3 次 Generate 的输入应含改对后成功查询的真实数据（华东），证明端到端自愈成功。
	healed := lastToolContent(scripted.inputs[2])
	assert.Contains(t, healed, "华东", "改对后应取到真实数据")
	assert.NotContains(t, healed, `"error_kind"`, "成功观察不应再带错误分级")
}

// TestSelfRepairE2E_TableErrorOneShotHeal 校验（9.1 表名错一次自愈）：
// 模型先写错表名 → 工具检索 available_tables 候选回灌 → 模型据真实表名一次改对 → 成功。
func TestSelfRepairE2E_TableErrorOneShotHeal(t *testing.T) {
	loadEnvForTest(t)
	db := setupDB(t)
	defer func() { sqlDB, _ := db.DB(); sqlDB.Close() }()
	setupSalesSchema(t, db)

	reg, err := ragtool.NewToolRegistry(ragtool.ToolDeps{SQLDB: db})
	require.NoError(t, err, "构造工具注册表失败")

	scripted := &scriptedToolModel{steps: []*schema.Message{
		sqlToolCall("SELECT region, amount FROM dataagent_saless LIMIT 5"), // 表名错（多一个 s）
		sqlToolCall("SELECT region, amount FROM dataagent_sales LIMIT 5"),  // 据线索改对
	}}
	agent := buildSelfRepairAgent(t, reg, scripted, 8)

	resp, err := agent.Chat(selfRepairCtx(), "查一下销售数据")
	require.NoError(t, err, "自愈闭环应成功收尾")
	assert.Equal(t, "done", resp.Content, "应自然收尾")

	require.GreaterOrEqual(t, len(scripted.inputs), 3, "应至少三次 Generate")
	obs := lastToolContent(scripted.inputs[1])
	assert.Contains(t, obs, `"error_kind":"unknown_table"`, "应回灌 unknown_table 分级")
	assert.Contains(t, obs, "available_tables", "应携带候选表名线索")
	assert.Contains(t, obs, "dataagent_sales", "候选表应含近似真实表（按近似度置前）")

	healed := lastToolContent(scripted.inputs[2])
	assert.Contains(t, healed, "华东", "改对后应取到真实数据")
}

// TestSelfRepairE2E_RepeatedFailureReplanAndWindDown 校验（9.2）：
// 连续同错 → 越过 replan 阈值时注入失败签名再规划提示；仍不改 → 达 wind-down 阈值诚实部分收尾，
// 且远早于 maxIter（不空转耗尽迭代预算）。
func TestSelfRepairE2E_RepeatedFailureReplanAndWindDown(t *testing.T) {
	loadEnvForTest(t)
	db := setupDB(t)
	defer func() { sqlDB, _ := db.DB(); sqlDB.Close() }()
	setupSalesSchema(t, db)

	reg, err := ragtool.NewToolRegistry(ragtool.ToolDeps{SQLDB: db})
	require.NoError(t, err, "构造工具注册表失败")

	const maxIter = 20
	// 模型「不知悔改」：恒用同一错列名，交由自我修复护栏兜底（触发再规划 + 提前收尾）。
	bad := sqlToolCall("SELECT nonexist_col FROM dataagent_sales LIMIT 5")
	scripted := &scriptedToolModel{loop: bad}
	agent := buildSelfRepairAgent(t, reg, scripted, maxIter)

	resp, err := agent.Chat(selfRepairCtx(), "查一下销售数据")
	require.NoError(t, err, "反复失败应诚实收尾而非返回错误")

	// 诚实部分收尾。
	assert.Equal(t, infraagent.TerminatedByRepairExhausted, resp.Metadata["terminated_by"],
		"连续同错应提前进入 repair_exhausted 收尾")
	assert.Equal(t, true, resp.Metadata["partial"], "提前收尾应标记 partial")

	// 不空转到 maxIter：护栏在累计失败达阈值时即收尾，Generate 次数应远小于 maxIter。
	assert.Less(t, scripted.calls, maxIter, "应远早于 maxIter 收尾，不耗尽迭代预算")

	// 越过 replan 阈值后应注入含失败签名的再规划提示（引导换路径而非重试）。
	var replanInjected bool
	for _, in := range scripted.inputs {
		for _, msg := range in {
			if msg.Role == schema.System && strings.Contains(msg.Content, "sql_execute:unknown_column") {
				replanInjected = true
			}
		}
	}
	assert.True(t, replanInjected, "连续同错应注入含失败签名 sql_execute:unknown_column 的再规划提示")
}
