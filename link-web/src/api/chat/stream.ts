import type { ChatRequest, StreamChatEvent } from '@/types'
import { storage } from '@/utils/security'

// 获取 API 基础 URL
const getApiBaseURL = () => {
  if (import.meta.env.DEV && import.meta.env.VITE_API_BASE_URL) {
    return import.meta.env.VITE_API_BASE_URL
  }
  return '/api/v1'
}

/**
 * SSE流式聊天
 */
export async function* streamChat(data: ChatRequest): AsyncGenerator<StreamChatEvent> {
  const response = await fetch(`${getApiBaseURL()}/chat/stream`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Accept': 'text/event-stream'
    },
    body: JSON.stringify(data)
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

  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })

      // 处理SSE格式的数据
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (line.startsWith('data: ')) {
          const data = line.slice(6).trim()
          if (data === '[DONE]') continue

          try {
            const event = JSON.parse(data) as StreamChatEvent
            yield event
          } catch (e) {
            console.error('Failed to parse SSE data:', e)
          }
        }
      }
    }
  } finally {
    reader.releaseLock()
  }
}

/**
 * 带认证的SSE流式聊天
 */
export async function* streamChatWithAuth(data: ChatRequest): AsyncGenerator<StreamChatEvent> {
  // 从 storage 获取 token（正确处理编码）
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

  // 根据是否启用 RAG 选择不同的端点
  const apiBase = getApiBaseURL()
  const url = data.rag_config?.enabled
    ? `${apiBase}/chat/rag/stream`
    : `${apiBase}/chat/auth/stream`

  const response = await fetch(url, {
    method: 'POST',
    headers,
    body: JSON.stringify(data)
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

  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })

      // 处理SSE格式的数据
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (line.startsWith('data: ')) {
          const data = line.slice(6).trim()
          if (data === '[DONE]') continue

          try {
            const event = JSON.parse(data) as StreamChatEvent
            yield event
          } catch (e) {
            console.error('Failed to parse SSE data:', e)
          }
        }
      }
    }
  } finally {
    reader.releaseLock()
  }
}
