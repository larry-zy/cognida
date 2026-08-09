// Package tools 提供知识库列表工具
package tools

import (
	"context"
	"fmt"
	"time"

	agentctx "cognida/internal/model/agent"
	domain_knowledge "cognida/internal/model/knowledge"
)

// getTenantIDFromContext 从上下文获取租户ID（统一使用 model/agent 的类型化 key）
func getTenantIDFromContext(ctx context.Context) (int64, error) {
	if tid, ok := agentctx.GetTenantID(ctx); ok {
		return tid, nil
	}
	// 默认返回租户ID 1（用于开发/测试）
	return 1, nil
}

// ========================================
// kb_list - 知识库列表工具
// ========================================

// KbListRequest 知识库列表请求
type KbListRequest struct {
	Status string `json:"status" jsonschema:"description=状态筛选：all(全部)/enabled(启用)/disabled(禁用)，默认all"`
}

// KbListResult 知识库列表结果
type KbListResult struct {
	KnowledgeBases []KbInfo `json:"knowledge_bases"`
	Count          int      `json:"count"`
	// Mode 当前知识库选择模式：manual/hybrid/auto。Agent 据此决定是否调用 kb_route 聚焦。
	Mode      string `json:"mode"`
	LatencyMs int64  `json:"latency_ms"`
}

// KbInfo 知识库信息
type KbInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	DocumentCount int64  `json:"document_count"`
	ChunkCount    int64  `json:"chunk_count"`
	Status        string `json:"status"`
	// Selectable 该库是否在当前模式的候选池内（可被 kb_route 聚焦）。
	// manual 模式下始终为 false（无需/无法路由）。
	Selectable bool `json:"selectable"`
}

// NewKbListTool 创建知识库列表工具；repo 知识库仓储（可为 nil）经参数注入。
func NewKbListTool(repo domain_knowledge.KnowledgeBaseRepository) *TypedBaseTool[KbListRequest, KbListResult] {
	handler := func(ctx context.Context, req *KbListRequest) (*KbListResult, error) {
		return kbList(ctx, req, repo)
	}
	return NewTypedBaseTool(
		"kb_list",
		`获取当前租户下的所有知识库列表，并返回当前的知识库选择模式（mode）与每个库是否可被聚焦（selectable）。

适用场景：
- 用户询问"有哪些知识库"
- 查看知识库基本信息、文档/片段数量
- 结合/智能模式下，作为 kb_route 选库前的候选发现：先看 mode 与 selectable，再决定聚焦哪些库

返回字段说明：
- mode: 当前选择模式 manual/hybrid/auto。
  · manual：范围由用户锁定，无需 kb_route（selectable 恒为 false）。
  · hybrid：可从 selectable=true 的库（用户勾选范围）中经 kb_route 聚焦。
  · auto  ：可从 selectable=true 的库（租户全部已启用）中经 kb_route 聚焦。
- selectable: 该库是否在当前候选池内，可作为 kb_route 的 kb_ids。

参数：
- status: 状态筛选，可选值: all/enabled/disabled，默认 all`,
		handler,
	)
}

// kbList 执行知识库列表查询
func kbList(ctx context.Context, req *KbListRequest, kbRepoInstance domain_knowledge.KnowledgeBaseRepository) (*KbListResult, error) {
	startTime := time.Now()

	status := req.Status
	if status == "" {
		status = "all"
	}

	if kbRepoInstance == nil {
		return nil, fmt.Errorf("knowledge base repository not initialized")
	}

	tenantID, err := getTenantIDFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取租户信息失败: %w", err)
	}

	// 取足够大的单页覆盖租户全部 KB，避免候选被分页截断（与 adapters.enabledKBIDs 一致）。
	kbs, _, err := kbRepoInstance.FindByTenantID(ctx, tenantID, 1, 1000)
	if err != nil {
		return nil, fmt.Errorf("查询知识库列表失败: %w", err)
	}

	// 计算当前模式下的候选池（可被 kb_route 聚焦的库集合）。
	mode := agentctx.MustGetKBScopeMode(ctx)
	candidates := candidateKBSet(mode, agentctx.MustGetAllowedKBIDs(ctx), kbs)

	// 转换格式
	var results []KbInfo
	for _, kb := range kbs {
		if kb.DeletedAt != nil {
			continue
		}

		kbStatus := "enabled"
		if kb.Status == 0 {
			kbStatus = "disabled"
		}

		if status != "all" && status != kbStatus {
			continue
		}

		results = append(results, KbInfo{
			ID:            kb.ID,
			Name:          kb.Name,
			Description:   kb.Description,
			DocumentCount: int64(kb.DocumentCount),
			ChunkCount:    int64(kb.ChunkCount),
			Status:        kbStatus,
			Selectable:    candidates[kb.ID],
		})
	}

	return &KbListResult{
		KnowledgeBases: results,
		Count:          len(results),
		Mode:           mode,
		LatencyMs:      time.Since(startTime).Milliseconds(),
	}, nil
}

// candidateKBSet 计算当前模式下可被 kb_route 聚焦的知识库集合（仅含已启用、未删除库）：
//   - manual：空集（该模式不路由）；
//   - auto  ：租户全部已启用；
//   - hybrid：用户勾选 ∩ 已启用；未勾选 → 全部已启用。
func candidateKBSet(mode string, selected []string, kbs []*domain_knowledge.KnowledgeBase) map[string]bool {
	set := make(map[string]bool)
	if mode == agentctx.KBScopeModeManual {
		return set // 手动模式不路由
	}
	enabled := make(map[string]bool)
	for _, kb := range kbs {
		if kb.DeletedAt == nil && kb.Status != 0 {
			enabled[kb.ID] = true
		}
	}
	if mode == agentctx.KBScopeModeAuto || len(selected) == 0 {
		return enabled // 智能：全部已启用；结合但未勾选：回退全部已启用
	}
	for _, id := range selected { // 结合：勾选 ∩ 已启用
		if enabled[id] {
			set[id] = true
		}
	}
	return set
}
