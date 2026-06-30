<template>
  <div class="ai-center">
    <!-- 左侧会话列表 -->
    <aside class="ai-sidebar" v-if="!isMobile || !showChat">
      <div class="sidebar-header">
        <button class="new-chat-btn" @click="createNewSession">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 5v14M5 12h14"/>
          </svg>
          <span>新对话</span>
        </button>
      </div>

      <div class="sidebar-search">
        <input
          v-model="searchQuery"
          type="text"
          placeholder="搜索对话记录..."
          class="search-input"
        >
      </div>

      <div class="session-list">
        <div
          v-for="session in filteredSessions"
          :key="session.id"
          class="session-item"
          :class="{ active: currentSessionId === session.id }"
          @click="switchSession(session.id)"
        >
          <div class="session-item__icon" :class="`icon-${session.agent_type || 'default'}`">
            <svg v-if="!session.agent_type || session.agent_type === 'default'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
            </svg>
            <svg v-else-if="session.agent_type === 'rag'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
              <path d="M9 12h6M9 8h6M9 16h3"/>
            </svg>
            <svg v-else-if="session.agent_type === 'text2sql'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
              <path d="M8 12h8M8 8h8M8 16h4"/>
            </svg>
          </div>
          <div class="session-item__content">
            <div class="session-item__title">{{ session.title || '新对话' }}</div>
            <div class="session-item__meta">
              <span class="session-item__type">{{ getAgentTypeName(session.agent_type) }}</span>
              <span class="session-item__time">{{ formatSessionTime(session.created_at) }}</span>
            </div>
          </div>
          <button
            class="session-item__delete"
            @click.stop="deleteSession(session.id)"
            title="删除记录"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
            </svg>
          </button>
        </div>

        <div v-if="loadingSessions" class="session-list__loading">
          <div class="spinner"></div>
        </div>

        <div v-if="!loadingSessions && filteredSessions.length === 0" class="session-list__empty">
          <p>暂无对话记录</p>
          <p class="tip">点击"新对话"开始</p>
        </div>
      </div>
    </aside>

    <!-- 主内容区 -->
    <main class="ai-main">
      <!-- 移动端返回按钮 -->
      <header class="ai-header" v-if="isMobile && !showChat">
        <button class="back-btn" @click="showChat = true">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M19 12H5M12 19l-7-7 7-7"/>
          </svg>
          返回
        </button>
      </header>

      <!-- 顶部标题栏 -->
      <header class="ai-header" v-if="!isMobile || showChat">
        <div class="ai-header__title">
          <svg v-if="activeTab === 'knowledge'" class="icon-knowledge" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
            <path d="M9 12h6M9 8h6M9 16h3"/>
          </svg>
          <svg v-else-if="activeTab === 'text2sql'" class="icon-sql" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
            <path d="M8 12h8M8 8h8M8 16h4"/>
          </svg>
          <span>{{ currentSessionTitle }}</span>
        </div>
        <div class="ai-header__actions">
          <button
            v-if="isMobile"
            class="icon-btn"
            @click="showChat = false"
            title="会话列表"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M4 6h16M4 12h16M4 18h16"/>
            </svg>
          </button>
          <button class="icon-btn" @click="clearCurrentSession" title="清空当前对话">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
            </svg>
          </button>
        </div>
      </header>

      <!-- Tab 切换栏 -->
      <div class="ai-tabs">
        <button
          class="ai-tab"
          :class="{ active: activeTab === 'knowledge' }"
          @click="switchTab('knowledge')"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
            <path d="M9 12h6M9 8h6M9 16h3"/>
          </svg>
          <span>知识库对话</span>
        </button>
        <button
          class="ai-tab"
          :class="{ active: activeTab === 'text2sql' }"
          @click="switchTab('text2sql')"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
            <path d="M8 12h8M8 8h8M8 16h4"/>
          </svg>
          <span>SQL 查询</span>
        </button>
      </div>

      <!-- 消息区域 -->
      <div class="ai-messages" ref="messagesContainer">
        <!-- 空状态 -->
        <div v-if="messages.length === 0 && !streamingContent" class="ai-welcome">
          <div class="ai-welcome__icon" :class="`welcome-${activeTab}`">
            <svg v-if="activeTab === 'knowledge'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
              <path d="M9 12h6M9 8h6M9 16h3"/>
            </svg>
            <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
              <path d="M8 12h8M8 8h8M8 16h4"/>
            </svg>
          </div>
          <h2>{{ welcomeTitle }}</h2>
          <p>{{ welcomeDescription }}</p>
          <div class="ai-welcome__tips">
            <div class="tip-item" v-for="(tip, idx) in welcomeTips" :key="idx">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M9 12l2 2 4-4"/>
                <circle cx="12" cy="12" r="9"/>
              </svg>
              <span>{{ tip }}</span>
            </div>
          </div>
        </div>

        <!-- 消息列表 -->
        <div class="ai-message-list">
          <div
            v-for="message in messages"
            :key="message.id"
            class="ai-message"
            :class="`ai-message--${message.role}`"
          >
            <div class="ai-message__avatar">
              <span v-if="message.role === 'user'">{{ userInitial }}</span>
              <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                <path v-if="activeTab === 'knowledge'" d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
                <path v-if="activeTab === 'knowledge'" d="M9 12h6M9 8h6M9 16h3"/>
                <path v-if="activeTab === 'text2sql'" d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
                <path v-if="activeTab === 'text2sql'" d="M8 12h8M8 8h8M8 16h4"/>
              </svg>
            </div>
            <div class="ai-message__content">
              <div class="ai-message__bubble" v-html="renderMarkdown(message.content)"></div>
              <!-- Text2SQL 特有：SQL 查询结果 -->
              <div v-if="message.sql_result" class="sql-result">
                <div class="sql-result__header">
                  <span>查询结果</span>
                  <button class="copy-btn" @click="copyMessage(message.sql_result)" title="复制">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <rect x="9" y="9" width="13" height="13" rx="2"/>
                      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
                    </svg>
                  </button>
                </div>
                <pre class="sql-result__content"><code>{{ message.sql_result }}</code></pre>
                <div v-if="message.query_data" class="query-data">
                  <table>
                    <thead>
                      <tr>
                        <th v-for="(header, idx) in Object.keys(message.query_data[0] || {})" :key="idx">
                          {{ header }}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="(row, rowIdx) in message.query_data" :key="rowIdx">
                        <td v-for="(header, colIdx) in Object.keys(message.query_data[0] || {})" :key="colIdx">
                          {{ row[header] }}
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
              <div class="ai-message__actions">
                <span class="ai-message__time">{{ formatTime(message.created_at) }}</span>
                <button class="ai-message__action" @click="copyMessage(message.content)" title="复制">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <rect x="9" y="9" width="13" height="13" rx="2"/>
                    <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
                  </svg>
                </button>
              </div>
            </div>
          </div>

          <!-- 流式消息 -->
          <div v-if="streamingContent" class="ai-message ai-message--assistant ai-message--streaming">
            <div class="ai-message__avatar">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                <path v-if="activeTab === 'knowledge'" d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
                <path v-if="activeTab === 'knowledge'" d="M9 12h6M9 8h6M9 16h3"/>
                <path v-if="activeTab === 'text2sql'" d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
                <path v-if="activeTab === 'text2sql'" d="M8 12h8M8 8h8M8 16h4"/>
              </svg>
            </div>
            <div class="ai-message__content">
              <div class="ai-message__bubble" v-html="renderMarkdown(streamingContent)"></div>
              <div class="ai-message__actions">
                <span class="streaming-indicator">{{ streamingIndicator }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 输入区域 -->
      <div class="ai-input-wrapper">
        <div class="ai-input">
          <textarea
            v-model="inputMessage"
            class="ai-input__field"
            :placeholder="inputPlaceholder"
            rows="1"
            @keydown.enter.exact="sendMessage"
            @input="adjustTextareaHeight"
            ref="textareaRef"
            :disabled="isStreaming"
          ></textarea>
          <div class="ai-input__actions">
            <span class="ai-input__count">{{ inputMessage.length }} / 4000</span>
            <button
              class="ai-input__send"
              :disabled="!inputMessage.trim() || isStreaming"
              @click="sendMessage"
            >
              <svg v-if="!isStreaming" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M22 2L11 13M22 2l-7 20-4-9-9 9"/>
              </svg>
              <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="6" y="4" width="4" height="16"/>
                <rect x="14" y="4" width="4" height="16"/>
              </svg>
            </button>
          </div>
        </div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, nextTick, computed, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import MarkdownIt from 'markdown-it'
import { useAuthStore } from '@/stores/auth'
import { agentApi } from '@/api/agent'
import { sessionApi } from '@/api/session'
import type { Session, Message } from '@/types'
import { formatTime } from '@/utils'
import { copyToClipboard } from '@/utils/security'

const authStore = useAuthStore()
const userInitial = computed(() => authStore.username?.charAt(0).toUpperCase() || 'U')

// 扩展 Message 类型以支持 SQL 结果
interface AIMessage extends Message {
  sql_result?: string
  query_data?: any[]
  agent_type?: string
}

// Tab 配置
interface TabConfig {
  id: string
  name: string
  streamingText: string
  welcomeTitle: string
  welcomeDescription: string
  welcomeTips: string[]
  inputPlaceholder: string
}

const tabConfigs: Record<string, TabConfig> = {
  knowledge: {
    id: 'knowledge',
    name: '知识库对话',
    streamingText: '正在检索知识库...',
    welcomeTitle: '知识库助手',
    welcomeDescription: '基于知识库内容回答问题',
    welcomeTips: ['可以查询文档内容', '支持多轮对话', '自动检索相关信息'],
    inputPlaceholder: '输入你的问题，例如：项目的技术架构是什么？'
  },
  text2sql: {
    id: 'text2sql',
    name: 'SQL 查询',
    streamingText: '正在生成 SQL...',
    welcomeTitle: 'Text2SQL 助手',
    welcomeDescription: '用自然语言查询数据库，自动生成 SQL',
    welcomeTips: ['例如：查询销售额前10的产品', '支持分组、排序、筛选等操作', '自动展示查询结果'],
    inputPlaceholder: '输入你的查询问题，例如：查询最近一周的订单总额'
  }
}

// 状态
const activeTab = ref<'knowledge' | 'text2sql'>('knowledge')
const messages = ref<AIMessage[]>([])
const inputMessage = ref('')
const isStreaming = ref(false)
const streamingContent = ref('')
const messagesContainer = ref<HTMLElement>()
const textareaRef = ref<HTMLTextAreaElement>()

// Session 状态
const sessions = ref<Session[]>([])
const currentSessionId = ref<string>('')
const searchQuery = ref('')
const loadingSessions = ref(false)

// 移动端状态
const isMobile = ref(false)
const showChat = ref(true)

// Markdown
const md = new MarkdownIt({ html: true, linkify: true, typographer: true })

function renderMarkdown(content: string): string {
  return md.render(content)
}

// 计算属性
const filteredSessions = computed(() => {
  if (!searchQuery.value) return sessions.value
  const query = searchQuery.value.toLowerCase()
  return sessions.value.filter(s =>
    (s.title || '新对话').toLowerCase().includes(query)
  )
})

const currentSessionTitle = computed(() => {
  if (!currentSessionId.value) return tabConfigs[activeTab.value].name
  const session = sessions.value.find(s => s.id === currentSessionId.value)
  return session?.title || tabConfigs[activeTab.value].name
})

const welcomeTitle = computed(() => tabConfigs[activeTab.value].welcomeTitle)
const welcomeDescription = computed(() => tabConfigs[activeTab.value].welcomeDescription)
const welcomeTips = computed(() => tabConfigs[activeTab.value].welcomeTips)
const inputPlaceholder = computed(() => tabConfigs[activeTab.value].inputPlaceholder)
const streamingIndicator = computed(() => tabConfigs[activeTab.value].streamingText)

// Agent 类型显示名称
function getAgentTypeName(type?: string): string {
  switch (type) {
    case 'rag':
      return '知识库'
    case 'text2sql':
      return 'SQL'
    default:
      return '对话'
  }
}

// 加载会话列表
async function loadSessions() {
  loadingSessions.value = true
  try {
    const response = await sessionApi.list({ page: 1, page_size: 50 })
    if (response.data) {
      sessions.value = (response.data.sessions || []).map(session => ({
        ...session,
        created_at: typeof session.created_at === 'number'
          ? new Date(session.created_at * 1000).toISOString()
          : session.created_at,
        updated_at: typeof session.updated_at === 'number'
          ? new Date(session.updated_at * 1000).toISOString()
          : session.updated_at
      }))
    }
  } catch (error: any) {
    console.error('加载会话列表失败:', error)
  } finally {
    loadingSessions.value = false
  }
}

// 创建新会话
function createNewSession() {
  currentSessionId.value = ''
  messages.value = []
  inputMessage.value = ''
  searchQuery.value = ''
  if (isMobile.value) showChat.value = true
  nextTick(() => {
    textareaRef.value?.focus()
  })
}

// 切换会话
async function switchSession(sessionId: string) {
  currentSessionId.value = sessionId
  messages.value = []

  try {
    const response = await sessionApi.getDetail(sessionId)
    if (response.data) {
      messages.value = (response.data.messages || []).map(msg => ({
        ...msg,
        created_at: typeof msg.created_at === 'number'
          ? new Date(msg.created_at * 1000).toISOString()
          : msg.created_at
      }))
      // 根据 session 类型设置 activeTab
      const session = sessions.value.find(s => s.id === sessionId)
      if (session?.agent_type === 'text2sql') {
        activeTab.value = 'text2sql'
      } else {
        activeTab.value = 'knowledge'
      }
    }
  } catch (error: any) {
    console.error('加载会话详情失败:', error)
    ElMessage.error('加载对话失败')
  }

  if (isMobile.value) showChat.value = true
  nextTick(() => {
    scrollToBottom()
    textareaRef.value?.focus()
  })
}

// 删除会话
async function deleteSession(sessionId: string) {
  try {
    await ElMessageBox.confirm('确定要删除这个对话记录吗？', '确认删除', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })

    await sessionApi.delete(sessionId)
    ElMessage.success('删除成功')

    sessions.value = sessions.value.filter(s => s.id !== sessionId)

    if (currentSessionId.value === sessionId) {
      createNewSession()
    }
  } catch (error: any) {
    if (error !== 'cancel') {
      console.error('删除会话失败:', error)
      ElMessage.error(error.response?.data?.message || '删除失败')
    }
  }
}

