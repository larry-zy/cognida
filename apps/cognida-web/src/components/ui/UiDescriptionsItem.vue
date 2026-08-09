<template>
  <div :class="itemClasses" :style="itemStyle">
    <div class="ui-descriptions-item__label" :style="labelStyle">
      <slot name="label">{{ label }}</slot>
    </div>
    <div class="ui-descriptions-item__value">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject } from 'vue'
import {
  UiDescriptionsContextKey,
  type UiDescriptionsContext,
  type UiDescriptionsDirection
} from './uiDescriptionsContext'

interface Props {
  label?: string
  span?: number
  labelWidth?: string
  mono?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  span: 1,
  mono: true
})

const fallbackContext: UiDescriptionsContext = {
  column: computed(() => 1),
  direction: computed<UiDescriptionsDirection>(() => 'horizontal'),
  border: computed(() => false),
  size: computed(() => 'md')
}

const context = inject(UiDescriptionsContextKey, fallbackContext)

const span = computed(() => {
  const max = context.column.value
  const requested = props.span && props.span > 0 ? props.span : 1
  return Math.min(requested, max)
})

const itemClasses = computed(() => [
  'ui-descriptions-item',
  `ui-descriptions-item--${context.direction.value}`,
  `ui-descriptions-item--${context.size.value}`,
  {
    'ui-descriptions-item--border': context.border.value,
    'ui-descriptions-item--mono': props.mono
  }
])

const itemStyle = computed(() => ({
  gridColumn: `span ${span.value}`
}))

const labelStyle = computed(() => {
  if (context.direction.value === 'horizontal' && props.labelWidth) {
    return { flex: `0 0 ${props.labelWidth}`, width: props.labelWidth }
  }
  return {}
})
</script>

<style scoped>
.ui-descriptions-item {
  min-width: 0;
  border-bottom: 1px solid var(--border-subtle);
}

/* Horizontal: label sits to the left of value */
.ui-descriptions-item--horizontal {
  display: flex;
  align-items: baseline;
  gap: var(--space-4);
}

/* Vertical: label stacked above value */
.ui-descriptions-item--vertical {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

/* Label: refined micro-label style */
.ui-descriptions-item__label {
  font-family: var(--font-body);
  font-size: 0.6875rem;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--text-muted);
  white-space: nowrap;
}

.ui-descriptions-item--horizontal .ui-descriptions-item__label {
  flex: 0 0 auto;
  min-width: 6rem;
}

/* Value: monospace by default, high-contrast text */
.ui-descriptions-item__value {
  min-width: 0;
  flex: 1 1 auto;
  font-size: var(--text-sm);
  color: var(--text-primary);
  word-break: break-word;
}

.ui-descriptions-item--mono .ui-descriptions-item__value {
  font-family: var(--font-mono);
}

/* Sizes */
.ui-descriptions-item--sm {
  padding: var(--space-2) var(--space-1);
}

.ui-descriptions-item--md {
  padding: var(--space-3) var(--space-1);
}

.ui-descriptions-item--lg {
  padding: var(--space-4) var(--space-2);
}

.ui-descriptions-item--sm .ui-descriptions-item__value {
  font-size: var(--text-xs);
}

.ui-descriptions-item--lg .ui-descriptions-item__value {
  font-size: var(--text-base);
}

/* Border mode: hairline cells, no heavy grid */
.ui-descriptions-item--border {
  border-bottom: 1px solid var(--border-subtle);
  padding-left: var(--space-4);
  padding-right: var(--space-4);
}

.ui-descriptions-item--border.ui-descriptions-item--sm {
  padding-top: var(--space-2);
  padding-bottom: var(--space-2);
}

.ui-descriptions-item--border.ui-descriptions-item--lg {
  padding-top: var(--space-4);
  padding-bottom: var(--space-4);
}
</style>
