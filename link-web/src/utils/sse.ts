/**
 * SSE 事件处理器接口
 */
export interface SSEHandlers {
  onMessage?: (event: MessageEvent) => void
  onError?: (event: Event) => void
  onOpen?: (event: Event) => void
}

/**
 * SSE 连接状态
 */
export enum SSEStatus {
  Connecting = 'connecting',
  Open = 'open',
  Closed = 'closed',
  Error = 'error'
}

/**
 * SSE 配置选项
 */
export interface SSEOptions {
  /**
   * 重连延迟（毫秒）
   */
  reconnectDelay?: number

  /**
   * 最大重连次数，-1 表示无限重连
   */
  maxRetries?: number

  /**
   * 是否自动重连
   */
  autoReconnect?: boolean

  /**
   * 心跳间隔（毫秒），用于检测连接是否存活
   */
  heartbeatInterval?: number
}

/**
 * SSE 客户端类
 *
 * 使用示例：
 * ```ts
 * const sse = new SSEClient()
 * sse.connect('/api/v1/stream', {
 *   onMessage: (e) => console.log(e.data),
 *   onError: (e) => console.error(e)
 * })
 * ```
 */
export class SSEClient {
  private eventSource: EventSource | null = null
  private status: SSEStatus = SSEStatus.Closed
  private url: string = ''
  private handlers: SSEHandlers = {}
  private options: Required<SSEOptions>
  private retryCount: number = 0
  private reconnectTimer: number | null = null
  private heartbeatTimer: number | null = null
  private lastHeartbeat: number = Date.now()

  constructor(options?: SSEOptions) {
    this.options = {
      reconnectDelay: options?.reconnectDelay || 3000,
      maxRetries: options?.maxRetries ?? -1,
      autoReconnect: options?.autoReconnect ?? true,
      heartbeatInterval: options?.heartbeatInterval || 30000
    }
  }

  /**
   * 连接 SSE 端点
   */
  connect(url: string, handlers: SSEHandlers = {}): void {
    if (this.eventSource) {
      this.disconnect()
    }

    this.url = url
    this.handlers = handlers
    this.status = SSEStatus.Connecting

    try {
      this.eventSource = new EventSource(url)

      // 连接成功
      this.eventSource.onopen = (event) => {
        this.status = SSEStatus.Open
        this.retryCount = 0
        this.lastHeartbeat = Date.now()
        this.startHeartbeat()
        this.handlers.onOpen?.(event)
      }

      // 接收消息
      this.eventSource.onmessage = (event) => {
        this.lastHeartbeat = Date.now()
        this.handlers.onMessage?.(event)
      }

      // 错误处理
      this.eventSource.onerror = (event) => {
        this.status = SSEStatus.Error
        this.handlers.onError?.(event)

        // EventSource 会在错误时自动尝试重连
        // 我们需要处理的是完全断开的情况
        if (this.eventSource?.readyState === EventSource.CLOSED) {
          this.status = SSEStatus.Closed
          this.stopHeartbeat()

          // 自动重连
          if (this.options.autoReconnect) {
            this.scheduleReconnect()
          }
        }
      }
    } catch (error) {
      this.status = SSEStatus.Error
      console.error('SSE connection error:', error)

      if (this.options.autoReconnect) {
        this.scheduleReconnect()
      }
    }
  }

  /**
   * 断开连接
   */
  disconnect(): void {
    this.stopHeartbeat()

    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }

    if (this.eventSource) {
      this.eventSource.close()
      this.eventSource = null
    }

    this.status = SSEStatus.Closed
    this.retryCount = 0
  }

  /**
   * 获取当前连接状态
   */
  getStatus(): SSEStatus {
    return this.status
  }

  /**
   * 是否已连接
   */
  isConnected(): boolean {
    return this.status === SSEStatus.Open && this.eventSource?.readyState === EventSource.OPEN
  }

  /**
   * 安排重连
   */
  private scheduleReconnect(): void {
    // 检查重试次数限制
    if (this.options.maxRetries >= 0 && this.retryCount >= this.options.maxRetries) {
      console.error('SSE max retries reached')
      return
    }

    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
    }

    this.retryCount++
    const delay = this.options.reconnectDelay

    console.log(`SSE reconnecting in ${delay}ms (attempt ${this.retryCount})`)

    this.reconnectTimer = window.setTimeout(() => {
      this.connect(this.url, this.handlers)
    }, delay)
  }

  /**
   * 启动心跳检测
   */
  private startHeartbeat(): void {
    this.stopHeartbeat()

    this.heartbeatTimer = window.setInterval(() => {
      const now = Date.now()
      const elapsed = now - this.lastHeartbeat

      // 如果超过心跳间隔的 1.5 倍没有收到消息，认为连接已断开
      if (elapsed > this.options.heartbeatInterval * 1.5) {
        console.warn('SSE heartbeat timeout, reconnecting...')
        this.disconnect()

        if (this.options.autoReconnect) {
          this.scheduleReconnect()
        }
      }
    }, this.options.heartbeatInterval)
  }

  /**
   * 停止心跳检测
   */
  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer)
      this.heartbeatTimer = null
    }
  }
}

/**
 * 创建 SSE 客户端实例的工厂函数
 */
export function createSSEClient(options?: SSEOptions): SSEClient {
  return new SSEClient(options)
}

/**
 * 连接到评测任务进度流
 *
 * @param taskId 任务ID
 * @param handlers 事件处理器
 * @param options SSE 选项
 * @returns SSE 客户端实例
 */
export function connectTaskProgress(
  taskId: string,
  handlers: SSEHandlers,
  options?: SSEOptions
): SSEClient {
  const url = `/api/v1/evaluation/tasks/${taskId}/stream`
  const client = createSSEClient(options)
  client.connect(url, handlers)
  return client
}
