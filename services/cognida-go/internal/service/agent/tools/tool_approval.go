// Package tools 策略级写审批：复用 pending-confirm 机制（agent-guardrail-runtime 任务 4）。
//
// 当会话/skill 策略把写类工具（sql_mutate / data_export / etl_run）标记为需人工审批时，
// framework 硬工具门（gateToolCall→handleApprovalRequired）不执行工具，转而委托本处理器：
//   - 解析工具原始 JSON 参数，构建 pending_action（Kind = mutate/etl/export）；
//   - 复用 sql_mutate 危险级同一套 pendingaction.Store / ConfirmToken / 确认卡片，不另造审批通道；
//   - 回灌 LLM 一条 pending_confirm ToolMessage（含 pending_action_id + confirm_token）；
//   - 人工确认后经既有 confirm-resume 端点按 Kind 分派执行（见 ExecuteConfirmed*）。
//
// 与 sql_mutate 内建行数阈值门叠加：策略级审批在框架门（执行前）拦截，行数阈值门在
// 工具内（dry-run 后）拦截，二者共用同一 pending-confirm 通道，互不排斥。
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"cognida/internal/model/agent/operations"
	"cognida/internal/service/agent/framework"
	"cognida/internal/service/agent/pendingaction"
)

// approvalPendingPayload 写审批待确认回灌 LLM 的载荷（复用 sql_mutate pending_confirm 语义，
// 使 LLM/前端确认卡片走与危险级写完全一致的确认路径）。
type approvalPendingPayload struct {
	Status          string `json:"status"` // 恒为 pending_confirm
	Tool            string `json:"tool"`
	Target          string `json:"target,omitempty"`
	PendingActionID string `json:"pending_action_id"`
	ConfirmToken    string `json:"confirm_token"`
	Message         string `json:"message"`
}

// handleWriteApproval 是挂接到 framework.SetToolApprovalHandler 的策略级写审批处理器。
// 返回值是回灌 LLM 的 ToolMessage 载荷；error 表示审批通道自身异常（门降级为不执行 + 说明）。
func (o *opTools) handleWriteApproval(ctx context.Context, req framework.ApprovalRequest) (string, error) {
	if o.cfg.Pending == nil {
		// 宁拒不闯：无待确认存储时审批通道不可用。
		return "", fmt.Errorf("待确认操作存储未启用，无法发起写审批")
	}

	action, target, err := o.buildApprovalAction(ctx, req)
	if err != nil {
		return "", err
	}

	id, err := o.cfg.Pending.Put(ctx, action, pendingaction.DefaultTTL)
	if err != nil {
		return "", fmt.Errorf("暂存待审批操作失败: %w", err)
	}

	// 操作审计留痕（pending_confirm）：与 sql_mutate 危险级暂停同源，供 confirm-resume 追溯。
	idemKey, _ := action.Params["idempotency_key"].(string)
	o.recordAudit(ctx, action.Kind, target, action.SQL, idemKey,
		operations.StatusPendingConfirm,
		fmt.Sprintf("策略级写审批：工具 %s 已暂停待人工确认 pending_action_id=%s", req.Tool, id),
		action.Params)

	payload := approvalPendingPayload{
		Status:          operations.StatusPendingConfirm,
		Tool:            req.Tool,
		Target:          target,
		PendingActionID: id,
		ConfirmToken:    action.Token,
		Message: fmt.Sprintf("工具 %s 被策略标记为需人工审批，已暂停。请通过确认卡片携带 token 恢复执行，%s 内有效。",
			req.Tool, pendingaction.DefaultTTL),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化审批载荷失败: %w", err)
	}
	return string(b), nil
}

