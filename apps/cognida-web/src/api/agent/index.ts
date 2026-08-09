import type { AgentStreamEvent } from '@/types'
import { storage } from '@/utils/security'
import { readSSE } from '@/utils/sse'
import { http } from '@/utils/request'

/** 运行中 Agent 概要（供评测选择被测 Agent） */
export interface AgentSummary {
  id: string
  name: string
  description?: string
  type: string
  status: string
  metadata?: Record<string, unknown>
}

/** ListAgents 响应体（信封 data 内） */
export interface ListAgentsResult {
  count: number
  agents: AgentSummary[]
}

/**
 * 列出已注册的 Agent（GET /api/v1/agents），供评测创建时选择被测 Agent。
 */
export function listAgents() {
  return http.get<ListAgentsResult>('/agents')
}

// 获取 API 基础 URL
const getApiBaseURL = () => {
  if (import.meta.env.DEV && import.meta.env.VITE_API_BASE_URL) {
    return import.meta.env.VITE_API_BASE_URL
  }
  return '/api/v1'
}

/**
 * Agent 流式聊天请求
 */
interface AgentChatRequest {
  query: string
  session_id?: string
  /** 结构化下发：限定检索的知识库 ID 列表（空数组=检索全部已启用知识库） */
  kb_ids?: string[]
  /** 结构化下发：是否开启知识图谱增强（关系推理/多跳），false 也会显式发送 */
  graph_enabled?: boolean
  /**
   * 知识库选择模式：
   * - manual（手动，默认）：范围由 kb_ids 锁定，AI 不参与选库；
   * - hybrid（结合）：kb_ids 为候选池，AI 经 kb_route 在池内自选；
   * - auto（智能）：忽略 kb_ids，AI 从租户全部已启用库自选。
   */
  kb_scope_mode?: 'manual' | 'hybrid' | 'auto'
}

/**
 * 流式请求可选项（不进请求体）
 */
interface StreamOptions {
  /** 传入 AbortSignal 可在组件卸载/切换时中断 SSE 连接 */
  signal?: AbortSignal
}

/**
 * 通用流式请求函数
 */
async function* streamChat(
  endpoint: string,
  request: string | AgentChatRequest,
  opts?: StreamOptions
): AsyncGenerator<AgentStreamEvent> {
  const query = typeof request === 'string' ? request : request.query
  const sessionId = typeof request === 'string' ? undefined : request.session_id
  const kbIds = typeof request === 'string' ? undefined : request.kb_ids
  const graphEnabled = typeof request === 'string' ? undefined : request.graph_enabled
  const kbScopeMode = typeof request === 'string' ? undefined : request.kb_scope_mode

  const body: AgentChatRequest = { query }
  if (sessionId) {
    body.session_id = sessionId
  }
  // 仅在非空数组时下发 kb_ids（空数组表示“全部知识库”，无需携带）
  if (Array.isArray(kbIds) && kbIds.length > 0) {
    body.kb_ids = kbIds
  }
  // graph_enabled 为布尔值时总是下发，false 也显式发送
  if (typeof graphEnabled === 'boolean') {
    body.graph_enabled = graphEnabled
  }
  // kb_scope_mode 有值即下发（manual/hybrid/auto），后端空串按 manual 处理
  if (kbScopeMode) {
    body.kb_scope_mode = kbScopeMode
  }

  // 统一走 readSSE：鉴权头注入、非 2xx 统一错误/登出、AbortSignal 透传
  const apiBase = getApiBaseURL()
  for await (const msg of readSSE(`${apiBase}${endpoint}`, body, opts?.signal)) {
    try {
      const parsed = JSON.parse(msg.data) as AgentStreamEvent
      if (msg.event && !parsed.event) {
        parsed.event = msg.event as any
      }
      yield parsed
    } catch (e) {
      console.error('Failed to parse SSE data:', e, 'Raw data:', msg.data)
    }
  }
}

/**
 * 知识库对话流式请求
 */
export async function* streamKnowledgeChat(request: string | AgentChatRequest, opts?: StreamOptions): AsyncGenerator<AgentStreamEvent> {
  yield* streamChat('/agent/knowledge/stream', request, opts)
}

/**
 * Agent 流式聊天（保留兼容性）
 */
export async function* streamAgentChat(request: string | AgentChatRequest, opts?: StreamOptions): AsyncGenerator<AgentStreamEvent> {
  yield* streamChat('/agent/chat/stream', request, opts)
}

