<template>
  <div :class="wrapperClasses">
    <!-- Line -->
    <template v-if="type === 'line'">
      <div class="ui-progress__line">
        <div class="ui-progress__track" :style="{ height: `${strokeWidth}px` }">
          <div
            class="ui-progress__bar"
            :class="{
              'ui-progress__bar--active': status === 'active',
              'ui-progress__bar--indeterminate': indeterminate
            }"
            :style="barStyle"
          >
            <span
              v-if="showText && textInside && !indeterminate"
              class="ui-progress__text ui-progress__text--inside"
            >
              {{ displayText }}
            </span>
          </div>
        </div>
        <span
          v-if="showText && !textInside && !indeterminate"
          class="ui-progress__text ui-progress__text--outside"
        >
          {{ displayText }}
        </span>
      </div>
    </template>

    <!-- Circle -->
    <template v-else>
      <div
        class="ui-progress__circle"
        :style="{ width: `${width}px`, height: `${width}px` }"
      >
        <svg :viewBox="`0 0 ${width} ${width}`" class="ui-progress__svg">
          <circle
            class="ui-progress__circle-track"
            :cx="center"
            :cy="center"
            :r="radius"
            fill="none"
            :stroke-width="strokeWidth"
          />
          <circle
            class="ui-progress__circle-bar"
            :class="{ 'ui-progress__circle-bar--indeterminate': indeterminate }"
            :cx="center"
            :cy="center"
            :r="radius"
            fill="none"
            stroke-linecap="round"
            :stroke-width="strokeWidth"
            :stroke="resolvedStroke"
            :stroke-dasharray="circumference"
            :stroke-dashoffset="dashOffset"
            :style="{ stroke: resolvedStroke }"
          />
        </svg>
        <span v-if="showText && !indeterminate" class="ui-progress__text ui-progress__text--circle">
          {{ displayText }}
        </span>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  percentage: number
  type?: 'line' | 'circle'
  strokeWidth?: number
  status?: 'normal' | 'success' | 'warning' | 'danger' | 'active'
  showText?: boolean
  textInside?: boolean
  color?: string
  width?: number
  indeterminate?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  type: 'line',
  status: 'normal',
  showText: true,
  textInside: false,
  width: 120,
  indeterminate: false
})

const strokeWidth = computed(() => {
  if (props.strokeWidth !== undefined) return props.strokeWidth
  return props.type === 'circle' ? 6 : 8
})

const clampedPercentage = computed(() => Math.min(100, Math.max(0, props.percentage)))

const displayText = computed(() => `${Math.round(clampedPercentage.value)}%`)

const statusColorMap: Record<string, string> = {
  success: 'var(--success)',
  warning: 'var(--warning)',
  danger: 'var(--danger)'
}

const resolvedStroke = computed(() => {
  if (props.color) return props.color
  return statusColorMap[props.status] ?? 'var(--primary)'
})

// Line fill: gradient by default, solid status/custom color otherwise.
const barStyle = computed(() => {
  if (props.indeterminate) return {}
  const style: Record<string, string> = {
    width: `${clampedPercentage.value}%`
  }
  if (props.color) {
    style.background = props.color
  } else if (statusColorMap[props.status]) {
    style.background = statusColorMap[props.status]
  }
  return style
})

// Circle geometry
const center = computed(() => props.width / 2)
const radius = computed(() => props.width / 2 - strokeWidth.value / 2)
const circumference = computed(() => 2 * Math.PI * radius.value)
const dashOffset = computed(
  () => circumference.value * (1 - clampedPercentage.value / 100)
)

const wrapperClasses = computed(() => [
  'ui-progress',
  `ui-progress--${props.type}`,
  `ui-progress--${props.status}`
])
</script>

<style scoped>
.ui-progress {
  display: flex;
  width: 100%;
}

.ui-progress--circle {
  width: auto;
}

/* Line */
.ui-progress__line {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  width: 100%;
}

.ui-progress__track {
  position: relative;
  flex: 1 1 auto;
  min-width: 0;
  background: var(--bg-tertiary);
  border-radius: var(--radius-full);
  overflow: hidden;
}

.ui-progress__bar {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  height: 100%;
  border-radius: var(--radius-full);
  background: linear-gradient(90deg, var(--primary), var(--primary-dark));
  box-shadow: 0 0 6px rgba(34, 211, 238, 0.35);
  transition: width var(--duration-base) var(--ease-default);
}

/* Active: shimmer overlay flowing across the fill */
.ui-progress__bar--active::after {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: var(--radius-full);
  background: linear-gradient(
    90deg,
    rgba(255, 255, 255, 0) 0%,
    rgba(255, 255, 255, 0.25) 50%,
    rgba(255, 255, 255, 0) 100%
  );
  background-size: 200% 100%;
  animation: ui-progress-shimmer 1.5s linear infinite;
}

/* Indeterminate: looping sweep on the track */
.ui-progress__bar--indeterminate {
  position: absolute;
  width: 40%;
  border-radius: var(--radius-full);
  animation: ui-progress-indeterminate 1.4s var(--ease-default) infinite;
}

@keyframes ui-progress-shimmer {
  0% {
    background-position: 200% 0;
  }
  100% {
    background-position: -200% 0;
  }
}

@keyframes ui-progress-indeterminate {
  0% {
    left: -40%;
  }
  100% {
    left: 100%;
  }
}

/* Text */
.ui-progress__text {
  font-family: var(--font-mono);
  font-size: var(--text-sm);
  color: var(--text-secondary);
  white-space: nowrap;
}

.ui-progress__text--outside {
  flex: 0 0 auto;
  min-width: 3ch;
  text-align: right;
}

.ui-progress__text--inside {
  font-size: var(--text-xs);
  color: var(--bg-primary);
  padding: 0 var(--space-2);
}

/* Circle */
.ui-progress__circle {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.ui-progress__svg {
  width: 100%;
  height: 100%;
  transform: rotate(-90deg);
}

.ui-progress__circle-track {
  stroke: var(--bg-tertiary);
}

.ui-progress__circle-bar {
  transition: stroke-dashoffset var(--duration-base) var(--ease-default);
  filter: drop-shadow(0 0 4px rgba(34, 211, 238, 0.35));
}

.ui-progress__circle-bar--indeterminate {
  transform-origin: center;
  animation: ui-progress-circle-spin 1.4s linear infinite;
}

@keyframes ui-progress-circle-spin {
  0% {
    transform: rotate(0deg);
  }
  100% {
    transform: rotate(360deg);
  }
}

.ui-progress__text--circle {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  font-size: var(--text-lg);
  color: var(--text-primary);
}

/* Status color overrides for line glow */
.ui-progress--success .ui-progress__bar,
.ui-progress--warning .ui-progress__bar,
.ui-progress--danger .ui-progress__bar {
  box-shadow: none;
}
</style>
