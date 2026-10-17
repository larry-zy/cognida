import { ref, computed, nextTick, onUnmounted, type Ref } from 'vue'
import { sessionApi } from '@/api/session'
import type { Session, Message } from '@/types'
import toast from '@/utils/toast'
import { ElMessageBox } from '@/utils/confirm'

// ========================================
// useChatSessions —— 会话列表 / 切换 / 删除 / 新建
// ========================================
//
// 抽取知识库助手与数据智能体两个页面共有的会话侧栏逻辑。页面差异（会话过滤谓词、
// 历史消息映射、删除确认与加载失败文案、切换后滚动行为）通过 options 注入。
// 依赖对话核心状态（messages / currentSessionId / inputMessage / textareaRef），
// 由页面创建后共享给本 composable 与 useAgentChat。

export interface UseChatSessionsOptions<M extends Message> {
  messages: Ref<M[]>
  currentSessionId: Ref<string>
  inputMessage: Ref<string>
  textareaRef: Ref<HTMLTextAreaElement | undefined>
  /** 本页面归属的会话过滤谓词（如 rag / text2sql） */
  filter: (s: Session) => boolean
  /** 把持久化消息映射成页面自己的消息类型（补 steps / ui_surfaces 等） */
  mapMessage: (msg: Message) => M
  /** 搜索时标题为空的兜底文案（如「新对话」/「新分析」） */
  searchTitleFallback: string
  /** 删除确认弹窗文案 */
  deleteConfirmMessage: string
  /** 加载会话详情失败提示 */
  loadFailMessage: string
  /** 切换会话并加载完成后的回调（通常滚动到底部） */
  onAfterSwitch?: () => void
  /** 离开当前会话前取消其流，防止迟到结果写入新会话。 */
  onBeforeSwitch?: () => void
}

export function useChatSessions<M extends Message>(options: UseChatSessionsOptions<M>) {
  const { messages, currentSessionId, inputMessage, textareaRef } = options

  const sessions = ref<Session[]>([])
  const searchQuery = ref('')
  const loadingSessions = ref(false)
  let historyGeneration = 0
  let listGeneration = 0
  let disposed = false

  function invalidateHistory() {
    historyGeneration++
  }

  onUnmounted(() => {
    disposed = true
    invalidateHistory()
    listGeneration++
  })

  const filteredSessions = computed(() => {
    const list = sessions.value.filter(options.filter)
    if (!searchQuery.value.trim()) return list
    const q = searchQuery.value.toLowerCase()
    return list.filter(s => (s.title || options.searchTitleFallback).toLowerCase().includes(q))
  })

  async function loadSessions() {
    const generation = ++listGeneration
    loadingSessions.value = true
    try {
      const res = await sessionApi.list({ page: 1, page_size: 50 })
      if (disposed || generation !== listGeneration) return
      if (res.data) {
        sessions.value = (res.data.sessions || []).map(s => ({
          ...s,
          created_at: typeof s.created_at === 'number'
            ? new Date((s.created_at as unknown as number) * 1000).toISOString()
            : s.created_at,
          updated_at: typeof s.updated_at === 'number'
            ? new Date((s.updated_at as unknown as number) * 1000).toISOString()
            : s.updated_at
        }))
      }
    } catch (err) {
      if (!disposed && generation === listGeneration) console.error('加载会话列表失败:', err)
    } finally {
      if (!disposed && generation === listGeneration) loadingSessions.value = false
    }
  }

  function createNewSession() {
    invalidateHistory()
    options.onBeforeSwitch?.()
    currentSessionId.value = ''
    messages.value = []
    inputMessage.value = ''
    searchQuery.value = ''
    nextTick(() => textareaRef.value?.focus())
  }

  async function switchSession(sessionId: string) {
    options.onBeforeSwitch?.()
    const generation = ++historyGeneration
    const targetId = sessionId
    const isCurrent = () => !disposed && generation === historyGeneration && currentSessionId.value === targetId
    currentSessionId.value = targetId
    messages.value = []
    try {
      const res = await sessionApi.getDetail(targetId)
      // 竞态保护：await 期间用户可能已切到其他会话/新建会话，丢弃过期结果
      if (!isCurrent()) return
      if (res.data) {
        messages.value = (res.data.messages || []).map(options.mapMessage)
      }
    } catch (err) {
      if (!isCurrent()) return
      console.error('加载会话详情失败:', err)
      toast.error(options.loadFailMessage)
    }
    nextTick(() => {
      if (!isCurrent()) return
      options.onAfterSwitch?.()
      textareaRef.value?.focus()
    })
  }

  async function deleteSession(sessionId: string) {
    try {
      await ElMessageBox.confirm({
        message: options.deleteConfirmMessage,
        title: '确认删除',
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning'
      })
      await sessionApi.delete(sessionId)
      toast.success('删除成功')
      sessions.value = sessions.value.filter(s => s.id !== sessionId)
      if (currentSessionId.value === sessionId) createNewSession()
    } catch (err) {
      if (err !== 'cancel') {
        console.error('删除会话失败:', err)
        toast.error('删除失败')
      }
    }
  }

  function formatSessionTime(dateStr: string): string {
    const date = new Date(dateStr)
    if (isNaN(date.getTime())) return ''
    const now = new Date()
    if (date.toDateString() === now.toDateString()) {
      return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
    }
    const yesterday = new Date(now)
    yesterday.setDate(yesterday.getDate() - 1)
    if (date.toDateString() === yesterday.toDateString()) return '昨天'
    return date.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })
  }

  return {
    sessions,
    searchQuery,
    loadingSessions,
    filteredSessions,
    loadSessions,
    invalidateHistory,
    createNewSession,
    switchSession,
    deleteSession,
    formatSessionTime
  }
}
