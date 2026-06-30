<template>
  <label :class="wrapperClasses">
    <input
      v-model="switchValue"
      type="checkbox"
      :class="inputClasses"
      :disabled="disabled"
      @change="handleChange"
    >

    <span :class="trackClasses">
      <span :class="thumbClasses"></span>
    </span>

    <span v-if="$slots.default || label" class="ui-switch__label">
      <slot>{{ label }}</slot>
    </span>
  </label>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  modelValue?: boolean | string | number
  label?: string
  trueValue?: boolean | string | number
  falseValue?: boolean | string | number
  disabled?: boolean
  size?: 'sm' | 'md' | 'lg'
  color?: 'primary' | 'success' | 'warning' | 'danger'
}

const props = withDefaults(defineProps<Props>(), {
  trueValue: true,
  falseValue: false,
  disabled: false,
  size: 'md',
  color: 'primary'
})

const emit = defineEmits<{
  'update:modelValue': [value: any]
  'change': [value: any]
}>()

const switchValue = computed({
  get: () => props.modelValue,
  set: (value) => {
    emit('update:modelValue', value)
  }
})

const wrapperClasses = computed(() => [
  'ui-switch',
  `ui-switch--${props.size}`,
  `ui-switch--${props.color}`,
  {
    'ui-switch--disabled': props.disabled,
    'ui-switch--checked': isChecked.value
  }
])

const inputClasses = computed(() => ['ui-switch__input'])

const trackClasses = computed(() => [
  'ui-switch__track',
  `ui-switch__track--${props.size}`
])

const thumbClasses = computed(() => [
  'ui-switch__thumb',
  `ui-switch__thumb--${props.size}`
])

const isChecked = computed(() => props.modelValue === props.trueValue)

function handleChange(event: Event) {
  const target = event.target as HTMLInputElement
  const newValue = target.checked ? props.trueValue : props.falseValue
  emit('update:modelValue', newValue)
  emit('change', newValue)
}
</script>

<style scoped>
.ui-switch {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: var(--space-3);
  cursor: pointer;
  user-select: none;
}

.ui-switch--disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.ui-switch__input {
  position: absolute;
  opacity: 0;
  width: 0;
  height: 0;
}

.ui-switch__track {
  position: relative;
  display: inline-block;
  background: var(--bg-tertiary);
  border: 2px solid var(--border-default);
  border-radius: var(--radius-full);
  transition: all var(--duration-base);
  flex-shrink: 0;
}

.ui-switch:hover .ui-switch__track:not(.ui-switch--disabled .ui-switch__track) {
  border-color: var(--border-strong);
}

.ui-switch--checked .ui-switch__track {
  background: var(--primary);
  border-color: var(--primary);
}

/* Colors */
.ui-switch--success.ui-switch--checked .ui-switch__track {
  background: var(--success);
  border-color: var(--success);
}

.ui-switch--warning.ui-switch--checked .ui-switch__track {
  background: var(--warning);
  border-color: var(--warning);
}

.ui-switch--danger.ui-switch--checked .ui-switch__track {
  background: var(--danger);
  border-color: var(--danger);
}

/* Sizes track */
.ui-switch__track--sm {
  width: 32px;
  height: 18px;
}

.ui-switch__track--md {
  width: 44px;
  height: 24px;
}

.ui-switch__track--lg {
  width: 56px;
  height: 30px;
}

/* Thumb */
.ui-switch__thumb {
  position: absolute;
  top: 2px;
  left: 2px;
  background: white;
  border-radius: 50%;
  transition: transform var(--duration-base);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
}

.ui-switch--checked .ui-switch__thumb {
  transform: translateX(100%);
}

/* Sizes thumb */
.ui-switch__thumb--sm {
  width: 12px;
  height: 12px;
}

.ui-switch__thumb--md {
  width: 18px;
  height: 18px;
}

.ui-switch__thumb--lg {
  width: 24px;
  height: 24px;
}

/* Label */
.ui-switch__label {
  font-size: var(--text-base);
  color: var(--text-primary);
  line-height: 1.5;
}

.ui-switch--disabled .ui-switch__label {
  color: var(--text-muted);
}

.ui-switch--sm .ui-switch__label {
  font-size: var(--text-sm);
}

.ui-switch--lg .ui-switch__label {
  font-size: var(--text-lg);
}

/* Focus */
.ui-switch__input:focus-visible + .ui-switch__track {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}
</style>
