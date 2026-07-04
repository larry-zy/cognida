<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { qualityApi } from '@/api/quality'
import type {
  StructuredReport,
  UnstructuredReport,
  CleaningReport,
  DimensionInfo,
  CheckRecordItem,
  CheckRecordDetail,
  QualityCheckType
} from '@/api/quality'
import UiPageHeader from '@/components/layout/UiPageHeader.vue'
import UiTabs from '@/components/ui/UiTabs.vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiTextarea from '@/components/ui/UiTextarea.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiTag from '@/components/ui/UiTag.vue'
import UiEmpty from '@/components/ui/UiEmpty.vue'
import UiStatistic from '@/components/ui/UiStatistic.vue'
import UiProgress from '@/components/ui/UiProgress.vue'

// ==================== Tab 状态 ====================
const activeTab = ref<'structured' | 'unstructured' | 'cleaning' | 'history'>('structured')
const tabs = [
  { key: 'structured', label: '结构化评估' },
  { key: 'unstructured', label: '非结构化文本' },
  { key: 'cleaning', label: '数据清洗' },
  { key: 'history', label: '历史记录' }
]

// ==================== 维度元数据 ====================
const dimensions = ref<DimensionInfo[]>([])

onMounted(async () => {
  try {
    const res = await qualityApi.listDimensions()
    dimensions.value = res.data?.dimensions ?? []
  } catch (e) {
    // 维度列表失败不阻塞主流程
  }
  loadRecords()
})

// ==================== 通用工具 ====================
function scoreVariant(score: number): 'success' | 'warning' | 'danger' {
  if (score >= 0.8) return 'success'
  if (score >= 0.6) return 'warning'
  return 'danger'
}
function severityVariant(sev: string): 'danger' | 'warning' | 'info' {
  const s = (sev || '').toLowerCase()
  if (s === 'high' || s === 'critical' || s === 'error') return 'danger'
  if (s === 'medium' || s === 'warn' || s === 'warning') return 'warning'
  return 'info'
}
function pct(score: number): number {
  return Math.round((score || 0) * 100)
}

// ==================== 结构化评估 ====================
const structuredInput = ref('')
const structuredSource = ref('')
const structuredLoading = ref(false)
const structuredReport = ref<StructuredReport | null>(null)
const structuredError = ref('')

async function runStructured() {
  if (!structuredInput.value.trim()) {
    structuredError.value = '请粘贴 CSV 数据'
    return
  }
  structuredError.value = ''
  structuredLoading.value = true
  structuredReport.value = null
  try {
    const res = await qualityApi.evaluateStructured({
      csv_data: structuredInput.value,
      source_name: structuredSource.value || '粘贴数据'
    })
    structuredReport.value = res.data ?? null
    loadRecords()
  } catch (e: any) {
    structuredError.value = e?.message || '评估失败'
  } finally {
    structuredLoading.value = false
  }
}

function onStructuredFile(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    structuredInput.value = String(reader.result || '')
    if (!structuredSource.value) structuredSource.value = file.name
  }
  reader.readAsText(file)
}

const issueColumns = [
  { key: 'severity', title: '级别', width: '90px' },
  { key: 'dimension', title: '维度', width: '120px' },
  { key: 'field', title: '字段', width: '140px' },
  { key: 'description', title: '描述' },
  { key: 'count', title: '数量', width: '80px', align: 'right' as const }
]

// ==================== 非结构化文本 ====================
const textInput = ref('')
const textSource = ref('')
const textLoading = ref(false)
const textReport = ref<UnstructuredReport | null>(null)
const textError = ref('')

async function runUnstructured() {
  if (!textInput.value.trim()) {
    textError.value = '请粘贴待检测文本'
    return
  }
  textError.value = ''
  textLoading.value = true
  textReport.value = null
  try {
    const res = await qualityApi.evaluateUnstructured({
      text: textInput.value,
      source_name: textSource.value || '粘贴文本'
    })
    textReport.value = res.data ?? null
    loadRecords()
  } catch (e: any) {
    textError.value = e?.message || '评估失败'
  } finally {
    textLoading.value = false
  }
}

