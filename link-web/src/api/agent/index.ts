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
 * Agent API
 */
export const agentApi = {
  streamKnowledgeChat,
  streamText2SQL,
  streamAgentChat
}

export default agentApi

