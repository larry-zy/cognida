<!--
  DaDa 数据智能体 —— 对话驱动 + 结果画布（Layout B）。

  只绑定 Data Agent：不选智能体、不选知识库。请求走 /agent/text2sql/stream
  （preset agent-data-agent，会话落库 agent_type=text2sql），流中除 step/done
  外，会随 render_ui / sql_mutate 工具成功「实时」下发 ui 事件（A2UI UISpec）。

  三栏：会话侧栏 | 窄对话线 | 结果画布。
  - 展示区实时：ui 事件到达即把 surface 推入画布并即时渲染（不等流结束）。
  - 不必全部展示：画布只渲染智能体本次「真实产出」的 surface，动态多面板。
  - 历史重现：会话重开从 agent_steps.ui_surfaces 恢复画布，不依赖 SSE 重放。
-->
<template>
  <div class="dada">

    <!-- ================= 会话侧栏 ================= -->
    <aside class="sessions">
      <div class="sessions__head">
        <button class="btn-new" @click="createNewSession">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
            <path d="M12 5v14M5 12h14"/>
          </svg>
          新对话
        </button>
      </div>

      <div class="sessions__search">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="11" cy="11" r="7"/>
          <path d="M21 21l-4.3-4.3"/>
        </svg>
        <input
          v-model="searchQuery"
          type="text"
          placeholder="搜索分析记录…"
          class="sessions__search-input"
        />
      </div>

      <div class="sessions__group">分析记录</div>

      <div class="sessions__list">
        <div v-if="loadingSessions" class="sessions__loading"><div class="spinner"></div></div>

        <template v-else>
          <div
            v-for="session in filteredSessions"
            :key="session.id"
            class="session"
            :class="{ 'session--active': currentSessionId === session.id }"
            @click="switchSession(session.id)"
          >
            <div class="session__icon">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                <path d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
                <path d="M8 12h8M8 8h8M8 16h4"/>
              </svg>
            </div>
            <div class="session__body">
              <div class="session__title">{{ session.title || '新分析' }}</div>
              <div class="session__meta">
                <span class="session__chip">数据</span>
                <span>{{ formatSessionTime(session.created_at) }}</span>
              </div>
            </div>
            <button class="session__delete" title="删除" @click.stop="deleteSession(session.id)">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
              </svg>
            </button>
          </div>

          <div v-if="!loadingSessions && filteredSessions.length === 0" class="sessions__empty">
            <span>暂无分析记录</span>
          </div>
        </template>
      </div>
    </aside>

    <!-- ================= 对话线（窄栏） ================= -->
    <section class="thread">
      <div class="topbar">
        <span class="topbar__title">DaDa 数据智能体</span>
        <span class="status-dot">
          <i></i>data-agent · {{ isStreaming ? 'streaming' : 'online' }}
        </span>
      </div>

      <div class="messages" ref="messagesContainer">
        <div class="messages__inner">
          <!-- 欢迎空态 -->
          <div
            v-if="messages.length === 0 && !streamingContent && streamingSteps.length === 0"
            class="welcome"
          >
            <div class="welcome__icon">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                <path d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
                <path d="M8 12h8M8 8h8M8 16h4"/>
              </svg>
            </div>
            <h2 class="welcome__title">DaDa 数据智能体</h2>
            <p class="welcome__desc">用自然语言提问，智能体自动取数、分析并把结果实时画到右侧画布。</p>
            <div class="welcome__suggest">
              <span
                v-for="q in starterQuestions"
                :key="q"
                class="suggest-chip"
                @click="applySuggestion(q)"
              >{{ q }}</span>
            </div>
          </div>

          <!-- 历史消息 -->
          <template v-for="message in messages" :key="message.id">
            <div v-if="message.role === 'user'" class="msg msg--user">
              <div class="avatar avatar--user">{{ userInitial }}</div>
              <div class="msg__col">
                <div class="bubble bubble--user">{{ message.content }}</div>
                <div class="msg__time">{{ formatTime(message.created_at) }}</div>
              </div>
            </div>

            <div v-else-if="message.role === 'assistant'" class="msg msg--ai">
              <div class="avatar avatar--ai">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                  <path d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
                  <path d="M8 12h8M8 8h8M8 16h4"/>
                </svg>
              </div>
              <div class="msg__col">
                <AgentTimeline
                  v-if="message.steps && message.steps.length > 0"
                  :steps="message.steps"
                  :default-open="false"
                />
                <div class="bubble bubble--ai" v-html="renderMarkdown(message.content)"></div>
                <div v-if="message.ui_surfaces && message.ui_surfaces.length" class="canvas-ref">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M3 3h18v14H3z"/><path d="M8 21h8M12 17v4"/>
                  </svg>
                  已在画布生成 {{ message.ui_surfaces.length }} 个可视化面板
                </div>
                <div class="msg__time">{{ formatTime(message.created_at) }}</div>
              </div>
            </div>
          </template>

          <!-- 流式消息 -->
          <div v-if="streamingContent || streamingSteps.length > 0" class="msg msg--ai">
            <div class="avatar avatar--ai">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                <path d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
                <path d="M8 12h8M8 8h8M8 16h4"/>
              </svg>
            </div>
            <div class="msg__col">
              <AgentTimeline v-if="streamingSteps.length > 0" :steps="streamingSteps" :active="true" />
              <div v-if="streamingContent" class="bubble bubble--ai" v-html="renderMarkdown(streamingContent)"></div>
              <div v-else class="streaming-cursor"><span></span><span></span><span></span></div>
            </div>
          </div>
        </div>
      </div>

      <!-- 输入区：仅 query + session_id，无智能体/知识库选择 -->
      <div class="composer-wrap">
        <div class="composer" :class="{ 'composer--focus': composerFocused }">
          <textarea
            v-model="inputMessage"
            ref="textareaRef"
            placeholder="向数据智能体提问，例如：近 30 天各渠道 GMV 趋势…"
            class="composer__textarea"
            rows="1"
            :disabled="isStreaming"
            @keydown.enter.exact.prevent="sendMessage"
            @input="adjustTextareaHeight"
            @focus="composerFocused = true"
            @blur="composerFocused = false"
          ></textarea>
          <div class="composer__foot">
            <span class="scope-hint">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M4 7v10c0 2 1 3 3 3h10c2 0 3-1 3-3V7c0-2-1-3-3-3H7C5 4 4 5 4 7z"/>
                <path d="M8 12h8M8 8h8"/>
              </svg>
              数据智能体
            </span>

            <!-- 数据源选择器 -->
            <div class="ds-selector" @click.stop>
              <button
                ref="dsTriggerRef"
                class="ds-selector__trigger"
                :class="{ 'ds-selector__trigger--active': selectedDatasourceId }"
                @click.stop="openDsDropdown"
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                  <ellipse cx="12" cy="5" rx="7" ry="2.5"/>
                  <path d="M5 5v14c0 1.4 3.1 2.5 7 2.5s7-1.1 7-2.5V5"/>
                  <path d="M5 12c0 1.4 3.1 2.5 7 2.5s7-1.1 7-2.5"/>
                </svg>
                {{ selectedDatasourceName }}
                <svg class="ds-selector__caret" :class="{ open: dsSelectOpen }" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <polyline points="6 9 12 15 18 9"/>
                </svg>
              </button>
              <Teleport to="body">
                <div v-if="dsSelectOpen" class="ds-selector__dropdown" :style="dsDropdownPos" @click.stop>
                  <div
                    v-for="opt in datasourceOptions"
                    :key="opt.id"
                    class="ds-selector__option"
                    :class="{ selected: selectedDatasourceId === opt.id }"
                    @click="selectDatasource(opt.id)"
                  >
                    {{ opt.name }}
                    <svg v-if="selectedDatasourceId === opt.id" class="ds-selector__check" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                      <polyline points="20 6 9 17 4 12"/>
                    </svg>
                  </div>
                </div>
              </Teleport>
            </div>

            <span class="composer__count">{{ inputMessage.length }} / 4000</span>
            <button class="btn-send" :disabled="!inputMessage.trim() || isStreaming" @click="sendMessage">
              <svg v-if="!isStreaming" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
                <path d="M22 2L11 13M22 2l-7 20-4-9-9 9"/>
              </svg>
              <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/>
              </svg>
            </button>
          </div>
        </div>
      </div>
    </section>

    <!-- ================= 结果画布（实时） ================= -->
    <section class="canvas">
      <div class="canvas__bar">
        <span class="canvas__title">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M3 3h18v14H3z"/><path d="M8 21h8M12 17v4"/>
          </svg>
          结果画布
        </span>
        <span v-if="isStreaming" class="canvas__live"><i></i>实时生成中</span>
        <span v-else-if="canvasGroups.length" class="canvas__count">{{ totalSurfaces }} 个面板</span>
      </div>

      <div class="canvas__body" ref="canvasContainer">
        <!-- 空态 -->
        <div v-if="canvasGroups.length === 0" class="canvas-empty">
          <div class="canvas-empty__icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.4">
              <path d="M3 3v18h18"/><path d="M7 15l3-4 3 2 4-6"/>
            </svg>
          </div>
          <p class="canvas-empty__title">智能体产出的可视化会实时出现在这里</p>
          <p class="canvas-empty__desc">指标卡、明细表、趋势图等由智能体按需生成，只展示本次分析真正需要的面板。</p>
        </div>

        <!-- 按轮次分组的 surface -->
        <div v-for="group in canvasGroups" :key="group.key" class="canvas-group">
          <div class="canvas-group__caption">
            <span class="canvas-group__q">{{ group.query || '分析结果' }}</span>
            <span v-if="group.live" class="canvas-group__live"><i></i>生成中</span>
          </div>
          <A2UIRenderer
            v-for="surface in group.surfaces"
            :key="surface.surface || surface.title"
            :spec="surface"
            @action="onA2UIAction"
          />
          <div v-if="group.live && group.surfaces.length === 0" class="canvas-group__pending">
            <div class="streaming-cursor"><span></span><span></span><span></span></div>
            正在取数与分析，面板即将出现…
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import MarkdownIt from 'markdown-it'
import { useAuthStore } from '@/stores/auth'
import { agentApi } from '@/api/agent'
import { sessionApi } from '@/api/session'
import { datasourceApi } from '@/api/datasource'
import type { Datasource } from '@/api/datasource'
import type { Session, Message, AgentStep } from '@/types'
import { formatTime } from '@/utils'
import toast from '@/utils/toast'
import { ElMessageBox } from '@/utils/confirm'
import AgentTimeline from '@/components/agent/AgentTimeline.vue'
import A2UIRenderer from '@/components/agent/A2UIRenderer.vue'
import type { A2UISpec, A2UIAction } from '@/components/agent/a2ui-context'

