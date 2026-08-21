<template>
  <div class="evaluation-detail-container">
    <!-- 返回按钮 -->
    <div class="back-header">
      <UiButton variant="secondary" icon @click="goBack">
        <template #icon><el-icon><ArrowLeft /></el-icon></template>
        返回列表
      </UiButton>
    </div>

    <!-- 加载状态 -->
    <div v-if="loading" class="loading-state">
      <el-icon class="is-loading" :size="48"><Loading /></el-icon>
      <p>加载中...</p>
    </div>

    <!-- 内容 -->
    <template v-else-if="detail">
      <!-- 任务信息卡片 -->
      <UiCard class="info-card">
        <template #header>
          <div class="card-header">
            <h3>任务信息</h3>
            <UiTag v-if="detail.task" :variant="getStatusType(detail.task.status)">
              {{ getStatusText(detail.task.status) }}
            </UiTag>
          </div>
        </template>
        <UiDescriptions v-if="detail.task" :column="3" border>
          <UiDescriptionsItem label="任务ID">{{ detail.task.task_id.substring(0, 8) }}...</UiDescriptionsItem>
          <UiDescriptionsItem label="数据集id">{{ detail.task.dataset_id || detail.dataset_id }}</UiDescriptionsItem>
          <UiDescriptionsItem label="数据集名称" :mono="false">
            <button
              v-if="detail.dataset_name || detail.task.dataset_id || detail.dataset_id"
              type="button"
              class="dataset-name-link"
              title="查看数据集"
              @click.stop="goToDataset(detail.dataset_name, detail.task.dataset_id || detail.dataset_id)"
            >
              {{ detail.dataset_name || detail.task.dataset_id || detail.dataset_id }}
            </button>
            <span v-else>-</span>
          </UiDescriptionsItem>
          <UiDescriptionsItem label="进度">
            {{ detail.task.success_count }} / {{ detail.task.total_count }}
          </UiDescriptionsItem>
          <UiDescriptionsItem label="创建时间">{{ formatTime(detail.task.created_at) }}</UiDescriptionsItem>
          <UiDescriptionsItem label="更新时间">
            {{ formatTime(detail.task.updated_at) }}
          </UiDescriptionsItem>
          <UiDescriptionsItem v-if="detail.task.error_message" label="错误信息" :span="3">
            <UiText type="danger">{{ detail.task.error_message }}</UiText>
          </UiDescriptionsItem>
        </UiDescriptions>
        <UiDescriptions v-else :column="3" border>
          <UiDescriptionsItem label="任务ID">{{ detail.task_id?.substring(0, 8) }}...</UiDescriptionsItem>
          <UiDescriptionsItem label="数据集id">{{ detail.dataset_id }}</UiDescriptionsItem>
          <UiDescriptionsItem label="数据集名称" :mono="false">
            <button
              v-if="detail.dataset_name || detail.dataset_id"
              type="button"
              class="dataset-name-link"
              title="查看数据集"
              @click.stop="goToDataset(detail.dataset_name, detail.dataset_id)"
            >
              {{ detail.dataset_name || detail.dataset_id }}
            </button>
            <span v-else>-</span>
          </UiDescriptionsItem>
        </UiDescriptions>
      </UiCard>

      <!-- 聚合指标卡片（任务级，注册表驱动动态渲染，兼容固定字段；
           = 各 QA 行动态 scores 均值 ∪ 后端 detail.metrics 扁平聚合，
           开发者新增 grader 自动出现，无需改前端） -->
      <UiCard v-if="aggregateEntries.length > 0" class="metrics-card">
        <template #header>
          <h3>聚合指标</h3>
        </template>
        <div class="metrics-grid">
          <div class="metric-card" v-for="item in aggregateEntries" :key="item.key">
            <div class="metric-label">{{ item.label }}</div>
            <div class="metric-value">{{ item.text }}</div>
          </div>
        </div>
      </UiCard>

      <!-- QA 级别结果 -->
      <UiCard class="qa-results-card">
        <template #header>
          <div class="card-header">
            <h3>QA 级别结果</h3>
            <div class="header-actions">
              <UiSelect
                v-model="sortBy"
                size="sm"
                class="sort-select"
                :options="sortByOptions"
                @change="handleSortChange"
              />
              <UiSelect
                v-model="sortOrder"
                size="sm"
                class="order-select"
                :options="sortOrderOptions"
                @change="handleSortChange"
              />
              <UiButton variant="secondary" size="sm" icon @click="exportResults">
                <template #icon><el-icon><Download /></el-icon></template>
                导出
              </UiButton>
            </div>
          </div>
        </template>

        <!-- 筛选器 -->
        <div class="filter-bar">
          <UiInput
            v-model="searchQuery"
            placeholder="搜索问题..."
            size="sm"
            class="search-input"
            clearable
          >
            <template #prefix><el-icon><Search /></el-icon></template>
          </UiInput>
          <div class="score-sliders">
            <UiSlider
              v-model="scoreRange[0]"
              :min="0"
              :max="100"
              :step="5"
              :show-value="false"
              :marks="scoreMarks"
              @change="handleScoreMinChange"
            />
            <UiSlider
              v-model="scoreRange[1]"
              :min="0"
              :max="100"
              :step="5"
              :show-value="false"
              :marks="scoreMarks"
              @change="handleScoreMaxChange"
            />
          </div>
          <span class="filter-label">得分: {{ scoreRange[0] }}% - {{ scoreRange[1] }}%</span>
        </div>

        <!-- QA 列表 -->
        <div v-loading="qaLoading" class="qa-list">
          <div v-for="qa in displayQAResults" :key="qa.index" class="qa-item">
            <div class="qa-header">
              <span class="qa-index">#{{ qa.index + 1 }}</span>
              <div class="qa-metrics">
                <UiTag v-if="qa.bleu !== undefined" size="sm" variant="info">
                  BLEU: {{ formatPercent(qa.bleu) }}
                </UiTag>
                <UiTag v-if="qa.rouge1 !== undefined" size="sm" variant="success">
                  ROUGE-1: {{ formatPercent(qa.rouge1) }}
                </UiTag>
                <UiTag v-if="qa.score !== undefined" size="sm" :variant="getScoreType(qa.score)">
                  得分: {{ formatPercent(qa.score) }}
                </UiTag>
              </div>
            </div>
            <div class="qa-content">
              <div class="qa-block">
                <div class="qa-label">
                  <el-icon><QuestionFilled /></el-icon>
                  问题
                </div>
                <div class="qa-text">{{ qa.question }}</div>
              </div>
              <div class="qa-block">
                <div class="qa-label">
                  <el-icon><CircleCheck /></el-icon>
                  参考答案
                </div>
                <div class="qa-text">{{ qa.reference_answer }}</div>
              </div>
              <div v-if="qa.generated_answer" class="qa-block">
                <div class="qa-label">
                  <el-icon><ChatLineSquare /></el-icon>
                  生成答案
                </div>
                <div class="qa-text">{{ qa.generated_answer }}</div>
              </div>
              <div v-if="qa.retrieved_passages && qa.retrieved_passages.length > 0" class="qa-block">
                <div class="qa-label">
                  <el-icon><Document /></el-icon>
                  检索段落
                </div>
                <div class="qa-passages">
                  <div v-for="(p, i) in qa.retrieved_passages.slice(0, 2)" :key="i" class="passage-item">
                    {{ p }}
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- 分页 -->
          <UiPagination
            v-if="totalQAResults > pageSize"
            :page="currentPage"
            :page-size="pageSize"
            :total="totalQAResults"
            :page-sizes="[10, 20, 50, 100]"
            show-size-changer
            class="qa-pagination"
            @update:page="handlePageChange"
            @update:page-size="handleSizeChange"
          />
        </div>
      </UiCard>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import toast from '@/utils/toast'
