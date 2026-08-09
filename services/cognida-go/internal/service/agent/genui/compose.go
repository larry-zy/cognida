package genui

import (
	"context"
	"log"
)

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
//   - model 非 nil → 先试 Level 2；失败回退 Level 1 模板。
//   - model 为 nil → 直接 Level 1 模板。
//
// model 由组合根构建 toolModel 后经 AgentHandler.SetGenUIModel 注入并显式传入，
// 替代此前的包级单例（消除隐藏依赖/并发/测试隐患〔GO-2〕）。
// 无论哪条路径，dataModel 中的数字都来自真实工具输出，LLM 只决定布局。
func Compose(ctx context.Context, model LLM, in ComposeInput) *UISpec {
	dm := AssembleReportDataModel(in.SQLOutputs, in.AnalysisOutputs)
	if dm == nil {
		return nil
	}

	if model != nil {
		if spec, err := LLMCompose(ctx, model, dm, in.Question); err == nil {
			return spec
		} else {
			log.Printf("[genui] Level2 LLM 生成失败，回退模板: %v", err)
		}
	}
	return TemplateCompose(dm, in.Question)
}
