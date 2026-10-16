import { http } from '@/utils/request'
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
  content_digest?: string
  acquired_at?: string
  independence_group?: string
  independence_basis?: string
  invalidated?: boolean
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
  claims?: ResearchClaim[]
  review_history?: { round: number; source_ids: string[]; source_digest: string; source_groups: Record<string, string>; claims: ResearchClaim[] }[]
  unverified?: string[]
  citation_checks?: { statement: string; source_id: string; quote: string; quote_matched: boolean }[]
  metadata: {
    draft_repair?: { attempts: number; status: 'reserved' | 'completed' | 'failed'; trigger: { stage: string; code: string }; failure?: { stage: string; code: string } }
    supplemental_skipped_reason?: 'model_call_budget_exhausted' | 'call_budget_exhausted'
    failure?: { stage: string; code: string }; reviewed_source_ids?: string[]; unreviewed_source_ids?: string[]; supplemental_review_incomplete?: boolean; semantic_review?: boolean; review_rounds?: number; calls_used?: number; model_calls_used?: number; partial: boolean; citation_checked: boolean; fact_checked: boolean; verification_limit?: string; execution_time_ms: number; total_sources: number
  }
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

export interface ResearchClaim { id: string; statement: string; status: 'supported' | 'conflict' | 'insufficient'; reason: string; independent_sources: number; evidence: { source_id: string; quote: string; relation: 'supports' | 'contradicts' | 'context'; quote_matched: boolean }[] }
export interface DurableResearchRequest { query: string; kb_ids: string[]; graph_enabled: boolean; enable_web_search: boolean }
export interface ResearchRun { id: string; version: number; status: string; stage: string; progress: number; error?: string; report?: ResearchReport; submission: { request: DurableResearchRequest }; output_refs: { asset_id: string; version: number }[]; updated_at: string }
export const researchApi = {
  create: (body: DurableResearchRequest, key: string, signal?: AbortSignal) => http.post<ResearchRun>('/research/runs', body, { headers: { 'Idempotency-Key': key }, signal }),
  get: (id: string, signal?: AbortSignal) => http.get<ResearchRun>(`/research/runs/${encodeURIComponent(id)}`, { signal }),
  cancel: (id: string, signal?: AbortSignal) => http.post<ResearchRun>(`/research/runs/${encodeURIComponent(id)}/cancel`, {}, { signal }),
}
