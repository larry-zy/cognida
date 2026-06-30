<template>
  <div class="evaluation-detail-container">
    <!-- 返回按钮 -->
    <div class="back-header">
      <el-button :icon="ArrowLeft" @click="goBack">返回列表</el-button>
    </div>

    <!-- 加载状态 -->
    <div v-if="loading" class="loading-state">
      <el-icon class="is-loading" :size="48"><Loading /></el-icon>
      <p>加载中...</p>
    </div>

    <!-- 内容 -->
    <template v-else-if="detail">
      <!-- 任务信息卡片 -->
      <el-card class="info-card" shadow="never">
        <template #header>
          <div class="card-header">
            <h3>任务信息</h3>
            <el-tag v-if="detail.task" :type="getStatusType(detail.task.status)">
              {{ getStatusText(detail.task.status) }}
            </el-tag>
          </div>
        </template>
        <el-descriptions v-if="detail.task" :column="3" border>
          <el-descriptions-item label="任务ID">{{ detail.task.task_id.substring(0, 8) }}...</el-descriptions-item>
          <el-descriptions-item label="数据集">{{ detail.task.dataset_id }}</el-descriptions-item>
          <el-descriptions-item label="进度">
            {{ detail.task.success_count }} / {{ detail.task.total_count }}
          </el-descriptions-item>
          <el-descriptions-item label="创建时间">{{ formatTime(detail.task.created_at) }}</el-descriptions-item>
          <el-descriptions-item label="更新时间">
            {{ formatTime(detail.task.updated_at) }}
          </el-descriptions-item>
          <el-descriptions-item v-if="detail.task.error_message" label="错误信息" :span="3">
            <el-text type="danger">{{ detail.task.error_message }}</el-text>
          </el-descriptions-item>
        </el-descriptions>
      </el-card>

      <!-- 聚合指标卡片 -->
      <el-card v-if="detail.metric" class="metrics-card" shadow="never">
        <template #header>
          <h3>聚合指标</h3>
        </template>

        <!-- 检索指标 -->
        <div v-if="detail.metric.retrieval_metrics" class="metrics-section">
          <h4>检索指标</h4>
          <div class="metrics-grid">
            <div class="metric-card" v-for="(value, key) in displayRetrievalMetrics" :key="key">
              <div class="metric-label">{{ retrievalLabels[key as keyof RetrievalMetrics] }}</div>
              <div class="metric-value">{{ formatPercent(value) }}</div>
            </div>
          </div>
        </div>

        <!-- 生成指标 -->
        <div v-if="detail.metric.generation_metrics" class="metrics-section">
          <h4>生成指标</h4>
          <div class="metrics-grid">
            <div class="metric-card" v-for="(value, key) in displayGenerationMetrics" :key="key">
              <div class="metric-label">{{ generationLabels[key as keyof GenerationMetrics] }}</div>
              <div class="metric-value">{{ formatPercent(value) }}</div>
            </div>
          </div>
        </div>
      </el-card>

      <!-- QA 级别结果 -->
      <el-card class="qa-results-card" shadow="never">
        <template #header>
          <div class="card-header">
            <h3>QA 级别结果</h3>
            <div class="header-actions">
              <el-select v-model="sortBy" size="small" style="width: 140px; margin-right: 8px" @change="handleSortChange">
                <el-option label="按索引" value="index" />
                <el-option label="按 BLEU" value="bleu" />
                <el-option label="按 ROUGE-1" value="rouge1" />
                <el-option label="按得分" value="score" />
              </el-select>
              <el-select v-model="sortOrder" size="small" style="width: 100px" @change="handleSortChange">
                <el-option label="降序" value="desc" />
                <el-option label="升序" value="asc" />
              </el-select>
              <el-button size="small" :icon="Download" @click="exportResults">导出</el-button>
            </div>
          </div>
        </template>

        <!-- 筛选器 -->
        <div class="filter-bar">
          <el-input
            v-model="searchQuery"
            placeholder="搜索问题..."
            :prefix-icon="Search"
            size="small"
            style="width: 240px"
            clearable
          />
          <el-slider
            v-model="scoreRange"
            range
            :min="0"
            :max="100"
            :step="5"
            :marks="{ 0: '0%', 50: '50%', 100: '100%' }"
            style="width: 300px; margin: 0 16px"
            @change="handleFilterChange"
          />
          <span class="filter-label">得分: {{ scoreRange[0] }}% - {{ scoreRange[1] }}%</span>
        </div>

        <!-- QA 列表 -->
        <div v-loading="qaLoading" class="qa-list">
          <div v-for="qa in displayQAResults" :key="qa.index" class="qa-item">
            <div class="qa-header">
              <span class="qa-index">#{{ qa.index + 1 }}</span>
              <div class="qa-metrics">
                <el-tag v-if="qa.bleu !== undefined" size="small" type="info">
                  BLEU: {{ formatPercent(qa.bleu) }}
                </el-tag>
                <el-tag v-if="qa.rouge1 !== undefined" size="small" type="success">
                  ROUGE-1: {{ formatPercent(qa.rouge1) }}
                </el-tag>
                <el-tag v-if="qa.score !== undefined" size="small" :type="getScoreType(qa.score)">
                  得分: {{ formatPercent(qa.score) }}
                </el-tag>
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
          <el-pagination
            v-if="totalQAResults > pageSize"
            v-model:current-page="currentPage"
            v-model:page-size="pageSize"
            :total="totalQAResults"
            layout="prev, pager, next, sizes"
            :page-sizes="[10, 20, 50, 100]"
            @current-change="handlePageChange"
            @size-change="handleSizeChange"
            style="margin-top: 16px; justify-content: center"
          />
        </div>
      </el-card>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from '@/utils/element'
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
import { connectTaskProgress, type SSEClient } from '@/utils/sse'
import {
  EvaluationStatus,
  EvaluationStatusText,
  EvaluationStatusType,
  type EvaluationDetail,
  type QAResult,
  type SSEProgressEvent
} from '@/types'