const textIssueColumns = [
  { key: 'severity', title: '级别', width: '90px' },
  { key: 'type', title: '类型', width: '120px' },
  { key: 'description', title: '描述' },
  { key: 'suggestion', title: '建议' }
]

// ==================== 数据清洗 ====================
const cleanInput = ref('')
const cleanSource = ref('')
const cleanLoading = ref(false)
const cleanReport = ref<CleaningReport | null>(null)
const cleanError = ref('')
const CLEANERS = [
  { key: 'trim', label: '去除首尾空白' },
  { key: 'dedup', label: '去重' },
  { key: 'drop_empty', label: '删除空行' },
  { key: 'normalize_ws', label: '规整空白字符' }
]
const selectedCleaners = ref<string[]>(['trim', 'dedup', 'drop_empty'])

function toggleCleaner(key: string) {
  const i = selectedCleaners.value.indexOf(key)
  if (i >= 0) selectedCleaners.value.splice(i, 1)
  else selectedCleaners.value.push(key)
}

async function runClean() {
  if (!cleanInput.value.trim()) {
    cleanError.value = '请粘贴 CSV 数据'
    return
  }
  cleanError.value = ''
  cleanLoading.value = true
  cleanReport.value = null
  try {
    const res = await qualityApi.clean(
      { csv_data: cleanInput.value, source_name: cleanSource.value || '粘贴数据' },
      selectedCleaners.value
    )
    cleanReport.value = res.data ?? null
    loadRecords()
  } catch (e: any) {
    cleanError.value = e?.message || '清洗失败'
  } finally {
    cleanLoading.value = false
  }
}

function onCleanFile(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    cleanInput.value = String(reader.result || '')
    if (!cleanSource.value) cleanSource.value = file.name
  }
  reader.readAsText(file)
}

