<template>
  <label :class="wrapperClasses">
    <input
      v-model="checkboxValue"
      type="checkbox"
      :class="inputClasses"
      :disabled="disabled"
      :indeterminate="indeterminate"
      :value="value"
      :true-value="trueValue"
      :false-value="falseValue"
      @change="handleChange"
    >

    <span :class="boxClasses">
      <span class="ui-checkbox__icon">
        <svg v-if="indeterminate" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
          <line x1="5" y1="12" x2="19" y2="12" />
        </svg>
        <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
          <polyline points="20 6 9 17 4 12" />
        </svg>
      </span>
    </span>

    <span v-if="$slots.default || label" class="ui-checkbox__label">
      <slot>{{ label }}</slot>
    </span>
  </label>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  modelValue?: boolean | string | number | (string | number | boolean)[]
  label?: string
  value?: string | number | boolean
  trueValue?: boolean | string | number
  falseValue?: boolean | string | number
  disabled?: boolean
  indeterminate?: boolean
  size?: 'sm' | 'md' | 'lg'
  color?: 'primary' | 'success' | 'warning' | 'danger'
}

const props = withDefaults(defineProps<Props>(), {
  trueValue: true,
  falseValue: false,
  disabled: false,
  indeterminate: false,
  size: 'md',
  color: 'primary'
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean | string | number]
  'change': [value: boolean | string | number]
}>()

const checkboxValue = computed({
  get: () => {
    if (props.modelValue === undefined) return props.falseValue
    return props.modelValue
  },
  set: (value) => {
    if (props.indeterminate) return
    const newValue = value ? props.trueValue : props.falseValue
    emit('update:modelValue', newValue)
  }
})

const wrapperClasses = computed(() => [
  'ui-checkbox',
  `ui-checkbox--${props.size}`,
  `ui-checkbox--${props.color}`,
  {
    'ui-checkbox--disabled': props.disabled,
    'ui-checkbox--indeterminate': props.indeterminate,
    'ui-checkbox--checked': isChecked.value
  }
])

const inputClasses = computed(() => ['ui-checkbox__input'])

const boxClasses = computed(() => [
  'ui-checkbox__box',
  `ui-checkbox__box--${props.size}`
])

const isChecked = computed(() => {
  if (Array.isArray(props.modelValue)) {
    return props.value !== undefined && props.modelValue.includes(props.value)
  }
  return props.modelValue === props.trueValue
})

function handleChange(event: Event) {
  const target = event.target as HTMLInputElement
  let newValue: any

  if (Array.isArray(props.modelValue)) {
    newValue = [...props.modelValue]
    const index = newValue.indexOf(props.value)
    if (index > -1) {
      newValue.splice(index, 1)
    } else {
      newValue.push(props.value)
    }
  } else {
    newValue = target.checked ? props.trueValue : props.falseValue
  }

  emit('update:modelValue', newValue)
  emit('change', newValue)
}
</script>

<style scoped>
.ui-checkbox {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  cursor: pointer;
  user-select: none;
}

.ui-checkbox--disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.ui-checkbox__input {
  position: absolute;
  opacity: 0;
  width: 0;
  height: 0;
}

.ui-checkbox__box {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-tertiary);
  border: 2px solid var(--border-default);
  border-radius: var(--radius-sm);
  transition: all var(--duration-fast);
  flex-shrink: 0;
}

.ui-checkbox:hover .ui-checkbox__box:not(.ui-checkbox--disabled .ui-checkbox__box) {
  border-color: var(--border-strong);
}

.ui-checkbox--checked .ui-checkbox__box,
.ui-checkbox--indeterminate .ui-checkbox__box {
  background: var(--primary);
  border-color: var(--primary);
}

/* Colors */
.ui-checkbox--success.ui-checkbox--checked .ui-checkbox__box,
.ui-checkbox--success.ui-checkbox--indeterminate .ui-checkbox__box {
  background: var(--success);
  border-color: var(--success);
}

.ui-checkbox--warning.ui-checkbox--checked .ui-checkbox__box,
.ui-checkbox--warning.ui-checkbox--indeterminate .ui-checkbox__box {
  background: var(--warning);
  border-color: var(--warning);
}

.ui-checkbox--danger.ui-checkbox--checked .ui-checkbox__box,
.ui-checkbox--danger.ui-checkbox--indeterminate .ui-checkbox__box {
  background: var(--danger);
  border-color: var(--danger);
}

/* Sizes */
.ui-checkbox__box--sm {
  width: 16px;
  height: 16px;
}

.ui-checkbox__box--md {
  width: 20px;
  height: 20px;
}

.ui-checkbox__box--lg {
  width: 24px;
  height: 24px;
}

.ui-checkbox--sm .ui-checkbox__icon svg {
  width: 12px;
  height: 12px;
}

.ui-checkbox--md .ui-checkbox__icon svg {
  width: 14px;
  height: 14px;
}

.ui-checkbox--lg .ui-checkbox__icon svg {
  width: 16px;
  height: 16px;
}

/* Icon */
.ui-checkbox__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  opacity: 0;
  transform: scale(0.5);
  transition: all var(--duration-fast);
}

.ui-checkbox--checked .ui-checkbox__icon,
.ui-checkbox--indeterminate .ui-checkbox__icon {
  opacity: 1;
  transform: scale(1);
}

/* Label */
.ui-checkbox__label {
  font-size: var(--text-base);
  color: var(--text-primary);
  line-height: 1.5;
}

.ui-checkbox--disabled .ui-checkbox__label {
  color: var(--text-muted);
}

.ui-checkbox--sm .ui-checkbox__label {
  font-size: var(--text-sm);
}

.ui-checkbox--lg .ui-checkbox__label {
  font-size: var(--text-lg);
}

/* Focus */
.ui-checkbox__input:focus-visible + .ui-checkbox__box {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}
</style>
