<template>
  <div :class="cardClasses">
    <!-- Header -->
    <div v-if="$slots.header || title || extra" class="ui-card__header">
      <div class="ui-card__header-content">
        <slot name="header">
          <h3 v-if="title" class="ui-card__title">{{ title }}</h3>
        </slot>
      </div>
      <div v-if="$slots.extra || extra" class="ui-card__extra">
        <slot name="extra">{{ extra }}</slot>
      </div>
    </div>

    <!-- Body -->
    <div :class="bodyClasses">
      <slot />
    </div>

    <!-- Footer -->
    <div v-if="$slots.footer" class="ui-card__footer">
      <slot name="footer" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, useSlots } from 'vue'

interface Props {
  title?: string
  extra?: string
  variant?: 'default' | 'bordered' | 'elevated'
  hoverable?: boolean
  padding?: 'none' | 'sm' | 'md' | 'lg'
  size?: 'sm' | 'md' | 'lg'
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'default',
  hoverable: false,
  padding: 'md',
  size: 'md'
})

const slots = useSlots()

const cardClasses = computed(() => [
  'ui-card',
  `ui-card--${props.variant}`,
  `ui-card--${props.size}`,
  `ui-card--padding-${props.padding}`,
  { 'ui-card--hoverable': props.hoverable }
])

const bodyClasses = computed(() => [
  'ui-card__body',
  { 'ui-card__body--no-header': !slots.header && !props.title && !props.extra },
  { 'ui-card__body--no-footer': !slots.footer }
])
</script>

<style scoped>
.ui-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  transition: all var(--duration-fast);
}

.ui-card--bordered {
  border-color: var(--border-default);
}

.ui-card--elevated {
  box-shadow: var(--shadow-md);
}

.ui-card--hoverable:hover {
  border-color: var(--border-strong);
  box-shadow: var(--shadow-lg);
}

/* Sizes */
.ui-card--sm {
  border-radius: var(--radius-md);
}

.ui-card--lg {
  border-radius: var(--radius-xl);
}

/* Header */
.ui-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  padding: var(--space-5) var(--space-6);
  border-bottom: 1px solid var(--border-subtle);
}

.ui-card__header-content {
  flex: 1;
  min-width: 0;
}

.ui-card__title {
  margin: 0;
  font: var(--font-display);
  font-size: var(--text-lg);
  font-weight: 600;
  color: var(--text-primary);
}

.ui-card__extra {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

/* Body */
.ui-card__body {
  padding: var(--space-6);
}

.ui-card__body--no-header {
  border-top-left-radius: var(--radius-lg);
  border-top-right-radius: var(--radius-lg);
}

.ui-card__body--no-footer {
  border-bottom-left-radius: var(--radius-lg);
  border-bottom-right-radius: var(--radius-lg);
}

/* Padding variants */
.ui-card--padding-none .ui-card__body {
  padding: 0;
}

.ui-card--padding-sm .ui-card__body {
  padding: var(--space-4);
}

.ui-card--padding-md .ui-card__body {
  padding: var(--space-6);
}

.ui-card--padding-lg .ui-card__body {
  padding: var(--space-8);
}

/* Footer */
.ui-card__footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-3);
  padding: var(--space-5) var(--space-6);
  border-top: 1px solid var(--border-subtle);
}

/* Size variations */
.ui-card--sm .ui-card__header,
.ui-card--sm .ui-card__footer {
  padding: var(--space-3) var(--space-4);
}

.ui-card--lg .ui-card__header,
.ui-card--lg .ui-card__footer {
  padding: var(--space-6) var(--space-8);
}

/* Responsive */
@media (max-width: 767px) {
  .ui-card__header,
  .ui-card__body,
  .ui-card__footer {
    padding: var(--space-4);
  }

  .ui-card__header {
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-3);
  }

  .ui-card__extra {
    width: 100%;
    justify-content: flex-start;
  }
}
</style>
