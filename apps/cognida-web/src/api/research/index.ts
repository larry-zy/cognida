import { readSSE } from '@/utils/sse'

export interface ResearchRequest {
  query: string
  kb_ids: string[]
  kb_scope_mode: 'manual'
  graph_enabled: boolean
  options: { enable_web_search: boolean; max_sub_questions: number; timeout_seconds: number }
}
export interface ResearchSource {
  id: string
  type: 'rag' | 'graph' | 'web'
  title: string
  url?: string
  excerpt: string
  knowledge_base_id?: string
  knowledge_id?: string
  chunk_id?: string
}
export interface ResearchReport {
  query: string
  success: boolean
  error?: string
  executive_summary: string
  detailed_analysis: string
  key_findings: string[]
  sub_questions: string[]
  sources: ResearchSource[]
  warnings?: string[]
  citation_checks?: { statement: string; source_id: string; quote: string; quote_matched: boolean }[]
  metadata: { partial: boolean; citation_checked: boolean; fact_checked: boolean; verification_limit?: string; execution_time_ms: number; total_sources: number }
}
export interface ResearchProgress {
  stage: string
  progress: number
  detail: string
  error?: string
  sub_questions?: string[]
  report?: ResearchReport
}

export async function* streamResearch(request: ResearchRequest, signal: AbortSignal): AsyncGenerator<ResearchProgress> {
  for await (const message of readSSE('/api/v1/agent/deep-research/stream', request, signal)) {
    if (message.event === 'heartbeat') continue
    const event = JSON.parse(message.data) as ResearchProgress
    if (!event.stage) continue
    if (event.stage === 'complete' && (!event.report?.success || !event.report.sources?.length)) {
      throw new Error('研究结束时未收到有效报告，请重试')
    }
    yield event
    if (event.stage === 'complete' || event.stage === 'error') return
  }
  if (!signal.aborted) throw new Error('连接已结束，尚未收到完整研究报告')
}
