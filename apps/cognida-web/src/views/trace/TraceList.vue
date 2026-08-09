<template>
  <div class="trace-list-container">
    <div class="header">
      <div>
        <h2>调用链追踪</h2>
        <p class="subtitle">
          agent 执行、工具调用等落成 span 树。按 request_id / session_id / agent 检索，点击某条 trace 查看调用链瀑布图。
        </p>
      </div>
      <UiButton variant="secondary" icon @click="loadTraces">
        <template #icon><el-icon><Refresh /></el-icon></template>
        刷新
      </UiButton>
    </div>

    <!-- 筛选器 -->
    <div class="filter-bar">
      <UiInput
        v-model="filters.request_id"
        placeholder="Request ID"
        clearable
        style="width: 220px"
        @keyup.enter="handleSearch"
      />
      <UiInput
        v-model="filters.session_id"
        placeholder="Session ID"
        clearable
        style="width: 200px"
        @keyup.enter="handleSearch"
      />
      <UiInput
        v-model="filters.agent_name"
        placeholder="Agent 名称"
        clearable
        style="width: 180px"
        @keyup.enter="handleSearch"
      />
      <UiInput
        v-model="filters.trace_id"
        placeholder="Trace ID"
        clearable
        style="width: 200px"
        @keyup.enter="handleSearch"
      />
      <UiSelect
        v-model="timeRange"
        placeholder="时间范围"
        style="width: 160px"
        :options="[
          { label: '全部时间', value: '' },
          { label: '最近 1 小时', value: '1h' },
          { label: '最近 24 小时', value: '24h' },
          { label: '最近 7 天', value: '7d' }
        ]"
        @update:modelValue="handleSearch"
      />
      <label class="only-error">
        <UiSwitch v-model="filters.only_error" @update:modelValue="handleSearch" />
        <span>仅看出错</span>
      </label>
      <UiButton variant="primary" @click="handleSearch">查询</UiButton>
      <UiButton variant="ghost" @click="resetFilters">重置</UiButton>
    </div>

    <!-- trace 列表 -->
    <div v-loading="loading">
      <UiTable
        :data="traces"
        border
        hover
        size="sm"
        row-key="trace_id"
        :columns="columns"
        empty="暂无调用链数据"
        @row-click="viewDetail"
      >
        <template #cell-start_time="{ row }">
          {{ formatTime(row.start_time) }}
        </template>
        <template #cell-root_name="{ row }">
          <span class="root-name">{{ row.root_name || '-' }}</span>
        </template>
        <template #cell-trace_id="{ row }">
          <span class="mono">{{ shortId(row.trace_id) }}</span>
        </template>
        <template #cell-request_id="{ row }">
          <span class="mono">{{ row.request_id || '-' }}</span>
        </template>
        <template #cell-agent_name="{ row }">
          {{ row.agent_name || '-' }}
        </template>
        <template #cell-span_count="{ row }">
          {{ row.span_count }}
        </template>
        <template #cell-duration_ms="{ row }">
          {{ formatDuration(row.duration_ms) }}
        </template>
        <template #cell-has_error="{ row }">
          <UiTag size="sm" :variant="row.has_error ? 'danger' : 'success'">
            {{ row.has_error ? '错误' : '正常' }}
          </UiTag>
        </template>
      </UiTable>

      <UiPagination
        v-if="total > 0"
        :page="page"
        :page-size="pageSize"
        :total="total"
        show-info
        style="margin-top: 16px; justify-content: flex-end"
        @change="handlePageChange"
      />
    </div>

    <!-- 详情抽屉：span 树 -->
    <UiDrawer v-model="showDetail" title="调用链详情" :size="900">
      <div v-loading="detailLoading" class="detail">
        <template v-if="currentSummary">
          <UiDescriptions :column="2" border>
            <UiDescriptionsItem label="Trace ID">
              <span class="mono">{{ currentSummary.trace_id }}</span>
            </UiDescriptionsItem>
            <UiDescriptionsItem label="根节点">{{ currentSummary.root_name || '-' }}</UiDescriptionsItem>
            <UiDescriptionsItem label="Request ID">
              <span class="mono">{{ currentSummary.request_id || '-' }}</span>
            </UiDescriptionsItem>
            <UiDescriptionsItem label="Session ID">
              <span class="mono">{{ currentSummary.session_id || '-' }}</span>
            </UiDescriptionsItem>
            <UiDescriptionsItem label="Agent">{{ currentSummary.agent_name || '-' }}</UiDescriptionsItem>
            <UiDescriptionsItem label="Span 数">{{ currentSummary.span_count }}</UiDescriptionsItem>
            <UiDescriptionsItem label="开始时间">{{ formatTime(currentSummary.start_time) }}</UiDescriptionsItem>
            <UiDescriptionsItem label="总耗时">{{ formatDuration(currentSummary.duration_ms) }}</UiDescriptionsItem>
            <UiDescriptionsItem label="状态">
              <UiTag size="sm" :variant="currentSummary.has_error ? 'danger' : 'success'">
                {{ currentSummary.has_error ? '错误' : '正常' }}
              </UiTag>
            </UiDescriptionsItem>
          </UiDescriptions>
        </template>

        <div class="detail-block">
          <h4>调用链瀑布图</h4>
          <p class="hint">点击任意 span 展开子节点与其 attributes / events 详情。</p>
          <SpanTree v-if="detail" :tree="detail.tree" />
        </div>
      </div>

      <template #footer>
        <UiButton variant="secondary" @click="showDetail = false">关闭</UiButton>
      </template>
    </UiDrawer>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Refresh } from '@element-plus/icons-vue'
