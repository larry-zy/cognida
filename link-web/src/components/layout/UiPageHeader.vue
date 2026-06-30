<template>
  <header :class="headerClasses">
    <div class="ui-page-header__content">
      <div class="ui-page-header__main">
        <slot name="breadcrumb">
          <nav v-if="breadcrumbs.length" class="ui-page-header__breadcrumb">
            <router-link
              v-for="(crumb, index) in breadcrumbs"
              :key="index"
              :to="crumb.to"
              class="ui-page-header__crumb"
              :class="{ 'ui-page-header__crumb--last': index === breadcrumbs.length - 1 }"
            >
              {{ crumb.label }}
            </router-link>
          </nav>
        </slot>

        <h1 v-if="title || $slots.title" class="ui-page-header__title">
          <slot name="title">{{ title }}</slot>
        </h1>

        <p v-if="subtitle || $slots.subtitle" class="ui-page-header__subtitle">
          <slot name="subtitle">{{ subtitle }}</slot>
        </p>
      </div>

      <div v-if="$slots.actions" class="ui-page-header__actions">
        <slot name="actions" />
      </div>
    </div>

    <div v-if="$slots.extra" class="ui-page-header__extra">
      <slot name="extra" />
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Breadcrumb {
  label: string
  to: string | object
}

interface Props {
  title?: string
  subtitle?: string
  breadcrumbs?: Breadcrumb[]
  size?: 'sm' | 'md' | 'lg'
  divider?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  breadcrumbs: () => [],
  size: 'md',
  divider: true
})

const headerClasses = computed(() => [
  'ui-page-header',
  `ui-page-header--${props.size}`,
  { 'ui-page-header--divider': props.divider }
])
</script>

<style scoped>
.ui-page-header {
  width: 100%;
  padding: var(--space-8) 0;
}

.ui-page-header--sm {
  padding: var(--space-4) 0;
}

.ui-page-header--lg {
  padding: var(--space-12) 0;
}

.ui-page-header--divider {
  border-bottom: 1px solid var(--border-subtle);
}

.ui-page-header__content {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-6);
}

.ui-page-header__main {
  flex: 1;
  min-width: 0;
}

.ui-page-header__breadcrumb {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: var(--space-3);
  font-size: var(--text-sm);
}

.ui-page-header__crumb {
  color: var(--text-muted);
  transition: color var(--duration-fast);
}

.ui-page-header__crumb:hover {
  color: var(--primary);
}

.ui-page-header__crumb:not(:last-child)::after {
  content: '/';
  margin-left: var(--space-2);
  color: var(--border-strong);
}

.ui-page-header__crumb--last {
  color: var(--text-secondary);
}

.ui-page-header__title {
  font: var(--font-display);
  font-size: var(--text-3xl);
  font-weight: 700;
  color: var(--text-primary);
  line-height: 1.2;
  margin: 0;
}

.ui-page-header--sm .ui-page-header__title {
  font-size: var(--text-xl);
}

.ui-page-header--lg .ui-page-header__title {
  font-size: var(--text-4xl);
}

.ui-page-header__subtitle {
  margin: var(--space-2) 0 0;
  font-size: var(--text-base);
  color: var(--text-secondary);
  line-height: 1.5;
}

.ui-page-header__actions {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.ui-page-header__extra {
  margin-top: var(--space-6);
}

@media (max-width: 767px) {
  .ui-page-header__content {
    flex-direction: column;
    gap: var(--space-4);
  }

  .ui-page-header__actions {
    width: 100%;
    flex-direction: column;
    align-items: stretch;
  }

  .ui-page-header__actions :deep(*) {
    width: 100%;
  }
}
</style>