// ===== 类型扩展 =====
interface DataMessage extends Message {
  steps?: AgentStep[]
  ui_surfaces?: A2UISpec[]   // 本轮智能体产出的 A2UI 面板（可多个）
}

// ===== Auth / Markdown =====
const authStore = useAuthStore()
const userInitial = computed(() => authStore.username?.charAt(0).toUpperCase() || 'U')
const md = new MarkdownIt({ html: false, linkify: true, typographer: true })
function renderMarkdown(content: string): string {
  return md.render(content || '')
}

// ===== 预制起始问题（预制即可）=====
const starterQuestions = [
  '近 30 天各渠道 GMV 趋势',
  '销售额 Top 10 的商品',
  '本月新增用户与留存概览'
]
function applySuggestion(text: string) {
  inputMessage.value = text
  nextTick(() => {
    textareaRef.value?.focus()
    adjustTextareaHeight()
  })
}

// ===== 会话状态（只列数据智能体会话）=====
const sessions = ref<Session[]>([])
const currentSessionId = ref<string>('')
const searchQuery = ref('')
const loadingSessions = ref(false)

const filteredSessions = computed(() => {
  const dataSessions = sessions.value.filter(s => s.agent_type === 'text2sql')
  if (!searchQuery.value.trim()) return dataSessions
  const q = searchQuery.value.toLowerCase()
  return dataSessions.filter(s => (s.title || '新分析').toLowerCase().includes(q))
})

