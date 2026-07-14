<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { qualityApi } from '@/api/quality'
import toast from '@/utils/toast'
import { datasourceApi } from '@/api/datasource'
import type { Datasource } from '@/api/datasource'
import { useAsyncTask } from '@/composables/useAsyncTask'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiInputNumber from '@/components/ui/UiInputNumber.vue'
import UiAsyncStatus from '@/components/ui/UiAsyncStatus.vue'
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
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiTextarea from '@/components/ui/UiTextarea.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiTag from '@/components/ui/UiTag.vue'
import UiEmpty from '@/components/ui/UiEmpty.vue'
import UiStatistic from '@/components/ui/UiStatistic.vue'
import UiProgress from '@/components/ui/UiProgress.vue'

// ==================== Tab 状态 ====================
const activeTab = ref<'structured' | 'datasource' | 'unstructured' | 'cleaning' | 'history'>('structured')
const tabs = [
  { key: 'structured', label: '结构化评估' },
  { key: 'datasource', label: '数据源直评' },
  { key: 'unstructured', label: '非结构化文本' },
  { key: 'cleaning', label: '数据清洗' },
  { key: 'history', label: '历史记录' }
]

// ==================== 维度元数据 ====================
const dimensions = ref<DimensionInfo[]>([])

// 结构化评测可选维度（仅 supports_structured）与用户勾选状态；默认全选。
const structuredDimLabels: Record<string, string> = {
  completeness: '完整性',
  accuracy: '准确性',
  consistency: '一致性',
  validity: '有效性',
  uniqueness: '唯一性',
  timeliness: '时效性'
}
const structuredDimOptions = computed(() => dimensions.value.filter((d) => d.supports_structured))
const selectedStructuredDims = ref<string[]>([])
function dimLabel(name: string): string {
  return structuredDimLabels[name] || name
}
function toggleStructuredDim(name: string) {
  const i = selectedStructuredDims.value.indexOf(name)
  if (i >= 0) selectedStructuredDims.value.splice(i, 1)
  else selectedStructuredDims.value.push(name)
}