// buildApprovalAction 按工具类型解析原始参数、构建待确认动作，返回 action 与目标（供审计/回灌展示）。
// mutate 复用 ExecuteConfirmedMutation 依赖的 SQL + Params{idempotency_key,target}；
// etl/export 存原始参数 JSON，供 confirm-resume 重建请求走各自工具既有全量执行路径。
func (o *opTools) buildApprovalAction(ctx context.Context, req framework.ApprovalRequest) (*pendingaction.PendingAction, string, error) {
	owner := operationOwner(ctx)
	switch req.Tool {
	case "sql_mutate":
		var r SQLMutateRequest
		if err := json.Unmarshal([]byte(req.Arguments), &r); err != nil {
			return nil, "", fmt.Errorf("解析 sql_mutate 参数失败: %w", err)
		}
		if r.IdempotencyKey == "" {
			return nil, "", fmt.Errorf("sql_mutate 缺少 idempotency_key，无法发起写审批")
		}
		target := extractMutateTarget(r.SQL)
		return &pendingaction.PendingAction{
			Owner: owner,
			Kind:  operations.OpMutate,
			SQL:   r.SQL,
			Params: map[string]interface{}{
				"idempotency_key": r.IdempotencyKey,
				"target":          target,
				"approval":        true,
			},
		}, target, nil

	case "etl_run":
		var r ETLRunRequest
		if err := json.Unmarshal([]byte(req.Arguments), &r); err != nil {
			return nil, "", fmt.Errorf("解析 etl_run 参数失败: %w", err)
		}
		if r.TargetTable == "" {
			return nil, "", fmt.Errorf("etl_run 缺少 target_table，无法发起写审批")
		}
		return &pendingaction.PendingAction{
			Owner: owner,
			Kind:  operations.OpETL,
			SQL:   r.SQL,
			Params: map[string]interface{}{
				"arguments": req.Arguments,
				"target":    r.TargetTable,
				"approval":  true,
			},
		}, r.TargetTable, nil

	case "data_export":
		var r DataExportRequest
		if err := json.Unmarshal([]byte(req.Arguments), &r); err != nil {
			return nil, "", fmt.Errorf("解析 data_export 参数失败: %w", err)
		}
		if r.ResultID == "" {
			return nil, "", fmt.Errorf("data_export 缺少 result_id，无法发起写审批")
		}
		return &pendingaction.PendingAction{
			Owner: owner,
			Kind:  operations.OpExport,
			Params: map[string]interface{}{
				"arguments": req.Arguments,
				"result_id": r.ResultID,
				"approval":  true,
			},
		}, r.ResultID, nil

	default:
		return nil, "", fmt.Errorf("工具 %s 不支持策略级写审批", req.Tool)
	}
}

// ExecuteConfirmedETL 执行已人工确认的策略级审批 ETL（confirm-resume 端点按 Kind=etl 分派）。
// 从暂存的原始参数重建请求后走既有 etlRun 全量校验与执行（框架门已在审批阶段放行）。
func (o *opTools) ExecuteConfirmedETL(ctx context.Context, action *pendingaction.PendingAction) (*ETLRunResult, error) {
	req, err := decodeApprovalArguments[ETLRunRequest](action)
	if err != nil {
		return nil, err
	}
	return o.etlRun(ctx, req)
}

// ExecuteConfirmedExport 执行已人工确认的策略级审批导出（confirm-resume 端点按 Kind=export 分派）。
func (o *opTools) ExecuteConfirmedExport(ctx context.Context, action *pendingaction.PendingAction) (*DataExportResult, error) {
	req, err := decodeApprovalArguments[DataExportRequest](action)
	if err != nil {
		return nil, err
	}
	return o.dataExport(ctx, req)
}

// decodeApprovalArguments 从待确认动作重建原始工具请求（etl/export 审批走此路径）。
func decodeApprovalArguments[T any](action *pendingaction.PendingAction) (*T, error) {
	raw, _ := action.Params["arguments"].(string)
	if raw == "" {
		return nil, fmt.Errorf("待确认动作缺少原始参数，无法恢复执行")
	}
	var req T
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		return nil, fmt.Errorf("恢复审批动作参数失败: %w", err)
	}
	return &req, nil
}
