<template>
  <div class="knowledge-detail-container">
    <UiPageHeader :title="knowledgeBase?.name || '知识库详情'" :divider="false">
      <template #actions>
        <UiButton variant="secondary" @click="goBack">返回</UiButton>
      </template>
    </UiPageHeader>

    <UiTabs
      v-model="activeTab"
      class="detail-tabs"
      :tabs="detailTabs"
      @change="handleTabChange"
    >
      <!-- 概览 -->
      <template #overview>
        <div class="overview-section" v-loading="statsLoading">
          <UiGrid v-if="stats" :columns="4" gap="lg">
            <UiStatistic title="文档数量" :value="stats.knowledge_count" suffix="个" />
            <UiStatistic title="分块数量" :value="stats.chunk_count" suffix="个" />
            <div class="statistic-item">
              <div class="statistic-title">总存储</div>
              <div class="statistic-value">{{ formatFileSize(stats.total_size) }}</div>
            </div>
            <div class="statistic-item">
              <div class="statistic-title">状态</div>
              <div class="statistic-value">
                <UiTag :variant="knowledgeBase?.status === 1 ? 'success' : 'info'">
                  {{ knowledgeBase?.status === 1 ? '启用' : '禁用' }}
                </UiTag>
              </div>
            </div>
          </UiGrid>

          <UiDivider />

          <UiDescriptions :column="2" border>
            <UiDescriptionsItem label="知识库名称">{{ knowledgeBase?.name }}</UiDescriptionsItem>
            <UiDescriptionsItem label="类型">通用</UiDescriptionsItem>
            <UiDescriptionsItem label="创建时间">
              {{ formatDateTime(knowledgeBase?.created_at) }}
            </UiDescriptionsItem>
            <UiDescriptionsItem label="更新时间">
              {{ formatDateTime(knowledgeBase?.updated_at) }}
            </UiDescriptionsItem>
            <UiDescriptionsItem label="描述" :span="2">
              {{ knowledgeBase?.description || '-' }}
            </UiDescriptionsItem>
          </UiDescriptions>
        </div>
      </template>

      <!-- 文档管理 -->
      <template #documents>
        <div class="documents-section">
          <div class="toolbar">
            <input
              ref="fileInputRef"
              type="file"
              accept=".txt,.md,.pdf,.doc,.docx"
              multiple
              style="display: none"
              @change="handleFileChange"
            />
            <input
              ref="folderInputRef"
              type="file"
              webkitdirectory
              directory
              multiple
              style="display: none"
              @change="handleFolderChange"
            />
            <UiButton variant="primary" icon @click="fileInputRef?.click()">
              <template #icon><el-icon><Upload /></el-icon></template>
              上传文档
            </UiButton>
            <UiButton variant="secondary" icon @click="folderInputRef?.click()">
              <template #icon><el-icon><FolderOpened /></el-icon></template>
              选择文件夹上传
            </UiButton>
            <UiText class="hint">支持 txt, md, pdf, doc, docx 格式，可多选或整个文件夹批量上传</UiText>
          </div>

          <UiDivider />

          <UiTable
            :columns="documentColumns"
            :data="knowledges"
            v-loading="knowledgesLoading"
            stripe
            row-key="id"
          >
            <template #cell-storage_size="{ row }">
              {{ formatFileSize(row.storage_size) }}
            </template>
            <template #cell-chunk_count="{ row }">
              {{ row.chunk_count || 0 }}
            </template>
            <template #cell-parse_status="{ row }">
              <UiTag :variant="getParseStatusType(row.parse_status)">
                {{ getParseStatusText(row.parse_status) }}
              </UiTag>
            </template>
            <template #cell-created_at="{ row }">
              {{ formatDateTime(row.created_at) }}
            </template>
            <template #cell-actions="{ row }">
              <UiButton
                variant="ghost"
                size="sm"
                @click.stop="viewKnowledgeChunks(row)"
                :disabled="row.chunk_count === 0"
              >
                查看分块
              </UiButton>
              <UiButton variant="danger" size="sm" @click.stop="deleteKnowledge(row.id)">
                删除
              </UiButton>
            </template>
          </UiTable>

          <UiEmpty v-if="!knowledgesLoading && knowledges.length === 0" description="暂无文档" />
        </div>
      </template>

      <!-- 知识检索 -->
      <template #search>
        <div class="search-section">
          <div class="search-input">
            <UiInput
              v-model="searchQuery"
              placeholder="输入要搜索的内容..."
              @keyup.enter="handleSearch"
            />
            <UiButton variant="primary" icon @click="handleSearch" :loading="searchLoading">
              <template #icon><el-icon><Search /></el-icon></template>
              搜索
            </UiButton>
          </div>

          <div class="search-options">
            <UiText>Top K:</UiText>
            <UiInputNumber v-model="searchTopK" :min="1" :max="20" size="sm" />
            <UiText>相似度阈值:</UiText>
            <UiInputNumber
              v-model="searchThreshold"
              :min="0"
              :max="1"
              :step="0.1"
              :precision="2"
              size="sm"
            />
          </div>

          <UiDivider />

          <div v-loading="searchLoading" class="search-results">
            <div v-if="searchResults.length > 0">
              <div
                v-for="(result, index) in searchResults"
                :key="result.chunk_id"
                class="search-result-item"
              >
                <div class="result-header">
                  <UiText type="primary" tag="b">结果 {{ index + 1 }}</UiText>
                </div>
                <div class="result-content">
                  <UiText>{{ result.content }}</UiText>
                </div>
                <div class="result-footer" v-if="result.knowledge_title">
                  <UiText size="sm" type="info">来源: {{ result.knowledge_title }}</UiText>
                </div>
              </div>
            </div>
            <UiEmpty
              v-else-if="!searchLoading && hasSearched"
              description="未找到相关内容"
            />
            <UiEmpty v-else description="输入关键词搜索知识库内容" />
          </div>
        </div>
      </template>

      <!-- 分块列表 -->
      <template #chunks>
        <div class="chunks-section">
          <div class="toolbar">
            <UiSelect
              v-model="selectedKnowledgeId"
              placeholder="选择文档"
              clearable
              :options="knowledgeOptions"
              @change="onSelectedKnowledgeChange"
              style="width: 300px"
            />
          </div>

          <UiDivider />

          <UiTable :columns="chunkColumns" :data="chunks" v-loading="chunksLoading" stripe>
            <template #cell-actions="{ row }">
              <UiButton variant="ghost" size="sm" @click.stop="viewChunkDetail(row)">
                详情
              </UiButton>
            </template>
          </UiTable>

          <div v-if="chunkTotal > chunkPageSize" class="chunks-pagination">
            <UiPagination
              :page="chunkPage"
              :page-size="chunkPageSize"
              :total="chunkTotal"
              show-info
              @update:page="loadChunks"
            />
          </div>

          <UiEmpty v-if="!chunksLoading && chunks.length === 0" description="暂无分块数据" />
        </div>
      </template>

      <!-- 图谱 -->
      <template #graph>
        <div class="graph-section">
          <GraphView
            v-if="graphTabLoaded"
            :kb-id="kbId"
            embedded
            :active="activeTab === 'graph'"
          />
        </div>
      </template>

      <!-- 设置 -->
      <template #settings>
        <div class="settings-section" v-loading="settingsLoading">
          <UiForm :model="settingsForm" label-position="left" label-width="140px" style="max-width: 700px">
            <!-- 基本信息 -->
            <UiDivider label-position="start">基本信息</UiDivider>

            <UiFormItem label="知识库名称">
              <UiInput v-model="settingsForm.name" placeholder="请输入知识库名称" />
            </UiFormItem>
            <UiFormItem label="描述">
              <UiTextarea
                v-model="settingsForm.description"
                :rows="3"
                placeholder="请输入描述"
              />
            </UiFormItem>
            <UiFormItem label="状态">
              <UiSwitch
                v-model="settingsForm.status"
                :true-value="1"
                :false-value="0"
              >{{ settingsForm.status === 1 ? '启用' : '禁用' }}</UiSwitch>
            </UiFormItem>

            <!-- 索引构建 -->
            <UiDivider label-position="start">索引构建</UiDivider>

            <UiFormItem label="构建知识图谱">
              <UiSwitch v-model="settingsForm.graph_enabled" />
              <UiText type="info" size="sm" style="display: block; margin-top: 4px">
                开启后，之后上传到本知识库的文档会自动提取知识图谱，供图谱检索使用；对已上传的历史文档不追溯重建，可用下方「补建图谱」处理。
              </UiText>
            </UiFormItem>

            <UiFormItem label="补建历史图谱">
              <div style="display: inline-flex; align-items: center; gap: 8px; flex-wrap: wrap;">
                <UiButton
                  variant="secondary"
                  @click="rebuildGraph"
                  :loading="rebuildTask.isPending.value"
                  :disabled="!settingsForm.graph_enabled"
                >
                  为本库补建图谱
                </UiButton>
                <UiAsyncStatus
                  :status="rebuildTask.status.value"
                  :message="rebuildTask.message.value"
                  @retry="rebuildGraph"
                />
              </div>
              <UiText type="info" size="sm" style="display: block; margin-top: 4px">
                复用已有分块，为开启开关之前上传的历史文档补建知识图谱，不会重复建块；文档较多时耗时较长，请耐心等待。
              </UiText>
            </UiFormItem>

            <UiFormItem>
              <UiButton variant="primary" @click="saveSettings" :loading="settingsSaving">
                保存设置
              </UiButton>
            </UiFormItem>
          </UiForm>

          <!-- 配置说明 -->
          <UiAlert
            title="配置说明"
            type="info"
            :closable="false"
            style="margin-top: 24px"
          >
            <ul style="margin: 0; padding-left: 20px;">
              <li><strong>分块配置</strong>：分块大小/重叠等数据处理参数在<strong>创建知识库时</strong>设定，创建后暂不支持修改</li>
              <li><strong>知识图谱</strong>：图谱提取为库级开关，可随时开关；仅对开启后<strong>新上传</strong>的文档生效，历史文档可用「补建图谱」按钮追溯重建</li>
              <li><strong>检索设置</strong>：TopK、相似度阈值等检索参数请在<strong>对话设置</strong>中调整，支持跨知识库检索</li>
            </ul>
          </UiAlert>
        </div>
      </template>
    </UiTabs>

    <!-- 上传文档对话框 -->
    <UiModal v-model="showUploadDialog" title="上传文档" size="md" :mask-closable="!uploading" :close-on-esc="!uploading">
      <UiForm :model="uploadForm" label-position="left" label-width="100px">
        <UiFormItem label="文件">
          <UiText>{{ uploadForm.file?.name }}</UiText>
        </UiFormItem>
        <UiFormItem v-if="uploading" label="进度">
          <UiProgress
            v-if="uploadStage === 'uploading'"
            :percentage="uploadProgress"
            :status="uploadProgress >= 100 ? 'success' : 'active'"
          />
          <UiProgress v-else :percentage="0" indeterminate />
        </UiFormItem>
      </UiForm>
      <UiText type="info" size="sm" style="display: block; margin-top: 8px">
        <template v-if="uploadStage === 'hashing'">正在校验文件（防止重复上传）…</template>
        <template v-else-if="uploadStage === 'uploading'">正在上传，请勿关闭页面…</template>
        <template v-else>文档标题默认取文件名，分块规则沿用知识库创建时的配置。</template>
      </UiText>
      <template #footer>
        <UiButton variant="secondary" @click="showUploadDialog = false" :disabled="uploading">取消</UiButton>
        <UiButton variant="primary" @click="uploadFile" :loading="uploading">
          上传
        </UiButton>
      </template>
    </UiModal>

    <!-- 文件夹选择：勾选/取消要上传的文件 -->
    <DirectoryTreePicker
      v-model="showTreePicker"
      :files="folderFiles"
      @confirm="startBatchUpload"
    />

    <!-- 批量上传进度 -->
    <UiModal
      v-model="showBatchDialog"
      title="批量上传"
      size="lg"
      :mask-closable="!batchUpload.running.value"
      :close-on-esc="!batchUpload.running.value"
    >
      <UiText size="sm" type="info" class="batch-summary">
        共 {{ batchUpload.summary.value.total }} 个文件
        · 成功 {{ batchUpload.summary.value.success }}
        · 重复跳过 {{ batchUpload.summary.value.duplicate }}
        · 失败 {{ batchUpload.summary.value.error }}
      </UiText>

      <UiDivider />

      <div class="batch-list">
        <div v-for="(item, idx) in batchUpload.items.value" :key="idx" class="batch-item">
          <UiText class="batch-item__name" :title="item.file.name">{{ item.file.name }}</UiText>
          <UiTag :variant="batchStatusVariant(item.status)" size="sm">
            {{ batchStatusText(item.status) }}
          </UiTag>
          <UiProgress
            v-if="item.status === 'uploading'"
            class="batch-item__progress"
            :percentage="item.progress"
            status="active"
          />
          <UiText v-if="item.message" size="sm" type="info" class="batch-item__message">
            {{ item.message }}
          </UiText>
        </div>
      </div>

      <template #footer>
        <UiButton variant="primary" :disabled="batchUpload.running.value" @click="showBatchDialog = false">
          {{ batchUpload.running.value ? '上传中…' : '完成' }}
        </UiButton>
      </template>
    </UiModal>

    <!-- 分块详情对话框 -->
    <UiModal v-model="showChunkDialog" title="分块详情" size="lg">
      <UiDescriptions :column="2" border v-if="currentChunk">
        <UiDescriptionsItem label="序号">{{ currentChunk.chunk_index }}</UiDescriptionsItem>
        <UiDescriptionsItem label="Token数">{{ currentChunk.token_count || 0 }}</UiDescriptionsItem>
        <UiDescriptionsItem label="内容" :span="2">
          <UiText style="white-space: pre-wrap">{{ currentChunk.content }}</UiText>
        </UiDescriptionsItem>
      </UiDescriptions>
    </UiModal>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
