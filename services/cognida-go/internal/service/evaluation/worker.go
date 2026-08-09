// Package evaluation 提供评测系统应用层实现
package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	agentctx "cognida/internal/model/agent"
	domeval "cognida/internal/model/evaluation"
	"cognida/internal/service/evaluation/executor"

	"github.com/google/uuid"
)

const (
	// DefaultMaxRetries 默认最大重试次数
	DefaultMaxRetries = 3
	// DefaultSlotCheckInterval 槽位检查间隔
	DefaultSlotCheckInterval = 1 * time.Second
	// MaxTaskTimeout 最大任务超时时间
	MaxTaskTimeout = 30 * time.Minute
)

// 类型别名，用于简化代码
type (
	DomainEvaluationTaskConfig = domeval.EvaluationTaskConfig
)

// EvaluationWorker 评测 Worker
type EvaluationWorker struct {
	queue         domeval.TaskQueue
	progressCache domeval.ProgressWriter
	pythonClient  *PythonEvaluationClient
	datasetLoader *DatasetLoader
	registry      *executor.ExecutorRegistry
	taskRepo      domeval.EvaluationTaskRepository
	resultRepo    domeval.EvaluationResultRepository
	stopCh        chan struct{}
	wg            sync.WaitGroup
	running       bool
	mu            sync.RWMutex

	// baseCtx/cancel 给所有在途任务一个可取消的根 context：Stop() 调 cancel() 能中断
	// 正在跑的被测 Agent（否则任务只能等自身超时或跑完，进程无法优雅停机）。
	baseCtx context.Context
	cancel  context.CancelFunc
}

// WorkerConfig Worker 配置
type WorkerConfig struct {
	MaxConcurrent int           // 最大并发数
	PollTimeout   time.Duration // 队列轮询超时
	MaxRetries    int           // 最大重试次数
}

// NewWorker 创建评测 Worker
func NewWorker(
	queue domeval.TaskQueue,
	progressCache domeval.ProgressWriter,
	pythonClient *PythonEvaluationClient,
	datasetLoader *DatasetLoader,
	registry *executor.ExecutorRegistry,
	taskRepo domeval.EvaluationTaskRepository,
	resultRepo domeval.EvaluationResultRepository,
	config *WorkerConfig,
) *EvaluationWorker {
	// 队列与进度缓存由 Wire 层装配（含并发限制等配置），经领域端口注入，
	// Worker 不再直接依赖 infrastructure 实现。
	_ = config

	baseCtx, cancel := context.WithCancel(context.Background())
	return &EvaluationWorker{
		queue:         queue,
		progressCache: progressCache,
		pythonClient:  pythonClient,
		datasetLoader: datasetLoader,
		registry:      registry,
		taskRepo:      taskRepo,
		resultRepo:    resultRepo,
		stopCh:        make(chan struct{}),
		baseCtx:       baseCtx,
		cancel:        cancel,
	}
}

// Run 启动 Worker 主循环
func (w *EvaluationWorker) Run() error {
	log.Printf("[Worker] Starting worker...")
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		log.Printf("[Worker] Worker already running")
		return fmt.Errorf("worker is already running")
	}
	w.running = true
	w.mu.Unlock()

	// 启动恢复：把上次进程被杀（如部署重启）时遗留的在途任务重新入队，
	// 并清零可能泄漏的并发槽位计数。必须在 workerLoop 之前、同步执行，
	// 避免恢复入队与正常消费竞争，也确保 count 清零时本进程尚无 worker 持槽。
	w.recoverStuckTasks(context.Background())

	log.Printf("[Worker] Starting worker loop in goroutine")
	w.wg.Add(1)
	go w.workerLoop()

	log.Printf("[Worker] Worker started successfully")
	return nil
}

