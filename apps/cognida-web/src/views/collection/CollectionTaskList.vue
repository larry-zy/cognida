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

    <UiTable :columns="taskColumns" :data="tasks" hover>
      <template #cell-id="{ row }"><button class="link" @click="openDetail(row.id)">{{ shortId(row.id) }}</button></template>
      <template #cell-knowledge_base_id="{ row }">{{ knowledgeBaseName(row.knowledge_base_id) }}</template>
      <template #cell-status="{ row }"><UiTag :variant="statusVariant(row.status)">{{ statusLabel(row.status) }}</UiTag></template>
      <template #cell-progress="{ row }">{{ progressLabel(row) }}</template>
      <template #cell-created_at="{ row }">{{ formatDate(row.created_at) }}</template>
      <template #cell-actions="{ row }">
        <div class="actions">
          <UiButton size="sm" variant="ghost" @click="openDetail(row.id)">详情</UiButton>
          <UiButton v-if="row.status === 'draft'" size="sm" variant="primary" @click="startTask(row.id)">开始</UiButton>
          <UiButton v-if="canCancel(row.status)" size="sm" variant="danger" @click="cancelTask(row.id)">取消</UiButton>
        </div>
      </template>
    </UiTable>

    <UiModal v-model="createVisible" title="创建网页采集任务" size="lg" @close="resetCreateForm">
      <UiAlert type="info" title="采集范围受限">每行填写一个 HTTPS 地址。系统不会递归抓取链接或访问需要登录的网站。</UiAlert>
      <UiForm ref="createFormRef" class="create-form" :model="createForm" :rules="createRules">
        <UiFormItem label="目标知识库" prop="knowledge_base_id">
          <UiSelect v-model="createForm.knowledge_base_id" placeholder="选择知识库" :options="knowledgeBaseOptions" />
        </UiFormItem>
        <UiFormItem label="种子 URL" prop="urlsText">
          <UiTextarea v-model="createForm.urlsText" :rows="7" placeholder="https://docs.example.com/guide\nhttps://docs.example.com/faq" />
        </UiFormItem>
      </UiForm>
      <template #footer>
        <UiButton variant="ghost" @click="closeCreateDialog">取消</UiButton>
        <UiButton variant="primary" :loading="creating" @click="createTask">创建</UiButton>
      </template>
    </UiModal>

    <UiModal v-model="detailVisible" title="采集任务详情" size="xl" @open="refreshDetail">
      <div v-if="detailLoading" class="detail-loading">正在加载任务详情…</div>
      <template v-else-if="detail">
        <div class="detail-summary">
          <div><span>任务 ID</span><code>{{ detail.id }}</code></div>
          <div><span>状态</span><UiTag :variant="statusVariant(detail.status)">{{ statusLabel(detail.status) }}</UiTag></div>
          <div><span>目标知识库</span>{{ knowledgeBaseName(detail.knowledge_base_id) }}</div>
          <div><span>创建时间</span>{{ formatDate(detail.created_at) }}</div>
        </div>
        <div class="detail-actions">
          <UiButton v-if="detail.status === 'draft'" variant="primary" @click="startTask(detail.id)">开始采集</UiButton>
          <UiButton v-if="canCancel(detail.status)" variant="danger" @click="cancelTask(detail.id)">取消任务</UiButton>
          <UiButton variant="ghost" @click="refreshDetail">刷新</UiButton>
        </div>

        <h3>来源结果</h3>
        <UiTable :columns="itemColumns" :data="detail.items || []" hover>
          <template #cell-source_url="{ row }"><a :href="row.source_url" target="_blank" rel="noopener noreferrer">{{ row.source_url }}</a></template>
          <template #cell-status="{ row }"><UiTag :variant="itemStatusVariant(row.status)">{{ itemStatusLabel(row.status) }}</UiTag></template>
          <template #cell-knowledge_id="{ row }">{{ row.knowledge_id || '—' }}</template>
          <template #cell-error_reason="{ row }"><span class="error-reason">{{ row.error_reason || '—' }}</span></template>
        </UiTable>
      </template>
    </UiModal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { UiAlert, UiButton, UiForm, UiFormItem, UiModal, UiSelect, UiTable, UiTag, UiTextarea } from '@/components'
import type { FormRule } from '@/components/ui/form-context'
import { knowledgeApi } from '@/api/knowledge'
import { collectionApi, type CollectionTask, type CollectionTaskDetail, type CollectionTaskStatus } from '@/api/collection'
import type { KnowledgeBase } from '@/types'
import toast from '@/utils/toast'
import { ElMessageBox } from '@/utils/confirm'

const loading = ref(false)
const creating = ref(false)
const detailLoading = ref(false)
const createVisible = ref(false)
const detailVisible = ref(false)
const createFormRef = ref<InstanceType<typeof UiForm>>()
const tasks = ref<CollectionTask[]>([])
const knowledgeBases = ref<KnowledgeBase[]>([])
const detail = ref<CollectionTaskDetail>()
const selectedTaskId = ref('')
const statusFilter = ref<CollectionTaskStatus | ''>('')
const createForm = reactive({ knowledge_base_id: '', urlsText: '' })

