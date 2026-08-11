// Package tools 提供数据源清单工具：让 Data Agent 按业务描述自选目标库。
package tools

import (
	"context"
	"fmt"

	agentctx "cognida/internal/model/agent"
	model_datasource "cognida/internal/model/datasource"
)

// ========================================
// List Datasources Tool
// ========================================

// ListDatasourcesRequest 数据源清单请求（无参数：始终列出当前租户已注册的全部外部数据源）。
type ListDatasourcesRequest struct{}

// DatasourceCard 单个数据源的安全视图（不含主机/端口/凭证），供 agent 选库决策。
type DatasourceCard struct {
	// ID 数据源 ID：选定后作为 get_schema / sql_execute 的 database_id 传入。
	ID string `json:"id"`
	// Name 数据源名称
	Name string `json:"name"`
	// Description 业务描述：agent 据此按语义匹配用户问题对应的库
	Description string `json:"description,omitempty"`
	// Type 数据源类型（mysql/postgres）
	Type string `json:"type"`
	// DatabaseName 库名
	DatabaseName string `json:"database_name"`
	// Status 状态（active/error/need_credentials）：非 active 的库不可用，勿选
	Status string `json:"status"`
	// Selected 是否为本会话已锁定（用户手动选定）的数据源；为 true 时直接查询、无需再传 database_id
	Selected bool `json:"selected,omitempty"`
}

// ListDatasourcesResult 数据源清单结果
type ListDatasourcesResult struct {
	// Datasources 当前租户已注册的外部数据源
	Datasources []DatasourceCard `json:"datasources"`
	// Note 选库指引，便于 LLM 决策
	Note string `json:"note"`
}

// listDatasourcesLimit 单次列出上限：租户数据源通常很少，足够；超出则在 Note 提示。
const listDatasourcesLimit = 200

// NewListDatasourcesTool 创建数据源清单工具。
// lister 为数据源仓储（可为 nil，缺失时工具返回未启用提示，仅默认业务库可查）。
func NewListDatasourcesTool(lister model_datasource.Repository) *TypedBaseTool[ListDatasourcesRequest, ListDatasourcesResult] {
	handler := func(ctx context.Context, _ *ListDatasourcesRequest) (*ListDatasourcesResult, error) {
		return listDatasources(ctx, lister)
	}
	return NewTypedBaseTool("list_datasources",
		`列出当前可用的外部数据源（已注册的数据库连接），用于在回答前挑选正确的目标库。

何时使用：
- 用户问题涉及的数据可能不在默认业务库、而在某个外部数据源（如"电商订单""财务"等业务库）时，
  先用本工具查看有哪些库及其业务描述，再据描述挑最匹配的库。
- 选定后，把该数据源的 id 作为后续 get_schema / sql_execute 的 database_id 传入。

注意：
- 若结果中某库标了 selected=true，表示用户已手动锁定该库——直接查询即可，不要再传 database_id 覆盖。
- 只选 status=active 的库；拿不准该用哪个库时，向用户澄清而非乱猜。`,
		handler,
	)
}

// listDatasources 是工具处理器：按租户列出数据源，并标注会话已锁定项、给出选库指引。
func listDatasources(ctx context.Context, lister model_datasource.Repository) (*ListDatasourcesResult, error) {
	if lister == nil {
		return &ListDatasourcesResult{
			Datasources: []DatasourceCard{},
			Note:        "外部数据源功能未启用，当前只能查询默认业务库（get_schema / sql_execute 不要传 database_id）。",
		}, nil
	}

	tenantID := agentctx.MustGetTenantID(ctx)
	items, total, err := lister.List(ctx, model_datasource.ListFilter{
		TenantID: tenantID,
		Limit:    listDatasourcesLimit,
		Offset:   0,
	})
	if err != nil {
		return nil, fmt.Errorf("列出数据源失败: %w", err)
	}

	// 会话已锁定的数据源（用户手动选库）：用于标注 selected 并调整指引。
	sessionDS := agentctx.MustGetDatasourceID(ctx)

	cards := make([]DatasourceCard, 0, len(items))
	var lockedName string
	for _, ds := range items {
		selected := sessionDS != "" && ds.ID == sessionDS
		if selected {
			lockedName = ds.Name
		}
		cards = append(cards, DatasourceCard{
			ID:           ds.ID,
			Name:         ds.Name,
			Description:  ds.Description,
			Type:         string(ds.Type),
			DatabaseName: ds.DatabaseName,
			Status:       ds.Status,
			Selected:     selected,
		})
	}

	note := buildDatasourceNote(cards, sessionDS, lockedName, total)
	return &ListDatasourcesResult{Datasources: cards, Note: note}, nil
}

// buildDatasourceNote 组织选库指引：区分「已手动锁定」「有候选待选」「无外部库」三种情形。
func buildDatasourceNote(cards []DatasourceCard, sessionDS, lockedName string, total int64) string {
	switch {
	case sessionDS != "" && lockedName != "":
		return fmt.Sprintf("用户已手动锁定数据源 %q（selected=true），直接查询即可，不要传 database_id 覆盖。", lockedName)
	case sessionDS != "":
		// 会话锁了一个 id，但不在清单里（可能已被删除/跨租户）：提示按锁定 id 查询或澄清。
		return fmt.Sprintf("会话已锁定数据源 id=%q（未在清单中，可能已删除），如查询失败请向用户澄清。", sessionDS)
	case len(cards) == 0:
		return "当前租户未注册任何外部数据源，只能查询默认业务库（不要传 database_id）。"
	default:
		note := "据各库的 description 挑选与用户问题最匹配的库，把其 id 作为后续 get_schema / sql_execute 的 database_id 传入；只选 status=active 的库，拿不准则向用户澄清。"
		if total > int64(len(cards)) {
			note = fmt.Sprintf("%s（共 %d 个数据源，仅列出前 %d 个）", note, total, len(cards))
		}
		return note
	}
}
