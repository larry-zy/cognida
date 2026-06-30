<template>
  <div class="base-input" :class="classes">
    <!-- 前置标签 -->
    <div v-if="prefix || $slots.prefix" class="input-prefix">
      <slot name="prefix">
        <span class="prefix-text">{{ prefix }}</span>
      </slot>
    </div>

    <!-- 输入框容器 -->
    <div class="input-wrapper">
      <!-- 图标前缀 -->
      <div v-if="iconPrefix" class="input-icon icon-prefix">
        <component :is="iconPrefix" v-if="typeof iconPrefix === 'object'" />
        <span v-else class="icon-string">{{ iconPrefix }}</span>
      </div>

      <!-- 终端风格提示符 -->
      <span v-if="prompt" class="input-prompt">❯</span>

      <!-- 输入框/文本域 -->
      <component
        :is="textarea ? 'textarea' : 'input'"
        ref="inputRef"
        class="input-field"
        :class="{
          'has-icon-prefix': iconPrefix,
          'has-icon-suffix': iconSuffix || showClear || showPasswordToggle,
          'has-prompt': prompt
        }"
        :type="computedType"
        :value="modelValue"
        :placeholder="placeholder"
        :disabled="disabled"
        :readonly="readonly"
        :maxlength="maxlength"
        :rows="rows"
        :autocomplete="autocomplete"
        @input="handleInput"
        @change="handleChange"
        @focus="handleFocus"
        @blur="handleBlur"
        @keydown="handleKeydown"
      />

      <!-- 后缀图标 -->
      <div v-if="iconSuffix || showClear || showPasswordToggle || loading" class="input-icon icon-suffix">
        <!-- 清除按钮 -->
        <button
          v-if="showClear && !disabled && !readonly"
          class="icon-button clear-button"
          @click="handleClear"
          tabindex="-1"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10"/>
            <path d="M15 9l-6 6m0-6l6 6"/>
          </svg>
        </button>

        <!-- 密码显示切换 -->
        <button
          v-if="showPasswordToggle"
          class="icon-button password-toggle"
          @click="togglePasswordVisibility"
          tabindex="-1"
        >
          <svg v-if="showPassword" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
            <circle cx="12" cy="12" r="3"/>
          </svg>
          <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"/>
            <path d="M1 1l22 22"/>
          </svg>
        </button>

        <!-- 加载状态 -->
        <span v-if="loading" class="input-spinner">
          <svg class="spinner" viewBox="0 0 24 24">
            <circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="3" stroke-dasharray="32" stroke-dashoffset="32" />
          </svg>
        </span>

        <!-- 自定义后缀图标 -->
        <component :is="iconSuffix" v-if="iconSuffix && !loading" />
        <span v-else-if="iconSuffix && !loading" class="icon-string">{{ iconSuffix }}</span>
      </div>

      <!-- 聚焦光晕 -->
      <div v-if="isFocused" class="input-glow"></div>
    </div>

    <!-- 后置标签 -->
    <div v-if="suffix || $slots.suffix" class="input-suffix">
      <slot name="suffix">
        <span class="suffix-text">{{ suffix }}</span>
      </slot>
    </div>

    <!-- 字符计数 -->
    <div v-if="showCount && maxlength" class="input-count">
      {{ String(modelValue || '').length }} / {{ maxlength }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, nextTick, type Component } from 'vue'

interface Props {
  modelValue: string | number
  type?: 'text' | 'password' | 'email' | 'number' | 'tel' | 'url' | 'search'
  placeholder?: string
  size?: 'sm' | 'md' | 'lg'
  variant?: 'default' | 'filled' | 'underlined'
  prefix?: string
  suffix?: string
  iconPrefix?: string | Component
  iconSuffix?: string | Component
  disabled?: boolean
  readonly?: boolean
  maxlength?: number
  rows?: number
  textarea?: boolean
  clearable?: boolean
  showCount?: boolean
  loading?: boolean
  autocomplete?: string
  error?: boolean
  success?: boolean
  prompt?: boolean // 终端风格提示符
}

