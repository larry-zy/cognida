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

// ComposeInput 是一次 UI 生成所需的原料：用户问题 + 真实工具输出（JSON 字符串）。
type ComposeInput struct {
	Question       string // 用户自然语言问题
	SQLOutput      string // sql_execute 工具的 tool_output（JSON）
	AnalysisOutput string // data_analysis 工具的 tool_output（JSON，可空）
}

// Compose 是本包对外入口：把真实工具输出装配成 DataModel，再生成 UISpec。
//   - 无行集（DataModel 为 nil）→ 返回 nil（本次回答无结构化数据可视化）。
//   - 已注入 LLM → 先试 Level 2；失败回退 Level 1 模板。
//   - 未注入 LLM → 直接 Level 1 模板。
//
// 无论哪条路径，dataModel 中的数字都来自真实工具输出，LLM 只决定布局。
func Compose(ctx context.Context, in ComposeInput) *UISpec {
	dm := AssembleDataModel(in.SQLOutput, in.AnalysisOutput)
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
