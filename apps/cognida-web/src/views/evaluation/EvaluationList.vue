<template>
  <div class="evaluation-list-container">
    <div class="header">
      <h2>Agent 测评</h2>
      <UiButton variant="primary" icon @click="showCreateDialog = true">
        <template #icon><el-icon><Plus /></el-icon></template>
        创建测评任务
      </UiButton>
    </div>

    <!-- 统计卡片 -->
    <EvaluationStatsCards :stats="stats" />

    <!-- 筛选 -->
    <div class="filter-bar">
      <UiSelect
        v-model="filters.status"
        placeholder="任务类型"
        clearable
        style="width: 160px"
        :options="statusFilterOptions"
        @update:modelValue="handleSearch"
      />
      <UiInput
        v-model="filters.task_id"
        placeholder="任务 ID"
        clearable
        style="width: 200px"
        @keyup.enter="handleSearch"
      />
      <UiInput
        v-model="filters.dataset_id"
        placeholder="数据集 ID"
        clearable
        style="width: 180px"
        @keyup.enter="handleSearch"
      />
      <UiInput
        v-model="filters.dataset_name"
        placeholder="数据集名称"
        clearable
        style="width: 180px"
        @keyup.enter="handleSearch"
      />
      <UiButton variant="primary" @click="handleSearch">查询</UiButton>
      <UiButton variant="ghost" @click="resetFilters">重置</UiButton>
    </div>

    <!-- 任务列表 -->
    <EvaluationTable
      :evaluations="evaluations"
      :loading="loading"
      @view="handleView"
      @refresh="refreshDetail"
      @remove="deleteEvaluation"
      @create="showCreateDialog = true"
    />

    <!-- 创建测评对话框 -->
    <CreateEvaluationDialog
      v-model="showCreateDialog"
      :datasets="datasets"
      :knowledge-bases="knowledgeBases"
      :chat-models="chatModels"
      :agents="agents"
      :datasources="datasources"
      :creating="creating"
      @submit="handleCreate"
    />

    <!-- 测评结果对话框 -->
    <EvaluationDetailDialog
      v-model="showDetailDialog"
      :detail="currentDetail"
      @close="disconnectSSE"
      @refresh="refreshCurrentDetail"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Plus } from '@element-plus/icons-vue'
import { UiButton, UiInput, UiSelect } from '@/components'
import type { CreateEvaluationRequest } from '@/types'
import { useEvaluationList } from './composables/useEvaluationList'
import EvaluationStatsCards from './components/EvaluationStatsCards.vue'
import EvaluationTable from './components/EvaluationTable.vue'
import CreateEvaluationDialog from './components/CreateEvaluationDialog.vue'
import EvaluationDetailDialog from './components/EvaluationDetailDialog.vue'

const showCreateDialog = ref(false)
const showDetailDialog = ref(false)

const statusFilterOptions = [
  { label: '全部', value: '' },
  { label: '执行中', value: 'running' },
  { label: '已完成', value: 'completed' },
  { label: '失败', value: 'failed' }
]

const {
  loading,
  creating,
  evaluations,
  filters,
  datasets,
  knowledgeBases,
  chatModels,
  agents,
  datasources,
  currentDetail,
  stats,
  loadAll,
  handleSearch,
  resetFilters,
  submitEvaluation,
  viewDetail,
  refreshDetail,
  refreshCurrentDetail,
  deleteEvaluation,
  disconnectSSE
} = useEvaluationList()

// 点击“查看”：加载详情并打开结果对话框
async function handleView(task: Parameters<typeof viewDetail>[0]): Promise<void> {
  await viewDetail(task)
  if (currentDetail.value) showDetailDialog.value = true
}

// 创建成功后关闭创建框，并打开详情框
async function handleCreate(request: CreateEvaluationRequest): Promise<void> {
  const ok = await submitEvaluation(request)
  if (ok) {
    showCreateDialog.value = false
    if (currentDetail.value) showDetailDialog.value = true
  }
}

onMounted(() => {
  loadAll()
})

onUnmounted(() => {
  disconnectSSE()
})
</script>

<style scoped>
.evaluation-list-container {
  padding: 24px;
  background: rgba(0, 0, 0, 0.2);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-lg);
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.filter-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
  margin-bottom: 20px;
}

.header h2 {
  font-size: 20px;
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}
</style>
