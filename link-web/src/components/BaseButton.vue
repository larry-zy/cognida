<template>
  <component
    :is="tag"
    :type="tag === 'button' ? nativeType : undefined"
    :to="tag === 'router-link' ? to : undefined"
    :href="tag === 'a' ? to : undefined"
    :disabled="disabled || loading"
    class="base-button"
    :class="classes"
    :style="customStyle"
    @click="handleClick"
  >
    <span v-if="loading" class="button-loader">
      <svg class="spinner" viewBox="0 0 24 24">
        <circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="3" stroke-dasharray="32" stroke-dashoffset="32" />
      </svg>
    </span>

    <span v-if="icon && !loading" class="button-icon" :class="{ 'icon-only': !$slots.default }">
      <component :is="icon" v-if="typeof icon === 'object'" />
      <span v-else class="icon-string">{{ icon }}</span>
    </span>

    <span v-if="$slots.default" class="button-content">
      <slot />
    </span>

    <span v-if="badge" class="button-badge">{{ badge }}</span>
  </component>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'

interface Props {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger'
  size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl'
  tag?: 'button' | 'a' | 'router-link'
  nativeType?: 'button' | 'submit' | 'reset'
  icon?: string | Component
  badge?: string | number
  disabled?: boolean
  loading?: boolean
  block?: boolean
  rounded?: boolean
  to?: string | object
  glow?: boolean
  shadow?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'primary',
  size: 'md',
  tag: 'button',
  nativeType: 'button',
  disabled: false,
  loading: false,
  block: false,
  rounded: false,
  glow: false,
  shadow: false
})

const emit = defineEmits<{
  click: [event: Event]
}>()

const classes = computed(() => [
  `variant-${props.variant}`,
  `size-${props.size}`,
  {
    'is-disabled': props.disabled,
    'is-loading': props.loading,
    'is-block': props.block,
    'is-rounded': props.rounded,
    'has-glow': props.glow,
    'has-shadow': props.shadow
  }
])

const customStyle = computed(() => {
  if (props.glow && !props.disabled) {
    return {
      '--button-glow-color': 'var(--color-primary, #22d3ee)'
    }
  }
  return {}
})

function handleClick(event: Event) {
  if (!props.disabled && !props.loading) {
    emit('click', event)
  }
}
</script>

<style scoped>
/* ==================== 基础样式 ==================== */
.base-button {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  font-weight: 500;
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
  user-select: none;
  transition: all 150ms ease;
  border: none;
  outline: none;
  text-decoration: none;
}

.base-button:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

/* ==================== 尺寸变体 ==================== */
.size-xs {
  padding: 4px 10px;
  font-size: 0.75rem;
  border-radius: 6px;
  gap: 4px;
}

.size-sm {
  padding: 6px 14px;
  font-size: 0.875rem;
  border-radius: 6px;
  gap: 6px;
}

.size-md {
  padding: 10px 18px;
  font-size: 1rem;
  border-radius: 10px;
  gap: 8px;
}

.size-lg {
  padding: 14px 24px;
  font-size: 1.125rem;
  border-radius: 14px;
  gap: 12px;
}

.size-xl {
  padding: 18px 32px;
  font-size: 1.25rem;
  border-radius: 18px;
  gap: 16px;
}

/* ==================== 变体样式 ==================== */

/* Primary - 主按钮 (青色渐变) */
.variant-primary {
  background: linear-gradient(135deg, #22d3ee 0%, #14b8a6 100%);
  color: #0f0f12;
  box-shadow: 0 4px 12px rgba(34, 211, 238, 0.25);
}

.variant-primary:hover:not(:disabled) {
  background: linear-gradient(135deg, #67e8f9 0%, #22d3ee 100%);
  box-shadow: 0 6px 20px rgba(34, 211, 238, 0.35);
  transform: translateY(-1px);
}

.variant-primary:active:not(:disabled) {
  transform: translateY(0);
  box-shadow: 0 2px 8px rgba(34, 211, 238, 0.2);
}

/* Secondary - 次要按钮 (青色边框) */
.variant-secondary {
  background: transparent;
  color: #22d3ee;
  border: 1px solid #22d3ee;
}

.variant-secondary:hover:not(:disabled) {
  background: rgba(34, 211, 238, 0.1);
  border-color: #67e8f9;
  box-shadow: 0 0 0 3px rgba(34, 211, 238, 0.1);
}

.variant-secondary:active:not(:disabled) {
  transform: scale(0.98);
}

/* Ghost - 幽灵按钮 */
.variant-ghost {
  background: transparent;
  color: #a0a0b0;
}

.variant-ghost:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.05);
  color: #f0f0f5;
}

/* Danger - 危险按钮 */
.variant-danger {
  background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%);
  color: white;
  box-shadow: 0 4px 12px rgba(239, 68, 68, 0.25);
}

.variant-danger:hover:not(:disabled) {
  background: linear-gradient(135deg, #f87171 0%, #ef4444 100%);
  box-shadow: 0 6px 20px rgba(239, 68, 68, 0.35);
  transform: translateY(-1px);
}

.variant-danger:active:not(:disabled) {
  transform: translateY(0);
  box-shadow: 0 2px 8px rgba(239, 68, 68, 0.2);
}

/* ==================== 修饰类 ==================== */
.is-block {
  width: 100%;
}

.is-rounded {
  border-radius: 9999px;
}

.has-glow:not(:disabled) {
  animation: buttonGlow 2s ease-in-out infinite;
}

@keyframes buttonGlow {
  0%, 100% {
    box-shadow: 0 0 5px var(--button-glow-color, #22d3ee),
                0 0 10px var(--button-glow-color, #22d3ee);
  }
  50% {
    box-shadow: 0 0 15px var(--button-glow-color, #22d3ee),
                0 0 25px var(--button-glow-color, #22d3ee),
                0 0 35px var(--button-glow-color, #22d3ee);
  }
}

.has-shadow {
  box-shadow: 0 10px 15px rgba(0, 0, 0, 0.5);
}

/* ==================== 图标 ==================== */
.button-icon {
  display: flex;
  align-items: center;
  justify-content: center;
}

.button-icon svg {
  width: 1em;
  height: 1em;
}

.icon-string {
  font-size: 1.2em;
}

.icon-only {
  margin: 0;
}

/* ==================== 加载状态 ==================== */
.button-loader {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.spinner {
  width: 1em;
  height: 1em;
  animation: spin 0.8s linear infinite;
}

.spinner circle {
  animation: dash 1.5s ease-in-out infinite;
}

@keyframes dash {
  0% {
    stroke-dasharray: 1, 150;
    stroke-dashoffset: 0;
  }
  50% {
    stroke-dasharray: 90, 150;
    stroke-dashoffset: -35;
  }
  100% {
    stroke-dasharray: 90, 150;
    stroke-dashoffset: -124;
  }
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* ==================== 徽章 ==================== */
.button-badge {
  position: absolute;
  top: -6px;
  right: -6px;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  background: #ef4444;
  color: white;
  font-size: 10px;
  font-weight: 600;
  line-height: 18px;
  text-align: center;
  border-radius: 9999px;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.3);
}

/* ==================== 内容 ==================== */
.button-content {
  display: inline-flex;
  align-items: center;
}
</style>
