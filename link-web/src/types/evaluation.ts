/**
 * 测评状态值类型
 */
export type EvaluationStatusValue = 'pending' | 'running' | 'completed' | 'failed'

/**
 * 测评状态枚举（用于运行时）
 */
export const EvaluationStatus = {
  Pending: 'pending',
  Running: 'running',
  Completed: 'completed',
  Failed: 'failed'
} as const

// 类型别名，用于类型注解
export type EvaluationStatus = EvaluationStatusValue

/**
 * 测评状态文本映射
 */
export const EvaluationStatusText: Record<EvaluationStatusValue, string> = {
  'pending': '等待中',
  'running': '执行中',
  'completed': '已完成',
  'failed': '失败'
}

/**
 * 测评状态 Element Plus Tag 类型映射
 */
export const EvaluationStatusType: Record<EvaluationStatusValue, 'info' | 'warning' | 'success' | 'danger'> = {
  'pending': 'info',
  'running': 'warning',
  'completed': 'success',
  'failed': 'danger'
}

/**
 * 检索指标
 */
export interface RetrievalMetrics {
  precision: number    // 精确率
  recall: number       // 召回率
  ndcg: number         // NDCG
  mrr: number          // 平均倒数排名
  map: number          // 平均精确率
}

/**
 * 生成指标
 */
export interface GenerationMetrics {
  bleu1: number   // BLEU-1
  bleu2: number   // BLEU-2
  bleu4: number   // BLEU-4
  rouge1: number  // ROUGE-1
  rouge2: number  // ROUGE-2
  rougeL: number  // ROUGE-L
}

/**
 * 综合指标结果
 */
export interface MetricResult {
  retrieval_metrics?: RetrievalMetrics
  generation_metrics?: GenerationMetrics
}

/**
 * 任务级聚合指标（扁平结构，与后端 Go TaskMetrics 对齐）
 * 后端在评测完成后写入 evaluation_tasks.metrics(JSON) 列。
 */
export interface TaskMetrics {
  // 检索指标
  precision?: number
  recall?: number
  ndcg?: number
  mrr?: number
  map?: number
  // 生成指标
  rouge_1?: number
  rouge_2?: number
  rouge_l?: number
  bleu_1?: number
  bleu_2?: number
  bleu_4?: number
  // LLM Judge / 语义
  llm_judge_score?: number
  semantic_similarity?: number
  // RAG 专属
  faithfulness?: number
  context_relevance?: number
  noise_ratio?: number
}

/**
 * 测评参数
 */
export interface EvaluationParams {
  vector_threshold: number
  keyword_threshold: number
  embedding_top_k: number
  rerank_threshold: number
  rerank_top_k: number
  chat_model_id: string
}

/**
 * 测评任务
 */
export interface EvaluationTask {
  id?: string
  task_id: string
  dataset_id: string
  type: string
  status: EvaluationStatus
  total_count: number
  success_count: number
  failure_count: number
  created_at: string
  updated_at: string
  kb_id?: string
  agent_id?: string
  model_id?: string
  error_message?: string
}

/**
 * 测评任务详情
 */
export interface EvaluationTaskDetail extends EvaluationTask {
  config?: any
}

/**
 * 测评详情
 */
export interface EvaluationDetail {
  task_id: string
  dataset_id: string
  type: string
  status: EvaluationStatus
  qa_results: any[]
  total_count: number
  success_count: number
  failure_count: number
  created_at: string
  updated_at: string
  // 任务信息
  task?: EvaluationTaskDetail
  // 评测指标（旧结构，逐步废弃）
  metric?: MetricResult
  // 任务级聚合指标（新契约，扁平结构）
  metrics?: TaskMetrics
}

/**
 * QA对
 */
export interface QAPair {
  question: string
  answer?: string
  reference_answer?: string
  pids?: number[]
  passages?: string[]
  relevant_pids?: string[]
}

/**
 * 创建测评请求
 */
export interface CreateEvaluationRequest {
  dataset_id: string
  type: 'rag' | 'qa' | 'agent'
  kb_id?: string
  agent_id?: string
  model_id?: string
  config?: EvaluationConfig
}

/**
 * 评测配置
 */
export interface EvaluationConfig {
  // 评测器列表
  graders?: string[]
  // LLM-as-Judge 配置
  llm_judge?: {
    enabled?: boolean
    dimensions?: string[]
  }
  // 其他参数
  [key: string]: any
}

/**
 * 创建数据集请求（数据库）
 */
export interface CreateDatasetRequest {
  dataset_id: string
  name: string
  description?: string
  evaluation_type: 'rag' | 'qa' | 'agent'
  qa_pairs?: QAPair[]
  // 兼容后端可能使用的字段名
  qapairs?: QAPair[]
}

/**
 * 更新数据集请求
 */
export interface UpdateDatasetRequest {
  name?: string
  description?: string
}

/**
 * 添加样本请求
 */
export interface AddSamplesRequest {
  qa_pairs: QAPair[]
}

/**
 * 样本记录
 */
export interface DatasetSample {
  id: number
  dataset_id: string
  question: string
  reference_answer: string
  relevant_pids?: string[]
  context?: string
  created_at: string
}

/**
 * 样本列表响应
 */
export interface SampleListResponse {
  samples: DatasetSample[]
  total: number
  page: number
  page_size: number
}

/**
 * 数据集类型
 */
export type DatasetType = 'file' | 'database'

/**
 * 数据集信息
 */
export interface DatasetInfo {
  id: string
  name?: string
  description?: string
  type: 'rag' | 'qa' | 'agent'
  eval_type?: 'rag' | 'qa' | 'agent'  // 后端返回字段
  dataset_type?: DatasetType  // 数据集存储类型
  version?: string
  domain?: string
  language?: string
  qa_count: number
  created_at?: string
  updated_at?: string
}

/**
 * 数据集列表响应
 */
export interface DatasetListResponse {
  datasets?: DatasetInfo[]
  tasks?: EvaluationTask[]
  total: number
}

/**
 * QA 级别评测结果
 */
export interface QAResult {
  index: number
  question: string
  reference_answer: string
  generated_answer?: string
  retrieved_passages?: string[]

  // 检索指标
  precision?: number
  recall?: number
  ndcg?: number
  mrr?: number

  // 生成指标
  bleu?: number
  rouge1?: number
  rouge2?: number
  rougeL?: number

  // 综合得分
  score?: number
}

/**
 * QA 级别评测结果详情（从后端返回）
 */
export interface QAResultDetail {
  id: number
  task_id: string
  question: string
  reference_answer: string
  generated_answer: string
  retrieved_pids?: string
  relevant_pids?: string
  success: boolean
  error?: string

  // 检索指标
  precision?: number
  recall?: number
  ndcg?: number
  rr?: number

  // 生成指标
  rouge_1?: number
  rouge_2?: number
  rouge_l?: number
  bleu_1?: number
  bleu_2?: number
  bleu_4?: number

  // LLM Judge
  llm_score?: number
  llm_reasoning?: string

  // 语义相似度
  semantic_similarity?: number
}

/**
 * SSE 进度事件
 */
export interface SSEProgressEvent {
  type: 'progress' | 'complete' | 'error'
  task_id: string
  data: {
    status: EvaluationStatus
    finished: number
    total: number
    current_qa?: {
      index: number
      question: string
    }
    error?: string
  }
}
