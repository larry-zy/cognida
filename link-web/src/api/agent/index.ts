import type { AgentStreamEvent } from '@/types'
import { storage } from '@/utils/security'

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

  const token = storage.get<string>('token')
  const currentTenant = storage.get<any>('current_tenant')

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'Accept': 'text/event-stream'
  }

  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  if (currentTenant?.id) {
    headers['X-Tenant-ID'] = currentTenant.id.toString()
  }

  const body: AgentChatRequest = { query }
  if (sessionId) {
    body.session_id = sessionId
  }

  const apiBase = getApiBaseURL()
  const response = await fetch(`${apiBase}${endpoint}`, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
    signal: opts?.signal
  })

  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }

  const reader = response.body?.getReader()
  if (!reader) {
    throw new Error('Failed to get response reader')
  }

  const decoder = new TextDecoder()
  let buffer = ''
  let currentEvent = ''

  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })

      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        const trimmed = line.trim()
        if (trimmed.startsWith('event:')) {
          currentEvent = trimmed.slice(6).trim()
        } else if (trimmed.startsWith('data:')) {
          const data = trimmed.slice(5).trim()
          if (!data || data === '[DONE]') {
            currentEvent = ''
            continue
          }

          try {
            const parsed = JSON.parse(data) as AgentStreamEvent
            if (currentEvent && !parsed.event) {
              parsed.event = currentEvent as any
            }
            yield parsed
          } catch (e) {
            console.error('Failed to parse SSE data:', e, 'Raw data:', data)
          }
          currentEvent = ''
        }
      }
    }
  } finally {
    reader.releaseLock()
  }
}

/**
 * 知识库对话流式请求
 */
export async function* streamKnowledgeChat(request: string | AgentChatRequest, opts?: StreamOptions): AsyncGenerator<AgentStreamEvent> {
  yield* streamChat('/agent/knowledge/stream', request, opts)
}

/**
 * Text2SQL 流式请求
 */
export async function* streamText2SQL(request: string | AgentChatRequest, opts?: StreamOptions): AsyncGenerator<AgentStreamEvent> {
  yield* streamChat('/agent/text2sql/stream', request, opts)
}

/**
 * Agent 流式聊天（保留兼容性）
 */
export async function* streamAgentChat(request: string | AgentChatRequest, opts?: StreamOptions): AsyncGenerator<AgentStreamEvent> {
  yield* streamChat('/agent/chat/stream', request, opts)
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
  streamText2SQL,
  streamAgentChat,
  getUISurfacePage,
  confirmOperation
}

export default agentApi

