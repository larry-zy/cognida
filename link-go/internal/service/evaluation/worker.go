// Package evaluation 提供评测系统应用层实现
package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"link/internal/service/evaluation/executor"
	domeval "link/internal/model/evaluation"
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

	return &EvaluationWorker{
		queue:         queue,
		progressCache: progressCache,
		pythonClient:  pythonClient,
		datasetLoader: datasetLoader,
		registry:      registry,
		taskRepo:      taskRepo,
		resultRepo:    resultRepo,
		stopCh:        make(chan struct{}),
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

	log.Printf("[Worker] Starting worker loop in goroutine")
	w.wg.Add(1)
	go w.workerLoop()

	log.Printf("[Worker] Worker started successfully")
	return nil
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
			w.queue.ReleaseSlot(context.Background())
			time.Sleep(backoffDuration)
			continue
		}

		if taskID == "" {
			// 队列为空，释放槽位并使用退避策略
			w.queue.ReleaseSlot(context.Background())
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
				w.queue.ReleaseSlot(context.Background())
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
	ctx := context.Background()

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

	// 更新进度：开始计算指标
	w.updateProgress(ctx, taskID, &domeval.Progress{
		Stage:      domeval.StageEvaluation,
		Current:    totalQAs,
		Total:      totalQAs,
		Message:    "Computing metrics...",
		RetryCount: 0,
	})

	// 6. 计算指标（转换类型）
	appResults := convertQAResultsToApp(domainResults)
	evalResult, err := w.computeMetrics(ctx, config, appResults)
	if err != nil {
		w.handleTaskError(ctx, taskID, fmt.Errorf("failed to compute metrics: %w", err), 0)
		return
	}

	// 7. 保存结果
	if err := w.saveResults(ctx, taskID, config, evalResult); err != nil {
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
		DatasetID: task.DatasetID,
		Type:      task.Type,
		KnowledgeBaseID:      task.KnowledgeBaseID,
		AgentID:   task.AgentID,
		ModelID:   task.ModelID,
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

// computeMetrics 计算评测指标
func (w *EvaluationWorker) computeMetrics(ctx context.Context, config *DomainEvaluationTaskConfig, qaResults []*QAResult) (*EvaluationResult, error) {
	// 准备请求
	items := make([]*ComputeItem, len(qaResults))
	for i, result := range qaResults {
		items[i] = &ComputeItem{
			Question:        result.Question,
			ReferenceAnswer: result.ReferenceAnswer,
			GeneratedAnswer: result.GeneratedAnswer,
			RetrievedPIDs:   result.RetrievedChunks,
			RelevantPIDs:    result.RelevantPIDs,
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

	// 合并结果
	evalResult := &EvaluationResult{
		DatasetID: config.DatasetID,
	 KnowledgeBaseID:      config.KnowledgeBaseID,
		ModelID:   config.ModelID,
		QAResults: qaResults,
	}

	// 填充指标
	w.fillMetrics(evalResult, resp)

	return evalResult, nil
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
		}
	}
}

// saveResults 保存评测结果
func (w *EvaluationWorker) saveResults(ctx context.Context, taskID string, config *domeval.EvaluationTaskConfig, appResult *EvaluationResult) error {
	// 转换 application QAResult 到 domain EvaluationResult
	domainResults := make([]*domeval.EvaluationResult, len(appResult.QAResults))
	for i, qa := range appResult.QAResults {
		domainResults[i] = &domeval.EvaluationResult{
			TaskID:          taskID,
			Question:        qa.Question,
			ReferenceAnswer: qa.ReferenceAnswer,
			GeneratedAnswer: qa.GeneratedAnswer,
			RelevantPIDs:    qa.RelevantPIDs,
			RetrievedPIDs:   qa.RetrievedChunks,
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
		}
	}

	// 批量保存结果
	if err := w.resultRepo.CreateBatch(ctx, domainResults); err != nil {
		return fmt.Errorf("failed to save results: %w", err)
	}

	// 更新任务状态为完成
	if err := w.taskRepo.UpdateStatus(ctx, taskID, domeval.TaskStatusCompleted); err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	return nil
}

// handleTaskError 处理任务错误
func (w *EvaluationWorker) handleTaskError(ctx context.Context, taskID string, err error, _ int) {
	// 更新 Redis 进度缓存
	w.progressCache.SetError(ctx, taskID, err.Error(), 0)

	// 更新数据库任务状态
	_ = w.taskRepo.UpdateError(ctx, taskID, err.Error())
	_ = w.taskRepo.UpdateStatus(ctx, taskID, domeval.TaskStatusFailed)
}

// updateProgress 更新进度
func (w *EvaluationWorker) updateProgress(ctx context.Context, taskID string, progress *domeval.Progress) {
	w.progressCache.SetProgress(ctx, taskID, progress)
}

// Stop 停止 Worker
func (w *EvaluationWorker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return
	}

	close(w.stopCh)
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
			RelevantPIDs:       r.RelevantPIDs,
			Success:            r.Success,
			Error:              r.Error,
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
		}
	}
	return appResults
}
