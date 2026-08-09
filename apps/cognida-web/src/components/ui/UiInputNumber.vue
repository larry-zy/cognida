<template>
  <div :class="wrapperClasses">
    <button
      v-if="controls && controlsPosition === 'default'"
      type="button"
      class="ui-input-number__btn ui-input-number__btn--decrease"
      :disabled="disabled || decreaseDisabled"
      tabindex="-1"
      @click="handleDecrease"
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M5 12h14" />
      </svg>
    </button>

    <input
      ref="inputRef"
      class="ui-input-number__input"
      type="text"
      inputmode="decimal"
      :value="displayValue"
      :placeholder="placeholder"
      :disabled="disabled"
      @input="handleInput"
      @focus="handleFocus"
      @blur="handleBlur"
    />

    <button
      v-if="controls && controlsPosition === 'default'"
      type="button"
      class="ui-input-number__btn ui-input-number__btn--increase"
      :disabled="disabled || increaseDisabled"
      tabindex="-1"
      @click="handleIncrease"
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M12 5v14M5 12h14" />
      </svg>
    </button>

    <div v-if="controls && controlsPosition === 'right'" class="ui-input-number__controls-right">
      <button
        type="button"
        class="ui-input-number__btn ui-input-number__btn--stacked ui-input-number__btn--increase"
        :disabled="disabled || increaseDisabled"
        tabindex="-1"
        @click="handleIncrease"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M18 15l-6-6-6 6" />
        </svg>
      </button>
      <button
        type="button"
        class="ui-input-number__btn ui-input-number__btn--stacked ui-input-number__btn--decrease"
        :disabled="disabled || decreaseDisabled"
        tabindex="-1"
        @click="handleDecrease"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M6 9l6 6 6-6" />
        </svg>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'

interface Props {
  modelValue?: number
  min?: number
  max?: number
  step?: number
  precision?: number
  disabled?: boolean
  size?: 'sm' | 'md' | 'lg'
  controls?: boolean
  controlsPosition?: 'default' | 'right'
  placeholder?: string
}

const props = withDefaults(defineProps<Props>(), {
  step: 1,
  disabled: false,
  size: 'md',
  controls: true,
  controlsPosition: 'default'
})

const emit = defineEmits<{
  'update:modelValue': [value: number]
  'change': [value: number]
  'blur': [event: FocusEvent]
  'focus': [event: FocusEvent]
}>()

const inputRef = ref<HTMLInputElement>()
// Raw text buffer while the user is actively typing (null = show model value).
const rawInput = ref<string | null>(null)

const currentValue = computed(() => props.modelValue)

const displayValue = computed(() => {
  if (rawInput.value !== null) return rawInput.value
  if (currentValue.value === undefined || currentValue.value === null) return ''
  return formatValue(currentValue.value)
})

const decreaseDisabled = computed(() => {
  if (props.min === undefined) return false
  const base = currentValue.value ?? props.min
  return clamp(base - props.step) <= base && base <= props.min
})

const increaseDisabled = computed(() => {
  if (props.max === undefined) return false
  const base = currentValue.value ?? props.max
  return clamp(base + props.step) >= base && base >= props.max
})

const wrapperClasses = computed(() => [
  'ui-input-number',
  `ui-input-number--${props.size}`,
  {
    'ui-input-number--disabled': props.disabled,
    'ui-input-number--controls-right': props.controls && props.controlsPosition === 'right',
    'ui-input-number--no-controls': !props.controls
  }
])

function formatValue(value: number): string {
  if (props.precision !== undefined) return value.toFixed(props.precision)
  return String(value)
}

function clamp(value: number): number {
  let next = value
  if (props.min !== undefined && next < props.min) next = props.min
  if (props.max !== undefined && next > props.max) next = props.max
  return next
}

function applyPrecision(value: number): number {
  if (props.precision !== undefined) return Number(value.toFixed(props.precision))
  return value
}

function setValue(value: number) {
  const next = applyPrecision(clamp(value))
  if (next !== currentValue.value) {
    emit('update:modelValue', next)
    emit('change', next)
  }
}

function handleInput(event: Event) {
  rawInput.value = (event.target as HTMLInputElement).value
}

function handleIncrease() {
  if (props.disabled) return
  const base = currentValue.value ?? props.min ?? 0
  rawInput.value = null
  setValue(base + props.step)
}