async function loadSessions() {
  loadingSessions.value = true
  try {
    const res = await sessionApi.list({ page: 1, page_size: 50 })
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
    console.error('加载会话列表失败:', err)
  } finally {
    loadingSessions.value = false
  }
}

function createNewSession() {
  currentSessionId.value = ''
  messages.value = []
  inputMessage.value = ''
  searchQuery.value = ''
  nextTick(() => textareaRef.value?.focus())
}

async function switchSession(sessionId: string) {
  const targetId = sessionId
  currentSessionId.value = targetId
  messages.value = []
  try {
    const res = await sessionApi.getDetail(targetId)
    // 竞态保护：await 期间用户可能已切走
    if (currentSessionId.value !== targetId) return
    if (res.data) {
      messages.value = (res.data.messages || []).map(msg => ({
        ...msg,
        created_at: typeof msg.created_at === 'number'
          ? new Date((msg.created_at as unknown as number) * 1000).toISOString()
          : msg.created_at,
        steps: parseAgentSteps(msg.agent_steps),
        ui_surfaces: parseUISurfaces(msg.agent_steps)
      } as DataMessage))
    }
  } catch (err) {
    if (currentSessionId.value !== targetId) return
    console.error('加载会话详情失败:', err)
    toast.error('加载分析记录失败')
  }
  nextTick(() => {
    scrollThreadToBottom()
    textareaRef.value?.focus()
  })
}