// 自研 UI 组件统一引入
import {
  UiPageHeader,
  UiTabs,
  UiGrid,
  UiStatistic,
  UiTag,
  UiDivider,
  UiDescriptions,
  UiDescriptionsItem,
  UiButton,
  UiText,
  UiTable,
  UiInput,
  UiTextarea,
  UiInputNumber,
  UiEmpty,
  UiModal,
  UiForm,
  UiFormItem,
  UiSwitch,
  UiAlert,
  UiSelect,
  UiProgress,
  UiAsyncStatus,
  UiPagination
} from '@/components'
import toast from '@/utils/toast'
import { useAsyncTask } from '@/composables/useAsyncTask'
import { useBatchUpload, type BatchItemStatus } from '@/composables/useBatchUpload'
import { ElMessageBox } from '@/utils/confirm'
import { Upload, Search, FolderOpened } from '@element-plus/icons-vue'
import GraphView from './GraphView.vue'
import DirectoryTreePicker from './DirectoryTreePicker.vue'
import { knowledgeApi } from '@/api/knowledge'
import { computeFileHash } from '@/utils/hash'
import type {
  KnowledgeBase,
  KnowledgeBaseStats,
  Knowledge,
  Chunk,
  UpdateKnowledgeBaseRequest
} from '@/types'