// recoverStuckTasks 启动时恢复被中断的评测任务。
//
// 背景：executeTask 全程不写 DB 的 running 状态（只写 Redis 进度），任务在 DB 中
// 从 pending 直接跳到 completed/failed。若进程在执行途中被杀（部署重启等），任务已被
// BRPOP 移出队列却永远停在 pending，无自动重试 —— 这正是孤儿任务的成因。
//
// 恢复策略（以 DB 为准）：扫描 DB 中未完结（pending/running）的任务，凡「已不在队列里」的
// 一律重新入队。仍在队列中的 pending 任务会被正常消费，跳过以免重复入队 → 重复执行。
func (w *EvaluationWorker) recoverStuckTasks(ctx context.Context) {
	// 回收上次进程被杀泄漏的并发槽位计数（此刻本进程尚无 worker 持槽，清零安全）。
	if err := w.queue.ResetSlots(ctx); err != nil {
		log.Printf("[Worker][Recover] reset slots failed: %v", err)
	}

	// 当前仍排队中的任务 ID 集合，用于去重。
	queued := make(map[string]struct{})
	if ids, err := w.queue.PendingIDs(ctx); err != nil {
		log.Printf("[Worker][Recover] read queue failed: %v", err)
	} else {
		for _, id := range ids {
			queued[id] = struct{}{}
		}
	}

	var recovered int
	for _, st := range []domeval.TaskStatus{domeval.TaskStatusPending, domeval.TaskStatusRunning} {
		status := st
		// 分页扫描直至取尽：taskRepo.List 会把 PageSize 收敛到 100，单页取不完 >100 的孤儿会漏恢复
		// （原写死 PageSize:1000 被静默截断到 100，第 101 个及以后的孤儿永远卡死）。逐页翻到累计
		// 覆盖 total 或空页为止。
		const recoverPageSize = 100
		for page := 1; ; page++ {
			tasks, total, err := w.taskRepo.List(ctx, &domeval.TaskFilter{
				Status:   &status,
				Page:     page,
				PageSize: recoverPageSize,
			})
			if err != nil {
				log.Printf("[Worker][Recover] list %s tasks (page %d) failed: %v", status, page, err)
				break
			}
			if len(tasks) == 0 {
				break // 空页兜底，防止 total 与实际不一致时死循环
			}
			for _, t := range tasks {
				if t == nil {
					continue
				}
				if _, ok := queued[t.ID]; ok {
					continue // 仍在队列里，会被正常消费
				}
				// running 说明有进程认领了该任务（executeTask 开头置 running 并刷新 updated_at 当作轻量租约）。
				// 租约判定：仅当租约陈旧（updated_at 早于 now-MaxTaskTimeout）才视为孤儿、重置回 pending 并
				// 重新入队；否则认为仍有进程在执行，跳过——避免多进程/重叠恢复把在途任务重复入队、进而重复
				// 执行、互删结果行。pending 任务无租约概念，照常入队。
				if t.Status == domeval.TaskStatusRunning {
					if time.Since(t.UpdatedAt) < MaxTaskTimeout {
						log.Printf("[Worker][Recover] task %s still running (lease fresh, updated %s ago), skip", t.ID, time.Since(t.UpdatedAt).Round(time.Second))
						continue
					}
					if err := w.taskRepo.UpdateStatus(ctx, t.ID, domeval.TaskStatusPending); err != nil {
						log.Printf("[Worker][Recover] reset task %s to pending failed: %v", t.ID, err)
					}
				}
				if err := w.queue.Enqueue(ctx, t.ID); err != nil {
					log.Printf("[Worker][Recover] re-enqueue task %s failed: %v", t.ID, err)
					continue
				}
				recovered++
				log.Printf("[Worker][Recover] re-enqueued stuck task %s (was %s)", t.ID, status)
			}
			if int64(page*recoverPageSize) >= total {
				break // 已覆盖全部记录
			}
		}
	}

	if recovered > 0 {
		log.Printf("[Worker][Recover] re-enqueued %d stuck task(s) on startup", recovered)
	} else {
		log.Printf("[Worker][Recover] no stuck tasks to recover")
	}
}

// workerLoop Worker 主循环
func (w *EvaluationWorker) workerLoop() {
	log.Printf("[Worker] Worker loop started")
	defer func() {
		log.Printf("[Worker] Worker loop stopped")
		w.wg.Done()
	}()

	// 指数退避：当队列为空时逐渐增加等待时间
	var emptyQueueCount int
	var backoffDuration = DefaultSlotCheckInterval
	const maxBackoff = 10 * time.Second

	for {
		select {
		case <-w.stopCh:
			log.Printf("[Worker] Received stop signal")
			return
		default:
			// 检查是否有停止信号
		}

		// 尝试获取槽位
		acquired, err := w.queue.AcquireSlot(context.Background())
		if err != nil {
			log.Printf("[Worker] Failed to acquire slot: %v", err)
			time.Sleep(backoffDuration)
			continue
		}

		if !acquired {
			// 槽位已满，等待后重试
			time.Sleep(backoffDuration)
			continue
		}

		// 从队列获取任务
		taskID, err := w.dequeue()
		if err != nil {
			log.Printf("[Worker] Dequeue error: %v", err)
			_ = w.queue.ReleaseSlot(context.Background()) // best-effort 释放，失败无从补救
			time.Sleep(backoffDuration)
			continue
		}

		if taskID == "" {
			// 队列为空，释放槽位并使用退避策略
			_ = w.queue.ReleaseSlot(context.Background()) // best-effort 释放，失败无从补救
			emptyQueueCount++

			// 指数退避：连续空队列时增加等待时间
			if emptyQueueCount > 3 {
				backoffDuration = time.Duration(float64(backoffDuration) * 1.5)
				if backoffDuration > maxBackoff {
					backoffDuration = maxBackoff
				}
			}
			time.Sleep(backoffDuration)
			continue
		}

		// 有任务了，重置退避时间
		emptyQueueCount = 0
		backoffDuration = DefaultSlotCheckInterval

		log.Printf("[Worker] Got task from queue: %s", taskID)

		// 在 goroutine 中执行任务
		w.wg.Add(1)
		go func(taskID string) {
			defer w.wg.Done()
			defer func() {
				_ = w.queue.ReleaseSlot(context.Background()) // best-effort 释放，失败无从补救
			}()
			// B1: 兜底 recover。后台 goroutine 里的 panic（单条坏 QA、被测 Agent、指标计算等）
			// 若不拦截会直接带崩整个进程、拖垮全部并发任务。此处捕获后打印堆栈并把任务标记 failed，
			// 使故障隔离在单个任务内。recover 需注册在最内层（最后 defer），先于 ReleaseSlot/wg.Done 执行。
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[Worker] task %s panicked: %v\n%s", taskID, r, debug.Stack())
					w.handleTaskError(context.Background(), taskID, fmt.Errorf("task panicked: %v", r), 0)
				}
			}()

			w.executeTask(taskID)
		}(taskID)
	}
}

