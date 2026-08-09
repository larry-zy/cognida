<template>
  <div class="kb-assistant">

    <!-- 左侧边栏 -->
    <aside class="sidebar">
      <!-- 新对话按钮 -->
      <div class="sidebar__head">
        <button class="btn-new" @click="createNewSession">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
            <path d="M12 5v14M5 12h14"/>
          </svg>
          新对话
        </button>
      </div>

      <!-- 搜索框 -->
      <div class="sidebar__search">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="11" cy="11" r="7"/>
          <path d="M21 21l-4.3-4.3"/>
        </svg>
        <input
          v-model="searchQuery"
          type="text"
          placeholder="搜索对话记录…"
          class="sidebar__search-input"
        />
      </div>

      <!-- 知识库分组（多选：不选=检索全部已启用知识库） -->
      <div class="sidebar__group">
        知识库 KB
        <span class="sidebar__group-hint">{{ selectedKbIds.length ? `已选 ${selectedKbIds.length}` : '全部' }}</span>
      </div>

      <!-- 选库模式切换：手动 / 结合 / 智能（自研分段控件） -->
      <div class="kb-mode">
        <button
          v-for="opt in kbModeOptions"
          :key="opt.value"
          class="kb-mode__seg"
          :class="{ 'kb-mode__seg--on': kbScopeMode === opt.value }"
          :title="opt.hint"
          @click="kbScopeMode = opt.value"
        >{{ opt.label }}</button>
      </div>
      <div class="kb-mode__cap">{{ kbModeCaption }}</div>

      <!-- 知识库列表（多选复选，智能模式下置灰仅作提示） -->
      <div v-if="kbLoading" class="sidebar__kb-skeleton">
        <div class="skeleton-line" style="width: 70%"></div>
        <div class="skeleton-line" style="width: 50%"></div>
      </div>
      <template v-else>
        <div
          v-for="kb in knowledgeBases"
          :key="kb.id"
          class="kb-row"
          :class="{ 'kb-row--active': isKbSelected(kb.id), 'kb-row--muted': isAutoMode }"
          @click="toggleKb(kb.id)"
        >
          <!-- 自研复选框 -->
          <span class="kb-check" :class="{ 'kb-check--on': isKbSelected(kb.id) }">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
              <path d="M5 12l5 5 9-11"/>
            </svg>
          </span>
          <div class="kb-row__body">
            <div class="kb-row__name">{{ kb.name }}</div>
            <div v-if="isKbSelected(kb.id)" class="kb-row__meta">
              {{ kb.document_count ?? 0 }} 篇
              <span v-if="kb.chunk_count"> / {{ formatChunkCount(kb.chunk_count) }} 向量</span>
            </div>
          </div>
        </div>
        <div v-if="knowledgeBases.length === 0" class="sidebar__empty">
          <span>暂无知识库</span>
        </div>
        <!-- 未选择时的提示：随模式变化 -->
        <div v-else class="sidebar__kb-hint">
          <template v-if="isAutoMode">智能模式 · 勾选忽略，AI 从全部已启用库自选</template>
          <template v-else-if="kbScopeMode === 'hybrid'">结合模式 · 勾选为候选池，AI 在池内自选</template>
          <template v-else>未选择 = 检索全部已启用知识库</template>
        </div>
      </template>

      <!-- 会话分组 -->
      <div class="sidebar__group">会话记录</div>

      <!-- 会话列表 -->
      <div class="sidebar__sessions">
        <div v-if="loadingSessions" class="sidebar__loading">
          <div class="spinner"></div>
        </div>

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
                <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20V2H6.5A2.5 2.5 0 0 0 4 4.5z"/>
                <path d="M4 19.5A2.5 2.5 0 0 0 6.5 22H20v-5"/>
              </svg>
            </div>
            <div class="session__body">
              <div class="session__title">{{ session.title || '新对话' }}</div>
              <div class="session__meta">
                <span class="session__chip">知识库</span>
                <span>{{ formatSessionTime(session.created_at) }}</span>
              </div>
            </div>
            <button
              class="session__delete"
              title="删除"
              @click.stop="deleteSession(session.id)"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
              </svg>
            </button>
          </div>

          <div v-if="!loadingSessions && filteredSessions.length === 0" class="sidebar__empty">
            <span>暂无对话记录</span>
          </div>
        </template>
      </div>

      <!-- 侧栏底部：知识库范围概览（真实数据，不虚构指标）
           未选择时聚合全部已启用知识库，选择后聚合所选 -->
      <div v-if="scopeKbs.length > 0" class="side-stats">
        <div class="side-stats__row"><span>{{ selectedKbIds.length ? `已选 ${scopeKbs.length} 个库` : '全部知识库' }}</span><b>{{ scopeDocCount }} 篇</b></div>
        <div class="side-stats__row"><span>向量索引</span><b>{{ formatChunkCount(scopeChunkCount) }}</b></div>
        <div class="side-stats__bar"><i :style="{ width: indexHealthPct + '%' }"></i></div>
        <div class="side-stats__cap">{{ indexHealthCap }}</div>
      </div>
    </aside>

    <!-- 主区 -->
    <main class="main">

      <!-- 顶栏 -->
      <div class="topbar">
        <span class="topbar__title">知识库助手</span>
        <div class="topbar__right">
          <span class="status-dot">
            <i></i>rag · {{ isStreaming ? 'streaming' : 'online' }}
          </span>
          <button class="icon-btn" title="清空对话" @click="clearCurrentSession">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
            </svg>
          </button>
        </div>
      </div>

      <!-- 消息区域 -->
      <div class="messages" ref="messagesContainer">
        <div class="messages__inner">

          <!-- 欢迎空状态 -->
          <div
            v-if="messages.length === 0 && !streamingContent && streamingSteps.length === 0"
            class="welcome"
          >
            <div class="welcome__icon">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20V2H6.5A2.5 2.5 0 0 0 4 4.5z"/>
                <path d="M4 19.5A2.5 2.5 0 0 0 6.5 22H20v-5"/>
              </svg>
            </div>
            <h2 class="welcome__title">知识库助手</h2>
            <p class="welcome__desc">
              {{ scopeDesc }}
            </p>
            <div class="welcome__tips">
              <div class="welcome__tip">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M9 12l2 2 4-4"/>
                  <circle cx="12" cy="12" r="9"/>
                </svg>
                <span>多轮对话，上下文连贯</span>
              </div>
              <div class="welcome__tip">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M9 12l2 2 4-4"/>
                  <circle cx="12" cy="12" r="9"/>
                </svg>
                <span>实时检索相关文档片段</span>
              </div>
              <div class="welcome__tip">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M9 12l2 2 4-4"/>
                  <circle cx="12" cy="12" r="9"/>
                </svg>
                <span>引用来源可追溯</span>
              </div>
            </div>
            <!-- 预制起始问题 -->
            <div class="suggest-row welcome__suggest">
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
            <!-- 用户消息 -->
            <div v-if="message.role === 'user'" class="msg msg--user">
              <div class="avatar avatar--user">{{ userInitial }}</div>
              <div class="msg__col">
                <div class="bubble bubble--user">{{ message.content }}</div>
                <div class="msg__time">{{ formatTime(message.created_at) }}</div>
              </div>
            </div>

            <!-- 助手消息 -->
            <div v-else-if="message.role === 'assistant'" class="msg msg--ai">
              <div class="avatar avatar--ai">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                  <path d="M21 12.8A9 9 0 1 1 11.2 3 7 7 0 0 0 21 12.8z"/>
                </svg>
              </div>
              <div class="msg__col">
                <!-- 检索时间线（仅当存在步骤数据时渲染） -->
                <AgentTimeline
                  v-if="message.steps && message.steps.length > 0"
                  :steps="message.steps"
                  :default-open="false"
                />
                <!-- 回答气泡 -->
                <div class="bubble bubble--ai" v-html="renderMarkdown(message.content)"></div>
                <!-- 引用来源（来自 ragContext，真实检索来源） -->
                <div v-if="message.ragContext && message.ragContext.contexts_with_score && message.ragContext.contexts_with_score.length > 0" class="sources">
                  <div class="sources__title">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M10 13a5 5 0 0 0 7.5.5l3-3a5 5 0 0 0-7-7l-1.7 1.7"/>
                      <path d="M14 11a5 5 0 0 0-7.5-.5l-3 3a5 5 0 0 0 7 7l1.7-1.7"/>
                    </svg>
                    REFERENCES · {{ message.ragContext.contexts_with_score.length }}
                  </div>
                  <div
                    v-for="(src, idx) in message.ragContext.contexts_with_score"
                    :key="idx"
                    class="src"
                  >
                    <div class="src__icon">
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
                        <path d="M14 2v6h6"/>
                      </svg>
                    </div>
                    <div class="src__body">
                      <div class="src__name">{{ src.source || `片段 ${idx + 1}` }}</div>
                      <div class="src__snip">{{ src.content?.slice(0, 80) }}…</div>
                    </div>
                    <div class="src__side">
                      <div class="src__bar">
                        <i :style="{ width: Math.round(src.score * 100) + '%' }"></i>
                      </div>
                      <span class="src__pct">.{{ Math.round(src.score * 100) }}</span>
                    </div>
                  </div>
                </div>
                <!-- 推荐追问（预制，仅最后一条回答显示） -->
                <div
                  v-if="message.id === lastAssistantId && !isStreaming"
                  class="suggest-row"
                >
                  <span
                    v-for="q in followupQuestions"
                    :key="q"
                    class="suggest-chip"
                    @click="applySuggestion(q)"
                  >{{ q }}</span>
                </div>
                <div class="msg__time">{{ formatAnswerMeta(message) }}</div>
              </div>
            </div>
          </template>

          <!-- 流式消息 -->
          <div v-if="streamingContent || streamingSteps.length > 0" class="msg msg--ai">
            <div class="avatar avatar--ai">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                <path d="M21 12.8A9 9 0 1 1 11.2 3 7 7 0 0 0 21 12.8z"/>
              </svg>
            </div>
            <div class="msg__col">
              <!-- 实时时间线 -->
              <AgentTimeline
                v-if="streamingSteps.length > 0"
                :steps="streamingSteps"
                :active="true"
              />
              <div v-if="streamingContent" class="bubble bubble--ai" v-html="renderMarkdown(streamingContent)"></div>
              <div v-else class="streaming-cursor">
                <span></span><span></span><span></span>
              </div>
            </div>
          </div>

        </div>
      </div>

      <!-- 输入区 -->
      <div class="composer-wrap">
        <div class="composer" :class="{ 'composer--focus': composerFocused }">
          <textarea
            v-model="inputMessage"
            ref="textareaRef"
            placeholder="就当前知识库提问…"
            class="composer__textarea"
            rows="1"
            :disabled="isStreaming"
            @keydown.enter.exact="onEnterKey"
            @input="adjustTextareaHeight"
            @focus="composerFocused = true"
            @blur="composerFocused = false"
          ></textarea>
          <div class="composer__foot">
            <!-- 知识库范围 chip：未选择=全部，已选=数量/名称 -->
            <span class="tool-chip">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20V2H6.5A2.5 2.5 0 0 0 4 4.5z"/>
                <path d="M4 19.5A2.5 2.5 0 0 0 6.5 22H20v-5"/>
              </svg>
              {{ scopeChipLabel }}
            </span>
            <!-- 图谱增强开关：开启后检索结合知识图谱做关系推理与多跳 -->
            <span
              class="tool-chip"
              :class="{ 'tool-chip--active': graphEnhanced }"
              title="开启后检索会调用 graph_query 补充实体关系，并启用多跳推理"
              @click="graphEnhanced = !graphEnhanced"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="6" cy="12" r="2.5"/>
                <circle cx="18" cy="6" r="2.5"/>
                <circle cx="18" cy="18" r="2.5"/>
                <line x1="8.2" y1="11" x2="15.8" y2="7"/>
                <line x1="8.2" y1="13" x2="15.8" y2="17"/>
              </svg>
              图谱增强
            </span>
            <span class="composer__count">{{ inputMessage.length }} / 4000</span>
            <button
              class="btn-send"
              :disabled="!inputMessage.trim() || isStreaming"
              @click="sendMessage"
            >
              <svg v-if="!isStreaming" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
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
import { ref, computed, onMounted, nextTick } from 'vue'
import { storeToRefs } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import { useKnowledgeStore } from '@/stores/knowledge'
import { agentApi } from '@/api/agent'
import type { Message, AgentStep, KnowledgeBase, RAGContext } from '@/types'
import { formatTime } from '@/utils'
import toast from '@/utils/toast'
import AgentTimeline from '@/components/agent/AgentTimeline.vue'
import { useMarkdown } from '@/composables/useMarkdown'
import { useAgentChat, parseAgentSteps } from '@/composables/useAgentChat'
import { useChatSessions } from '@/composables/useChatSessions'