const router = useRouter()
const route = useRoute()

const kbId = ref<string>(route.params.id as string)
const activeTab = ref('overview')
const knowledgeBase = ref<KnowledgeBase | null>(null)

// Tabs 配置
const detailTabs = [
  { key: 'overview', label: '概览' },
  { key: 'documents', label: '文档' },
  { key: 'search', label: '检索' },
  { key: 'chunks', label: '分块' },
  { key: 'graph', label: '图谱' },
  { key: 'settings', label: '设置' }
]

// 图谱 tab 懒挂载：UiTabs 用 v-show 常驻所有面板，vis-network 在 display:none
// 容器里会拿到 0×0 尺寸而无法布局，故首次切到图谱 tab 才挂载 GraphView。
const graphTabLoaded = ref(false)

// 文档表格列
const documentColumns = [
  { key: 'title', title: '标题', width: 200 },
  { key: 'type', title: '类型', width: 100 },
  { key: 'storage_size', title: '大小', width: 120 },
  { key: 'chunk_count', title: '分块数', width: 100 },
  { key: 'parse_status', title: '处理状态', width: 120 },
  { key: 'created_at', title: '上传时间', width: 180 },
  { key: 'actions', title: '操作', width: 150 }
]

// 分块表格列
const chunkColumns = [
  { key: 'chunk_index', title: '序号', width: 80 },
  { key: 'content', title: '内容', width: 400 },
  { key: 'token_count', title: 'Token数', width: 100 },
  { key: 'actions', title: '操作', width: 120 }
]