// dequeue 从队列取出任务
func (w *EvaluationWorker) dequeue() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	taskID, err := w.queue.Dequeue(ctx)
	if err != nil {
		log.Printf("[Worker][Dequeue] Error: %v", err)
		return "", err
	}
	if taskID != "" {
		log.Printf("[Worker][Dequeue] Got task: %s", taskID)
	}
	return taskID, nil
}

// executeTask 执行评测任务
func (w *EvaluationWorker) executeTask(taskID string) {
	// 注入 task 级 request_id，使本次评测的全链路（被测 Agent 运行→其 LLM/工具调用→
	// audit_logs/Loki→跨进程调 Python 指标计算的 X-Request-ID）统一挂在同一 rid 下，
	// 可按 rid 检索一整次评测。rid 内嵌 taskID 便于反查，尾缀 uuid 区分同一任务的多次重跑。
	// 被测 Agent 此前跑在裸 context.Background() 上，无 rid，运行完全脱离追踪链路。
	requestID := fmt.Sprintf("%s-%s", taskID, uuid.New().String()[:8])

	// B4: 任务根 ctx 从 baseCtx 派生并叠加 MaxTaskTimeout（真实 30min 上限），再叠加 request_id。
	// 此前用裸 context.Background() → MaxTaskTimeout 零引用、Stop() 无法取消在途任务。
	// baseCtx 为空（测试直接构造 worker 字面量）时回退 Background，保持向后兼容。
	baseCtx := w.baseCtx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	runCtx, cancel := context.WithTimeout(baseCtx, MaxTaskTimeout)
	defer cancel()
	ctx := agentctx.WithRequestID(runCtx, requestID)
	log.Printf("[Worker] task %s trace request_id=%s", taskID, requestID)

	// B3: 置 running 并刷新 updated_at 当作轻量租约，供 recoverStuckTasks 区分「在途」与陈旧孤儿，
	// 避免多进程/重叠恢复重复执行、互删结果行。失败仅记日志，不阻断执行（loadTask 会再校验存在性）。
	if err := w.taskRepo.UpdateStatus(ctx, taskID, domeval.TaskStatusRunning); err != nil {
		log.Printf("[Worker] task %s mark running failed: %v", taskID, err)
	}

	// 更新进度：开始加载
	w.updateProgress(ctx, taskID, &domeval.Progress{
		Stage:      domeval.StageLoading,
		Current:    0,
		Total:      100,
		Message:    "Loading dataset...",
		RetryCount: 0,
	})

	// 1. 从数据库加载任务
	task, err := w.loadTask(ctx, taskID)
	if err != nil {
		w.handleTaskError(ctx, taskID, fmt.Errorf("failed to load task: %w", err), 0)
		return
	}

	// 2. 解析配置
	config, err := w.parseTaskConfig(task)
	if err != nil {
		w.handleTaskError(ctx, taskID, fmt.Errorf("failed to parse config: %w", err), 0)
		return
	}

	// 2.1 数据源上下文：让评测跑的 Agent 与线上执行的 Agent 走同一条数据链路。
	// 生产在 agent_handler 用 WithDatasourceID 把查询类工具路由到会话选定的外部数据源
	// （如 ecommerce_demo）；评测侧此前只挂 request_id，被测 Agent 落到默认业务库，
	// 而数据集 golden 又是基于外部库算的 → 指标必然失真。此处从任务 config 读 datasource_id
	// 注入 ctx（并补 tenant_id/tool_scope，外部数据源 Acquire 依赖租户、评测只读），
	// 使 sql_execute/get_schema 等工具查到与 golden 同一个库。为空则保持查业务库，向后兼容。
	if dsID := configString(config.Config, "datasource_id"); dsID != "" {
		tenantID := configTenantID(config.Config, 1)
		ctx = agentctx.WithDatasourceID(ctx, dsID)
		ctx = agentctx.WithTenantID(ctx, tenantID)
		ctx = agentctx.WithToolScope(ctx, "read")
		log.Printf("[Worker] task %s routed to datasource_id=%s (tenant=%d)", taskID, dsID, tenantID)
	}

	// 3. 加载数据集
	dataset, err := w.datasetLoader.Load(ctx, config.DatasetID)
	if err != nil {
		w.handleTaskError(ctx, taskID, fmt.Errorf("failed to load dataset: %w", err), 0)
		return
	}

	totalQAs := len(dataset.QAPairs)

	// 更新进度：开始生成
	w.updateProgress(ctx, taskID, &domeval.Progress{
		Stage:      domeval.StageGeneration,
		Current:    0,
		Total:      totalQAs,
		Message:    "Generating answers...",
		RetryCount: 0,
	})

	// 4. 获取执行器
	log.Printf("[Worker] Getting executor for task %s, type: %s", taskID, config.Type)
	exec, err := w.registry.Get(config.Type)
	if err != nil {
		log.Printf("[Worker] Failed to get executor: %v", err)
		w.handleTaskError(ctx, taskID, fmt.Errorf("failed to get executor: %w", err), 0)
		return
	}
	log.Printf("[Worker] Got executor: %s", exec.Type())

	// 5. 执行评测
	log.Printf("[Worker] Executing evaluation for task %s with %d QAs", taskID, len(dataset.QAPairs))
	domainQAPairs := convertQAPairsToDomain(dataset.QAPairs)
	domainResults, err := exec.Execute(ctx, config, domainQAPairs)
	if err != nil {
		log.Printf("[Worker] Execution failed: %v", err)
		// 执行失败，标记为错误
		w.handleTaskError(ctx, taskID, fmt.Errorf("execution failed: %w", err), 0)
		return
	}
	log.Printf("[Worker] Execution completed, got %d results", len(domainResults))

	appResults := convertQAResultsToApp(domainResults)

	// 5.1 打分前先落库原始运行产物（答案/轨迹/运行时指标）。
	// 被测 Agent 的一次运行可能横跨十几条 QA、耗时数分钟，若把落库放在算指标之后，
	// 一旦 Python(:18888) 计分不可用就会连同整轮昂贵的 Agent 执行一起丢弃、被迫全量重跑。
	// 此处先做「幂等替换」持久化（delete+insert），既保住产物，又让后续重跑不会重复插行。
	if err := w.persistResults(ctx, taskID, appResults); err != nil {
		w.handleTaskError(ctx, taskID, fmt.Errorf("failed to persist raw results: %w", err), 0)
		return
	}

	// 更新进度：开始计算指标
	w.updateProgress(ctx, taskID, &domeval.Progress{
		Stage:      domeval.StageEvaluation,
		Current:    totalQAs,
		Total:      totalQAs,
		Message:    "Computing metrics...",
		RetryCount: 0,
	})

	// 6. 计算指标
	evalResult, err := w.computeMetrics(ctx, config, appResults)
	if err != nil {
		// 原始产物已在 5.1 落库，不会丢；仅标记任务失败，重跑时幂等覆盖。
		w.handleTaskError(ctx, taskID, fmt.Errorf("failed to compute metrics: %w", err), 0)
		return
	}

	// 7. 保存结果
	if err := w.saveResults(ctx, taskID, evalResult); err != nil {
		w.handleTaskError(ctx, taskID, fmt.Errorf("failed to save results: %w", err), 0)
		return
	}

	// 更新进度：完成
	w.updateProgress(ctx, taskID, &domeval.Progress{
		Stage:      domeval.StageCompleted,
		Current:    totalQAs,
		Total:      totalQAs,
		Message:    "Evaluation completed",
		RetryCount: 0,
	})
}