// ===== 类型扩展 =====
interface KBMessage extends Message {
  steps?: AgentStep[]
  ragContext?: RAGContext
}

// ===== Auth =====
const authStore = useAuthStore()
const userInitial = computed(() => authStore.username?.charAt(0).toUpperCase() || 'U')

// ===== Markdown（唯一渲染入口，含 DOMPurify 消毒）=====
const { renderMarkdown } = useMarkdown()

// ===== 对话核心状态（由 useChatSessions / useAgentChat 共享）=====
const currentSessionId = ref<string>('')
const messages = ref<KBMessage[]>([])
const inputMessage = ref('')
const textareaRef = ref<HTMLTextAreaElement>()
const messagesContainer = ref<HTMLElement>()

// ===== 知识库状态（多选）=====
// 知识库列表为跨视图共享域数据，统一走带缓存的 store（〔FE-10〕），不再本地各拉一份。
const knowledgeStore = useKnowledgeStore()
const { knowledgeBases, loading: kbLoading } = storeToRefs(knowledgeStore)
// 多选：空数组表示“检索全部已启用知识库”
const selectedKbIds = ref<string[]>([])

// ===== 选库模式：manual(手动) / hybrid(结合) / auto(智能) =====
type KbScopeMode = 'manual' | 'hybrid' | 'auto'
const kbScopeMode = ref<KbScopeMode>('manual')
const kbModeOptions: { value: KbScopeMode; label: string; hint: string }[] = [
  { value: 'manual', label: '手动', hint: '完全由你勾选决定，AI 不参与选库' },
  { value: 'hybrid', label: '结合', hint: '勾选定义候选池，AI 在池内自选最相关的库' },
  { value: 'auto', label: '智能', hint: '忽略勾选，AI 从全部已启用库中自选' }
]
const isAutoMode = computed(() => kbScopeMode.value === 'auto')
const kbModeCaption = computed(() => {
  switch (kbScopeMode.value) {
    case 'hybrid': return '勾选为候选池，AI 在池内自选相关库检索'
    case 'auto': return '勾选仅作参考，AI 从全部已启用库自选'
    default: return '范围由你的勾选锁定，AI 不参与选库'
  }
})

