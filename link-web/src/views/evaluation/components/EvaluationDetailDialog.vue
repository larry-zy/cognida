<template>
  <UiModal v-model="visible" title="测评结果" size="xl" :mask-closable="false">
    <div v-if="detail">
      <!-- 任务信息 -->
      <div class="detail-section">
        <h4>任务信息</h4>
        <UiDescriptions :column="2" border>
          <UiDescriptionsItem label="任务ID">{{ detail.task_id }}</UiDescriptionsItem>
          <UiDescriptionsItem label="数据集">{{ detail.dataset_id }}</UiDescriptionsItem>
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

      <!-- 整体评测指标 -->
      <div class="detail-section" v-if="aggregateMetrics">
        <h4>整体评测指标</h4>
        <div class="metrics-grid">
          <div class="metric-item">
            <div class="metric-label">成功率</div>
            <div class="metric-value">{{ aggregateMetrics.successRate.toFixed(1) }}%</div>
          </div>
          <div v-if="aggregateMetrics.rouge1 !== null" class="metric-item">
            <div class="metric-label">ROUGE-1 (平均)</div>
            <div class="metric-value">{{ (aggregateMetrics.rouge1 * 100).toFixed(2) }}%</div>
          </div>
          <div v-if="aggregateMetrics.rougeL !== null" class="metric-item">
            <div class="metric-label">ROUGE-L (平均)</div>
            <div class="metric-value">{{ (aggregateMetrics.rougeL * 100).toFixed(2) }}%</div>
          </div>
          <div v-if="aggregateMetrics.bleu1 !== null" class="metric-item">
            <div class="metric-label">BLEU-1 (平均)</div>
            <div class="metric-value">{{ (aggregateMetrics.bleu1 * 100).toFixed(2) }}%</div>
          </div>
          <div v-if="aggregateMetrics.precision !== null" class="metric-item">
            <div class="metric-label">Precision (平均)</div>
            <div class="metric-value">{{ (aggregateMetrics.precision * 100).toFixed(2) }}%</div>
          </div>
          <div v-if="aggregateMetrics.recall !== null" class="metric-item">
            <div class="metric-label">Recall (平均)</div>
            <div class="metric-value">{{ (aggregateMetrics.recall * 100).toFixed(2) }}%</div>
          </div>
          <div v-if="aggregateMetrics.llmScore !== null" class="metric-item">
            <div class="metric-label">LLM 评分 (平均)</div>
            <div class="metric-value">{{ aggregateMetrics.llmScore.toFixed(2) }}</div>
          </div>
          <div v-if="aggregateMetrics.semanticSim !== null" class="metric-item">
            <div class="metric-label">语义相似度 (平均)</div>
            <div class="metric-value">{{ (aggregateMetrics.semanticSim * 100).toFixed(2) }}%</div>
          </div>
        </div>
      </div>

      <!-- QA 结果 -->
      <div class="detail-section" v-if="detail.qa_results && detail.qa_results.length > 0">
        <QaResultTable :results="detail.qa_results" />
      </div>

      <!-- 检索指标 -->
      <div v-if="detail.metric?.retrieval_metrics" class="detail-section">
        <h4>检索指标</h4>
        <div class="metrics-grid">
          <div class="metric-item">
            <div class="metric-label">Precision</div>
            <div class="metric-value">{{ formatPercent(detail.metric.retrieval_metrics.precision) }}</div>
          </div>
          <div class="metric-item">
            <div class="metric-label">Recall</div>
            <div class="metric-value">{{ formatPercent(detail.metric.retrieval_metrics.recall) }}</div>
          </div>
          <div class="metric-item">
            <div class="metric-label">NDCG@3</div>
            <div class="metric-value">{{ formatPercent(detail.metric.retrieval_metrics.ndcg3) }}</div>
          </div>
          <div class="metric-item">
            <div class="metric-label">NDCG@10</div>
            <div class="metric-value">{{ formatPercent(detail.metric.retrieval_metrics.ndcg10) }}</div>
          </div>
          <div class="metric-item">
            <div class="metric-label">MRR</div>
            <div class="metric-value">{{ formatPercent(detail.metric.retrieval_metrics.mrr) }}</div>
          </div>
          <div class="metric-item">
            <div class="metric-label">MAP</div>
            <div class="metric-value">{{ formatPercent(detail.metric.retrieval_metrics.map) }}</div>
          </div>
        </div>
      </div>

      <!-- 生成指标 -->
      <div v-if="detail.metric?.generation_metrics" class="detail-section">
        <h4>生成指标</h4>
        <div class="metrics-grid">
          <div class="metric-item">
            <div class="metric-label">BLEU-1</div>
            <div class="metric-value">{{ formatPercent(detail.metric.generation_metrics.bleu1) }}</div>
          </div>
          <div class="metric-item">
            <div class="metric-label">BLEU-2</div>
            <div class="metric-value">{{ formatPercent(detail.metric.generation_metrics.bleu2) }}</div>
          </div>
          <div class="metric-item">
            <div class="metric-label">BLEU-4</div>
            <div class="metric-value">{{ formatPercent(detail.metric.generation_metrics.bleu4) }}</div>
          </div>
          <div class="metric-item">
            <div class="metric-label">ROUGE-1</div>
            <div class="metric-value">{{ formatPercent(detail.metric.generation_metrics.rouge1) }}</div>
          </div>
          <div class="metric-item">
            <div class="metric-label">ROUGE-2</div>
            <div class="metric-value">{{ formatPercent(detail.metric.generation_metrics.rouge2) }}</div>
          </div>
          <div class="metric-item">
            <div class="metric-label">ROUGE-L</div>
            <div class="metric-value">{{ formatPercent(detail.metric.generation_metrics.rougeL) }}</div>
          </div>
        </div>
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
import { formatTime, formatPercent, type AggregateMetrics } from '../evaluation-config'
import {
  EvaluationStatusType,
  EvaluationStatusText,
  type EvaluationDetail
} from '@/types'

const props = defineProps<{
  modelValue: boolean
  detail: EvaluationDetail | null
  aggregateMetrics: AggregateMetrics | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'close'): void
  (e: 'refresh'): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit('update:modelValue', v)
})

function handleClose() {
  emit('close')
  visible.value = false
}
</script>

<style scoped>
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
