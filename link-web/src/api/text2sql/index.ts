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
 * Text2SQL 流式聊天请求
 */
interface Text2SQLRequest {
  query: string
  session_id?: string
}

/**
 * Text2SQL 流式聊天 - 实时显示 SQL 生成和查询过程
 */
async function* streamText2SQL(request: string | Text2SQLRequest): AsyncGenerator<AgentStreamEvent> {
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

  const body: Text2SQLRequest = { query }
  if (sessionId) {
    body.session_id = sessionId
  }

  const apiBase = getApiBaseURL()
  const response = await fetch(`${apiBase}/agent/text2sql/stream`, {
    method: 'POST',
    headers,
    body: JSON.stringify(body)
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
 * 获取数据库 schema
 */
async function getDatabaseSchema() {
  const token = storage.get<string>('token')
  const currentTenant = storage.get<any>('current_tenant')

  const headers: Record<string, string> = {
    'Content-Type': 'application/json'
  }

  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  if (currentTenant?.id) {
    headers['X-Tenant-ID'] = currentTenant.id.toString()
  }

  const apiBase = getApiBaseURL()
  const response = await fetch(`${apiBase}/agent/text2sql/schema`, {
    method: 'GET',
    headers
  })

  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }

  return response.json()
}

/**
 * Text2SQL API
 */
export const text2sqlApi = {
  streamText2SQL,
  getDatabaseSchema
}

export default text2sqlApi
