// Package tools 提供 etl_run 派生工具（Phase 4）。
package tools

import (
	"context"
	"fmt"
	"github.com/cloudwego/eino/components/tool/utils"
	"strings"
	"time"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/agent/operations"
	"cognida/internal/service/agent/resultstore"
)

// ========================================
// ETL Run Tool
// ========================================

// ETLRunRequest ETL 派生请求：把 SELECT 结果或已有 result_id 物化为新表。
type ETLRunRequest struct {
	IdempotencyKey string `json:"idempotency_key" jsonschema:"required,description=幂等键，重试复用原键"`
	// TargetTable 派生目标表名，必须以 agent_etl_ 前缀开头
	TargetTable string `json:"target_table" jsonschema:"required,description=派生目标表名，必须以agent_etl_前缀开头"`

	// SQL SELECT 输入源（与 result_id 二选一）
	SQL string `json:"sql" jsonschema:"description=SELECT查询作为输入源，与result_id二选一"`

	// ResultID Result Store 引用输入源（与 sql 二选一）
	ResultID string `json:"result_id" jsonschema:"description=已有结果集引用作为输入源，与sql二选一"`

	// Replace 目标表已存在时是否替换（仅允许替换 agent_etl_ 前缀表）
	Replace bool `json:"replace" jsonschema:"description=目标表已存在时是否替换，默认false"`
}

// ETLRunResult ETL 派生结果。
type ETLRunResult struct {
	PlanID     string `json:"plan_id,omitempty"`
	PlanHash   string `json:"plan_hash,omitempty"`
	ConfirmURL string `json:"confirm_url,omitempty"`
	// Status success / rejected
	Status string `json:"status"`

	// TargetTable 派生出的表名
	TargetTable string `json:"target_table,omitempty"`

	// RowCount 派生表行数
	RowCount int64 `json:"row_count,omitempty"`

	// Source 输入源描述（sql / result_id）
	Source string `json:"source,omitempty"`

	// Message 说明（拒绝原因等）
	Message string `json:"message,omitempty"`

	// LatencyMs 耗时（毫秒）
	LatencyMs int64 `json:"latency_ms"`
}

// NewETLRunTool 创建 ETL 派生工具；o 操作工具依赖（写库/审计/结果存储）经参数注入。
func NewETLRunTool(o *opTools) *TypedBaseTool[ETLRunRequest, ETLRunResult] {
	handler := func(ctx context.Context, req *ETLRunRequest) (*ETLRunResult, error) {
		return o.etlRun(ctx, req)
	}
	created := NewTypedBaseTool("etl_run",
		`生成查询结果派生表（ETL）的真实待确认计划，不直接写入。

安全限制：
- 只派生新对象，目标表名必须以 agent_etl_ 前缀开头（强校验）
- 绝不写入或修改任何源表
- 输入源二选一：sql（仅 SELECT）或 result_id（本会话的结果集引用）
- 返回真实 plan_id、plan_hash、confirm_url；用户明确确认后先新后切，旧表保留
- idempotency_key 必填，重复调用复用原计划

参数：
- target_table: 派生目标表名（必需，agent_etl_ 前缀）
- sql: SELECT 输入源（可选）
- result_id: 结果集引用输入源（可选）
- replace: 目标已存在时是否替换（可选，默认 false）`,
		handler,
	)
	params, err := utils.GoStruct2ParamsOneOf[ETLRunRequest]()
	if err != nil {
		panic("invalid static governed operation schema")
	}
	created.ParamsOneOf = params
	return created
}

// etlRun 执行 ETL 派生：前缀强校验 → 输入源解析 → 物化新表。
func (o *opTools) etlRun(ctx context.Context, req *ETLRunRequest) (*ETLRunResult, error) {
	started := time.Now()
	if agentctx.MustGetDatasourceID(ctx) != "" {
		return nil, fmt.Errorf("外部数据源为只读，禁止 ETL 派生")
	}
	if req == nil {
		return nil, fmt.Errorf("ETL 请求不能为空")
	}
	if err := validateETLTarget(req.TargetTable); err != nil {
		o.recordAudit(ctx, operations.OpETL, req.TargetTable, req.SQL, req.IdempotencyKey, operations.StatusRejected, err.Error(), nil)
		return &ETLRunResult{Status: operations.StatusRejected, Message: err.Error()}, nil
	}
	hasSQL := strings.TrimSpace(req.SQL) != ""
	if hasSQL == (req.ResultID != "") {
		return nil, fmt.Errorf("输入源必须且只能提供一个：sql 或 result_id")
	}
	if hasSQL {
		if err := validateSQL(req.SQL); err != nil {
			o.recordAudit(ctx, operations.OpETL, req.TargetTable, req.SQL, req.IdempotencyKey, operations.StatusFailed, err.Error(), nil)
			return nil, fmt.Errorf("输入源 SQL 校验失败（仅允许 SELECT）: %w", err)
		}
	} else if _, err := o.loadETLResult(ctx, req.ResultID); err != nil {
		return nil, err
	}
	if req.IdempotencyKey == "" {
		return nil, fmt.Errorf("idempotency_key 不能为空")
	}
	if o.cfg.PrepareETL == nil {
		return nil, fmt.Errorf("受控操作服务未启用，未执行任何写入")
	}
	plan, err := o.cfg.PrepareETL(ctx, req.TargetTable, req.ResultID, req.SQL, req.IdempotencyKey, req.Replace)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, fmt.Errorf("受控操作未返回计划")
	}
	return &ETLRunResult{Status: "pending_confirm", TargetTable: req.TargetTable, PlanID: plan.ID, PlanHash: plan.Plan.Hash, ConfirmURL: "/operations?job_id=" + plan.ID, Message: "已固定真实输入并保存 ETL 计划；用户明确确认后先写新版本、核验成功再切换，旧版本保留。", LatencyMs: time.Since(started).Milliseconds()}, nil
}

// loadETLResult authorizes the source before any target replacement takes place.
func (o *opTools) loadETLResult(ownerCtx context.Context, resultID string) (*resultstore.Result, error) {
	if o.resultStore == nil {
		return nil, fmt.Errorf("结果存储未启用，无法按 result_id 派生，请改用 sql 输入源")
	}
	owner := resultstore.OwnerKey(agentctx.MustGetTenantID(ownerCtx), agentctx.MustGetSessionID(ownerCtx))
	stored, err := o.resultStore.Get(ownerCtx, owner, resultID)
	if err != nil {
		switch err {
		case resultstore.ErrNotFound:
			return nil, fmt.Errorf("result_id %s 不存在或已过期，请重新取数", resultID)
		case resultstore.ErrUnauthorized:
			return nil, fmt.Errorf("result_id %s 不属于当前会话", resultID)
		default:
			return nil, fmt.Errorf("读取结果存储失败: %w", err)
		}
	}
	if err := validatePrivateDataAccess(stored.SourceSQL); err != nil {
		return nil, err
	}
	return stored, nil
}