// 清空当前会话
function clearCurrentSession() {
  if (messages.value.length === 0) return
  messages.value = []
  ElMessage.success('已清空当前对话')
}

// 切换 Tab
function switchTab(tab: 'knowledge' | 'text2sql') {
  if (activeTab.value === tab) return
  activeTab.value = tab
  // 切换 tab 时清空当前消息，创建新会话
  createNewSession()
}

// 发送消息
async function sendMessage() {
  const content = inputMessage.value.trim()
  if (!content || isStreaming.value) return

  const userMessage: AIMessage = {
    id: Date.now().toString(),
    session_id: currentSessionId.value,
    role: 'user',
    content,
    token_count: Math.ceil(content.length / 3),
    created_at: new Date().toISOString(),
    agent_type: activeTab.value === 'text2sql' ? 'text2sql' : 'rag'
  }
  messages.value.push(userMessage)

  inputMessage.value = ''
  adjustTextareaHeight()
  scrollToBottom()

  isStreaming.value = true
  streamingContent.value = ''

  try {
    let assistantContent = ''
    let sqlResult = ''
    let queryData: any[] | null = null
    let newSessionId = ''

    // 根据当前 tab 选择不同的流式 API
    const streamFn = activeTab.value === 'text2sql'
      ? agentApi.streamText2SQL
      : agentApi.streamKnowledgeChat

    for await (const event of streamFn({
      query: content,
      session_id: currentSessionId.value || undefined
    })) {
      // 更新 session_id
      if (event.session_id && event.session_id !== currentSessionId.value) {
        newSessionId = event.session_id
        currentSessionId.value = event.session_id
      }

      if (event.event === 'step') {
        if (event.type === 'thought' || event.type === 'thinking') {
          if (event.content) {
            streamingContent.value = event.content
            // 不要覆盖 assistantContent，只更新流式显示
            // assistantContent 应该从 done 事件的 answer 获取
          }
        } else if (event.type === 'tool_result' && event.tool_name === 'sql_execute') {
          if (event.tool_output) {
            try {
              const output = JSON.parse(event.tool_output)
              // executed_sql 包含实际执行的 SQL，rows 包含查询结果
              sqlResult = output.executed_sql || output.sql || ''
              // rows 是查询结果数据
              queryData = output.rows || output.data || null
            } catch {
              sqlResult = event.tool_output
            }
          }
        }
      } else if (event.event === 'done') {
        // done 事件包含完整答案
        if (event.answer) {
          assistantContent = event.answer
          streamingContent.value = event.answer
        } else if (event.content) {
          // 兼容：如果没有 answer 但有 content，使用 content
          assistantContent = event.content
          streamingContent.value = event.content
        }
      } else if (event.event === 'error') {
        ElMessage.error(event.content || '查询失败')
      }

      scrollToBottom()
    }

    // 添加助手回复
    messages.value.push({
      id: (Date.now() + 1).toString(),
      session_id: currentSessionId.value,
      role: 'assistant',
      content: assistantContent || '抱歉，没有收到回复内容。请稍后重试。',
      token_count: Math.ceil(assistantContent.length / 3),
      sql_result: sqlResult || undefined,
      query_data: queryData || undefined,
      agent_type: activeTab.value === 'text2sql' ? 'text2sql' : 'rag',
      created_at: new Date().toISOString()
    })

    // 如果没有收到内容，记录调试信息
    if (!assistantContent) {
      console.error('[Text2SQL] 没有收到回复内容，请检查后端 SSE 流')
    }

    // 重新加载会话列表
    if (newSessionId) {
      await loadSessions()
    }
  } catch (error: any) {
    console.error('发送消息失败:', error)
    ElMessage.error(error.response?.data?.message || '发送消息失败')
  } finally {
    isStreaming.value = false
    streamingContent.value = ''
  }
}