import {
  UiButton,
  UiCard,
  UiTag,
  UiText,
  UiDescriptions,
  UiDescriptionsItem,
  UiInput,
  UiSelect,
  UiSlider,
  UiPagination
} from '@/components'
import {
  ArrowLeft,
  Loading,
  Download,
  Search,
  QuestionFilled,
  CircleCheck,
  ChatLineSquare,
  Document
} from '@element-plus/icons-vue'
import { evaluationApi } from '@/api/evaluation'
import { connectTaskProgress, type SSEConnection } from '@/utils/sse'
import { taskAggregateEntries } from './evaluation-config'
import {
  EvaluationStatus,
  EvaluationStatusText,
  EvaluationStatusType,
  type EvaluationDetail,
  type QAResult,
  type SSEProgressEvent
} from '@/types'

const route = useRoute()
const router = useRouter()
const taskId = computed(() => route.params.id as string || route.query.taskId as string)

const loading = ref(true)
const qaLoading = ref(false)
const detail = ref<EvaluationDetail | null>(null)
const qaResults = ref<QAResult[]>([])
const sseClient = ref<SSEConnection | null>(null)

// 筛选和排序
const searchQuery = ref('')
const scoreRange = ref<[number, number]>([0, 100])
const sortBy = ref('index')
const sortOrder = ref<'asc' | 'desc'>('desc')
const currentPage = ref(1)
const pageSize = ref(20)
const totalQAResults = ref(0)

// 排序选项
const sortByOptions = [
  { label: '按索引', value: 'index' },
  { label: '按 BLEU', value: 'bleu' },
  { label: '按 ROUGE-1', value: 'rouge1' },
  { label: '按得分', value: 'score' }
]
const sortOrderOptions = [
  { label: '降序', value: 'desc' },
  { label: '升序', value: 'asc' }
]