import {
  UiButton,
  UiInput,
  UiSelect,
  UiSwitch,
  UiTable,
  UiTag,
  UiPagination,
  UiDrawer,
  UiDescriptions,
  UiDescriptionsItem
} from '@/components'
import toast from '@/utils/toast'
import SpanTree from './SpanTree.vue'
import { traceApi, type TraceSummary, type TraceDetail } from '@/api/trace'

const route = useRoute()
const router = useRouter()

const columns = [
  { key: 'start_time', title: '开始时间', width: 170 },
  { key: 'root_name', title: '根节点' },
  { key: 'trace_id', title: 'Trace ID', width: 130 },
  { key: 'request_id', title: 'Request ID', width: 180 },
  { key: 'agent_name', title: 'Agent', width: 130 },
  { key: 'span_count', title: 'Span', width: 70, align: 'right' as const },
  { key: 'duration_ms', title: '耗时', width: 90, align: 'right' as const },
  { key: 'has_error', title: '状态', width: 80 }
]

const loading = ref(false)
const traces = ref<TraceSummary[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

const filters = ref({
  request_id: '',
  session_id: '',
  agent_name: '',
  trace_id: '',
  only_error: false
})
const timeRange = ref('')

const showDetail = ref(false)
const detailLoading = ref(false)
const currentSummary = ref<TraceSummary | null>(null)
const detail = ref<TraceDetail | null>(null)

function shortId(id: string): string {
  if (!id) return '-'
  return id.length > 16 ? `${id.slice(0, 8)}…${id.slice(-4)}` : id
}

function formatTime(iso: string): string {
  if (!iso) return '-'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

function formatDuration(ms: number): string {
  if (ms == null) return '-'
  if (ms < 1000) return `${Math.round(ms)} ms`
  return `${(ms / 1000).toFixed(2)} s`
}

function computeStartTime(): string | undefined {
  if (!timeRange.value) return undefined
  const now = Date.now()
  const deltas: Record<string, number> = {
    '1h': 3600_000,
    '24h': 24 * 3600_000,
    '7d': 7 * 24 * 3600_000
  }
  const delta = deltas[timeRange.value]
  if (!delta) return undefined
  return new Date(now - delta).toISOString()
}

async function loadTraces() {
  loading.value = true
  try {
    const res = await traceApi.listTraces({
      page: page.value,
      page_size: pageSize.value,
      request_id: filters.value.request_id.trim() || undefined,
      session_id: filters.value.session_id.trim() || undefined,
      agent_name: filters.value.agent_name.trim() || undefined,
      trace_id: filters.value.trace_id.trim() || undefined,
      only_error: filters.value.only_error || undefined,
      start_time: computeStartTime()
    })
    if (res.data) {
      traces.value = res.data.list || []
      total.value = res.data.total || 0
    }
  } catch (error: any) {
    toast.error(error.message || '加载调用链失败')
  } finally {
    loading.value = false
  }
}

function handleSearch() {
  page.value = 1
  loadTraces()
}

function resetFilters() {
  filters.value = { request_id: '', session_id: '', agent_name: '', trace_id: '', only_error: false }
  timeRange.value = ''
  page.value = 1
  // 清掉 URL 上的 query，避免刷新后又被带回
  if (Object.keys(route.query).length) {
    router.replace({ query: {} })
  }
  loadTraces()
}

function handlePageChange(p: number) {
  page.value = p
  loadTraces()
}

async function viewDetail(row: TraceSummary) {
  currentSummary.value = row
  detail.value = null
  showDetail.value = true
  detailLoading.value = true
  try {
    const res = await traceApi.getTrace(row.trace_id)
    if (res.data) detail.value = res.data
  } catch (error: any) {
    toast.error(error.message || '加载调用链详情失败')
  } finally {
    detailLoading.value = false
  }
}

// 从审计页跳转过来时，query 里带 request_id / trace_id，自动回填并过滤。
function applyQuery() {
  const q = route.query
  if (typeof q.request_id === 'string') filters.value.request_id = q.request_id
  if (typeof q.session_id === 'string') filters.value.session_id = q.session_id
  if (typeof q.agent_name === 'string') filters.value.agent_name = q.agent_name
  if (typeof q.trace_id === 'string') filters.value.trace_id = q.trace_id
  if (q.only_error === 'true') filters.value.only_error = true
}

onMounted(() => {
  applyQuery()
  loadTraces()
})
</script>

<style scoped>
.trace-list-container {
  padding: 24px;
  background: var(--bg-primary);
  border-radius: 8px;
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 20px;
}

.header h2 {
  font-size: 20px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
}

.subtitle {
  margin: 6px 0 0;
  font-size: 13px;
  color: var(--text-muted);
}

.filter-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
  margin-bottom: 20px;
}

.only-error {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text-secondary);
  cursor: pointer;
}

.mono {
  font-family: var(--font-mono, ui-monospace, monospace);
  font-size: 12px;
}

.root-name {
  color: var(--text-primary);
  font-weight: 500;
}

.detail-block {
  margin-top: 20px;
}

.detail-block h4 {
  margin: 0 0 8px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.hint {
  margin: 0 0 12px;
  font-size: 12px;
  color: var(--text-muted);
}
</style>