async function copyMessage(content: string) {
  const success = await copyToClipboard(content)
  if (success) ElMessage.success('已复制')
}

function scrollToBottom() {
  nextTick(() => {
    if (messagesContainer.value) {
      messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
    }
  })
}

function adjustTextareaHeight() {
  nextTick(() => {
    if (textareaRef.value) {
      textareaRef.value.style.height = 'auto'
      textareaRef.value.style.height = Math.min(textareaRef.value.scrollHeight, 200) + 'px'
    }
  })
}

function formatSessionTime(dateStr: string): string {
  const date = new Date(dateStr)
  const now = new Date()
  const diff = now.getTime() - date.getTime()

  if (diff < 24 * 60 * 60 * 1000 && date.getDate() === now.getDate()) {
    return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
  }

  const yesterday = new Date(now)
  yesterday.setDate(yesterday.getDate() - 1)
  if (date.getDate() === yesterday.getDate() && date.getMonth() === yesterday.getMonth()) {
    return '昨天'
  }

  return date.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })
}

function checkMobile() {
  isMobile.value = window.innerWidth < 768
  if (isMobile.value) {
    showChat.value = true
  }
}

onMounted(() => {
  loadSessions()
  checkMobile()
  window.addEventListener('resize', checkMobile)
  nextTick(() => {
    textareaRef.value?.focus()
  })
})