async function deleteSession(sessionId: string) {
  try {
    await ElMessageBox.confirm({
      message: '确定要删除这条分析记录吗？',
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

// ===== 消息 / 流式状态 =====
const messages = ref<DataMessage[]>([])
const inputMessage = ref('')
const isStreaming = ref(false)
const streamingContent = ref('')
const streamingSteps = ref<AgentStep[]>([])
const streamingUISurfaces = ref<A2UISpec[]>([])   // 当前流式轮次的实时面板
const streamingUserQuery = ref('')                // 当前流式轮次的问题（画布分组标题）
const messagesContainer = ref<HTMLElement>()
const canvasContainer = ref<HTMLElement>()
const textareaRef = ref<HTMLTextAreaElement>()
const composerFocused = ref(false)
let streamAbort: AbortController | null = null

// ===== 数据源选择器 =====
const datasources = ref<Datasource[]>([])
/** 当前选中的数据源 id；空字符串表示"当前库" */
const selectedDatasourceId = ref<string>('')
const dsSelectOpen = ref(false)
const dsTriggerRef = ref<HTMLButtonElement | null>(null)
const dsDropdownPos = ref({ top: '0px', left: '0px' })

const datasourceOptions = computed(() => {
  const opts = [{ id: '', name: '当前库' }]
  datasources.value.forEach(ds => opts.push({ id: ds.id, name: ds.name }))
  return opts
})

const selectedDatasourceName = computed(() => {
  if (!selectedDatasourceId.value) return '当前库'
  return datasources.value.find(d => d.id === selectedDatasourceId.value)?.name ?? '当前库'
})

async function loadDatasources() {
  try {
    const res = await datasourceApi.list({ page: 1, page_size: 200 })
    datasources.value = res.data?.items ?? []
  } catch {
    // 非关键路径，静默忽略
  }
}

function openDsDropdown() {
  if (dsTriggerRef.value) {
    const rect = dsTriggerRef.value.getBoundingClientRect()
    dsDropdownPos.value = {
      top: `${rect.bottom + 4}px`,
      left: `${rect.left}px`
    }
  }
  dsSelectOpen.value = !dsSelectOpen.value
}

function selectDatasource(id: string) {
  selectedDatasourceId.value = id
  dsSelectOpen.value = false
}

// ===== 结果画布分组：历史轮次 + 实时轮次 =====
interface CanvasGroup {
  key: string
  query: string
  surfaces: A2UISpec[]
  live: boolean
}
const canvasGroups = computed<CanvasGroup[]>(() => {
  const groups: CanvasGroup[] = []
  for (let i = 0; i < messages.value.length; i++) {
    const m = messages.value[i]
    if (m.role === 'assistant' && m.ui_surfaces && m.ui_surfaces.length) {
      // 取紧邻的上一条用户消息作为分组标题
      let q = ''
      for (let j = i - 1; j >= 0; j--) {
        if (messages.value[j].role === 'user') { q = messages.value[j].content; break }
      }
      groups.push({ key: m.id, query: q, surfaces: m.ui_surfaces, live: false })
    }
  }
  if (isStreaming.value && (streamingUISurfaces.value.length || streamingUserQuery.value)) {
    groups.push({ key: 'live', query: streamingUserQuery.value, surfaces: streamingUISurfaces.value, live: true })
  }
  return groups
})
const totalSurfaces = computed(() => canvasGroups.value.reduce((n, g) => n + g.surfaces.length, 0))

// ===== 持久化解析 =====
// 思考步骤（历史时间线）
function parseAgentSteps(raw?: string): AgentStep[] {
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

// UI 面板（会话重开重现画布；数据来自持久化的有界快照，独立于 Result Store TTL）
function parseUISurfaces(raw?: string): A2UISpec[] {
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    const list = parsed?.ui_surfaces
    if (!Array.isArray(list)) return []
    return list.filter((s: any) => s && Array.isArray(s.components) && s.components.length)
  } catch {
    return []
  }
}

// ===== A2UI 交互：confirm/cancel 走危险操作确认端点（Phase 5 confirm-resume）=====
// 其余动作（paginate 等）由 A2UIRenderer 内部消化，不冒泡到这里。
async function onA2UIAction(action: A2UIAction) {
  if (action.name === 'cancel') {
    toast.info('已取消，该操作将自动过期')
    return
  }
  if (action.name !== 'confirm') return
  const { pending_action_id, token, session_id } = action.params || {}
  if (!pending_action_id || !token) {
    toast.error('确认参数缺失，无法执行')
    return
  }
  try {
    const res = await agentApi.confirmOperation({
      pending_action_id,
      token,
      session_id: session_id || currentSessionId.value
    })
    if (res.status === 'expired') {
      toast.warning(res.message || '待确认操作已过期，请重新发起')
      return
    }
    toast.success(res.duplicate
      ? `操作已执行过（幂等命中），影响 ${res.rows_affected ?? 0} 行`
      : `操作已执行：${res.target || ''} 影响 ${res.rows_affected ?? 0} 行`)
  } catch (e: any) {
    toast.error(e?.message || '确认执行失败')
  }
}

// 合并流式步骤事件：tool_call 新建，tool_result 更新同一步
function upsertStep(target: AgentStep[], event: any) {
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

// 把当前流式轮次落成一条 assistant 历史消息（快照 steps / ui_surfaces）。
// 成功收流与异常/中断分支共用，确保任何情况下已产出的内容都不会「消失」。
function commitAssistantMessage(content: string) {
  messages.value.push({
    id: (Date.now() + 1).toString(),
    session_id: currentSessionId.value,
    role: 'assistant',
    content,
    token_count: Math.ceil(content.length / 3),
    steps: streamingSteps.value.length ? [...streamingSteps.value] : undefined,
    ui_surfaces: streamingUISurfaces.value.length ? [...streamingUISurfaces.value] : undefined,
    created_at: new Date().toISOString()
  } as DataMessage)
}

// ===== 发送 =====
async function sendMessage() {
  const content = inputMessage.value.trim()
  if (!content || isStreaming.value) return

  const userMsg: DataMessage = {
    id: Date.now().toString(),
    session_id: currentSessionId.value,
    role: 'user',
    content,
    token_count: Math.ceil(content.length / 3),
    created_at: new Date().toISOString()
  }
  messages.value.push(userMsg)

  inputMessage.value = ''
  adjustTextareaHeight()
  scrollThreadToBottom()

  isStreaming.value = true
  streamingContent.value = ''
  streamingSteps.value = []
  streamingUISurfaces.value = []
  streamingUserQuery.value = content

  let assistantContent = ''
  let newSessionId = ''

  streamAbort?.abort()
  streamAbort = new AbortController()

  try {
    // 只携 query + session_id + datasource_id（可选）：不选智能体、不选知识库
    for await (const event of agentApi.streamDataChat({
      query: content,
      session_id: currentSessionId.value || undefined,
      datasource_id: selectedDatasourceId.value || undefined
    }, { signal: streamAbort.signal })) {
      if (event.session_id && event.session_id !== currentSessionId.value) {
        newSessionId = event.session_id
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
          assistantContent = event.answer
          streamingContent.value = event.answer
        } else if (event.content) {
          assistantContent = event.content
          streamingContent.value = event.content
        }
      } else if (event.event === 'ui') {
        // 生成式 UI：render_ui / sql_mutate 每次成功即推一个独立 surface（实时）。
        // SSE 解析器把 spec 字段直接铺在 event 上（并附加 event:'ui'）。
        const spec = event as unknown as A2UISpec
        if (Array.isArray(spec.components) && spec.components.length) {
          const idx = spec.surface
            ? streamingUISurfaces.value.findIndex(s => s.surface === spec.surface)
            : -1
          if (idx >= 0) streamingUISurfaces.value.splice(idx, 1, spec)  // 同 surface 覆盖（幂等）
          else streamingUISurfaces.value.push(spec)                     // 新 surface 追加
          scrollCanvasToBottom()
        }
      } else if (event.event === 'error') {
        toast.error(event.error || event.content || '查询失败')
      }

      scrollThreadToBottom()
    }

    commitAssistantMessage(assistantContent || '抱歉，没有收到回复内容，请稍后重试。')

    if (newSessionId) await loadSessions()
  } catch (err: any) {
    if (err?.name === 'AbortError') {
      // 主动中断（组件卸载/新一轮发送）：保留已流式产出的部分，避免整块丢失
      if (assistantContent || streamingContent.value || streamingSteps.value.length || streamingUISurfaces.value.length) {
        commitAssistantMessage((assistantContent || streamingContent.value || '').trim() || '（生成已中断）')
      }
    } else {
      // 连接中断/服务异常：不能让已经流式呈现的思考步骤与内容凭空「消失」，
      // 把已产出的部分连同错误提示落成一条 assistant 消息保留下来。
      console.error('发送消息失败:', err)
      toast.error(err?.message || '发送消息失败')
      const partial = (assistantContent || streamingContent.value || '').trim()
      commitAssistantMessage(
        (partial ? partial + '\n\n' : '') +
        `> ⚠️ 生成中断：${err?.message || '与数据智能体的连接已断开，请重试。'}`
      )
    }
  } finally {
    isStreaming.value = false
    streamingContent.value = ''
    streamingSteps.value = []
    streamingUISurfaces.value = []
    streamingUserQuery.value = ''
    nextTick(scrollThreadToBottom)
  }
}

// ===== 工具函数 =====
function scrollThreadToBottom() {
  nextTick(() => {
    if (messagesContainer.value) messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
  })
}
function scrollCanvasToBottom() {
  nextTick(() => {
    if (canvasContainer.value) canvasContainer.value.scrollTop = canvasContainer.value.scrollHeight
  })
}
function adjustTextareaHeight() {
  nextTick(() => {
    if (textareaRef.value) {
      textareaRef.value.style.height = 'auto'
      textareaRef.value.style.height = Math.min(textareaRef.value.scrollHeight, 180) + 'px'
    }
  })
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

// ===== 生命周期 =====
function closeDsDropdown() {
  dsSelectOpen.value = false
}

onMounted(() => {
  loadSessions()
  loadDatasources()
  nextTick(() => textareaRef.value?.focus())
  document.addEventListener('click', closeDsDropdown)
})
onUnmounted(() => {
  streamAbort?.abort()
  document.removeEventListener('click', closeDsDropdown)
})
</script>

<style scoped>
/* ====================================================
   DaDa 数据智能体 —— 月夜台账样式
   渲染在 platform/index.vue 的 .content-wrapper 内（padding: 24px），
   抵消父层 padding 撑满面板。三栏：会话 | 对话 | 画布
   ==================================================== */
.dada {
  display: flex;
  gap: 0;
  height: calc(100% + 48px);
  margin: -24px;
  overflow: hidden;
}

/* ===================== 会话侧栏 ===================== */
.sessions {
  width: 236px;
  flex: 0 0 236px;
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--color-border-subtle);
  overflow: hidden;
}
.sessions__head { padding: 14px 14px 10px; }
.btn-new {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  padding: 10px;
  border: 0;
  border-radius: var(--radius-sm);
  cursor: pointer;
  background: var(--primary);
  color: var(--on-primary);
  font-size: 13px;
  font-weight: 600;
  transition: background var(--duration-fast);
}
.btn-new:hover { background: var(--primary-light); }
.btn-new svg { width: 14px; height: 14px; }

.sessions__search {
  margin: 2px 14px 8px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 11px;
  border-radius: var(--radius-sm);
  background: var(--bg-primary);
}
.sessions__search svg { width: 13px; height: 13px; color: var(--text-muted); flex: 0 0 13px; }
.sessions__search-input {
  flex: 1;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--text-primary);
  font-size: 12.5px;
  font-family: inherit;
}
.sessions__search-input::placeholder { color: var(--text-muted); }

.sessions__group {
  padding: 10px 16px 6px;
  font-size: 10.5px;
  color: var(--text-muted);
  font-family: var(--font-mono);
  letter-spacing: 1.5px;
}

.sessions__list { flex: 1; overflow-y: auto; padding: 0 8px 8px; }
.session {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 9px 10px;
  margin-bottom: 2px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background var(--duration-fast);
}
.session:hover { background: var(--bg-tertiary); }
.session--active { background: var(--bg-elevated); }
.session--active .session__title { color: var(--primary); }
.session__icon {
  width: 30px;
  height: 30px;
  flex: 0 0 30px;
  border-radius: var(--radius-sm);
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-tertiary);
  color: var(--text-muted);
}
.session--active .session__icon { background: var(--primary-dim); color: var(--primary); }
.session__icon svg { width: 15px; height: 15px; }
.session__body { flex: 1; min-width: 0; }
.session__title {
  font-size: 13px;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.session__meta {
  display: flex;
  gap: 7px;
  align-items: center;
  margin-top: 3px;
  font-size: 11px;
  color: var(--text-muted);
}
.session__chip {
  font-family: var(--font-mono);
  font-size: 10px;
  padding: 0 6px;
  border-radius: 3px;
  border: 1px solid var(--color-border-subtle);
  color: var(--text-muted);
}
.session--active .session__chip { border-color: rgba(156, 180, 205, 0.4); color: var(--primary); }
.session__delete {
  width: 22px;
  height: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: 0;
  color: var(--text-muted);
  cursor: pointer;
  border-radius: var(--radius-sm);
  opacity: 0;
  transition: opacity var(--duration-fast), color var(--duration-fast);
}
.session:hover .session__delete { opacity: 1; }
.session__delete:hover { color: var(--danger); }
.session__delete svg { width: 13px; height: 13px; }

.sessions__empty { padding: 20px 16px; text-align: center; font-size: 12px; color: var(--text-muted); }
.sessions__loading { display: flex; justify-content: center; padding: 20px; }
.spinner {
  width: 20px;
  height: 20px;
  border: 2px solid var(--color-border-default);
  border-top-color: var(--primary);
  border-radius: 50%;
  animation: spin var(--duration-slow) linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }

/* ===================== 对话线 ===================== */
.thread {
  flex: 0 0 448px;
  min-width: 0;
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--color-border-subtle);
  overflow: hidden;
}
.topbar {
  display: flex;
  align-items: center;
  gap: 18px;
  padding: 0 20px;
  height: 54px;
  border-bottom: 1px solid var(--color-border-subtle);
  flex-shrink: 0;
}
.topbar__title {
  font-family: var(--font-display);
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary);
}
.status-dot {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: var(--text-muted);
  font-family: var(--font-mono);
}
.status-dot i {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--success);
  animation: pulse 3s ease-in-out infinite;
  flex-shrink: 0;
}
@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.4; } }