function isKbSelected(id: string): boolean {
  return selectedKbIds.value.includes(id)
}

function toggleKb(id: string) {
  // 智能模式下勾选被忽略，禁止交互（仅作提示）
  if (isAutoMode.value) return
  const idx = selectedKbIds.value.indexOf(id)
  if (idx >= 0) {
    selectedKbIds.value.splice(idx, 1)
  } else {
    selectedKbIds.value.push(id)
  }
}

// 当前检索范围内的知识库：未选择时聚合全部，已选时聚合所选
const scopeKbs = computed<KnowledgeBase[]>(() =>
  selectedKbIds.value.length
    ? knowledgeBases.value.filter(kb => selectedKbIds.value.includes(kb.id))
    : knowledgeBases.value
)
const scopeDocCount = computed(() =>
  scopeKbs.value.reduce((sum, kb) => sum + (kb.document_count ?? 0), 0)
)
const scopeChunkCount = computed(() =>
  scopeKbs.value.reduce((sum, kb) => sum + (kb.chunk_count ?? 0), 0)
)
// 欢迎语文案
const scopeDesc = computed(() => {
  if (knowledgeBases.value.length === 0) return '暂无知识库'
  // 智能模式：勾选忽略，AI 从全部已启用库自选，不能宣称固定范围
  if (isAutoMode.value) return '智能模式 · AI 从全部已启用库自选'
  if (!selectedKbIds.value.length) return '未选择知识库 · 将检索全部已启用知识库'
  // 结合模式：勾选是候选池而非最终范围
  if (kbScopeMode.value === 'hybrid') return `候选池 ${scopeKbs.value.length} 个库 · AI 池内自选`
  if (scopeKbs.value.length === 1) return `当前知识库：${scopeKbs.value[0].name}`
  return `已选 ${scopeKbs.value.length} 个知识库`
})
// 输入区 chip 文案
const scopeChipLabel = computed(() => {
  if (isAutoMode.value) return '智能选库'
  if (!selectedKbIds.value.length) return '全部知识库'
  if (kbScopeMode.value === 'hybrid') return `${scopeKbs.value.length} 库候选`
  if (scopeKbs.value.length === 1) return scopeKbs.value[0].name
  return `${scopeKbs.value.length} 个知识库`
})