// 得分滑块刻度
const scoreMarks = [
  { value: 0, label: '0%' },
  { value: 50, label: '50%' },
  { value: 100, label: '100%' }
]

// 任务级聚合指标（注册表驱动动态渲染：per-QA 均值 ∪ 后端 detail.metrics 扁平聚合）
const aggregateEntries = computed(() => taskAggregateEntries(detail.value))

// 过滤和排序后的 QA 结果
const filteredQAResults = computed(() => {
  let results = [...qaResults.value]

  // 搜索过滤
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    results = results.filter(qa =>
      qa.question.toLowerCase().includes(query) ||
      qa.reference_answer.toLowerCase().includes(query)
    )
  }

  // 得分范围过滤
  results = results.filter(qa => {
    if (qa.score === undefined) return true
    const scorePercent = qa.score * 100
    return scorePercent >= scoreRange.value[0] && scorePercent <= scoreRange.value[1]
  })

  // 排序
  results.sort((a, b) => {
    let aVal: number, bVal: number

    switch (sortBy.value) {
      case 'bleu':
        aVal = a.bleu ?? 0
        bVal = b.bleu ?? 0
        break
      case 'rouge1':
        aVal = a.rouge1 ?? 0
        bVal = b.rouge1 ?? 0
        break
      case 'score':
        aVal = a.score ?? 0
        bVal = b.score ?? 0
        break
      default:
        aVal = a.index
        bVal = b.index
    }

    return sortOrder.value === 'asc' ? aVal - bVal : bVal - aVal
  })

  return results
})

// 分页显示的 QA 结果
const displayQAResults = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  const end = start + pageSize.value
  return filteredQAResults.value.slice(start, end)
})

function getStatusType(status: EvaluationStatus) {
  return EvaluationStatusType[status]
}

function getStatusText(status: EvaluationStatus) {
  return EvaluationStatusText[status]
}

function formatTime(timeStr: string): string {
  if (!timeStr) return '-'
  return new Date(timeStr).toLocaleString('zh-CN')
}

function formatPercent(value: number): string {
  return (value * 100).toFixed(2) + '%'
}

function getScoreType(score: number): 'success' | 'warning' | 'danger' | 'info' {
  if (score >= 0.8) return 'success'
  if (score >= 0.6) return 'info'
  if (score >= 0.4) return 'warning'
  return 'danger'
}

/** 跳转数据集管理，并把名称 / ID 写入筛选条件 */
async function goToDataset(name?: string, id?: string) {
  const query: Record<string, string> = {}
  if (name?.trim()) query.name = name.trim()
  if (id?.trim()) query.id = id.trim()
  await router.push({ path: '/datasets', query })
}

function goBack() {
  router.push({ name: 'EvaluationList' })
}

async function loadDetail() {
  if (!taskId.value) return

  loading.value = true
  try {
    const res = await evaluationApi.getResult(taskId.value)
    if (res.data) {
      detail.value = res.data

      // 如果任务正在运行，连接 SSE
      if (res.data.task?.status === EvaluationStatus.Running) {
        connectSSE(taskId.value)
      }

      // 加载 QA 结果
      await loadQAResults()
    }
  } catch (error: any) {
    toast.error(error.message || '加载详情失败')
  } finally {
    loading.value = false
  }
}

async function loadQAResults() {
  if (!taskId.value) return

  qaLoading.value = true
  try {
    const res = await evaluationApi.getQAResults(taskId.value, {
      page: currentPage.value,
      page_size: 1000 // 获取所有结果用于前端筛选
    })
    if (res.data) {
      qaResults.value = res.data.results || []
      totalQAResults.value = res.data.total || qaResults.value.length
    }
  } catch (error: any) {
    console.error('Failed to load QA results:', error)
  } finally {
    qaLoading.value = false
  }
}

function connectSSE(id: string) {
  disconnectSSE()

  sseClient.value = connectTaskProgress(
    id,
    {
      onMessage: (event) => {
        try {
          const data = JSON.parse(event.data) as SSEProgressEvent

          if (detail.value?.task && detail.value.task.task_id === data.task_id) {
            detail.value.task.status = data.data.status

            // 完成或失败时重新加载
            if (data.data.status === EvaluationStatus.Completed ||
                data.data.status === EvaluationStatus.Failed) {
              disconnectSSE()
              loadDetail()
            }
          }
        } catch (e) {
          console.error('Failed to parse SSE message:', e)
        }
      }
    },
    { autoReconnect: true }
  )
}

function disconnectSSE() {
  sseClient.value?.disconnect()
  sseClient.value = null
}

function handleSortChange() {
  currentPage.value = 1
}

function handleFilterChange() {
  currentPage.value = 1
}