onUnmounted(() => {
  window.removeEventListener('resize', checkMobile)
})
</script>

<style scoped>
.ai-center {
  display: flex;
  height: 100vh;
  background: var(--bg-primary);
}

/* 左侧边栏 */
.ai-sidebar {
  width: 280px;
  background: var(--bg-secondary);
  border-right: 1px solid var(--border-subtle);
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}

.sidebar-header {
  padding: var(--space-4);
  border-bottom: 1px solid var(--border-subtle);
}

.new-chat-btn {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  padding: var(--space-3);
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: #fff;
  border: none;
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  font-weight: 500;
  cursor: pointer;
  transition: all var(--duration-fast);
}

.new-chat-btn:hover {
  opacity: 0.9;
  transform: translateY(-1px);
}

.new-chat-btn svg {
  width: 18px;
  height: 18px;
}

.sidebar-search {
  padding: var(--space-3) var(--space-4);
}

.search-input {
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  color: var(--text-primary);
  font-size: var(--text-sm);
  outline: none;
}

.search-input:focus {
  border-color: #667eea;
}

.session-list {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-2);
}

.session-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all var(--duration-fast);
}

.session-item:hover {
  background: var(--bg-hover);
}

.session-item.active {
  background: rgba(102, 126, 234, 0.15);
  border-left: 3px solid #667eea;
}