// 概览相关
const statsLoading = ref(false)
const stats = ref<KnowledgeBaseStats | null>(null)

// 文档相关
const knowledgesLoading = ref(false)
const knowledges = ref<Knowledge[]>([])
const showUploadDialog = ref(false)
const uploading = ref(false)
// 上传进度（0-100）；-1 表示处于计算哈希/去重预检阶段（不确定进度）
const uploadProgress = ref(0)
const uploadStage = ref<'idle' | 'hashing' | 'uploading'>('idle')
// 后端上传接口仅接收 file，标题/分块参数由服务端按知识库配置决定，故不再收集
const uploadForm = reactive({
  file: null as File | null
})
// 文档状态轮询：指数退避 + 次数上限，避免卡 processing 的文档永久打后端。
// 起始 3s，每轮翻倍，封顶 30s；最多 40 轮（约 18 分钟）后停止并提示手动刷新。
const POLL_INITIAL_MS = 3000
const POLL_MAX_MS = 30000
const POLL_MAX_ATTEMPTS = 40
interface PollState {
  timer: ReturnType<typeof setTimeout> | null
  attempts: number
}
const statusPolling = ref<Record<string, PollState>>({})
// 文件选择（替代 el-upload）
const fileInputRef = ref<HTMLInputElement>()
const selectedFileName = ref('')