// loadTask 从数据库加载任务
func (w *EvaluationWorker) loadTask(ctx context.Context, taskID string) (*domeval.EvaluationTask, error) {
	return w.taskRepo.FindByID(ctx, taskID)
}

// parseTaskConfig 解析任务配置
func (w *EvaluationWorker) parseTaskConfig(task *domeval.EvaluationTask) (*domeval.EvaluationTaskConfig, error) {
	// 从 domain 实体构建配置
	config := &domeval.EvaluationTaskConfig{
		DatasetID:       task.DatasetID,
		Type:            task.Type,
		KnowledgeBaseID: task.KnowledgeBaseID,
		AgentID:         task.AgentID,
		ModelID:         task.ModelID,
	}

	// 解析 Config (json.RawMessage -> map[string]interface{})
	if len(task.Config) > 0 {
		var configMap map[string]interface{}
		if err := json.Unmarshal(task.Config, &configMap); err != nil {
			return nil, fmt.Errorf("failed to parse config: %w", err)
		}
		config.Config = configMap
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// configString 从任务 config map 安全读取字符串值（缺失/类型不符返回空串）。
func configString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// configTenantID 从任务 config 读取 tenant_id（JSON 数字解析为 float64，也兼容字符串），
// 缺省返回 def。外部数据源 Acquire 依赖 ctx 中的 tenant，评测默认落 dev 租户 1。
func configTenantID(m map[string]interface{}, def int64) int64 {
	if m == nil {
		return def
	}
	switch n := m["tenant_id"].(type) {
	case float64:
		if n > 0 {
			return int64(n)
		}
	case string:
		if parsed, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64); err == nil && parsed > 0 {
			return parsed
		}
	}
	return def
}

// computeMetrics 计算评测指标
func (w *EvaluationWorker) computeMetrics(ctx context.Context, config *DomainEvaluationTaskConfig, qaResults []*QAResult) (*EvaluationResult, error) {
	// 准备请求
	items := make([]*ComputeItem, len(qaResults))
	for i, result := range qaResults {
		items[i] = &ComputeItem{
			Question:          result.Question,
			ReferenceAnswer:   result.ReferenceAnswer,
			GeneratedAnswer:   result.GeneratedAnswer,
			RetrievedPIDs:     result.RetrievedPIDs,
			RelevantPIDs:      result.RelevantPIDs,
			RetrievedContexts: result.RetrievedChunks, // 分块正文，供忠实度/上下文相关性/噪声比计算
			// Agent 轨迹与期望标注（QA/RAG 为空，不影响既有指标）
			ExpectedTools: result.ExpectedTools,
			ExpectedSteps: result.ExpectedSteps,
			ToolsUsed:     result.ToolsUsed,
			Trajectory:    result.Trajectory,
			TotalSteps:    result.TotalSteps,
			// Text2SQL 评测：金标准/生成 SQL 文本 + 双方结果集，供 Python 算结构与执行准确率
			// （QA/RAG/Agent 为空，不影响既有指标）。
			GoldSQL:       result.GoldSQL,
			GeneratedSQL:  result.GeneratedSQL,
			ResultSet:     result.ResultSet,
			GoldResultSet: result.GoldResultSet,
		}
	}

	// 获取评分器配置
	graders := []string{"rouge", "bleu"}
	if config.Config != nil {
		if g, ok := config.Config["graders"].([]interface{}); ok {
			graders = make([]string, len(g))
			for i, v := range g {
				if s, ok := v.(string); ok {
					graders[i] = s
				}
			}
		}
	}

	// Agent 类型：注入 Agent 家族评分器，命中 Python compute_agent_metrics 专属路径
	// （tool_selection/tool_order/trajectory/step_efficiency/answer_accuracy）。
	// 用户若未在 config.graders 指定，仅有 rouge/bleu 会漏算轨迹类核心指标。
	if config.Type == domeval.EvaluationTypeAgent {
		graders = ensureAgentGraders(graders)
	}

	// SQL 类型：注入 SQL 家族评分器，命中 Python compute_sql_metrics 专属路径
	// （sql_exact_match/sql_component_match/sql_execution_accuracy）。
	if config.Type == domeval.EvaluationTypeSQL {
		graders = ensureSQLGraders(graders)
	}

	// 构建 LLM Judge 配置
	var llmJudgeConfig map[string]interface{}
	if config.Config != nil {
		if lj, ok := config.Config["llm_judge"].(map[string]interface{}); ok {
			llmJudgeConfig = lj
		}
	}

	req := &ComputeMetricsRequest{
		Items:    items,
		Graders:  graders,
		LLMJudge: llmJudgeConfig,
	}

	// 调用 Python 服务计算指标
	resp, err := w.pythonClient.ComputeMetrics(ctx, req)
	if err != nil {
		return nil, err
	}

	// Python 会把「请求了但注册表无对应 grader」的指标名回报在 Unsupported 里而非静默丢弃；
	// Go 侧此前只解析不消费 → 拼错/未注册的评分器名被无声吞掉，指标缺失却无人知。此处显式告警。
	if len(resp.Unsupported) > 0 {
		log.Printf("[Worker] unsupported graders ignored by python service: %v", resp.Unsupported)
	}

	// 合并结果
	evalResult := &EvaluationResult{
		DatasetID:       config.DatasetID,
		KnowledgeBaseID: config.KnowledgeBaseID,
		ModelID:         config.ModelID,
		QAResults:       qaResults,
	}

	// 填充指标
	w.fillMetrics(evalResult, resp)

	// Agent / SQL 类型：补充由 Go 执行器采集的运行时基础指标（耗时/Token/LLM 调用次数/成功率）。
	// 两者都经被测 Agent 产出（Text2SQLExecutor 同样 applyAgentChatResult 采集 token/调用数），
	// 需在 fillMetrics 之后执行——后者会用 Python 结果整体替换逐条 Scores，此处再合并注入。
	if config.Type == domeval.EvaluationTypeAgent || config.Type == domeval.EvaluationTypeSQL {
		augmentRuntimeMetrics(evalResult)
	}

	return evalResult, nil
}

// augmentRuntimeMetrics 为 Agent / SQL 评测补充运行时基础指标：逐条把耗时/Token/LLM 调用次数
// 注入 QAResult.Scores，并在任务级 evalResult.Scores 汇总均值 + 成功率。
// 这些指标由 Go 执行器直接采集（非 Python 计算），全部走既有 scores JSON 列持久化，无需迁移。
func augmentRuntimeMetrics(evalResult *EvaluationResult) {
	if evalResult == nil || len(evalResult.QAResults) == 0 {
		return
	}
	var sumLatency, sumTokens, sumCalls float64
	var successCnt, n int
	for _, qa := range evalResult.QAResults {
		if qa == nil {
			continue
		}
		n++
		if qa.Scores == nil {
			qa.Scores = make(map[string]float64, 3)
		}
		qa.Scores["latency_ms"] = float64(qa.LatencyMs)
		qa.Scores["tokens_used"] = float64(qa.TokensUsed)
		qa.Scores["llm_calls"] = float64(qa.LLMCalls)
		sumLatency += float64(qa.LatencyMs)
		sumTokens += float64(qa.TokensUsed)
		sumCalls += float64(qa.LLMCalls)
		if qa.Success {
			successCnt++
		}
	}
	if n == 0 {
		return
	}
	if evalResult.Scores == nil {
		evalResult.Scores = make(map[string]float64, 4)
	}
	evalResult.Scores["latency_ms"] = sumLatency / float64(n)
	evalResult.Scores["tokens_used"] = sumTokens / float64(n)
	evalResult.Scores["llm_calls"] = sumCalls / float64(n)
	evalResult.Scores["success_rate"] = float64(successCnt) / float64(n)
}

// agentGraders 是 Agent 评测默认注入的家族评分器：命中其中任一名即触发 Python
// compute_agent_metrics 分流，产出 answer_accuracy/tool_selection/tool_order/step_efficiency。
//
// 刻意不含 trajectory_match：其 exact_match 要求「实际步骤序列 == 期望序列」完全相等，而数据集的
// expected_steps/expected_tools 是最小锚点集（convert_eval_datasets.py 只标必需工具），实际轨迹必然
// 更长，exact_match 结构性恒为 0，会无差别拉低所有样本的轨迹分、误导判读。用户如确有完整期望轨迹
// 标注，仍可在 config.graders 显式请求 trajectory_match（Python _AGENT_GRADERS 仍保留该算子）。
var agentGraders = []string{
	"answer_accuracy",
	"tool_selection",
	"tool_order",
	"step_efficiency",
}

// ensureAgentGraders 幂等地把 Agent 家族评分器并入现有列表（保留用户在 config 中显式指定的其它评分器如 rouge/bleu）。
func ensureAgentGraders(graders []string) []string {
	seen := make(map[string]struct{}, len(graders))
	for _, g := range graders {
		seen[g] = struct{}{}
	}
	for _, g := range agentGraders {
		if _, ok := seen[g]; !ok {
			graders = append(graders, g)
			seen[g] = struct{}{}
		}
	}
	return graders
}

// sqlGraders 与 Python fastapi_app._SQL_GRADERS 保持一致：命中其中任一名即触发
// compute_sql_metrics 分流，产出 sql_exact_match/sql_component_match/sql_execution_accuracy。
var sqlGraders = []string{
	"sql_exact_match",
	"sql_component_match",
	"sql_execution_accuracy",
}

// sqlDroppedGraders 是 SQL 评测中应剔除的生成类评分器：SQL 数据集把金标准 SQL 存在
// reference_answer，被测 Agent 产出的是散文答案，rouge/bleu 会拿「金标准 SQL 文本」比「散文回答」
// ——口径完全错位、纯噪声，还会污染 SQL 任务详情页。SQL 结构/执行正确性由 sql_* 家族负责。
var sqlDroppedGraders = map[string]struct{}{
	"rouge": {}, "bleu": {},
	"rouge_1": {}, "rouge_2": {}, "rouge_l": {},
	"bleu_1": {}, "bleu_2": {}, "bleu_4": {},
}

// ensureSQLGraders 幂等地把 SQL 家族评分器并入现有列表，并剔除对 SQL 无意义的生成类评分器
// （rouge/bleu）。保留用户显式指定的其它非生成评分器。
func ensureSQLGraders(graders []string) []string {
	seen := make(map[string]struct{}, len(graders))
	filtered := graders[:0:0] // 新底层数组，避免原地改写破坏调用方切片
	for _, g := range graders {
		if _, drop := sqlDroppedGraders[g]; drop {
			continue // 剔除生成类评分器
		}
		if _, ok := seen[g]; ok {
			continue
		}
		seen[g] = struct{}{}
		filtered = append(filtered, g)
	}
	for _, g := range sqlGraders {
		if _, ok := seen[g]; !ok {
			filtered = append(filtered, g)
			seen[g] = struct{}{}
		}
	}
	return filtered
}

// fillMetrics 填充指标
func (w *EvaluationWorker) fillMetrics(evalResult *EvaluationResult, resp *ComputeMetricsResponse) {
	// 填充聚合指标 - 检索指标
	if val, ok := resp.Aggregate["precision"]; ok {
		evalResult.Precision = &val
	}
	if val, ok := resp.Aggregate["recall"]; ok {
		evalResult.Recall = &val
	}
	if val, ok := resp.Aggregate["ndcg"]; ok {
		evalResult.NDCG = &val
	}
	if val, ok := resp.Aggregate["mrr"]; ok {
		evalResult.MRR = &val
	}
	if val, ok := resp.Aggregate["map"]; ok {
		evalResult.MAP = &val
	}

	// 填充聚合指标 - 生成指标
	if val, ok := resp.Aggregate["rouge_1"]; ok {
		evalResult.ROUGE1 = &val
	}
	if val, ok := resp.Aggregate["rouge_2"]; ok {
		evalResult.ROUGE2 = &val
	}
	if val, ok := resp.Aggregate["rouge_l"]; ok {
		evalResult.ROUGEL = &val
	}
	if val, ok := resp.Aggregate["bleu_1"]; ok {
		evalResult.BLEU1 = &val
	}
	if val, ok := resp.Aggregate["bleu_2"]; ok {
		evalResult.BLEU2 = &val
	}
	if val, ok := resp.Aggregate["bleu_4"]; ok {
		evalResult.BLEU4 = &val
	}

	// 填充聚合指标 - LLM Judge
	if val, ok := resp.Aggregate["llm_score"]; ok {
		evalResult.LLMJudgeScore = &val
	}

	// 填充聚合指标 - 语义相似度
	if val, ok := resp.Aggregate["semantic_similarity"]; ok {
		evalResult.SemanticSimilarity = &val
	}

	// 填充聚合指标 - RAG 专属指标（忠实度/上下文相关性/噪声比）
	if val, ok := resp.Aggregate["faithfulness"]; ok {
		evalResult.Faithfulness = &val
	}
	if val, ok := resp.Aggregate["context_relevance"]; ok {
		evalResult.ContextRelevance = &val
	}
	if val, ok := resp.Aggregate["noise_ratio"]; ok {
		evalResult.NoiseRatio = &val
	}

	// 填充动态聚合指标载体（注册表驱动，name->value，与固定字段并存）
	if len(resp.Aggregate) > 0 {
		evalResult.Scores = resp.Aggregate
	}

	// 填充单项指标
	for i, item := range resp.Items {
		if i < len(evalResult.QAResults) {
			// 检索指标
			evalResult.QAResults[i].Precision = item.Precision
			evalResult.QAResults[i].Recall = item.Recall
			evalResult.QAResults[i].NDCG = item.NDCG
			evalResult.QAResults[i].RR = item.RR

			// 生成指标
			evalResult.QAResults[i].ROUGE1 = item.ROUGE1
			evalResult.QAResults[i].ROUGE2 = item.ROUGE2
			evalResult.QAResults[i].ROUGEL = item.ROUGEL
			evalResult.QAResults[i].BLEU1 = item.BLEU1
			evalResult.QAResults[i].BLEU2 = item.BLEU2
			evalResult.QAResults[i].BLEU4 = item.BLEU4

			// LLM Judge
			evalResult.QAResults[i].LLMScore = item.LLMScore
			evalResult.QAResults[i].LLMReasoning = item.LLMReasoning

			// 语义相似度
			evalResult.QAResults[i].SemanticSimilarity = item.SemanticSimilarity

			// 动态单项指标载体（注册表驱动，name->value）
			if len(item.Scores) > 0 {
				evalResult.QAResults[i].Scores = item.Scores
			}
		}
	}
}

// persistResults 幂等地把逐条评测结果写库：先按 taskID 清除旧结果再批量插入。
// delete+insert 使 executeTask 里「打分前落原始产物」与「打分后落富结果」两次调用、
// 以及任务重跑（恢复重入队/手动重跑）都不会重复插行——否则同一 task 的结果行会累积，
// 前端逐条列表出现重复、成功/失败计数虚高。
//
// B2: 通过 ReplaceByTaskID 在单个事务内完成「先删后插」，保证原子性——此前 delete 与 insert
// 是两条独立语句，中途崩溃（进程被杀/DB 连接断）会只删不插、丢失整批已算好的结果行。
func (w *EvaluationWorker) persistResults(ctx context.Context, taskID string, qaResults []*QAResult) error {
	if err := w.resultRepo.ReplaceByTaskID(ctx, taskID, buildDomainResults(taskID, qaResults)); err != nil {
		return fmt.Errorf("failed to replace results: %w", err)
	}
	return nil
}

// buildDomainResults 将应用层逐条 QAResult 转为领域层 EvaluationResult（含检索/生成/裁判/
// 语义/动态 Scores 指标与子 request_id），供幂等落库使用。
func buildDomainResults(taskID string, qaResults []*QAResult) []*domeval.EvaluationResult {
	domainResults := make([]*domeval.EvaluationResult, len(qaResults))
	for i, qa := range qaResults {
		domainResults[i] = &domeval.EvaluationResult{
			TaskID:          taskID,
			Question:        qa.Question,
			ReferenceAnswer: qa.ReferenceAnswer,
			GeneratedAnswer: qa.GeneratedAnswer,
			RelevantPIDs:    qa.RelevantPIDs,
			RetrievedPIDs:   qa.RetrievedPIDs,
			Success:         qa.Success,
			Error:           qa.Error,
			CreatedAt:       time.Now(),

			// 检索指标
			Precision: qa.Precision,
			Recall:    qa.Recall,
			NDCG:      qa.NDCG,
			RR:        qa.RR,

			// 生成指标
			ROUGE1: qa.ROUGE1,
			ROUGE2: qa.ROUGE2,
			ROUGEL: qa.ROUGEL,
			BLEU1:  qa.BLEU1,
			BLEU2:  qa.BLEU2,
			BLEU4:  qa.BLEU4,

			// LLM Judge
			LLMScore:     qa.LLMScore,
			LLMReasoning: qa.LLMReasoning,

			// 语义相似度
			SemanticSimilarity: qa.SemanticSimilarity,

			// 动态指标载体（注册表驱动，name->value）
			Scores: qa.Scores,

			// 本条 QA 的子 request_id，供前端深链到该轮 trace 瀑布图
			RequestID: qa.RequestID,
		}
	}
	return domainResults
}

// saveResults 保存评测结果
func (w *EvaluationWorker) saveResults(ctx context.Context, taskID string, appResult *EvaluationResult) error {
	// 幂等落库富结果（覆盖 5.1 落的原始产物；重跑不重复插行）
	if err := w.persistResults(ctx, taskID, appResult.QAResults); err != nil {
		return err
	}

	// 统计成功/失败条数并持久化——否则任务级 success_count/failure_count 恒为 0，
	// 前端进度与成功率无法显示。逐条以 QAResult.Success 判定。
	var successCount, failureCount int
	for _, qa := range appResult.QAResults {
		if qa.Success {
			successCount++
		} else {
			failureCount++
		}
	}
	if err := w.taskRepo.UpdateProgress(ctx, taskID, successCount, failureCount); err != nil {
		return fmt.Errorf("failed to update task progress: %w", err)
	}

	// 持久化任务级聚合指标（含 faithfulness/context_relevance/noise_ratio 等只在批级存在的指标）
	if err := w.taskRepo.UpdateMetrics(ctx, taskID, buildTaskMetrics(appResult)); err != nil {
		return fmt.Errorf("failed to save task metrics: %w", err)
	}

	// 更新任务状态为完成
	if err := w.taskRepo.UpdateStatus(ctx, taskID, domeval.TaskStatusCompleted); err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	return nil
}

// buildTaskMetrics 将应用层聚合结果映射为领域层任务级指标
func buildTaskMetrics(r *EvaluationResult) *domeval.TaskMetrics {
	if r == nil {
		return nil
	}
	return &domeval.TaskMetrics{
		Precision:          r.Precision,
		Recall:             r.Recall,
		NDCG:               r.NDCG,
		MRR:                r.MRR,
		MAP:                r.MAP,
		ROUGE1:             r.ROUGE1,
		ROUGE2:             r.ROUGE2,
		ROUGEL:             r.ROUGEL,
		BLEU1:              r.BLEU1,
		BLEU2:              r.BLEU2,
		BLEU4:              r.BLEU4,
		LLMJudgeScore:      r.LLMJudgeScore,
		SemanticSimilarity: r.SemanticSimilarity,
		Faithfulness:       r.Faithfulness,
		ContextRelevance:   r.ContextRelevance,
		NoiseRatio:         r.NoiseRatio,
		Scores:             r.Scores,
	}
}

// handleTaskError 处理任务错误
func (w *EvaluationWorker) handleTaskError(ctx context.Context, taskID string, err error, _ int) {
	// 落库/写缓存必须跑在「不随调用方取消」的 ctx 上：任务超时或 Stop() 取消 baseCtx 后，
	// 传入的 runCtx 已 Done，db.WithContext(ctx) 会在执行 UPDATE 前直接短路返回 context.Canceled，
	// 导致失败状态永远写不进库、任务卡在 running，只能等下一轮 recoverStuckTasks 兜底
	// （B4 引入 WithTimeout 后的回归）。WithoutCancel 保留 request_id 等值、剥离取消信号，
	// 再叠加独立超时避免落库长挂。
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	// 更新 Redis 进度缓存（best-effort，数据库状态才是权威来源）
	_ = w.progressCache.SetError(persistCtx, taskID, err.Error(), 0)

	// 更新数据库任务状态
	_ = w.taskRepo.UpdateError(persistCtx, taskID, err.Error())
	_ = w.taskRepo.UpdateStatus(persistCtx, taskID, domeval.TaskStatusFailed)
}

// updateProgress 更新进度
func (w *EvaluationWorker) updateProgress(ctx context.Context, taskID string, progress *domeval.Progress) {
	_ = w.progressCache.SetProgress(ctx, taskID, progress) // best-effort 进度上报，失败不影响任务执行
}

// Stop 停止 Worker
func (w *EvaluationWorker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return
	}

	close(w.stopCh)
	// B4: 取消 baseCtx，中断所有在途任务的被测 Agent 运行（否则只能等各自超时或跑完），
	// 再等待所有 goroutine 退出，实现优雅停机。
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	w.running = false
}

