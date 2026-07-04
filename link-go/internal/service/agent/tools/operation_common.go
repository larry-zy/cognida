// Package tools 操作工具公共层：注入、危险分级、审计留痕（Phase 4）。
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

	"gorm.io/gorm"

	agentctx "link/internal/model/agent"
	"link/internal/model/agent/operations"
	"link/internal/service/agent/framework"
	"link/internal/service/agent/pendingaction"
	"link/internal/service/agent/resultstore"
)

// ========================================
// 配置与注入
// ========================================

// DefaultDangerRowThreshold 危险影响行数默认阈值（保守默认，可配）：
// dry-run 影响行数达到该值的 DML 判定为危险级，暂停等待人机确认。
const DefaultDangerRowThreshold = 100

// ETLTablePrefix ETL 派生对象强制前缀（红线：派生新对象，不动原始表）。
const ETLTablePrefix = "agent_etl_"

// OperationConfig 操作工具配置（组合根注入）。
type OperationConfig struct {
	// DB 写库连接；nil 时 sql_mutate/etl_run 不可用。
	DB *gorm.DB
	// Audit 审计仓储；所有写/ETL/导出/拦截必须留痕。
	Audit operations.AuditRepository
	// Pending 待确认操作存储；nil 时危险操作直接拒绝（宁拒不闯）。
	Pending pendingaction.Store
	// RedlineTables 红线原始业务表（禁止 sql_mutate 修改），小写表名。
	RedlineTables []string
	// DangerRowThreshold 危险影响行数阈值；<=0 用 DefaultDangerRowThreshold。
	DangerRowThreshold int
	// ExportDir 导出文件目录；空则用系统临时目录下 link-agent-exports。
	ExportDir string
}

// opConfig 当前生效的操作工具配置。
var opConfig OperationConfig

// InitOperationTools 注入操作工具配置（组合根调用，与 InitSQLExecuteTool 对称）。
func InitOperationTools(cfg OperationConfig) {
	if cfg.DangerRowThreshold <= 0 {
		cfg.DangerRowThreshold = DefaultDangerRowThreshold
	}
	opConfig = cfg

	// 挂接硬工具门拦截审计（Phase 6 任务 7.4）：被拒调用与写/ETL/导出共用
	// agent_operation_audit 留痕（type=tool_gate, status=rejected, result=原因）。
	framework.SetToolBlockRecorder(func(ctx context.Context, call framework.BlockedToolCall) {
		recordAudit(ctx, operations.OpToolGate, call.Tool, "", "",
			operations.StatusRejected, call.Reason, map[string]interface{}{
				"skill": call.Skill,
				"scope": call.Scope,
			})
	})

	// 挂接子代理委派审计（Phase 7 任务 8.6）：委派与治理目录串联留痕
	// （type=delegate, target=子代理, params 携 goal/授予 scope/风险级）。
	framework.SetDelegationRecorder(func(ctx context.Context, rec framework.DelegationRecord) {
		recordAudit(ctx, operations.OpDelegate, rec.Agent, "", "",
			rec.Status, rec.Detail, map[string]interface{}{
				"goal":       rec.Goal,
				"scope":      rec.Scope,
				"risk_class": rec.RiskClass,
			})
	})
}

// GetPendingActionStore 取当前待确认操作存储（供 handler 层 confirm-resume 消费）。
func GetPendingActionStore() pendingaction.Store {
	return opConfig.Pending
}

// ========================================
// 审计留痕
// ========================================

// recordAudit 写入一条操作审计；审计失败不阻断主流程，但必须落日志。
func recordAudit(ctx context.Context, opType, target, sqlText, idempotencyKey, status, result string, params map[string]interface{}) {
	if opConfig.Audit == nil {
		log.Printf("[操作审计] 仓储未注入，审计丢失: type=%s target=%s status=%s", opType, target, status)
		return
	}
	paramsJSON := ""
	if len(params) > 0 {
		if b, err := json.Marshal(params); err == nil {
			paramsJSON = string(b)
		}
	}
	audit := &operations.OperationAudit{
		TenantID:       agentctx.MustGetTenantID(ctx),
		SessionID:      agentctx.MustGetSessionID(ctx),
		RequestID:      agentctx.MustGetRequestID(ctx),
		OperationType:  opType,
		Target:         target,
		SQLText:        sqlText,
		Params:         paramsJSON,
		IdempotencyKey: idempotencyKey,
		Status:         status,
		Result:         result,
	}
	if err := opConfig.Audit.Record(ctx, audit); err != nil {
		log.Printf("[操作审计] 写入失败: %v (type=%s target=%s status=%s)", err, opType, target, status)
	}
}

// RecordUnsupportedConfirmKind 留痕「已消费但类型不支持」的确认请求：
// pending action 是消费即失效的，进不了执行分支也必须可从审计追溯，防静默丢弃。
func RecordUnsupportedConfirmKind(ctx context.Context, action *pendingaction.PendingAction) {
	recordAudit(ctx, action.Kind, "", action.SQL, "",
		operations.StatusRejected,
		fmt.Sprintf("确认端点不支持的操作类型 %q（pending_action_id=%s），已消费未执行", action.Kind, action.ID),
		action.Params)
}

// operationOwner 当前会话的归属键（与 resultstore.OwnerKey 同构）。
func operationOwner(ctx context.Context) string {
	return resultstore.OwnerKey(agentctx.MustGetTenantID(ctx), agentctx.MustGetSessionID(ctx))
}

