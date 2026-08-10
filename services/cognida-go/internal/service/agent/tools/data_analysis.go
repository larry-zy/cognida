// Package tools 提供 data_analysis 工具：通过 MCP 调用 Python analytics 引擎
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	agentctx "cognida/internal/model/agent"
	modeltools "cognida/internal/model/agent/tools"
	"cognida/internal/service/agent/resultstore"
)

// ========================================
// data_analysis 工具 - 量化数据分析
// ========================================

// analysis_type→MCP 工具名映射、schema Enum 与白名单均由 contract.go 的单一真源
// analysisTypeMappings 派生（见 analysisTypeToMCPTool / analysisTypeEnum），此处不再重复字面量。

// MCPInvoker 抽象 MCP 调用能力，便于注入真实客户端或单测 mock。
//
// 该接口仅依赖 model 层类型（SkillInvokeResult），因此 service 工具层
// 无需直接 import infrastructure/mcp，符合 handler → service → model ←
// infrastructure 的依赖方向。真实实现由 infrastructure/mcp.MCPClient 提供，
// 并在组合根（cmd/server）通过 InitDataAnalysisTool 注入。
type MCPInvoker interface {
	Invoke(ctx interface{}, skillName string, params map[string]interface{}) (*modeltools.SkillInvokeResult, error)
}

// dataAnalysisTool data_analysis 工具实现
//
// 取数（sql_execute）之后，把行集交给 Python analytics 引擎做真正的
// 描述统计 / 趋势 / 异常 / 相关性 / 综合洞察 / 对比 / 归因，产出有计算依据的结论。
// 复用既有 MCP 通道（注入的 MCPInvoker → infrastructure/mcp.MCPClient.Invoke）。
type dataAnalysisTool struct {
	invoker        MCPInvoker                   // 注入的 MCP 调用器；nil 时返回非致命错误结果
	resultStore    resultstore.Store            // 注入的结果存储；nil 时无法按 result_id 取数
	sessionResults *resultstore.SessionRegistry // 会话取数注册表；缺 result_id 时据其回退到最近取数
}

// NewDataAnalysisTool 创建 data_analysis 工具；invoker MCP 调用器（可为 nil）、rs 结果存储（可为 nil）、
// reg 会话取数注册表（可为 nil）经参数注入。
func NewDataAnalysisTool(invoker MCPInvoker, rs resultstore.Store, reg *resultstore.SessionRegistry) (tool.InvokableTool, error) {
	return &dataAnalysisTool{invoker: invoker, resultStore: rs, sessionResults: reg}, nil
}