function handleDecrease() {
  if (props.disabled) return
  const base = currentValue.value ?? props.max ?? 0
  rawInput.value = null
  setValue(base - props.step)
}

function handleFocus(event: FocusEvent) {
  emit('focus', event)
}

function handleBlur(event: FocusEvent) {
  // Resolve the raw buffer to a legal value; revert on illegal input.
  if (rawInput.value !== null) {
    const trimmed = rawInput.value.trim()
    if (trimmed === '') {
      const fallback = props.min ?? currentValue.value
      if (fallback !== undefined) setValue(fallback)
    } else {
      const parsed = Number(trimmed)
      if (Number.isNaN(parsed)) {
        // Illegal: fall back to existing legal value (re-render from model).
      } else {
        setValue(parsed)
      }
    }
    rawInput.value = null
  }
  emit('blur', event)
}

function focus() {
  inputRef.value?.focus()
}

function blur() {
  inputRef.value?.blur()
}

defineExpose({
  focus,
  blur
})
</script>

<style scoped>
.ui-input-number {
  display: inline-flex;
  align-items: stretch;
  width: 100%;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  overflow: hidden;
  transition: all var(--duration-fast);
}

.ui-input-number:hover:not(.ui-input-number--disabled) {
  border-color: var(--border-strong);
}

.ui-input-number:focus-within:not(.ui-input-number--disabled) {
  border-color: var(--primary);
  box-shadow: 0 0 0 3px rgba(34, 211, 238, 0.1);
}

.ui-input-number--disabled {
  background: var(--bg-secondary);
  cursor: not-allowed;
}

/* Input */
.ui-input-number__input {
  flex: 1 1 auto;
  min-width: 0;
  width: 100%;
  text-align: center;
  font-family: var(--font-mono);
  font-size: var(--text-base);
  color: var(--text-primary);
  background: transparent;
  border: none;
  outline: none;
  padding: var(--space-3);
}

.ui-input-number__input::placeholder {
  color: var(--text-muted);
  font-family: var(--font-body);
}

.ui-input-number--disabled .ui-input-number__input {
  color: var(--text-muted);
  cursor: not-allowed;
}

/* Stepper buttons */
.ui-input-number__btn {
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 40px;
  padding: 0;
  background: var(--bg-tertiary);
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--duration-fast);
}

.ui-input-number__btn--decrease {
  border-right: 1px solid var(--border-default);
}

.ui-input-number__btn--increase {
  border-left: 1px solid var(--border-default);
}

.ui-input-number__btn:hover:not(:disabled) {
  background: var(--bg-elevated);
  color: var(--primary);
}

.ui-input-number__btn:active:not(:disabled) {
  background: var(--bg-active);
}

.ui-input-number__btn:disabled {
  color: var(--text-disabled);
  cursor: not-allowed;
}

.ui-input-number__btn svg {
  width: 16px;
  height: 16px;
}

/* Controls on the right (stacked) */
.ui-input-number__controls-right {
  display: flex;
  flex-direction: column;
  flex: 0 0 auto;
  border-left: 1px solid var(--border-default);
}

.ui-input-number__btn--stacked {
  width: 32px;
  flex: 1 1 0;
  border: none;
}

.ui-input-number__btn--stacked.ui-input-number__btn--increase {
  border-bottom: 1px solid var(--border-default);
}

.ui-input-number--controls-right .ui-input-number__input {
  text-align: left;
}

/* Sizes */
.ui-input-number--sm .ui-input-number__input {
  padding: var(--space-2) var(--space-3);
  font-size: var(--text-sm);
}

.ui-input-number--sm .ui-input-number__btn {
  width: 32px;
}

.ui-input-number--sm .ui-input-number__btn--stacked {
  width: 28px;
}

.ui-input-number--sm .ui-input-number__btn svg {
  width: 14px;
  height: 14px;
}

.ui-input-number--lg .ui-input-number__input {
  padding: var(--space-4);
  font-size: var(--text-lg);
}

.ui-input-number--lg .ui-input-number__btn {
  width: 48px;
}

.ui-input-number--lg .ui-input-number__btn--stacked {
  width: 40px;
}

.ui-input-number--lg .ui-input-number__btn svg {
  width: 18px;
  height: 18px;
}
</style>
