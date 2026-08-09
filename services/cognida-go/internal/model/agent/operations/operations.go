// Package operations 定义数据操作（写/ETL/导出）的审计实体与仓储接口。
//
// 依赖方向：service → model ← repository。操作工具（service/agent/tools）
// 只依赖本包接口，MySQL 实现位于 repository/mysql/operation_audit.go。
package operations

import (
	"context"
	"time"
)

// 操作类型
const (
	OpMutate   = "mutate"    // sql_mutate DML 写库
	OpETL      = "etl"       // etl_run 派生新对象
	OpExport   = "export"    // data_export 按 result_id 导出
	OpToolGate  = "tool_gate" // 硬工具门/scope 拦截留痕（Phase 6）
	OpDelegate  = "delegate"  // 子代理委派留痕（Phase 7，携治理元数据 risk_class/scope）
	OpGuardrail = "guardrail" // 护栏拦截/脱敏/审批留痕（guardrail-runtime，携 event/tool）
)

// 操作结果状态
const (
	StatusSuccess        = "success"         // 已执行成功
	StatusRejected       = "rejected"        // 被安全规则拒绝（DDL/红线/前缀等）
	StatusPendingConfirm = "pending_confirm" // 危险操作暂停，等待人机确认
	StatusFailed         = "failed"          // 执行出错
)

// OperationAudit 一条操作审计记录：所有写/ETL/导出以及被拦截的操作都必须留痕。
type OperationAudit struct {
	ID             int64
	TenantID       int64
	SessionID      string
	RequestID      string
	OperationType  string // mutate / etl / export
	Target         string // 目标表 / 导出文件引用
	SQLText        string // 提交的 SQL（导出类为空）
	Params         string // 其余参数（JSON）
	IdempotencyKey string // 写操作幂等键；非写操作可为空
	Status         string // success / rejected / pending_confirm / failed
	Result         string // 影响行数 / 拒绝原因 / 文件引用 / 错误信息
	CreatedAt      time.Time
}

// AuditRepository 审计仓储接口。
type AuditRepository interface {
	// Record 追加一条审计记录（append-only）。
	Record(ctx context.Context, audit *OperationAudit) error
	// FindByIdempotencyKey 按租户 + 幂等键查最近一条成功记录；无则返回 (nil, nil)。
	FindByIdempotencyKey(ctx context.Context, tenantID int64, key string) (*OperationAudit, error)
}