// Info 返回工具信息（含参数 schema）
func (t *dataAnalysisTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "data_analysis",
		Desc: `对取数得到的行集做量化数据分析，产出有计算依据的结论与行动建议。

适用场景：text2sql / sql_execute 取数之后，需要真正的统计计算而非凭空叙述时调用。

analysis_type 取值（命名能力）：
- trend: 趋势 — 时间序列线性趋势、预测与增长率（环比/同比/CAGR），需 options.value_col
- comparison: 对比 — 分组聚合排名与占比，恰两组时附显著性检验，需 options.value_col、options.dim_col
- attribution: 归因/根因 — 可加方差分解 + 驱动排序 + 隐藏因子提示，需 options.value_col、options.period_col，可选 options.dim_col（缺省自动扫描最能解释变化的维度）
- report: 报告解读 — 综合汇总趋势/异常/相关性并给出 recommendations

底层原子分析：
- describe: 描述统计（均值/中位数/分位数/标准差/偏度峰度 + 正态性）
- anomaly: 异常点检测（IQR/zscore），需 options.value_col
- correlation: 多数值列相关性矩阵与显著相关对（需 ≥2 数值列）
- insight: 综合洞察（与 report 等价）

数据来源二选一：
- result_id: 引用 sql_execute 返回的结果引用（推荐，避免大数据重复传输）
- data: 行集，形如 {"columns": [...], "rows": [[...], ...]} 或 records 数组

归因（attribution）成功时额外返回：drivers 表的新 result_id、文字洞察 insight、
口径 caliber、置信 confidence、下钻建议 drill_down。`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"analysis_type": {
				Type:     schema.String,
				Desc:     "分析类型：trend / comparison / attribution / report / describe / anomaly / correlation / insight",
				Required: true,
				Enum:     analysisTypeEnum, // 由 contract.go 单一真源 analysisTypeMappings 派生
			},
			"result_id": {
				Type:     schema.String,
				Desc:     "（与 data 二选一）sql_execute 返回的结果引用，工具自动按引用取回行集",
				Required: false,
			},
			"data": {
				Type:     schema.Object,
				Desc:     "（与 result_id 二选一）要分析的行集，{columns, rows} 或 records 数组",
				Required: false,
			},
			"options": {
				Type:     schema.Object,
				Desc:     "（可选）分析参数，如 value_col、time_col、period_col、dim_col、baseline、current、columns、method、threshold、forecast_steps 等",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun 执行数据分析
func (t *dataAnalysisTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	// 解析参数
	var arguments map[string]interface{}
	if err := json.Unmarshal([]byte(argumentsInJSON), &arguments); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// analysis_type 必填且需在白名单内（缺参属调用方错误，走非致命结果供 Agent 自纠）
	analysisType, _ := arguments["analysis_type"].(string)
	if analysisType == "" {
		return failResult("", "缺少必填参数 analysis_type"), nil
	}
	mcpTool, ok := analysisTypeToMCPTool[analysisType]
	if !ok {
		// 未知类型属调用方错误，返回非致命错误结果供 Agent 纠正
		return failResult(analysisType, fmt.Sprintf("不支持的 analysis_type: %s", analysisType)), nil
	}

	// 数据来源：result_id 引用（data-by-reference）优先，其次内联 data
	data := arguments["data"]
	sourceResultID, _ := arguments["result_id"].(string)
	// 委派带外句柄：模型既没在参数里显式给 result_id、也没内联 data 时，采用委派信封钉进 childCtx
	// 的确切句柄（指挥官为本次委派选定的那一个，非「最近一次」猜测）。句柄经带外通道抵达，无需
	// 模型从委派提示文本里转述回填；比 ① 会话最近取数回退更精确，故置于其前。显式参数仍优先。
	if sourceResultID == "" && data == nil {
		sourceResultID = agentctx.MustGetDelegatedResultID(ctx)
	}
	if sourceResultID != "" {
		if t.resultStore == nil {
			return failResult(analysisType, "结果存储未启用，无法按 result_id 取数，请直接传 data"), nil
		}
		owner := resultstore.OwnerKey(agentctx.MustGetTenantID(ctx), agentctx.MustGetSessionID(ctx))
		stored, err := t.resultStore.Get(ctx, owner, sourceResultID)
		if errors.Is(err, resultstore.ErrNotFound) {
			return failResult(analysisType, fmt.Sprintf("result_id %s 不存在或已过期，请重新取数", sourceResultID)), nil
		}
		if errors.Is(err, resultstore.ErrUnauthorized) {
			return failResult(analysisType, fmt.Sprintf("result_id %s 不属于当前会话", sourceResultID)), nil
		}
		if err != nil {
			return failResult(analysisType, fmt.Sprintf("读取结果存储失败: %v", err)), nil
		}
		// records 数组形态，Python 侧 records_to_df 直接可用
		data = stored.Rows
	}
	// ① 会话取数回退：既无内联 data、无显式 result_id、也无委派 pin 时，从本会话已取数结果里挑一份。
	// 收敛「取数→分析」委派链在 result_id 未串联时的 dead-end（Analysis 空转找不到数据）。但**不盲取
	// latest**——多份候选时静默取「最近一次」可能取错数据集（正是要避免的「之前的情况」）：
	//   · 恰好 1 份 live 取数 → 无歧义，直接取回；
	//   · ≥2 份           → 宁拒不猜：非致命消歧，列出候选交回模型/指挥官显式指定其一（一次有界回合，
	//                        failureGuard 封顶重复，不会兜圈子）；
	//   · 0 份 live（签名索引空）→ 退到 Latest 兜底，覆盖 sql 签名为空的历史取数，保持既有兜底不回退。
	if data == nil && sourceResultID == "" && t.sessionResults != nil && t.resultStore != nil {
		owner := resultstore.OwnerKey(agentctx.MustGetTenantID(ctx), agentctx.MustGetSessionID(ctx))
		switch live := t.sessionResults.LiveFetches(owner); {
		case len(live) >= 2:
			return failResult(analysisType, "本会话存在多份可用取数结果，无法确定分析对象，请在参数 result_id 显式指定其一"+
				"（委派场景由指挥官在信封 inputs.result_id 指定）。候选："+t.describeCandidates(ctx, owner, live)), nil
		case len(live) == 1:
			sourceResultID = live[0]
		default:
			if latestID, ok := t.sessionResults.Latest(owner); ok {
				sourceResultID = latestID
			}
		}
		if sourceResultID != "" {
			if stored, gerr := t.resultStore.Get(ctx, owner, sourceResultID); gerr == nil {
				data = stored.Rows // sourceResultID 保留，让归因信封的 drill_down 能引用来源
			} else {
				sourceResultID = "" // 取回失败则回到统一的「缺来源」非致命提示
			}
		}
	}
	if data == nil {
		// 缺数据来源同属调用方错误：返回非致命结果让 Agent 补传 data 或 result_id
		return failResult(analysisType, "缺少数据来源：data 与 result_id 必须二选一，且本会话尚无可复用的取数结果"), nil
	}

	// MCP 调用器需已注入
	if t.invoker == nil {
		return failResult(analysisType, "MCP 调用器未初始化（请在组合根调用 InitDataAnalysisTool）"), nil
	}

	// 组装 MCP 工具参数：data + 展开的 options
	params := map[string]interface{}{"data": data}
	if rawOpts, ok := arguments["options"].(map[string]interface{}); ok {
		for k, v := range rawOpts {
			params[k] = v
		}
	}

	// 通过既有 MCP 通道调用 Python analytics 工具
	result, err := t.invoker.Invoke(ctx, string(mcpTool), params)
	if err != nil {
		return failResult(analysisType, fmt.Sprintf("分析调用失败: %v", err)), nil
	}

	// 透传结果，附带 analysis_type 便于结论生成定位
	out := map[string]interface{}{
		"analysis_type": analysisType,
		"success":       result.Success,
	}
	if result.Data != nil {
		out["data"] = result.Data
	}
	if result.Error != "" {
		out["error"] = result.Error
	}

	// 归因成功：拼装一等信封（drivers 落 Result Store + 文字洞察 + 口径/置信/下钻）
	if analysisType == string(AnalysisAttribution) && result.Success && result.Data != nil {
		t.enrichAttributionEnvelope(ctx, out, result.Data, sourceResultID)
	}

	resultBytes, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}
	return string(resultBytes), nil
}

// describeCandidates 为「多份候选」消歧提示列出每个 result_id 的简述（列 + 行数），best-effort：
// 单个候选读取失败只列 id，不因个别读失败中断整体提示；并对最近一次取数加注，便于模型快速确认。
func (t *dataAnalysisTool) describeCandidates(ctx context.Context, owner string, ids []string) string {
	latestID, _ := t.sessionResults.Latest(owner)
	var b strings.Builder
	for _, id := range ids {
		b.WriteString("\n- ")
		b.WriteString(id)
		if stored, err := t.resultStore.Get(ctx, owner, id); err == nil {
			cols := strings.Join(stored.Columns, ",")
			if len(cols) > 80 {
				cols = cols[:80] + "…"
			}
			b.WriteString(fmt.Sprintf("（列: %s / %d 行）", cols, len(stored.Rows)))
		}
		if id == latestID {
			b.WriteString(" [最近一次取数]")
		}
	}
	return b.String()
}

// ========================================
// 归因信封拼装（任务 4a.3）
// ========================================

// attributionDriverColumns drivers 表落 Result Store 的固定列序
var attributionDriverColumns = []string{
	"segment", "baseline", "current", "delta",
	"contribution_pct", "share_of_change", "direction",
}

// enrichAttributionEnvelope 把 Python 归因结果拼装成一等信封：
//   - drivers 表写入 Result Store，返回新 result_id（可继续 render_ui / 再分析）
//   - insight：Go 侧确定性文字洞察（不依赖 LLM 复述数字）
//   - caliber / confidence 提升到顶层，保证口径与置信标注不被裁剪
//   - drill_down：下钻校验建议（引用取数来源 result_id）
func (t *dataAnalysisTool) enrichAttributionEnvelope(ctx context.Context, out map[string]interface{}, data map[string]interface{}, sourceResultID string) {
	// 口径与置信直接上提
	if caliber, ok := data["caliber"]; ok {
		out["caliber"] = caliber
	}
	if confidence, ok := data["confidence"]; ok {
		out["confidence"] = confidence
	}

	// drivers 表落 Result Store → 新 result_id
	drivers, _ := data["drivers"].([]interface{})
	rows := make([]map[string]interface{}, 0, len(drivers))
	for _, item := range drivers {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		row := make(map[string]interface{}, len(attributionDriverColumns))
		for _, c := range attributionDriverColumns {
			row[c] = m[c]
		}
		rows = append(rows, row)
	}
	if t.resultStore != nil && len(rows) > 0 {
		driverResult := &resultstore.Result{
			Owner:   resultstore.OwnerKey(agentctx.MustGetTenantID(ctx), agentctx.MustGetSessionID(ctx)),
			Columns: attributionDriverColumns,
			Rows:    rows,
		}
		if id, err := t.resultStore.Put(ctx, driverResult, resultstore.DefaultTTL); err == nil {
			out["result_id"] = id
		}
	}

	// 确定性文字洞察
	if insight := attributionInsight(data); insight != "" {
		out["insight"] = insight
	}

	// 下钻校验建议
	drillDown := map[string]interface{}{
		"hint": "按头号驱动切片过滤明细重新取数，校验归因结论是否在明细层成立",
	}
	if sourceResultID != "" {
		drillDown["source_result_id"] = sourceResultID
	}
	if dim, _ := data["dimension"].(string); dim != "" && len(drivers) > 0 {
		if top, ok := drivers[0].(map[string]interface{}); ok {
			if seg, _ := top["segment"].(string); seg != "" {
				drillDown["suggestion"] = fmt.Sprintf("按 %s = %s 过滤明细下钻，校验头号驱动", dim, seg)
			}
		}
	}
	out["drill_down"] = drillDown
}

// attributionInsight 由归因结果生成确定性中文洞察（数字来自计算而非 LLM 复述）。
func attributionInsight(data map[string]interface{}) string {
	metric, _ := data["metric"].(string)
	if metric == "" {
		return ""
	}
	var b strings.Builder

	// 总量变化
	baseline, _ := data["baseline"].(map[string]interface{})
	current, _ := data["current"].(map[string]interface{})
	bLabel, _ := baseline["label"].(string)
	cLabel, _ := current["label"].(string)
	bTotal, _ := toNumber(baseline["total"])
	cTotal, _ := toNumber(current["total"])
	b.WriteString(fmt.Sprintf("%s 由 %s 的 %s 变为 %s 的 %s", metric, bLabel, fmtNum(bTotal), cLabel, fmtNum(cTotal)))
	b.WriteString(fmtDelta(data["total_delta"], data["total_delta_pct"]))
	b.WriteString("。")

	// 驱动排序（top 3）
	dim, _ := data["dimension"].(string)
	drivers, _ := data["drivers"].([]interface{})
	if dim != "" && len(drivers) > 0 {
		b.WriteString(fmt.Sprintf("按 %s 维度可加分解，主要驱动：", dim))
		n := len(drivers)
		if n > 3 {
			n = 3
		}
		parts := make([]string, 0, n)
		for _, item := range drivers[:n] {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			seg, _ := m["segment"].(string)
			part := seg
			if d, ok := toNumber(m["delta"]); ok {
				part += fmt.Sprintf(" %s", fmtSigned(d))
			}
			if cp, ok := toNumber(m["contribution_pct"]); ok {
				part += fmt.Sprintf("（贡献 %s%%）", fmtNum(cp))
			}
			parts = append(parts, part)
		}
		b.WriteString(strings.Join(parts, "、"))
		b.WriteString("。")
	}

	// 隐藏因子提示
	if hf, ok := data["hidden_factor"].(map[string]interface{}); ok {
		if suspected, _ := hf["suspected"].(bool); suspected {
			if notes, ok := hf["notes"].([]interface{}); ok && len(notes) > 0 {
				strs := make([]string, 0, len(notes))
				for _, n := range notes {
					if s, ok := n.(string); ok {
						strs = append(strs, s)
					}
				}
				if len(strs) > 0 {
					b.WriteString("疑似存在隐藏因子：")
					b.WriteString(strings.Join(strs, "；"))
					b.WriteString("。")
				}
			}
		}
	}

	// 置信标注
	if conf, _ := data["confidence"].(string); conf != "" {
		label := map[string]string{"high": "高", "medium": "中", "low": "低"}[conf]
		if label == "" {
			label = conf
		}
		b.WriteString(fmt.Sprintf("置信度：%s。", label))
	}
	return b.String()
}

// fmtDelta 格式化总变化量（含可选百分比），如 "（-350, -15.2%）"
func fmtDelta(delta interface{}, pct interface{}) string {
	d, ok := toNumber(delta)
	if !ok {
		return ""
	}
	s := fmt.Sprintf("（%s", fmtSigned(d))
	if p, ok := toNumber(pct); ok {
		s += fmt.Sprintf(", %s%%", fmtSigned(p))
	}
	return s + "）"
}

// toNumber 从 JSON 解码值中取数值（float64 / json.Number / 整型）
func toNumber(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// fmtNum 数值展示：整数不带小数，其余保留两位
func fmtNum(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// fmtSigned 带符号数值展示（正数前置 +）
func fmtSigned(v float64) string {
	s := fmtNum(v)
	if v > 0 {
		return "+" + s
	}
	return s
}

// failResult 构造非致命错误结果（JSON 字符串），交给 Agent 推理
func failResult(analysisType, msg string) string {
	out := map[string]interface{}{
		"analysis_type": analysisType,
		"success":       false,
		"error":         msg,
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return string(b)
}
