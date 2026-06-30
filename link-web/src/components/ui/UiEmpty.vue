<template>
  <div :class="emptyClasses">
    <div class="ui-empty__illustration">
      <slot name="illustration">
        <svg viewBox="0 0 64 64" fill="none" stroke="currentColor" stroke-width="1.5">
          <rect x="8" y="8" width="48" height="48" rx="4" />
          <line x1="8" y1="20" x2="56" y2="20" />
          <line x1="20" y1="8" x2="20" y2="12" />
          <line x1="32" y1="8" x2="32" y2="12" />
          <line x1="44" y1="8" x2="44" y2="12" />
        </svg>
      </slot>
    </div>

    <div v-if="title || $slots.title" class="ui-empty__title">
      <slot name="title">{{ title || '暂无数据' }}</slot>
    </div>

    <div v-if="description || $slots.description" class="ui-empty__description">
      <slot name="description">{{ description }}</slot>
    </div>

    <div v-if="$slots.default" class="ui-empty__actions">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  title?: string
  description?: string
  size?: 'sm' | 'md' | 'lg'
}

const props = withDefaults(defineProps<Props>(), {
  size: 'md'
})

const emptyClasses = computed(() => [
  'ui-empty',
  `ui-empty--${props.size}`
])
</script>

<style scoped>
.ui-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--space-12);
  text-align: center;
}

.ui-empty__illustration {
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: var(--space-6);
  color: var(--text-muted);
  opacity: 0.5;
}

.ui-empty__illustration svg {
  width: 64px;
  height: 64px;
}

.ui-empty--sm .ui-empty__illustration svg {
  width: 48px;
  height: 48px;
}

.ui-empty--lg .ui-empty__illustration svg {
  width: 80px;
  height: 80px;
}

.ui-empty__title {
  font: var(--font-display);
  font-size: var(--text-lg);
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: var(--space-2);
}

.ui-empty--sm .ui-empty__title {
  font-size: var(--text-base);
}

.ui-empty--lg .ui-empty__title {
  font-size: var(--text-xl);
}

.ui-empty__description {
  font-size: var(--text-sm);
  color: var(--text-muted);
  max-width: 320px;
  margin-bottom: var(--space-6);
}

.ui-empty__actions {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

/* Custom illustration animation */
.ui-empty__illustration svg {
  animation: float 3s ease-in-out infinite;
}

@keyframes float {
  0%, 100% {
    transform: translateY(0);
  }
  50% {
    transform: translateY(-8px);
  }
}
</style>