function downloadCleaned() {
  if (!cleanReport.value?.cleaned_csv) return
  const blob = new Blob([cleanReport.value.cleaned_csv], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `cleaned_${cleanSource.value || 'data'}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

const opColumns = [
  { key: 'type', title: '操作', width: '140px' },
  { key: 'field', title: '字段', width: '140px' },
  { key: 'count', title: '影响行数', width: '100px', align: 'right' as const },
  { key: 'description', title: '说明' }
]

// ==================== 历史记录 ====================
const records = ref<CheckRecordItem[]>([])
const recordsTotal = ref(0)
const recordsLoading = ref(false)
const page = ref(1)
const pageSize = ref(10)
const filterType = ref<QualityCheckType | ''>('')
const detail = ref<CheckRecordDetail | null>(null)
const detailLoading = ref(false)

const typeLabels: Record<QualityCheckType, string> = {
  structured: '结构化评估',
  unstructured: '文本质量',
  cleaning: '数据清洗'
}

async function loadRecords() {
  recordsLoading.value = true
  try {
    const res = await qualityApi.listRecords({
      page: page.value,
      page_size: pageSize.value,
      type: filterType.value || undefined as any
    })
    records.value = res.data?.items ?? []
    recordsTotal.value = res.data?.total ?? 0
  } catch (e) {
    records.value = []
    recordsTotal.value = 0
  } finally {
    recordsLoading.value = false
  }
}

function onFilterType(t: QualityCheckType | '') {
  filterType.value = t
  page.value = 1
  loadRecords()
}

async function viewDetail(row: CheckRecordItem) {
  detailLoading.value = true
  detail.value = null
  try {
    const res = await qualityApi.getRecord(row.id)
    detail.value = res.data ?? null
  } finally {
    detailLoading.value = false
  }
}

async function removeRecord(row: CheckRecordItem) {
  await qualityApi.deleteRecord(row.id)
  if (detail.value?.id === row.id) detail.value = null
  loadRecords()
}

function onPageChange(p: number) {
  page.value = p
  loadRecords()
}

const recordColumns = [
  { key: 'type', title: '类型', width: '120px' },
  { key: 'source_name', title: '来源' },
  { key: 'overall_score', title: '综合得分', width: '110px', align: 'center' as const },
  { key: 'record_count', title: '记录数', width: '90px', align: 'right' as const },
  { key: 'created_at', title: '时间', width: '170px' },
  { key: 'actions', title: '操作', width: '140px', align: 'center' as const }
]

function fmtTime(s: string): string {
  if (!s) return '-'
  const d = new Date(s)
  if (isNaN(d.getTime())) return s
  return d.toLocaleString('zh-CN', { hour12: false })
}
</script>

<template>
  <div class="quality-center">
    <UiPageHeader
      title="数据质量管理中心"
      subtitle="评估结构化数据与文本质量、清洗脏数据，并保留质检历史"
      divider
    />

    <UiTabs v-model="activeTab" :tabs="tabs" type="line" class="q-tabs">
      <!-- ========== 结构化评估 ========== -->
      <template #structured>
        <div class="tab-body">
          <UiCard title="输入数据" variant="bordered">
            <div class="input-row">
              <input type="text" class="q-input" v-model="structuredSource" placeholder="来源名称（可选）" />
              <label class="file-btn">
                上传 CSV
                <input type="file" accept=".csv,text/csv" hidden @change="onStructuredFile" />
              </label>
            </div>
            <UiTextarea
              v-model="structuredInput"
              placeholder="粘贴 CSV 数据，首行为表头，例如：&#10;name,age,email&#10;张三,28,zhang@x.com"
              :rows="8"
            />
            <div v-if="structuredError" class="err">{{ structuredError }}</div>
            <template #footer>
              <UiButton variant="primary" :loading="structuredLoading" @click="runStructured">开始评估</UiButton>
            </template>
          </UiCard>

          <UiCard v-if="structuredReport" title="评估结果" variant="bordered" class="result-card">
            <div class="stat-grid">
              <UiStatistic title="综合得分" :value="pct(structuredReport.overall_score)" suffix="分" />
              <UiStatistic title="记录数" :value="structuredReport.record_count" />
              <UiStatistic title="问题数" :value="structuredReport.issues?.length || 0" />
            </div>

            <div class="dim-list">
              <div v-for="d in structuredReport.dimensions" :key="d.name" class="dim-item">
                <div class="dim-head">
                  <span class="dim-name">{{ d.name }}</span>
                  <UiTag :variant="d.passed ? 'success' : 'danger'" size="sm">
                    {{ d.passed ? '通过' : '未通过' }}
                  </UiTag>
                </div>
                <UiProgress :percentage="pct(d.score)" :status="scoreVariant(d.score)" />
              </div>
            </div>

            <h4 class="sub-title">质量问题</h4>
            <UiTable :columns="issueColumns" :data="structuredReport.issues || []">
              <template #cell-severity="{ row }">
                <UiTag :variant="severityVariant(row.severity)" size="sm">{{ row.severity }}</UiTag>
              </template>
              <template #empty><UiEmpty title="未发现问题" description="数据质量良好" /></template>
            </UiTable>
          </UiCard>
        </div>
      </template>

      <!-- ========== 非结构化文本 ========== -->
      <template #unstructured>
        <div class="tab-body">
          <UiCard title="输入文本" variant="bordered">
            <div class="input-row">
              <input type="text" class="q-input" v-model="textSource" placeholder="来源名称（可选）" />
            </div>
            <UiTextarea
              v-model="textInput"
              placeholder="粘贴待检测的文本内容……"
              :rows="10"
            />
            <div v-if="textError" class="err">{{ textError }}</div>
            <template #footer>
              <UiButton variant="primary" :loading="textLoading" @click="runUnstructured">开始评估</UiButton>
            </template>
          </UiCard>

          <UiCard v-if="textReport" title="评估结果" variant="bordered" class="result-card">
            <div class="stat-grid">
              <UiStatistic title="综合得分" :value="pct(textReport.overall_score)" suffix="分" />
              <UiStatistic
                v-for="(v, k) in (textReport.text_stats || {})"
                :key="k"
                :title="String(k)"
                :value="Number(v) || v"
              />
            </div>

            <div class="dim-list">
              <div v-for="d in textReport.dimensions" :key="d.name" class="dim-item">
                <div class="dim-head">
                  <span class="dim-name">{{ d.name }}</span>
                  <UiTag :variant="d.passed ? 'success' : 'danger'" size="sm">
                    {{ d.passed ? '通过' : '未通过' }}
                  </UiTag>
                </div>
                <UiProgress :percentage="pct(d.score)" />
              </div>
            </div>

            <h4 class="sub-title">质量问题</h4>
            <UiTable :columns="textIssueColumns" :data="textReport.issues || []">
              <template #cell-severity="{ row }">
                <UiTag :variant="severityVariant(row.severity)" size="sm">{{ row.severity }}</UiTag>
              </template>
              <template #empty><UiEmpty title="未发现问题" description="文本质量良好" /></template>
            </UiTable>
          </UiCard>
        </div>
      </template>

      <!-- ========== 数据清洗 ========== -->
      <template #cleaning>
        <div class="tab-body">
          <UiCard title="输入数据" variant="bordered">
            <div class="input-row">
              <input type="text" class="q-input" v-model="cleanSource" placeholder="来源名称（可选）" />
              <label class="file-btn">
                上传 CSV
                <input type="file" accept=".csv,text/csv" hidden @change="onCleanFile" />
              </label>
            </div>
            <div class="cleaner-picker">
              <span
                v-for="c in CLEANERS"
                :key="c.key"
                class="cleaner-chip"
                :class="{ active: selectedCleaners.includes(c.key) }"
                @click="toggleCleaner(c.key)"
              >{{ c.label }}</span>
            </div>
            <UiTextarea
              v-model="cleanInput"
              placeholder="粘贴 CSV 数据，首行为表头"
              :rows="8"
            />
            <div v-if="cleanError" class="err">{{ cleanError }}</div>
            <template #footer>
              <UiButton variant="primary" :loading="cleanLoading" @click="runClean">开始清洗</UiButton>
            </template>
          </UiCard>

          <UiCard v-if="cleanReport" title="清洗结果" variant="bordered" class="result-card">
            <div class="stat-grid">
              <UiStatistic title="原始行数" :value="cleanReport.original_count" />
              <UiStatistic title="清洗后行数" :value="cleanReport.cleaned_count" />
              <UiStatistic title="移除行数" :value="cleanReport.removed_count" />
            </div>

            <h4 class="sub-title">清洗操作</h4>
            <UiTable :columns="opColumns" :data="cleanReport.operations || []">
              <template #empty><UiEmpty title="无操作" description="没有需要清洗的内容" /></template>
            </UiTable>

            <div v-if="cleanReport.cleaned_csv" class="download-row">
              <UiButton variant="secondary" @click="downloadCleaned">下载清洗后 CSV</UiButton>
            </div>
          </UiCard>
        </div>
      </template>

      <!-- ========== 历史记录 ========== -->
      <template #history>
        <div class="tab-body">
          <UiCard title="质检历史" variant="bordered">
            <div class="filter-row">
              <span
                class="cleaner-chip"
                :class="{ active: filterType === '' }"
                @click="onFilterType('')"
              >全部</span>
              <span
                v-for="(label, key) in typeLabels"
                :key="key"
                class="cleaner-chip"
                :class="{ active: filterType === key }"
                @click="onFilterType(key as QualityCheckType)"
              >{{ label }}</span>
            </div>

            <UiTable
              :columns="recordColumns"
              :data="records"
              show-pagination
              :pagination="{ total: recordsTotal, page, pageSize }"
              @page-change="onPageChange"
            >
              <template #cell-type="{ row }">
                <UiTag variant="info" size="sm">{{ typeLabels[row.type as QualityCheckType] || row.type }}</UiTag>
              </template>
              <template #cell-overall_score="{ row }">
                <UiTag v-if="row.type !== 'cleaning'" :variant="scoreVariant(row.overall_score)" size="sm">
                  {{ pct(row.overall_score) }}
                </UiTag>
                <span v-else class="muted">—</span>
              </template>
              <template #cell-created_at="{ row }">{{ fmtTime(row.created_at) }}</template>
              <template #cell-actions="{ row }">
                <div class="row-actions">
                  <UiButton variant="ghost" size="sm" @click="viewDetail(row)">查看</UiButton>
                  <UiButton variant="ghost" size="sm" @click="removeRecord(row)">删除</UiButton>
                </div>
              </template>
              <template #empty><UiEmpty title="暂无记录" description="完成一次质检后会出现在这里" /></template>
            </UiTable>
          </UiCard>

          <UiCard v-if="detail" :title="`记录详情 · ${detail.source_name}`" variant="bordered" class="result-card">
            <div class="detail-summary">{{ detail.summary || '无摘要' }}</div>
            <pre class="report-json">{{ JSON.stringify(detail.report, null, 2) }}</pre>
          </UiCard>
        </div>
      </template>
    </UiTabs>
  </div>
</template>

<style scoped>
.quality-center {
  padding: var(--space-6, 24px);
  max-width: 1100px;
  margin: 0 auto;
}
.q-tabs {
  margin-top: var(--space-4, 16px);
}
.tab-body {
  display: flex;
  flex-direction: column;
  gap: var(--space-5, 20px);
  padding-top: var(--space-4, 16px);
}
.input-row {
  display: flex;
  gap: var(--space-3, 12px);
  align-items: center;
  margin-bottom: var(--space-3, 12px);
}
.q-input {
  flex: 1;
  height: 36px;
  padding: 0 var(--space-3, 12px);
  background: var(--bg-elevated, #1a1d24);
  border: 1px solid var(--border-default, #2a2d35);
  border-radius: var(--radius-md, 8px);
  color: var(--text-primary, #e6e8eb);
  font-size: 14px;
  outline: none;
}
.q-input:focus {
  border-color: var(--primary, #6366f1);
}
.file-btn {
  display: inline-flex;
  align-items: center;
  height: 36px;
  padding: 0 var(--space-4, 16px);
  background: var(--bg-elevated, #1a1d24);
  border: 1px solid var(--border-default, #2a2d35);
  border-radius: var(--radius-md, 8px);
  color: var(--text-secondary, #9ca3af);
  font-size: 14px;
  cursor: pointer;
  white-space: nowrap;
}
.file-btn:hover {
  border-color: var(--primary, #6366f1);
  color: var(--text-primary, #e6e8eb);
}
.err {
  margin-top: var(--space-2, 8px);
  color: var(--danger, #ef4444);
  font-size: 13px;
}
.result-card {
  margin-top: 0;
}
.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: var(--space-4, 16px);
  margin-bottom: var(--space-5, 20px);
}
.dim-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-4, 16px);
  margin-bottom: var(--space-5, 20px);
}
.dim-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-2, 8px);
}
.dim-name {
  color: var(--text-primary, #e6e8eb);
  font-size: 14px;
  font-weight: 500;
}
.sub-title {
  margin: var(--space-5, 20px) 0 var(--space-3, 12px);
  color: var(--text-secondary, #9ca3af);
  font-size: 13px;
  font-weight: 600;
}
.cleaner-picker,
.filter-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2, 8px);
  margin-bottom: var(--space-3, 12px);
}
.cleaner-chip {
  padding: 4px 12px;
  border-radius: var(--radius-md, 8px);
  border: 1px solid var(--border-default, #2a2d35);
  color: var(--text-secondary, #9ca3af);
  font-size: 13px;
  cursor: pointer;
  user-select: none;
  transition: all 0.15s;
}
.cleaner-chip:hover {
  border-color: var(--primary, #6366f1);
}
.cleaner-chip.active {
  background: var(--primary, #6366f1);
  border-color: var(--primary, #6366f1);
  color: #fff;
}
.download-row {
  margin-top: var(--space-4, 16px);
}
.row-actions {
  display: flex;
  gap: var(--space-2, 8px);
  justify-content: center;
}
.muted {
  color: var(--text-muted, #6b7280);
}
.detail-summary {
  color: var(--text-secondary, #9ca3af);
  font-size: 14px;
  margin-bottom: var(--space-3, 12px);
}
.report-json {
  background: var(--bg-elevated, #1a1d24);
  border: 1px solid var(--border-default, #2a2d35);
  border-radius: var(--radius-md, 8px);
  padding: var(--space-4, 16px);
  color: var(--text-secondary, #9ca3af);
  font-size: 12px;
  line-height: 1.6;
  max-height: 400px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
