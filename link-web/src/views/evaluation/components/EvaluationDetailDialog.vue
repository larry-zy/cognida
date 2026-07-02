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

      <!-- 检索指标（任务级聚合，来自 detail.metrics 扁平契约） -->
      <div v-if="hasRetrievalMetrics" class="detail-section">
        <h4>检索指标</h4>
        <div class="metrics-grid">
          <div v-if="detail.metrics?.precision != null" class="metric-item">
            <div class="metric-label">Precision</div>
            <div class="metric-value">{{ formatPercent(detail.metrics.precision) }}</div>
          </div>
          <div v-if="detail.metrics?.recall != null" class="metric-item">
            <div class="metric-label">Recall</div>
            <div class="metric-value">{{ formatPercent(detail.metrics.recall) }}</div>
          </div>
          <div v-if="detail.metrics?.ndcg != null" class="metric-item">
            <div class="metric-label">NDCG</div>
            <div class="metric-value">{{ formatPercent(detail.metrics.ndcg) }}</div>
          </div>
          <div v-if="detail.metrics?.mrr != null" class="metric-item">
            <div class="metric-label">MRR</div>
            <div class="metric-value">{{ formatPercent(detail.metrics.mrr) }}</div>
          </div>
          <div v-if="detail.metrics?.map != null" class="metric-item">
            <div class="metric-label">MAP</div>
            <div class="metric-value">{{ formatPercent(detail.metrics.map) }}</div>
          </div>
        </div>
      </div>

      <!-- 生成指标（任务级聚合，来自 detail.metrics 扁平契约） -->
      <div v-if="hasGenerationMetrics" class="detail-section">
        <h4>生成指标</h4>
        <div class="metrics-grid">
          <div v-if="detail.metrics?.bleu_1 != null" class="metric-item">
            <div class="metric-label">BLEU-1</div>
            <div class="metric-value">{{ formatPercent(detail.metrics.bleu_1) }}</div>
          </div>
          <div v-if="detail.metrics?.bleu_2 != null" class="metric-item">
            <div class="metric-label">BLEU-2</div>
            <div class="metric-value">{{ formatPercent(detail.metrics.bleu_2) }}</div>
          </div>
          <div v-if="detail.metrics?.bleu_4 != null" class="metric-item">
            <div class="metric-label">BLEU-4</div>
            <div class="metric-value">{{ formatPercent(detail.metrics.bleu_4) }}</div>
          </div>
          <div v-if="detail.metrics?.rouge_1 != null" class="metric-item">
            <div class="metric-label">ROUGE-1</div>
            <div class="metric-value">{{ formatPercent(detail.metrics.rouge_1) }}</div>
          </div>
          <div v-if="detail.metrics?.rouge_2 != null" class="metric-item">
            <div class="metric-label">ROUGE-2</div>
            <div class="metric-value">{{ formatPercent(detail.metrics.rouge_2) }}</div>
          </div>
          <div v-if="detail.metrics?.rouge_l != null" class="metric-item">
            <div class="metric-label">ROUGE-L</div>
            <div class="metric-value">{{ formatPercent(detail.metrics.rouge_l) }}</div>
          </div>
        </div>
      </div>

      <!-- RAG 专属指标（忠实度/上下文相关性/噪声比） -->
      <div v-if="hasRagMetrics" class="detail-section">
        <h4>RAG 专属指标</h4>
        <div class="metrics-grid">
          <div v-if="detail.metrics?.faithfulness != null" class="metric-item">
            <div class="metric-label">忠实度</div>
            <div class="metric-value">{{ formatPercent(detail.metrics.faithfulness) }}</div>
          </div>
          <div v-if="detail.metrics?.context_relevance != null" class="metric-item">
            <div class="metric-label">上下文相关性</div>
            <div class="metric-value">{{ formatPercent(detail.metrics.context_relevance) }}</div>
          </div>
          <div v-if="detail.metrics?.noise_ratio != null" class="metric-item">
            <div class="metric-label">噪声比例</div>
            <div class="metric-value">{{ formatPercent(detail.metrics.noise_ratio) }}</div>
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

/** 任务级检索指标是否存在任意值 */
const hasRetrievalMetrics = computed(() => {
  const m = props.detail?.metrics
  return !!m && (m.precision != null || m.recall != null || m.ndcg != null || m.mrr != null || m.map != null)
})

/** 任务级生成指标是否存在任意值 */
const hasGenerationMetrics = computed(() => {
  const m = props.detail?.metrics
  return !!m && (m.bleu_1 != null || m.bleu_2 != null || m.bleu_4 != null ||
    m.rouge_1 != null || m.rouge_2 != null || m.rouge_l != null)
})

/** RAG 专属指标是否存在任意值 */
const hasRagMetrics = computed(() => {
  const m = props.detail?.metrics
  return !!m && (m.faithfulness != null || m.context_relevance != null || m.noise_ratio != null)
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
