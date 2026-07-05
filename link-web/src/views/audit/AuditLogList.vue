<template>
  <div class="audit-list-container">
    <div class="header">
      <div>
        <h2>审计日志</h2>
        <p class="subtitle">每一次请求的结构化审计记录（落库 MySQL）。用 request_id 可在 Grafana/Loki 中检索原始日志。</p>
      </div>
      <UiButton variant="secondary" icon @click="loadLogs">
        <template #icon><el-icon><Refresh /></el-icon></template>
        刷新
      </UiButton>
    </div>

    <!-- 概览统计 -->
    <div class="stats-row">
      <UiStatistic title="近 7 天总请求" :value="stats?.Total ?? 0" />
      <UiStatistic title="涉及模块" :value="moduleCount" />
      <UiStatistic title="当前筛选结果" :value="total" />
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
        v-model="filters.module"
        placeholder="模块（如 evaluation）"
        clearable
        style="width: 180px"
        @keyup.enter="handleSearch"
      />
      <UiSelect
        v-model="filters.status"
        placeholder="状态"
        clearable
        style="width: 140px"
        :options="[
          { label: '全部状态', value: '' },
          { label: '成功', value: 'success' },
          { label: '失败', value: 'error' }
        ]"
        @update:modelValue="handleSearch"
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
      <UiInput
        v-model="filters.keywords"
        placeholder="关键词（路径 / 资源名）"
        clearable
        style="width: 200px"
        @keyup.enter="handleSearch"
      />
      <UiButton variant="primary" @click="handleSearch">查询</UiButton>
      <UiButton variant="ghost" @click="resetFilters">重置</UiButton>
    </div>

    <!-- 日志列表 -->
    <div v-loading="loading">
      <UiTable
        :data="logs"
        border
        hover
        size="sm"
        row-key="ID"
        :columns="columns"
        empty="暂无审计日志"
        @row-click="viewDetail"
      >
        <template #cell-CreatedAt="{ row }">
          {{ formatTime(row.CreatedAt) }}
        </template>
        <template #cell-RequestID="{ row }">
          <span class="mono">{{ row.RequestID || '-' }}</span>
        </template>
        <template #cell-Status="{ row }">
          <UiTag size="sm" :variant="row.Status === 'success' ? 'success' : 'danger'">
            {{ row.Status === 'success' ? '成功' : '失败' }}
          </UiTag>
        </template>
        <template #cell-DurationMs="{ row }">
          {{ row.DurationMs != null ? `${row.DurationMs} ms` : '-' }}
        </template>
        <template #cell-UserID="{ row }">
          {{ row.UserID ?? '-' }}
        </template>
        <template #cell-IPAddress="{ row }">
          <span class="mono">{{ row.IPAddress || '-' }}</span>
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

    <!-- 详情抽屉 -->
    <UiDrawer v-model="showDetail" title="审计详情" :size="520">
      <div v-if="current" class="detail">
        <UiDescriptions :column="1" border>
          <UiDescriptionsItem label="ID">{{ current.ID }}</UiDescriptionsItem>
          <UiDescriptionsItem label="时间">{{ formatTime(current.CreatedAt) }}</UiDescriptionsItem>
          <UiDescriptionsItem label="Request ID">
            <span class="mono">{{ current.RequestID || '-' }}</span>
          </UiDescriptionsItem>
          <UiDescriptionsItem label="状态">
            <UiTag size="sm" :variant="current.Status === 'success' ? 'success' : 'danger'">
              {{ current.Status === 'success' ? '成功' : '失败' }}
            </UiTag>
          </UiDescriptionsItem>
          <UiDescriptionsItem label="模块">{{ current.Module || '-' }}</UiDescriptionsItem>
          <UiDescriptionsItem label="操作">
            <span class="mono">{{ current.Action || '-' }}</span>
          </UiDescriptionsItem>
          <UiDescriptionsItem label="资源类型">{{ current.ResourceType || '-' }}</UiDescriptionsItem>
          <UiDescriptionsItem label="资源">
            {{ current.ResourceName || current.ResourceID || '-' }}
          </UiDescriptionsItem>
          <UiDescriptionsItem label="耗时">
            {{ current.DurationMs != null ? `${current.DurationMs} ms` : '-' }}
          </UiDescriptionsItem>
          <UiDescriptionsItem label="用户 ID">{{ current.UserID ?? '-' }}</UiDescriptionsItem>
          <UiDescriptionsItem label="来源 IP">
            <span class="mono">{{ current.IPAddress || '-' }}</span>
          </UiDescriptionsItem>
        </UiDescriptions>

        <div v-if="current.ErrorMessage" class="detail-block">
          <h4>错误信息</h4>
          <pre class="error-msg">{{ current.ErrorMessage }}</pre>
        </div>

        <div class="detail-block">
          <h4>请求详情</h4>
          <pre class="details-json">{{ prettyDetails }}</pre>
        </div>

        <div v-if="current.RequestID" class="detail-block">
          <h4>关联原始日志</h4>
          <p class="hint">在 Grafana Explore（Loki 数据源）中用如下 LogQL 检索该请求在各服务的原始日志：</p>
          <pre class="logql">{{ `{job="link"} |= "${current.RequestID}"` }}</pre>
        </div>
      </div>

      <template #footer>
        <UiButton variant="secondary" @click="showDetail = false">关闭</UiButton>
      </template>
    </UiDrawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import {
  UiButton,
  UiInput,
  UiSelect,
  UiTable,
  UiTag,
  UiPagination,
  UiDrawer,
  UiDescriptions,
  UiDescriptionsItem,
  UiStatistic
} from '@/components'
import toast from '@/utils/toast'
import { auditApi, type AuditLog, type AuditStats } from '@/api/audit'

