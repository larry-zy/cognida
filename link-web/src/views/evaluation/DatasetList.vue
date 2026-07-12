<template>
  <div class="dataset-list-container">
    <div class="header">
      <h2>评测数据集</h2>
      <div class="header-actions">
        <UiButton variant="secondary" icon @click="showCreateDialog = true">
          <template #icon><el-icon><Plus /></el-icon></template>
          创建数据集
        </UiButton>
        <UiButton variant="primary" icon @click="showUploadDialog = true">
          <template #icon><el-icon><Upload /></el-icon></template>
          上传文件
        </UiButton>
      </div>
    </div>

    <!-- 筛选器 -->
    <div class="filter-bar">
      <UiRadioGroup
        v-model="filterType"
        :options="[
          { label: '全部', value: '' },
          { label: 'RAG', value: 'rag' },
          { label: 'QA', value: 'qa' },
          { label: 'Agent', value: 'agent' }
        ]"
        @update:modelValue="handleFilterChange"
      />
    </div>

    <!-- 数据集列表 -->
    <div v-loading="loading" class="dataset-grid">
      <div
        v-for="dataset in filteredDatasets"
        :key="dataset.id"
        class="dataset-card"
        @click="viewDataset(dataset)"
      >
        <div class="card-header">
          <div class="card-badges">
            <UiTag :variant="getTypeColor(dataset.type || dataset.eval_type)" size="sm">
              {{ (dataset.type || dataset.eval_type || '').toUpperCase() }}
            </UiTag>
            <UiTag v-if="dataset.dataset_type" :variant="dataset.dataset_type === 'database' ? 'success' : 'info'" size="sm">
              {{ dataset.dataset_type === 'database' ? '数据库' : '文件' }}
            </UiTag>
          </div>
          <span class="dataset-name">{{ dataset.name || dataset.id }}</span>
        </div>
        <div class="card-body">
          <p v-if="dataset.description" class="description">{{ dataset.description }}</p>
          <div class="meta-info">
            <span><el-icon><Document /></el-icon> {{ dataset.qa_count }} QA</span>
            <span v-if="dataset.version"><el-icon><PriceTag /></el-icon> {{ dataset.version }}</span>
          </div>
          <div v-if="dataset.domain || dataset.language" class="tags">
            <UiTag v-if="dataset.domain" size="sm" variant="info">{{ dataset.domain }}</UiTag>
            <UiTag v-if="dataset.language" size="sm">{{ dataset.language }}</UiTag>
          </div>
        </div>
        <div v-if="dataset.dataset_type === 'database'" class="card-actions" @click.stop>
          <UiButton variant="ghost" size="sm" icon @click="editDataset(dataset)">
            <template #icon><el-icon><Edit /></el-icon></template>
            编辑
          </UiButton>
          <UiButton variant="danger" size="sm" icon @click="confirmDeleteDataset(dataset.id)">
            <template #icon><el-icon><Delete /></el-icon></template>
            删除
          </UiButton>
        </div>
      </div>
    </div>

    <UiEmpty v-if="!loading && filteredDatasets.length === 0" description="暂无数据集">
      <UiButton variant="primary" @click="showCreateDialog = true">创建第一个数据集</UiButton>
    </UiEmpty>

    <!-- 上传数据集对话框（文件系统） -->
    <UiModal
      v-model="showUploadDialog"
      title="上传数据集文件"
      size="md"
      :mask-closable="false"
    >
      <UiAlert
        type="info"
        title="数据集格式说明"
        :closable="false"
        style="margin-bottom: 16px"
      >
        <p>数据集应为 JSONL 格式，每行一个 QA 对：</p>
        <pre>{"question": "问题", "answer": "答案", "pids": [1, 2], "passages": ["段落1", "段落2"]}</pre>
      </UiAlert>

      <UiForm :model="uploadForm" :rules="uploadRules" ref="uploadFormRef" label-width="100px">
        <UiFormItem label="数据集ID" prop="dataset_id">
          <UiInput v-model="uploadForm.dataset_id" placeholder="请输入数据集ID" />
        </UiFormItem>

        <UiFormItem label="类型" prop="type">
          <UiSelect
            v-model="uploadForm.type"
            placeholder="请选择类型"
            :options="[
              { label: 'RAG', value: 'rag' },
              { label: 'QA', value: 'qa' },
              { label: 'Agent', value: 'agent' }
            ]"
          />
        </UiFormItem>

        <UiFormItem label="JSONL 文件" prop="file">
          <input
            ref="fileInputRef"
            type="file"
            accept=".jsonl"
            style="display: none"
            @change="handleFileChange"
          />
          <UiButton variant="primary" @click="fileInputRef?.click()">选择文件</UiButton>
          <span v-if="selectedFileName" class="upload-filename">{{ selectedFileName }}</span>
        </UiFormItem>
      </UiForm>

      <template #footer>
        <div class="footer-with-status">
          <UiAsyncStatus
            :status="uploadTask.status.value"
            :message="uploadTask.message.value"
            @retry="uploadDataset"
          />
          <UiButton variant="secondary" @click="showUploadDialog = false">取消</UiButton>
          <UiButton variant="primary" @click="uploadDataset" :loading="uploadTask.isPending.value">上传</UiButton>
        </div>
      </template>
    </UiModal>

    <!-- 创建数据集对话框（数据库） -->
    <UiModal
      v-model="showCreateDialog"
      title="创建数据集"
      size="md"
      :mask-closable="false"
    >
      <UiForm :model="createForm" :rules="createRules" ref="createFormRef" label-width="100px">
        <UiFormItem label="数据集ID" prop="dataset_id">
          <UiInput v-model="createForm.dataset_id" placeholder="请输入数据集ID" />
        </UiFormItem>

        <UiFormItem label="名称" prop="name">
          <UiInput v-model="createForm.name" placeholder="请输入数据集名称" />
        </UiFormItem>

        <UiFormItem label="评测类型" prop="evaluation_type">
          <UiSelect
            v-model="createForm.evaluation_type"
            placeholder="请选择评测类型"
            :options="[
              { label: 'RAG', value: 'rag' },
              { label: 'QA', value: 'qa' },
              { label: 'Agent', value: 'agent' }
            ]"
          />
        </UiFormItem>

        <UiFormItem label="描述">
          <UiTextarea v-model="createForm.description" :rows="3" placeholder="请输入数据集描述" />
        </UiFormItem>

        <UiDivider>初始样本（可选）</UiDivider>

        <UiFormItem label="QA 对">
          <UiButton variant="secondary" size="sm" icon @click="addQAPair">
            <template #icon><el-icon><Plus /></el-icon></template>
            添加 QA 对
          </UiButton>
        </UiFormItem>

        <div v-for="(qa, index) in createForm.qa_pairs" :key="index" class="qa-pair-item">
          <UiCard>
            <template #header>
              <div class="qa-header">
                <span>QA #{{ index + 1 }}</span>
                <UiButton variant="danger" size="sm" icon @click="removeQAPair(index)">
                  <template #icon><el-icon><Delete /></el-icon></template>
                </UiButton>
              </div>
            </template>
            <UiFormItem label="问题">
              <UiInput v-model="qa.question" placeholder="请输入问题" />
            </UiFormItem>
            <UiFormItem label="参考答案">
              <UiTextarea v-model="qa.reference_answer" :rows="2" placeholder="请输入参考答案" />
            </UiFormItem>
          </UiCard>
        </div>
      </UiForm>

      <template #footer>
        <div class="footer-with-status">
          <UiAsyncStatus
            :status="createTask.status.value"
            :message="createTask.message.value"
            @retry="createDataset"
          />
          <UiButton variant="secondary" @click="showCreateDialog = false">取消</UiButton>
          <UiButton variant="primary" @click="createDataset" :loading="createTask.isPending.value">创建</UiButton>
        </div>
      </template>
    </UiModal>

    <!-- 编辑数据集对话框 -->
    <UiModal
      v-model="showEditDialog"
      title="编辑数据集"
      size="md"
      :mask-closable="false"
    >
      <UiForm :model="editForm" :rules="editRules" ref="editFormRef" label-width="100px">
        <UiFormItem label="名称" prop="name">
          <UiInput v-model="editForm.name" placeholder="请输入数据集名称" />
        </UiFormItem>

        <UiFormItem label="描述">
          <UiTextarea v-model="editForm.description" :rows="3" placeholder="请输入数据集描述" />
        </UiFormItem>
      </UiForm>

      <template #footer>
        <UiButton variant="secondary" @click="showEditDialog = false">取消</UiButton>
        <UiButton variant="primary" @click="updateDataset" :loading="updating">保存</UiButton>
      </template>
    </UiModal>

    <!-- 数据集详情对话框 -->
    <UiModal
      v-model="showDetailDialog"
      :title="currentDataset?.name || currentDataset?.id"
      size="lg"
    >
      <div v-if="currentDataset" class="dataset-detail">
        <UiDescriptions :column="2" border>
          <UiDescriptionsItem label="ID">{{ currentDataset.id }}</UiDescriptionsItem>
          <UiDescriptionsItem label="名称">{{ currentDataset.name || '-' }}</UiDescriptionsItem>
          <UiDescriptionsItem label="类型">
            <UiTag :variant="getTypeColor(currentDataset.type || currentDataset.eval_type)">
              {{ (currentDataset.type || currentDataset.eval_type || '').toUpperCase() }}
            </UiTag>
            <UiTag v-if="currentDataset.dataset_type" style="margin-left: 8px" :variant="currentDataset.dataset_type === 'database' ? 'success' : 'info'">
              {{ currentDataset.dataset_type === 'database' ? '数据库' : '文件' }}
            </UiTag>
          </UiDescriptionsItem>
          <UiDescriptionsItem label="版本">{{ currentDataset.version || '-' }}</UiDescriptionsItem>
          <UiDescriptionsItem label="QA 数量">{{ currentDataset.qa_count }}</UiDescriptionsItem>
          <UiDescriptionsItem label="语言">{{ currentDataset.language || '-' }}</UiDescriptionsItem>
          <UiDescriptionsItem label="领域" :span="2">{{ currentDataset.domain || '-' }}</UiDescriptionsItem>
          <UiDescriptionsItem label="描述" :span="2">{{ currentDataset.description || '-' }}</UiDescriptionsItem>
        </UiDescriptions>

        <!-- 数据库数据集样本管理 -->
        <template v-if="currentDataset.dataset_type === 'database'">
          <UiDivider>样本管理</UiDivider>

          <div class="samples-header">
            <h4>样本列表</h4>
            <UiButton variant="primary" size="sm" icon @click="showAddSampleDialog = true">
              <template #icon><el-icon><Plus /></el-icon></template>
              添加样本
            </UiButton>
          </div>

          <UiTable
            :data="samples"
            border
            size="sm"
            row-key="id"
            :columns="[
              { key: 'id', title: 'ID', width: 60 },
              { key: 'question', title: '问题' },
              { key: 'reference_answer', title: '参考答案' },
              { key: 'actions', title: '操作', width: 80, align: 'right' }
            ]"
          >
            <template #cell-actions="{ row }">
              <UiButton variant="danger" size="sm" @click.stop="confirmDeleteSample(row.id)">删除</UiButton>
            </template>
          </UiTable>

          <UiPagination
            v-if="sampleTotal > 0"
            :page="samplePage"
            :page-size="samplePageSize"
            :total="sampleTotal"
            show-info
            size="sm"
            style="margin-top: 12px; justify-content: flex-end"
            @change="(page: number) => loadSamples(page)"
          />
        </template>

        <!-- 文件系统数据集样本预览 -->
        <template v-else>
          <div class="preview-section">
            <div class="preview-header">
              <h4>样本预览</h4>
              <UiButton variant="ghost" size="sm" icon @click="loadMorePreview">
                <template #icon><el-icon><Refresh /></el-icon></template>
                刷新
              </UiButton>
            </div>
            <div v-loading="previewLoading" class="preview-list">
              <div v-for="(qa, index) in previewData" :key="index" class="qa-item">
                <div class="qa-question">
                  <UiTag size="sm">Q</UiTag>
                  <span>{{ qa.question }}</span>
                </div>
                <div class="qa-answer">
                  <UiTag size="sm" variant="success">A</UiTag>
                  <span>{{ qa.answer }}</span>
                </div>
                <div v-if="qa.passages && qa.passages.length > 0" class="qa-passages">
                  <UiTag size="sm" variant="info">检索段落</UiTag>
                  <ul>
                    <li v-for="(p, i) in qa.passages.slice(0, 2)" :key="i">{{ p.substring(0, 100) }}...</li>
                  </ul>
                </div>
              </div>
            </div>
          </div>
        </template>
      </div>

      <template #footer>
        <UiButton variant="secondary" @click="showDetailDialog = false">关闭</UiButton>
        <UiButton variant="primary" @click="createEvaluationWithDataset">
          使用此数据集创建评测
        </UiButton>
      </template>
    </UiModal>

    <!-- 添加样本对话框 -->
    <UiModal
      v-model="showAddSampleDialog"
      title="添加样本"
      size="md"
      :mask-closable="false"
    >
      <UiForm :model="addSampleForm" ref="addSampleFormRef" label-width="100px">
        <UiFormItem label="问题" prop="question" :rules="[{ required: true, message: '请输入问题' }]">
          <UiTextarea v-model="addSampleForm.question" :rows="2" placeholder="请输入问题" />
        </UiFormItem>

        <UiFormItem label="参考答案" prop="reference_answer" :rules="[{ required: true, message: '请输入参考答案' }]">
          <UiTextarea v-model="addSampleForm.reference_answer" :rows="3" placeholder="请输入参考答案" />
        </UiFormItem>

        <UiFormItem label="相关 PID">
          <UiInput v-model="relevantPIDsInput" placeholder="多个 PID 用逗号分隔，如: 1,2,3" />
        </UiFormItem>

        <UiFormItem label="上下文">
          <UiTextarea v-model="addSampleForm.context" :rows="3" placeholder="请输入上下文信息（可选）" />
        </UiFormItem>
      </UiForm>

      <template #footer>
        <UiButton variant="secondary" @click="showAddSampleDialog = false">取消</UiButton>
        <UiButton variant="primary" @click="addSample" :loading="addingSample">添加</UiButton>
      </template>
    </UiModal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import toast from '@/utils/toast'
