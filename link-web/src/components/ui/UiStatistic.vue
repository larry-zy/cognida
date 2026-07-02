<template>
  <div class="ui-statistic">
    <div v-if="title || $slots.title" class="ui-statistic__title">
      <slot name="title">{{ title }}</slot>
    </div>

    <div class="ui-statistic__content">
      <span v-if="prefix || $slots.prefix" class="ui-statistic__prefix">
        <slot name="prefix">{{ prefix }}</slot>
      </span>

      <span class="ui-statistic__value" :style="valueStyle">{{ displayValue }}</span>

      <span v-if="suffix || $slots.suffix" class="ui-statistic__suffix">
        <slot name="suffix">{{ suffix }}</slot>
      </span>
    </div>

    <div v-if="trend !== 'none'" :class="trendClasses">
      <svg
        v-if="trend === 'up'"
        class="ui-statistic__trend-icon"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
      >
        <path d="M12 19V5" />
        <path d="M5 12l7-7 7 7" />
      </svg>
      <svg
        v-else
        class="ui-statistic__trend-icon"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
      >
        <path d="M12 5v14" />
        <path d="M19 12l-7 7-7-7" />
      </svg>
      <span v-if="trendValue !== undefined" class="ui-statistic__trend-value">{{ trendValue }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

interface Props {
  title?: string
  value: number | string
  precision?: number
  prefix?: string
  suffix?: string
  valueColor?: string
  trend?: 'up' | 'down' | 'none'
  trendValue?: string | number
  animation?: boolean
  animationDuration?: number
}

const props = withDefaults(defineProps<Props>(), {
  trend: 'none',
  animation: false,
  animationDuration: 1000
})

const isNumeric = computed(() => typeof props.value === 'number')

const animatedValue = ref<number>(
  props.animation && isNumeric.value ? 0 : (isNumeric.value ? (props.value as number) : 0)
)

let rafId: number | null = null

function formatNumber(n: number): string {
  if (props.precision !== undefined) {
    return n.toFixed(props.precision)
  }
  return String(n)
}

const displayValue = computed(() => {
  if (!isNumeric.value) {
    return props.value as string
  }
  if (props.animation) {
    return formatNumber(animatedValue.value)
  }
  return formatNumber(props.value as number)
})

const valueStyle = computed(() => ({
  color: props.valueColor || 'var(--text-primary)'
}))

const trendClasses = computed(() => [
  'ui-statistic__trend',
  `ui-statistic__trend--${props.trend}`
])

function cancelAnimation() {
  if (rafId !== null) {
    cancelAnimationFrame(rafId)
    rafId = null
  }
}

function runAnimation() {
  cancelAnimation()

  if (!props.animation || !isNumeric.value) {
    animatedValue.value = isNumeric.value ? (props.value as number) : 0
    return
  }

  const target = props.value as number
  const duration = props.animationDuration
  let startTime: number | null = null

  const step = (timestamp: number) => {
    if (startTime === null) {
      startTime = timestamp
    }
    const elapsed = timestamp - startTime
    const progress = duration > 0 ? Math.min(elapsed / duration, 1) : 1
    // easeOutCubic for a refined settle
    const eased = 1 - Math.pow(1 - progress, 3)
    animatedValue.value = target * eased

    if (progress < 1) {
      rafId = requestAnimationFrame(step)
    } else {
      animatedValue.value = target
      rafId = null
    }
  }

  animatedValue.value = 0
  rafId = requestAnimationFrame(step)
}

onMounted(() => {
  if (props.animation && isNumeric.value) {
    runAnimation()
  }
})

watch(
  () => props.value,
  () => {
    if (props.animation && isNumeric.value) {
      runAnimation()
    }
  }
)

onBeforeUnmount(() => {
  cancelAnimation()
})
</script>

<style scoped>
.ui-statistic {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.ui-statistic__title {
  font-family: var(--font-body);
  font-size: 0.6875rem;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--text-muted);
}

.ui-statistic__content {
  display: flex;
  align-items: baseline;
  gap: var(--space-1);
}

.ui-statistic__prefix,
.ui-statistic__suffix {
  font-family: var(--font-mono);
  font-size: var(--text-lg);
  color: var(--text-secondary);
}

.ui-statistic__value {
  font-family: var(--font-mono);
  font-size: var(--text-3xl);
  font-weight: 600;
  line-height: 1.1;
  letter-spacing: -0.01em;
  font-variant-numeric: tabular-nums;
}

.ui-statistic__trend {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}

.ui-statistic__trend--up {
  color: var(--success);
}

.ui-statistic__trend--down {
  color: var(--danger);
}

.ui-statistic__trend-icon {
  width: 16px;
  height: 16px;
}

.ui-statistic__trend-value {
  font-family: var(--font-mono);
  font-size: var(--text-sm);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
</style>
