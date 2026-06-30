<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="show" class="confirm-overlay" @click="handleCancel">
        <div class="confirm-dialog" @click.stop>
          <div class="confirm-header">
            <div class="confirm-icon" :class="`confirm-icon--${type}`">
              <svg v-if="type === 'warning'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
                <line x1="12" y1="9" x2="12" y2="13"/>
                <line x1="12" y1="17" x2="12.01" y2="17"/>
              </svg>
              <svg v-else-if="type === 'danger'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10"/>
                <line x1="15" y1="9" x2="9" y2="15"/>
                <line x1="9" y1="9" x2="15" y2="15"/>
              </svg>
              <svg v-else-if="type === 'success'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/>
                <polyline points="22 4 12 14.01 9 11.01"/>
              </svg>
              <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10"/>
                <line x1="12" y1="16" x2="12" y2="12"/>
                <line x1="12" y1="8" x2="12.01" y2="8"/>
              </svg>
            </div>
          </div>

          <div class="confirm-body">
            <h3 v-if="title" class="confirm-title">{{ title }}</h3>
            <p class="confirm-message">{{ message }}</p>
          </div>

          <div class="confirm-footer">
            <button
              v-if="showCancelButton"
              class="confirm-btn confirm-btn--cancel"
              @click="handleCancel"
            >
              {{ cancelButtonText }}
            </button>
            <button
              class="confirm-btn"
              :class="`confirm-btn--${confirmButtonType}`"
              @click="handleConfirm"
            >
              {{ confirmButtonText }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'

type ConfirmType = 'warning' | 'danger' | 'success' | 'info'

interface Props {
  show: boolean
  title?: string
  message: string
  type?: ConfirmType
  confirmButtonText?: string
  cancelButtonText?: string
  showCancelButton?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  type: 'warning',
  confirmButtonText: '确定',
  cancelButtonText: '取消',
  showCancelButton: true
})

const emit = defineEmits<{
  'update:show': [value: boolean]
  confirm: []
  cancel: []
}>()

const confirmButtonType = computed(() => {
  if (props.type === 'danger') return 'danger'
  if (props.type === 'success') return 'primary'
  return 'default'
})

function handleConfirm() {
  emit('confirm')
  emit('update:show', false)
}

function handleCancel() {
  emit('cancel')
  emit('update:show', false)
}

// ESC 键关闭
watch(() => props.show, (show) => {
  if (show) {
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        handleCancel()
      }
    }
    document.addEventListener('keydown', handleEsc)
    return () => document.removeEventListener('keydown', handleEsc)
  }
})
</script>

<style scoped>
.confirm-overlay {
  position: fixed;
  inset: 0;
  z-index: 10000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
}

.confirm-dialog {
  width: 100%;
  max-width: 420px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-xl);
  box-shadow: var(--shadow-2xl);
  overflow: hidden;
}

.confirm-header {
  display: flex;
  justify-content: center;
  padding: 24px 24px 16px;
}

.confirm-icon {
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
}

.confirm-icon svg {
  width: 28px;
  height: 28px;
}

.confirm-icon--warning {
  background: rgba(251, 191, 36, 0.15);
  color: var(--color-warning);
}

.confirm-icon--danger {
  background: rgba(239, 68, 68, 0.15);
  color: var(--color-danger);
}

.confirm-icon--success {
  background: rgba(34, 197, 94, 0.15);
  color: var(--color-success);
}

.confirm-icon--info {
  background: rgba(34, 211, 238, 0.15);
  color: var(--primary);
}

.confirm-body {
  padding: 16px 24px 24px;
  text-align: center;
}

.confirm-title {
  font: var(--font-display);
  font-size: var(--text-lg);
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 12px 0;
}

.confirm-message {
  font-size: var(--text-sm);
  color: var(--text-secondary);
  line-height: 1.6;
  margin: 0;
  white-space: pre-wrap;
}

.confirm-footer {
  display: flex;
  gap: 12px;
  padding: 16px 24px;
  background: var(--bg-secondary);
  border-top: 1px solid var(--border-subtle);
}

.confirm-btn {
  flex: 1;
  padding: 10px 20px;
  font-size: var(--text-sm);
  font-weight: 500;
  border-radius: var(--radius-md);
  border: none;
  cursor: pointer;
  transition: all var(--duration-fast);
}

.confirm-btn--cancel {
  background: transparent;
  color: var(--text-secondary);
  border: 1px solid var(--border-subtle);
}

.confirm-btn--cancel:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.confirm-btn--primary {
  background: var(--primary);
  color: var(--bg-primary);
}

.confirm-btn--primary:hover {
  background: var(--primary-dark);
}

.confirm-btn--danger {
  background: var(--color-danger);
  color: white;
}

.confirm-btn--danger:hover {
  background: #dc2626;
}

.confirm-btn--default {
  background: var(--bg-tertiary);
  color: var(--text-primary);
}

.confirm-btn--default:hover {
  background: var(--bg-hover);
}

/* 动画 */
.modal-enter-active,
.modal-leave-active {
  transition: all var(--duration-base);
}

.modal-enter-active .confirm-dialog,
.modal-leave-active .confirm-dialog {
  transition: all var(--duration-base);
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-from .confirm-dialog,
.modal-leave-to .confirm-dialog {
  transform: scale(0.9);
  opacity: 0;
}

/* 响应式 */
@media (max-width: 480px) {
  .confirm-overlay {
    padding: 16px;
  }

  .confirm-footer {
    flex-direction: column;
  }

  .confirm-btn {
    width: 100%;
  }
}
</style>