function formatChunkCount(n: number): string {
  if (n >= 1000) return (n / 1000).toFixed(1) + 'k'
  return n.toString()
}

// ===== 图谱增强开关 =====
const graphEnhanced = ref(false)

// ===== 预制推荐追问（用户已授权 "预制的就可以"）=====
const starterQuestions = ['这个知识库主要涵盖哪些内容？', '帮我总结最近入库的重点文档', '有哪些关键概念需要了解？']
const followupQuestions = ['能展开讲讲吗？', '有相关的文档来源吗？', '换个角度再解释一下']
function applySuggestion(text: string) {
  inputMessage.value = text
  nextTick(() => {
    textareaRef.value?.focus()
    adjustTextareaHeight()
  })
}

// ===== 侧栏索引概览（仅用真实字段，不虚构百分比）=====
const indexHealthPct = computed(() => (scopeChunkCount.value > 0 ? 100 : 0))
const indexHealthCap = computed(() => {
  const c = scopeChunkCount.value
  return c > 0 ? `向量索引正常 · 共 ${formatChunkCount(c)} 个片段` : '尚未建立向量索引'
})

// ===== 回答元信息：时间 + 引用统计 =====
function formatAnswerMeta(m: KBMessage): string {
  const t = formatTime(m.created_at)
  const n = m.ragContext?.contexts_with_score?.length ?? 0
  return n > 0 ? `${t} · 引用 ${n} 篇文档 · 已按相关度重排` : t
}