/**
 * Data Agent 流式聊天。
 *
 * 后端入口 /agent/text2sql/stream 已迁移为 Data Agent 主入口（单一 ReAct 内核
 * + 子代理委派，preset id = agent-data-agent），会话类型落库为 text2sql。
 * 该入口只绑定数据智能体：不选智能体、不选知识库，故请求体只携 query + session_id，
 * 不下发 kb_ids / graph_enabled。流中除 step / done 外，会随 render_ui 工具成功
 * 实时下发 ui 事件（A2UI UISpec），由结果画布即时渲染。
 */
export async function* streamDataChat(
  request: { query: string; session_id?: string; datasource_id?: string },
  opts?: StreamOptions
): AsyncGenerator<AgentStreamEvent> {
  const body: AgentChatRequest & { datasource_id?: string } = { query: request.query }
  if (request.session_id) body.session_id = request.session_id
  // 仅在非空时下发 datasource_id（空串表示"使用当前库"，无需携带）
  if (request.datasource_id) body.datasource_id = request.datasource_id
  yield* streamChat('/agent/text2sql/stream', body, opts)
}

/**
 * UI surface 分页回源响应（对齐 Go 端 GetUISurfacePage）。
 * status: ok | session_expired（绑定超会话 TTL）| data_expired（result_id 过期，可重跑）
 */
export interface UISurfacePageResult {
  status: 'ok' | 'session_expired' | 'data_expired'
  message?: string
  result_id?: string
  columns?: string[]
  rows?: Record<string, any>[]
  row_count?: number
  cursor?: number
  next_cursor?: number
}

/**
 * 按 surface + 能力 token 从 Result Store 回源取一页数据（cursor 分页，不重跑查询）。
 * 供 A2UI Pagination/Filter 组件回调使用。
 */
export async function getUISurfacePage(
  surface: string,
  params: { token: string; cursor?: number; page_size?: number }
): Promise<UISurfacePageResult> {
  const token = storage.get<string>('token')
  const currentTenant = storage.get<any>('current_tenant')

  const headers: Record<string, string> = {}
  if (token) headers['Authorization'] = `Bearer ${token}`
  if (currentTenant?.id) headers['X-Tenant-ID'] = currentTenant.id.toString()

  const qs = new URLSearchParams({ token: params.token })
  if (params.cursor != null) qs.set('cursor', String(params.cursor))
  if (params.page_size != null) qs.set('page_size', String(params.page_size))

  const response = await fetch(
    `${getApiBaseURL()}/agent/ui/surfaces/${encodeURIComponent(surface)}/page?${qs}`,
    { headers }
  )
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  const body = await response.json()
  // 后端统一 {code, message, data} 信封，页数据在 data 内
  return (body?.data ?? body) as UISurfacePageResult
}

/**
 * 危险操作确认响应（对齐 Go 端 ConfirmOperation / SQLMutateResult）。
 * 成功时 status=success 携 rows_affected；pending action 不存在/已过期时
 * status=expired（非错误路由）；token 不匹配走 HTTP 403（该操作随即失效）。
 */
export interface ConfirmOperationResult {
  status: 'success' | 'rejected' | 'expired' | string
  target?: string
  rows_affected?: number
  duplicate?: boolean
  message?: string
}

/**
 * 危险操作人机确认 resume（Phase 5）：Confirm 卡片确认后携
 * pending_action_id + 一次性确认 token + session_id 调用，
 * 后端校验 token/归属通过后提交暂存的写事务。
 */
export async function confirmOperation(params: {
  pending_action_id: string
  token: string
  session_id: string
}): Promise<ConfirmOperationResult> {
  const token = storage.get<string>('token')
  const currentTenant = storage.get<any>('current_tenant')

  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (token) headers['Authorization'] = `Bearer ${token}`
  if (currentTenant?.id) headers['X-Tenant-ID'] = currentTenant.id.toString()

  const response = await fetch(`${getApiBaseURL()}/agent/operations/confirm`, {
    method: 'POST',
    headers,
    body: JSON.stringify(params)
  })
  const body = await response.json().catch(() => null)
  if (!response.ok) {
    // 403（token 不匹配）等：透出后端 message，供宿主视图提示
    throw new Error(body?.message || `HTTP error! status: ${response.status}`)
  }
  // 后端统一 {code, message, data} 信封，结果在 data 内
  return (body?.data ?? body) as ConfirmOperationResult
}

/**
 * Agent API
 */
export const agentApi = {
  streamKnowledgeChat,
  streamAgentChat,
  streamDataChat,
  getUISurfacePage,
  confirmOperation,
  listAgents
}

export default agentApi