.session-item__icon {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  color: #667eea;
  background: rgba(102, 126, 234, 0.1);
}

.session-item__icon svg {
  width: 18px;
  height: 18px;
}

.session-item__content {
  flex: 1;
  min-width: 0;
}

.session-item__title {
  font-size: var(--text-sm);
  font-weight: 500;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.session-item__meta {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-top: var(--space-1);
  font-size: var(--text-xs);
  color: var(--text-muted);
}

.session-item__type {
  padding: 2px 6px;
  background: rgba(102, 126, 234, 0.1);
  border-radius: 4px;
  color: #667eea;
}

.session-item__delete {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  opacity: 0;
  transition: all var(--duration-fast);
}

.session-item:hover .session-item__delete {
  opacity: 1;
}

.session-item__delete:hover {
  color: #f56c6c;
  background: rgba(245, 108, 108, 0.1);
  border-radius: var(--radius-sm);
}

.session-item__delete svg {
  width: 14px;
  height: 14px;
}

.session-list__loading,
.session-list__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--space-8);
  color: var(--text-muted);
  font-size: var(--text-sm);
}

.session-list__empty .tip {
  font-size: var(--text-xs);
  margin-top: var(--space-2);
}

.spinner {
  width: 24px;
  height: 24px;
  border: 2px solid var(--border-subtle);
  border-top-color: #667eea;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* 主内容区 */
.ai-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: var(--bg-primary);
  min-width: 0;
}