// 文件夹批量上传：选文件夹 → 树状勾选 → 批量上传进度
const folderInputRef = ref<HTMLInputElement>()
const folderFiles = ref<File[]>([])
const showTreePicker = ref(false)
const showBatchDialog = ref(false)
const batchUpload = useBatchUpload(kbId)

// 检索相关
const searchQuery = ref('')
const searchTopK = ref(5)
const searchThreshold = ref(0.6)
const searchLoading = ref(false)
const searchResults = ref<any[]>([])
const hasSearched = ref(false)

// 分块相关
const selectedKnowledgeId = ref<string>('')
const knowledgeOptions = computed(() =>
  knowledges.value.map((kb) => ({ label: kb.title, value: kb.id }))
)
const chunksLoading = ref(false)
const chunks = ref<Chunk[]>([])
const showChunkDialog = ref(false)
const currentChunk = ref<Chunk | null>(null)
// 分块分页：后端 /chunks 支持 page/size（默认 page_size=20），改为按页拉取，避免一次性拉全库分块卡死。
const chunkPage = ref(1)
const chunkPageSize = ref(20)
const chunkTotal = ref(0)
// 加载序号：仅采用最新一次请求的响应，消除「选文档」与「切 tab」两条路径的竞态双加载。
let chunkLoadSeq = 0

// 设置相关
const settingsLoading = ref(false)
const settingsSaving = ref(false)
// 补建图谱状态机：idle→pending→success|error，就地由 <UiAsyncStatus> 常驻展示
const rebuildTask = useAsyncTask()
// 后端 UpdateKnowledgeBase 接受 name/description/status，以及库级图谱开关 graph_enabled。
// 分块/BM25 等其余数据处理配置仅在创建时生效，故不在此维护。
const settingsForm = reactive<UpdateKnowledgeBaseRequest>({
  name: '',
  description: '',
  status: 1,
  graph_enabled: false
})

function formatDateTime(date?: string | number) {
  if (!date) return '-'
  // 后端 created_at/updated_at 为 Unix 秒级时间戳（整数）；兼容纯数字字符串与 ISO 字符串
  const ms =
    typeof date === 'number'
      ? date * 1000
      : /^\d+$/.test(date)
        ? Number(date) * 1000
        : Date.parse(date)
  if (Number.isNaN(ms)) return '-'
  return new Date(ms).toLocaleString('zh-CN')
}

function formatFileSize(bytes: number) {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i]
}

// 状态取值与后端一致：pending / processing / completed / failed
function getParseStatusType(status: string) {
  const map: Record<string, any> = {
    pending: 'info',
    processing: 'warning',
    completed: 'success',
    failed: 'danger'
  }
  return map[status] || 'info'
}

function getParseStatusText(status: string) {
  const map: Record<string, string> = {
    pending: '待处理',
    processing: '处理中',
    completed: '已完成',
    failed: '失败'
  }
  return map[status] || status
}

function goBack() {
  router.push('/knowledge')
}

// 加载知识库详情
async function loadKnowledgeBase() {
  try {
    const res = await knowledgeApi.getDetail(kbId.value)
    if (res.data) {
      knowledgeBase.value = res.data
      // 初始化设置表单（仅后端可更新的字段）
      const data = res.data
      settingsForm.name = data.name
      settingsForm.description = data.description || ''
      settingsForm.status = data.status
      settingsForm.graph_enabled = data.setting?.graph_enabled ?? false
    }
  } catch (error) {
    console.error('Failed to load knowledge base:', error)
  }
}

// 加载统计信息
async function loadStats() {
  statsLoading.value = true
  try {
    const res = await knowledgeApi.getStats(kbId.value)
    if (res.data) {
      stats.value = res.data
    }
  } catch (error) {
    console.error('Failed to load stats:', error)
  } finally {
    statsLoading.value = false
  }
}

