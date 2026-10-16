<template>
  <div class="collection-tasks">
    <header class="collection-tasks__header">
      <div>
        <span class="collection-eyebrow">从网页到可用知识</span><h2>网页采集任务</h2>
        <p>仅采集明确指定的 HTTPS 地址，保存候选快照。清洗、审批与发布完成后才进入知识检索。</p>
      </div>
      <UiButton variant="primary" @click="createVisible = true">创建采集任务</UiButton>
    </header>
    <ol class="collection-flow" aria-label="采集操作流程"><li><b>01</b> 选择知识库与来源</li><li><b>02</b> 开始采集并核对结果</li><li><b>03</b> 候选转入流水线审批发布</li></ol>
    <div class="collection-tasks__toolbar">
      <UiSelect v-model="statusFilter" aria-label="筛选采集任务状态" :options="statusOptions" style="width: 160px" @update:model-value="loadTasks" />
      <UiButton variant="ghost" :loading="loading" @click="loadTasks">刷新</UiButton>
      <span class="list-count" aria-live="polite">{{ loading ? '正在更新任务…' : `当前列表 ${tasks.length} 个任务` }}</span>
    </div>
    <UiAlert v-if="error" type="error" :title="error" />

    <UiTable :columns="taskColumns" :data="tasks" :aria-busy="loading" hover>
      <template #empty><div class="collection-empty" role="status"><h3>{{ loading ? '正在加载采集任务…' : error ? '任务暂时无法加载' : statusFilter ? '没有符合此状态的任务' : '从第一个网页开始' }}</h3><p>{{ loading ? '请稍候，正在读取最新状态。' : error ? '可以刷新重试，已创建的任务不会丢失。' : statusFilter ? '调整筛选条件，或创建新的采集任务。' : '指定网页地址和知识库，采集完成后可继续清洗、审批与分析。' }}</p><UiButton v-if="!loading && !error && !statusFilter" variant="primary" @click="createVisible = true">新建第一个采集任务</UiButton><UiButton v-else-if="!loading && statusFilter" variant="ghost" @click="clearFilter">查看全部任务</UiButton></div></template>
      <template #cell-id="{ row }"><button class="link" @click="openDetail(row.id)">{{ shortId(row.id) }}</button></template>
      <template #cell-knowledge_base_id="{ row }">{{ knowledgeBaseName(row.knowledge_base_id) }}</template>
      <template #cell-status="{ row }"><UiTag :variant="statusVariant(row.status)">{{ statusLabel(row.status) }}</UiTag><div v-if="row.error_message" class="error-reason">{{ row.error_message }}</div></template>
      <template #cell-progress="{ row }">{{ progressLabel(row) }}</template>
      <template #cell-created_at="{ row }">{{ formatDate(row.created_at) }}</template>
      <template #cell-actions="{ row }">
        <div class="actions">
          <UiButton size="sm" variant="ghost" @click="openDetail(row.id)">详情</UiButton>
          <UiButton v-if="row.status === 'draft' && row.mode !== 'snapshot'" size="sm" variant="primary" :disabled="pendingTasks.has(row.id)" @click="startTask(row.id)">开始</UiButton>
          <UiButton v-if="canCancel(row.status) && row.mode !== 'snapshot'" size="sm" variant="danger" :disabled="pendingTasks.has(row.id)" @click="cancelTask(row.id)">取消</UiButton>
        </div>
      </template>
    </UiTable>

    <UiModal v-model="createVisible" title="创建网页采集任务" size="lg" :closable="!creating" :mask-closable="false" :close-on-esc="!creating" @close="dismissCreateDialog">
      <UiAlert type="info" title="采集范围受限">每行填写一个 HTTPS 地址，最多 50 个。系统不会递归抓取链接或访问需要登录的网站。</UiAlert>
      <UiAlert v-if="createError" type="error" :title="createError" />
      <UiForm ref="createFormRef" class="create-form" :model="createForm" :rules="createRules" :disabled="creating" @submit.prevent="createTask">
        <UiFormItem label="目标知识库" prop="knowledge_base_id">
          <UiSelect v-model="createForm.knowledge_base_id" placeholder="选择知识库" :options="knowledgeBaseOptions" />
          <p v-if="knowledgeError" role="alert">{{ knowledgeError }}</p>
          <p v-else-if="knowledgeLoading" role="status">正在加载可访问的知识库…</p>
          <p v-else-if="!knowledgeBases.length">暂无可访问的知识库。请先在知识库页面创建。</p>
          <UiButton v-if="knowledgeHasMore || knowledgeError" variant="ghost" :loading="knowledgeLoading" @click="loadKnowledgeBases">{{ knowledgeError ? '重试加载知识库' : '加载更多知识库' }}</UiButton>
        </UiFormItem>
        <UiFormItem label="种子 URL" prop="urlsText">
          <UiTextarea v-model="createForm.urlsText" :rows="7" placeholder="https://docs.example.com/guide\nhttps://docs.example.com/faq" />
          <p class="url-feedback" :class="{ invalid: urlIssue }" aria-live="polite">{{ urlIssue || `已填写 ${urlCount} / 50 个地址${duplicateCount ? `，${duplicateCount} 个重复地址将合并` : ''}` }}</p>
        </UiFormItem>
      </UiForm>
      <p class="draft-note">创建后还需手动开始采集。关闭弹窗保留本页草稿；离开页面后不保留。</p>
      <template #footer>
        <UiButton variant="ghost" :disabled="creating" @click="dismissCreateDialog">稍后继续</UiButton>
        <UiButton v-if="hasDraft" variant="ghost" :disabled="creating" @click="resetCreateForm">清空草稿</UiButton>
        <UiButton variant="primary" :loading="creating" :disabled="creating || !createForm.knowledge_base_id" @click="createTask">创建</UiButton>
      </template>
    </UiModal>

    <UiModal v-model="detailVisible" title="采集任务详情" size="xl" @close="closeDetail">
      <UiAlert v-if="detailError" type="error" :title="detailError" />
      <UiAlert v-if="detailActionError" type="error" :title="detailActionError" />
      <div v-if="detailLoading && !detail" class="detail-loading" role="status">正在加载任务详情…</div>
      <template v-else-if="detail">
        <section class="collection-outcome" aria-label="采集进度与下一步">
          <div class="outcome-heading"><h3>{{ detailHeadline }}</h3><UiTag :variant="statusVariant(detail.task.status)">{{ statusLabel(detail.task.status) }}</UiTag></div>
          <p>{{ detailNextStep }}</p>
          <div v-if="detailCounts.total" class="source-progress"><progress :value="detailCounts.total - detailCounts.pending" :max="detailCounts.total" aria-label="已处理来源" /><span>{{ detailCounts.total - detailCounts.pending }} / {{ detailCounts.total }} 个来源已处理</span></div>
          <div class="source-metrics"><div><strong>{{ detailCounts.usable }}</strong><span>可用来源</span></div><div><strong>{{ detailCounts.failed + detailCounts.rejected }}</strong><span>需要处理</span></div><div><strong>{{ detailCounts.pending }}</strong><span>等待处理</span></div></div>
          <p class="progress-detail">{{ progressSummary(detail) }}</p>
        </section>
        <UiAlert v-if="detail.task.error_message || detail.run?.error_message" type="error" :title="detail.task.error_message || detail.run?.error_message" />
        <UiAlert v-if="detail.summary?.outcome === 'partial'" type="warning" title="部分来源采集失败">可用来源 {{ detail.summary.usable }} / {{ detail.summary.total }}，请查看各来源原因。</UiAlert>
        <div class="detail-actions">
          <UiButton v-if="detail.task.status === 'draft' && !selectedRunId && detail.task.mode !== 'snapshot'" variant="primary" :disabled="pendingTasks.has(detail.task.id)" @click="startTask(detail.task.id)">开始采集</UiButton>
          <UiButton v-if="canCancel(detail.task.status) && !selectedRunId && detail.task.mode !== 'snapshot'" variant="danger" :disabled="pendingTasks.has(detail.task.id)" @click="cancelTask(detail.task.id)">取消任务</UiButton>
          <UiButton v-if="['failed', 'cancelled'].includes(detail.task.status) || detail.summary?.outcome === 'partial'" variant="primary" @click="prepareRetry">新建任务重新采集</UiButton>
          <UiButton variant="ghost" :loading="detailLoading" @click="refreshDetail">刷新</UiButton>
          <RouterLink v-if="detail.items?.some(item => !!item.knowledge_id) && detail.task.knowledge_base_id" :to="`/knowledge/${encodeURIComponent(detail.task.knowledge_base_id)}`">查看知识库 →</RouterLink>
        </div>
        <details class="collection-technical"><summary>任务与运行信息</summary><div class="detail-summary">
          <div><span>任务 ID</span><code>{{ detail.task.id }}</code></div>
          <div><span>状态</span><UiTag :variant="statusVariant(detail.task.status)">{{ statusLabel(detail.task.status) }}</UiTag></div>
          <div><span>目标知识库</span><RouterLink v-if="detail.task.knowledge_base_id" :to="`/knowledge/${encodeURIComponent(detail.task.knowledge_base_id)}`">{{ knowledgeBaseName(detail.task.knowledge_base_id) }} · 查看知识库</RouterLink></div>
          <div><span>创建时间</span>{{ formatDate(detail.task.created_at) }}</div>
          <template v-if="detail.run">
            <div><span>最新运行 ID</span><code>{{ detail.run.id }}</code></div>
            <div><span>运行状态</span>{{ statusLabel(detail.run.status) }}</div>
            <div><span>开始时间</span>{{ formatDate(detail.run.started_at) }}</div>
            <div><span>结束时间</span>{{ formatDate(detail.run.completed_at) }}</div>
          </template>
        </div></details>
        <p v-if="!detail.run">尚未开始采集。来源：{{ detail.sources.map(source => source.url).join('、') }}</p>
        <p v-else>{{ selectedRunId ? '展示指定运行；' : '展示最新一次运行；' }}执行中每 4 秒更新。关闭详情不影响后台采集。</p>
        <div class="source-result-head"><h3>来源结果</h3><div class="source-filters" aria-label="筛选来源结果"><button v-for="filter in itemFilters" :key="filter.value" type="button" :aria-pressed="itemFilter === filter.value" @click="itemFilter = filter.value">{{ filter.label }}</button></div></div>
        <UiTable :columns="itemColumns" :data="visibleItems" hover>
          <template #empty><p class="collection-empty">{{ detail.run ? '此筛选下暂无来源结果。执行中的结果会自动更新。' : '开始采集后，来源结果会显示在这里。' }}</p></template>
          <template #cell-source_url="{ row }"><a :href="safeSourceURL(row.source_url)" target="_blank" rel="noopener noreferrer">{{ row.source_url }}</a><div v-if="row.canonical_url && row.canonical_url !== row.source_url">最终地址：{{ row.canonical_url }}</div><div><small>产物 ID：{{ row.id }}</small></div></template>
          <template #cell-status="{ row }"><UiTag :variant="itemStatusVariant(row.status)">{{ itemStatusLabel(row.status) }}</UiTag></template>
          <template #cell-artifact="{ row }"><div>HTTP {{ row.http_status || '—' }} · {{ row.content_type || '—' }}</div><div v-if="row.normalized_object_key">规范化内容：{{ row.normalized_size || 0 }} 字节</div><details v-if="row.content_hash || row.raw_hash"><summary>内容指纹</summary><code>{{ row.content_hash || row.raw_hash }}</code></details></template>
          <template #cell-knowledge_id="{ row }"><RouterLink v-if="row.knowledge_id" :to="`/knowledge/${encodeURIComponent(detail.task.knowledge_base_id)}`">{{ row.knowledge_id }}</RouterLink><span v-else>—</span></template>
          <template #cell-error_reason="{ row }"><span class="error-reason">{{ row.error_reason || '—' }}</span></template>
          <template #cell-actions="{ row }"><RouterLink v-if="isUsable(row) && row.normalized_object_key && detail.run" :to="{ path: '/pipelines', query: { source: 'collection', run_id: row.run_id || detail.run.id, item_id: row.id, name: row.canonical_url || row.source_url } }">转入流水线</RouterLink><span v-else>暂无可用产物</span></template>
        </UiTable>
      </template>
      <UiButton v-else-if="detailError" variant="ghost" @click="refreshDetail">重试加载详情</UiButton>
    </UiModal>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute } from 'vue-router'