onMounted(async () => {
  try {
    const res = await qualityApi.listDimensions()
    dimensions.value = res.data?.dimensions ?? []
    // 默认全选结构化维度
    selectedStructuredDims.value = structuredDimOptions.value.map((d) => d.name)
  } catch (e) {
    // 维度列表失败不阻塞主流程
  }
  loadDatasources()
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
const structuredFormat = ref<'csv' | 'json'>('csv')
const structuredTask = useAsyncTask<StructuredReport | null>()
const structuredReport = ref<StructuredReport | null>(null)
const structuredError = ref('')

const structuredPlaceholder = computed(() =>
  structuredFormat.value === 'json'
    ? '粘贴 JSON 对象数组，例如：\n[\n  {"id": 1, "name": "Alice", "age": 30},\n  {"id": 2, "name": "Bob", "age": 25}\n]'
    : '粘贴 CSV 数据，第一行为表头，例如：\nid,name,age\n1,Alice,30\n2,Bob,25'
)

async function runStructured() {
  if (!structuredInput.value.trim()) {
    structuredError.value = structuredFormat.value === 'json' ? '请粘贴 JSON 数据' : '请粘贴 CSV 数据'
    return
  }
  if (structuredDimOptions.value.length && !selectedStructuredDims.value.length) {
    structuredError.value = '请至少选择一个评测维度'
    return
  }
  structuredError.value = ''
  structuredReport.value = null
  // 选满（或维度列表未加载）时省略 dimensions，交由后端跑默认全集。
  const dims =
    selectedStructuredDims.value.length &&
    selectedStructuredDims.value.length < structuredDimOptions.value.length
      ? [...selectedStructuredDims.value]
      : undefined
  await structuredTask.run(
    async () => {
      const res = await qualityApi.evaluateStructured({
        csv_data: structuredInput.value,
        source_name: structuredSource.value || '粘贴数据',
        format: structuredFormat.value,
        dimensions: dims
      })
      structuredReport.value = res.data ?? null
      loadRecords()
      return res.data ?? null
    },
    {
      pendingMessage: '评估中…',
      successMessage: (r) =>
        r
          ? `评估完成：综合得分 ${pct(r.overall_score)} 分，${r.record_count} 条记录，${r.issues?.length || 0} 个问题`
          : '评估完成',
      errorMessage: (e) => e?.message || '评估失败'
    }
  )
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

// ==================== 数据源直评 ====================
const dsList = ref<Datasource[]>([])
const dsId = ref('')
const dsTables = ref<string[]>([])
const dsTable = ref('')
const dsSampleSize = ref(200)
const dsTask = useAsyncTask<StructuredReport | null>()
const dsTablesLoading = ref(false)
const dsReport = ref<StructuredReport | null>(null)
const dsError = ref('')

const dsOptions = computed(() =>
  dsList.value.map((d) => ({ label: `${d.name}（${d.type}）`, value: d.id }))
)
const dsTableOptions = computed(() =>
  dsTables.value.map((t) => ({ label: t, value: t }))
)

async function loadDatasources() {
  try {
    const res = await datasourceApi.list({ page: 1, page_size: 100 })
    dsList.value = res.data?.items ?? []
  } catch (e) {
    // 列表失败不阻塞
  }
}

async function onDatasourceChange() {
  dsTable.value = ''
  dsTables.value = []
  dsReport.value = null
  if (!dsId.value) return
  dsTablesLoading.value = true
  try {
    const res = await datasourceApi.getTables(dsId.value)
    dsTables.value = res.data?.tables ?? []
  } catch (e: any) {
    dsError.value = e?.message || '获取表清单失败'
  } finally {
    dsTablesLoading.value = false
  }
}

async function runDatasource() {
  if (!dsId.value) {
    dsError.value = '请选择数据源'
    return
  }
  if (!dsTable.value) {
    dsError.value = '请选择数据表'
    return
  }
  dsError.value = ''
  dsReport.value = null
  await dsTask.run(
    async () => {
      const res = await qualityApi.evaluateDatasource({
        datasource_id: dsId.value,
        table: dsTable.value,
        sample_size: dsSampleSize.value
      })
      dsReport.value = res.data ?? null
      loadRecords()
      return res.data ?? null
    },
    {
      pendingMessage: '抽样评估中…',
      successMessage: (r) =>
        r
          ? `评估完成：综合得分 ${pct(r.overall_score)} 分，抽样 ${r.record_count} 行，${r.issues?.length || 0} 个问题`
          : '评估完成',
      errorMessage: (e) => e?.message || '评估失败'
    }
  )
}

// ==================== 非结构化文本 ====================
const textInput = ref('')
const textSource = ref('')
const textTask = useAsyncTask<UnstructuredReport | null>()
const textReport = ref<UnstructuredReport | null>(null)
const textError = ref('')

async function runUnstructured() {
  if (!textInput.value.trim()) {
    textError.value = '请粘贴待检测文本'
    return
  }
  textError.value = ''
  textReport.value = null
  await textTask.run(
    async () => {
      const res = await qualityApi.evaluateUnstructured({
        text: textInput.value,
        source_name: textSource.value || '粘贴文本'
      })
      textReport.value = res.data ?? null
      loadRecords()
      return res.data ?? null
    },
    {
      pendingMessage: '评估中…',
      successMessage: (r) =>
        r
          ? `评估完成：综合得分 ${pct(r.overall_score)} 分，${r.issues?.length || 0} 个问题`
          : '评估完成',
      errorMessage: (e) => e?.message || '评估失败'
    }
  )
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
const cleanFormat = ref<'csv' | 'json'>('csv')
const cleanOutputFormat = ref<'csv' | 'json'>('csv')
const cleanTask = useAsyncTask<CleaningReport | null>()
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
    cleanError.value = cleanFormat.value === 'json' ? '请粘贴 JSON 数据' : '请粘贴 CSV 数据'
    return
  }
  cleanError.value = ''
  cleanReport.value = null
  await cleanTask.run(
    async () => {
      const res = await qualityApi.clean(
        {
          csv_data: cleanInput.value,
          source_name: cleanSource.value || '粘贴数据',
          format: cleanFormat.value
        },
        selectedCleaners.value,
        cleanOutputFormat.value
      )
      cleanReport.value = res.data ?? null
      loadRecords()
      return res.data ?? null
    },
    {
      pendingMessage: '清洗中…',
      successMessage: (r) =>
        r
          ? `清洗完成：原始 ${r.original_count} 行 → 清洗后 ${r.cleaned_count} 行，移除 ${r.removed_count} 行`
          : '清洗完成',
      errorMessage: (e) => e?.message || '清洗失败'
    }
  )
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
  const fmt = cleanReport.value.cleaned_format || 'csv'
  const mime = fmt === 'json' ? 'application/json;charset=utf-8' : 'text/csv;charset=utf-8'
  const blob = new Blob([cleanReport.value.cleaned_csv], { type: mime })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `cleaned_${cleanSource.value || 'data'}.${fmt}`
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
// 详情以居中弹窗展示：避免详情卡片渲染在长表格下方、超出视口而“点了没反应”。
const detailVisible = ref(false)

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
  // 先打开弹窗给出即时反馈（加载态在弹窗内展示），再拉详情。
  detailVisible.value = true
  try {
    const res = await qualityApi.getRecord(row.id)
    if (!res.data) {
      toast.error('记录详情为空或未返回')
      detailVisible.value = false
      return
    }
    detail.value = res.data
  } catch (e: any) {
    // 不吞错误：请求失败（如代理 502）时给出明确反馈，避免“点击无反应”
    toast.error(e?.message || '加载记录详情失败')
    detailVisible.value = false
  } finally {
    detailLoading.value = false
  }
}

async function removeRecord(row: CheckRecordItem) {
  try {
    await qualityApi.deleteRecord(row.id)
    if (detail.value?.id === row.id) {
      detail.value = null
      detailVisible.value = false
    }
    loadRecords()
  } catch (e: any) {
    toast.error(e?.message || '删除记录失败')
  }
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
              <div class="fmt-toggle" role="group" aria-label="输入格式">
                <button
                  type="button"
                  class="fmt-btn"
                  :class="{ active: structuredFormat === 'csv' }"
                  @click="structuredFormat = 'csv'"
                >CSV</button>
                <button
                  type="button"
                  class="fmt-btn"
                  :class="{ active: structuredFormat === 'json' }"
                  @click="structuredFormat = 'json'"
                >JSON</button>
              </div>
              <input type="text" class="q-input" v-model="structuredSource" placeholder="来源名称（可选）" />
              <label class="file-btn">
                {{ structuredFormat === 'json' ? '上传 JSON' : '上传 CSV' }}
                <input
                  type="file"
                  :accept="structuredFormat === 'json' ? '.json,application/json' : '.csv,text/csv'"
                  hidden
                  @change="onStructuredFile"
                />
              </label>
            </div>
            <div v-if="structuredDimOptions.length" class="dim-picker">
              <span class="dim-picker-label">评测维度</span>
              <span
                v-for="d in structuredDimOptions"
                :key="d.name"
                class="cleaner-chip"
                :class="{ active: selectedStructuredDims.includes(d.name) }"
                :title="d.description"
                @click="toggleStructuredDim(d.name)"
              >{{ dimLabel(d.name) }}</span>
            </div>
            <UiTextarea
              v-model="structuredInput"
              :placeholder="structuredPlaceholder"
              :rows="8"
            />
            <div v-if="structuredError" class="err">{{ structuredError }}</div>
            <template #footer>
              <div class="footer-actions">
                <UiButton variant="primary" :loading="structuredTask.isPending.value" @click="runStructured">开始评估</UiButton>
                <UiAsyncStatus :status="structuredTask.status.value" :message="structuredTask.message.value" @retry="runStructured" />
              </div>
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

      <!-- ========== 数据源直评 ========== -->
      <template #datasource>
        <div class="tab-body">
          <UiCard title="选择数据源与表" variant="bordered">
            <p class="ds-hint">取数在服务端完成（受管连接池 + 只读 SELECT），仅抽样样本用于评估，Python 不直连数据库。</p>
            <div class="ds-form">
              <UiSelect
                v-model="dsId"
                :options="dsOptions"
                label="数据源"
                placeholder="请选择数据源"
                filterable
                @change="onDatasourceChange"
              />
              <UiSelect
                v-model="dsTable"
                :options="dsTableOptions"
                label="数据表"
                :placeholder="dsTablesLoading ? '加载中…' : '请选择数据表'"
                :disabled="!dsId || dsTablesLoading"
                filterable
              />
              <div class="ds-num">
                <span class="ds-num-label">抽样行数</span>
                <UiInputNumber v-model="dsSampleSize" :min="1" :max="10000" :step="100" />
              </div>
            </div>
            <div v-if="dsError" class="err">{{ dsError }}</div>
            <template #footer>
              <div class="footer-actions">
                <UiButton variant="primary" :loading="dsTask.isPending.value" @click="runDatasource">抽样评估</UiButton>
                <UiAsyncStatus :status="dsTask.status.value" :message="dsTask.message.value" @retry="runDatasource" />
              </div>
            </template>
          </UiCard>

          <UiCard v-if="dsReport" title="评估结果" variant="bordered" class="result-card">
            <div class="stat-grid">
              <UiStatistic title="综合得分" :value="pct(dsReport.overall_score)" suffix="分" />
              <UiStatistic title="抽样行数" :value="dsReport.record_count" />
              <UiStatistic title="问题数" :value="dsReport.issues?.length || 0" />
            </div>

            <div class="dim-list">
              <div v-for="d in dsReport.dimensions" :key="d.name" class="dim-item">
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
            <UiTable :columns="issueColumns" :data="dsReport.issues || []">
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
              <div class="footer-actions">
                <UiButton variant="primary" :loading="textTask.isPending.value" @click="runUnstructured">开始评估</UiButton>
                <UiAsyncStatus :status="textTask.status.value" :message="textTask.message.value" @retry="runUnstructured" />
              </div>
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
              <div class="fmt-toggle" role="group" aria-label="输入格式">
                <button
                  type="button"
                  class="fmt-btn"
                  :class="{ active: cleanFormat === 'csv' }"
                  @click="cleanFormat = 'csv'"
                >CSV</button>
                <button
                  type="button"
                  class="fmt-btn"
                  :class="{ active: cleanFormat === 'json' }"
                  @click="cleanFormat = 'json'"
                >JSON</button>
              </div>
              <input type="text" class="q-input" v-model="cleanSource" placeholder="来源名称（可选）" />
              <label class="file-btn">
                {{ cleanFormat === 'json' ? '上传 JSON' : '上传 CSV' }}
                <input
                  type="file"
                  :accept="cleanFormat === 'json' ? '.json,application/json' : '.csv,text/csv'"
                  hidden
                  @change="onCleanFile"
                />
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
            <div class="fmt-row">
              <span class="fmt-row-label">导出格式</span>
              <div class="fmt-toggle" role="group" aria-label="导出格式">
                <button
                  type="button"
                  class="fmt-btn"
                  :class="{ active: cleanOutputFormat === 'csv' }"
                  @click="cleanOutputFormat = 'csv'"
                >CSV</button>
                <button
                  type="button"
                  class="fmt-btn"
                  :class="{ active: cleanOutputFormat === 'json' }"
                  @click="cleanOutputFormat = 'json'"
                >JSON</button>
              </div>
            </div>
            <UiTextarea
              v-model="cleanInput"
              :placeholder="cleanFormat === 'json' ? '粘贴 JSON 数据，形如 [{&quot;id&quot;:1,&quot;name&quot;:&quot;张三&quot;}]' : '粘贴 CSV 数据，首行为表头'"
              :rows="8"
            />
            <div v-if="cleanError" class="err">{{ cleanError }}</div>
            <template #footer>
              <div class="footer-actions">
                <UiButton variant="primary" :loading="cleanTask.isPending.value" @click="runClean">开始清洗</UiButton>
                <UiAsyncStatus :status="cleanTask.status.value" :message="cleanTask.message.value" @retry="runClean" />
              </div>
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
              <UiButton variant="secondary" @click="downloadCleaned">
                下载清洗后数据（{{ (cleanReport.cleaned_format || 'csv').toUpperCase() }}）
              </UiButton>
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

        </div>
      </template>
    </UiTabs>

    <!-- 记录详情弹窗：居中展示，避免详情渲染在长表格下方超出视口。 -->
    <UiModal
      v-model="detailVisible"
      :title="detail ? `记录详情 · ${detail.source_name}` : '记录详情'"
      size="lg"
      centered
    >
      <div v-if="detailLoading" class="detail-loading">加载中…</div>
      <template v-else-if="detail">
        <div class="detail-summary">{{ detail.summary || '无摘要' }}</div>
        <pre class="report-json">{{ JSON.stringify(detail.report, null, 2) }}</pre>
      </template>
      <UiEmpty v-else title="暂无详情" description="未获取到记录详情" />
    </UiModal>
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
.fmt-toggle {
  display: inline-flex;
  height: 36px;
  border: 1px solid var(--border-default, #2a2d35);
  border-radius: var(--radius-md, 8px);
  overflow: hidden;
  flex: none;
}
.fmt-btn {
  height: 100%;
  padding: 0 var(--space-4, 16px);
  border: none;
  background: var(--bg-elevated, #1a1d24);
  color: var(--text-secondary, #9ca3af);
  font-size: 14px;
  cursor: pointer;
}
.fmt-btn + .fmt-btn {
  border-left: 1px solid var(--border-default, #2a2d35);
}
.fmt-btn.active {
  background: var(--primary, #6366f1);
  color: #fff;
}
.fmt-row {
  display: flex;
  align-items: center;
  gap: var(--space-3, 12px);
  margin-bottom: var(--space-3, 12px);
}
.fmt-row-label {
  font-size: 13px;
  color: var(--text-secondary, #9ca3af);
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
.ds-hint {
  margin: 0 0 var(--space-3, 12px);
  color: var(--text-secondary, #9ca3af);
  font-size: 13px;
  line-height: 1.6;
}
.ds-form {
  display: grid;
  grid-template-columns: 1fr 1fr auto;
  gap: var(--space-4, 16px);
  align-items: end;
}
.ds-num {
  display: flex;
  flex-direction: column;
  gap: var(--space-2, 8px);
}
.ds-num-label {
  font-size: 13px;
  color: var(--text-secondary, #9ca3af);
}
@media (max-width: 720px) {
  .ds-form {
    grid-template-columns: 1fr;
  }
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
.dim-picker {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2, 8px);
  margin-bottom: var(--space-3, 12px);
}
.dim-picker-label {
  color: var(--text-secondary, #9ca3af);
  font-size: 13px;
  margin-right: var(--space-1, 4px);
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
.footer-actions {
  display: flex;
  align-items: center;
  gap: var(--space-3, 12px);
  flex-wrap: wrap;
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
.detail-loading {
  padding: var(--space-6, 24px);
  text-align: center;
  color: var(--text-secondary, #9ca3af);
  font-size: 14px;
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
