// Package tools 提供 etl_run 派生工具（Phase 4）。
package tools

import (
	"context"
	"fmt"
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
	return NewTypedBaseTool("etl_run",
		`把查询结果物化为派生表（ETL）。

安全限制：
- 只派生新对象，目标表名必须以 agent_etl_ 前缀开头（强校验）
- 绝不写入或修改任何源表
- 输入源二选一：sql（仅 SELECT）或 result_id（本会话的结果集引用）
- 所有执行与被拒都写入操作审计

参数：
- target_table: 派生目标表名（必需，agent_etl_ 前缀）
- sql: SELECT 输入源（可选）
- result_id: 结果集引用输入源（可选）
- replace: 目标已存在时是否替换（可选，默认 false）`,
		handler,
	)
}

// etlRun 执行 ETL 派生：前缀强校验 → 输入源解析 → 物化新表。
func (o *opTools) etlRun(ctx context.Context, req *ETLRunRequest) (*ETLRunResult, error) {
	startTime := time.Now()

	// 外部数据源会话红线：ETL 派生只面向业务库，外部数据源会话一律拒绝
	if dsID := agentctx.MustGetDatasourceID(ctx); dsID != "" {
		return nil, fmt.Errorf("当前会话选择了外部数据源（%s），外部数据源为只读，禁止 ETL 派生", dsID)
	}
	if o.cfg.DB == nil {
		return nil, fmt.Errorf("写库未初始化（etl_run 不可用）")
	}
	if o.cfg.Audit == nil {
		return nil, fmt.Errorf("操作审计未启用，ETL 操作被禁止")
	}

	// 1) 目标名前缀强校验（红线：只派生新对象，不碰原始表；拒绝也留痕）
	if err := validateETLTarget(req.TargetTable); err != nil {
		o.recordAudit(ctx, operations.OpETL, req.TargetTable, req.SQL, "",
			operations.StatusRejected, err.Error(), map[string]interface{}{"result_id": req.ResultID})
		return &ETLRunResult{
			Status:    operations.StatusRejected,
			Message:   err.Error(),
			LatencyMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	// 2) 输入源二选一
	hasSQL := strings.TrimSpace(req.SQL) != ""
	if hasSQL == (req.ResultID != "") {
		return nil, fmt.Errorf("输入源必须且只能提供一个：sql 或 result_id")
	}

	execCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// 3) 目标表已存在的处理（replace 只可能删 agent_etl_ 前缀表，前缀已校验）
	if req.Replace {
		if err := o.cfg.DB.WithContext(execCtx).Exec(
			fmt.Sprintf("DROP TABLE IF EXISTS `%s`", req.TargetTable)).Error; err != nil {
			o.recordAudit(ctx, operations.OpETL, req.TargetTable, req.SQL, "",
				operations.StatusFailed, err.Error(), nil)
			return nil, fmt.Errorf("替换旧派生表失败: %w", err)
		}
	}

	var rowCount int64
	var source string
	var err error
	if hasSQL {
		source = "sql"
		rowCount, err = o.etlFromSQL(execCtx, req.TargetTable, req.SQL)
	} else {
		source = "result_id:" + req.ResultID
		rowCount, err = o.etlFromResultID(execCtx, ctx, req.TargetTable, req.ResultID)
	}
	if err != nil {
		o.recordAudit(ctx, operations.OpETL, req.TargetTable, req.SQL, "",
			operations.StatusFailed, err.Error(), map[string]interface{}{"source": source})
		return nil, err
	}

	o.recordAudit(ctx, operations.OpETL, req.TargetTable, req.SQL, "",
		operations.StatusSuccess, fmt.Sprintf("派生 %d 行", rowCount),
		map[string]interface{}{"source": source, "row_count": rowCount, "replace": req.Replace})

	return &ETLRunResult{
		Status:      operations.StatusSuccess,
		TargetTable: req.TargetTable,
		RowCount:    rowCount,
		Source:      source,
		LatencyMs:   time.Since(startTime).Milliseconds(),
	}, nil
}

// etlFromSQL 用 SELECT 物化派生表：CREATE TABLE agent_etl_x AS (SELECT …)。
// 源 SQL 沿用只读校验（仅 SELECT/WITH），保证绝不写源表。
func (o *opTools) etlFromSQL(execCtx context.Context, target, sourceSQL string) (int64, error) {
	if err := validateSQL(sourceSQL); err != nil {
		return 0, fmt.Errorf("输入源 SQL 校验失败（仅允许 SELECT）: %w", err)
	}
	trimmed := strings.TrimRight(strings.TrimSpace(sourceSQL), "; \t\n")
	ddl := fmt.Sprintf("CREATE TABLE `%s` AS (%s)", target, trimmed)
	if err := o.cfg.DB.WithContext(execCtx).Exec(ddl).Error; err != nil {
		// 派生失败源自 Agent 自撰的 SELECT（列/表/语法/超时）：产出结构化可修复观察，
		// 用源 SELECT 检索定向列/表线索，引导子代理定向改写而非盲目重试。
		return 0, newRepairableSQLError(execCtx, o.writeQueryTarget(), sourceSQL, err, nil, "")
	}
	return o.countTableRows(execCtx, target)
}

// etlFromResultID 从 Result Store 取本会话结果集并物化为派生表。
func (o *opTools) etlFromResultID(execCtx, ownerCtx context.Context, target, resultID string) (int64, error) {
	if o.resultStore == nil {
		return 0, fmt.Errorf("结果存储未启用，无法按 result_id 派生，请改用 sql 输入源")
	}
	owner := resultstore.OwnerKey(agentctx.MustGetTenantID(ownerCtx), agentctx.MustGetSessionID(ownerCtx))
	stored, err := o.resultStore.Get(ownerCtx, owner, resultID)
	if err != nil {
		switch err {
		case resultstore.ErrNotFound:
			return 0, fmt.Errorf("result_id %s 不存在或已过期，请重新取数", resultID)
		case resultstore.ErrUnauthorized:
			return 0, fmt.Errorf("result_id %s 不属于当前会话", resultID)
		default:
			return 0, fmt.Errorf("读取结果存储失败: %w", err)
		}
	}
	if len(stored.Columns) == 0 {
		return 0, fmt.Errorf("结果集无列定义，无法建表")
	}

	// 按行数据推断列类型建表
	colDefs := make([]string, 0, len(stored.Columns))
	for _, col := range stored.Columns {
		colDefs = append(colDefs, fmt.Sprintf("`%s` %s", col, inferColumnType(col, stored.Rows)))
	}
	ddl := fmt.Sprintf("CREATE TABLE `%s` (%s)", target, strings.Join(colDefs, ", "))
	if err := o.cfg.DB.WithContext(execCtx).Exec(ddl).Error; err != nil {
		return 0, fmt.Errorf("派生表创建失败: %w", err)
	}

	// 批量插入
	if len(stored.Rows) > 0 {
		if err := o.insertRows(execCtx, target, stored.Columns, stored.Rows); err != nil {
			return 0, fmt.Errorf("派生表写入失败: %w", err)
		}
	}
	return int64(len(stored.Rows)), nil
}

// inferColumnType 根据首个非空取值推断列类型（number→DOUBLE，bool→TINYINT(1)，其余 TEXT）。
func inferColumnType(col string, rows []map[string]interface{}) string {
	for _, row := range rows {
		v, ok := row[col]
		if !ok || v == nil {
			continue
		}
		switch v.(type) {
		case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return "DOUBLE"
		case bool:
			return "TINYINT(1)"
		default:
			return "TEXT"
		}
	}
	return "TEXT"
}

// insertRows 按批插入行（每批 200 行，参数化绑定防注入）。
func (o *opTools) insertRows(execCtx context.Context, target string, columns []string, rows []map[string]interface{}) error {
	const batchSize = 200
	quoted := make([]string, len(columns))
	for i, c := range columns {
		quoted[i] = "`" + c + "`"
	}
	placeholderRow := "(" + strings.TrimRight(strings.Repeat("?,", len(columns)), ",") + ")"

	for start := 0; start < len(rows); start += batchSize {
		end := start + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[start:end]
		placeholders := make([]string, len(batch))
		args := make([]interface{}, 0, len(batch)*len(columns))
		for i, row := range batch {
			placeholders[i] = placeholderRow
			for _, c := range columns {
				args = append(args, row[c])
			}
		}
		// target 已由 validateETLTarget 正则白名单校验、列名反引号包裹，行值全部 ? 参数化绑定；
		// 此处标识符不可参数化，故用字符串拼接组装（等价于原 fmt.Sprintf，安全性不变）。
		stmt := "INSERT INTO `" + target + "` (" + strings.Join(quoted, ", ") +
			") VALUES " + strings.Join(placeholders, ", ")
		if err := o.cfg.DB.WithContext(execCtx).Exec(stmt, args...).Error; err != nil {
			return err
		}
	}
	return nil
}

// countTableRows 统计派生表行数。
func (o *opTools) countTableRows(execCtx context.Context, table string) (int64, error) {
	var count int64
	if err := o.cfg.DB.WithContext(execCtx).
		Raw("SELECT COUNT(*) FROM `" + table + "`").Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("统计派生表行数失败: %w", err)
	}
	return count, nil
}