import { UiAlert, UiButton, UiForm, UiFormItem, UiModal, UiSelect, UiTable, UiTag, UiTextarea } from '@/components'
import type { FormRule } from '@/components/ui/form-context'
import { lifecycleApi, type PublicationTarget } from '@/api/lifecycle'
import { collectionApi, type CollectionTask, type CollectionTaskDetailResponse, type CollectionTaskStatus, type CollectionItemOutcome } from '@/api/collection'
import { useAuthStore } from '@/stores/auth'
import toast from '@/utils/toast'
import { ElMessageBox } from '@/utils/confirm'

const auth = useAuthStore()
const route = useRoute()
const loading = ref(false), creating = ref(false), detailLoading = ref(false)
const createVisible = ref(false), detailVisible = ref(false)
const error = ref(''), createError = ref(''), detailError = ref(''), detailActionError = ref(''), knowledgeError = ref('')
const knowledgeLoading = ref(false), knowledgeHasMore = ref(true)
const createFormRef = ref<InstanceType<typeof UiForm>>()
const tasks = ref<CollectionTask[]>([])
const knowledgeBases = ref<PublicationTarget[]>([])
const detail = ref<CollectionTaskDetailResponse>()
const summaries = reactive<Record<string, string>>({})
const pendingTasks = reactive(new Set<string>())
const selectedTaskId = ref('')
const statusFilter = ref<CollectionTaskStatus | ''>('')
const createForm = reactive({ knowledge_base_id: '', urlsText: '' })
const hasDraft = computed(() => !!(createForm.knowledge_base_id || createForm.urlsText.trim()))
const urlCount = computed(() => urlsFromForm().length)
const duplicateCount = computed(() => createForm.urlsText.split(/\r?\n/).filter(line => line.trim()).length - urlCount.value)
const urlIssue = computed(() => {
  if (urlCount.value > 50) return '最多填写 50 个不同的 HTTPS 地址'
  const invalid = createForm.urlsText.split(/\r?\n/).map((url, index) => ({ url: url.trim(), line: index + 1 })).filter(item => item.url && !isHttpsUrl(item.url))
  return invalid.length ? `第 ${invalid.slice(0, 3).map(item => item.line).join('、')} 行需要不含账号密码的 HTTPS 地址` : ''
})
const itemFilter = ref('all')
const itemFilters = [{ value: 'all', label: '全部来源' }, { value: 'usable', label: '可用来源' }, { value: 'problems', label: '失败与拒绝' }]
const visibleItems = computed(() => (detail.value?.items || []).filter(item => itemFilter.value === 'all' || (itemFilter.value === 'usable' ? isUsable(item) : ['failed', 'rejected'].includes(item.status))))
const detailCounts = computed(() => detail.value ? sourceCounts(detail.value) : { total: 0, pending: 0, usable: 0, accepted: 0, skipped: 0, rejected: 0, failed: 0 })
const detailHeadline = computed(() => detail.value?.summary?.outcome === 'partial' ? '采集已结束，部分来源需要处理' : ({ draft: '准备开始采集', running: '正在采集指定来源', completed: '采集完成，内容已可使用', failed: '采集未成功，请检查来源原因', cancelled: '采集已取消' } as Record<string, string>)[detail.value?.task.status || ''] || '采集进度')
const detailNextStep = computed(() => ({ draft: '核对目标知识库和来源后，点击开始采集。', running: '结果会自动更新。离开本页不会中止已开始的任务。', completed: '已采集内容仅为候选。转入流水线完成清洗、审批与发布后，才进入正式知识检索。', failed: '查看失败或拒绝原因。修正地址后可创建新任务重新采集。', cancelled: '已完成的来源会保留；如需继续，可使用原地址创建新任务。' } as Record<string, string>)[detail.value?.task.status || ''] || '')
let disposed = false, epoch = 0, listRequest = 0, detailRequest = 0, knowledgePage = 1, formGeneration = 0
let createAttempt: { payload: string; key: string } | undefined
let timer: ReturnType<typeof setInterval> | undefined
const taskColumns = [
  { key: 'id', title: '任务 ID', width: 120 },
  { key: 'knowledge_base_id', title: '目标知识库' },
  { key: 'status', title: '状态', width: 100 },
  { key: 'progress', title: '来源进度' },
  { key: 'created_at', title: '创建时间', width: 170 },
  { key: 'actions', title: '操作', width: 210 }
]
const itemColumns = [
  { key: 'source_url', title: '来源与产物', width: 240 },
  { key: 'status', title: '结果', width: 100 },
  { key: 'artifact', title: '采集内容', width: 170 },
  { key: 'knowledge_id', title: '知识条目', width: 150 },
  { key: 'error_reason', title: '原因', width: 220 },
  { key: 'actions', title: '下一步', width: 130 }
]
const statusOptions = [
  { label: '全部状态', value: '' }, ...['draft', 'running', 'completed', 'failed', 'cancelled'].map(value => ({ label: statusLabel(value as CollectionTaskStatus), value }))
]
const knowledgeBaseOptions = computed(() => knowledgeBases.value.map(kb => ({ label: kb.name, value: kb.id })))
const createRules: Record<string, FormRule[]> = {
  knowledge_base_id: [{ required: true, message: '请选择目标知识库', trigger: 'change' }],
  urlsText: [{ required: true, message: '请至少填写一个 HTTPS 地址', trigger: 'blur' }]
}
function statusLabel(status: CollectionTaskStatus) {
  return ({ draft: '草稿', running: '执行中', completed: '已完成', failed: '失败', cancelled: '已取消' })[status] || status
}
function itemStatusLabel(status: string) {
  return ({ pending: '等待中', running: '执行中', candidate: '候选 · 待审批', accepted: '已有回执', skipped: '已跳过', rejected: '已拒绝', failed: '失败', cancelled: '已取消' } as Record<string, string>)[status] || status
}
function statusVariant(status: string) {
  return (({ draft: 'info', running: 'primary', completed: 'success', failed: 'danger', cancelled: 'warning' } as Record<string, string>)[status] || 'info') as 'info' | 'primary' | 'success' | 'danger' | 'warning'
}
function itemStatusVariant(status: string) {
  return status === 'candidate' ? 'warning' : status === 'rejected' ? 'danger' : status === 'skipped' ? 'warning' : statusVariant(status === 'accepted' ? 'completed' : status)
}
function canCancel(status: CollectionTaskStatus) { return status === 'draft' || status === 'running' }
function isUsable(item: CollectionItemOutcome) { return (item.status === 'candidate' && !!item.content_hash && !!item.normalized_object_key) || ((item.status === 'accepted' || item.status === 'skipped') && !!(item.knowledge_id || item.asset_ref_id)) }
function clearFilter() { statusFilter.value = ''; void loadTasks() }
function shortId(id: string) { return id.length > 12 ? `${id.slice(0, 8)}…` : id }
function formatDate(value?: string) { return value ? new Date(value).toLocaleString() : '—' }
function progressLabel(task: CollectionTask) { return summaries[task.id] || (task.status === 'draft' ? '尚未开始' : '打开详情查看') }
function sourceCounts(payload: CollectionTaskDetailResponse) {
  const items = payload.items || []
  if (payload.summary) return { ...payload.summary, accepted: payload.summary.accepted + (payload.summary.candidates || 0) }
  return {
    total: payload.sources.length,
    accepted: items.filter(item => item.status === 'accepted' || item.status === 'candidate').length,
    skipped: items.filter(item => item.status === 'skipped').length,
    failed: items.filter(item => item.status === 'failed').length,
    rejected: items.filter(item => item.status === 'rejected').length,
    pending: Math.max(0, payload.sources.length - items.filter(item => ['candidate', 'accepted', 'skipped', 'rejected', 'failed', 'cancelled'].includes(item.status)).length),
    usable: items.filter(isUsable).length
  }
}
function progressSummary(payload: CollectionTaskDetailResponse) {
  const counts = sourceCounts(payload)
  return `共 ${counts.total} 个 · 已采集 ${counts.accepted} · 跳过 ${counts.skipped} · 拒绝 ${counts.rejected} · 失败 ${counts.failed} · 待处理 ${counts.pending}`
}
function knowledgeBaseName(id: string) { return knowledgeBases.value.find(kb => kb.id === id)?.name || id }
function active(requestEpoch: number) { return !disposed && requestEpoch === epoch }
function message(cause: unknown, fallback: string) { return cause instanceof Error ? cause.message : fallback }
async function loadKnowledgeBases() {
  if (knowledgeLoading.value || !knowledgeHasMore.value) return
  const requestEpoch = epoch, page = knowledgePage
  knowledgeLoading.value = true
  knowledgeError.value = ''
  try {
    const payload = (await lifecycleApi.knowledgeBases(page)).data
    if (!active(requestEpoch)) return
    if (!payload || !Array.isArray(payload.items)) throw new Error('知识库列表格式异常，请重试')
    const hasMore = page * payload.size < payload.total
    if (hasMore && !payload.items.length) throw new Error('知识库分页异常，请重试')
    knowledgeBases.value = [...new Map([...knowledgeBases.value, ...payload.items].map(kb => [kb.id, kb])).values()]
    knowledgeHasMore.value = hasMore
    knowledgePage = page + 1
  } catch (cause) {
    if (active(requestEpoch)) knowledgeError.value = message(cause, '知识库加载失败')
  } finally { if (active(requestEpoch)) knowledgeLoading.value = false }
}
async function loadTasks() {
  const requestEpoch = epoch, request = ++listRequest
  loading.value = true
  error.value = ''
  try {
    const response = await collectionApi.list({ status: statusFilter.value || undefined })
    if (active(requestEpoch) && request === listRequest) tasks.value = response.data?.tasks || []
  } catch (cause) {
    if (active(requestEpoch) && request === listRequest) error.value = message(cause, '采集任务加载失败')
  } finally { if (active(requestEpoch) && request === listRequest) loading.value = false }
}
function urlsFromForm() { return [...new Set(createForm.urlsText.split(/\r?\n/).map(url => url.trim()).filter(Boolean))] }
async function createTask() {
  if (creating.value) return
  const requestEpoch = epoch
  const generation = formGeneration
  creating.value = true
  createError.value = ''
  try {
    const valid = await createFormRef.value?.validate()
    if (!active(requestEpoch) || !valid) return
    const seedUrls = urlsFromForm()
    if (!seedUrls.length || seedUrls.length > 50) { toast.error('请填写 1 到 50 个 HTTPS 地址'); return }
    if (seedUrls.some(url => !isHttpsUrl(url))) { toast.error('仅允许不含账号密码的 HTTPS 地址'); return }
    if (!knowledgeBases.value.some(kb => kb.id === createForm.knowledge_base_id)) { toast.error('请选择当前可访问的知识库'); return }
    const body = { knowledge_base_id: createForm.knowledge_base_id, sources: seedUrls }
    const payload = JSON.stringify(body)
    if (!createAttempt || createAttempt.payload !== payload) createAttempt = { payload, key: crypto.randomUUID() }
    const response = await collectionApi.create({ ...body, idempotency_key: createAttempt.key })
    if (!active(requestEpoch)) return
    const showCreated = generation === formGeneration
    if (showCreated) { toast.success('采集任务已创建'); closeCreateDialog() }
    const completedGeneration = formGeneration
    await loadTasks()
    if (active(requestEpoch) && showCreated && completedGeneration === formGeneration && !createVisible.value && response.data?.id) await openDetail(response.data.id)
  } catch (cause) { if (active(requestEpoch)) { createError.value = message(cause, '创建失败，可重试'); toast.error(createError.value) } }
  finally { if (active(requestEpoch)) creating.value = false }
}
function isHttpsUrl(value: string) { try { const url = new URL(value); return url.protocol === 'https:' && !!url.hostname && !url.username && !url.password } catch { return false } }
function safeSourceURL(value: string) { return isHttpsUrl(value) ? value : undefined }
function resetCreateForm() { ++formGeneration; createForm.knowledge_base_id = ''; createForm.urlsText = ''; createError.value = ''; createAttempt = undefined; createFormRef.value?.clearValidate() }
function closeCreateDialog() { createVisible.value = false; resetCreateForm() }
function dismissCreateDialog() { if (!creating.value) createVisible.value = false }
const selectedRunId = ref('')
function closeDetail() { selectedRunId.value = ''; ++detailRequest; detailVisible.value = false; selectedTaskId.value = ''; detail.value = undefined; detailLoading.value = false; detailError.value = ''; detailActionError.value = '' }
async function openDetail(id: string, runOnly = false) { selectedRunId.value = runOnly ? id : ''; itemFilter.value = 'all'; detailActionError.value = ''; selectedTaskId.value = id; detail.value = undefined; detailVisible.value = true; await refreshDetail() }
function prepareRetry() {
  if (!detail.value) return
  const failedURLs = (detail.value.items || []).filter(item => ['failed', 'rejected'].includes(item.status)).map(item => item.source_url)
  resetCreateForm()
  createForm.knowledge_base_id = detail.value.task.knowledge_base_id
  createForm.urlsText = [...new Set(failedURLs.length ? failedURLs : detail.value.sources.map(source => source.url))].join('\n')
  closeDetail()
  createVisible.value = true
}
async function refreshDetail() {
  const id = selectedTaskId.value
  if (!id || !detailVisible.value) return
  const requestEpoch = epoch, request = ++detailRequest
  detailLoading.value = true
  detailError.value = ''
  try {
    const payload = (await (selectedRunId.value ? collectionApi.getRun(selectedRunId.value) : collectionApi.getById(id))).data
    if (!active(requestEpoch) || request !== detailRequest || id !== selectedTaskId.value) return
    if (!payload) throw new Error('采集任务详情为空')
    detail.value = payload
    summaries[id] = progressSummary(payload)
    const index = tasks.value.findIndex(task => task.id === id)
    if (index >= 0) tasks.value[index] = payload.task
  } catch (cause) {
    if (active(requestEpoch) && request === detailRequest) { const status = (cause as { response?: { status?: number } })?.response?.status; if (status && [401, 403, 404].includes(status)) detail.value = undefined; detailError.value = message(cause, '采集任务详情加载失败') }
  } finally { if (active(requestEpoch) && request === detailRequest) detailLoading.value = false }
}
async function startTask(id: string) {
  if (pendingTasks.has(id)) return
  const requestEpoch = epoch
  pendingTasks.add(id)
  if (selectedTaskId.value === id) detailActionError.value = ''
  try {
    await collectionApi.start(id)
    if (!active(requestEpoch)) return
    toast.success('采集任务已开始')
    await Promise.all([loadTasks(), selectedTaskId.value === id ? refreshDetail() : Promise.resolve()])
  } catch (cause) { if (active(requestEpoch)) { error.value = message(cause, '开始采集失败'); if (selectedTaskId.value === id) detailActionError.value = error.value; toast.error(error.value) } }
  finally { if (active(requestEpoch)) pendingTasks.delete(id) }
}
async function cancelTask(id: string) {
  if (pendingTasks.has(id)) return
  const requestEpoch = epoch
  pendingTasks.add(id)
  if (selectedTaskId.value === id) detailActionError.value = ''
  try {
    await ElMessageBox.confirm({ title: '取消采集任务', message: '确定取消此任务？未开始的来源将不会继续采集。', confirmButtonText: '取消任务', cancelButtonText: '返回', type: 'warning' })
    if (!active(requestEpoch)) return
    await collectionApi.cancel(id)
    if (!active(requestEpoch)) return
    toast.success('采集任务已取消')
    await Promise.all([loadTasks(), selectedTaskId.value === id ? refreshDetail() : Promise.resolve()])
  } catch (cause) { if (active(requestEpoch) && cause !== 'cancel') { error.value = message(cause, '取消采集失败'); if (selectedTaskId.value === id) detailActionError.value = error.value; toast.error(error.value) } }
  finally { if (active(requestEpoch)) pendingTasks.delete(id) }
}
watch(() => `${auth.authEpoch}:${auth.currentTenant?.id}`, () => {
  ++epoch
  closeDetail()
  closeCreateDialog()
  tasks.value = []; knowledgeBases.value = []; pendingTasks.clear()
  for (const id of Object.keys(summaries)) delete summaries[id]
  knowledgePage = 1; knowledgeHasMore.value = true; knowledgeLoading.value = false; creating.value = false
  void Promise.all([loadKnowledgeBases(), loadTasks()])
})
watch(() => [route?.query.task_id, route?.query.run_id], ([value, runID]) => {
  if (typeof runID === 'string' && runID.trim()) void openDetail(runID.trim(), true)
  else if (typeof value === 'string' && value.trim()) void openDetail(value.trim())
  else closeDetail()
}, { immediate: true })
function warnBeforeUnload(event: BeforeUnloadEvent) { if (hasDraft.value || creating.value) { event.preventDefault(); event.returnValue = '' } }
onBeforeRouteLeave(() => !(hasDraft.value || creating.value) || window.confirm(creating.value ? '任务正在提交。离开后请在任务列表确认结果，仍要离开吗？' : '采集草稿尚未创建，离开页面将丢失草稿。仍要离开吗？'))
onMounted(() => {
  window.addEventListener('beforeunload', warnBeforeUnload)
  void Promise.all([loadKnowledgeBases(), loadTasks()])
  timer = setInterval(() => {
    if (detailVisible.value && !detailLoading.value && detail.value?.task.status === 'running') void refreshDetail()
  }, 4000)
})
onBeforeUnmount(() => { disposed = true; ++epoch; window.removeEventListener('beforeunload', warnBeforeUnload); if (timer) clearInterval(timer) })
</script>