import { ElMessageBox } from '@/utils/confirm'
import {
  UiButton,
  UiRadioGroup,
  UiTag,
  UiEmpty,
  UiModal,
  UiAlert,
  UiForm,
  UiFormItem,
  UiInput,
  UiTextarea,
  UiSelect,
  UiDivider,
  UiCard,
  UiDescriptions,
  UiDescriptionsItem,
  UiTable,
  UiPagination,
  UiAsyncStatus,
  type FormRule
} from '@/components'
import { useAsyncTask } from '@/composables/useAsyncTask'
import {
  Plus,
  Document,
  PriceTag,
  Refresh,
  Upload,
  Edit,
  Delete
} from '@element-plus/icons-vue'
import { evaluationApi } from '@/api/evaluation'
import type {
  DatasetInfo,
  CreateDatasetRequest,
  UpdateDatasetRequest,
  AddSamplesRequest,
  DatasetSample
} from '@/types'

const router = useRouter()

// State
const loading = ref(false)
// 上传/创建数据集状态机：idle→pending→success|error，就地由 <UiAsyncStatus> 常驻展示
const uploadTask = useAsyncTask()
const createTask = useAsyncTask()
const updating = ref(false)
const samplesLoading = ref(false)
const addingSample = ref(false)
const previewLoading = ref(false)

