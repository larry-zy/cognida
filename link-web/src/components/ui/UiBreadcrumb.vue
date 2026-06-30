<template>
  <nav :class="breadcrumbClasses" aria-label="Breadcrumb">
    <ol class="ui-breadcrumb__list">
      <li
        v-for="(item, index) in items"
        :key="index"
        class="ui-breadcrumb__item"
      >
        <component
          :is="item.to ? (item.external ? 'a' : 'router-link') : 'span'"
          v-bind="getItemProps(item)"
          :class="itemClasses(index)"
        >
          <slot name="item" :item="item" :index="index">
            <span v-if="item.icon" class="ui-breadcrumb__icon">
              <component :is="item.icon" />
            </span>
            <span class="ui-breadcrumb__label">{{ item.label }}</span>
          </slot>
        </component>

        <span
          v-if="index < items.length - 1"
          class="ui-breadcrumb__separator"
          aria-hidden="true"
        >
          <slot name="separator">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="9 18 15 12 9 6" />
            </svg>
          </slot>
        </span>
      </li>
    </ol>
  </nav>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface BreadcrumbItem {
  label: string
  to?: string | object
  external?: boolean
  icon?: any
  disabled?: boolean
}

interface Props {
  items: BreadcrumbItem[]
  size?: 'sm' | 'md' | 'lg'
}

const props = withDefaults(defineProps<Props>(), {
  size: 'md'
})

const breadcrumbClasses = computed(() => [
  'ui-breadcrumb',
  `ui-breadcrumb--${props.size}`
])

function itemClasses(index: number) {
  return [
    'ui-breadcrumb__link',
    `ui-breadcrumb__link--${props.size}`,
    {
      'ui-breadcrumb__link--current': index === props.items.length - 1,
      'ui-breadcrumb__link--disabled': props.items[index].disabled
    }
  ]
}

function getItemProps(item: BreadcrumbItem) {
  if (item.external) {
    return { href: item.to }
  } else if (item.to) {
    return { to: item.to }
  }
  return {}
}
</script>

<style scoped>
.ui-breadcrumb {
  display: flex;
  align-items: center;
}

.ui-breadcrumb__list {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.ui-breadcrumb__item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.ui-breadcrumb__link {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  font-size: var(--text-sm);
  color: var(--text-muted);
  text-decoration: none;
  transition: color var(--duration-fast);
  white-space: nowrap;
}

.ui-breadcrumb__link:hover:not(.ui-breadcrumb__link--current):not(.ui-breadcrumb__link--disabled) {
  color: var(--primary);
}

.ui-breadcrumb__link--current {
  color: var(--text-secondary);
  font-weight: 500;
}

.ui-breadcrumb__link--disabled {
  color: var(--text-muted);
  pointer-events: none;
}

.ui-breadcrumb__icon {
  display: flex;
  align-items: center;
}

.ui-breadcrumb__icon svg {
  width: 14px;
  height: 14px;
}

.ui-breadcrumb__separator {
  display: flex;
  align-items: center;
  color: var(--text-muted);
}

.ui-breadcrumb__separator svg {
  width: 14px;
  height: 14px;
}

/* Sizes */
.ui-breadcrumb__link--sm {
  font-size: var(--text-xs);
}

.ui-breadcrumb__link--lg {
  font-size: var(--text-base);
}

.ui-breadcrumb--sm .ui-breadcrumb__icon svg,
.ui-breadcrumb--sm .ui-breadcrumb__separator svg {
  width: 12px;
  height: 12px;
}

.ui-breadcrumb--lg .ui-breadcrumb__icon svg,
.ui-breadcrumb--lg .ui-breadcrumb__separator svg {
  width: 16px;
  height: 16px;
}

/* Responsive */
@media (max-width: 767px) {
  .ui-breadcrumb__list {
    gap: var(--space-1);
  }

  .ui-breadcrumb__separator {
    display: none;
  }
}
</style>
