// ==================== 测评页配置与格式化工具 ====================
// 从 EvaluationList.vue 抽离的纯常量与纯函数，供页面与子组件共享。

import type { EvaluationTask, GraderMeta } from '@/types'

/** 评测器分组配置（用于创建对话框的评测器多选） */
export interface GraderOption {
  /** 评测器标识，提交到后端 config.graders */
  value: string
  /** 展示标签 */
  label: string
}

export interface GraderGroup {
  /** 分组标题 */
  title: string
  /** 仅在指定评测类型下展示（不填则始终展示） */
  onlyType?: 'qa' | 'rag' | 'agent'
  options: GraderOption[]
}

/**
 * 分组键 → 展示标题。后端 grader 的 group 键是唯一事实来源，
 * 此表仅负责把机器键本地化为中文标题；缺失时回退为原始键。
 */
export const GRADER_GROUP_TITLES: Record<string, string> = {
  generation: '文本生成质量',
  retrieval: '检索质量',
  rag: 'RAG 专属指标',
  semantic: '语义质量',
  llm: 'AI 评分 (LLM-as-Judge)',
  rule: '规则匹配',
  general: '通用指标'
}

/**
 * 将后端返回的扁平 grader 目录按 group 归类为展示分组。
 * 前端不再自行判断适用类型——后端已按 eval_type 过滤，返回什么就展示什么，
 * 从根源消除前后端目录漂移（如 llm_factual/llm_safety）。
 */
export function buildGraderGroups(graders: GraderMeta[]): GraderGroup[] {
  const byGroup = new Map<string, GraderOption[]>()
  for (const g of graders) {
    const opts = byGroup.get(g.group) ?? []
    opts.push({ value: g.name, label: g.label })
    byGroup.set(g.group, opts)
  }
  return Array.from(byGroup.entries()).map(([key, options]) => ({
    title: GRADER_GROUP_TITLES[key] ?? key,
    options
  }))
}

/**
 * 前端回退目录（仅在后端目录接口不可用时使用）。
 * 注意：唯一事实来源是后端注册表，此常量不再决定"用户能勾选什么"，
 * 仅作为离线/降级兜底。已移除后端无对应实现的漂移项（llm_factual/llm_safety）。
 */
export const GRADER_GROUPS: GraderGroup[] = [
  {
    title: '文本生成质量',
    options: [
      { value: 'rouge', label: 'ROUGE (rouge_1/rouge_2/rouge_l)' },
      { value: 'bleu', label: 'BLEU (bleu_1/bleu_4)' }
    ]
  },
  {
    title: '检索质量',
    onlyType: 'rag',
    options: [
      { value: 'precision', label: '精确率' },
      { value: 'recall', label: '召回率' },
      { value: 'ndcg', label: 'NDCG 排序质量' },
      { value: 'mrr', label: 'MRR 倒数排名' },
      { value: 'map', label: 'MAP 平均精确率' }
    ]
  },
  {
    title: 'RAG 专属指标',
    onlyType: 'rag',
    options: [
      { value: 'faithfulness', label: '忠实度（答案是否基于检索内容）' },
      { value: 'context_relevance', label: '上下文相关性（检索内容与问题相关性）' },
      { value: 'noise_ratio', label: '噪声比例（检索到的不相关文档占比）' }
    ]
  },
  {
    title: '语义质量',
    options: [
      { value: 'semantic_similarity', label: '语义相似度' },
      { value: 'semantic_relevance', label: '语义相关性' }
    ]
  },
  {
    title: 'AI 评分 (LLM-as-Judge)',
    options: [
      { value: 'llm_judge', label: 'LLM 裁判 (多维度)' }
    ]
  }
]

/** LLM 裁判评分维度（1-5 分） */
export const LLM_JUDGE_DIMENSIONS: GraderOption[] = [
  { value: 'accuracy', label: '准确性' },
  { value: 'completeness', label: '完整性' },
  { value: 'clarity', label: '清晰度' },
  { value: 'relevance', label: '相关性' },
  { value: 'helpfulness', label: '帮助程度' }
]