/* 顶部标题栏 */
.ai-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--space-4) var(--space-6);
  border-bottom: 1px solid var(--border-subtle);
  background: var(--bg-secondary);
  flex-shrink: 0;
}

.ai-header__title {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.ai-header__title svg {
  width: 24px;
  height: 24px;
  color: #667eea;
}

.ai-header__title span {
  font: var(--font-display);
  font-size: var(--text-lg);
  font-weight: 600;
  color: var(--text-primary);
}

.ai-header__actions {
  display: flex;
  gap: var(--space-2);
}

.back-btn {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  background: transparent;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  color: var(--text-secondary);
  cursor: pointer;
  font-size: var(--text-sm);
}

.back-btn svg {
  width: 18px;
  height: 18px;
}

.icon-btn {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--duration-fast);
}

.icon-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.icon-btn svg {
  width: 18px;
  height: 18px;
}

/* Tab 切换栏 */
.ai-tabs {
  display: flex;
  padding: var(--space-2) var(--space-6);
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-subtle);
  gap: var(--space-2);
}

.ai-tab {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-4);
  background: transparent;
  border: none;
  border-radius: var(--radius-md);
  color: var(--text-secondary);
  font-size: var(--text-sm);
  cursor: pointer;
  transition: all var(--duration-fast);
}

