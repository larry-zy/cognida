<template>
  <div v-if="steps.length > 0" class="agent-timeline" :class="{ 'agent-timeline--active': active }">
    <button class="agent-timeline__toggle" type="button" @click="open = !open">
      <span class="agent-timeline__brain">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6">
          <path d="M9.5 2a3 3 0 0 0-3 3 3 3 0 0 0-2 5.2A3 3 0 0 0 6 16a3 3 0 0 0 3.5 3V2z" />
          <path d="M14.5 2a3 3 0 0 1 3 3 3 3 0 0 1 2 5.2A3 3 0 0 1 18 16a3 3 0 0 1-3.5 3V2z" />
        </svg>
      </span>
      <span class="agent-timeline__title">
        思考过程
        <span class="agent-timeline__count">{{ steps.length }} 步</span>
        <span v-if="active" class="agent-timeline__live">运行中</span>
      </span>
      <svg class="agent-timeline__chevron" :class="{ 'is-open': open }" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M6 9l6 6 6-6" />
      </svg>
    </button>

    <div v-if="open" class="agent-timeline__steps">
      <AgentStepCard v-for="s in steps" :key="s.id" :step="s" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import type { AgentStep } from '@/types'
import AgentStepCard from './AgentStepCard.vue'

const props = withDefaults(defineProps<{
  steps: AgentStep[]
  active?: boolean
  defaultOpen?: boolean
}>(), {
  active: false,
  defaultOpen: false
})

// 运行中默认展开，便于实时观察；历史消息默认折叠
const open = ref(props.defaultOpen || props.active)

watch(() => props.active, (v) => {
  if (v) open.value = true
})
</script>

<style scoped>
.agent-timeline {
  margin-bottom: var(--space-3);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  background: var(--bg-primary, var(--bg-secondary));
  overflow: hidden;
}

.agent-timeline--active {
  border-color: color-mix(in srgb, var(--color-primary, #6366f1) 40%, var(--border-subtle));
}

.agent-timeline__toggle {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: transparent;
  border: none;
  cursor: pointer;
  text-align: left;
}

.agent-timeline__brain {
  display: flex;
  color: var(--color-primary, #6366f1);
}

.agent-timeline__brain svg { width: 16px; height: 16px; }

.agent-timeline__title {
  flex: 1;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-primary);
}

.agent-timeline__count {
  font-size: var(--text-xs);
  font-weight: 500;
  color: var(--text-tertiary);
}

.agent-timeline__live {
  font-size: var(--text-xs);
  font-weight: 500;
  color: var(--color-primary, #4f46e5);
  padding: 1px 8px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--color-primary, #6366f1) 15%, transparent);
}

.agent-timeline__chevron {
  width: 16px;
  height: 16px;
  color: var(--text-tertiary);
  transition: transform var(--duration-fast);
}

.agent-timeline__chevron.is-open { transform: rotate(180deg); }

.agent-timeline__steps {
  padding: var(--space-3) var(--space-3) var(--space-1);
  border-top: 1px solid var(--border-subtle);
}
</style>
