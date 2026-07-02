<template>
  <div :class="rootClasses">
    <header v-if="title || $slots.title || $slots.extra" class="ui-descriptions__header">
      <div class="ui-descriptions__title">
        <slot name="title">{{ title }}</slot>
      </div>
      <div v-if="$slots.extra" class="ui-descriptions__extra">
        <slot name="extra" />
      </div>
    </header>

    <div class="ui-descriptions__body" :style="bodyStyle">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, provide, useSlots } from 'vue'
import {
  UiDescriptionsContextKey,
  type UiDescriptionsContext,
  type UiDescriptionsSize,
  type UiDescriptionsDirection
} from './uiDescriptionsContext'

interface Props {
  title?: string
  column?: number
  border?: boolean
  size?: UiDescriptionsSize
  direction?: UiDescriptionsDirection
}

const props = withDefaults(defineProps<Props>(), {
  column: 2,
  border: false,
  size: 'md',
  direction: 'horizontal'
})

const slots = useSlots()

const column = computed(() => (props.column && props.column > 0 ? props.column : 1))

provide<UiDescriptionsContext>(UiDescriptionsContextKey, {
  column,
  direction: computed(() => props.direction),
  border: computed(() => props.border),
  size: computed(() => props.size)
})

const rootClasses = computed(() => [
  'ui-descriptions',
  `ui-descriptions--${props.size}`,
  `ui-descriptions--${props.direction}`,
  {
    'ui-descriptions--border': props.border
  }
])

const bodyStyle = computed(() => ({
  gridTemplateColumns: `repeat(${column.value}, minmax(0, 1fr))`
}))

// referenced to avoid unused warning when only slot presence is checked in template
void slots
</script>

<style scoped>
.ui-descriptions {
  width: 100%;
  font-family: var(--font-body);
  color: var(--text-primary);
}

.ui-descriptions__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  margin-bottom: var(--space-4);
}

.ui-descriptions__title {
  font-family: var(--font-display);
  font-size: var(--text-lg);
  font-weight: 600;
  color: var(--text-primary);
  letter-spacing: 0.01em;
}

.ui-descriptions__extra {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.ui-descriptions__body {
  display: grid;
  gap: 0;
}

/* Hairline separators between rows via row-spanning items */
.ui-descriptions--border .ui-descriptions__body {
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  overflow: hidden;
}
</style>