// Dialogs
const showUploadDialog = ref(false)
const showCreateDialog = ref(false)
const showEditDialog = ref(false)
const showDetailDialog = ref(false)
const showAddSampleDialog = ref(false)

// Data
const datasets = ref<DatasetInfo[]>([])
const currentDataset = ref<DatasetInfo | null>(null)
const previewData = ref<any[]>([])
const samples = ref<DatasetSample[]>([])

// Pagination for samples
const samplePage = ref(1)
const samplePageSize = ref(20)
const sampleTotal = ref(0)

// Filter
const filterType = ref('')

// Form refs
const uploadFormRef = ref()
const createFormRef = ref()
const editFormRef = ref()
const addSampleFormRef = ref()

// File input (原生上传替代 el-upload)
const fileInputRef = ref<HTMLInputElement>()
const selectedFileName = ref('')

// Upload form
const uploadForm = ref({
  dataset_id: '',
  type: 'rag',
  file: null as File | null
})

const uploadRules: Record<string, FormRule[]> = {
  dataset_id: [{ required: true, message: '请输入数据集ID', trigger: 'blur' }],
  type: [{ required: true, message: '请选择类型', trigger: 'change' }]
}

// Create form
const createForm = ref<{
  dataset_id: string
  name: string
  evaluation_type: 'rag' | 'qa' | 'agent'
  description?: string
  qa_pairs: Array<{ question: string; reference_answer: string }>
}>({
  dataset_id: '',
  name: '',
  evaluation_type: 'rag',
  description: '',
  qa_pairs: []
})

