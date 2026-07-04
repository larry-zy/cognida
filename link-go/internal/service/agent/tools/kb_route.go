// Package tools 提供知识库路由（agentic 选库）工具
package tools

import (
	"context"
	"time"

	agentctx "link/internal/model/agent"
)

// ========================================
// kb_route - 知识库路由工具（让 Agent 自主聚焦检索范围）
// ========================================

// KbRouteRequest 知识库路由请求
type KbRouteRequest struct {
	// KBIDs Agent 决定聚焦的知识库 ID 列表（来自 kb_list 返回、且 selectable=true 的库）
	KBIDs []string `json:"kb_ids" jsonschema:"required,description=要聚焦检索的知识库ID列表，取自 kb_list 返回中 selectable=true 的库"`
	// Reason 选择理由（可选，便于调试与向用户解释）
	Reason string `json:"reason" jsonschema:"description=选择这些知识库的简要理由，可选"`
}

// KbRouteResult 知识库路由结果
type KbRouteResult struct {
	// ScopedTo 已记录的聚焦知识库 ID
	ScopedTo []string `json:"scoped_to"`
	// Applied 路由是否被本次会话接受（手动模式或缺少路由 holder 时为 false）
	Applied bool `json:"applied"`
	// Note 说明
	Note string `json:"note"`
	// LatencyMs 耗时
	LatencyMs int64 `json:"latency_ms"`
}

// NewKbRouteTool 创建知识库路由工具
func NewKbRouteTool() *TypedBaseTool[KbRouteRequest, KbRouteResult] {
	return NewTypedBaseTool(
		"kb_route",
		`将后续检索聚焦到你选定的若干知识库（agentic 选库）。

使用时机：
- 仅在「结合(hybrid)」或「智能(auto)」模式下有意义——先调用 kb_list 查看当前 mode 与各库的 selectable，
  再从 selectable=true 的库中挑出与问题最相关的 1~N 个，调用本工具声明聚焦范围，然后再 rag_query / graph_query 检索。
- 「手动(manual)」模式下检索范围由用户锁定，本工具的选择会被系统忽略——无需调用，直接检索即可。

安全说明：系统始终在「允许范围」内应用你的选择，越权/超范围/禁用的库会被自动丢弃；
选空或全部越界时本次不缩小范围（按允许上界检索）。

参数：
- kb_ids: 要聚焦的知识库 ID 列表（必需）
- reason: 选择理由（可选）`,
		kbRoute,
	)
}

// kbRoute 记录 Agent 的聚焦选择到 ctx 中的路由 holder，供其后的 rag_query / graph_query 读取。
func kbRoute(ctx context.Context, req *KbRouteRequest) (*KbRouteResult, error) {
	startTime := time.Now()

	sel, ok := agentctx.GetRouteSelection(ctx)
	if !ok || sel == nil {
		// 防御：入口未注入路由 holder（如手动模式或非 agentic 入口）。不报错，提示范围由系统强制。
		return &KbRouteResult{
			ScopedTo:  req.KBIDs,
			Applied:   false,
			Note:      "当前会话未启用 agentic 路由（可能为手动模式）；检索范围由系统按用户选定强制，本次选择被忽略。",
			LatencyMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	sel.Set(req.KBIDs)
	return &KbRouteResult{
		ScopedTo:  req.KBIDs,
		Applied:   true,
		Note:      "已记录聚焦范围；系统将在允许范围内应用，越权/超范围的库会被自动忽略。",
		LatencyMs: time.Since(startTime).Milliseconds(),
	}, nil
}
