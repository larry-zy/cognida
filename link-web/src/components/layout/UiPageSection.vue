<template>
  <section :class="sectionClasses">
    <UiContainer v-if="!fullWidth">
      <slot />
    </UiContainer>
    <slot v-else />
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import UiContainer from './UiContainer.vue'

interface Props {
  spacing?: 'none' | 'sm' | 'md' | 'lg' | 'xl'
  fullWidth?: boolean
  variant?: 'default' | 'primary' | 'secondary'
}

const props = withDefaults(defineProps<Props>(), {
  spacing: 'lg',
  fullWidth: false,
  variant: 'default'
})

const sectionClasses = computed(() => [
  'ui-page-section',
  `ui-page-section--spacing-${props.spacing}`,
  `ui-page-section--${props.variant}`
])
</script>

<style scoped>
.ui-page-section {
  width: 100%;
}

.ui-page-section--spacing-none {
  padding: 0;
}

.ui-page-section--spacing-sm {
  padding: var(--space-6) 0;
}

.ui-page-section--spacing-md {
  padding: var(--space-8) 0;
}

.ui-page-section--spacing-lg {
  padding: var(--space-12) 0;
}

.ui-page-section--spacing-xl {
  padding: var(--space-16) 0;
}

.ui-page-section--primary {
  background: var(--bg-secondary);
  border-top: 1px solid var(--border-subtle);
  border-bottom: 1px solid var(--border-subtle);
}

.ui-page-section--secondary {
  background: var(--bg-tertiary);
}

@media (max-width: 767px) {
  .ui-page-section--spacing-lg {
    padding: var(--space-8) 0;
  }

  .ui-page-section--spacing-xl {
    padding: var(--space-12) 0;
  }
}
</style>
