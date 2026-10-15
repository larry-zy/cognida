<template>
  <div class="collection-tasks">
    <header class="collection-tasks__header">
      <div>
        <h2>网页采集任务</h2>
        <p>仅采集明确指定的 HTTPS 地址，并将通过校验的内容写入选定知识库。</p>
      </div>
      <UiButton variant="primary" @click="createVisible = true">创建采集任务</UiButton>
    </header>

    <div class="collection-tasks__toolbar">
      <UiSelect v-model="statusFilter" :options="statusOptions" style="width: 160px" @update:model-value="loadTasks" />
      <UiButton variant="ghost" :loading="loading" @click="loadTasks">刷新</UiButton>
    </div>
    <UiAlert v-if="error" type="error" :title="error" />

    <UiTable :columns="taskColumns" :data="tasks" hover>
      <template #cell-id="{ row }"><button class="link" @click="openDetail(row.id)">{{ shortId(row.id) }}</button></template>
      <template #cell-knowledge_base_id="{ row }">{{ knowledgeBaseName(row.knowledge_base_id) }}</template>
      <template #cell-status="{ row }"><UiTag :variant="statusVariant(row.status)">{{ statusLabel(row.status) }}</UiTag><div v-if="row.error_message" class="error-reason">{{ row.error_message }}</div></template>
      <template #cell-progress="{ row }">{{ progressLabel(row) }}</template>
      <template #cell-created_at="{ row }">{{ formatDate(row.created_at) }}</template>
      <template #cell-actions="{ row }">
        <div class="actions">
          <UiButton size="sm" variant="ghost" @click="openDetail(row.id)">详情</UiButton>
          <UiButton v-if="row.status === 'draft'" size="sm" variant="primary" :disabled="pendingTasks.has(row.id)" @click="startTask(row.id)">开始</UiButton>
          <UiButton v-if="canCancel(row.status)" size="sm" variant="danger" :disabled="pendingTasks.has(row.id)" @click="cancelTask(row.id)">取消</UiButton>
        </div>
      </template>
    </UiTable>

    <UiModal v-model="createVisible" title="创建网页采集任务" size="lg" @close="resetCreateForm">
      <UiAlert type="info" title="采集范围受限">每行填写一个 HTTPS 地址，最多 50 个。系统不会递归抓取链接或访问需要登录的网站。</UiAlert>
      <UiForm ref="createFormRef" class="create-form" :model="createForm" :rules="createRules">
        <UiFormItem label="目标知识库" prop="knowledge_base_id">
          <UiSelect v-model="createForm.knowledge_base_id" placeholder="选择知识库" :options="knowledgeBaseOptions" />
          <p v-if="knowledgeError" role="alert">{{ knowledgeError }}</p>
          <UiButton v-if="knowledgeHasMore || knowledgeError" variant="ghost" :loading="knowledgeLoading" @click="loadKnowledgeBases">{{ knowledgeError ? '重试加载知识库' : '加载更多知识库' }}</UiButton>
        </UiFormItem>
        <UiFormItem label="种子 URL" prop="urlsText">
          <UiTextarea v-model="createForm.urlsText" :rows="7" placeholder="https://docs.example.com/guide\nhttps://docs.example.com/faq" />
        </UiFormItem>
      </UiForm>
      <template #footer>
        <UiButton variant="ghost" @click="closeCreateDialog">取消</UiButton>
        <UiButton variant="primary" :loading="creating" :disabled="creating || !createForm.knowledge_base_id" @click="createTask">创建</UiButton>
      </template>
    </UiModal>

    <UiModal v-model="detailVisible" title="采集任务详情" size="xl" @close="closeDetail">
      <UiAlert v-if="detailError" type="error" :title="detailError" />
      <div v-if="detailLoading && !detail" class="detail-loading">正在加载任务详情…</div>
      <template v-else-if="detail">
        <div class="detail-summary">
          <div><span>任务 ID</span><code>{{ detail.task.id }}</code></div>
          <div><span>状态</span><UiTag :variant="statusVariant(detail.task.status)">{{ statusLabel(detail.task.status) }}</UiTag></div>
          <div><span>目标知识库</span><RouterLink :to="`/knowledge/${encodeURIComponent(detail.task.knowledge_base_id)}`">{{ knowledgeBaseName(detail.task.knowledge_base_id) }} · 查看知识库</RouterLink></div>
          <div><span>创建时间</span>{{ formatDate(detail.task.created_at) }}</div>
          <template v-if="detail.run">
            <div><span>最新运行 ID</span><code>{{ detail.run.id }}</code></div>
            <div><span>运行状态</span>{{ statusLabel(detail.run.status) }}</div>
            <div><span>开始时间</span>{{ formatDate(detail.run.started_at) }}</div>
            <div><span>结束时间</span>{{ formatDate(detail.run.completed_at) }}</div>
          </template>
          <div><span>来源进度</span>{{ progressSummary(detail) }}</div>
        </div>
        <UiAlert v-if="detail.task.error_message || detail.run?.error_message" type="error" :title="detail.task.error_message || detail.run?.error_message" />
        <UiAlert v-if="detail.summary?.outcome === 'partial'" type="warning" title="部分来源采集失败">可用来源 {{ detail.summary.usable }} / {{ detail.summary.total }}，请查看各来源原因。</UiAlert>
        <div class="detail-actions">
          <UiButton v-if="detail.task.status === 'draft'" variant="primary" :disabled="pendingTasks.has(detail.task.id)" @click="startTask(detail.task.id)">开始采集</UiButton>
          <UiButton v-if="canCancel(detail.task.status)" variant="danger" :disabled="pendingTasks.has(detail.task.id)" @click="cancelTask(detail.task.id)">取消任务</UiButton>
          <UiButton variant="ghost" :loading="detailLoading" @click="refreshDetail">刷新</UiButton>
        </div>
        <p v-if="!detail.run">尚未开始采集。来源：{{ detail.sources.map(source => source.url).join('、') }}</p>
        <p v-else>展示最新一次运行；执行中每 4 秒更新。</p>
        <h3>来源结果</h3>
        <UiTable :columns="itemColumns" :data="detail.items || []" hover>
          <template #cell-source_url="{ row }"><a :href="safeSourceURL(row.source_url)" target="_blank" rel="noopener noreferrer">{{ row.source_url }}</a><div v-if="row.canonical_url && row.canonical_url !== row.source_url">最终地址：{{ row.canonical_url }}</div><div><small>产物 ID：{{ row.id }}</small></div></template>
          <template #cell-status="{ row }"><UiTag :variant="itemStatusVariant(row.status)">{{ itemStatusLabel(row.status) }}</UiTag></template>
          <template #cell-artifact="{ row }"><div>HTTP {{ row.http_status || '—' }} · {{ row.content_type || '—' }}</div><div v-if="row.normalized_object_key">规范化内容：{{ row.normalized_size || 0 }} 字节</div><details v-if="row.content_hash || row.raw_hash"><summary>内容指纹</summary><code>{{ row.content_hash || row.raw_hash }}</code></details></template>
          <template #cell-knowledge_id="{ row }"><RouterLink v-if="row.knowledge_id" :to="`/knowledge/${encodeURIComponent(detail.task.knowledge_base_id)}`">{{ row.knowledge_id }}</RouterLink><span v-else>—</span></template>
          <template #cell-error_reason="{ row }"><span class="error-reason">{{ row.error_reason || '—' }}</span></template>
          <template #cell-actions="{ row }"><RouterLink v-if="row.normalized_object_key && detail.run" :to="{ path: '/pipelines', query: { source: 'collection', run_id: row.run_id || detail.run.id, item_id: row.id, name: row.canonical_url || row.source_url } }">转入流水线</RouterLink><span v-else>暂无可用产物</span></template>
        </UiTable>
      </template>
      <UiButton v-else-if="detailError" variant="ghost" @click="refreshDetail">重试加载详情</UiButton>
    </UiModal>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { UiAlert, UiButton, UiForm, UiFormItem, UiModal, UiSelect, UiTable, UiTag, UiTextarea } from '@/components'
