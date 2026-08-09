<template>
  <div :class="wrapperClasses">
    <label v-if="label" :for="textareaId" class="ui-textarea__label">
      {{ label }}
      <span v-if="required" class="ui-textarea__required">*</span>
    </label>

    <div class="ui-textarea__container">
      <textarea
        :id="textareaId"
        ref="textareaRef"
        v-model="textareaValue"
        :class="textareaClasses"
        :placeholder="placeholder"
        :disabled="disabled"
        :readonly="readonly"
        :maxlength="maxlength"
        :minlength="minlength"
        :rows="rows"
        :cols="cols"
        :autocomplete="autocomplete"
        :name="name"
        :resize="resize"
        v-bind="$attrs"
      />

      <div v-if="showCharCount && maxlength" class="ui-textarea__counter">
        {{ charCount }} / {{ maxlength }}
      </div>
    </div>

    <div v-if="hint || error" class="ui-textarea__hint">
      <span v-if="error" class="ui-textarea__error">{{ error }}</span>
      <span v-else-if="hint" class="ui-textarea__hint-text">{{ hint }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, nextTick } from 'vue'

interface Props {
  modelValue?: string
  label?: string
  placeholder?: string
  hint?: string
  error?: string
  disabled?: boolean
  readonly?: boolean
  required?: boolean
  maxlength?: number
  minlength?: number
  rows?: number
  cols?: number
  autocomplete?: string
  name?: string
  id?: string
  resize?: 'none' | 'vertical' | 'horizontal' | 'both'
  showCharCount?: boolean
  size?: 'sm' | 'md' | 'lg'
  autoResize?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  readonly: false,
  required: false,
  rows: 4,
  resize: 'vertical',
  showCharCount: false,
  size: 'md',
  autoResize: false
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
  'focus': [event: FocusEvent]
  'blur': [event: FocusEvent]
}>()

const textareaRef = ref<HTMLTextAreaElement>()
const textareaId = props.id || `ui-textarea-${Math.random().toString(36).substr(2, 9)}`

const textareaValue = computed({
  get: () => props.modelValue || '',
  set: (value) => {
    emit('update:modelValue', value)
    if (props.autoResize) {
      nextTick(autoResizeTextarea)
    }
  }
})

const textareaClasses = computed(() => [
  'ui-textarea',
  `ui-textarea--${props.size}`,
  {
    'ui-textarea--error': props.error,
    'ui-textarea--disabled': props.disabled,
    'ui-textarea--readonly': props.readonly
  }
])

const wrapperClasses = computed(() => [
  'ui-textarea-wrapper',
  `ui-textarea-wrapper--${props.size}`
])

const charCount = computed(() => (props.modelValue || '').length)

function autoResizeTextarea() {
  const textarea = textareaRef.value
  if (!textarea) return

  textarea.style.height = 'auto'
  textarea.style.height = `${textarea.scrollHeight}px`
}

function focus() {
  textareaRef.value?.focus()
}

function blur() {
  textareaRef.value?.blur()
}

defineExpose({
  focus,
  blur
})
</script>

<style scoped>
.ui-textarea-wrapper {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.ui-textarea__label {
  font-size: var(--text-sm);
  font-weight: 500;
  color: var(--text-secondary);
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.ui-textarea__required {
  color: var(--danger);
}

.ui-textarea__container {
  position: relative;
  display: flex;
  align-items: flex-end;
}

.ui-textarea {
  width: 100%;
  min-height: 80px;
  font-family: var(--font-body);
  font-size: var(--text-base);
  line-height: 1.6;
  color: var(--text-primary);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  resize: vertical;
  transition: all var(--duration-fast);
}

.ui-textarea::placeholder {
  color: var(--text-muted);
}

.ui-textarea:hover:not(.ui-textarea--disabled):not(.ui-textarea--readonly) {
  border-color: var(--border-strong);
}

.ui-textarea:focus {
  outline: none;
  border-color: var(--primary);
}

.ui-textarea--error {
  border-color: var(--danger);
}

.ui-textarea--error:focus {
  border-color: var(--danger);
}

.ui-textarea--disabled,
.ui-textarea--readonly {
  background: var(--bg-secondary);
  color: var(--text-muted);
  cursor: not-allowed;
}

.ui-textarea--disabled {
  resize: none;
}

/* Resize controls */
.ui-textarea[style*='resize: none'] {
  resize: none;
}

.ui-textarea[style*='resize: horizontal'] {
  resize: horizontal;
}

.ui-textarea[style*='resize: vertical'] {
  resize: vertical;
}

.ui-textarea[style*='resize: both'] {
  resize: both;
}

/* Sizes */
.ui-textarea--sm {
  padding: var(--space-2) var(--space-3);
  font-size: var(--text-sm);
  min-height: 60px;
}

.ui-textarea--lg {
  padding: var(--space-4);
  font-size: var(--text-lg);
  min-height: 120px;
}

/* Character counter */
.ui-textarea__counter {
  position: absolute;
  bottom: var(--space-3);
  right: var(--space-3);
  font-size: var(--text-xs);
  color: var(--text-muted);
  background: var(--bg-tertiary);
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  pointer-events: none;
}

.ui-textarea--lg + .ui-textarea__counter {
  bottom: var(--space-4);
  right: var(--space-4);
}

.ui-textarea--sm + .ui-textarea__counter {
  bottom: var(--space-2);
  right: var(--space-3);
}

/* Hint */
.ui-textarea__hint {
  display: flex;
  align-items: center;
  min-height: 20px;
}

.ui-textarea__hint-text {
  font-size: var(--text-sm);
  color: var(--text-muted);
}

.ui-textarea__error {
  font-size: var(--text-sm);
  color: var(--danger);
}

/* Scrollbar styling for textarea */
.ui-textarea::-webkit-scrollbar {
  width: 6px;
}

.ui-textarea::-webkit-scrollbar-track {
  background: transparent;
  border-radius: var(--radius-sm);
}

.ui-textarea::-webkit-scrollbar-thumb {
  background: var(--border-default);
  border-radius: var(--radius-sm);
}

.ui-textarea::-webkit-scrollbar-thumb:hover {
  background: var(--border-strong);
}
</style>
