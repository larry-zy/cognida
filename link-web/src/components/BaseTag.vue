<template>
  <component
    :is="tag"
    class="base-tag"
    :class="classes"
  >
    <!-- 图标 -->
    <span v-if="icon" class="tag-icon">
      <component :is="icon" v-if="typeof icon === 'object'" />
      <span v-else class="icon-string">{{ icon }}</span>
    </span>

    <!-- 文字内容 -->
    <span v-if="$slots.default" class="tag-content">
      <slot />
    </span>

    <!-- 关闭按钮 -->
    <button
      v-if="closable"
      class="tag-close"
      @click="handleClose"
      type="button"
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M18 6L6 18M6 6l12 12"/>
      </svg>
    </button>

    <!-- 装饰点 -->
    <span v-if="dot" class="tag-dot"></span>
  </component>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'

interface Props {
  variant?: 'default' | 'success' | 'warning' | 'error' | 'info'
  size?: 'xs' | 'sm' | 'md' | 'lg'
  shape?: 'square' | 'rounded' | 'pill'
  icon?: string | Component
  closable?: boolean
  dot?: boolean
  tag?: 'span' | 'div' | 'label' | 'a'
  href?: string
  bordered?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'default',
  size: 'md',
  shape: 'rounded',
  tag: 'span',
  closable: false,
  dot: false,
  bordered: false
})

const emit = defineEmits<{
  close: []
}>()

const classes = computed(() => [
  `variant-${props.variant}`,
  `size-${props.size}`,
  `shape-${props.shape}`,
  {
    'is-closable': props.closable,
    'has-icon': props.icon,
    'has-dot': props.dot,
    'is-bordered': props.bordered
  }
])

function handleClose(event: Event) {
  event.stopPropagation()
  emit('close')
}
</script>

<style scoped>
/* ==================== 基础样式 ==================== */
.base-tag {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  font-weight: 500;
  line-height: 1;
  white-space: nowrap;
  transition: all 150ms ease;
  cursor: default;
}

/* ==================== 形状变体 ==================== */
.shape-square {
  border-radius: 6px;
}

.shape-rounded {
  border-radius: 10px;
}

.shape-pill {
  border-radius: 9999px;
}

/* ==================== 尺寸变体 ==================== */
.size-xs {
  padding: 2px 8px;
  font-size: 0.75rem;
  gap: 3px;
}

.size-sm {
  padding: 4px 10px;
  font-size: 0.875rem;
  gap: 4px;
}

.size-md {
  padding: 6px 12px;
  font-size: 0.875rem;
  gap: 4px;
}

.size-lg {
  padding: 8px 16px;
  font-size: 1rem;
  gap: 6px;
}

/* ==================== 变体样式 ==================== */

/* Default - 默认 */
.variant-default {
  background: rgba(255, 255, 255, 0.05);
  color: #a0a0b0;
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.variant-default:hover {
  background: rgba(255, 255, 255, 0.08);
  color: #f0f0f5;
}

/* Success - 成功 (绿色) */
.variant-success {
  background: rgba(16, 185, 129, 0.15);
  color: #34d399;
  border: 1px solid rgba(16, 185, 129, 0.3);
}

.variant-success:hover {
  background: rgba(16, 185, 129, 0.2);
  border-color: rgba(16, 185, 129, 0.4);
}

/* Warning - 警告 (橙色) */
.variant-warning {
  background: rgba(245, 158, 11, 0.15);
  color: #fbbf24;
  border: 1px solid rgba(245, 158, 11, 0.3);
}

.variant-warning:hover {
  background: rgba(245, 158, 11, 0.2);
  border-color: rgba(245, 158, 11, 0.4);
}

/* Error - 错误 (红色) */
.variant-error {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
  border: 1px solid rgba(239, 68, 68, 0.3);
}

.variant-error:hover {
  background: rgba(239, 68, 68, 0.2);
  border-color: rgba(239, 68, 68, 0.4);
}

/* Info - 信息 (青色) */
.variant-info {
  background: rgba(34, 211, 238, 0.15);
  color: #67e8f9;
  border: 1px solid rgba(34, 211, 238, 0.3);
}

.variant-info:hover {
  background: rgba(34, 211, 238, 0.2);
  border-color: rgba(34, 211, 238, 0.4);
}

/* ==================== 修饰样式 ==================== */
.is-bordered {
  border-width: 2px;
}

/* ==================== 图标 ==================== */
.tag-icon {
  display: flex;
  align-items: center;
  font-size: 1em;
}

.icon-string {
  font-size: 1.2em;
}

/* ==================== 关闭按钮 ==================== */
.tag-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: 6px;
  color: inherit;
  cursor: pointer;
  opacity: 0.7;
  transition: all 150ms ease;
}

.tag-close:hover {
  opacity: 1;
  background: rgba(0, 0, 0, 0.2);
}

.tag-close svg {
  width: 12px;
  height: 12px;
}

/* ==================== 装饰点 ==================== */
.tag-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  margin-right: -2px;
  background: currentColor;
}

/* ==================== 内容 ==================== */
.tag-content {
  display: inline-flex;
  align-items: center;
}

/* ==================== 响应式 ==================== */
@media (max-width: 640px) {
  .size-lg {
    padding: 6px 12px;
    font-size: 0.875rem;
  }
}
</style>