.messages { flex: 1; overflow-y: auto; padding: 22px 0; }
.messages__inner { padding: 0 20px; }

/* 欢迎态 */
.welcome {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 320px;
  text-align: center;
}
.welcome__icon {
  width: 56px;
  height: 56px;
  border-radius: var(--radius-md);
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--primary-dim);
  color: var(--primary);
  margin-bottom: 18px;
}
.welcome__icon svg { width: 28px; height: 28px; }
.welcome__title {
  font-family: var(--font-display);
  font-size: 19px;
  font-weight: 700;
  color: var(--text-primary);
  margin-bottom: 8px;
}
.welcome__desc { font-size: 13px; color: var(--text-muted); margin-bottom: 22px; line-height: 1.7; }
.welcome__suggest { display: flex; flex-wrap: wrap; gap: 7px; justify-content: center; }
.suggest-chip {
  display: inline-flex;
  align-items: center;
  padding: 5px 12px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  border: 1px solid var(--color-border-subtle);
  color: var(--text-muted);
  font-size: 12px;
  transition: color var(--duration-fast), border-color var(--duration-fast);
}
.suggest-chip:hover { color: var(--text-secondary); border-color: var(--color-border-default); }

/* 消息 */
.msg { display: flex; gap: 12px; margin-bottom: 26px; }
.msg--user { flex-direction: row-reverse; }
.avatar {
  width: 30px;
  height: 30px;
  flex: 0 0 30px;
  border-radius: var(--radius-sm);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 600;
}
.avatar--user { background: var(--primary); color: var(--on-primary); }
.avatar--ai {
  background: var(--bg-elevated);
  border: 1px solid var(--color-border-subtle);
  color: var(--primary-dark);
}
.avatar--ai svg { width: 16px; height: 16px; }
.msg__col { max-width: 82%; min-width: 0; }
.msg--user .msg__col { display: flex; flex-direction: column; align-items: flex-end; }
.bubble {
  padding: 11px 15px;
  border-radius: var(--radius-md);
  font-size: 14px;
  line-height: 1.75;
  word-wrap: break-word;
}
.bubble--user { background: var(--bg-elevated); color: var(--text-primary); border-top-right-radius: 3px; }
.bubble--ai { background: var(--bg-tertiary); color: var(--text-primary); border-top-left-radius: 3px; }
.bubble--ai :deep(p) { margin: 4px 0; }
.bubble--ai :deep(p:first-child) { margin-top: 0; }
.bubble--ai :deep(p:last-child) { margin-bottom: 0; }
.bubble--ai :deep(b), .bubble--ai :deep(strong) { color: var(--primary); font-weight: 600; }
.bubble--ai :deep(ul), .bubble--ai :deep(ol) { margin: 6px 0 2px; padding-left: 18px; }
.bubble--ai :deep(li) { margin: 4px 0; }
.bubble--ai :deep(code) {
  font-family: var(--font-mono);
  font-size: 12px;
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--bg-primary);
  color: var(--primary-dark);
}
.bubble--ai :deep(pre) {
  background: var(--bg-primary);
  padding: 12px;
  border-radius: var(--radius-sm);
  overflow-x: auto;
  margin: 8px 0;
}
.bubble--ai :deep(pre code) { background: transparent; padding: 0; }

