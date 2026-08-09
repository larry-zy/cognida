<template>
  <div :class="wrapperClasses">
    <label v-if="label" :for="inputId" class="ui-input__label">
      {{ label }}
      <span v-if="required" class="ui-input__required">*</span>
    </label>

    <div class="ui-input__container">
      <span v-if="$slots.prefix" class="ui-input__prefix">
        <slot name="prefix" />
      </span>

      <input
        :id="inputId"
        ref="inputRef"
        v-model="inputValue"
        :class="inputClasses"
        :type="type"
        :placeholder="placeholder"
        :disabled="disabled"
        :readonly="readonly"
        :maxlength="maxlength"
        :minlength="minlength"
        :min="min"
        :max="max"
        :step="step"
        :autocomplete="autocomplete"
        :name="name"
        v-bind="$attrs"
      />

      <span v-if="$slots.suffix || clearable || showPasswordToggle" class="ui-input__suffix">
        <button
          v-if="clearable && inputValue && !disabled && !readonly"
          type="button"
          class="ui-input__clear"
          @click="handleClear"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M18 6L6 18M6 6l12 12" />
          </svg>
        </button>
        <button
          v-if="showPasswordToggle"
          type="button"
          class="ui-input__toggle"
          @click="togglePasswordVisibility"
        >
          <svg v-if="showPassword" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
            <circle cx="12" cy="12" r="3" />
          </svg>
          <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
            <line x1="1" y1="1" x2="23" y2="23" />
          </svg>
        </button>
        <slot name="suffix" />
      </span>
    </div>

    <div v-if="hint || error" class="ui-input__hint">
      <span v-if="error" class="ui-input__error">{{ error }}</span>
      <span v-else-if="hint" class="ui-input__hint-text">{{ hint }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, nextTick, useSlots } from 'vue'

interface Props {
  modelValue?: string | number
  type?: 'text' | 'password' | 'email' | 'number' | 'tel' | 'url' | 'search'
  label?: string
  placeholder?: string
  hint?: string
  error?: string
  disabled?: boolean
  readonly?: boolean
  required?: boolean
  clearable?: boolean
  showPasswordToggle?: boolean
  maxlength?: number
  minlength?: number
  min?: number | string
  max?: number | string
  step?: number | string
  autocomplete?: string
  name?: string
  id?: string
  size?: 'sm' | 'md' | 'lg'
}

const props = withDefaults(defineProps<Props>(), {
  type: 'text',
  disabled: false,
  readonly: false,
  required: false,
  clearable: false,
  showPasswordToggle: false,
  size: 'md'
})

const emit = defineEmits<{
  'update:modelValue': [value: string | number]
  'focus': [event: FocusEvent]
  'blur': [event: FocusEvent]
  'clear': []
  'change': [value: string | number]
}>()

const inputRef = ref<HTMLInputElement>()
const inputId = props.id || `ui-input-${Math.random().toString(36).substr(2, 9)}`
const showPassword = ref(false)
const slots = useSlots()

const inputValue = computed({
  get: () => props.modelValue ?? '',
  set: (value) => emit('update:modelValue', value)
})

const inputClasses = computed(() => [
  'ui-input',
  `ui-input--${props.size}`,
  {
    'ui-input--error': props.error,
    'ui-input--disabled': props.disabled,
    'ui-input--readonly': props.readonly,
    'ui-input--has-prefix': !!slots.prefix,
    'ui-input--has-suffix': !!slots.suffix || props.clearable || props.showPasswordToggle
  }
])

const wrapperClasses = computed(() => [
  'ui-input-wrapper',
  `ui-input-wrapper--${props.size}`
])

function handleClear() {
  inputValue.value = ''
  emit('clear')
  nextTick(() => inputRef.value?.focus())
}

function togglePasswordVisibility() {
  showPassword.value = !showPassword.value
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
.ui-input-wrapper {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.ui-input__label {
  font-size: var(--text-sm);
  font-weight: 500;
  color: var(--text-secondary);
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.ui-input__required {
  color: var(--danger);
}

.ui-input__container {
  position: relative;
  display: flex;
  align-items: center;
}

.ui-input {
  width: 100%;
  font-family: var(--font-body);
  font-size: var(--text-base);
  color: var(--text-primary);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  transition: all var(--duration-fast);
}

.ui-input::placeholder {
  color: var(--text-muted);
}

.ui-input:hover:not(.ui-input--disabled):not(.ui-input--readonly) {
  border-color: var(--border-strong);
}

.ui-input:focus {
  outline: none;
  border-color: var(--primary);
}

.ui-input--error {
  border-color: var(--danger);
}

.ui-input--error:focus {
  border-color: var(--danger);
}

.ui-input--disabled,
.ui-input--readonly {
  background: var(--bg-secondary);
  color: var(--text-muted);
  cursor: not-allowed;
}

.ui-input--has-prefix {
  padding-left: var(--space-10);
}

.ui-input--has-suffix {
  padding-right: var(--space-10);
}

/* Sizes */
.ui-input--sm {
  padding: var(--space-2) var(--space-3);
  font-size: var(--text-sm);
}

.ui-input--sm.ui-input--has-prefix {
  padding-left: var(--space-8);
}

.ui-input--sm.ui-input--has-suffix {
  padding-right: var(--space-8);
}

.ui-input--lg {
  padding: var(--space-4) var(--space-4);
  font-size: var(--text-lg);
}

.ui-input--lg.ui-input--has-prefix {
  padding-left: var(--space-12);
}

.ui-input--lg.ui-input--has-suffix {
  padding-right: var(--space-12);
}

/* Prefix */
.ui-input__prefix {
  position: absolute;
  left: var(--space-3);
  display: flex;
  align-items: center;
  color: var(--text-muted);
  pointer-events: none;
}

.ui-input--sm .ui-input__prefix {
  left: var(--space-2);
}

.ui-input--lg .ui-input__prefix {
  left: var(--space-4);
}

/* Suffix */
.ui-input__suffix {
  position: absolute;
  right: var(--space-3);
  display: flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--text-muted);
}

.ui-input--sm .ui-input__suffix {
  right: var(--space-2);
}

.ui-input--lg .ui-input__suffix {
  right: var(--space-4);
}

.ui-input__clear,
.ui-input__toggle {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  cursor: pointer;
  transition: all var(--duration-fast);
}

.ui-input__clear:hover,
.ui-input__toggle:hover {
  color: var(--text-secondary);
  background: var(--bg-hover);
}

.ui-input__clear svg,
.ui-input__toggle svg {
  width: 16px;
  height: 16px;
}

/* Hint */
.ui-input__hint {
  display: flex;
  align-items: center;
  min-height: 20px;
}

.ui-input__hint-text {
  font-size: var(--text-sm);
  color: var(--text-muted);
}

.ui-input__error {
  font-size: var(--text-sm);
  color: var(--danger);
}

/* Remove number input spinners */
.ui-input[type='number'] {
  -moz-appearance: textfield;
}

.ui-input[type='number']::-webkit-inner-spin-button,
.ui-input[type='number']::-webkit-outer-spin-button {
  -webkit-appearance: none;
  margin: 0;
}

/* Remove password input reveal button (Edge) */
.ui-input[type='password']::-ms-reveal {
  display: none;
}

/* Search input styles */
.ui-input[type='search']::-webkit-search-decoration,
.ui-input[type='search']::-webkit-search-cancel-button {
  display: none;
}
</style>