// ===== 消息状态 =====
// 最后一条助手消息 id —— 仅在其下方展示推荐追问
const lastAssistantId = computed(() => {
  for (let i = messages.value.length - 1; i >= 0; i--) {
    if (messages.value[i].role === 'assistant') return messages.value[i].id
  }
  return ''
})
// 本轮检索来源（rag_context 事件，落入助手消息）
const lastRagContext = ref<RAGContext>()

// ===== 会话侧栏 =====
const {
  searchQuery,
  loadingSessions,
  filteredSessions,
  loadSessions,
  createNewSession,
  switchSession,
  deleteSession,
  formatSessionTime
} = useChatSessions<KBMessage>({
  messages,
  currentSessionId,
  inputMessage,
  textareaRef,
  filter: s => s.agent_type === 'rag' || !s.agent_type || s.agent_type === 'default',
  mapMessage: msg => ({
    ...msg,
    created_at: typeof msg.created_at === 'number'
      ? new Date((msg.created_at as unknown as number) * 1000).toISOString()
      : msg.created_at,
    steps: parseAgentSteps(msg.agent_steps)
  } as KBMessage),
  searchTitleFallback: '新对话',
  deleteConfirmMessage: '确定要删除这个对话记录吗？',
  loadFailMessage: '加载对话失败',
  onAfterSwitch: () => scrollToBottom()
})

// ===== 对话发送 / 流式 =====
const {
  isStreaming,
  streamingContent,
  streamingSteps,
  composerFocused,
  sendMessage,
  onEnterKey,
  adjustTextareaHeight,
  scrollToBottom
} = useAgentChat<KBMessage>({
  messages,
  currentSessionId,
  inputMessage,
  textareaRef,
  messagesContainer,
  createStream: (content, signal) => agentApi.streamKnowledgeChat({
    // 结构化下发：原始 content + 知识库范围 + 图谱开关（不再用文本前缀 hack）
    query: content,
    session_id: currentSessionId.value || undefined,
    // 智能模式：忽略勾选，发空数组避免自相矛盾的载荷（后端本就按模式丢弃）
    kb_ids: isAutoMode.value ? [] : selectedKbIds.value,
    graph_enabled: graphEnhanced.value,
    kb_scope_mode: kbScopeMode.value
  }, { signal }),
  onSendStart: () => { lastRagContext.value = undefined },
  onEvent: event => {
    // RAG context 来自 rag_context 事件（StreamChatEvent 兼容字段）
    const rc = (event as any).rag_context
    if (rc) lastRagContext.value = rc as RAGContext
  },
  buildAssistantMessage: (content, { steps }) => ({
    id: (Date.now() + 1).toString(),
    session_id: currentSessionId.value,
    role: 'assistant',
    content,
    token_count: Math.ceil(content.length / 3),
    steps: steps.length ? [...steps] : undefined,
    ragContext: lastRagContext.value,
    created_at: new Date().toISOString()
  } as KBMessage),
  onNewSession: () => loadSessions()
})

function clearCurrentSession() {
  if (messages.value.length === 0) return
  messages.value = []
  toast.success('已清空当前对话')
}

// ===== 生命周期 =====
onMounted(() => {
  knowledgeStore.loadKnowledgeBases()
  loadSessions()
  nextTick(() => textareaRef.value?.focus())
})
</script>

<style scoped>
/* ====================================================
   知识库助手页面 — 月夜台账样式
   页面渲染在 platform/index.vue 的 .content-wrapper 内
   .content-wrapper { padding: 24px }
   所以这里不需要再套大卡片，用发丝线分区
   ==================================================== */

/* 需要抵消父层 padding 并撑满容器 */
.kb-assistant {
  display: flex;
  gap: 0;
  /* 抵消父层 .content-wrapper { padding: 24px }，精确撑满面板。
     底色/圆角/裁剪由壳层 .main-content 提供，这里不再叠一层半透明底 */
  height: calc(100% + 48px);
  margin: -24px;
  overflow: hidden;
}

/* ===================== 左侧边栏 ===================== */
.sidebar {
  width: 256px;
  flex: 0 0 256px;
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--color-border-subtle);
  overflow: hidden;
}

