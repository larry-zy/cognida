<template>
  <div :class="skeletonClasses">
    <!-- Avatar skeleton -->
    <div v-if="avatar" :class="avatarClasses"></div>

    <!-- Content skeleton -->
    <div class="ui-skeleton__content">
      <!-- Title skeleton -->
      <div v-if="title" :class="titleClasses"></div>

      <!-- Paragraph skeletons -->
      <div v-for="index in rows" :key="index" :class="rowClasses(index)"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  avatar?: boolean
  title?: boolean
  rows?: number
  loading?: boolean
  size?: 'sm' | 'md' | 'lg'
  animate?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  avatar: false,
  title: true,
  rows: 3,
  loading: true,
  size: 'md',
  animate: true
})

const skeletonClasses = computed(() => [
  'ui-skeleton',
  `ui-skeleton--${props.size}`,
  { 'ui-skeleton--animate': props.animate }
])

const avatarClasses = computed(() => [
  'ui-skeleton__avatar',
  `ui-skeleton__avatar--${props.size}`
])

const titleClasses = computed(() => [
  'ui-skeleton__title',
  `ui-skeleton__title--${props.size}`
])

function rowClasses(index: number) {
  return [
    'ui-skeleton__row',
    `ui-skeleton__row--${props.size}`,
    { 'ui-skeleton__row--last': index === props.rows }
  ]
}
</script>

<style scoped>
.ui-skeleton {
  display: flex;
  gap: var(--space-4);
}

.ui-skeleton--animate .ui-skeleton__avatar,
.ui-skeleton--animate .ui-skeleton__title,
.ui-skeleton--animate .ui-skeleton__row {
  animation: skeletonPulse 1.5s ease-in-out infinite;
}

@keyframes skeletonPulse {
  0%, 100% {
    opacity: 0.4;
  }
  50% {
    opacity: 1;
  }
}

/* Avatar */
.ui-skeleton__avatar {
  flex-shrink: 0;
  background: var(--bg-tertiary);
  border-radius: 50%;
}

.ui-skeleton__avatar--sm {
  width: 32px;
  height: 32px;
}

.ui-skeleton__avatar--md {
  width: 40px;
  height: 40px;
}

.ui-skeleton__avatar--lg {
  width: 48px;
  height: 48px;
}

/* Content */
.ui-skeleton__content {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

/* Title */
.ui-skeleton__title {
  width: 40%;
  height: 16px;
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
}

.ui-skeleton__title--sm {
  height: 14px;
}

.ui-skeleton__title--lg {
  height: 20px;
}

/* Row */
.ui-skeleton__row {
  width: 100%;
  height: 14px;
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
}

.ui-skeleton__row--last {
  width: 60%;
}

.ui-skeleton__row--sm {
  height: 12px;
}

.ui-skeleton__row--lg {
  height: 16px;
}
</style>