// 加载文档列表
async function loadKnowledges() {
  knowledgesLoading.value = true
  try {
    const res = await knowledgeApi.getKnowledgeList(kbId.value)
    if (res.data) {
      // 处理分页响应或直接数组
      const items = (res.data as any).items || res.data || []
      knowledges.value = Array.isArray(items) ? items : []
      // 为处理中的文档启动状态轮询
      knowledges.value.forEach((kb) => {
        if (kb.parse_status === 'processing' || kb.parse_status === 'pending') {
          startStatusPolling(kb.id)
        }
      })
    } else {
      knowledges.value = []
    }
  } catch (error) {
    console.error('Failed to load knowledges:', error)
    knowledges.value = []
  } finally {
    knowledgesLoading.value = false
  }
}

// 启动状态轮询（指数退避 + 次数上限）
function startStatusPolling(knowledgeId: string) {
  // 清除旧的轮询，避免同一文档重复计时器
  stopStatusPolling(knowledgeId)

  const state: PollState = { timer: null, attempts: 0 }
  statusPolling.value[knowledgeId] = state

  const scheduleNext = () => {
    // 指数退避：3s、6s、12s… 封顶 30s
    const delay = Math.min(POLL_INITIAL_MS * 2 ** state.attempts, POLL_MAX_MS)
    state.timer = setTimeout(poll, delay)
  }

  const poll = async () => {
    state.attempts += 1
    try {
      const res = await knowledgeApi.getKnowledgeStatus(kbId.value, knowledgeId)
      if (res.data) {
        const status = res.data
        // 如果处理完成或失败，停止轮询并刷新列表
        if (status.parse_status === 'completed' || status.parse_status === 'failed') {
          stopStatusPolling(knowledgeId)
          await loadKnowledges()
          return
        }
      }
    } catch (error) {
      console.error('Failed to poll status:', error)
      stopStatusPolling(knowledgeId)
      return
    }
    // 达到次数上限：停止轮询并提示，避免卡 processing 的文档永久打后端
    if (state.attempts >= POLL_MAX_ATTEMPTS) {
      stopStatusPolling(knowledgeId)
      toast.info('文档处理耗时较长，已暂停自动刷新，可稍后手动刷新查看最新状态')
      return
    }
    scheduleNext()
  }

  scheduleNext()
}

// 停止状态轮询
function stopStatusPolling(knowledgeId: string) {
  const state = statusPolling.value[knowledgeId]
  if (state) {
    if (state.timer) clearTimeout(state.timer)
    delete statusPolling.value[knowledgeId]
  }
}

// 处理文件选择（原生 input，替代 el-upload 的 :on-change）
// 选中单个文件走原有单文件对话框流程；选中多个文件则走批量上传流程
function handleFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  const files = input.files ? Array.from(input.files) : []
  input.value = '' // 重置 input 以便重复选择同一文件时仍触发 change
  if (!files.length) return

  if (files.length === 1) {
    uploadForm.file = files[0]
    selectedFileName.value = files[0].name
    showUploadDialog.value = true
    return
  }
  startBatchUpload(files)
}

// 处理文件夹选择：先弹出目录树供用户勾选/取消，确认后再进入批量上传
function handleFolderChange(e: Event) {
  const input = e.target as HTMLInputElement
  const files = input.files ? Array.from(input.files) : []
  input.value = ''
  if (!files.length) return
  folderFiles.value = files
  showTreePicker.value = true
}

// 状态展示映射（与 useBatchUpload 的 BatchItemStatus 一一对应）
function batchStatusText(status: BatchItemStatus) {
  const map: Record<BatchItemStatus, string> = {
    pending: '等待中',
    checking: '校验中',
    duplicate: '已跳过',
    uploading: '上传中',
    success: '成功',
    error: '失败'
  }
  return map[status]
}

function batchStatusVariant(status: BatchItemStatus) {
  const map: Record<BatchItemStatus, 'primary' | 'success' | 'warning' | 'info' | 'danger' | 'default'> = {
    pending: 'info',
    checking: 'info',
    duplicate: 'warning',
    uploading: 'warning',
    success: 'success',
    error: 'danger'
  }
  return map[status]
}

// 启动批量上传：装入队列、打开进度弹窗，完成后若有成功项则刷新文档列表
function startBatchUpload(files: File[]) {
  batchUpload.setFiles(files)
  showBatchDialog.value = true
  batchUpload.start().then(() => {
    if (batchUpload.summary.value.success > 0) {
      loadKnowledges()
    }
  })
}