const createRules: Record<string, FormRule[]> = {
  dataset_id: [{ required: true, message: '请输入数据集ID', trigger: 'blur' }],
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  evaluation_type: [{ required: true, message: '请选择评测类型', trigger: 'change' }]
}

// Edit form
const editForm = ref<{
  id: string
  name: string
  description?: string
}>({
  id: '',
  name: '',
  description: ''
})

const editRules: Record<string, FormRule[]> = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }]
}

// Add sample form
const addSampleForm = ref({
  question: '',
  reference_answer: '',
  relevant_pids: [] as string[],
  context: ''
})

const relevantPIDsInput = ref('')

// Computed
const filteredDatasets = computed(() => {
  if (!filterType.value) return datasets.value
  return datasets.value.filter(d =>
    (d.type || d.eval_type) === filterType.value
  )
})

// Methods
function getTypeColor(type: string) {
  const colors: Record<string, any> = {
    rag: 'primary',
    qa: 'success',
    agent: 'warning'
  }
  return colors[type] || 'info'
}

async function loadDatasets() {
  loading.value = true
  try {
    const params = filterType.value ? { type: filterType.value } : undefined
    const res = await evaluationApi.listDatasets(params)
    if (res.data) {
      datasets.value = res.data.datasets || []
    }
  } catch (error: any) {
    toast.error(error.message || '加载数据集失败')
  } finally {
    loading.value = false
  }
}

