<template>
  <div :class="dividerClasses" :style="dividerStyle">
    <slot />
  </div>
</template>

<script setup lang="ts">
import { computed, useSlots } from 'vue'

interface Props {
  orientation?: 'horizontal' | 'vertical'
  variant?: 'solid' | 'dashed'
  color?: 'default' | 'subtle' | 'strong'
  spacing?: 'none' | 'sm' | 'md' | 'lg'
  labelPosition?: 'start' | 'center' | 'end'
}

const props = withDefaults(defineProps<Props>(), {
  orientation: 'horizontal',
  variant: 'solid',
  color: 'default',
  spacing: 'md',
  labelPosition: 'center'
})

const slots = useSlots()

const dividerClasses = computed(() => [
  'ui-divider',
  `ui-divider--${props.orientation}`,
  `ui-divider--${props.variant}`,
  `ui-divider--${props.color}`,
  `ui-divider--spacing-${props.spacing}`,
  `ui-divider--label-${props.labelPosition}`,
  { 'ui-divider--has-content': slots.default }
])

const dividerStyle = computed(() => {
  if (props.orientation === 'vertical') {
    return {
      height: slots.default ? 'auto' : '100%',
      minHeight: slots.default ? undefined : '24px'
    }
  }
  return undefined
})
</script>

<style scoped>
.ui-divider {
  display: flex;
  align-items: center;
  width: 100%;
  border: none;
  margin: 0;
}

.ui-divider--horizontal {
  flex-direction: row;
}

.ui-divider--vertical {
  flex-direction: column;
  width: auto;
  height: 100%;
  min-height: 24px;
}

.ui-divider--spacing-none {
  margin: 0;
}

.ui-divider--spacing-sm {
  margin: var(--space-3) 0;
}

.ui-divider--spacing-md {
  margin: var(--space-6) 0;
}

.ui-divider--spacing-lg {
  margin: var(--space-8) 0;
}

.ui-divider--vertical.ui-divider--spacing-sm {
  margin: 0 var(--space-3);
}

.ui-divider--vertical.ui-divider--spacing-md {
  margin: 0 var(--space-6);
}

.ui-divider--vertical.ui-divider--spacing-lg {
  margin: 0 var(--space-8);
}

/* Line styles */
.ui-divider::before,
.ui-divider::after {
  content: '';
  flex: 1;
  border: none;
}

.ui-divider--horizontal::before,
.ui-divider--horizontal::after {
  border-top: 1px solid;
}

.ui-divider--vertical::before,
.ui-divider--vertical::after {
  border-left: 1px solid;
}

/* Colors */
.ui-divider--subtle::before,
.ui-divider--subtle::after {
  border-color: var(--border-subtle);
}

.ui-divider--default::before,
.ui-divider--default::after {
  border-color: var(--border-default);
}

.ui-divider--strong::before,
.ui-divider--strong::after {
  border-color: var(--border-strong);
}

/* Variant */
.ui-divider--dashed::before,
.ui-divider--dashed::after {
  border-style: dashed;
}

/* With content */
.ui-divider--has-content::before,
.ui-divider--has-content::after {
  flex: 1;
}

.ui-divider--label-start::before {
  flex: 0 0 32px;
}

.ui-divider--label-start::after {
  flex: 1;
}

.ui-divider--label-end::before {
  flex: 1;
}

.ui-divider--label-end::after {
  flex: 0 0 32px;
}

.ui-divider--has-content > * {
  padding: 0 var(--space-4);
  font-size: var(--text-sm);
  color: var(--text-muted);
  white-space: nowrap;
}

.ui-divider--vertical.ui-divider--has-content {
  flex-direction: column;
}

.ui-divider--vertical.ui-divider--has-content > * {
  padding: var(--space-2) 0;
}

/* Vertical divider specific */
.ui-divider--vertical::before,
.ui-divider--vertical::after {
  flex: 1;
}

.ui-divider--vertical.ui-divider--has-content::before,
.ui-divider--vertical.ui-divider--has-content::after {
  height: 32px;
  flex: none;
}
</style>