const taskColumns = [
  { key: 'id', title: '任务 ID', width: 120 },
  { key: 'knowledge_base_id', title: '目标知识库' },
  { key: 'status', title: '状态', width: 100 },
  { key: 'progress', title: '进度', width: 110 },
  { key: 'created_at', title: '创建时间', width: 170 },
  { key: 'actions', title: '操作', width: 210 }
]
const itemColumns = [
  { key: 'source_url', title: '来源 URL' },
  { key: 'status', title: '结果', width: 100 },
  { key: 'knowledge_id', title: '知识条目', width: 150 },
  { key: 'error_reason', title: '原因', width: 220 }
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
  return ({ draft: '草稿', running: '执行中', completed: '已完成', failed: '失败', cancelled: '已取消' })[status]
}
function itemStatusLabel(status: string) {
  return ({ pending: '等待中', running: '执行中', accepted: '已入库', skipped: '已跳过', rejected: '已拒绝', failed: '失败', cancelled: '已取消' })[status] || status
}
function statusVariant(status: string) {
  return ({ draft: 'info', running: 'primary', completed: 'success', failed: 'danger', cancelled: 'warning' })[status] as 'info' | 'primary' | 'success' | 'danger' | 'warning'
}
function itemStatusVariant(status: string) {
  if (status === 'rejected') return 'danger'
  if (status === 'pending') return 'info'
  return statusVariant(status === 'accepted' ? 'completed' : status)
}
function canCancel(status: CollectionTaskStatus) { return status === 'draft' || status === 'running' }
function shortId(id: string) { return id.length > 12 ? `${id.slice(0, 8)}…` : id }
function formatDate(value?: string) { return value ? new Date(value).toLocaleString() : '—' }
function progressLabel(task: CollectionTask) {
  const finished = (task.completed_count || 0) + (task.failed_count || 0)
  return task.source_count === undefined ? '—' : `${finished} / ${task.source_count}`
}
function knowledgeBaseName(id: string) { return knowledgeBases.value.find(kb => kb.id === id)?.name || id }

async function loadKnowledgeBases() {
  const response = await knowledgeApi.getList()
  knowledgeBases.value = response.data || []
}
async function loadTasks() {
  loading.value = true
  try {
    const response = await collectionApi.list({ page: 1, page_size: 100, status: statusFilter.value || undefined })
    tasks.value = response.data?.tasks || []
  } catch (error) {
    console.error('Failed to load collection tasks:', error)
  } finally { loading.value = false }
}
function urlsFromForm() {
  return [...new Set(createForm.urlsText.split(/\r?\n/).map(url => url.trim()).filter(Boolean))]
}
async function createTask() {
  const valid = await createFormRef.value?.validate()
  const seedUrls = urlsFromForm()
  if (!valid || seedUrls.length === 0) return
  if (seedUrls.some(url => !isHttpsUrl(url))) {
    toast.error('仅允许 HTTPS 地址')
    return
  }
  creating.value = true
  try {
    const response = await collectionApi.create({ knowledge_base_id: createForm.knowledge_base_id, sources: seedUrls })
    toast.success('采集任务已创建')
    closeCreateDialog()
    await loadTasks()
    if (response.data?.id) await openDetail(response.data.id)
  } catch (error) { console.error('Failed to create collection task:', error) } finally { creating.value = false }
}
function isHttpsUrl(value: string) { try { return new URL(value).protocol === 'https:' } catch { return false } }
function resetCreateForm() { createForm.knowledge_base_id = ''; createForm.urlsText = ''; createFormRef.value?.clearValidate() }
function closeCreateDialog() { createVisible.value = false; resetCreateForm() }
async function openDetail(id: string) { selectedTaskId.value = id; detailVisible.value = true; await refreshDetail() }
async function refreshDetail() {
  if (!selectedTaskId.value) return
  detailLoading.value = true
  try {
    const payload = (await collectionApi.getById(selectedTaskId.value)).data
    if (payload) {
      detail.value = { ...payload.task, items: payload.items }
    }
  } catch (error) { console.error('Failed to load collection task:', error) } finally { detailLoading.value = false }
}
async function startTask(id: string) {
  try { await collectionApi.start(id); toast.success('采集任务已开始'); await Promise.all([loadTasks(), refreshDetail()]) } catch (error) { console.error('Failed to start collection task:', error) }
}
async function cancelTask(id: string) {
  try {
    await ElMessageBox.confirm({ title: '取消采集任务', message: '确定取消此任务？未开始的来源将不会继续采集。', confirmButtonText: '取消任务', cancelButtonText: '返回', type: 'warning' })
    await collectionApi.cancel(id)
    toast.success('采集任务已取消')
    await Promise.all([loadTasks(), refreshDetail()])
  } catch (error) { if (error !== 'cancel') console.error('Failed to cancel collection task:', error) }
}
onMounted(async () => { await Promise.all([loadKnowledgeBases(), loadTasks()]) })
</script>

<style scoped>
.collection-tasks { padding: 24px; background: rgba(0, 0, 0, .2); border: 1px solid rgba(255, 255, 255, .1); border-radius: var(--radius-lg); }
.collection-tasks__header { display: flex; justify-content: space-between; gap: 20px; align-items: flex-start; margin-bottom: 24px; }
.collection-tasks h2 { margin: 0; font-size: 20px; }.collection-tasks p { margin: 8px 0 0; color: var(--color-text-secondary); }
.collection-tasks__toolbar, .actions, .detail-actions { display: flex; align-items: center; gap: 8px; }.collection-tasks__toolbar { margin-bottom: 20px; }
.link { padding: 0; border: 0; background: transparent; color: var(--primary); cursor: pointer; }.create-form { margin-top: 20px; }
.detail-loading { padding: 36px; text-align: center; color: var(--color-text-secondary); }.detail-summary { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; margin-bottom: 18px; }.detail-summary > div { display: flex; flex-direction: column; gap: 6px; }.detail-summary span { color: var(--color-text-secondary); font-size: var(--text-sm); }.detail-summary code { overflow-wrap: anywhere; }.detail-actions { margin-bottom: 24px; }.error-reason { color: var(--color-text-secondary); overflow-wrap: anywhere; }
@media (max-width: 720px) { .collection-tasks__header { flex-direction: column; }.detail-summary { grid-template-columns: 1fr; } }
</style>