const props = withDefaults(defineProps<Props>(), {
  type: 'text',
  size: 'md',
  variant: 'default',
  rows: 3,
  clearable: false,
  showCount: false,
  loading: false,
  autocomplete: 'off',
  prompt: false
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
  'change': [value: string]
  'focus': [event: FocusEvent]
  'blur': [event: FocusEvent]
  'clear': []
  'keydown': [event: KeyboardEvent]
}>()

const inputRef = ref<HTMLInputElement | HTMLTextAreaElement>()
const isFocused = ref(false)
const showPassword = ref(false)

const classes = computed(() => [
  `size-${props.size}`,
  `variant-${props.variant}`,
  {
    'is-focused': isFocused.value,
    'is-disabled': props.disabled,
    'is-readonly': props.readonly,
    'has-error': props.error,
    'has-success': props.success,
    'is-textarea': props.textarea
  }
])

const computedType = computed(() => {
  if (props.type === 'password') {
    return showPassword.value ? 'text' : 'password'
  }
  return props.type
})

const showClear = computed(() =>
  props.clearable &&
  props.modelValue &&
  !props.disabled &&
  !props.readonly
)

const showPasswordToggle = computed(() => props.type === 'password')

function handleInput(event: Event) {
  const target = event.target as HTMLInputElement
  emit('update:modelValue', target.value)
}

function handleChange(event: Event) {
  const target = event.target as HTMLInputElement
  emit('change', target.value)
}

function handleFocus(event: FocusEvent) {
  isFocused.value = true
  emit('focus', event)
}

function handleBlur(event: FocusEvent) {
  isFocused.value = false
  emit('blur', event)
}

function handleKeydown(event: KeyboardEvent) {
  emit('keydown', event)
}

function handleClear() {
  emit('update:modelValue', '')
  emit('clear')
  focus()
}

function togglePasswordVisibility() {
  showPassword.value = !showPassword.value
}

function focus() {
  nextTick(() => {
    inputRef.value?.focus()
  })
}

defineExpose({
  focus,
  inputRef
})
</script>

<style scoped>
/* ==================== 基础样式 ==================== */
.base-input {
  position: relative;
  display: flex;
  align-items: center;
  gap: 8px;
}

.input-wrapper {
  position: relative;
  flex: 1;
  display: flex;
  align-items: center;
}

.input-field {
  width: 100%;
  font-family: inherit;
  font-size: 1rem;
  line-height: 1.5;
  color: #f0f0f5;
  background: rgba(31, 31, 35, 0.8);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  padding: 0 16px;
  transition: all 150ms ease;
  outline: none;
}

.input-field::placeholder {
  color: #606070;
}

/* ==================== 尺寸变体 ==================== */
.size-sm .input-field {
  height: 32px;
  font-size: 0.875rem;
  padding: 0 12px;
}

.size-md .input-field {
  height: 40px;
}

.size-lg .input-field {
  height: 48px;
  font-size: 1.125rem;
  padding: 0 20px;
}

/* 文本域 */
.is-textarea .input-field {
  height: auto;
  min-height: 80px;
  padding: 12px 16px;
  resize: vertical;
}

/* ==================== 变体样式 ==================== */

/* Default - 默认样式 */
.variant-default .input-field {
  background: rgba(31, 31, 35, 0.8);
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.variant-default .input-field:hover:not(:disabled) {
  border-color: rgba(255, 255, 255, 0.15);
}

.variant-default.is-focused .input-field {
  border-color: #22d3ee;
  box-shadow: 0 0 0 3px rgba(34, 211, 238, 0.1);
}

/* Filled - 填充样式 */
.variant-filled .input-field {
  background: rgba(39, 39, 43, 0.6);
  border: 1px solid transparent;
}

.variant-filled .input-field:hover:not(:disabled) {
  background: rgba(39, 39, 43, 0.8);
}

.variant-filled.is-focused .input-field {
  background: rgba(31, 31, 35, 0.8);
  border-color: #22d3ee;
  box-shadow: 0 0 0 3px rgba(34, 211, 238, 0.1);
}

/* Underlined - 下划线样式 */
.variant-underlined {
  background: transparent;
  border-radius: 0;
}

.variant-underlined .input-field {
  background: transparent;
  border: none;
  border-bottom: 2px solid rgba(255, 255, 255, 0.1);
  border-radius: 0;
  padding-left: 0;
  padding-right: 0;
}

.variant-underlined.is-focused .input-field {
  border-bottom-color: #22d3ee;
  box-shadow: 0 1px 0 0 #22d3ee;
}

/* ==================== 状态样式 ==================== */
.is-disabled .input-field {
  opacity: 0.5;
  cursor: not-allowed;
}

.is-readonly .input-field {
  background: transparent;
  cursor: default;
}

.has-error .input-field {
  border-color: #ef4444;
}

.has-error.is-focused .input-field {
  box-shadow: 0 0 0 3px rgba(239, 68, 68, 0.15);
}

.has-success .input-field {
  border-color: #10b981;
}

.has-success.is-focused .input-field {
  box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.15);
}

/* ==================== 青色聚焦光晕 ==================== */
.input-glow {
  position: absolute;
  inset: -3px;
  border-radius: inherit;
  pointer-events: none;
  background: radial-gradient(ellipse at center, rgba(34, 211, 238, 0.15), transparent 70%);
  opacity: 0;
  animation: pulseGlow 2s ease-in-out infinite;
}

@keyframes pulseGlow {
  0%, 100% { opacity: 0.5; }
  50% { opacity: 1; }
}

/* ==================== 终端风格提示符 ==================== */
.input-prompt {
  position: absolute;
  left: 12px;
  color: #22d3ee;
  font-family: 'JetBrains Mono', monospace;
  font-size: 1rem;
  pointer-events: none;
  z-index: 1;
}

.input-field.has-prompt {
  padding-left: 28px;
}

/* ==================== 图标 ==================== */
.input-icon {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  display: flex;
  align-items: center;
  color: #606070;
  pointer-events: none;
}

.input-icon svg {
  width: 18px;
  height: 18px;
}

.icon-string {
  font-size: 16px;
}

.icon-prefix {
  left: 12px;
}

.icon-suffix {
  right: 12px;
  pointer-events: auto;
  gap: 4px;
}

.input-field.has-icon-prefix {
  padding-left: 40px;
}

.input-field.has-icon-suffix {
  padding-right: 40px;
}

.size-sm .input-field.has-icon-prefix,
.size-sm .input-field.has-icon-suffix {
  padding-left: 32px;
  padding-right: 32px;
}

.size-lg .input-field.has-icon-prefix,
.size-lg .input-field.has-icon-suffix {
  padding-left: 48px;
  padding-right: 48px;
}

/* 图标按钮 */
.icon-button {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: 6px;
  color: #606070;
  cursor: pointer;
  transition: all 150ms ease;
}

.icon-button:hover {
  color: #f0f0f5;
  background: rgba(255, 255, 255, 0.1);
}

/* 加载动画 */
.input-spinner {
  display: flex;
  align-items: center;
}

.input-spinner .spinner {
  width: 16px;
  height: 16px;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* ==================== 前后缀 ==================== */
.input-prefix,
.input-suffix {
  display: flex;
  align-items: center;
  color: #a0a0b0;
  font-size: 0.875rem;
  white-space: nowrap;
}

.prefix-text,
.suffix-text {
  padding: 0 8px;
}

/* ==================== 字符计数 ==================== */
.input-count {
  position: absolute;
  right: 16px;
  bottom: -20px;
  font-size: 0.75rem;
  color: #404050;
  pointer-events: none;
}

/* ==================== 响应式 ==================== */
@media (max-width: 640px) {
  .input-field {
    font-size: 16px; /* 防止iOS自动缩放 */
  }
}
</style>