/* 画布关联提示 */
.canvas-ref {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
  font-size: 11.5px;
  color: var(--primary);
  font-family: var(--font-mono);
}
.canvas-ref svg { width: 13px; height: 13px; }

.msg__time { margin-top: 7px; font-size: 10.5px; color: var(--text-muted); font-family: var(--font-mono); }

/* 流式光标 */
.streaming-cursor { display: flex; gap: 4px; align-items: center; padding: 8px 2px; }
.streaming-cursor span {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--primary);
  animation: pulse 1.2s ease-in-out infinite;
}
.streaming-cursor span:nth-child(2) { animation-delay: 0.2s; }
.streaming-cursor span:nth-child(3) { animation-delay: 0.4s; }

/* 输入区 */
.composer-wrap { padding: 14px 20px 18px; border-top: 1px solid var(--color-border-subtle); flex-shrink: 0; }
.composer {
  border-radius: var(--radius-md);
  padding: 12px 14px;
  background: var(--bg-primary);
  border: 1px solid var(--color-border-subtle);
  transition: border-color var(--duration-fast);
}
.composer--focus { border-color: rgba(156, 180, 205, 0.5); }
.composer__textarea {
  width: 100%;
  border: 0;
  outline: 0;
  resize: none;
  background: transparent;
  color: var(--text-primary);
  font-size: 14px;
  line-height: 1.6;
  font-family: inherit;
  min-height: 42px;
  max-height: 180px;
}
.composer__textarea::placeholder { color: var(--text-muted); }
.composer__foot { display: flex; align-items: center; gap: 8px; margin-top: 8px; }
.scope-hint {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  color: var(--primary);
  padding: 3px 9px;
  border-radius: 3px;
  border: 1px solid rgba(156, 180, 205, 0.35);
}
.scope-hint svg { width: 12px; height: 12px; }
.composer__count { margin-left: auto; font-family: var(--font-mono); font-size: 10.5px; color: var(--text-muted); }
.btn-send {
  width: 34px;
  height: 34px;
  border: 0;
  border-radius: var(--radius-sm);
  cursor: pointer;
  background: var(--primary);
  color: var(--on-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background var(--duration-fast);
  flex-shrink: 0;
}
.btn-send:hover:not(:disabled) { background: var(--primary-light); }
.btn-send:disabled { background: var(--bg-elevated); color: var(--text-muted); cursor: not-allowed; }
.btn-send svg { width: 15px; height: 15px; }

/* ===================== 结果画布 ===================== */
.canvas {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  /* 补齐 A2UIRenderer 依赖但暗色主题未定义的 --color-* 令牌，
     使生成式 UI 卡片继承月夜暗色，而非回退到浅色默认值 */
  --color-bg-container: var(--bg-secondary);
  --color-border: var(--border-subtle);
  --color-fill-secondary: var(--bg-tertiary);
  --color-text-tertiary: var(--text-muted);
}
.canvas__bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 20px;
  height: 54px;
  border-bottom: 1px solid var(--color-border-subtle);
  flex-shrink: 0;
}
.canvas__title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-family: var(--font-display);
  font-size: 15px;
  font-weight: 700;
  color: var(--text-primary);
}
.canvas__title svg { width: 16px; height: 16px; color: var(--primary); }
.canvas__live {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: var(--primary);
  font-family: var(--font-mono);
}
.canvas__live i {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--primary);
  animation: pulse 1.2s ease-in-out infinite;
}
.canvas__count { margin-left: auto; font-size: 11px; color: var(--text-muted); font-family: var(--font-mono); }

