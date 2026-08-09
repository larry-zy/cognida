import { ref, nextTick, onUnmounted, type Ref } from 'vue'
import toast from '@/utils/toast'
import type { AgentStep, AgentStreamEvent, Message } from '@/types'

// ========================================
// useAgentChat —— 对话发送 / 流式 / 中断 / 流式状态
// ========================================
//
// 抽取知识库助手与数据智能体两个页面共有的发送-流式-落库骨架：
//   push 用户消息 → 开流 → 逐事件消费（thought 追加内容 / tool_* 合并步骤 / done 定稿）
//   → push 助手消息 → 新会话回刷。
// 页面差异（携带的请求参数、rag_context / ui 等副产物、错误分支的部分落库）
// 通过 options 回调注入，保持各页面既有行为不变。

// 解析持久化的 agent_steps JSON 字符串为时间线步骤（会话重开重现思考过程）。
export function parseAgentSteps(raw?: string): AgentStep[] {
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    const list = Array.isArray(parsed) ? parsed : parsed?.steps
    if (!Array.isArray(list)) return []
    return list
      .filter((s: any) => s && (s.type === 'tool_call' || s.type === 'tool_result'))
      .map((s: any, idx: number): AgentStep => ({
        id: `${s.tool_id || s.step || idx}`,
        step: Number(s.step) || idx + 1,
        type: s.type,
        tool_name: s.tool_name,
        tool_input: s.tool_input,
        tool_output: s.tool_output,
        tool_id: s.tool_id,
        status: s.status,
        error: s.error,
        timestamp: 0
      }))
      .reduce((acc: AgentStep[], cur: AgentStep) => {
        const key = cur.tool_id || String(cur.step)
        const existing = acc.find(a => (a.tool_id || String(a.step)) === key)
        if (existing) Object.assign(existing, cur)
        else acc.push(cur)
        return acc
      }, [])
  } catch {
    return []
  }
}

// 合并流式步骤事件：tool_call 新建，tool_result 更新同一步。
function upsertStep(target: AgentStep[], event: AgentStreamEvent) {
  const key = event.tool_id || String(event.step)
  const existing = target.find(s => (s.tool_id || String(s.step)) === key)
  const patch = {
    step: Number(event.step) || target.length + 1,
    type: event.type,
    tool_name: event.tool_name ?? existing?.tool_name,
    tool_input: event.tool_input ?? existing?.tool_input,
    tool_output: event.tool_output ?? existing?.tool_output,
    tool_id: event.tool_id ?? existing?.tool_id,
    status: event.status ?? existing?.status,
    error: event.error ?? existing?.error
  }
  if (existing) {
    Object.assign(existing, patch)
  } else {
    target.push({ id: key, timestamp: 0, ...patch } as AgentStep)
  }
}

/** 单次发送过程中在事件循环里累计的状态 */
export interface SendState {
  /** done 事件定稿的助手回答内容 */
  assistantContent: string
  /** 流中若下发了新的 session_id */
  newSessionId: string
}

/** 错误/中断分支的上下文：用于把已流式产出的部分落成一条消息 */
export interface SendErrorContext {
  state: SendState
  streamingContent: string
  streamingSteps: AgentStep[]
  /** 用当前流式步骤快照 push 一条助手消息 */
  commit: (content: string) => void
}

export interface UseAgentChatOptions<M extends Message> {
  messages: Ref<M[]>
  currentSessionId: Ref<string>
  inputMessage: Ref<string>
  textareaRef: Ref<HTMLTextAreaElement | undefined>
  messagesContainer: Ref<HTMLElement | undefined>
  /** 为一次发送构造流式生成器（携带各页面自己的请求参数） */
  createStream: (content: string, signal: AbortSignal) => AsyncGenerator<AgentStreamEvent>
  /** 成功收流后构造要 push 的助手消息（读取页面自己的副产物：ragContext / ui_surfaces 等） */
  buildAssistantMessage: (content: string, ctx: { steps: AgentStep[] }) => M
  /** 逐事件回调：页面用于捕获 rag_context、消费 ui 事件等副产物 */
  onEvent?: (event: AgentStreamEvent, state: SendState) => void
  /** 发送开始（重置页面自己的流式副产物状态） */
  onSendStart?: (content: string) => void
  /** 发送结束 finally（清理页面自己的流式副产物状态） */
  onSendEnd?: () => void
  /** 收到新 session_id 后的回调（通常回刷会话列表） */
  onNewSession?: () => void | Promise<void>
  /** 自定义错误/中断处理；不提供则默认「非主动中断才 console + toast」 */
  onError?: (err: any, ctx: SendErrorContext) => void
}