<style scoped>
.collection-tasks { padding: 24px; background: var(--bg-secondary); border: 1px solid var(--border-default); border-radius: var(--radius-lg); }
.collection-tasks__header { display: flex; justify-content: space-between; gap: 20px; align-items: flex-start; margin-bottom: 24px; }
.collection-tasks h2 { margin: 0; font-size: 20px; }.collection-tasks p { margin: 8px 0 0; color: var(--color-text-secondary); }
.collection-tasks__toolbar, .actions, .detail-actions { display: flex; align-items: center; gap: 8px; }.collection-tasks__toolbar { margin-bottom: 20px; }
.link { padding: 0; border: 0; background: transparent; color: var(--primary); cursor: pointer; }.create-form { margin-top: 20px; }
.detail-loading { padding: 36px; text-align: center; color: var(--color-text-secondary); }.detail-summary { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; margin-bottom: 18px; }.detail-summary > div { display: flex; flex-direction: column; gap: 6px; }.detail-summary span { color: var(--color-text-secondary); font-size: var(--text-sm); }.collection-tasks code, .collection-tasks a { overflow-wrap: anywhere; }.detail-actions { margin-bottom: 24px; }.error-reason { color: var(--color-text-secondary); overflow-wrap: anywhere; }
@media (max-width: 720px) { .collection-tasks__header { flex-direction: column; }.detail-summary { grid-template-columns: 1fr; } }
.collection-eyebrow { display: block; margin-bottom: 8px; color: var(--primary); font-size: 12px; font-weight: 600; }
.collection-flow { display: flex; gap: 24px; list-style: none; margin: 0 0 26px; padding: 16px 0; border-block: 1px solid var(--border-default); color: var(--text-secondary); font-size: 13px; }
.collection-flow b { color: var(--primary); font-variant-numeric: tabular-nums; margin-right: 6px; }
.list-count { margin-left: auto; color: var(--text-muted); font-size: 12px; }
.collection-empty { padding: 30px 16px; text-align: center; line-height: 1.7; }
.collection-empty h3 { margin: 0; }.collection-empty .ui-button { margin-top: 16px; }
.collection-outcome { padding: 20px; border: 1px solid var(--border-default); border-radius: var(--radius-lg); background: var(--bg-elevated); margin-bottom: 18px; }
.outcome-heading, .source-result-head { display: flex; justify-content: space-between; align-items: center; gap: 12px; }.outcome-heading h3 { margin: 0; font-size: 17px; }
.source-progress { display: flex; gap: 12px; align-items: center; font-size: 12px; margin-top: 18px; }.source-progress progress { flex: 1; min-width: 0; height: 8px; accent-color: var(--primary); }.source-progress span { white-space: nowrap; }
.source-metrics { display: flex; gap: 36px; margin-top: 18px; }.source-metrics strong { display: block; font-size: 25px; font-variant-numeric: tabular-nums; }.source-metrics span { font-size: 12px; color: var(--text-muted); }
.progress-detail, .draft-note, .url-feedback { font-size: 12px; line-height: 1.7; }.url-feedback.invalid { color: var(--danger); }
.collection-technical { border-block: 1px solid var(--border-default); padding: 14px 0; margin-bottom: 14px; }.collection-technical summary { cursor: pointer; font-size: 13px; }.collection-technical .detail-summary { margin: 18px 0 0; }
.detail-actions, .collection-tasks__toolbar, .actions { flex-wrap: wrap; }.source-result-head { margin: 20px 0 12px; }.source-result-head h3 { margin: 0; }
.source-filters { display: flex; gap: 4px; flex-wrap: wrap; }.source-filters button { border: 1px solid var(--border-default); background: transparent; color: var(--text-secondary); border-radius: var(--radius-sm); padding: 7px 10px; font: inherit; font-size: 12px; cursor: pointer; }.source-filters button[aria-pressed="true"] { color: var(--primary); border-color: var(--primary); background: var(--primary-dim); }
button:focus-visible, a:focus-visible, summary:focus-visible { outline: 2px solid var(--primary); outline-offset: 3px; }
@media (max-width: 720px) { .collection-tasks { padding: 16px; }.collection-flow { flex-direction: column; gap: 12px; }.list-count { margin-left: 0; }.source-result-head { align-items: flex-start; flex-direction: column; }.source-metrics { gap: 22px; }.collection-outcome { padding: 16px; }.source-progress { flex-direction: column; align-items: stretch; } }
</style>