type RetrievalMetrics = NonNullable<EvaluationDetail['metric']>['retrieval_metrics']
type GenerationMetrics = NonNullable<EvaluationDetail['metric']>['generation_metrics']

const route = useRoute()
const router = useRouter()
const taskId = computed(() => route.params.id as string || route.query.taskId as string)

const loading = ref(true)
const qaLoading = ref(false)
const detail = ref<EvaluationDetail | null>(null)
const qaResults = ref<QAResult[]>([])
const sseClient = ref<SSEClient | null>(null)

// 筛选和排序
const searchQuery = ref('')
const scoreRange = ref<[number, number]>([0, 100])
const sortBy = ref('index')
const sortOrder = ref<'asc' | 'desc'>('desc')
const currentPage = ref(1)
const pageSize = ref(20)
const totalQAResults = ref(0)

// 显示的检索指标
const displayRetrievalMetrics = computed(() => {
  if (!detail.value?.metric?.retrieval_metrics) return {}
  return detail.value.metric.retrieval_metrics
})

// 显示的生成指标
const displayGenerationMetrics = computed(() => {
  if (!detail.value?.metric?.generation_metrics) return {}
  return detail.value.metric.generation_metrics
})

// 指标标签映射
const retrievalLabels: Record<string, string> = {
  precision: '精确率',
  recall: '召回率',
  ndcg3: 'NDCG@3',
  ndcg10: 'NDCG@10',
  mrr: 'MRR',
  map: 'MAP'
}

const generationLabels: Record<string, string> = {
  bleu1: 'BLEU-1',
  bleu2: 'BLEU-2',
  bleu4: 'BLEU-4',
  rouge1: 'ROUGE-1',
  rouge2: 'ROUGE-2',
  rougeL: 'ROUGE-L'
}

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
    ElMessage.error(error.message || '加载详情失败')
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

function handlePageChange() {
  // 由 computed 处理
}

function handleSizeChange() {
  currentPage.value = 1
}

function exportResults() {
  if (!filteredQAResults.value.length) {
    ElMessage.warning('没有可导出的数据')
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

  ElMessage.success('导出成功')
}

onMounted(() => {
  loadDetail()
})

onUnmounted(() => {
  disconnectSSE()
})
</script>

<style scoped>
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

.filter-label {
  font-size: 13px;
  color: var(--color-text-secondary);
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