// IsRunning 检查 Worker 是否正在运行
func (w *EvaluationWorker) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.running
}

// ========================================
// Helper Functions
// ========================================

// convertQAPairsToDomain 将 application 层的 QAPair 转换为 domain 层
func convertQAPairsToDomain(pairs []*QAPair) []*domeval.QAPair {
	if pairs == nil {
		return nil
	}
	result := make([]*domeval.QAPair, len(pairs))
	for i, p := range pairs {
		result[i] = &domeval.QAPair{
			Question:        p.Question,
			ReferenceAnswer: p.ReferenceAnswer,
			RelevantPIDs:    p.RelevantPIDs,
			Context:         p.Context,
			ExpectedTools:   p.ExpectedTools,
			ExpectedSteps:   p.ExpectedSteps,
			GoldSQL:         p.GoldSQL,
		}
	}
	return result
}

// convertQAResultsToApp 将 domain 层的 QAResult 转换为 application 层
func convertQAResultsToApp(results []*domeval.QAResult) []*QAResult {
	if results == nil {
		return nil
	}
	appResults := make([]*QAResult, len(results))
	for i, r := range results {
		appResults[i] = &QAResult{
			Question:           r.Question,
			ReferenceAnswer:    r.ReferenceAnswer,
			GeneratedAnswer:    r.GeneratedAnswer,
			RetrievedChunks:    r.RetrievedChunks,
			RetrievedPIDs:      r.RetrievedPIDs,
			RelevantPIDs:       r.RelevantPIDs,
			Success:            r.Success,
			Error:              r.Error,
			ToolsUsed:          r.ToolsUsed,
			Trajectory:         r.Trajectory,
			TotalSteps:         r.TotalSteps,
			LatencyMs:          r.LatencyMs,
			TokensUsed:         r.TokensUsed,
			LLMCalls:           r.LLMCalls,
			RequestID:          r.RequestID,
			ExpectedTools:      r.ExpectedTools,
			ExpectedSteps:      r.ExpectedSteps,
			Precision:          r.Precision,
			Recall:             r.Recall,
			NDCG:               r.NDCG,
			RR:                 r.RR,
			ROUGE1:             r.ROUGE1,
			ROUGE2:             r.ROUGE2,
			ROUGEL:             r.ROUGEL,
			BLEU1:              r.BLEU1,
			BLEU2:              r.BLEU2,
			BLEU4:              r.BLEU4,
			LLMScore:           r.LLMScore,
			LLMReasoning:       r.LLMReasoning,
			SemanticSimilarity: r.SemanticSimilarity,
			Scores:             r.Scores,
			GeneratedSQL:       r.GeneratedSQL,
			GoldSQL:            r.GoldSQL,
			ResultSet:          r.ResultSet,
			GoldResultSet:      r.GoldResultSet,
		}
	}
	return appResults
}
