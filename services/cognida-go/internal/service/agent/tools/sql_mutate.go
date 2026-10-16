// Package tools 提供 sql_mutate 写操作工具（Phase 4）。
package tools

import (
	"context"
	"fmt"
	"github.com/cloudwego/eino/components/tool/utils"
	"time"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/agent/operations"
	"cognida/internal/service/agent/pendingaction"
)

// ========================================
// SQL Mutate Tool
// ========================================

// SQLMutateRequest 写操作请求。
type SQLMutateRequest struct {
	// SQL 要执行的 DML 语句（仅 INSERT/UPDATE/DELETE）
	SQL string `json:"sql" jsonschema:"required,description=要执行的DML语句，仅支持INSERT/UPDATE/DELETE"`

	// IdempotencyKey 幂等键（必填）：同键重复提交返回首次结果，不重复写库
	IdempotencyKey string `json:"idempotency_key" jsonschema:"required,description=幂等键，同键重复提交不重复写库"`
}

// SQLMutateResult 写操作结果。
type SQLMutateResult struct {
	PlanID     string `json:"plan_id,omitempty"`
	PlanHash   string `json:"plan_hash,omitempty"`
	ConfirmURL string `json:"confirm_url,omitempty"`
	// Status success / rejected / pending_confirm
	Status string `json:"status"`

	// Target 目标表
	Target string `json:"target,omitempty"`

	// RowsAffected 影响行数（success 时）
	RowsAffected int64 `json:"rows_affected,omitempty"`

	// Duplicate 幂等命中：本次未写库，返回的是首次执行结果
	Duplicate bool `json:"duplicate,omitempty"`

	// PendingActionID 危险操作暂停后的确认凭据 ID（pending_confirm 时）
	PendingActionID string `json:"pending_action_id,omitempty"`

	// ConfirmToken 一次性确认 token，供 UI 确认卡片 resume 时携带
	ConfirmToken string `json:"confirm_token,omitempty"`

	// Message 说明（拒绝原因 / 暂停提示 / 幂等提示）
	Message string `json:"message,omitempty"`

	// LatencyMs 耗时（毫秒）
	LatencyMs int64 `json:"latency_ms"`
}

// NewSQLMutateTool 创建写操作工具；o 操作工具依赖（写库/审计/待确认存储/红线/阈值）经参数注入。
func NewSQLMutateTool(o *opTools) *TypedBaseTool[SQLMutateRequest, SQLMutateResult] {
	handler := func(ctx context.Context, req *SQLMutateRequest) (*SQLMutateResult, error) {
		return o.sqlMutate(ctx, req)
	}
	created := NewTypedBaseTool("sql_mutate",
		`生成受控数据写操作计划（DML），不执行写入。

安全限制：
- 仅支持 INSERT/UPDATE/DELETE，拒绝任何 DDL
- 必须携带 idempotency_key，同键重复提交不重复写库
- 红线原始业务表拒绝修改
- 每次操作都返回真实 plan_id、plan_hash、confirm_url，必须由用户在确认页明确提交
- 只支持已管理派生目标的单表字面值 INSERT、UPDATE、DELETE；UPDATE/DELETE 必须等值 WHERE
- 所有执行与被拒都写入操作审计

参数：
- sql: DML 语句（必需）
- idempotency_key: 幂等键（必需）`,
		handler,
	)
	params, err := utils.GoStruct2ParamsOneOf[SQLMutateRequest]()
	if err != nil {
		panic("invalid static governed operation schema")
	}
	created.ParamsOneOf = params
	return created
}

// sqlMutate 执行 DML 写操作：校验 → 红线 → 幂等 → 事务内 dry-run → 危险分级。
func (o *opTools) sqlMutate(ctx context.Context, req *SQLMutateRequest) (*SQLMutateResult, error) {
	started := time.Now()
	if agentctx.MustGetDatasourceID(ctx) != "" {
		return nil, fmt.Errorf("外部数据源为只读，禁止写操作")
	}
	if req == nil || req.IdempotencyKey == "" {
		return nil, fmt.Errorf("idempotency_key 不能为空")
	}
	target := extractMutateTarget(req.SQL)
	if err := validateDML(req.SQL); err != nil {
		o.recordAudit(ctx, operations.OpMutate, target, req.SQL, req.IdempotencyKey, operations.StatusRejected, err.Error(), nil)
		return &SQLMutateResult{Status: operations.StatusRejected, Target: target, Message: err.Error()}, nil
	}
	if o.isRedlineTable(target) {
		o.recordAudit(ctx, operations.OpMutate, target, req.SQL, req.IdempotencyKey, operations.StatusRejected, "红线原始业务表禁止修改", nil)
		return &SQLMutateResult{Status: operations.StatusRejected, Target: target, Message: "红线原始业务表禁止修改；仅允许当前用户拥有的受控派生目标"}, nil
	}
	if o.cfg.PrepareSQL == nil {
		return nil, fmt.Errorf("受控操作服务未启用，未执行任何写入")
	}
	plan, err := o.cfg.PrepareSQL(ctx, req.SQL, req.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, fmt.Errorf("受控操作未返回计划")
	}
	return &SQLMutateResult{Status: "pending_confirm", Target: target, PlanID: plan.ID, PlanHash: plan.Plan.Hash, ConfirmURL: "/operations?job_id=" + plan.ID, Message: "已保存受控操作计划；请到确认页面核对目标、版本、影响行数与摘要后明确确认。此工具未执行写入。", LatencyMs: time.Since(started).Milliseconds()}, nil
}

// Old pending-action tokens do not authorize the new immutable operation plan.
func (o *opTools) ExecuteConfirmedMutation(ctx context.Context, action *pendingaction.PendingAction) (*SQLMutateResult, error) {
	o.recordAudit(ctx, operations.OpMutate, "", "", "", operations.StatusRejected, "旧确认记录不能授权受控写入", nil)
	return &SQLMutateResult{Status: operations.StatusRejected, Message: "旧确认记录已停用；请重新生成受控操作计划并在确认页面核对后提交。未执行写入。"}, nil
}
