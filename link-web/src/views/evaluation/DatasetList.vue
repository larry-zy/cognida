<template>
  <div class="dataset-list-container">
    <div class="header">
      <h2>评测数据集</h2>
      <div class="header-actions">
        <el-button type="default" :icon="Plus" @click="showCreateDialog = true">
          创建数据集
        </el-button>
        <el-button type="primary" :icon="Upload" @click="showUploadDialog = true">
          上传文件
        </el-button>
      </div>
    </div>

    <!-- 筛选器 -->
    <div class="filter-bar">
      <el-radio-group v-model="filterType" @change="handleFilterChange">
        <el-radio-button label="">全部</el-radio-button>
        <el-radio-button label="rag">RAG</el-radio-button>
        <el-radio-button label="qa">QA</el-radio-button>
        <el-radio-button label="agent">Agent</el-radio-button>
      </el-radio-group>
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
            <el-tag :type="getTypeColor(dataset.type || dataset.eval_type)" size="small">
              {{ (dataset.type || dataset.eval_type || '').toUpperCase() }}
            </el-tag>
            <el-tag v-if="dataset.dataset_type" :type="dataset.dataset_type === 'database' ? 'success' : 'info'" size="small">
              {{ dataset.dataset_type === 'database' ? '数据库' : '文件' }}
            </el-tag>
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
            <el-tag v-if="dataset.domain" size="small" type="info">{{ dataset.domain }}</el-tag>
            <el-tag v-if="dataset.language" size="small">{{ dataset.language }}</el-tag>
          </div>
        </div>
        <div v-if="dataset.dataset_type === 'database'" class="card-actions" @click.stop>
          <el-button link type="primary" size="small" @click="editDataset(dataset)">
            <el-icon><Edit /></el-icon> 编辑
          </el-button>
          <el-popconfirm
            title="确定删除此数据集吗？"
            confirm-button-text="确定"
            cancel-button-text="取消"
            @confirm="deleteDataset(dataset.id)"
          >
            <template #reference>
              <el-button link type="danger" size="small">
                <el-icon><Delete /></el-icon> 删除
              </el-button>
            </template>
          </el-popconfirm>
        </div>
      </div>
    </div>

    <el-empty v-if="!loading && filteredDatasets.length === 0" description="暂无数据集">
      <el-button type="primary" @click="showCreateDialog = true">创建第一个数据集</el-button>
    </el-empty>

    <!-- 上传数据集对话框（文件系统） -->
    <el-dialog
      v-model="showUploadDialog"
      title="上传数据集文件"
      width="600px"
      :close-on-click-modal="false"
    >
      <el-alert
        type="info"
        title="数据集格式说明"
        :closable="false"
        style="margin-bottom: 16px"
      >
        <p>数据集应为 JSONL 格式，每行一个 QA 对：</p>
        <pre>{"question": "问题", "answer": "答案", "pids": [1, 2], "passages": ["段落1", "段落2"]}</pre>
      </el-alert>

      <el-form :model="uploadForm" :rules="uploadRules" ref="uploadFormRef" label-width="100px">
        <el-form-item label="数据集ID" prop="dataset_id">
          <el-input v-model="uploadForm.dataset_id" placeholder="请输入数据集ID" />
        </el-form-item>

        <el-form-item label="类型" prop="type">
          <el-select v-model="uploadForm.type" placeholder="请选择类型">
            <el-option label="RAG" value="rag" />
            <el-option label="QA" value="qa" />
            <el-option label="Agent" value="agent" />
          </el-select>
        </el-form-item>

        <el-form-item label="JSONL 文件" prop="file">
          <el-upload
            ref="uploadRef"
            :auto-upload="false"
            :limit="1"
            accept=".jsonl"
            :on-change="handleFileChange"
            :on-exceed="handleExceed"
          >
            <el-button type="primary">选择文件</el-button>
          </el-upload>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="showUploadDialog = false">取消</el-button>
        <el-button type="primary" @click="uploadDataset" :loading="uploading">上传</el-button>
      </template>
    </el-dialog>

    <!-- 创建数据集对话框（数据库） -->
    <el-dialog
      v-model="showCreateDialog"
      title="创建数据集"
      width="600px"
      :close-on-click-modal="false"
    >
      <el-form :model="createForm" :rules="createRules" ref="createFormRef" label-width="100px">
        <el-form-item label="数据集ID" prop="dataset_id">
          <el-input v-model="createForm.dataset_id" placeholder="请输入数据集ID" />
        </el-form-item>

        <el-form-item label="名称" prop="name">
          <el-input v-model="createForm.name" placeholder="请输入数据集名称" />
        </el-form-item>

        <el-form-item label="评测类型" prop="evaluation_type">
          <el-select v-model="createForm.evaluation_type" placeholder="请选择评测类型">
            <el-option label="RAG" value="rag" />
            <el-option label="QA" value="qa" />
            <el-option label="Agent" value="agent" />
          </el-select>
        </el-form-item>

        <el-form-item label="描述">
          <el-input v-model="createForm.description" type="textarea" :rows="3" placeholder="请输入数据集描述" />
        </el-form-item>

        <el-divider>初始样本（可选）</el-divider>

        <el-form-item label="QA 对">
          <el-button @click="addQAPair" :icon="Plus" size="small">添加 QA 对</el-button>
        </el-form-item>

        <div v-for="(qa, index) in createForm.qa_pairs" :key="index" class="qa-pair-item">
          <el-card>
            <template #header>
              <div class="qa-header">
                <span>QA #{{ index + 1 }}</span>
                <el-button link type="danger" :icon="Delete" @click="removeQAPair(index)" />
              </div>
            </template>
            <el-form-item label="问题">
              <el-input v-model="qa.question" placeholder="请输入问题" />
            </el-form-item>
            <el-form-item label="参考答案">
              <el-input v-model="qa.reference_answer" type="textarea" :rows="2" placeholder="请输入参考答案" />
            </el-form-item>
          </el-card>
        </div>
      </el-form>

      <template #footer>
        <el-button @click="showCreateDialog = false">取消</el-button>
        <el-button type="primary" @click="createDataset" :loading="creating">创建</el-button>
      </template>
    </el-dialog>

    <!-- 编辑数据集对话框 -->
    <el-dialog
      v-model="showEditDialog"
      title="编辑数据集"
      width="600px"
      :close-on-click-modal="false"
    >
      <el-form :model="editForm" :rules="editRules" ref="editFormRef" label-width="100px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="editForm.name" placeholder="请输入数据集名称" />
        </el-form-item>

        <el-form-item label="描述">
          <el-input v-model="editForm.description" type="textarea" :rows="3" placeholder="请输入数据集描述" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="showEditDialog = false">取消</el-button>
        <el-button type="primary" @click="updateDataset" :loading="updating">保存</el-button>
      </template>
    </el-dialog>

    <!-- 数据集详情对话框 -->
    <el-dialog
      v-model="showDetailDialog"
      :title="currentDataset?.name || currentDataset?.id"
      width="900px"
    >
      <div v-if="currentDataset" class="dataset-detail">
        <el-descriptions :column="2" border>
          <el-descriptions-item label="ID">{{ currentDataset.id }}</el-descriptions-item>
          <el-descriptions-item label="名称">{{ currentDataset.name || '-' }}</el-descriptions-item>
          <el-descriptions-item label="类型">
            <el-tag :type="getTypeColor(currentDataset.type || currentDataset.eval_type)">
              {{ (currentDataset.type || currentDataset.eval_type || '').toUpperCase() }}
            </el-tag>
            <el-tag v-if="currentDataset.dataset_type" style="margin-left: 8px" :type="currentDataset.dataset_type === 'database' ? 'success' : 'info'">
              {{ currentDataset.dataset_type === 'database' ? '数据库' : '文件' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="版本">{{ currentDataset.version || '-' }}</el-descriptions-item>
          <el-descriptions-item label="QA 数量">{{ currentDataset.qa_count }}</el-descriptions-item>
          <el-descriptions-item label="语言">{{ currentDataset.language || '-' }}</el-descriptions-item>
          <el-descriptions-item label="领域" :span="2">{{ currentDataset.domain || '-' }}</el-descriptions-item>
          <el-descriptions-item label="描述" :span="2">{{ currentDataset.description || '-' }}</el-descriptions-item>
        </el-descriptions>

        <!-- 数据库数据集样本管理 -->
        <template v-if="currentDataset.dataset_type === 'database'">
          <el-divider>样本管理</el-divider>

          <div class="samples-header">
            <h4>样本列表</h4>
            <el-button type="primary" size="small" @click="showAddSampleDialog = true">
              <el-icon><Plus /></el-icon> 添加样本
            </el-button>
          </div>

          <el-table
            :data="samples"
            v-loading="samplesLoading"
            border
            size="small"
            max-height="400"
          >
            <el-table-column prop="id" label="ID" width="60" />
            <el-table-column prop="question" label="问题" min-width="200" show-overflow-tooltip />
            <el-table-column prop="reference_answer" label="参考答案" min-width="200" show-overflow-tooltip />
            <el-table-column label="操作" width="80" fixed="right">
              <template #default="{ row }">
                <el-popconfirm
                  title="确定删除此样本吗？"
                  confirm-button-text="确定"
                  cancel-button-text="取消"
                  @confirm="deleteSample(row.id)"
                >
                  <template #reference>
                    <el-button link type="danger" size="small">删除</el-button>
                  </template>
                </el-popconfirm>
              </template>
            </el-table-column>
          </el-table>

          <el-pagination
            v-if="sampleTotal > 0"
            :current-page="samplePage"
            :page-size="samplePageSize"
            :total="sampleTotal"
            layout="total, prev, pager, next"
            small
            style="margin-top: 12px; justify-content: flex-end"
            @current-change="loadSamples"
          />
        </template>

        <!-- 文件系统数据集样本预览 -->
        <template v-else>
          <div class="preview-section">
            <div class="preview-header">
              <h4>样本预览</h4>
              <el-button link type="primary" @click="loadMorePreview">
                <el-icon><Refresh /></el-icon> 刷新
              </el-button>
            </div>
            <div v-loading="previewLoading" class="preview-list">
              <div v-for="(qa, index) in previewData" :key="index" class="qa-item">
                <div class="qa-question">
                  <el-tag size="small">Q</el-tag>
                  <span>{{ qa.question }}</span>
                </div>
                <div class="qa-answer">
                  <el-tag size="small" type="success">A</el-tag>
                  <span>{{ qa.answer }}</span>
                </div>
                <div v-if="qa.passages && qa.passages.length > 0" class="qa-passages">
                  <el-tag size="small" type="info">检索段落</el-tag>
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
        <el-button @click="showDetailDialog = false">关闭</el-button>
        <el-button type="primary" @click="createEvaluationWithDataset">
          使用此数据集创建评测
        </el-button>
      </template>
    </el-dialog>

    <!-- 添加样本对话框 -->
    <el-dialog
      v-model="showAddSampleDialog"
      title="添加样本"
      width="600px"
      :close-on-click-modal="false"
    >
      <el-form :model="addSampleForm" ref="addSampleFormRef" label-width="100px">
        <el-form-item label="问题" prop="question" :rules="{ required: true, message: '请输入问题' }">
          <el-input v-model="addSampleForm.question" type="textarea" :rows="2" placeholder="请输入问题" />
        </el-form-item>

        <el-form-item label="参考答案" prop="reference_answer" :rules="{ required: true, message: '请输入参考答案' }">
          <el-input v-model="addSampleForm.reference_answer" type="textarea" :rows="3" placeholder="请输入参考答案" />
        </el-form-item>

        <el-form-item label="相关 PID">
          <el-input v-model="relevantPIDsInput" placeholder="多个 PID 用逗号分隔，如: 1,2,3" />
        </el-form-item>

        <el-form-item label="上下文">
          <el-input v-model="addSampleForm.context" type="textarea" :rows="3" placeholder="请输入上下文信息（可选）" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="showAddSampleDialog = false">取消</el-button>
        <el-button type="primary" @click="addSample" :loading="addingSample">添加</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from '@/utils/element'
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
const uploading = ref(false)
const creating = ref(false)
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

// Upload form
const uploadForm = ref({
  dataset_id: '',
  type: 'rag',
  file: null as File | null
})

const uploadRules = {
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

const createRules = {
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

const editRules = {
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
    ElMessage.error(error.message || '加载数据集失败')
  } finally {
    loading.value = false
  }
}

function handleFilterChange() {
  loadDatasets()
}

// Upload dataset (file system)
function handleFileChange(file: any) {
  uploadForm.value.file = file.raw
}

function handleExceed() {
  ElMessage.warning('只能上传一个文件')
}

async function uploadDataset() {
  await uploadFormRef.value?.validate()

  if (!uploadForm.value.file) {
    ElMessage.warning('请选择文件')
    return
  }

  uploading.value = true
  try {
    const text = await uploadForm.value.file.text()
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

    ElMessage.success('数据集上传成功')
    showUploadDialog.value = false
    uploadForm.value = { dataset_id: '', type: 'rag', file: null }
    await loadDatasets()
  } catch (error: any) {
    ElMessage.error(error.message || '上传失败')
  } finally {
    uploading.value = false
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
  await createFormRef.value?.validate()

  // Filter out empty QA pairs
  const validQAPairs = createForm.value.qa_pairs.filter(
    qa => qa.question.trim() && qa.reference_answer.trim()
  )

  creating.value = true
  try {
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
    ElMessage.success('数据集创建成功')
    showCreateDialog.value = false
    createForm.value = {
      dataset_id: '',
      name: '',
      evaluation_type: 'rag',
      description: '',
      qa_pairs: []
    }
    await loadDatasets()
  } catch (error: any) {
    ElMessage.error(error.message || '创建失败')
  } finally {
    creating.value = false
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
  await editFormRef.value?.validate()

  updating.value = true
  try {
    const data: UpdateDatasetRequest = {
      name: editForm.value.name,
      description: editForm.value.description
    }

    await evaluationApi.updateDataset(editForm.value.id, data)
    ElMessage.success('数据集更新成功')
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
    ElMessage.error(error.message || '更新失败')
  } finally {
    updating.value = false
  }
}

// Delete dataset
async function deleteDataset(id: string) {
  try {
    await evaluationApi.deleteDataset(id)
    ElMessage.success('数据集删除成功')
    await loadDatasets()
  } catch (error: any) {
    ElMessage.error(error.message || '删除失败')
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
    ElMessage.error(error.message || '加载样本失败')
  } finally {
    samplesLoading.value = false
  }
}

// Add sample
async function addSample() {
  await addSampleFormRef.value?.validate()

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
    ElMessage.success('样本添加成功')

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
    ElMessage.error(error.message || '添加失败')
  } finally {
    addingSample.value = false
  }
}

// Delete sample
async function deleteSample(sampleId: number) {
  if (!currentDataset.value) return

  try {
    await evaluationApi.deleteSample(currentDataset.value.id, sampleId)
    ElMessage.success('样本删除成功')

    // Reload samples and dataset list
    await loadSamples(samplePage.value)
    await loadDatasets()

    // Update current dataset QA count
    if (currentDataset.value && currentDataset.value.qa_count > 0) {
      currentDataset.value.qa_count--
    }
  } catch (error: any) {
    ElMessage.error(error.message || '删除失败')
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
    ElMessage.error(error.message || '加载预览失败')
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
  background: white;
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
  color: #303133;
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
  border: 1px solid #ebeef5;
  border-radius: 8px;
  padding: 16px;
  cursor: pointer;
  transition: all 0.3s;
}

.dataset-card:hover {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
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
  color: #303133;
  font-size: 16px;
}

.card-body .description {
  color: #606266;
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
  color: #909399;
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
  border-top: 1px solid #ebeef5;
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
}

.preview-list {
  max-height: 400px;
  overflow-y: auto;
}

.qa-item {
  padding: 12px;
  border: 1px solid #ebeef5;
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
  color: #606266;
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

:deep(.el-alert__content) pre {
  background: #f5f7fa;
  padding: 8px;
  border-radius: 4px;
  font-size: 12px;
  overflow-x: auto;
}

:deep(.el-pagination) {
  display: flex;
}
</style>
