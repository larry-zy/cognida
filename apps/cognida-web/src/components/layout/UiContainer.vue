<template>
  <component :is="tag" :class="containerClasses">
    <slot />
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  variant?: 'standard' | 'narrow' | 'wide'
  tag?: string
  fluid?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'standard',
  tag: 'div',
  fluid: false
})

const containerClasses = computed(() => [
  'ui-container',
  `ui-container--${props.variant}`,
  { 'ui-container--fluid': props.fluid }
])
</script>

<style scoped>
.ui-container {
  width: 100%;
  margin: 0 auto;
  padding: 0 var(--space-4);
}

.ui-container--standard {
  max-width: 1400px;
}

.ui-container--narrow {
  max-width: 900px;
}

.ui-container--wide {
  max-width: 1600px;
}

.ui-container--fluid {
  max-width: none;
}

@media (min-width: 640px) {
  .ui-container {
    padding: 0 var(--space-6);
  }
}

@media (min-width: 1024px) {
  .ui-container {
    padding: 0 var(--space-8);
  }
}
</style>