import type { FormRule } from '@/components/ui/form-context'
import { lifecycleApi, type PublicationTarget } from '@/api/lifecycle'
import { collectionApi, type CollectionTask, type CollectionTaskDetailResponse, type CollectionTaskStatus } from '@/api/collection'
import { useAuthStore } from '@/stores/auth'
import toast from '@/utils/toast'
import { ElMessageBox } from '@/utils/confirm'

const auth = useAuthStore()
const loading = ref(false), creating = ref(false), detailLoading = ref(false)
const createVisible = ref(false), detailVisible = ref(false)
const error = ref(''), detailError = ref(''), knowledgeError = ref('')
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
  return ({ pending: '等待中', running: '执行中', accepted: '已入库', skipped: '已跳过', rejected: '已拒绝', failed: '失败', cancelled: '已取消' } as Record<string, string>)[status] || status
}
function statusVariant(status: string) {
  return (({ draft: 'info', running: 'primary', completed: 'success', failed: 'danger', cancelled: 'warning' } as Record<string, string>)[status] || 'info') as 'info' | 'primary' | 'success' | 'danger' | 'warning'
}
function itemStatusVariant(status: string) {
  return status === 'rejected' ? 'danger' : status === 'skipped' ? 'warning' : statusVariant(status === 'accepted' ? 'completed' : status)
}
function canCancel(status: CollectionTaskStatus) { return status === 'draft' || status === 'running' }
function shortId(id: string) { return id.length > 12 ? `${id.slice(0, 8)}…` : id }
function formatDate(value?: string) { return value ? new Date(value).toLocaleString() : '—' }
function progressLabel(task: CollectionTask) { return summaries[task.id] || (task.status === 'draft' ? '尚未开始' : '打开详情查看') }
function progressSummary(payload: CollectionTaskDetailResponse) {
  const items = payload.items || []
  const counts = payload.summary || {
    total: payload.sources.length,
    accepted: items.filter(item => item.status === 'accepted').length,
    skipped: items.filter(item => item.status === 'skipped').length,
    failed: items.filter(item => item.status === 'failed').length,
    rejected: items.filter(item => item.status === 'rejected').length,
    pending: Math.max(0, payload.sources.length - items.filter(item => item.status !== 'pending').length)
  }
  return `共 ${counts.total} 个 · 入库 ${counts.accepted} · 跳过 ${counts.skipped} · 拒绝 ${counts.rejected} · 失败 ${counts.failed} · 待处理 ${counts.pending}`
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
  } catch (cause) { if (active(requestEpoch)) { error.value = message(cause, '创建失败，可重试'); toast.error(error.value) } }
  finally { if (active(requestEpoch)) creating.value = false }
}
function isHttpsUrl(value: string) { try { const url = new URL(value); return url.protocol === 'https:' && !!url.hostname && !url.username && !url.password } catch { return false } }
function safeSourceURL(value: string) { return isHttpsUrl(value) ? value : undefined }
function resetCreateForm() { ++formGeneration; createForm.knowledge_base_id = ''; createForm.urlsText = ''; createAttempt = undefined; createFormRef.value?.clearValidate() }
function closeCreateDialog() { createVisible.value = false; resetCreateForm() }
function closeDetail() { ++detailRequest; detailVisible.value = false; selectedTaskId.value = ''; detail.value = undefined; detailLoading.value = false; detailError.value = '' }
async function openDetail(id: string) { selectedTaskId.value = id; detail.value = undefined; detailVisible.value = true; await refreshDetail() }
async function refreshDetail() {
  const id = selectedTaskId.value
  if (!id || !detailVisible.value) return
  const requestEpoch = epoch, request = ++detailRequest
  detailLoading.value = true
  detailError.value = ''
  try {
    const payload = (await collectionApi.getById(id)).data
    if (!active(requestEpoch) || request !== detailRequest || id !== selectedTaskId.value) return
    if (!payload) throw new Error('采集任务详情为空')
    detail.value = payload
    summaries[id] = progressSummary(payload)
    const index = tasks.value.findIndex(task => task.id === id)
    if (index >= 0) tasks.value[index] = payload.task
  } catch (cause) {
    if (active(requestEpoch) && request === detailRequest) detailError.value = message(cause, '采集任务详情加载失败')
  } finally { if (active(requestEpoch) && request === detailRequest) detailLoading.value = false }
}
async function startTask(id: string) {
  if (pendingTasks.has(id)) return
  const requestEpoch = epoch
  pendingTasks.add(id)
  try {
    await collectionApi.start(id)
    if (!active(requestEpoch)) return
    toast.success('采集任务已开始')
    await Promise.all([loadTasks(), selectedTaskId.value === id ? refreshDetail() : Promise.resolve()])
  } catch (cause) { if (active(requestEpoch)) { error.value = message(cause, '开始采集失败'); toast.error(error.value) } }
  finally { if (active(requestEpoch)) pendingTasks.delete(id) }
}
async function cancelTask(id: string) {
  if (pendingTasks.has(id)) return
  const requestEpoch = epoch
  pendingTasks.add(id)
  try {
    await ElMessageBox.confirm({ title: '取消采集任务', message: '确定取消此任务？未开始的来源将不会继续采集。', confirmButtonText: '取消任务', cancelButtonText: '返回', type: 'warning' })
    if (!active(requestEpoch)) return
    await collectionApi.cancel(id)
    if (!active(requestEpoch)) return
    toast.success('采集任务已取消')
    await Promise.all([loadTasks(), selectedTaskId.value === id ? refreshDetail() : Promise.resolve()])
  } catch (cause) { if (active(requestEpoch) && cause !== 'cancel') { error.value = message(cause, '取消采集失败'); toast.error(error.value) } }
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
onMounted(() => {
  void Promise.all([loadKnowledgeBases(), loadTasks()])
  timer = setInterval(() => {
    if (detailVisible.value && !detailLoading.value && detail.value?.task.status === 'running') void refreshDetail()
  }, 4000)
})
onBeforeUnmount(() => { disposed = true; ++epoch; if (timer) clearInterval(timer) })
</script>

<style scoped>
.collection-tasks { padding: 24px; background: rgba(0, 0, 0, .2); border: 1px solid rgba(255, 255, 255, .1); border-radius: var(--radius-lg); }
.collection-tasks__header { display: flex; justify-content: space-between; gap: 20px; align-items: flex-start; margin-bottom: 24px; }
.collection-tasks h2 { margin: 0; font-size: 20px; }.collection-tasks p { margin: 8px 0 0; color: var(--color-text-secondary); }
.collection-tasks__toolbar, .actions, .detail-actions { display: flex; align-items: center; gap: 8px; }.collection-tasks__toolbar { margin-bottom: 20px; }
.link { padding: 0; border: 0; background: transparent; color: var(--primary); cursor: pointer; }.create-form { margin-top: 20px; }
.detail-loading { padding: 36px; text-align: center; color: var(--color-text-secondary); }.detail-summary { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; margin-bottom: 18px; }.detail-summary > div { display: flex; flex-direction: column; gap: 6px; }.detail-summary span { color: var(--color-text-secondary); font-size: var(--text-sm); }.collection-tasks code, .collection-tasks a { overflow-wrap: anywhere; }.detail-actions { margin-bottom: 24px; }.error-reason { color: var(--color-text-secondary); overflow-wrap: anywhere; }
@media (max-width: 720px) { .collection-tasks__header { flex-direction: column; }.detail-summary { grid-template-columns: 1fr; } }
</style>
