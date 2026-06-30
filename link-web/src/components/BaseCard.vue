<template>
  <div class="base-card" :class="classes" @click="handleClick">
    <!-- 头部青色渐变装饰 -->
    <div v-if="showHeaderAccent" class="header-accent"></div>

    <!-- 头部 -->
    <div v-if="$slots.header || title || subtitle" class="card-header">
      <div class="header-content">
        <div v-if="icon" class="header-icon">
          <component :is="icon" v-if="typeof icon === 'object'" />
          <span v-else class="icon-string">{{ icon }}</span>
        </div>
        <div class="header-text">
          <h3 v-if="title" class="card-title">{{ title }}</h3>
          <p v-if="subtitle" class="card-subtitle">{{ subtitle }}</p>
        </div>
      </div>
      <div v-if="$slots.extra" class="header-extra">
        <slot name="extra" />
      </div>
    </div>

    <!-- 内容 -->
    <div class="card-body" :class="{ 'no-padding': noPadding }">
      <slot />
    </div>

    <!-- 底部 -->
    <div v-if="$slots.footer" class="card-footer">
      <slot name="footer" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, useSlots, type Component } from 'vue'

interface Props {
  variant?: 'default' | 'subtle' | 'bordered' | 'elevated'
  size?: 'sm' | 'md' | 'lg'
  title?: string
  subtitle?: string
  icon?: string | Component
  noPadding?: boolean
  hoverable?: boolean
  clickable?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'default',
  size: 'md',
  noPadding: false,
  hoverable: true,
  clickable: false
})

const emit = defineEmits<{
  click: [event: MouseEvent]
}>()

const classes = computed(() => [
  `variant-${props.variant}`,
  `size-${props.size}`,
  {
    'is-hoverable': props.hoverable,
    'is-clickable': props.clickable
  }
])

const slots = useSlots()
const showHeaderAccent = computed(() =>
  props.variant === 'default' && (props.title || slots.header)
)

function handleClick(event: MouseEvent) {
  if (props.clickable) {
    emit('click', event)
  }
}
</script>

<style scoped>
/* ==================== 基础样式 ==================== */
.base-card {
  position: relative;
  overflow: hidden;
  border-radius: 14px;
  transition: all 200ms ease;
}

.base-card.is-clickable {
  cursor: pointer;
}

/* ==================== 尺寸变体 ==================== */
.size-sm {
  border-radius: 10px;
}

.size-md {
  border-radius: 14px;
}

.size-lg {
  border-radius: 18px;
}

/* ==================== 变体样式 ==================== */

/* Default - 默认卡片（深色背景 + 青色头部装饰） */
.variant-default {
  background: rgba(24, 24, 27, 0.8);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.variant-default.is-hoverable:hover {
  border-color: rgba(34, 211, 238, 0.3);
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.4), 0 0 20px rgba(34, 211, 238, 0.1);
  transform: translateY(-2px);
}

.variant-default.is-clickable:active {
  transform: translateY(0) scale(0.98);
}

/* Subtle - 柔和卡片 */
.variant-subtle {
  background: rgba(15, 15, 18, 0.6);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  border: 1px solid rgba(255, 255, 255, 0.06);
}

.variant-subtle.is-hoverable:hover {
  background: rgba(24, 24, 27, 0.7);
  border-color: rgba(255, 255, 255, 0.1);
}

/* Bordered - 边框卡片 */
.variant-bordered {
  background: transparent;
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.variant-bordered.is-hoverable:hover {
  border-color: rgba(34, 211, 238, 0.4);
  background: rgba(34, 211, 238, 0.05);
}

/* Elevated - 浮起卡片 */
.variant-elevated {
  background: rgba(39, 39, 43, 0.9);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border: 1px solid rgba(255, 255, 255, 0.15);
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.5);
}

.variant-elevated.is-hoverable:hover {
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.6);
  transform: translateY(-4px);
}

/* ==================== 头部青色渐变装饰 ==================== */
.header-accent {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
  background: linear-gradient(90deg, #22d3ee 0%, #14b8a6 100%);
  z-index: 1;
}

/* ==================== 头部 ==================== */
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.header-content {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  background: linear-gradient(135deg, #22d3ee 0%, #14b8a6 100%);
  border-radius: 10px;
  color: #0f0f12;
  font-size: 18px;
}

.header-icon svg {
  width: 20px;
  height: 20px;
}

.icon-string {
  font-size: 20px;
}

.header-text {
  flex: 1;
}

.card-title {
  font-size: 1.125rem;
  font-weight: 600;
  color: #f0f0f5;
  margin: 0;
}

.card-subtitle {
  font-size: 0.875rem;
  color: #606070;
  margin: 2px 0 0 0;
}

.header-extra {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* ==================== 内容区域 ==================== */
.card-body {
  padding: 20px;
}

.card-body.no-padding {
  padding: 0;
}

/* ==================== 底部 ==================== */
.card-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(0, 0, 0, 0.15);
}

/* ==================== 响应式 ==================== */
@media (max-width: 640px) {
  .card-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 12px;
    padding: 16px;
  }

  .header-extra {
    width: 100%;
    justify-content: flex-end;
  }

  .card-body {
    padding: 16px;
  }

  .card-footer {
    padding: 12px 16px;
  }
}
</style>