.ai-tab:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.ai-tab.active {
  background: rgba(102, 126, 234, 0.1);
  color: #667eea;
}

.ai-tab svg {
  width: 16px;
  height: 16px;
}

/* 消息区域 */
.ai-messages {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-6);
}

.ai-message-list {
  max-width: 900px;
  margin: 0 auto;
}

/* 欢迎界面 */
.ai-welcome {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  text-align: center;
}

.ai-welcome__icon {
  width: 80px;
  height: 80px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: var(--space-6);
  color: #667eea;
  border-radius: 50%;
  background: rgba(102, 126, 234, 0.1);
}

.ai-welcome__icon svg {
  width: 40px;
  height: 40px;
}

.ai-welcome h2 {
  font: var(--font-display);
  font-size: var(--text-2xl);
  font-weight: 700;
  color: var(--text-primary);
  margin-bottom: var(--space-2);
}

.ai-welcome p {
  font-size: var(--text-base);
  color: var(--text-secondary);
  margin-bottom: var(--space-6);
}

.ai-welcome__tips {
  display: flex;
  gap: var(--space-6);
}

.tip-item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  color: var(--text-muted);
  font-size: var(--text-sm);
}

.tip-item svg {
  width: 16px;
  height: 16px;
  color: #667eea;
}

/* 消息气泡 */
.ai-message {
  display: flex;
  gap: var(--space-3);
  margin-bottom: var(--space-6);
  animation: messageIn 0.3s ease;
}

@keyframes messageIn {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.ai-message--user {
  flex-direction: row-reverse;
}

.ai-message__avatar {
  flex-shrink: 0;
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 600;
  font-size: var(--text-sm);
}

.ai-message--user .ai-message__avatar {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: #fff;
}

.ai-message--assistant .ai-message__avatar {
  background: rgba(102, 126, 234, 0.1);
  color: #667eea;
}

.ai-message--assistant .ai-message__avatar svg {
  width: 20px;
  height: 20px;
}

.ai-message__content {
  flex: 1;
  max-width: 75%;
}

.ai-message--user .ai-message__content {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
}

.ai-message__bubble {
  padding: var(--space-4);
  border-radius: var(--radius-lg);
  font-size: var(--text-base);
  line-height: 1.6;
  word-wrap: break-word;
  white-space: pre-wrap;
}

.ai-message__bubble :deep(p) {
  margin: 0;
}

.ai-message__bubble :deep(p:not(:last-child)) {
  margin-bottom: 0.5em;
}

.ai-message__bubble :deep(code) {
  background: var(--bg-tertiary);
  padding: 0.2em 0.4em;
  border-radius: 4px;
  font-size: 0.9em;
}

.ai-message__bubble :deep(pre) {
  background: var(--bg-tertiary);
  padding: var(--space-3);
  border-radius: var(--radius-md);
  overflow-x: auto;
  margin: var(--space-2) 0;
}

.ai-message__bubble :deep(pre code) {
  background: transparent;
  padding: 0;
}

.ai-message--user .ai-message__bubble {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: #fff;
  border-bottom-right-radius: var(--radius-sm);
}

.ai-message--assistant .ai-message__bubble {
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-bottom-left-radius: var(--radius-sm);
}

.ai-message--streaming .ai-message__bubble {
  border-left: 3px solid #667eea;
}

/* SQL 结果展示 */
.sql-result {
  margin-top: var(--space-3);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  overflow: hidden;
}

.sql-result__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--space-2) var(--space-3);
  background: rgba(102, 126, 234, 0.1);
  border-bottom: 1px solid var(--border-subtle);
  font-size: var(--text-sm);
  font-weight: 500;
  color: #667eea;
}

