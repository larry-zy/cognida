<template>
  <Teleport to="body">
    <Transition name="ui-drawer">
      <div
        v-if="modelValue"
        class="ui-drawer"
        @click="handleBackdropClick"
      >
        <!-- Backdrop -->
        <div class="ui-drawer__backdrop"></div>

        <!-- Drawer -->
        <div
          :class="drawerClasses"
          :style="drawerStyle"
          @click.stop
        >
          <!-- Header -->
          <div v-if="$slots.header || title || closable" class="ui-drawer__header">
            <slot name="header">
              <h3 class="ui-drawer__title">{{ title }}</h3>
            </slot>
            <button
              v-if="closable"
              type="button"
              class="ui-drawer__close"
              @click="handleClose"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M18 6L6 18M6 6l12 12" />
              </svg>
            </button>
          </div>

          <!-- Body -->
          <div :class="bodyClasses">
            <slot />
          </div>

          <!-- Footer -->
          <div v-if="$slots.footer" class="ui-drawer__footer">
            <slot name="footer" />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, watch, onMounted, onBeforeUnmount, useSlots } from 'vue'

interface Props {
  modelValue?: boolean
  title?: string
  position?: 'left' | 'right' | 'top' | 'bottom'
  size?: number | string
  closable?: boolean
  maskClosable?: boolean
  closeOnEsc?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  position: 'right',
  size: 400,
  closable: true,
  maskClosable: true,
  closeOnEsc: true
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  'open': []
  'close': []
}>()

const drawerClasses = computed(() => [
  'ui-drawer__panel',
  `ui-drawer__panel--${props.position}`
])

const drawerStyle = computed(() => {
  const size = typeof props.size === 'number' ? `${props.size}px` : props.size

  const styles: Record<string, string> = {
    zIndex: '1050'
  }

  if (props.position === 'left' || props.position === 'right') {
    styles.width = size
    styles.height = '100vh'
  } else {
    styles.width = '100vw'
    styles.height = size
  }

  return styles
})

const slots = useSlots()

const bodyClasses = computed(() => [
  'ui-drawer__body',
  { 'ui-drawer__body--no-header': !slots.header && !props.title && !props.closable },
  { 'ui-drawer__body--no-footer': !slots.footer }
])

function handleClose() {
  emit('update:modelValue', false)
  emit('close')
}

function handleBackdropClick() {
  if (props.maskClosable) {
    handleClose()
  }
}

function handleKeydown(event: KeyboardEvent) {
  if (props.closeOnEsc && event.key === 'Escape') {
    handleClose()
  }
}

watch(() => props.modelValue, (value) => {
  if (value) {
    emit('open')
    document.body.style.overflow = 'hidden'
  } else {
    document.body.style.overflow = ''
  }
})

onMounted(() => {
  document.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleKeydown)
  document.body.style.overflow = ''
})
</script>

<style scoped>
.ui-drawer {
  position: fixed;
  inset: 0;
  z-index: 1050;
}

/* Backdrop */
.ui-drawer__backdrop {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

/* Panel */
.ui-drawer__panel {
  position: absolute;
  display: flex;
  flex-direction: column;
  background: var(--bg-secondary);
  box-shadow: var(--shadow-xl);
}

.ui-drawer__panel--left {
  top: 0;
  left: 0;
  border-right: 1px solid var(--border-default);
}

.ui-drawer__panel--right {
  top: 0;
  right: 0;
  border-left: 1px solid var(--border-default);
}

.ui-drawer__panel--top {
  top: 0;
  left: 0;
  border-bottom: 1px solid var(--border-default);
}

.ui-drawer__panel--bottom {
  bottom: 0;
  left: 0;
  border-top: 1px solid var(--border-default);
}

/* Header */
.ui-drawer__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  padding: var(--space-6);
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

.ui-drawer__title {
  margin: 0;
  font: var(--font-display);
  font-size: var(--text-xl);
  font-weight: 600;
  color: var(--text-primary);
}

.ui-drawer__close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  cursor: pointer;
  transition: all var(--duration-fast);
  flex-shrink: 0;
}

.ui-drawer__close:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.ui-drawer__close svg {
  width: 20px;
  height: 20px;
}

/* Body */
.ui-drawer__body {
  flex: 1;
  padding: var(--space-6);
  overflow-y: auto;
  color: var(--text-secondary);
}

.ui-drawer__body--no-header {
  border-top-left-radius: var(--radius-lg);
  border-top-right-radius: var(--radius-lg);
}

.ui-drawer__body--no-footer {
  border-bottom-left-radius: var(--radius-lg);
  border-bottom-right-radius: var(--radius-lg);
}

/* Footer */
.ui-drawer__footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-3);
  padding: var(--space-6);
  border-top: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

/* Scrollbar */
.ui-drawer__body::-webkit-scrollbar {
  width: 6px;
}

.ui-drawer__body::-webkit-scrollbar-track {
  background: transparent;
}

.ui-drawer__body::-webkit-scrollbar-thumb {
  background: var(--border-default);
  border-radius: var(--radius-sm);
}

.ui-drawer__body::-webkit-scrollbar-thumb:hover {
  background: var(--border-strong);
}

/* Responsive */
@media (max-width: 767px) {
  .ui-drawer__panel--left,
  .ui-drawer__panel--right {
    width: 100vw !important;
    max-width: 100vw;
  }

  .ui-drawer__panel--top,
  .ui-drawer__panel--bottom {
    height: 50vh !important;
    max-height: 50vh;
  }

  .ui-drawer__header,
  .ui-drawer__body,
  .ui-drawer__footer {
    padding: var(--space-4);
  }
}

/* Transitions */
.ui-drawer-enter-active,
.ui-drawer-leave-active {
  transition: opacity var(--duration-base);
}

.ui-drawer-enter-active .ui-drawer__panel,
.ui-drawer-leave-active .ui-drawer__panel {
  transition: transform var(--duration-base) ease;
}

.ui-drawer-enter-from {
  opacity: 0;
}

.ui-drawer-enter-from .ui-drawer__panel--left {
  transform: translateX(-100%);
}

.ui-drawer-enter-from .ui-drawer__panel--right {
  transform: translateX(100%);
}

.ui-drawer-enter-from .ui-drawer__panel--top {
  transform: translateY(-100%);
}

.ui-drawer-enter-from .ui-drawer__panel--bottom {
  transform: translateY(100%);
}

.ui-drawer-leave-to {
  opacity: 0;
}

.ui-drawer-leave-to .ui-drawer__panel--left {
  transform: translateX(-100%);
}

.ui-drawer-leave-to .ui-drawer__panel--right {
  transform: translateX(100%);
}

.ui-drawer-leave-to .ui-drawer__panel--top {
  transform: translateY(-100%);
}

.ui-drawer-leave-to .ui-drawer__panel--bottom {
  transform: translateY(100%);
}

.ui-drawer-enter-to,
.ui-drawer-leave-from {
  opacity: 1;
}

.ui-drawer-enter-to .ui-drawer__panel,
.ui-drawer-leave-from .ui-drawer__panel {
  transform: translate(0);
}
</style>
