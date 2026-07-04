<template>
  <div class="qa-result">
    <!-- 头部：标题 + 展开全部 -->
    <div class="qa-result__header">
      <h4>QA 结果详情 ({{ results.length }} 条)</h4>
      <UiButton size="sm" variant="ghost" @click="toggleAll">
        {{ allExpanded ? '收起全部' : '展开全部' }}
      </UiButton>
    </div>

    <!-- 自定义可展开表格（UiTable 不支持 expand row，故自绘） -->
    <div class="qa-table">
      <!-- 表头 -->
      <div class="qa-table__head">
        <span class="col-toggle"></span>
        <span class="col-question">问题</span>
        <span class="col-status">状态</span>
        <span class="col-metric">ROUGE-1</span>
        <span class="col-metric">BLEU-1</span>
        <span class="col-metric">LLM评分</span>
      </div>

      <!-- 数据行 -->
      <div v-for="(row, index) in results" :key="row.id ?? index" class="qa-table__group">
        <!-- 摘要行 -->
        <div
          class="qa-table__row"
          :class="{ 'is-expanded': isExpanded(row, index) }"
          @click="toggle(row, index)"
        >
          <span class="col-toggle">
            <span class="qa-caret" :class="{ 'qa-caret--open': isExpanded(row, index) }">▶</span>
          </span>
          <span class="col-question" :title="row.question">{{ row.question }}</span>
          <span class="col-status">
            <UiTag :variant="row.success ? 'success' : 'danger'" size="sm">
              {{ row.success ? '成功' : '失败' }}
            </UiTag>
          </span>
          <span class="col-metric">{{ pct(metricValue(row, 'rouge_1'), 1) }}</span>
          <span class="col-metric">{{ pct(metricValue(row, 'bleu_1'), 1) }}</span>
          <span class="col-metric">{{ llmScoreText(row) }}</span>
        </div>

        <!-- 展开详情 -->
        <div v-if="isExpanded(row, index)" class="qa-detail-content">
          <div class="qa-detail-section">
            <h5>问题</h5>
            <p>{{ row.question }}</p>
          </div>
          <div class="qa-detail-section">
            <h5>参考答案</h5>
            <p>{{ row.reference_answer }}</p>
          </div>
          <div class="qa-detail-section">
            <h5>生成答案</h5>
            <p>{{ row.generated_answer }}</p>
          </div>

          <div class="qa-detail-section" v-if="hasMetrics(row)">
            <h5>评测指标</h5>
            <div class="qa-metrics-grid">
              <!-- 按动态 scores map 渲染，兼容固定字段；开发者新增指标自动出现 -->
              <div
                v-for="entry in metricEntries(row)"
                :key="entry.key"
                class="qa-metric-item"
              >
                <span class="metric-label">{{ entry.label }}:</span>
                <span class="metric-value">{{ entry.text }}</span>
              </div>
            </div>
          </div>

          <div class="qa-detail-section" v-if="row.llm_reasoning">
            <h5>AI 评分理由</h5>
            <p class="reasoning-text">{{ row.llm_reasoning }}</p>
          </div>

          <div class="qa-detail-section" v-if="row.error">
            <h5>错误信息</h5>
            <UiAlert type="error" :closable="false">{{ row.error }}</UiAlert>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { UiTag, UiButton, UiAlert } from '@/components'
import { hasMetrics, metricEntries, metricValue } from '../evaluation-config'

const props = defineProps<{ results: Array<Record<string, any>> }>()

// 记录已展开行的标识（无 id 时退回索引）
const expandedKeys = ref<Set<string | number>>(new Set())

function rowKey(row: Record<string, any>, index: number): string | number {
  return row.id ?? index
}

function isExpanded(row: Record<string, any>, index: number): boolean {
  return expandedKeys.value.has(rowKey(row, index))
}

function toggle(row: Record<string, any>, index: number): void {
  const key = rowKey(row, index)
  const next = new Set(expandedKeys.value)
  if (next.has(key)) {
    next.delete(key)
  } else {
    next.add(key)
  }
  expandedKeys.value = next
}

const allExpanded = computed(
  () => props.results.length > 0 && expandedKeys.value.size === props.results.length
)

function toggleAll(): void {
  if (allExpanded.value) {
    expandedKeys.value = new Set()
  } else {
    expandedKeys.value = new Set(props.results.map((r, i) => rowKey(r, i)))
  }
}

/** 0~1 数值转百分比，空值返回 '-' */
function pct(value: unknown, digits: number): string {
  if (value == null) return '-'
  return (Number(value) * 100).toFixed(digits) + '%'
}

/** 摘要列的 LLM 评分：动态 scores 优先，回退固定 llm_score 字段 */
function llmScoreText(row: Record<string, any>): string {
  const v = metricValue(row, 'llm_score')
  return v != null ? v.toFixed(1) : '-'
}
</script>

<style scoped>
.qa-result__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.qa-result__header h4 {
  font-size: 16px;
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.qa-table {
  max-height: 400px;
  overflow: auto;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 12px;
}

.qa-table__head,
.qa-table__row {
  display: grid;
  grid-template-columns: 44px minmax(200px, 1fr) 80px 90px 90px 90px;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
}

.qa-table__head {
  position: sticky;
  top: 0;
  z-index: 1;
  background: rgba(0, 0, 0, 0.35);
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-secondary);
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.qa-table__row {
  cursor: pointer;
  font-size: 14px;
  color: var(--color-text-primary);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  transition: background-color var(--transition-base);
}

.qa-table__row:hover,
.qa-table__row.is-expanded {
  background-color: rgba(34, 211, 238, 0.05);
}

.col-question {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.col-metric {
  text-align: right;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-secondary);
}

.qa-table__head .col-metric {
  color: var(--color-text-secondary);
}

.col-toggle {
  display: flex;
  align-items: center;
  justify-content: center;
}

.qa-caret {
  display: inline-block;
  font-size: 10px;
  color: #22d3ee;
  transition: transform var(--transition-base);
}

.qa-caret--open {
  transform: rotate(90deg);
}

/* QA 详细内容样式（沿用原页面观感） */
.qa-detail-content {
  padding: 16px 24px;
  background: rgba(0, 0, 0, 0.15);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.qa-detail-section {
  margin-bottom: 16px;
}

.qa-detail-section:last-child {
  margin-bottom: 0;
}

.qa-detail-section h5 {
  font-size: 14px;
  font-weight: 600;
  color: #22d3ee;
  margin-bottom: 8px;
}

.qa-detail-section p {
  font-size: 14px;
  color: var(--color-text-secondary);
  line-height: 1.6;
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
}

.qa-metrics-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 12px;
}

.qa-metric-item {
  padding: 10px 14px;
  background: rgba(0, 0, 0, 0.25);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 8px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.qa-metric-item .metric-label {
  font-size: 13px;
  color: var(--color-text-secondary);
  font-weight: 500;
}

.qa-metric-item .metric-value {
  font-size: 16px;
  color: #22d3ee;
  font-weight: 600;
}

.reasoning-text {
  font-size: 13px !important;
  color: var(--color-text-secondary) !important;
  background: rgba(0, 0, 0, 0.2);
  padding: 12px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.1);
}
</style>
