<template>
  <div class="agent-step" :class="`agent-step--${statusClass}`">
    <!-- 步骤连接线上的节点 -->
    <div class="agent-step__node">
      <span class="agent-step__dot" :class="`agent-step__dot--${statusClass}`">
        <svg v-if="statusClass === 'success'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
          <path d="M20 6L9 17l-5-5" />
        </svg>
        <svg v-else-if="statusClass === 'error'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
          <path d="M18 6L6 18M6 6l12 12" />
        </svg>
        <span v-else class="agent-step__spinner" />
      </span>
    </div>

    <!-- 步骤主体 -->
    <div class="agent-step__body">
      <button class="agent-step__header" type="button" @click="expanded = !expanded">
        <span class="agent-step__index">#{{ step.step }}</span>
        <span class="agent-step__tool">
          <svg class="agent-step__tool-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M14.7 6.3a4 4 0 0 0-5.4 5.4l-6 6a1.5 1.5 0 0 0 2.1 2.1l6-6a4 4 0 0 0 5.4-5.4l-2.6 2.6-2.1-2.1z" />
          </svg>
          {{ step.tool_name || '工具调用' }}
        </span>
        <span class="agent-step__badge" :class="`agent-step__badge--${statusClass}`">{{ statusText }}</span>
        <svg class="agent-step__chevron" :class="{ 'is-open': expanded }" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M6 9l6 6 6-6" />
        </svg>
      </button>

      <div v-if="expanded" class="agent-step__detail">
        <div v-if="prettyInput" class="agent-step__field">
          <div class="agent-step__field-label">入参</div>
          <pre class="agent-step__code">{{ prettyInput }}</pre>
        </div>
        <div v-if="step.error" class="agent-step__field">
          <div class="agent-step__field-label agent-step__field-label--error">错误</div>
          <pre class="agent-step__code agent-step__code--error">{{ step.error }}</pre>
        </div>
        <div v-else-if="prettyOutput" class="agent-step__field">
          <div class="agent-step__field-label">输出</div>
          <pre class="agent-step__code">{{ prettyOutput }}</pre>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { AgentStep } from '@/types'

const props = defineProps<{ step: AgentStep }>()

const expanded = ref(false)

const statusClass = computed(() => {
  if (props.step.status === 'error' || props.step.type === 'error') return 'error'
  if (props.step.status === 'success') return 'success'
  return 'calling'
})

const statusText = computed(() => {
  switch (statusClass.value) {
    case 'success': return '完成'
    case 'error': return '失败'
    default: return '执行中'
  }
})

// 尝试把 JSON 字符串美化，失败则原样返回
function pretty(raw?: string): string {
  if (!raw) return ''
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

const prettyInput = computed(() => pretty(props.step.tool_input))

const prettyOutput = computed(() => {
  const out = pretty(props.step.tool_output)
  // 输出过长时截断，展开卡片仍可复制核心内容
  if (out.length > 2000) return out.slice(0, 2000) + '\n… (已截断)'
  return out
})
</script>

<style scoped>
.agent-step {
  display: flex;
  gap: var(--space-3);
  position: relative;
}

/* 时间线节点与竖线 */
.agent-step__node {
  position: relative;
  display: flex;
  justify-content: center;
  width: 20px;
  flex: 0 0 20px;
}

.agent-step__node::before {
  content: '';
  position: absolute;
  top: 20px;
  bottom: -8px;
  left: 50%;
  width: 2px;
  transform: translateX(-50%);
  background: var(--border-subtle);
}

.agent-step:last-child .agent-step__node::before {
  display: none;
}

.agent-step__dot {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 20px;
  z-index: 1;
  color: #fff;
}

.agent-step__dot svg {
  width: 12px;
  height: 12px;
}

.agent-step__dot--success { background: var(--color-success, #22c55e); }
.agent-step__dot--error { background: var(--color-danger, #ef4444); }
.agent-step__dot--calling { background: var(--color-primary, #6366f1); }

.agent-step__spinner {
  width: 10px;
  height: 10px;
  border: 2px solid rgba(255, 255, 255, 0.4);
  border-top-color: #fff;
  border-radius: 50%;
  animation: agent-step-spin 0.7s linear infinite;
}

@keyframes agent-step-spin {
  to { transform: rotate(360deg); }
}

/* 主体 */
.agent-step__body {
  flex: 1;
  min-width: 0;
  padding-bottom: var(--space-3);
}

.agent-step__header {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  cursor: pointer;
  text-align: left;
  transition: border-color var(--duration-fast);
}

.agent-step__header:hover { border-color: var(--border-strong); }

.agent-step__index {
  font-size: var(--text-xs);
  color: var(--text-tertiary);
  font-variant-numeric: tabular-nums;
}

.agent-step__tool {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-primary);
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.agent-step__tool-icon { width: 14px; height: 14px; flex: 0 0 14px; color: var(--text-tertiary); }

.agent-step__badge {
  font-size: var(--text-xs);
  padding: 1px 8px;
  border-radius: 999px;
  flex: 0 0 auto;
}

.agent-step__badge--success { background: color-mix(in srgb, var(--color-success, #22c55e) 15%, transparent); color: var(--color-success, #16a34a); }
.agent-step__badge--error { background: color-mix(in srgb, var(--color-danger, #ef4444) 15%, transparent); color: var(--color-danger, #dc2626); }
.agent-step__badge--calling { background: color-mix(in srgb, var(--color-primary, #6366f1) 15%, transparent); color: var(--color-primary, #4f46e5); }

.agent-step__chevron {
  width: 14px;
  height: 14px;
  flex: 0 0 14px;
  color: var(--text-tertiary);
  transition: transform var(--duration-fast);
}

.agent-step__chevron.is-open { transform: rotate(180deg); }

.agent-step__detail {
  margin-top: var(--space-2);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.agent-step__field-label {
  font-size: var(--text-xs);
  color: var(--text-tertiary);
  margin-bottom: 2px;
}

.agent-step__field-label--error { color: var(--color-danger, #dc2626); }

.agent-step__code {
  margin: 0;
  padding: var(--space-2) var(--space-3);
  background: var(--bg-tertiary, var(--bg-secondary));
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm, 6px);
  font-size: var(--text-xs);
  line-height: 1.5;
  color: var(--text-secondary);
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-word;
}

.agent-step__code--error { color: var(--color-danger, #dc2626); }
</style>