function handleFilterChange() {
  loadDatasets()
}

// Upload dataset (file system)
// 原生文件输入替代 el-upload：单文件，每次选择覆盖上一个
function handleFileChange(e: Event) {
  const files = (e.target as HTMLInputElement).files
  if (files && files.length) {
    uploadForm.value.file = files[0]
    selectedFileName.value = files[0].name
  }
}

async function uploadDataset() {
  if (!(await uploadFormRef.value?.validate())) return

  if (!uploadForm.value.file) {
    toast.warning('请选择文件')
    return
  }

  const r = await uploadTask.run(
    async () => {
      const text = await uploadForm.value.file!.text()
      const qapairs = text.split('\n')
        .filter(line => line.trim())
        .map(line => {
          try {
            return JSON.parse(line)
          } catch {
            return null
          }
        })
        .filter(qa => qa !== null)

      await evaluationApi.createDataset({
        dataset_id: uploadForm.value.dataset_id,
        name: uploadForm.value.dataset_id,
        evaluation_type: uploadForm.value.type as 'rag' | 'qa' | 'agent',
        qapairs
      })

      return { count: qapairs.length }
    },
    {
      pendingMessage: '上传中…',
      successMessage: (res) => `成功导入 ${res.count} 条`,
      errorMessage: (e) => e?.message || '上传失败'
    }
  )

  if (r) {
    showUploadDialog.value = false
    uploadForm.value = { dataset_id: '', type: 'rag', file: null }
    selectedFileName.value = ''
    if (fileInputRef.value) fileInputRef.value.value = ''
    await loadDatasets()
  }
}

// Create dataset (database)
function addQAPair() {
  createForm.value.qa_pairs.push({ question: '', reference_answer: '' })
}

function removeQAPair(index: number) {
  createForm.value.qa_pairs.splice(index, 1)
}

async function createDataset() {
  if (!(await createFormRef.value?.validate())) return

  // Filter out empty QA pairs
  const validQAPairs = createForm.value.qa_pairs.filter(
    qa => qa.question.trim() && qa.reference_answer.trim()
  )

  const r = await createTask.run(
    async () => {
      const data: CreateDatasetRequest = {
        dataset_id: createForm.value.dataset_id,
        name: createForm.value.name,
        evaluation_type: createForm.value.evaluation_type,
        description: createForm.value.description
      }

      if (validQAPairs.length > 0) {
        data.qa_pairs = validQAPairs
      }

      await evaluationApi.createDataset(data)
      return { count: validQAPairs.length }
    },
    {
      pendingMessage: '创建中…',
      successMessage: (res) =>
        res.count > 0 ? `数据集创建成功，已导入 ${res.count} 条` : '数据集创建成功',
      errorMessage: (e) => e?.message || '创建失败'
    }
  )

  if (r) {
    showCreateDialog.value = false
    createForm.value = {
      dataset_id: '',
      name: '',
      evaluation_type: 'rag',
      description: '',
      qa_pairs: []
    }
    await loadDatasets()
  }
}

