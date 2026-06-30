<template>
  <span :class="tagClasses">
    <slot />
    <button
      v-if="closable"
      type="button"
      class="ui-tag__close"
      @click="handleClose"
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M18 6L6 18M6 6l12 12" />
      </svg>
    </button>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  variant?: 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info'
  size?: 'sm' | 'md' | 'lg'
  closable?: boolean
  rounded?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'default',
  size: 'md',
  closable: false,
  rounded: false
})

const emit = defineEmits<{
  'close': []
}>()

const tagClasses = computed(() => [
  'ui-tag',
  `ui-tag--${props.variant}`,
  `ui-tag--${props.size}`,
  { 'ui-tag--rounded': props.rounded }
])

function handleClose() {
  emit('close')
}
</script>

<style scoped>
.ui-tag {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: var(--space-1) var(--space-3);
  font-size: var(--text-sm);
  font-weight: 500;
  line-height: 1;
  border-radius: var(--radius-sm);
  transition: all var(--duration-fast);
}

.ui-tag--rounded {
  border-radius: var(--radius-full);
}

.ui-tag--sm {
  padding: 2px var(--space-2);
  font-size: var(--text-xs);
}

.ui-tag--lg {
  padding: var(--space-2) var(--space-4);
  font-size: var(--text-base);
}

/* Variants */
.ui-tag--default {
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  border: 1px solid var(--border-default);
}

.ui-tag--primary {
  background: rgba(34, 211, 238, 0.15);
  color: var(--primary);
  border: 1px solid rgba(34, 211, 238, 0.3);
}

.ui-tag--success {
  background: rgba(16, 185, 129, 0.15);
  color: var(--success);
  border: 1px solid rgba(16, 185, 129, 0.3);
}

.ui-tag--warning {
  background: rgba(245, 158, 11, 0.15);
  color: var(--warning);
  border: 1px solid rgba(245, 158, 11, 0.3);
}

.ui-tag--danger {
  background: rgba(239, 68, 68, 0.15);
  color: var(--danger);
  border: 1px solid rgba(239, 68, 68, 0.3);
}

.ui-tag--info {
  background: rgba(59, 130, 246, 0.15);
  color: var(--info);
  border: 1px solid rgba(59, 130, 246, 0.3);
}

/* Close button */
.ui-tag__close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: 50%;
  color: inherit;
  cursor: pointer;
  opacity: 0.6;
  transition: all var(--duration-fast);
}

.ui-tag__close:hover {
  opacity: 1;
  background: rgba(0, 0, 0, 0.1);
}

.ui-tag__close svg {
  width: 10px;
  height: 10px;
}

.ui-tag--sm .ui-tag__close {
  width: 14px;
  height: 14px;
}

.ui-tag--lg .ui-tag__close {
  width: 18px;
  height: 18px;
}
</style>
