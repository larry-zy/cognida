<template>
  <component :is="tag" :class="textClasses">
    <slot />
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  type?: 'default' | 'primary' | 'secondary' | 'muted' | 'success' | 'warning' | 'danger' | 'info'
  size?: 'xs' | 'sm' | 'base' | 'lg' | 'xl' | '2xl' | '3xl'
  tag?: string
  weight?: 'normal' | 'medium' | 'semibold' | 'bold'
  mono?: boolean
  gradient?: boolean
  truncate?: boolean
  italic?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  type: 'default',
  size: 'base',
  tag: 'span',
  weight: 'normal',
  mono: false,
  gradient: false,
  truncate: false,
  italic: false
})

const textClasses = computed(() => [
  'ui-text',
  `ui-text--${props.type}`,
  `ui-text--size-${props.size}`,
  `ui-text--weight-${props.weight}`,
  {
    'ui-text--mono': props.mono,
    'ui-text--gradient': props.gradient,
    'ui-text--truncate': props.truncate,
    'ui-text--italic': props.italic
  }
])
</script>

<style scoped>
.ui-text {
  font-family: var(--font-body);
  line-height: 1.5;
  transition: color var(--duration-fast) var(--ease-default);
}

/* Types */
.ui-text--default {
  color: var(--text-primary);
}

.ui-text--primary {
  color: var(--primary);
}

.ui-text--secondary {
  color: var(--text-secondary);
}

.ui-text--muted {
  color: var(--text-muted);
}

.ui-text--success {
  color: var(--success);
}

.ui-text--warning {
  color: var(--warning);
}

.ui-text--danger {
  color: var(--danger);
}

.ui-text--info {
  color: var(--info);
}

/* Sizes */
.ui-text--size-xs {
  font-size: var(--text-xs);
}

.ui-text--size-sm {
  font-size: var(--text-sm);
}

.ui-text--size-base {
  font-size: var(--text-base);
}

.ui-text--size-lg {
  font-size: var(--text-lg);
}

.ui-text--size-xl {
  font-size: var(--text-xl);
}

.ui-text--size-2xl {
  font-size: var(--text-2xl);
}

.ui-text--size-3xl {
  font-size: var(--text-3xl);
}

/* Weights */
.ui-text--weight-normal {
  font-weight: 400;
}

.ui-text--weight-medium {
  font-weight: 500;
}

.ui-text--weight-semibold {
  font-weight: 600;
}

.ui-text--weight-bold {
  font-weight: 700;
}

/* Modifiers */
.ui-text--mono {
  font-family: var(--font-mono);
}

.ui-text--italic {
  font-style: italic;
}

.ui-text--truncate {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}

.ui-text--gradient {
  background: linear-gradient(135deg, var(--primary), var(--primary-dark));
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  color: transparent;
}
</style>