// 上传文件
async function uploadFile() {
  if (!uploadForm.file) {
    toast.warning('请选择文件')
    return
  }

  const file = uploadForm.file
  uploading.value = true
  uploadStage.value = 'hashing'
  uploadProgress.value = 0
  try {
    // 1. 计算哈希并预检，命中则拒绝重传，不再上传字节
    const fileHash = await computeFileHash(file)
    const check = await knowledgeApi.checkFile(kbId.value, fileHash)
    if (check.data?.duplicate) {
      toast.warning(`该文件已存在于知识库中（${check.data.title || file.name}），请勿重复上传`)
      return
    }

    // 2. 真正上传，实时上报进度
    uploadStage.value = 'uploading'
    const formData = new FormData()
    formData.append('file', file)

    const res = await knowledgeApi.uploadFile(kbId.value, formData, (percent) => {
      uploadProgress.value = percent
    })
    if (res.data) {
      toast.success('文件上传成功，正在处理中...')
      showUploadDialog.value = false
      // 重置文件选择状态
      selectedFileName.value = ''
      uploadForm.file = null
      if (fileInputRef.value) fileInputRef.value.value = ''
      // 后端已同步创建 processing 状态的文档记录并返回真实 ID，
      // 刷新列表即可立即看到该文档，loadKnowledges 会为其启动状态轮询。
      await loadKnowledges()
    }
  } catch (error: any) {
    // 后端在并发竞态下可能返回 409（预检后被他人抢先上传相同文件）
    const resp = error?.response
    if (resp?.status === 409) {
      const dup = resp.data?.data
      toast.warning(resp.data?.message || `该文件已存在于知识库中（${dup?.title || file.name}），请勿重复上传`)
    } else {
      toast.error(error.message || '上传失败')
    }
  } finally {
    uploading.value = false
    uploadStage.value = 'idle'
    uploadProgress.value = 0
  }
}

// 删除文档
async function deleteKnowledge(id: string) {
  try {
    await ElMessageBox.confirm({
      message: '确定要删除这个文档吗？此操作不可恢复。',
      title: '删除确认',
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })

    await knowledgeApi.deleteKnowledge(kbId.value, id)
    toast.success('文档删除成功')
    await loadKnowledges()
    await loadStats()
  } catch (error: any) {
    if (error !== 'cancel') {
      toast.error(error.message || '删除失败')
    }
  }
}

// 查看文档分块
function viewKnowledgeChunks(knowledge: Knowledge) {
  selectedKnowledgeId.value = knowledge.id
  activeTab.value = 'chunks'
  loadChunks()
}

// 搜索知识
async function handleSearch() {
  if (!searchQuery.value.trim()) {
    toast.warning('请输入搜索内容')
    return
  }

  searchLoading.value = true
  hasSearched.value = true
  try {
    const res = await knowledgeApi.search({
      query: searchQuery.value,
      kb_ids: [kbId.value],
      top_k: searchTopK.value,
      min_score: searchThreshold.value
    })
    if (res.data) {
      // 后端返回 { total, items }
      searchResults.value = res.data.items || []
      if (searchResults.value.length === 0) {
        toast.info('未找到相关内容')
      }
    }
  } catch (error: any) {
    toast.error(error.message || '搜索失败')
  } finally {
    searchLoading.value = false
  }
}

// 加载分块（分页；selectedKnowledgeId 为空时加载全库分块，非空时限定文档）
async function loadChunks(page = 1) {
  const seq = ++chunkLoadSeq
  chunksLoading.value = true
  try {
    const res = await knowledgeApi.getChunks(kbId.value, {
      page,
      size: chunkPageSize.value,
      ...(selectedKnowledgeId.value ? { knowledge_id: selectedKnowledgeId.value } : {})
    })
    // 已有更新的加载发起，丢弃过期响应，避免竞态覆盖
    if (seq !== chunkLoadSeq) return
    const data = res.data as any
    if (data) {
      const items = data.items || data || []
      chunks.value = Array.isArray(items) ? items : []
      chunkTotal.value = typeof data.total === 'number' ? data.total : chunks.value.length
      chunkPage.value = typeof data.page === 'number' ? data.page : page
      // 后端权威页大小（默认 20）；若返回则同步，保证分页器与实际口径一致
      if (typeof data.size === 'number' && data.size > 0) chunkPageSize.value = data.size
    } else {
      chunks.value = []
      chunkTotal.value = 0
    }
  } catch (error) {
    if (seq !== chunkLoadSeq) return
    console.error('Failed to load chunks:', error)
    chunks.value = []
    chunkTotal.value = 0
  } finally {
    if (seq === chunkLoadSeq) chunksLoading.value = false
  }
}

// 切换文档筛选：重置到第一页再加载
function onSelectedKnowledgeChange() {
  loadChunks(1)
}