.sidebar__head {
  padding: 14px 14px 10px;
}

/* 新对话按钮：月光银青实心 */
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

.btn-new:hover {
  background: var(--primary-light);
}

.btn-new svg {
  width: 14px;
  height: 14px;
}

/* 搜索框 */
.sidebar__search {
  margin: 2px 14px 8px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 11px;
  border-radius: var(--radius-sm);
  background: var(--bg-primary);
}

.sidebar__search svg {
  width: 13px;
  height: 13px;
  color: var(--text-muted);
  flex: 0 0 13px;
}

.sidebar__search-input {
  flex: 1;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--text-primary);
  font-size: 12.5px;
  font-family: inherit;
}

.sidebar__search-input::placeholder {
  color: var(--text-muted);
}

/* 分组标签 */
.sidebar__group {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px 6px;
  font-size: 10.5px;
  color: var(--text-muted);
  font-family: var(--font-mono);
  letter-spacing: 1.5px;
}

/* 分组右侧计数提示（已选 N / 全部） */
.sidebar__group-hint {
  letter-spacing: 0.5px;
  color: var(--primary);
  opacity: 0.85;
}

/* 未选择时的范围提示 */
.sidebar__kb-hint {
  padding: 6px 16px 4px;
  font-size: 10.5px;
  color: var(--text-muted);
  line-height: 1.5;
}

/* 选库模式分段控件（自研，手动/结合/智能） */
.kb-mode {
  display: flex;
  gap: 2px;
  margin: 2px 14px 6px;
  padding: 2px;
  border-radius: var(--radius-sm);
  background: var(--bg-primary);
}

.kb-mode__seg {
  flex: 1;
  border: 0;
  background: transparent;
  color: var(--text-muted);
  font-size: 11.5px;
  font-family: inherit;
  padding: 5px 0;
  border-radius: 5px;
  cursor: pointer;
  transition: background var(--duration-fast), color var(--duration-fast);
}

.kb-mode__seg:hover {
  color: var(--text-secondary);
}

.kb-mode__seg--on {
  background: var(--primary);
  color: var(--on-primary);
  font-weight: 600;
}

.kb-mode__cap {
  padding: 0 16px 6px;
  font-size: 10.5px;
  color: var(--text-muted);
  line-height: 1.5;
}

/* 智能模式：知识库勾选置灰、禁用交互（仅作提示） */
.kb-row--muted {
  opacity: 0.45;
  pointer-events: none;
}

/* 知识库行（未选中） */
.kb-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 12px;
  margin: 0 8px 1px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background var(--duration-fast);
}

.kb-row:hover {
  background: var(--bg-tertiary);
}

/* 选中知识库：卡片样式，带上下结构 */
.kb-row--active {
  align-items: flex-start;
  padding: 10px 12px;
  background: var(--bg-tertiary);
}

/* 自研复选框：未选中为空框，选中填充主色并显示对勾 */
.kb-check {
  width: 15px;
  height: 15px;
  flex: 0 0 15px;
  border-radius: 4px;
  border: 1.5px solid var(--color-border-default);
  display: flex;
  align-items: center;
  justify-content: center;
  color: transparent;
  transition: background var(--duration-fast), border-color var(--duration-fast), color var(--duration-fast);
}

.kb-check svg {
  width: 10px;
  height: 10px;
}

.kb-check--on {
  background: var(--primary);
  border-color: var(--primary);
  color: var(--on-primary);
}

.kb-row--active .kb-check {
  margin-top: 1px;
}

.kb-row__body {
  flex: 1;
  min-width: 0;
}

