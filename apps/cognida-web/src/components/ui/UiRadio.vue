<template>
  <label :class="wrapperClasses">
    <input
      v-model="radioValue"
      type="radio"
      :class="inputClasses"
      :disabled="disabled"
      :value="value"
      @change="handleChange"
    >

    <span :class="radioClasses">
      <span class="ui-radio__dot"></span>
    </span>

    <span v-if="$slots.default || label" class="ui-radio__label">
      <slot>{{ label }}</slot>
    </span>
  </label>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  modelValue?: string | number | boolean
  label?: string
  value: string | number | boolean
  disabled?: boolean
  size?: 'sm' | 'md' | 'lg'
  color?: 'primary' | 'success' | 'warning' | 'danger'
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  size: 'md',
  color: 'primary'
})

const emit = defineEmits<{
  'update:modelValue': [value: any]
  'change': [value: any]
}>()

const radioValue = computed({
  get: () => props.modelValue,
  set: (value) => {
    emit('update:modelValue', value)
  }
})

const wrapperClasses = computed(() => [
  'ui-radio',
  `ui-radio--${props.size}`,
  `ui-radio--${props.color}`,
  {
    'ui-radio--disabled': props.disabled,
    'ui-radio--checked': isChecked.value
  }
])

const inputClasses = computed(() => ['ui-radio__input'])

const radioClasses = computed(() => [
  'ui-radio__outer',
  `ui-radio__outer--${props.size}`
])

const isChecked = computed(() => props.modelValue === props.value)

function handleChange(event: Event) {
  const target = event.target as HTMLInputElement
  emit('update:modelValue', target.value)
  emit('change', target.value)
}
</script>

<style scoped>
.ui-radio {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  cursor: pointer;
  user-select: none;
}

.ui-radio--disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.ui-radio__input {
  position: absolute;
  opacity: 0;
  width: 0;
  height: 0;
}

.ui-radio__outer {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-tertiary);
  border: 2px solid var(--border-default);
  border-radius: 50%;
  transition: all var(--duration-fast);
  flex-shrink: 0;
}

.ui-radio:hover .ui-radio__outer:not(.ui-radio--disabled .ui-radio__outer) {
  border-color: var(--border-strong);
}

.ui-radio--checked .ui-radio__outer {
  border-color: var(--primary);
}

/* Colors */
.ui-radio--success.ui-radio--checked .ui-radio__outer {
  border-color: var(--success);
}

.ui-radio--warning.ui-radio--checked .ui-radio__outer {
  border-color: var(--warning);
}

.ui-radio--danger.ui-radio--checked .ui-radio__outer {
  border-color: var(--danger);
}

/* Sizes */
.ui-radio__outer--sm {
  width: 16px;
  height: 16px;
}

.ui-radio__outer--md {
  width: 20px;
  height: 20px;
}

.ui-radio__outer--lg {
  width: 24px;
  height: 24px;
}

/* Dot */
.ui-radio__dot {
  border-radius: 50%;
  background: var(--primary);
  opacity: 0;
  transform: scale(0);
  transition: all var(--duration-fast);
}

.ui-radio--sm .ui-radio__dot {
  width: 8px;
  height: 8px;
}

.ui-radio--md .ui-radio__dot {
  width: 10px;
  height: 10px;
}

.ui-radio--lg .ui-radio__dot {
  width: 12px;
  height: 12px;
}

.ui-radio--checked .ui-radio__dot {
  opacity: 1;
  transform: scale(1);
}

/* Colors dot */
.ui-radio--success.ui-radio--checked .ui-radio__dot {
  background: var(--success);
}

.ui-radio--warning.ui-radio--checked .ui-radio__dot {
  background: var(--warning);
}

.ui-radio--danger.ui-radio--checked .ui-radio__dot {
  background: var(--danger);
}

/* Label */
.ui-radio__label {
  font-size: var(--text-base);
  color: var(--text-primary);
  line-height: 1.5;
}

.ui-radio--disabled .ui-radio__label {
  color: var(--text-muted);
}

.ui-radio--sm .ui-radio__label {
  font-size: var(--text-sm);
}

.ui-radio--lg .ui-radio__label {
  font-size: var(--text-lg);
}

/* Focus */
.ui-radio__input:focus-visible + .ui-radio__outer {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}
</style>
