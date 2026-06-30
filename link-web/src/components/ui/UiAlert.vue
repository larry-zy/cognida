<template>
  <div :class="alertClasses">
    <div class="ui-alert__icon">
      <slot name="icon">
        <component :is="iconComponent" />
      </slot>
    </div>

    <div class="ui-alert__content">
      <div v-if="title || $slots.title" class="ui-alert__title">
        <slot name="title">{{ title }}</slot>
      </div>
      <div v-if="$slots.default" class="ui-alert__description">
        <slot />
      </div>
    </div>

    <button
      v-if="closable"
      type="button"
      class="ui-alert__close"
      @click="handleClose"
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M18 6L6 18M6 6l12 12" />
      </svg>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, h } from 'vue'

interface Props {
  type?: 'info' | 'success' | 'warning' | 'error'
  title?: string
  closable?: boolean
  showIcon?: boolean
  effect?: 'light' | 'dark'
}

const props = withDefaults(defineProps<Props>(), {
  type: 'info',
  closable: false,
  showIcon: true,
  effect: 'light'
})

const emit = defineEmits<{
  'close': []
}>()

const alertClasses = computed(() => [
  'ui-alert',
  `ui-alert--${props.type}`,
  `ui-alert--${props.effect}`,
  { 'ui-alert--no-icon': !props.showIcon }
])

const iconComponent = computed(() => {
  const icons = {
    info: () => h('svg', { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 2 }, [
      h('circle', { cx: '12', cy: '12', r: '10' }),
      h('line', { x1: '12', y1: '16', x2: '12', y2: '12' }),
      h('line', { x1: '12', y1: '8', x2: '12.01', y2: '8' })
    ]),
    success: () => h('svg', { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 2 }, [
      h('path', { d: 'M22 11.08V12a10 10 0 1 1-5.93-9.14' }),
      h('polyline', { points: '22 4 12 14.01 9 11.01' })
    ]),
    warning: () => h('svg', { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 2 }, [
      h('path', { d: 'M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z' }),
      h('line', { x1: '12', y1: '9', x2: '12', y2: '13' }),
      h('line', { x1: '12', y1: '17', x2: '12.01', y2: '17' })
    ]),
    error: () => h('svg', { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 2 }, [
      h('circle', { cx: '12', cy: '12', r: '10' }),
      h('line', { x1: '15', y1: '9', x2: '9', y2: '15' }),
      h('line', { x1: '9', y1: '9', x2: '15', y2: '15' })
    ])
  }
  return icons[props.type] || icons.info
})

function handleClose() {
  emit('close')
}
</script>

<style scoped>
.ui-alert {
  position: relative;
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  padding: var(--space-4);
  border-radius: var(--radius-md);
  border: 1px solid;
}

.ui-alert--light {
  background: var(--bg-secondary);
}

.ui-alert--dark {
  background: var(--bg-tertiary);
}

/* Types */
.ui-alert--info {
  border-color: rgba(59, 130, 246, 0.3);
  background: rgba(59, 130, 246, 0.1);
}

.ui-alert--info .ui-alert__icon {
  color: var(--info);
}

.ui-alert--success {
  border-color: rgba(16, 185, 129, 0.3);
  background: rgba(16, 185, 129, 0.1);
}

.ui-alert--success .ui-alert__icon {
  color: var(--success);
}

.ui-alert--warning {
  border-color: rgba(245, 158, 11, 0.3);
  background: rgba(245, 158, 11, 0.1);
}

.ui-alert--warning .ui-alert__icon {
  color: var(--warning);
}

.ui-alert--error {
  border-color: rgba(239, 68, 68, 0.3);
  background: rgba(239, 68, 68, 0.1);
}

.ui-alert--error .ui-alert__icon {
  color: var(--danger);
}

/* Icon */
.ui-alert__icon {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
}

.ui-alert__icon svg {
  width: 20px;
  height: 20px;
}

.ui-alert--no-icon .ui-alert__icon {
  display: none;
}

/* Content */
.ui-alert__content {
  flex: 1;
  min-width: 0;
}

.ui-alert__title {
  font-size: var(--text-base);
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: var(--space-1);
}

.ui-alert__description {
  font-size: var(--text-sm);
  color: var(--text-secondary);
  line-height: 1.5;
}

/* Close */
.ui-alert__close {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  cursor: pointer;
  transition: all var(--duration-fast);
}

.ui-alert__close:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.ui-alert__close svg {
  width: 14px;
  height: 14px;
}
</style>