// 得分下限滑块：不得超过上限
function handleScoreMinChange(value: number) {
  if (value > scoreRange.value[1]) {
    scoreRange.value[1] = value
  }
  scoreRange.value[0] = value
  handleFilterChange()
}

// 得分上限滑块：不得低于下限
function handleScoreMaxChange(value: number) {
  if (value < scoreRange.value[0]) {
    scoreRange.value[0] = value
  }
  scoreRange.value[1] = value
  handleFilterChange()
}

function handlePageChange(page: number) {
  currentPage.value = page
}

function handleSizeChange(size: number) {
  pageSize.value = size
  currentPage.value = 1
}

function exportResults() {
  if (!filteredQAResults.value.length) {
    toast.warning('没有可导出的数据')
    return
  }

  // 生成 CSV
  const headers = ['Index', 'Question', 'Reference Answer', 'Generated Answer', 'BLEU', 'ROUGE-1', 'Score']
  const rows = filteredQAResults.value.map(qa => [
    qa.index,
    `"${qa.question.replace(/"/g, '""')}"`,
    `"${qa.reference_answer.replace(/"/g, '""')}"`,
    `"${(qa.generated_answer || '').replace(/"/g, '""')}"`,
    qa.bleu?.toFixed(4) || '',
    qa.rouge1?.toFixed(4) || '',
    qa.score?.toFixed(4) || ''
  ])

  const csv = [headers.join(','), ...rows.map(r => r.join(','))].join('\n')
  const blob = new Blob(['﻿' + csv], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `evaluation_${taskId.value}_results.csv`
  link.click()
  URL.revokeObjectURL(url)

  toast.success('导出成功')
}

onMounted(() => {
  loadDetail()
})

onUnmounted(() => {
  disconnectSSE()
})
</script>

<style scoped>
.dataset-name-link {
  padding: 0;
  border: none;
  background: none;
  color: var(--primary, #22d3ee);
  cursor: pointer;
  font: inherit;
  text-decoration: underline;
  text-underline-offset: 2px;
}

.dataset-name-link:hover {
  opacity: 0.85;
}

.evaluation-detail-container {
  padding: 24px;
  max-width: 1400px;
  margin: 0 auto;
}

.back-header {
  margin-bottom: 16px;
}

.loading-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px;
  color: var(--color-text-muted);
}

.loading-state p {
  margin-top: 16px;
}

.info-card,
.metrics-card,
.qa-results-card {
  margin-bottom: 24px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-header h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--color-text-primary);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.sort-select {
  width: 140px;
}

.order-select {
  width: 100px;
}

.metrics-section {
  margin-bottom: 24px;
}

.metrics-section:last-child {
  margin-bottom: 0;
}

.metrics-section h4 {
  margin: 0 0 16px 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--color-text-secondary);
}

.metrics-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 12px;
}

.metric-card {
  padding: 16px;
  background: rgba(0, 0, 0, 0.25);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 12px;
  text-align: center;
  transition: all var(--transition-base);
}

.metric-card:hover {
  border-color: rgba(34, 211, 238, 0.3);
}

.metric-label {
  font-size: 13px;
  color: var(--color-text-muted);
  margin-bottom: 8px;
}

.metric-value {
  font-size: 20px;
  font-weight: 600;
  color: #22d3ee;
}

.filter-bar {
  display: flex;
  align-items: center;
  margin-bottom: 16px;
}

.search-input {
  width: 240px;
}

.score-sliders {
  display: flex;
  flex-direction: column;
  gap: 12px;
  width: 300px;
  margin: 0 16px;
}

.filter-label {
  font-size: 13px;
  color: var(--color-text-secondary);
}

.qa-pagination {
  margin-top: 16px;
  justify-content: center;
}

.qa-list {
  max-height: 1200px;
  overflow-y: auto;
}

.qa-item {
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 12px;
  padding: 16px;
  margin-bottom: 16px;
  background: rgba(0, 0, 0, 0.2);
  transition: all var(--transition-base);
}

.qa-item:hover {
  border-color: rgba(34, 211, 238, 0.2);
}

.qa-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.qa-index {
  font-weight: 600;
  color: var(--color-text-primary);
}

.qa-metrics {
  display: flex;
  gap: 8px;
}

.qa-content {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.qa-block {
  background: rgba(0, 0, 0, 0.25);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 8px;
  padding: 12px;
}

.qa-label {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-secondary);
  margin-bottom: 8px;
}

.qa-text {
  color: var(--color-text-secondary);
  line-height: 1.6;
  white-space: pre-wrap;
}

.qa-passages {
  margin-top: 8px;
}

.passage-item {
  padding: 8px;
  background: rgba(0, 0, 0, 0.2);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 8px;
  font-size: 13px;
  color: var(--color-text-secondary);
  margin-bottom: 8px;
}

.passage-item:last-child {
  margin-bottom: 0;
}
</style>
