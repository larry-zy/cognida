<template>
  <!-- 加载态 -->
  <div v-if="loading" class="evaluation-table__loading">
    <UiLoader text="加载中..." />
  </div>

  <!-- 空态 -->
  <UiEmpty
    v-else-if="evaluations.length === 0"
    description="暂无测评任务"
  >
    <UiButton variant="primary" @click="emit('create')">创建第一个测评任务</UiButton>
  </UiEmpty>

  <!-- 任务列表表格 -->
  <UiTable
    v-else
    class="evaluation-table"
    :columns="columns"
    :data="evaluations"
    stripe
    row-key="task_id"
    @row-click="(row: EvaluationTask) => emit('view', row)"
  >
    <!-- 状态 -->
    <template #cell-status="{ row }: { row: EvaluationTask }">
      <UiTag :variant="EvaluationStatusType[row.status]">
        {{ EvaluationStatusText[row.status] }}
      </UiTag>
    </template>

    <!-- 进度 -->
    <template #cell-progress="{ row }: { row: EvaluationTask }">
      <UiProgress
        :percentage="getProgress(row)"
        :status="progressStatus(row.status)"
      />
    </template>

    <!-- 创建时间 -->
    <template #cell-created_at="{ row }: { row: EvaluationTask }">
      {{ formatTime(row.created_at) }}
    </template>

    <!-- 操作 -->
    <template #cell-actions="{ row }: { row: EvaluationTask }">
      <UiButton variant="ghost" size="sm" @click.stop="emit('view', row)">查看</UiButton>
      <UiButton
        v-if="row.status === 'running'"
        variant="ghost"
        size="sm"
        @click.stop="emit('refresh', row)"
      >
        刷新
      </UiButton>
      <UiButton
        variant="danger"
        size="sm"
        @click.stop="row.id && emit('remove', row.id)"
      >
        删除
      </UiButton>
    </template>
  </UiTable>
</template>

<script setup lang="ts">
import { UiTable, UiTag, UiProgress, UiButton, UiEmpty, UiLoader } from '@/components'
import {
  EvaluationStatusType,
  EvaluationStatusText,
  type EvaluationTask
} from '@/types'
import { getProgress, progressStatus, formatTime } from '../evaluation-config'

defineProps<{
  evaluations: EvaluationTask[]
  loading: boolean
}>()

const emit = defineEmits<{
  (e: 'view', task: EvaluationTask): void
  (e: 'refresh', task: EvaluationTask): void
  (e: 'remove', id: string): void
  (e: 'create'): void
}>()

const columns = [
  { key: 'task_id', title: '任务ID', width: 200 },
  { key: 'dataset_id', title: '数据集', width: 150 },
  { key: 'status', title: '状态', width: 100 },
  { key: 'progress', title: '进度', width: 200 },
  { key: 'created_at', title: '创建时间', width: 180 },
  { key: 'actions', title: '操作', width: 150 }
]
</script>

<style scoped>
.evaluation-table {
  cursor: pointer;
}

/* 保留原表格 hover 观感（青色高亮） */
.evaluation-table :deep(.ui-table__row):hover {
  background-color: rgba(34, 211, 238, 0.05);
}

.evaluation-table__loading {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px;
}
</style>
