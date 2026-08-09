<template>
  <div :class="wrapperClasses">
    <div :class="tabsClasses" role="tablist">
      <div
        v-for="tab in tabs"
        :key="tab.key"
        :class="tabClasses(tab)"
        role="tab"
        :aria-selected="activeKey === tab.key"
        :aria-disabled="tab.disabled"
        @click="handleTabClick(tab)"
      >
        <slot name="icon" :tab="tab">
          <component v-if="tab.icon" :is="tab.icon" class="ui-tabs__icon" />
        </slot>
        <span class="ui-tabs__label">{{ tab.label }}</span>
        <span v-if="tab.count !== undefined" class="ui-tabs__count">{{ tab.count }}</span>
      </div>

      <div
        :class="indicatorClasses"
        :style="indicatorStyle"
      ></div>
    </div>

    <div class="ui-tabs__content">
      <template v-for="tab in tabs" :key="tab.key">
        <div v-show="activeKey === tab.key" class="ui-tabs__panel">
          <slot :name="tab.key" />
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'

interface Tab {
  key: string
  label: string
  icon?: any
  disabled?: boolean
  count?: number
}

interface Props {
  tabs: Tab[]
  modelValue?: string
  type?: 'line' | 'card' | 'segment'
  size?: 'sm' | 'md' | 'lg'
  animated?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  type: 'line',
  size: 'md',
  animated: true
})

const emit = defineEmits<{
  'update:modelValue': [key: string]
  'change': [key: string]
}>()

const activeKey = ref(props.modelValue || props.tabs[0]?.key)
const indicatorPosition = ref(0)
const indicatorWidth = ref(0)

const wrapperClasses = computed(() => [
  'ui-tabs',
  `ui-tabs--${props.type}`,
  `ui-tabs--${props.size}`
])

const tabsClasses = computed(() => [
  'ui-tabs__nav',
  `ui-tabs__nav--${props.type}`
])

const indicatorClasses = computed(() => [
  'ui-tabs__indicator',
  `ui-tabs__indicator--${props.type}`,
  { 'ui-tabs__indicator--animated': props.animated }
])

const indicatorStyle = computed(() => {
  if (props.type !== 'line') return {}

  return {
    transform: `translateX(${indicatorPosition.value}px)`,
    width: `${indicatorWidth.value}px`
  }
})

function tabClasses(tab: Tab) {
  return [
    'ui-tabs__tab',
    `ui-tabs__tab--${props.size}`,
    {
      'ui-tabs__tab--active': activeKey.value === tab.key,
      'ui-tabs__tab--disabled': tab.disabled
    }
  ]
}

function handleTabClick(tab: Tab) {
  if (tab.disabled) return

  activeKey.value = tab.key
  emit('update:modelValue', tab.key)
  emit('change', tab.key)

  nextTick(updateIndicator)
}

function updateIndicator() {
  const activeTab = document.querySelector('.ui-tabs__tab--active') as HTMLElement
  if (!activeTab) return

  const nav = document.querySelector('.ui-tabs__nav') as HTMLElement
  if (!nav) return

  indicatorPosition.value = activeTab.offsetLeft
  indicatorWidth.value = activeTab.offsetWidth
}

watch(() => props.modelValue, (value) => {
  if (value) {
    activeKey.value = value
    nextTick(updateIndicator)
  }
})

watch(activeKey, () => {
  nextTick(updateIndicator)
})
</script>

<style scoped>
.ui-tabs {
  display: flex;
  flex-direction: column;
}

.ui-tabs__nav {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.ui-tabs__nav--line {
  border-bottom: 1px solid var(--border-subtle);
}

.ui-tabs__nav--card {
  gap: var(--space-2);
  padding: var(--space-2);
  background: var(--bg-tertiary);
  border-radius: var(--radius-md);
}

.ui-tabs__nav--segment {
  gap: 0;
  padding: 2px;
  background: var(--bg-tertiary);
  border-radius: var(--radius-md);
}

.ui-tabs__tab {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  font-size: var(--text-base);
  font-weight: 500;
  color: var(--text-muted);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  cursor: pointer;
  user-select: none;
  transition: all var(--duration-fast);
  white-space: nowrap;
}

.ui-tabs__tab:hover:not(.ui-tabs__tab--disabled) {
  color: var(--text-secondary);
}

.ui-tabs__tab--active {
  color: var(--primary);
}

.ui-tabs__tab--disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* Sizes */
.ui-tabs__tab--sm {
  padding: var(--space-2) var(--space-3);
  font-size: var(--text-sm);
}

.ui-tabs__tab--lg {
  padding: var(--space-4) var(--space-6);
  font-size: var(--text-lg);
}

/* Type: Card */
.ui-tabs--card .ui-tabs__tab {
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
}

.ui-tabs--card .ui-tabs__tab--active {
  background: var(--bg-primary);
  border-color: var(--primary);
}

/* Type: Segment */
.ui-tabs--segment .ui-tabs__tab {
  border-radius: 0;
}

.ui-tabs--segment .ui-tabs__tab:first-child {
  border-top-left-radius: var(--radius-sm);
  border-bottom-left-radius: var(--radius-sm);
}

.ui-tabs--segment .ui-tabs__tab:last-child {
  border-top-right-radius: var(--radius-sm);
  border-bottom-right-radius: var(--radius-sm);
}

.ui-tabs--segment .ui-tabs__tab--active {
  background: var(--bg-secondary);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}

/* Icon */
.ui-tabs__icon {
  width: 16px;
  height: 16px;
}

.ui-tabs__tab--sm .ui-tabs__icon {
  width: 14px;
  height: 14px;
}

.ui-tabs__tab--lg .ui-tabs__icon {
  width: 18px;
  height: 18px;
}

/* Count */
.ui-tabs__count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  padding: 0 var(--space-1);
  font-size: var(--text-xs);
  background: var(--bg-elevated);
  border-radius: var(--radius-full);
}

.ui-tabs__tab--active .ui-tabs__count {
  background: rgba(34, 211, 238, 0.2);
  color: var(--primary);
}

/* Indicator */
.ui-tabs__indicator {
  position: absolute;
  bottom: -1px;
  left: 0;
  height: 2px;
  background: var(--primary);
  border-radius: var(--radius-full) var(--radius-full) 0 0;
}

.ui-tabs__indicator--animated {
  transition: transform var(--duration-base) ease, width var(--duration-base) ease;
}

/* Content */
.ui-tabs__content {
  margin-top: var(--space-6);
}

/* Responsive */
@media (max-width: 767px) {
  .ui-tabs__nav {
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
  }

  .ui-tabs__nav::-webkit-scrollbar {
    display: none;
  }

  .ui-tabs__tab {
    flex-shrink: 0;
  }
}
</style>
