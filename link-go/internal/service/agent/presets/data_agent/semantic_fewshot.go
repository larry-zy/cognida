package dataagent

import (
	"context"
	"fmt"
	"strings"

	domainagent "link/internal/model/agent"
	domaincache "link/internal/model/cache"
	infraagent "link/internal/service/agent/framework"
)

// defaultFewShotTopN 首答注入的历史问答示例条数上限。
const defaultFewShotTopN = 3

// SemanticRecaller 软语义缓存召回的窄接口（由 infrastructure/cache.SemanticCacheService 实现）。
// 刻意收窄到只读召回一个方法，让本 hook 不依赖缓存服务的写侧/失效侧，便于替换与测试。
type SemanticRecaller interface {
	// RecallSimilar 返回与 req.Query 相似的历史问答示例（不短路、不校验 PromptHash）。
	RecallSimilar(ctx context.Context, req *domaincache.CacheRequest, topN int) ([]domaincache.RecalledExample, error)
}

// semanticFewShotHook 通用向量 few-shot（软语义缓存）Before hook：应答前把与当前问题
// 向量相似的历史问答作为**参考示例**注入用户 turn，由 LLM 结合实时数据重新生成——
// 而非硬缓存那样直接返回旧答案。用于 data agent 等"数据会变、必须重算"的场景。
//
// 与图谱经验召回 experienceRecallHook 并存、互补：图谱侧沉淀的是结构化「问题→解法」，
// 本 hook 沉淀的是"整段问答"的向量近邻；二者共同构成 data agent 的语义参考层。
//
// recaller 为 nil（未接线/未开启读侧门控）时恒等透传，零回归。
// agentKey 为该 agent 在缓存策略中的逻辑键（如 "data_agent"），用于隔离命名空间与开关。
func semanticFewShotHook(recaller SemanticRecaller, agentKey string) infraagent.BeforeHook {
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

		examples, err := recaller.RecallSimilar(ctx, &domaincache.CacheRequest{
			Query:     query,
			TenantID:  tenantID,
			AgentType: agentKey,
		}, defaultFewShotTopN)
		if err != nil || len(examples) == 0 {
			return ctx, message, nil // 召回失败或为空 → 透传，不阻断应答
		}
		return ctx, renderFewShotBlock(examples) + message, nil
	}
}

// renderFewShotBlock 把召回的历史问答渲染成注入块。刻意弱化为「仅供参考」的先验：
// 数据可能已变化，旧答案只作表达/思路参考，最终由模型基于实时查询独立作答。
func renderFewShotBlock(examples []domaincache.RecalledExample) string {
	var b strings.Builder
	b.WriteString("--- 相似历史问答参考（来自过往会话，数据可能已变化，仅供表达与思路参考）---\n")
	for i, e := range examples {
		q := strings.TrimSpace(e.Query)
		if q == "" {
			q = "（无问题文本）"
		}
		fmt.Fprintf(&b, "%d. 问：%s\n", i+1, q)
		if s := strings.TrimSpace(e.Response); s != "" {
			fmt.Fprintf(&b, "   答：%s\n", s)
		}
	}
	b.WriteString("--- 以上为历史参考，请基于当前实时数据独立作答 ---\n\n")
	return b.String()
}
