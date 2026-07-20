package genui

import (
	"context"
	"log"
)

// composeModel 是注入的 LLM（Level 2 用）。为 nil 时仅用 Level 1 模板。
// 采用与本仓一致的包级单例 + Init 注入（对齐 tools.InitDataAnalysisTool 等），
// 组合根在构建 toolModel 后调用 SetModel，无需改动 wire。
var composeModel LLM

// SetModel 注入用于生成式布局的 LLM。传 nil 表示禁用 Level 2（退化为纯模板）。
func SetModel(m LLM) {
	composeModel = m
}

// ComposeInput 是一次 UI 生成所需的原料：用户问题 + 本次回答里「全部」真实工具输出。
//
// 一次回答（尤其报告/综合解读）常跑多段 sql_execute / data_analysis：KPI 汇总、明细趋势、
// 分组对比各是独立结果集。收全后交给 AssembleReportDataModel 融合，单行 KPI 结果派生标量指标、
// 多行结果作主表/序列，避免「只取最后一段」导致 KPI 卡无数据。
type ComposeInput struct {
	Question        string   // 用户自然语言问题
	SQLOutputs      []string // 本次回答里全部 sql_execute 的 tool_output（JSON，按发生顺序）
	AnalysisOutputs []string // 本次回答里全部 data_analysis 的 tool_output（JSON，可空）
}

// Compose 是本包对外入口：把真实工具输出装配成 DataModel，再生成 UISpec。
//   - 无行集（DataModel 为 nil）→ 返回 nil（本次回答无结构化数据可视化）。
//   - 已注入 LLM → 先试 Level 2；失败回退 Level 1 模板。
//   - 未注入 LLM → 直接 Level 1 模板。
//
// 无论哪条路径，dataModel 中的数字都来自真实工具输出，LLM 只决定布局。
func Compose(ctx context.Context, in ComposeInput) *UISpec {
	dm := AssembleReportDataModel(in.SQLOutputs, in.AnalysisOutputs)
	if dm == nil {
		return nil
	}

	if composeModel != nil {
		if spec, err := LLMCompose(ctx, composeModel, dm, in.Question); err == nil {
			return spec
		} else {
			log.Printf("[genui] Level2 LLM 生成失败，回退模板: %v", err)
		}
	}
	return TemplateCompose(dm, in.Question)
}