/** 评测类型下拉选项 */
export const EVALUATION_TYPE_OPTIONS = [
  { value: 'agent', label: 'Agent 评测' },
  { value: 'qa', label: 'QA 评测' },
  { value: 'rag', label: 'RAG 评测' }
]

/** 计算任务进度百分比 */
export function getProgress(task: EvaluationTask): number {
  if (task.total_count === 0) return 0
  return Math.round(((task.success_count + task.failure_count) / task.total_count) * 100)
}

/**
 * 将测评状态映射为 UiProgress 的 status。
 * failed → danger，completed → success，其余 → normal。
 */
export function progressStatus(status: string): 'normal' | 'success' | 'danger' {
  if (status === 'failed') return 'danger'
  if (status === 'completed') return 'success'
  return 'normal'
}

/** 格式化时间为本地字符串 */
export function formatTime(timeStr: string): string {
  if (!timeStr) return '-'
  const date = new Date(timeStr)
  return date.toLocaleString('zh-CN')
}

/** 将 0~1 的比值格式化为百分比字符串 */
export function formatPercent(value: number): string {
  return (value * 100).toFixed(2) + '%'
}

/**
 * 指标名 → 展示标签 + 是否按百分比（0~1 值）展示。
 * 未知指标回退为原始名并按百分比展示——这样开发者在后端新增一个 grader 后，
 * 无需改前端即可在结果里看到它（可选地再补一条美化映射）。
 */
export const METRIC_DISPLAY: Record<string, { label: string; percent: boolean }> = {
  rouge_1: { label: 'ROUGE-1', percent: true },
  rouge_2: { label: 'ROUGE-2', percent: true },
  rouge_l: { label: 'ROUGE-L', percent: true },
  bleu_1: { label: 'BLEU-1', percent: true },
  bleu_2: { label: 'BLEU-2', percent: true },
  bleu_4: { label: 'BLEU-4', percent: true },
  precision: { label: 'Precision', percent: true },
  recall: { label: 'Recall', percent: true },
  ndcg: { label: 'NDCG', percent: true },
  mrr: { label: 'MRR', percent: true },
  map: { label: 'MAP', percent: true },
  faithfulness: { label: '忠实度', percent: true },
  context_relevance: { label: '上下文相关性', percent: true },
  noise_ratio: { label: '噪声比例', percent: true },
  semantic_similarity: { label: '语义相似度', percent: true },
  semantic_relevance: { label: '语义相关性', percent: true },
  llm_score: { label: 'LLM Score', percent: false },
  llm_judge: { label: 'LLM 裁判', percent: false },
  llm_judge_score: { label: 'LLM 评分', percent: false },
  llm_factual: { label: '事实正确性', percent: false },
  llm_safety: { label: '内容安全性', percent: false },
  exact_match: { label: '精确匹配', percent: true },
  contains_match: { label: '包含匹配', percent: true },
  regex_match: { label: '正则匹配', percent: true },
  numeric_match: { label: '数值匹配', percent: true }
}

/** 兼容读取的固定指标列（旧契约，逐步由动态 scores 取代） */
const FIXED_METRIC_KEYS = [
  'rouge_1', 'rouge_2', 'rouge_l', 'bleu_1', 'bleu_4',
  'precision', 'recall', 'ndcg', 'mrr', 'map',
  'faithfulness', 'context_relevance', 'noise_ratio',
  'semantic_similarity', 'semantic_relevance', 'llm_score'
]

/**
 * 把一行结果的所有数值指标合并为 {固定列 ∪ 动态 scores}。
 * 动态 scores（新契约）覆盖同名固定列，实现平滑迁移。
 */
