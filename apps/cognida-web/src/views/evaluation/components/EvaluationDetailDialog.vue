<template>
  <UiModal v-model="visible" title="测评结果" size="xl" :mask-closable="false">
    <div v-if="detail">
      <!-- 任务信息 -->
      <div class="detail-section">
        <h4>任务信息</h4>
        <UiDescriptions :column="2" border>
          <UiDescriptionsItem label="任务ID">{{ detail.task_id }}</UiDescriptionsItem>
          <UiDescriptionsItem label="数据集id">{{ detail.dataset_id }}</UiDescriptionsItem>
          <UiDescriptionsItem label="数据集名称" :mono="false">
            <button
              v-if="detail.dataset_name || detail.dataset_id"
              type="button"
              class="dataset-name-link"
              title="查看数据集"
              @click.stop="goToDataset(detail.dataset_name, detail.dataset_id)"
            >
              {{ detail.dataset_name || detail.dataset_id }}
            </button>
            <span v-else>-</span>
          </UiDescriptionsItem>
          <UiDescriptionsItem label="评测类型">{{ detail.type?.toUpperCase() }}</UiDescriptionsItem>
          <UiDescriptionsItem label="状态">
            <UiTag :variant="EvaluationStatusType[detail.status]">
              {{ EvaluationStatusText[detail.status] }}
            </UiTag>
          </UiDescriptionsItem>
          <UiDescriptionsItem label="进度">
            {{ detail.success_count + detail.failure_count }} / {{ detail.total_count }}
          </UiDescriptionsItem>
          <UiDescriptionsItem label="成功率">
            {{ detail.total_count > 0 ? ((detail.success_count / detail.total_count) * 100).toFixed(1) : 0 }}%
          </UiDescriptionsItem>
          <UiDescriptionsItem label="创建时间">
            {{ formatTime(detail.created_at) }}
          </UiDescriptionsItem>
          <UiDescriptionsItem label="结束时间">
            {{ (detail.status === 'completed' || detail.status === 'failed') ? formatTime(detail.updated_at) : '-' }}
          </UiDescriptionsItem>
        </UiDescriptions>
      </div>

      <!-- 整体评测指标（任务级聚合，注册表驱动动态渲染，兼容固定字段；
           = 各 QA 行动态 scores 均值 ∪ 后端 detail.metrics 扁平聚合，
           开发者新增 grader 自动出现，无需改前端） -->
      <div class="detail-section" v-if="showAggregate">
        <h4>整体评测指标</h4>
        <div class="metrics-grid">
          <div class="metric-item">
            <div class="metric-label">成功率</div>
            <div class="metric-value">{{ successRate.toFixed(1) }}%</div>
          </div>
          <div v-for="entry in aggregateEntries" :key="entry.key" class="metric-item">
            <div class="metric-label">{{ entry.label }}</div>
            <div class="metric-value">{{ entry.text }}</div>
          </div>
        </div>
      </div>

      <!-- QA 结果 -->
      <div class="detail-section" v-if="detail.qa_results && detail.qa_results.length > 0">
        <QaResultTable :results="detail.qa_results" />
      </div>

      <!-- 运行中状态 -->
      <div v-if="detail.status === 'running'" class="loading-state">
        <UiLoader />
        <span>测评执行中，实时更新中...</span>
        <UiTag variant="info" size="sm">SSE 已连接</UiTag>
      </div>

      <!-- 失败状态 -->
      <div v-if="detail.status === 'failed' && detail.task?.error_message" class="error-state">
        <UiAlert type="error" :closable="false">{{ detail.task.error_message }}</UiAlert>
      </div>
    </div>

    <template #footer>
      <UiButton variant="ghost" @click="handleClose">关闭</UiButton>
      <UiButton v-if="detail?.status === 'running'" variant="primary" @click="emit('refresh')">刷新</UiButton>
    </template>
  </UiModal>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import {
  UiModal,
  UiDescriptions,
  UiDescriptionsItem,
  UiTag,
  UiAlert,
  UiLoader,
  UiButton
} from '@/components'
import QaResultTable from './QaResultTable.vue'
import { formatTime, taskAggregateEntries, taskSuccessRate } from '../evaluation-config'
import {
  EvaluationStatusType,
  EvaluationStatusText,
  type EvaluationDetail
} from '@/types'

const props = defineProps<{
  modelValue: boolean
  detail: EvaluationDetail | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'close'): void
  (e: 'refresh'): void
}>()

const router = useRouter()

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit('update:modelValue', v)
})

/** 任务级聚合指标条目（动态：per-QA 均值 ∪ 后端扁平聚合）。
 *  剔除 success_rate——本弹窗已有独立「成功率」卡片（对所有评测类型通用），避免重复展示。 */
const aggregateEntries = computed(() =>
  taskAggregateEntries(props.detail).filter((e) => e.key !== 'success_rate')
)

/** 任务成功率（百分比数值） */
const successRate = computed(() => taskSuccessRate(props.detail))

/** 有任意聚合指标或已有 QA 结果时展示整体指标区块 */
const showAggregate = computed(
  () => aggregateEntries.value.length > 0 || (props.detail?.qa_results?.length ?? 0) > 0
)

function handleClose() {
  emit('close')
  visible.value = false
}

/** 跳转数据集管理，并把名称 / ID 写入筛选条件 */
async function goToDataset(name?: string, id?: string) {
  const query: Record<string, string> = {}
  if (name?.trim()) query.name = name.trim()
  if (id?.trim()) query.id = id.trim()
  // 先跳转再关弹窗，避免关闭动画打断路由
  await router.push({ path: '/datasets', query })
  handleClose()
}
</script>

<style scoped>
.dataset-name-link {
  padding: 0;
  border: none;
  background: none;
  color: var(--primary, #22d3ee);
  cursor: pointer;
  font: inherit;
  text-decoration: underline;
  text-underline-offset: 2px;
  position: relative;
  z-index: 1;
}

.dataset-name-link:hover {
  opacity: 0.85;
}

.detail-section {
  margin-bottom: 24px;
}

.detail-section h4 {
  font-size: 16px;
  font-weight: 600;
  color: var(--color-text-primary);
  margin-bottom: 12px;
}

.metrics-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}

.metric-item {
  padding: 16px;
  background: rgba(0, 0, 0, 0.25);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 12px;
  text-align: center;
  transition: all var(--transition-base);
}

.metric-item:hover {
  border-color: rgba(34, 211, 238, 0.3);
}

.metric-label {
  font-size: 14px;
  color: var(--color-text-secondary);
  margin-bottom: 8px;
}

.metric-value {
  font-size: 24px;
  font-weight: 600;
  color: #22d3ee;
}

.loading-state {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px;
  color: var(--color-text-muted);
}

.error-state {
  margin-top: 16px;
}
</style>
