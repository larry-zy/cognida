// ==================== 测评页配置与格式化工具 ====================
// 从 EvaluationList.vue 抽离的纯常量与纯函数，供页面与子组件共享。

import type { EvaluationTask } from '@/types'

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

/** 评测器分组（与后端评测器一一对应） */
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
    title: '语义质量',
    options: [
      { value: 'semantic_similarity', label: '语义相似度' },
      { value: 'semantic_relevance', label: '语义相关性' }
    ]
  },
  {
    title: 'AI 评分 (LLM-as-Judge)',
    options: [
      { value: 'llm_judge', label: 'LLM 裁判 (多维度)' },
      { value: 'llm_factual', label: '事实正确性' },
      { value: 'llm_safety', label: '内容安全性' }
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
  { value: 'qa', label: 'QA 评测' },
  { value: 'rag', label: 'RAG 评测' },
  { value: 'agent', label: 'Agent 评测' }
]

/** 聚合指标（详情对话框顶部整体指标卡片） */
export interface AggregateMetrics {
  rouge1: number | null
  rougeL: number | null
  bleu1: number | null
  precision: number | null
  recall: number | null
  llmScore: number | null
  semanticSim: number | null
  successRate: number
}

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

/** 判断一条 QA 结果是否包含任意指标 */
export function hasMetrics(row: Record<string, unknown>): boolean {
  return row.rouge_1 != null || row.rouge_l != null ||
    row.bleu_1 != null || row.precision != null ||
    row.recall != null || row.llm_score != null ||
    row.semantic_similarity != null
}
