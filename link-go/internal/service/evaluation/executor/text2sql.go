// Package executor 提供评测执行器实现
package executor

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentctx "link/internal/model/agent"
	domeval "link/internal/model/evaluation"
)

// Text2SQLExecutor Text2SQL / SQL 生成评测执行器。
//
// 每条样本：跑被测 Agent 生成 SQL（取最后一次 sql_execute 调用），金标准 SQL 取
// QAPair.GoldSQL（为空回退 ReferenceAnswer——sql 数据集把金标准 SQL 存在 reference_answer 列）。
// 随后用 SQLRunner 只读执行「金标准 + 生成」两条 SQL，把双方结果集与 SQL 文本透传给
// Python：结构指标（sql_exact_match/sql_component_match）比对 SQL 文本，执行准确率
// （sql_execution_accuracy）无序比对两个结果集。生成 SQL 为空或执行失败不阻断整批——
// 该条对应指标缺席（而非记 0），与既有执行器「失败即 Success=false」语义一致。
type Text2SQLExecutor struct {
	agentService AgentService
	sqlRunner    SQLRunner
	timeout      time.Duration
}

// NewText2SQLExecutor 创建 Text2SQL 评测执行器。sqlRunner 可为 nil（此时仅算结构指标，
// 不执行 SQL，执行准确率缺席），使无外部数据源装配时仍可退化跑静态指标。
func NewText2SQLExecutor(agentService AgentService, sqlRunner SQLRunner, timeout time.Duration) *Text2SQLExecutor {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Text2SQLExecutor{
		agentService: agentService,
		sqlRunner:    sqlRunner,
		timeout:      timeout,
	}
}

// Type 返回执行器类型
func (e *Text2SQLExecutor) Type() domeval.EvaluationType {
	return domeval.EvaluationTypeSQL
}

// Execute 执行 Text2SQL 评测
func (e *Text2SQLExecutor) Execute(ctx context.Context, task *EvaluationTaskConfig, dataset []*QAPair) ([]*QAResult, error) {
	if task.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required for sql evaluation")
	}
	if _, err := e.agentService.GetAgent(ctx, task.AgentID); err != nil {
		return nil, fmt.Errorf("agent not found: %s, error: %w", task.AgentID, err)
	}

	results := make([]*QAResult, len(dataset))
	for i, qa := range dataset {
		// 金标准 SQL：优先 GoldSQL，回退 ReferenceAnswer（sql 数据集不新增列，金标准存 reference_answer）。
		goldSQL := strings.TrimSpace(qa.GoldSQL)
		if goldSQL == "" {
			goldSQL = strings.TrimSpace(qa.ReferenceAnswer)
		}
		results[i] = &QAResult{
			Question:        qa.Question,
			ReferenceAnswer: qa.ReferenceAnswer,
			RelevantPIDs:    qa.RelevantPIDs,
			GoldSQL:         goldSQL,
		}

		// 派生每条子 rid + 单条超时，跑被测 Agent 生成 SQL（失败路径也记录耗时/rid，便于查错）。
		qaCtx, cancel := context.WithTimeout(withItemRequestID(ctx, i), e.timeout)
		if rid, ok := agentctx.GetRequestID(qaCtx); ok {
			results[i].RequestID = rid
		}
		start := time.Now()
		chatResult, err := safeChat(qaCtx, e.agentService, task.AgentID, qa.Question)
		results[i].LatencyMs = time.Since(start).Milliseconds()

		if err != nil {
			cancel()
			results[i].Success = false
			results[i].Error = fmt.Sprintf("agent chat failed: %v", err)
			continue
		}

		applyAgentChatResult(results[i], chatResult)
		if chatResult != nil {
			results[i].GeneratedSQL = strings.TrimSpace(chatResult.GeneratedSQL)
		}

		// 只读执行金标准 + 生成 SQL，采集结果集供执行准确率比对（复用同一路由后的数据源）。
		// 单条 SQL 执行用 runSafely 包 recover：panic 不阻断整批，仅让该条结果集缺席。
		if e.sqlRunner != nil {
			if results[i].GoldSQL != "" {
				var rs *SQLResultSet
				if rerr := runSafely(func() error {
					r, e2 := e.sqlRunner.RunReadOnly(qaCtx, results[i].GoldSQL)
					rs = r
					return e2
				}); rerr == nil && rs != nil {
					results[i].GoldResultSet = rs.Rows
				}
			}
			if results[i].GeneratedSQL != "" {
				var rs *SQLResultSet
				if rerr := runSafely(func() error {
					r, e2 := e.sqlRunner.RunReadOnly(qaCtx, results[i].GeneratedSQL)
					rs = r
					return e2
				}); rerr == nil && rs != nil {
					results[i].ResultSet = rs.Rows
				}
			}
		}
		cancel()

		// 生成出 SQL 即视为该条成功产出（结构指标恒可算）；执行准确率视结果集是否取到。
		results[i].Success = results[i].GeneratedSQL != ""
		if !results[i].Success {
			results[i].Error = "agent did not produce SQL (no sql_execute tool call)"
		}
	}

	return results, nil
}

// GetTimeout 获取超时时间
func (e *Text2SQLExecutor) GetTimeout() time.Duration {
	return e.timeout
}