.copy-btn {
  padding: var(--space-1);
  background: transparent;
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: all var(--duration-fast);
}

.copy-btn:hover {
  background: rgba(102, 126, 234, 0.1);
  color: #667eea;
}

.copy-btn svg {
  width: 14px;
  height: 14px;
}

.sql-result__content {
  padding: var(--space-3);
  background: #1e1e2e;
  margin: 0;
  overflow-x: auto;
}

.sql-result__content code {
  color: #a6e3a1;
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 13px;
}

/* 查询结果表格 */
.query-data {
  overflow-x: auto;
}

.query-data table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-sm);
}

.query-data thead {
  background: var(--bg-tertiary);
}

.query-data th {
  padding: var(--space-2) var(--space-3);
  text-align: left;
  font-weight: 600;
  color: var(--text-primary);
  border-bottom: 1px solid var(--border-subtle);
}

.query-data td {
  padding: var(--space-2) var(--space-3);
  color: var(--text-secondary);
  border-bottom: 1px solid var(--border-subtle);
}

.query-data tbody tr:hover {
  background: var(--bg-hover);
}

.query-data tbody tr:last-child td {
  border-bottom: none;
}

.ai-message__actions {
  margin-top: var(--space-2);
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.ai-message--user .ai-message__actions {
  justify-content: flex-end;
}

.ai-message__time {
  font-size: var(--text-xs);
  color: var(--text-muted);
}

.ai-message__action {
  padding: var(--space-1);
  background: transparent;
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: all var(--duration-fast);
}

.ai-message__action:hover {
  background: var(--bg-hover);
  color: var(--text-secondary);
}

.ai-message__action svg {
  width: 14px;
  height: 14px;
}

.streaming-indicator {
  font-size: var(--text-sm);
  color: #667eea;
}

/* 输入区域 */
.ai-input-wrapper {
  padding: var(--space-4) var(--space-6);
  border-top: 1px solid var(--border-subtle);
  background: var(--bg-secondary);
  flex-shrink: 0;
}

.ai-input {
  max-width: 900px;
  margin: 0 auto;
  background: var(--bg-primary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  padding: var(--space-3) var(--space-4);
  display: flex;
  gap: var(--space-3);
  align-items: flex-end;
  transition: border-color var(--duration-fast);
}

.ai-input:focus-within {
  border-color: #667eea;
}

.ai-input__field {
  flex: 1;
  background: transparent;
  border: none;
  color: var(--text-primary);
  font-size: var(--text-base);
  line-height: 1.5;
  resize: none;
  outline: none;
  font-family: inherit;
  max-height: 200px;
}

.ai-input__field::placeholder {
  color: var(--text-muted);
}

.ai-input__actions {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.ai-input__count {
  font-size: var(--text-xs);
  color: var(--text-muted);
}

.ai-input__send {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  border: none;
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all var(--duration-fast);
  color: #fff;
}

.ai-input__send:hover:not(:disabled) {
  transform: scale(1.05);
}

.ai-input__send:disabled {
  background: var(--bg-tertiary);
  cursor: not-allowed;
  opacity: 0.5;
}

.ai-input__send svg {
  width: 18px;
  height: 18px;
}

/* 响应式 */
@media (max-width: 767px) {
  .ai-sidebar {
    position: fixed;
    left: 0;
    top: 0;
    bottom: 0;
    z-index: 100;
    transform: translateX(-100%);
    transition: transform 0.3s ease;
  }

  .ai-sidebar.show {
    transform: translateX(0);
  }

  .ai-welcome__tips {
    flex-direction: column;
    gap: var(--space-3);
  }

  .ai-message__content {
    max-width: 85%;
  }

  .ai-tabs {
    padding: var(--space-2) var(--space-4);
  }

  .ai-tab span {
    display: none;
  }
}
</style>
