<template>
  <div :class="wrapperClasses">
    <label v-if="label" class="ui-slider__label">
      {{ label }}
      <span v-if="showValue" class="ui-slider__value">{{ displayValue }}</span>
    </label>

    <div
      :class="trackClasses"
      ref="trackRef"
      @mousedown="handleDragStart"
      @touchstart="handleDragStart"
    >
      <div :class="fillClasses" :style="fillStyle"></div>
      <div
        :class="thumbClasses"
        :style="thumbStyle"
        @keydown="handleKeydown"
        tabindex="0"
      ></div>

      <div v-if="marks && marks.length" class="ui-slider__marks">
        <div
          v-for="(mark, index) in marks"
          :key="index"
          :class="[
            'ui-slider__mark',
            { 'ui-slider__mark--active': mark.value <= internalValue }
          ]"
          :style="{ left: `${((mark.value - min) / (max - min)) * 100}%` }"
        >
          <div class="ui-slider__mark-point"></div>
          <div v-if="mark.label" class="ui-slider__mark-label">{{ mark.label }}</div>
        </div>
      </div>
    </div>

    <div v-if="hint" class="ui-slider__hint">{{ hint }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'

interface Mark {
  value: number
  label?: string
}

interface Props {
  modelValue?: number
  label?: string
  hint?: string
  min?: number
  max?: number
  step?: number
  disabled?: boolean
  showValue?: boolean
  marks?: Mark[]
  formatValue?: (value: number) => string
  size?: 'sm' | 'md' | 'lg'
}

const props = withDefaults(defineProps<Props>(), {
  min: 0,
  max: 100,
  step: 1,
  disabled: false,
  showValue: true,
  formatValue: (v: number) => v.toString(),
  size: 'md'
})

const emit = defineEmits<{
  'update:modelValue': [value: number]
  'change': [value: number]
}>()

const trackRef = ref<HTMLElement>()
const isDragging = ref(false)
const internalValue = ref(props.modelValue ?? props.min)

const wrapperClasses = computed(() => [
  'ui-slider',
  `ui-slider--${props.size}`,
  { 'ui-slider--disabled': props.disabled }
])

const trackClasses = computed(() => [
  'ui-slider__track',
  `ui-slider__track--${props.size}`
])

const fillClasses = computed(() => [
  'ui-slider__fill',
  `ui-slider__fill--${props.size}`
])

const thumbClasses = computed(() => [
  'ui-slider__thumb',
  `ui-slider__thumb--${props.size}`
])

const displayValue = computed(() => props.formatValue(internalValue.value))

const percentage = computed(() => {
  return ((internalValue.value - props.min) / (props.max - props.min)) * 100
})

const fillStyle = computed(() => ({
  width: `${percentage.value}%`
}))

const thumbStyle = computed(() => ({
  left: `${percentage.value}%`
}))

function updateValue(clientX: number) {
  if (!trackRef.value || props.disabled) return

  const rect = trackRef.value.getBoundingClientRect()
  const percentage = (clientX - rect.left) / rect.width
  const rawValue = props.min + percentage * (props.max - props.min)

  let newValue = Math.round(rawValue / props.step) * props.step
  newValue = Math.max(props.min, Math.min(props.max, newValue))

  internalValue.value = newValue
  emit('update:modelValue', newValue)
  emit('change', newValue)
}

function handleDragStart(event: MouseEvent | TouchEvent) {
  if (props.disabled) return

  event.preventDefault()
  isDragging.value = true

  const clientX = 'touches' in event ? event.touches[0].clientX : event.clientX
  updateValue(clientX)

  document.addEventListener('mousemove', handleDragMove)
  document.addEventListener('mouseup', handleDragEnd)
  document.addEventListener('touchmove', handleDragMove)
  document.addEventListener('touchend', handleDragEnd)
}

function handleDragMove(event: MouseEvent | TouchEvent) {
  if (!isDragging.value) return

  const clientX = 'touches' in event ? event.touches[0].clientX : event.clientX
  updateValue(clientX)
}

function handleDragEnd() {
  isDragging.value = false

  document.removeEventListener('mousemove', handleDragMove)
  document.removeEventListener('mouseup', handleDragEnd)
  document.removeEventListener('touchmove', handleDragMove)
  document.removeEventListener('touchend', handleDragEnd)
}

function handleKeydown(event: KeyboardEvent) {
  if (props.disabled) return

  let newValue = internalValue.value

  switch (event.key) {
    case 'ArrowRight':
    case 'ArrowUp':
      event.preventDefault()
      newValue = Math.min(props.max, internalValue.value + props.step)
      break
    case 'ArrowLeft':
    case 'ArrowDown':
      event.preventDefault()
      newValue = Math.max(props.min, internalValue.value - props.step)
      break
    case 'Home':
      event.preventDefault()
      newValue = props.min
      break
    case 'End':
      event.preventDefault()
      newValue = props.max
      break
    default:
      return
  }

  internalValue.value = newValue
  emit('update:modelValue', newValue)
  emit('change', newValue)
}

onMounted(() => {
  internalValue.value = props.modelValue ?? props.min
})

onBeforeUnmount(() => {
  document.removeEventListener('mousemove', handleDragMove)
  document.removeEventListener('mouseup', handleDragEnd)
  document.removeEventListener('touchmove', handleDragMove)
  document.removeEventListener('touchend', handleDragEnd)
})
</script>

<style scoped>
.ui-slider {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.ui-slider--disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.ui-slider__label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: var(--text-sm);
  font-weight: 500;
  color: var(--text-secondary);
}

.ui-slider__value {
  font-family: var(--font-mono);
  color: var(--primary);
}

.ui-slider__track {
  position: relative;
  width: 100%;
  background: var(--bg-tertiary);
  border-radius: var(--radius-full);
  cursor: pointer;
}

.ui-slider--disabled .ui-slider__track {
  cursor: not-allowed;
}

.ui-slider__track--sm {
  height: 4px;
}

.ui-slider__track--md {
  height: 6px;
}

.ui-slider__track--lg {
  height: 8px;
}

.ui-slider__fill {
  position: absolute;
  top: 0;
  left: 0;
  height: 100%;
  background: var(--primary);
  border-radius: var(--radius-full);
  pointer-events: none;
}

.ui-slider__fill--sm {
  height: 4px;
}

.ui-slider__fill--md {
  height: 6px;
}

.ui-slider__fill--lg {
  height: 8px;
}

.ui-slider__thumb {
  position: absolute;
  top: 50%;
  transform: translate(-50%, -50%);
  background: white;
  border: 2px solid var(--primary);
  border-radius: 50%;
  cursor: grab;
  transition: transform var(--duration-fast), box-shadow var(--duration-fast);
  z-index: 1;
}

.ui-slider__thumb:hover {
  transform: translate(-50%, -50%) scale(1.1);
  box-shadow: 0 0 0 4px rgba(34, 211, 238, 0.1);
}

.ui-slider__thumb:active {
  cursor: grabbing;
  transform: translate(-50%, -50%) scale(1.2);
}

.ui-slider--disabled .ui-slider__thumb {
  cursor: not-allowed;
}

.ui-slider__thumb--sm {
  width: 14px;
  height: 14px;
}

.ui-slider__thumb--md {
  width: 18px;
  height: 18px;
}

.ui-slider__thumb--lg {
  width: 22px;
  height: 22px;
}

.ui-slider__thumb:focus-visible {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}

/* Marks */
.ui-slider__marks {
  position: absolute;
  top: 100%;
  left: 0;
  width: 100%;
  margin-top: var(--space-2);
  pointer-events: none;
}

.ui-slider__mark {
  position: absolute;
  transform: translateX(-50%);
  display: flex;
  flex-direction: column;
  align-items: center;
}

.ui-slider__mark-point {
  width: 4px;
  height: 4px;
  background: var(--border-default);
  border-radius: 50%;
}

.ui-slider__mark--active .ui-slider__mark-point {
  background: var(--primary);
}

.ui-slider__mark-label {
  margin-top: var(--space-1);
  font-size: var(--text-xs);
  color: var(--text-muted);
}

.ui-slider__mark--active .ui-slider__mark-label {
  color: var(--text-secondary);
}

/* Hint */
.ui-slider__hint {
  font-size: var(--text-sm);
  color: var(--text-muted);
}
</style>