// ========================================
// DML 校验与目标表提取
// ========================================

// ddlKeywords 明确拒绝的 DDL/危险关键词（操作范围 = DML only）。
var ddlKeywords = []string{
	"CREATE", "ALTER", "DROP", "TRUNCATE", "RENAME",
	"GRANT", "REVOKE", "EXECUTE", "CALL",
}

// validateDML 校验 sql_mutate 提交的语句：仅 INSERT/UPDATE/DELETE 单语句，无注释。
func validateDML(sqlStr string) error {
	trimmed := strings.TrimSpace(sqlStr)
	if trimmed == "" {
		return fmt.Errorf("sql 不能为空")
	}
	upper := strings.ToUpper(trimmed)

	for _, kw := range ddlKeywords {
		if matched, _ := regexp.MatchString(fmt.Sprintf(`\b%s\b`, kw), upper); matched {
			return fmt.Errorf("不支持 DDL/危险语句（含 %s），仅允许 INSERT/UPDATE/DELETE", kw)
		}
	}
	if !strings.HasPrefix(upper, "INSERT") && !strings.HasPrefix(upper, "UPDATE") && !strings.HasPrefix(upper, "DELETE") {
		return fmt.Errorf("仅允许 INSERT/UPDATE/DELETE 语句")
	}
	if strings.Contains(upper, "--") || strings.Contains(upper, "/*") {
		return fmt.Errorf("不能包含 SQL 注释")
	}
	if inner := strings.TrimRight(trimmed, "; \t\n"); strings.Contains(inner, ";") {
		return fmt.Errorf("不能包含多语句")
	}

	// 多表写防护：红线校验只识别单一目标表，UPDATE ... JOIN / DELETE t1 FROM ... /
	// DELETE ... USING 等多表写法可携带第二写目标绕过红线，一律拒绝单表以外的形式。
	// （INSERT ... SELECT 的 JOIN 只读不写，不受此限。）
	switch {
	case strings.HasPrefix(upper, "UPDATE"):
		if !singleTableUpdateRe.MatchString(trimmed) {
			return fmt.Errorf("UPDATE 仅支持单表形式（UPDATE <表> SET ...），不支持 JOIN/多表/别名")
		}
	case strings.HasPrefix(upper, "DELETE"):
		if !singleTableDeleteRe.MatchString(trimmed) {
			return fmt.Errorf("DELETE 仅支持单表形式（DELETE FROM <表> [WHERE ...]），不支持 JOIN/USING/多表/别名")
		}
	}
	return nil
}

// identPat 单个表标识符（可反引号包裹）；tablePat 允许一级库名限定（db.table）。
const identPat = "(?:`[A-Za-z0-9_]+`|[A-Za-z0-9_]+)"
const tablePat = "(" + identPat + `(?:\.` + identPat + `)?)`

// mutateTargetRe 提取 DML 目标表：INSERT INTO x / UPDATE x / DELETE FROM x。
var mutateTargetRe = regexp.MustCompile(`(?i)^\s*(?:INSERT\s+(?:IGNORE\s+)?INTO|UPDATE(?:\s+IGNORE)?|DELETE\s+FROM)\s+` + tablePat)

// 单表 DML 形式白名单：UPDATE 的表引用区（SET 之前）与 DELETE 的 FROM 之后
// 必须恰好是一个表标识符，防止 JOIN/逗号多表/USING 携带隐藏写目标。
var (
	singleTableUpdateRe = regexp.MustCompile(`(?i)^\s*UPDATE(?:\s+IGNORE)?\s+` + tablePat + `\s+SET\b`)
	singleTableDeleteRe = regexp.MustCompile(`(?i)^\s*DELETE\s+FROM\s+` + tablePat + `\s*(?:WHERE\b|ORDER\s+BY\b|LIMIT\b|;?\s*$)`)
)

// extractMutateTarget 提取 DML 语句的目标表名（去反引号；含库名前缀时保留原样）。
func extractMutateTarget(sqlStr string) string {
	m := mutateTargetRe.FindStringSubmatch(sqlStr)
	if len(m) < 2 {
		return ""
	}
	return strings.ReplaceAll(m[1], "`", "")
}

// isRedlineTable 判断目标是否命中红线原始表（忽略库名前缀与大小写）。
func isRedlineTable(target string) bool {
	name := strings.ToLower(target)
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	for _, t := range opConfig.RedlineTables {
		if strings.ToLower(t) == name {
			return true
		}
	}
	return false
}

// etlTargetNameRe ETL 派生对象名白名单模式（前缀强校验 + 标识符字符集，防注入）。
var etlTargetNameRe = regexp.MustCompile(`^` + ETLTablePrefix + `[A-Za-z0-9_]+$`)

// validateETLTarget 校验 ETL 派生目标名：必须 agent_etl_ 前缀且为合法标识符。
func validateETLTarget(name string) error {
	if !strings.HasPrefix(name, ETLTablePrefix) {
		return fmt.Errorf("派生对象名必须以 %s 前缀开头（红线：不覆盖原始表），得到 %q", ETLTablePrefix, name)
	}
	if !etlTargetNameRe.MatchString(name) {
		return fmt.Errorf("派生对象名只允许字母/数字/下划线: %q", name)
	}
	return nil
}