// Edit dataset
function editDataset(dataset: DatasetInfo) {
  editForm.value = {
    id: dataset.id,
    name: dataset.name || '',
    description: dataset.description
  }
  showEditDialog.value = true
}

async function updateDataset() {
  if (!(await editFormRef.value?.validate())) return

  updating.value = true
  try {
    const data: UpdateDatasetRequest = {
      name: editForm.value.name,
      description: editForm.value.description
    }

    await evaluationApi.updateDataset(editForm.value.id, data)
    toast.success('数据集更新成功')
    showEditDialog.value = false
    await loadDatasets()

    // Update current dataset if it's the same
    if (currentDataset.value?.id === editForm.value.id) {
      currentDataset.value = {
        ...currentDataset.value,
        name: data.name,
        description: data.description
      }
    }
  } catch (error: any) {
    toast.error(error.message || '更新失败')
  } finally {
    updating.value = false
  }
}

// Delete dataset
async function confirmDeleteDataset(id: string) {
  try {
    await ElMessageBox.confirm({
      title: '删除确认',
      message: '确定删除此数据集吗？此操作不可恢复。',
      type: 'warning',
      confirmButtonText: '确定删除',
      cancelButtonText: '取消'
    })
    await deleteDataset(id)
  } catch (e) {
    if (e !== 'cancel') throw e
  }
}

async function deleteDataset(id: string) {
  try {
    await evaluationApi.deleteDataset(id)
    toast.success('数据集删除成功')
    await loadDatasets()
  } catch (error: any) {
    toast.error(error.message || '删除失败')
  }
}

// View dataset details
async function viewDataset(dataset: DatasetInfo) {
  currentDataset.value = dataset
  showDetailDialog.value = true

  if (dataset.dataset_type === 'database') {
    await loadSamples()
  } else {
    await loadPreview(dataset.id)
  }
}

// Load samples (database dataset)
async function loadSamples(page: number = 1) {
  if (!currentDataset.value) return

  samplePage.value = page
  samplesLoading.value = true
  try {
    const res = await evaluationApi.listSamples(
      currentDataset.value.id,
      { page, page_size: samplePageSize.value }
    )
    if (res.data) {
      samples.value = res.data.samples || []
      sampleTotal.value = res.data.total || 0
    }
  } catch (error: any) {
    toast.error(error.message || '加载样本失败')
  } finally {
    samplesLoading.value = false
  }
}

// Add sample
async function addSample() {
  if (!(await addSampleFormRef.value?.validate())) return

  // Parse relevant PIDs
  if (relevantPIDsInput.value.trim()) {
    addSampleForm.value.relevant_pids = relevantPIDsInput.value
      .split(',')
      .map(s => s.trim())
      .filter(s => s)
  }

  addingSample.value = true
  try {
    const data: AddSamplesRequest = {
      qa_pairs: [{
        question: addSampleForm.value.question,
        reference_answer: addSampleForm.value.reference_answer,
        relevant_pids: addSampleForm.value.relevant_pids
      }]
    }

    await evaluationApi.addSamples(currentDataset.value!.id, data)
    toast.success('样本添加成功')

    // Reset form
    addSampleForm.value = {
      question: '',
      reference_answer: '',
      relevant_pids: [],
      context: ''
    }
    relevantPIDsInput.value = ''
    showAddSampleDialog.value = false

    // Reload samples and dataset list
    await loadSamples(samplePage.value)
    await loadDatasets()

    // Update current dataset QA count
    if (currentDataset.value) {
      currentDataset.value.qa_count++
    }
  } catch (error: any) {
    toast.error(error.message || '添加失败')
  } finally {
    addingSample.value = false
  }
}