export function useAgentChat<M extends Message>(options: UseAgentChatOptions<M>) {
  const { messages, currentSessionId, inputMessage, textareaRef, messagesContainer } = options

  const isStreaming = ref(false)
  const streamingContent = ref('')
  const streamingSteps = ref<AgentStep[]>([])
  const composerFocused = ref(false)
  // 当前流式请求的中断控制器（组件卸载或重新发送时 abort）
  let streamAbort: AbortController | null = null

  function scrollToBottom() {
    nextTick(() => {
      if (messagesContainer.value) {
        messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
      }
    })
  }

  function adjustTextareaHeight() {
    nextTick(() => {
      const el = textareaRef.value
      if (el) {
        el.style.height = 'auto'
        el.style.height = Math.min(el.scrollHeight, 180) + 'px'
      }
    })
  }

  // 回车发送：输入法合成期（拼音候选框开着）按回车只确认选字，不触发发送
  function onEnterKey(e: KeyboardEvent) {
    if (e.isComposing || e.keyCode === 229) return
    e.preventDefault()
    sendMessage()
  }

  async function sendMessage() {
    const content = inputMessage.value.trim()
    if (!content || isStreaming.value) return

    messages.value.push({
      id: Date.now().toString(),
      session_id: currentSessionId.value,
      role: 'user',
      content,
      token_count: Math.ceil(content.length / 3),
      created_at: new Date().toISOString()
    } as M)

    inputMessage.value = ''
    adjustTextareaHeight()
    scrollToBottom()

    isStreaming.value = true
    streamingContent.value = ''
    streamingSteps.value = []
    options.onSendStart?.(content)

    const state: SendState = { assistantContent: '', newSessionId: '' }

    streamAbort?.abort()
    streamAbort = new AbortController()

    const commit = (c: string) => {
      messages.value.push(options.buildAssistantMessage(c, { steps: streamingSteps.value }))
    }

    try {
      for await (const event of options.createStream(content, streamAbort.signal)) {
        // 更新 session_id
        if (event.session_id && event.session_id !== currentSessionId.value) {
          state.newSessionId = event.session_id
          currentSessionId.value = event.session_id
        }

        if (event.event === 'step') {
          if (event.type === 'thought' || event.type === 'thinking') {
            if (event.content) streamingContent.value += event.content
          } else if (event.type === 'tool_call' || event.type === 'tool_result') {
            upsertStep(streamingSteps.value, event)
          } else if (event.type === 'error') {
            toast.error(event.error || event.content || '执行出错')
          }
        } else if (event.event === 'done') {
          if (event.answer) {
            state.assistantContent = event.answer
            streamingContent.value = event.answer
          } else if (event.content) {
            state.assistantContent = event.content
            streamingContent.value = event.content
          }
        } else if (event.event === 'error') {
          toast.error(event.error || event.content || '查询失败')
        }

        // 页面副产物（rag_context / ui 等）
        options.onEvent?.(event, state)

        scrollToBottom()
      }

      commit(state.assistantContent || '抱歉，没有收到回复内容，请稍后重试。')

      if (state.newSessionId) await options.onNewSession?.()
    } catch (err: any) {
      if (options.onError) {
        options.onError(err, {
          state,
          streamingContent: streamingContent.value,
          streamingSteps: streamingSteps.value,
          commit
        })
      } else if (err?.name !== 'AbortError') {
        // 主动中断（组件卸载/重新发送）不提示错误
        console.error('发送消息失败:', err)
        toast.error(err?.message || '发送消息失败')
      }
    } finally {
      isStreaming.value = false
      streamingContent.value = ''
      streamingSteps.value = []
      options.onSendEnd?.()
      nextTick(scrollToBottom)
    }
  }

  onUnmounted(() => {
    // 路由离开时中断未完成的 SSE 连接
    streamAbort?.abort()
  })

  return {
    isStreaming,
    streamingContent,
    streamingSteps,
    composerFocused,
    sendMessage,
    onEnterKey,
    adjustTextareaHeight,
    scrollToBottom
  }
}
