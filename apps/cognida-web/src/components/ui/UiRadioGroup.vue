<template>
  <div class="radio-group">
    <label
      v-for="option in options"
      :key="option.value"
      class="radio-item"
      :class="{ 'radio-item--checked': modelValue === option.value }"
    >
      <input
        type="radio"
        :name="name"
        :value="option.value"
        :checked="modelValue === option.value"
        @change="handleChange(option.value)"
        class="radio-input"
      />
      <span class="radio-indicator"></span>
      <span class="radio-label">{{ option.label }}</span>
    </label>
  </div>
</template>

<script setup lang="ts">
interface RadioOption {
  label: string
  value: string | number
}

interface Props {
  modelValue: string | number
  options: RadioOption[]
  name?: string
  disabled?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  name: () => `radio-${Math.random().toString(36).slice(2, 11)}`,
  disabled: false
})

const emit = defineEmits<{
  'update:modelValue': [value: string | number | boolean]
}>()

function handleChange(value: string | number | boolean) {
  if (!props.disabled) {
    emit('update:modelValue', value)
  }
}
</script>

<style scoped>
.radio-group {
  display: flex;
  gap: 16px;
}

.radio-item {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  user-select: none;
  position: relative;
}

.radio-item--disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.radio-input {
  position: absolute;
  opacity: 0;
  pointer-events: none;
}

.radio-indicator {
  position: relative;
  width: 18px;
  height: 18px;
  border: 2px solid var(--border-default);
  border-radius: 50%;
  background: var(--bg-primary);
  transition: all var(--duration-fast);
  flex-shrink: 0;
}

.radio-item:hover .radio-indicator {
  border-color: var(--primary);
}

.radio-item--checked .radio-indicator {
  border-color: var(--primary);
  background: var(--primary);
}

.radio-item--checked .radio-indicator::after {
  content: '';
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: white;
}

.radio-label {
  font-size: var(--text-sm);
  color: var(--text-primary);
  transition: color var(--duration-fast);
}

.radio-item:hover .radio-label {
  color: var(--primary);
}

.radio-item--checked .radio-label {
  font-weight: 500;
  color: var(--primary);
}
</style>
