/**
 * 统一 SSE 工具：
 * - readSSE：带鉴权头的 fetch 流式读取（Authorization + X-Tenant-ID、
 *   非 2xx 统一错误/登出、透传 AbortSignal）
 * - connectTaskProgress：评测任务进度流（基于 readSSE，4xx 停止重连）
 *
 * 不再使用裸 EventSource——它无法携带鉴权头，且 401/403 下会无限重连。
 */
import { storage } from '@/utils/security'
import { useAuthStore } from '@/stores/auth'
import { handleAuthFailure } from '@/utils/request'

/**
 * SSE 单条消息（已按 event/data 行解析）
 */
export interface SSEMessage {
  /** event: 行的值，无则为空串 */
  event: string
  /** data: 行的原始负载（未 JSON.parse） */
  data: string
}

/**
 * 非 2xx 响应错误，携带 HTTP 状态码供调用方区分 4xx/5xx
 */
export class SSEHttpError extends Error {
  readonly status: number

  constructor(status: number, message?: string) {
    super(message || `SSE HTTP error: ${status}`)
    this.name = 'SSEHttpError'
    this.status = status
  }
}

/**
 * 构建鉴权头（与 axios 拦截器同源：token + 当前租户）
 */
function buildAuthHeaders(): Record<string, string> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'Accept': 'text/event-stream'
  }
  const token = storage.get<string>('token')
  const currentTenant = storage.get<{ id: number | string }>('current_tenant')
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }
  if (currentTenant?.id) {
    headers['X-Tenant-ID'] = String(currentTenant.id)
  }
  return headers
}

/**
 * 发起一次流式请求；401 时走统一 refresh 单飞重放一次，仍失败则登出跳登录。
 */
async function fetchSSE(url: string, body: unknown, signal?: AbortSignal): Promise<Response> {
  const doFetch = () =>
    fetch(url, {
      method: body !== undefined ? 'POST' : 'GET',
      headers: buildAuthHeaders(),
      body: body !== undefined ? JSON.stringify(body) : undefined,
      signal
    })

  let response = await doFetch()

  // 401：尝试 refresh（stores/auth 内单飞）后重放一次
  if (response.status === 401) {
    const authStore = useAuthStore()
    const refreshed = await authStore.refreshAccessToken()
    if (refreshed) {
      response = await doFetch()
    }
    if (response.status === 401) {
      // 与 axios 拦截器共用一次性处理：只弹一次过期提示、只跳一次登录页
      handleAuthFailure()
      throw new SSEHttpError(401, '登录已过期，请重新登录')
    }
  }

  if (!response.ok) {
    throw new SSEHttpError(response.status)
  }
  return response
}

/**
 * 统一的带鉴权 SSE 读取器。
 *
 * @param url    完整请求地址（含 /api/v1 前缀）
 * @param body   请求体；undefined 时发 GET，否则 POST JSON
 * @param signal AbortSignal，中断时取消底层 fetch 并停止产出
 *
 * 使用示例：
 * ```ts
 * for await (const msg of readSSE('/api/v1/agent/chat/stream', { query }, signal)) {
 *   const event = JSON.parse(msg.data)
 * }
 * ```
 */
export async function* readSSE(
  url: string,
  body?: unknown,
  signal?: AbortSignal
): AsyncGenerator<SSEMessage> {
  const response = await fetchSSE(url, body, signal)

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
          yield { event: currentEvent, data }
          currentEvent = ''
        }
      }
    }
  } finally {
    reader.releaseLock()
    // 消费方 break/异常时取消底层流，通知后端客户端已断开（配合 Go 端 ctx.Done）
    try {
      await response.body?.cancel()
    } catch {
      /* 已关闭则忽略 */
    }
  }
}

/**
 * SSE 事件处理器接口（保留兼容原 EventSource 版签名的最小面）
 */
export interface SSEHandlers {
  onMessage?: (event: { data: string }) => void
  onError?: (error: unknown) => void
  onOpen?: () => void
}

/**
 * SSE 配置选项
 */
export interface SSEOptions {
  /** 重连延迟（毫秒） */
  reconnectDelay?: number
  /** 最大重连次数，-1 表示无限重连（仅网络/5xx 错误；4xx 一律停止） */
  maxRetries?: number
  /** 是否自动重连 */
  autoReconnect?: boolean
}

/**
 * SSE 连接句柄
 */
export interface SSEConnection {
  disconnect(): void
  isConnected(): boolean
}

/**
 * 连接到评测任务进度流（带鉴权头；4xx 停止重连并抛给 onError）。
 *
 * @param taskId 任务ID
 * @param handlers 事件处理器
 * @param options SSE 选项
 * @returns 连接句柄（disconnect 中断流）
 */
export function connectTaskProgress(
  taskId: string,
  handlers: SSEHandlers,
  options?: SSEOptions
): SSEConnection {
  const url = `/api/v1/evaluation/tasks/${taskId}/stream`
  const reconnectDelay = options?.reconnectDelay ?? 3000
  const maxRetries = options?.maxRetries ?? -1
  const autoReconnect = options?.autoReconnect ?? true

  const controller = new AbortController()
  let connected = false
  let retryCount = 0

  const run = async () => {
    while (!controller.signal.aborted) {
      try {
        connected = true
        handlers.onOpen?.()
        for await (const msg of readSSE(url, undefined, controller.signal)) {
          retryCount = 0
          handlers.onMessage?.({ data: msg.data })
        }
        // 服务端正常结束（任务完成后关流）：不再重连
        break
      } catch (error) {
        connected = false
        if (controller.signal.aborted) break

        // 4xx 属于客户端/鉴权/参数错误：重连不可能自愈，停止并抛出
        if (error instanceof SSEHttpError && error.status >= 400 && error.status < 500) {
          handlers.onError?.(error)
          break
        }

        handlers.onError?.(error)
        if (!autoReconnect) break
        if (maxRetries >= 0 && retryCount >= maxRetries) {
          console.error('SSE max retries reached')
          break
        }
        retryCount++
        await new Promise((resolve) => setTimeout(resolve, reconnectDelay))
      }
    }
    connected = false
  }

  void run()

  return {
    disconnect() {
      controller.abort()
      connected = false
    },
    isConnected() {
      return connected
    }
  }
}