.kb-row__name {
  font-size: 12.5px;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.kb-row--active .kb-row__name {
  color: var(--primary);
  font-weight: 600;
  font-size: 13px;
}

.kb-row__meta {
  margin-top: 4px;
  font-size: 10.5px;
  color: var(--text-muted);
  font-family: var(--font-mono);
}

/* 会话列表 */
.sidebar__sessions {
  flex: 1;
  overflow-y: auto;
  padding: 0 8px 8px;
}

.session {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 9px 10px;
  margin-bottom: 2px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background var(--duration-fast);
  position: relative;
}

.session:hover {
  background: var(--bg-tertiary);
}

.session--active {
  background: var(--bg-elevated);
}

.session--active .session__title {
  color: var(--primary);
}

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

.session--active .session__icon {
  background: var(--primary-dim);
  color: var(--primary);
}

.session__icon svg {
  width: 15px;
  height: 15px;
}

.session__body {
  flex: 1;
  min-width: 0;
}

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

.session--active .session__chip {
  border-color: rgba(156, 180, 205, 0.4);
  color: var(--primary);
}

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

.session:hover .session__delete {
  opacity: 1;
}

.session__delete:hover {
  color: var(--danger);
}

.session__delete svg {
  width: 13px;
  height: 13px;
}

/* 空状态 & 加载 */
.sidebar__empty {
  padding: 20px 16px;
  text-align: center;
  font-size: 12px;
  color: var(--text-muted);
}

.sidebar__loading {
  display: flex;
  justify-content: center;
  padding: 20px;
}

.sidebar__kb-skeleton {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 8px 20px;
}

.skeleton-line {
  height: 12px;
  border-radius: 4px;
  background: var(--bg-elevated);
}

.spinner {
  width: 20px;
  height: 20px;
  border: 2px solid var(--color-border-default);
  border-top-color: var(--primary);
  border-radius: 50%;
  animation: spin var(--duration-slow) linear infinite;
}

@keyframes spin { to { transform: rotate(360deg); } }

/* ===================== 主区 ===================== */
.main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* 顶栏 */
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

.topbar__right {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 10px;
}

/* 状态呼吸点 */
.status-dot {
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

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

/* 图标按钮 */
.icon-btn {
  width: 30px;
  height: 30px;
  border-radius: var(--radius-sm);
  border: 0;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: color var(--duration-fast), background var(--duration-fast);
}

.icon-btn:hover {
  color: var(--text-primary);
  background: var(--bg-elevated);
}

.icon-btn svg {
  width: 15px;
  height: 15px;
}

/* ===================== 消息区 ===================== */
.messages {
  flex: 1;
  overflow-y: auto;
  padding: 24px 0;
}

.messages__inner {
  max-width: 840px;
  margin: 0 auto;
  padding: 0 24px;
}

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
  margin-bottom: 20px;
}

.welcome__icon svg {
  width: 28px;
  height: 28px;
}

.welcome__title {
  font-family: var(--font-display);
  font-size: 20px;
  font-weight: 700;
  color: var(--text-primary);
  margin-bottom: 8px;
}

.welcome__desc {
  font-size: 13px;
  color: var(--text-muted);
  margin-bottom: 28px;
}

.welcome__tips {
  display: flex;
  gap: 24px;
  flex-wrap: wrap;
  justify-content: center;
}

.welcome__tip {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-muted);
}

.welcome__tip svg {
  width: 14px;
  height: 14px;
  color: var(--primary);
  flex-shrink: 0;
}

/* 消息行 */
.msg {
  display: flex;
  gap: 12px;
  margin-bottom: 28px;
}

.msg--user {
  flex-direction: row-reverse;
}

/* 头像 */
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

.avatar--user {
  background: var(--primary);
  color: var(--on-primary);
}

.avatar--ai {
  background: var(--bg-elevated);
  border: 1px solid var(--color-border-subtle);
  color: var(--primary-dark);
}

.avatar--ai svg {
  width: 16px;
  height: 16px;
}

/* 消息列 */
.msg__col {
  max-width: 78%;
  min-width: 0;
}

/* AI 回复允许更宽（含时间线 + 引用卡）*/
.msg--ai .msg__col {
  max-width: 88%;
}

.msg--user .msg__col {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
}

/* 气泡 */
.bubble {
  padding: 11px 15px;
  border-radius: var(--radius-md);
  font-size: 14px;
  line-height: 1.75;
  word-wrap: break-word;
}

.bubble--user {
  background: var(--bg-elevated);
  color: var(--text-primary);
  border-top-right-radius: 3px;
}

.bubble--ai {
  background: var(--bg-tertiary);
  color: var(--text-primary);
  border-top-left-radius: 3px;
}

/* Markdown 渲染 */
.bubble--ai :deep(p) { margin: 4px 0; }
.bubble--ai :deep(p:first-child) { margin-top: 0; }
.bubble--ai :deep(p:last-child) { margin-bottom: 0; }
.bubble--ai :deep(b), .bubble--ai :deep(strong) { color: var(--primary); font-weight: 600; }
.bubble--ai :deep(ul) { margin: 6px 0 2px; padding-left: 18px; }
.bubble--ai :deep(ol) { margin: 6px 0 2px; padding-left: 18px; }
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
.bubble--ai :deep(pre code) {
  background: transparent;
  padding: 0;
}

/* 时间戳 */
.msg__time {
  margin-top: 7px;
  font-size: 10.5px;
  color: var(--text-muted);
  font-family: var(--font-mono);
}

/* ===================== 引用来源 ===================== */
.sources {
  margin-top: 10px;
  border-radius: var(--radius-md);
  overflow: hidden;
  background: var(--bg-tertiary);
}

