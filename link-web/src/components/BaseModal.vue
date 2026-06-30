<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="modelValue" class="modal-overlay" :class="{ 'has-backdrop': showBackdrop }" @click="handleOverlayClick">
        <Transition name="modal-content">
          <div
            v-if="modelValue"
            class="modal-container"
            :class="[
              `size-${size}`,
              { 'is-fullscreen': fullscreen }
            ]"
            @click.stop
          >
            <!-- 头部 -->
            <div v-if="!hideHeader" class="modal-header">
              <div class="header-left">
                <div v-if="icon" class="modal-icon">
                  <component :is="icon" v-if="typeof icon === 'object'" />
                  <span v-else>{{ icon }}</span>
                </div>
                <div class="header-text">
                  <h3 class="modal-title">{{ title }}</h3>
                  <p v-if="subtitle" class="modal-subtitle">{{ subtitle }}</p>
                </div>
              </div>
              <div class="header-actions">
                <slot name="actions" />
                <button v-if="closable" class="close-button" @click="close" :title="closeText">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M18 6L6 18M6 6l12 12"/>
                  </svg>
                </button>
              </div>
            </div>

            <!-- 内容 -->
            <div class="modal-body" :class="{ 'no-padding': noPadding }">
              <slot />
            </div>

            <!-- 底部 -->
            <div v-if="!hideFooter && ($slots.footer || showDefaultFooter)" class="modal-footer">
              <slot name="footer">
                <BaseButton variant="ghost" @click="handleCancel">{{ cancelText }}</BaseButton>
                <BaseButton variant="primary" :loading="loading" @click="handleConfirm">{{ confirmText }}</BaseButton>
              </slot>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { watch, type Component } from 'vue'
import BaseButton from './BaseButton.vue'

interface Props {
  modelValue: boolean
  title?: string
  subtitle?: string
  icon?: string | Component
  size?: 'sm' | 'md' | 'lg' | 'xl' | 'full'
  closable?: boolean
  closeOnClickModal?: boolean
  closeOnPressEscape?: boolean
  showBackdrop?: boolean
  hideHeader?: boolean
  hideFooter?: boolean
  showDefaultFooter?: boolean
  noPadding?: boolean
  fullscreen?: boolean
  loading?: boolean
  confirmText?: string
  cancelText?: string
  closeText?: string
  glow?: boolean
  destroyOnClose?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  title: '',
  size: 'md',
  closable: true,
  closeOnClickModal: true,
  closeOnPressEscape: true,
  showBackdrop: true,
  hideHeader: false,
  hideFooter: false,
  showDefaultFooter: true,
  noPadding: false,
  fullscreen: false,
  loading: false,
  confirmText: '确定',
  cancelText: '取消',
  closeText: '关闭',
  glow: false,
  destroyOnClose: false
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  'open': []
  'close': []
  'confirm': []
  'cancel': []
}>()

function close() {
  emit('update:modelValue', false)
  emit('close')
}

function handleConfirm() {
  emit('confirm')
}

function handleCancel() {
  emit('cancel')
  close()
}

function handleOverlayClick() {
  if (props.closeOnClickModal) {
    close()
  }
}

// 键盘事件处理
watch(() => props.modelValue, (isOpen) => {
  if (isOpen) {
    emit('open')
    document.addEventListener('keydown', handleKeydown)
  } else {
    document.removeEventListener('keydown', handleKeydown)
  }
})

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && props.closeOnPressEscape) {
    close()
  }
}
</script>

<style scoped>
/* ==================== 遮罩层 ==================== */
.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 1040;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
}

.modal-overlay.has-backdrop {
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

/* ==================== 容器 ==================== */
.modal-container {
  position: relative;
  width: 100%;
  max-height: 90vh;
  background: rgba(24, 24, 27, 0.95);
  backdrop-filter: blur(24px);
  -webkit-backdrop-filter: blur(24px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 18px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.6);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* 尺寸变体 */
.size-sm {
  max-width: 400px;
}

.size-md {
  max-width: 540px;
}

.size-lg {
  max-width: 720px;
}

.size-xl {
  max-width: 900px;
}

.size-full {
  max-width: 100%;
  height: 100%;
  max-height: 100%;
  border-radius: 0;
}

.is-fullscreen {
  width: 100vw;
  height: 100vh;
  max-width: 100vw;
  max-height: 100vh;
  border-radius: 0;
}

/* ==================== 头部 ==================== */
.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: 1;
  min-width: 0;
}

.modal-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  background: linear-gradient(135deg, #22d3ee 0%, #14b8a6 100%);
  border-radius: 10px;
  color: #0f0f12;
  font-size: 20px;
  flex-shrink: 0;
}

.modal-icon svg {
  width: 22px;
  height: 22px;
}

.header-text {
  flex: 1;
  min-width: 0;
}

.modal-title {
  font-size: 1.25rem;
  font-weight: 600;
  color: #f0f0f5;
  margin: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.modal-subtitle {
  font-size: 0.875rem;
  color: #606070;
  margin: 4px 0 0 0;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.close-button {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  background: transparent;
  border: none;
  border-radius: 10px;
  color: #606070;
  cursor: pointer;
  transition: all 150ms ease;
}

.close-button:hover {
  background: rgba(255, 255, 255, 0.05);
  color: #f0f0f5;
}

.close-button svg {
  width: 20px;
  height: 20px;
}

/* ==================== 内容 ==================== */
.modal-body {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
  color: #a0a0b0;
}

.modal-body.no-padding {
  padding: 0;
}

/* ==================== 底部 ==================== */
.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 16px 24px;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(0, 0, 0, 0.15);
  flex-shrink: 0;
}

/* ==================== 过渡动画 ==================== */
.modal-enter-active,
.modal-leave-active {
  transition: opacity 200ms ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-active .modal-container,
.modal-leave-active .modal-container {
  transition: all 250ms cubic-bezier(0.34, 1.56, 0.64, 1);
}

.modal-enter-from .modal-container,
.modal-leave-to .modal-container {
  opacity: 0;
  transform: scale(0.95) translateY(20px);
}

/* 内容过渡 */
.modal-content-enter-active {
  transition: all 300ms cubic-bezier(0.34, 1.56, 0.64, 1);
}

.modal-content-leave-active {
  transition: all 200ms ease-out;
}

.modal-content-enter-from {
  opacity: 0;
  transform: scale(0.9) translateY(-30px);
}

.modal-content-leave-to {
  opacity: 0;
  transform: scale(0.95) translateY(10px);
}

/* ==================== 响应式 ==================== */
@media (max-width: 640px) {
  .modal-overlay {
    padding: 0;
  }

  .modal-container {
    max-width: 100vw;
    max-height: 100vh;
    border-radius: 0;
    height: 100%;
  }

  .modal-header {
    padding: 16px 20px;
  }

  .modal-body {
    padding: 16px;
  }

  .modal-footer {
    padding: 12px 16px;
    flex-direction: column-reverse;
    gap: 8px;
  }

  .modal-footer :deep(.base-button) {
    width: 100%;
  }
}
</style>