// 查看分块详情
function viewChunkDetail(chunk: Chunk) {
  currentChunk.value = chunk
  showChunkDialog.value = true
}

// 保存设置
async function saveSettings() {
  if (!settingsForm.name) {
    toast.warning('请输入知识库名称')
    return
  }

  settingsSaving.value = true
  try {
    // 后端 UpdateKnowledgeBase 接受 name/description/status + 库级图谱开关 graph_enabled
    const updateData: UpdateKnowledgeBaseRequest = {
      name: settingsForm.name,
      description: settingsForm.description,
      status: settingsForm.status,
      graph_enabled: settingsForm.graph_enabled
    }
    await knowledgeApi.update(kbId.value, updateData)
    toast.success('设置保存成功')
    await loadKnowledgeBase()
  } catch (error: any) {
    toast.error(error.message || '保存失败')
  } finally {
    settingsSaving.value = false
  }
}

// 为历史文档补建知识图谱（复用已存分块）
async function rebuildGraph() {
  if (!settingsForm.graph_enabled) {
    toast.warning('请先开启并保存「构建知识图谱」后再补建')
    return
  }
  await rebuildTask.run(
    async () => {
      const res = await knowledgeApi.rebuildGraph(kbId.value)
      return res.data
    },
    {
      pendingMessage: '补建中…（文档较多时耗时较长，请勿关闭页面）',
      successMessage: (r: any) => {
        if (!r) return '补建完成'
        // 有失败文档时如实标注，避免把「部分失败」伪装成完全成功
        const failed = r.failed_documents ? `，失败 ${r.failed_documents} 篇` : ''
        const skipped = r.skipped_documents ? `，跳过 ${r.skipped_documents} 篇` : ''
        return `补建完成：处理 ${r.processed_documents}/${r.total_documents} 篇，` +
          `新增 ${r.total_nodes} 节点、${r.total_relations} 关系${skipped}${failed}`
      },
      errorMessage: (e: any) => e?.message || '补建失败',
    }
  )
}

// 标签切换处理
function handleTabChange(tabName: string | number) {
  const tab = String(tabName)
  if (tab === 'overview' && !stats.value) {
    loadStats()
  } else if (tab === 'documents' && knowledges.value.length === 0) {
    loadKnowledges()
  } else if (tab === 'chunks' && chunks.value.length === 0) {
    // 首次切换到分块tab时，自动加载第一页分块（不限定文档）
    loadChunks(1)
  } else if (tab === 'graph') {
    // 首次进入图谱 tab 时挂载 GraphView（容器此时可见，vis-network 能正确取到尺寸）
    graphTabLoaded.value = true
  }
}

onMounted(async () => {
  await loadKnowledgeBase()
  await loadStats()
})

onUnmounted(() => {
  // 清理所有轮询
  Object.keys(statusPolling.value).forEach((key) => {
    stopStatusPolling(key)
  })
})
</script>

<style scoped>
.knowledge-detail-container {
  padding: 24px;
  background: rgba(0, 0, 0, 0.2);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-lg);
}

.detail-tabs {
  margin-top: 24px;
}

.overview-section,
.documents-section,
.search-section,
.chunks-section,
.settings-section {
  padding: 16px 0;
}

.graph-section {
  padding: 16px 0;
}

.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
}

.chunks-pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

.hint {
  font-size: 12px;
  color: var(--color-text-muted);
}

.search-input {
  margin-bottom: 16px;
}

.search-options {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

.search-results {
  min-height: 200px;
}

.search-result-item {
  padding: 16px;
  background: rgba(0, 0, 0, 0.2);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-md);
  margin-bottom: 12px;
  transition: all var(--transition-base);
}

.search-result-item:hover {
  border-color: rgba(34, 211, 238, 0.3);
  background: rgba(0, 0, 0, 0.3);
}

.result-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.result-content {
  margin-bottom: 12px;
  line-height: 1.6;
  color: var(--color-text-secondary);
}

.result-footer {
  display: flex;
  align-items: center;
}

.statistic-item {
  text-align: center;
}

.statistic-title {
  font-size: 14px;
  color: var(--color-text-muted);
  margin-bottom: 8px;
}

.statistic-value {
  font-size: 24px;
  font-weight: bold;
  color: var(--color-text-primary);
}

.batch-summary {
  display: block;
  margin-bottom: 8px;
}

.batch-list {
  max-height: 360px;
  overflow-y: auto;
}

.batch-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.batch-item__name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.batch-item__progress {
  width: 120px;
  flex-shrink: 0;
}

.batch-item__message {
  flex-shrink: 0;
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