.sources__title {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 9px 13px;
  font-size: 11px;
  color: var(--text-muted);
  font-family: var(--font-mono);
  letter-spacing: 1px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.sources__title svg {
  width: 12px;
  height: 12px;
}

.src {
  display: flex;
  gap: 11px;
  align-items: center;
  padding: 9px 13px;
}

.src + .src {
  border-top: 1px solid var(--color-border-subtle);
}

.src__icon {
  color: var(--text-muted);
  flex: 0 0 14px;
  display: flex;
}

.src__icon svg {
  width: 14px;
  height: 14px;
}

.src__body {
  flex: 1;
  min-width: 0;
}

.src__name {
  font-size: 12px;
  font-family: var(--font-mono);
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.src__snip {
  font-size: 11px;
  color: var(--text-muted);
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.src__side {
  display: flex;
  align-items: center;
  gap: 8px;
}

.src__bar {
  width: 60px;
  height: 3px;
  border-radius: 2px;
  background: var(--bg-elevated);
  overflow: hidden;
}

.src__bar i {
  display: block;
  height: 100%;
  background: var(--primary-dark);
}

.src__pct {
  font-family: var(--font-mono);
  font-size: 10.5px;
  color: var(--text-secondary);
  width: 30px;
  text-align: right;
}

/* ===================== 流式光标 ===================== */
.streaming-cursor {
  display: flex;
  gap: 4px;
  align-items: center;
  padding: 14px 16px;
}

.streaming-cursor span {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--primary);
  animation: pulse 1.2s ease-in-out infinite;
}

.streaming-cursor span:nth-child(2) { animation-delay: 0.2s; }
.streaming-cursor span:nth-child(3) { animation-delay: 0.4s; }

/* ===================== 输入区 ===================== */
.composer-wrap {
  padding: 14px 24px 18px;
  border-top: 1px solid var(--color-border-subtle);
  flex-shrink: 0;
}

.composer {
  max-width: 840px;
  margin: 0 auto;
  border-radius: var(--radius-md);
  padding: 12px 14px;
  background: var(--bg-primary);
  border: 1px solid var(--color-border-subtle);
  transition: border-color var(--duration-fast);
}

.composer--focus {
  border-color: rgba(156, 180, 205, 0.5);
}

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

.composer__textarea::placeholder {
  color: var(--text-muted);
}

.composer__foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}

.tool-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  color: var(--text-muted);
  padding: 3px 9px;
  border-radius: 3px;
  border: 1px solid var(--color-border-subtle);
  cursor: pointer;
  transition: color var(--duration-fast), border-color var(--duration-fast);
}

.tool-chip:hover {
  color: var(--text-secondary);
  border-color: var(--color-border-default);
}

.tool-chip svg {
  width: 12px;
  height: 12px;
}

.tool-chip--warn {
  color: var(--warning);
  border-color: rgba(201, 168, 108, 0.3);
}

.composer__count {
  margin-left: auto;
  font-family: var(--font-mono);
  font-size: 10.5px;
  color: var(--text-muted);
}

/* 发送按钮 */
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

.btn-send:hover:not(:disabled) {
  background: var(--primary-light);
}

.btn-send:disabled {
  background: var(--bg-elevated);
  color: var(--text-muted);
  cursor: not-allowed;
}

.btn-send svg {
  width: 15px;
  height: 15px;
}

/* 图谱增强激活态 */
.tool-chip--active {
  color: var(--primary);
  border-color: rgba(156, 180, 205, 0.5);
}

/* ===================== 推荐追问 chips ===================== */
.suggest-row {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
  margin-top: 12px;
}

.welcome__suggest {
  justify-content: center;
  margin-top: 24px;
}

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

.suggest-chip:hover {
  color: var(--text-secondary);
  border-color: var(--color-border-default);
}

/* ===================== 侧栏底部索引概览 ===================== */
.side-stats {
  margin: 8px 14px 12px;
  padding-top: 10px;
  border-top: 1px solid var(--color-border-subtle);
  flex-shrink: 0;
}

.side-stats__row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 11.5px;
  color: var(--text-muted);
  margin-bottom: 6px;
}

.side-stats__row b {
  color: var(--text-secondary);
  font-family: var(--font-mono);
  font-weight: 500;
}

.side-stats__bar {
  height: 3px;
  border-radius: 2px;
  background: var(--bg-elevated);
  overflow: hidden;
  margin-top: 2px;
}

.side-stats__bar i {
  display: block;
  height: 100%;
  background: var(--primary-dark);
  transition: width var(--duration-slow);
}

.side-stats__cap {
  margin-top: 7px;
  font-size: 10.5px;
  color: var(--text-muted);
}
</style>