const columns = [
  { key: 'CreatedAt', title: '时间', width: 170 },
  { key: 'RequestID', title: 'Request ID', width: 180 },
  { key: 'Module', title: '模块', width: 120 },
  { key: 'Action', title: '操作' },
  { key: 'Status', title: '状态', width: 90 },
  { key: 'DurationMs', title: '耗时', width: 90, align: 'right' as const },
  { key: 'UserID', title: '用户', width: 70, align: 'right' as const },
  { key: 'IPAddress', title: '来源 IP', width: 130 }
]

const loading = ref(false)
const logs = ref<AuditLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

const stats = ref<AuditStats | null>(null)
const moduleCount = computed(() => Object.keys(stats.value?.ByModule ?? {}).length)

const filters = ref({
  request_id: '',
  module: '',
  status: '',
  keywords: ''
})
const timeRange = ref('')

const showDetail = ref(false)
const current = ref<AuditLog | null>(null)

const prettyDetails = computed(() => {
  const raw = current.value?.Details
  if (!raw) return '-'
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
})

function formatTime(iso: string): string {
  if (!iso) return '-'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
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

async function loadLogs() {
  loading.value = true
  try {
    const res = await auditApi.getAuditLogs({
      page: page.value,
      page_size: pageSize.value,
      request_id: filters.value.request_id.trim() || undefined,
      module: filters.value.module.trim() || undefined,
      status: filters.value.status || undefined,
      keywords: filters.value.keywords.trim() || undefined,
      start_time: computeStartTime()
    })
    if (res.data) {
      logs.value = res.data.list || []
      total.value = res.data.total || 0
    }
  } catch (error: any) {
    toast.error(error.message || '加载审计日志失败')
  } finally {
    loading.value = false
  }
}

async function loadStats() {
  try {
    const res = await auditApi.getAuditStats(7)
    if (res.data) stats.value = res.data
  } catch {
    // 统计失败不阻断列表
  }
}

function handleSearch() {
  page.value = 1
  loadLogs()
}

function resetFilters() {
  filters.value = { request_id: '', module: '', status: '', keywords: '' }
  timeRange.value = ''
  page.value = 1
  loadLogs()
}

function handlePageChange(p: number) {
  page.value = p
  loadLogs()
}

function viewDetail(row: AuditLog) {
  current.value = row
  showDetail.value = true
}

onMounted(() => {
  loadStats()
  loadLogs()
})
</script>

<style scoped>
.audit-list-container {
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

.stats-row {
  display: flex;
  gap: 32px;
  padding: 16px 20px;
  margin-bottom: 20px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
}

.filter-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
  margin-bottom: 20px;
}

.mono {
  font-family: var(--font-mono, ui-monospace, monospace);
  font-size: 12px;
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

.details-json,
.error-msg,
.logql {
  margin: 0;
  padding: 12px;
  background: var(--bg-tertiary, var(--bg-secondary));
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  font-family: var(--font-mono, ui-monospace, monospace);
  font-size: 12px;
  line-height: 1.6;
  color: var(--text-secondary);
  white-space: pre-wrap;
  word-break: break-all;
}

.error-msg {
  color: var(--danger);
}

.hint {
  margin: 0 0 8px;
  font-size: 12px;
  color: var(--text-muted);
}
</style>
