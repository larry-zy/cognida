// Package tools 提供 sql_mutate 写操作工具（Phase 4）。
package tools

import (
	"context"
	"fmt"
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
	return NewTypedBaseTool("sql_mutate",
		`执行数据写操作（DML）。

安全限制：
- 仅支持 INSERT/UPDATE/DELETE，拒绝任何 DDL
- 必须携带 idempotency_key，同键重复提交不重复写库
- 红线原始业务表拒绝修改
- 影响行数达到危险阈值时暂停执行，返回 pending_action_id 等待人工确认
- 所有执行与被拒都写入操作审计

参数：
- sql: DML 语句（必需）
- idempotency_key: 幂等键（必需）`,
		handler,
	)
}

// sqlMutate 执行 DML 写操作：校验 → 红线 → 幂等 → 事务内 dry-run → 危险分级。
func (o *opTools) sqlMutate(ctx context.Context, req *SQLMutateRequest) (*SQLMutateResult, error) {
	startTime := time.Now()

	// 外部数据源会话红线：外部库只读，写操作一律拒绝（绝不落到外部库或误写业务库）
	if dsID := agentctx.MustGetDatasourceID(ctx); dsID != "" {
		return nil, fmt.Errorf("当前会话选择了外部数据源（%s），外部数据源为只读，禁止写操作", dsID)
	}
	if req.IdempotencyKey == "" {
		return nil, fmt.Errorf("idempotency_key 不能为空（写操作必须幂等）")
	}
	if o.cfg.DB == nil {
		return nil, fmt.Errorf("写库未初始化（sql_mutate 不可用）")
	}
	if o.cfg.Audit == nil {
		// 无审计不落写：留痕是写操作的硬前置
		return nil, fmt.Errorf("操作审计未启用，写操作被禁止")
	}

	target := extractMutateTarget(req.SQL)

	// 1) DML 校验：拒绝 DDL / 多语句 / 注释（拒绝也必须留痕）
	if err := validateDML(req.SQL); err != nil {
		o.recordAudit(ctx, operations.OpMutate, target, req.SQL, req.IdempotencyKey,
			operations.StatusRejected, err.Error(), nil)
		return &SQLMutateResult{
			Status:    operations.StatusRejected,
			Target:    target,
			Message:   err.Error(),
			LatencyMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	// 2) 红线原始表拒改
	if o.isRedlineTable(target) {
		msg := fmt.Sprintf("表 %s 属于红线原始业务表，禁止 Agent 直接修改", target)
		o.recordAudit(ctx, operations.OpMutate, target, req.SQL, req.IdempotencyKey,
			operations.StatusRejected, msg, nil)
		return &SQLMutateResult{
			Status:    operations.StatusRejected,
			Target:    target,
			Message:   msg,
			LatencyMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	// 3) 幂等去重：同键已成功执行过则直接返回首次结果，不重复写库
	tenantID := agentctx.MustGetTenantID(ctx)
	if prior, err := o.cfg.Audit.FindByIdempotencyKey(ctx, tenantID, req.IdempotencyKey); err != nil {
		return nil, fmt.Errorf("幂等检查失败: %w", err)
	} else if prior != nil {
		return &SQLMutateResult{
			Status:    operations.StatusSuccess,
			Target:    prior.Target,
			Duplicate: true,
			Message:   fmt.Sprintf("幂等命中：该操作已于 %s 执行（%s），本次未重复写库", prior.CreatedAt.Format(time.RFC3339), prior.Result),
			LatencyMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	// 4) 事务内执行并评估影响行数（dry-run：先不提交）
	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	tx := o.cfg.DB.WithContext(execCtx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("开启事务失败: %w", tx.Error)
	}
	res := tx.Exec(req.SQL)
	if res.Error != nil {
		tx.Rollback()
		o.recordAudit(ctx, operations.OpMutate, target, req.SQL, req.IdempotencyKey,
			operations.StatusFailed, res.Error.Error(), nil)
		// 写失败也产出结构化可修复观察（分级 error_kind + 定向列/表线索 + 脱敏摘要）：
		// 交 Operation 子代理（或简单直接写的主循环）按修复纪律本地定向重试，而非盲目重试。
		// 写库恒为内部业务库（外部源已在入口拒绝），detail 经脱敏截断透传；错误细节留在
		// 本工具调用者上下文——经委派边界时 executeDelegation 只回传紧凑 handle/结论，不回灌指挥官。
		// 每次失败尝试（含修复重试）都已先落 agent_operation_audit（上一行）。
		return nil, newRepairableSQLError(ctx, o.writeQueryTarget(), req.SQL, res.Error, nil, "")
	}
	affected := res.RowsAffected

	// 5) 危险分级：影响行数达到阈值 → 回滚并暂停待人工确认
	if affected >= int64(o.cfg.DangerRowThreshold) {
		tx.Rollback()
		return o.suspendDangerousMutation(ctx, req, target, affected, startTime)
	}

	// 安全级：提交并留痕
	if err := tx.Commit().Error; err != nil {
		o.recordAudit(ctx, operations.OpMutate, target, req.SQL, req.IdempotencyKey,
			operations.StatusFailed, err.Error(), nil)
		return nil, fmt.Errorf("提交失败: %w", err)
	}
	o.recordAudit(ctx, operations.OpMutate, target, req.SQL, req.IdempotencyKey,
		operations.StatusSuccess, fmt.Sprintf("影响 %d 行", affected),
		map[string]interface{}{"rows_affected": affected})

	return &SQLMutateResult{
		Status:       operations.StatusSuccess,
		Target:       target,
		RowsAffected: affected,
		LatencyMs:    time.Since(startTime).Milliseconds(),
	}, nil
}

// suspendDangerousMutation 危险级写操作：暂存 pending action 并留痕 pending_confirm。
func (o *opTools) suspendDangerousMutation(ctx context.Context, req *SQLMutateRequest, target string, affected int64, startTime time.Time) (*SQLMutateResult, error) {
	if o.cfg.Pending == nil {
		// 宁拒不闯：无待确认存储时直接拒绝
		msg := fmt.Sprintf("预计影响 %d 行（阈值 %d），且确认通道未启用，操作被拒绝", affected, o.cfg.DangerRowThreshold)
		o.recordAudit(ctx, operations.OpMutate, target, req.SQL, req.IdempotencyKey,
			operations.StatusRejected, msg, map[string]interface{}{"rows_affected": affected})
		return &SQLMutateResult{
			Status:    operations.StatusRejected,
			Target:    target,
			Message:   msg,
			LatencyMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	action := &pendingaction.PendingAction{
		Owner: operationOwner(ctx),
		Kind:  operations.OpMutate,
		SQL:   req.SQL,
		Params: map[string]interface{}{
			"idempotency_key": req.IdempotencyKey,
			"target":          target,
			"rows_affected":   affected,
		},
	}
	id, err := o.cfg.Pending.Put(ctx, action, pendingaction.DefaultTTL)
	if err != nil {
		return nil, fmt.Errorf("暂存待确认操作失败: %w", err)
	}
	o.recordAudit(ctx, operations.OpMutate, target, req.SQL, req.IdempotencyKey,
		operations.StatusPendingConfirm,
		fmt.Sprintf("预计影响 %d 行（阈值 %d），已暂停待确认 pending_action_id=%s", affected, o.cfg.DangerRowThreshold, id),
		map[string]interface{}{"rows_affected": affected, "pending_action_id": id})

	return &SQLMutateResult{
		Status:          operations.StatusPendingConfirm,
		Target:          target,
		RowsAffected:    affected,
		PendingActionID: id,
		ConfirmToken:    action.Token,
		Message: fmt.Sprintf("该操作预计影响 %d 行（≥ 危险阈值 %d），已回滚并暂停。请通过确认卡片携带 token 恢复执行，%s 内有效。",
			affected, o.cfg.DangerRowThreshold, pendingaction.DefaultTTL),
		LatencyMs: time.Since(startTime).Milliseconds(),
	}, nil
}

// ExecuteConfirmedMutation 执行已经人工确认的写操作（Phase 5 confirm-resume 端点调用）。
// 前提：pending action 已通过 Store.Consume 原子取出（token 校验在 Consume 内完成）。
// 确认后不再做阈值分级，但幂等与审计仍然生效。
func (o *opTools) ExecuteConfirmedMutation(ctx context.Context, action *pendingaction.PendingAction) (*SQLMutateResult, error) {
	startTime := time.Now()
	if o.cfg.DB == nil {
		return nil, fmt.Errorf("写库未初始化")
	}
	if o.cfg.Audit == nil {
		return nil, fmt.Errorf("操作审计未启用，写操作被禁止")
	}

	idempotencyKey, _ := action.Params["idempotency_key"].(string)
	target, _ := action.Params["target"].(string)
	if target == "" {
		target = extractMutateTarget(action.SQL)
	}

	// 消费后依旧复核 DML/红线：pending 期间配置可能收紧
	if err := validateDML(action.SQL); err != nil {
		o.recordAudit(ctx, operations.OpMutate, target, action.SQL, idempotencyKey,
			operations.StatusRejected, err.Error(), nil)
		return &SQLMutateResult{Status: operations.StatusRejected, Target: target, Message: err.Error()}, nil
	}
	if o.isRedlineTable(target) {
		msg := fmt.Sprintf("表 %s 属于红线原始业务表，禁止修改", target)
		o.recordAudit(ctx, operations.OpMutate, target, action.SQL, idempotencyKey,
			operations.StatusRejected, msg, nil)
		return &SQLMutateResult{Status: operations.StatusRejected, Target: target, Message: msg}, nil
	}

	// 幂等：确认前若同键已成功执行（如重复确认），不重复写库
	tenantID := agentctx.MustGetTenantID(ctx)
	if idempotencyKey != "" {
		if prior, err := o.cfg.Audit.FindByIdempotencyKey(ctx, tenantID, idempotencyKey); err != nil {
			return nil, fmt.Errorf("幂等检查失败: %w", err)
		} else if prior != nil {
			return &SQLMutateResult{
				Status:    operations.StatusSuccess,
				Target:    prior.Target,
				Duplicate: true,
				Message:   fmt.Sprintf("幂等命中：该操作已执行（%s），本次未重复写库", prior.Result),
				LatencyMs: time.Since(startTime).Milliseconds(),
			}, nil
		}
	}

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tx := o.cfg.DB.WithContext(execCtx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("开启事务失败: %w", tx.Error)
	}
	res := tx.Exec(action.SQL)
	if res.Error != nil {
		tx.Rollback()
		o.recordAudit(ctx, operations.OpMutate, target, action.SQL, idempotencyKey,
			operations.StatusFailed, res.Error.Error(), nil)
		return nil, fmt.Errorf("执行失败: %w", res.Error)
	}
	if err := tx.Commit().Error; err != nil {
		o.recordAudit(ctx, operations.OpMutate, target, action.SQL, idempotencyKey,
			operations.StatusFailed, err.Error(), nil)
		return nil, fmt.Errorf("提交失败: %w", err)
	}
	auditParams := map[string]interface{}{"rows_affected": res.RowsAffected, "confirmed": true, "pending_action_id": action.ID}
	if uid, ok := agentctx.GetUserID(ctx); ok {
		// 确认人归因：审计须可追溯是谁批准了该危险操作
		auditParams["confirmed_by"] = uid
	}
	o.recordAudit(ctx, operations.OpMutate, target, action.SQL, idempotencyKey,
		operations.StatusSuccess, fmt.Sprintf("影响 %d 行（人工确认后执行）", res.RowsAffected),
		auditParams)

	return &SQLMutateResult{
		Status:       operations.StatusSuccess,
		Target:       target,
		RowsAffected: res.RowsAffected,
		Message:      "人工确认后执行成功",
		LatencyMs:    time.Since(startTime).Milliseconds(),
	}, nil
}
