package dataagent

import (
	"context"
	"fmt"
	"strings"

	domainagent "cognida/internal/model/agent"
	experiencesvc "cognida/internal/service/agent/experience"
	infraagent "cognida/internal/service/agent/framework"
)

// defaultRecallTopN 首答注入的历史经验条数上限（业务策略层设定；能力层只按 limit 返回）。
const defaultRecallTopN = 3

// experienceRecallHook 在应答前把与当前问题相关的历史经验（过往会话沉淀）注入用户 turn，
// 闭合「蒸馏→图谱→召回→注入」自进化回路的读侧。
//
// recaller 为 nil（未接线/未开启 EXPERIENCE_RECALL_ENABLED）时恒等透传，零回归。
// 召回用 UserQueryFromContext 取原始问题（不受前置 playbook/skill 注入污染），命中为空则透传。
// 注入文本只进本轮用户 turn、不落库（框架持久化的是原始消息），不污染历史与缓存基线。
func experienceRecallHook(recaller *experiencesvc.GraphRecaller) infraagent.BeforeHook {
	return func(ctx context.Context, message string) (context.Context, string, error) {
		if recaller == nil {
			return ctx, message, nil
		}
		tenantID, ok := domainagent.GetTenantID(ctx)
		if !ok || tenantID <= 0 {
			return ctx, message, nil
		}
		query := infraagent.UserQueryFromContext(ctx)
		if strings.TrimSpace(query) == "" {
			query = message
		}
		recalled := recaller.Recall(ctx, tenantID, query, defaultRecallTopN)
		if len(recalled) == 0 {
			return ctx, message, nil
		}
		return ctx, renderRecallBlock(recalled) + message, nil
	}
}

// renderRecallBlock 把召回经验渲染成注入块。刻意弱化为「仅供参考」的先验，
// 避免历史经验误导与当前问题不符的情形；解法只作提示，最终由模型结合当前问题独立判断。
func renderRecallBlock(recalled []experiencesvc.RecalledExperience) string {
	var b strings.Builder
	b.WriteString("--- 历史经验参考（来自过往同类会话沉淀，仅供参考，未必适用当前问题）---\n")
	for i, e := range recalled {
		title := strings.TrimSpace(e.Title)
		if title == "" {
			title = "（无标题）"
		}
		fmt.Fprintf(&b, "%d. 【%s】\n", i+1, title)
		if s := strings.TrimSpace(e.Problem); s != "" {
			fmt.Fprintf(&b, "   诉求：%s\n", s)
		}
		if s := strings.TrimSpace(e.Solution); s != "" {
			fmt.Fprintf(&b, "   解法：%s\n", s)
		}
	}
	b.WriteString("--- 以上为历史参考，请结合当前问题独立判断 ---\n\n")
	return b.String()
}