export function mergedScores(row: Record<string, unknown>): Record<string, number> {
  const merged: Record<string, number> = {}
  for (const k of FIXED_METRIC_KEYS) {
    const v = row[k]
    if (typeof v === 'number') merged[k] = v
  }
  const scores = row.scores
  if (scores && typeof scores === 'object') {
    for (const [k, v] of Object.entries(scores as Record<string, unknown>)) {
      if (typeof v === 'number') merged[k] = v
    }
  }
  return merged
}

export interface MetricEntry {
  key: string
  label: string
  text: string
}

/** 将指标 map 转为展示条目：未知指标回退原始名并按百分比展示。 */
function scoresToEntries(scores: Record<string, number>): MetricEntry[] {
  return Object.entries(scores).map(([key, value]) => {
    const disp = METRIC_DISPLAY[key] ?? { label: key, percent: true }
    const text = disp.percent ? (value * 100).toFixed(2) + '%' : value.toFixed(2)
    return { key, label: disp.label, text }
  })
}

/**
 * 从一行结果提取用于展示的指标条目：优先动态 scores map，兼容固定字段。
 * 供结果详情按 map 动态渲染，开发者新增指标自动出现。
 */
export function metricEntries(row: Record<string, unknown>): MetricEntry[] {
  return scoresToEntries(mergedScores(row))
}

/** 任务级聚合的最小结构（避免耦合完整 EvaluationDetail 类型）。 */
export interface TaskAggregateSource {
  qa_results?: unknown[]
  metrics?: unknown
  total_count?: number
  success_count?: number
}

/**
 * 任务级聚合指标 map：各 QA 行动态指标（mergedScores）取均值，
 * 再叠加后端 detail.metrics 扁平契约（任务级权威聚合，覆盖同名项）。
 * 开发者在后端新增 grader → per-QA scores 自动带出 → 此处自动出现，无需改前端。
 */
export function taskAggregateScores(
  detail: TaskAggregateSource | null | undefined
): Record<string, number> {
  const merged: Record<string, number> = {}
  const results = (detail?.qa_results ?? []) as Array<Record<string, unknown>>
  if (results.length > 0) {
    const sums: Record<string, number> = {}
    const counts: Record<string, number> = {}
    for (const row of results) {
      for (const [k, v] of Object.entries(mergedScores(row))) {
        sums[k] = (sums[k] ?? 0) + v
        counts[k] = (counts[k] ?? 0) + 1
      }
    }
    for (const k of Object.keys(sums)) merged[k] = sums[k] / counts[k]
  }
  const m = detail?.metrics
  if (m && typeof m === 'object') {
    for (const [k, v] of Object.entries(m as Record<string, unknown>)) {
      if (typeof v === 'number') merged[k] = v
    }
  }
  return merged
}

/** 任务级聚合指标展示条目（注册表驱动动态渲染，兼容固定字段）。 */
export function taskAggregateEntries(
  detail: TaskAggregateSource | null | undefined
): MetricEntry[] {
  return scoresToEntries(taskAggregateScores(detail))
}

/** 任务成功率（百分比数值）。 */
export function taskSuccessRate(
  detail: TaskAggregateSource | null | undefined
): number {
  const total = Number(detail?.total_count ?? 0)
  const success = Number(detail?.success_count ?? 0)
  return total > 0 ? (success / total) * 100 : 0
}

/** 读取单个指标数值（动态 scores 优先，回退固定字段），空值返回 null。 */
export function metricValue(row: Record<string, unknown>, key: string): number | null {
  const scores = row.scores as Record<string, unknown> | undefined
  const fromScores = scores?.[key]
  if (typeof fromScores === 'number') return fromScores
  const fixed = row[key]
  return typeof fixed === 'number' ? fixed : null
}

/** 判断一条 QA 结果是否包含任意指标（含动态 scores） */
export function hasMetrics(row: Record<string, unknown>): boolean {
  return Object.keys(mergedScores(row)).length > 0
}