.canvas__body { flex: 1; overflow-y: auto; padding: 20px 22px; }

/* 画布空态 */
.canvas-empty {
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  padding: 40px;
}
.canvas-empty__icon {
  width: 60px;
  height: 60px;
  border-radius: var(--radius-lg);
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-tertiary);
  color: var(--text-muted);
  margin-bottom: 18px;
}
.canvas-empty__icon svg { width: 30px; height: 30px; }
.canvas-empty__title { font-size: 14px; color: var(--text-secondary); margin-bottom: 8px; }
.canvas-empty__desc { font-size: 12px; color: var(--text-muted); max-width: 320px; line-height: 1.7; }

/* 画布分组 */
.canvas-group { margin-bottom: 26px; }
.canvas-group:last-child { margin-bottom: 0; }
.canvas-group__caption {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
  padding-bottom: 8px;
  border-bottom: 1px dashed var(--color-border-subtle);
}
.canvas-group__q {
  font-size: 12.5px;
  color: var(--text-secondary);
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.canvas-group__live {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 10.5px;
  color: var(--primary);
  font-family: var(--font-mono);
  flex-shrink: 0;
}
.canvas-group__live i {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--primary);
  animation: pulse 1.2s ease-in-out infinite;
}
.canvas-group__pending {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px;
  font-size: 12.5px;
  color: var(--text-muted);
  border: 1px dashed var(--color-border-subtle);
  border-radius: var(--radius-md);
}

