<template>
  <div :class="gridClasses" :style="gridStyle">
    <slot />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  columns?: 1 | 2 | 3 | 4
  gap?: 'none' | 'sm' | 'md' | 'lg' | 'xl'
  responsive?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  columns: 2,
  gap: 'md',
  responsive: true
})

const gridClasses = computed(() => [
  'ui-grid',
  `ui-grid--cols-${props.columns}`,
  `ui-grid--gap-${props.gap}`,
  { 'ui-grid--responsive': props.responsive }
])

const gridStyle = computed(() => {
  if (props.gap === 'none') return undefined

  const gapMap = {
    sm: 'var(--space-3)',
    md: 'var(--space-6)',
    lg: 'var(--space-8)',
    xl: 'var(--space-12)'
  }

  return { gap: gapMap[props.gap] }
})
</script>

<style scoped>
.ui-grid {
  display: grid;
  width: 100%;
}

.ui-grid--cols-1 {
  grid-template-columns: repeat(1, 1fr);
}

.ui-grid--cols-2 {
  grid-template-columns: repeat(2, 1fr);
}

.ui-grid--cols-3 {
  grid-template-columns: repeat(3, 1fr);
}

.ui-grid--cols-4 {
  grid-template-columns: repeat(4, 1fr);
}

/* Responsive behavior */
.ui-grid--responsive.ui-grid--cols-2,
.ui-grid--responsive.ui-grid--cols-3,
.ui-grid--responsive.ui-grid--cols-4 {
  @media (max-width: 767px) {
    grid-template-columns: 1fr;
  }
}

.ui-grid--responsive.ui-grid--cols-3,
.ui-grid--responsive.ui-grid--cols-4 {
  @media (min-width: 768px) and (max-width: 1023px) {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