// Delete sample
async function confirmDeleteSample(sampleId: number) {
  try {
    await ElMessageBox.confirm({
      title: '删除确认',
      message: '确定删除此样本吗？此操作不可恢复。',
      type: 'warning',
      confirmButtonText: '确定删除',
      cancelButtonText: '取消'
    })
    await deleteSample(sampleId)
  } catch (e) {
    if (e !== 'cancel') throw e
  }
}

async function deleteSample(sampleId: number) {
  if (!currentDataset.value) return

  try {
    await evaluationApi.deleteSample(currentDataset.value.id, sampleId)
    toast.success('样本删除成功')

    // Reload samples and dataset list
    await loadSamples(samplePage.value)
    await loadDatasets()

    // Update current dataset QA count
    if (currentDataset.value && currentDataset.value.qa_count > 0) {
      currentDataset.value.qa_count--
    }
  } catch (error: any) {
    toast.error(error.message || '删除失败')
  }
}

// Load preview (file system dataset)
async function loadPreview(datasetId: string) {
  previewLoading.value = true
  try {
    const res = await evaluationApi.previewDataset(datasetId, 5)
    if (res.data) {
      previewData.value = res.data.qapairs || []
    }
  } catch (error: any) {
    toast.error(error.message || '加载预览失败')
  } finally {
    previewLoading.value = false
  }
}

async function loadMorePreview() {
  if (currentDataset.value) {
    await loadPreview(currentDataset.value.id)
  }
}

function createEvaluationWithDataset() {
  if (currentDataset.value) {
    showDetailDialog.value = false
    router.push({
      name: 'EvaluationList',
      query: { dataset: currentDataset.value.id }
    })
  }
}

onMounted(() => {
  loadDatasets()
})
</script>

<style scoped>
.dataset-list-container {
  padding: 24px;
  background: var(--bg-primary);
  border-radius: 8px;
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.header h2 {
  font-size: 20px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
}

.header-actions {
  display: flex;
  gap: 12px;
}

.filter-bar {
  margin-bottom: 24px;
}

.dataset-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}

.dataset-card {
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 16px;
  background: var(--bg-secondary);
  cursor: pointer;
  transition: all 0.3s;
}

.dataset-card:hover {
  box-shadow: 0 4px 12px rgba(34, 211, 238, 0.15);
  border-color: var(--primary);
  transform: translateY(-2px);
}

.card-header {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 12px;
}

.card-badges {
  display: flex;
  gap: 6px;
}

.dataset-name {
  font-weight: 600;
  color: var(--text-primary);
  font-size: 16px;
}

.card-body .description {
  color: var(--text-secondary);
  font-size: 14px;
  margin: 0 0 12px 0;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.meta-info {
  display: flex;
  gap: 16px;
  color: var(--text-muted);
  font-size: 13px;
  margin-bottom: 8px;
}

.meta-info span {
  display: flex;
  align-items: center;
  gap: 4px;
}

.tags {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.card-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--border-subtle);
}

.dataset-detail .samples-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.dataset-detail .samples-header h4 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
}

.dataset-detail .preview-section {
  margin-top: 24px;
}

.preview-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.preview-header h4 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
}

.preview-list {
  max-height: 400px;
  overflow-y: auto;
}

.qa-item {
  padding: 12px;
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  margin-bottom: 12px;
}

.qa-question,
.qa-answer {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-bottom: 8px;
}

.qa-question span,
.qa-answer span {
  flex: 1;
  line-height: 1.6;
}

.qa-passages {
  padding-left: 32px;
}

.qa-passages ul {
  margin: 8px 0;
  padding-left: 20px;
}

.qa-passages li {
  font-size: 13px;
  color: var(--text-secondary);
  margin: 4px 0;
}

.qa-pair-item {
  margin-bottom: 12px;
}

.qa-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.qa-header {
  color: var(--text-primary);
}

.footer-with-status {
  display: flex;
  align-items: center;
  gap: 12px;
  justify-content: flex-end;
}

.footer-with-status :deep(.ui-async-status) {
  margin-right: auto;
}

.upload-filename {
  color: var(--color-text-secondary);
  margin-left: 8px;
  font-size: 13px;
}

:deep(.ui-alert) pre {
  background: var(--bg-tertiary);
  padding: 8px;
  border-radius: 4px;
  font-size: 12px;
  color: var(--text-secondary);
  overflow-x: auto;
}
</style>