/* ===================== 数据源选择器 ===================== */
.ds-selector {
  position: relative;
  flex-shrink: 0;
}

.ds-selector__trigger {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 9px;
  font-size: 11px;
  font-family: var(--font-mono);
  color: var(--text-muted);
  background: transparent;
  border: 1px solid var(--color-border-subtle);
  border-radius: 3px;
  cursor: pointer;
  transition: color var(--duration-fast), border-color var(--duration-fast);
  white-space: nowrap;
}

.ds-selector__trigger svg:first-child {
  width: 12px;
  height: 12px;
}

.ds-selector__trigger:hover {
  color: var(--text-secondary);
  border-color: var(--color-border-default);
}

.ds-selector__trigger--active {
  color: var(--primary);
  border-color: rgba(34, 211, 238, 0.35);
}

.ds-selector__caret {
  width: 11px;
  height: 11px;
  transition: transform var(--duration-fast);
}

.ds-selector__caret.open {
  transform: rotate(180deg);
}

.ds-selector__dropdown {
  position: fixed;
  min-width: 160px;
  max-height: 220px;
  overflow-y: auto;
  background: var(--bg-secondary);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-xl);
  z-index: 9999;
  padding: 4px;
}

.ds-selector__option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 7px 10px;
  font-size: 13px;
  color: var(--text-secondary);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background var(--duration-fast);
}

.ds-selector__option:hover {
  background: var(--bg-elevated);
  color: var(--text-primary);
}

.ds-selector__option.selected {
  color: var(--primary);
  background: var(--primary-dim);
}

.ds-selector__check {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
}
</style>
