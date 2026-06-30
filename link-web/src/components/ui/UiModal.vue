<template>
  <Teleport to="body">
    <Transition name="ui-modal">
      <div
        v-if="modelValue"
        :class="modalClasses"
        @click="handleBackdropClick"
      >
        <!-- Backdrop -->
        <div class="ui-modal__backdrop"></div>

        <!-- Modal -->
        <div
          :class="containerClasses"
          :style="containerStyle"
          @click.stop
        >
          <!-- Header -->
          <div v-if="$slots.header || title || closable" class="ui-modal__header">
            <slot name="header">
              <h3 class="ui-modal__title">{{ title }}</h3>
            </slot>
            <button
              v-if="closable"
              type="button"
              class="ui-modal__close"
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
          <div v-if="$slots.footer" class="ui-modal__footer">
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
  size?: 'sm' | 'md' | 'lg' | 'xl' | 'full'
  closable?: boolean
  maskClosable?: boolean
  closeOnEsc?: boolean
  centered?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  closable: true,
  maskClosable: true,
  closeOnEsc: true,
  centered: false,
  size: 'md'
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  'open': []
  'close': []
}>()

const modalClasses = computed(() => [
  'ui-modal',
  `ui-modal--${props.size}`,
  { 'ui-modal--centered': props.centered }
])

const containerClasses = computed(() => [
  'ui-modal__container',
  `ui-modal__container--${props.size}`
])

const containerStyle = computed(() => ({
  zIndex: '1050'
}))

const slots = useSlots()

const bodyClasses = computed(() => [
  'ui-modal__body',
  { 'ui-modal__body--no-header': !slots.header && !props.title && !props.closable },
  { 'ui-modal__body--no-footer': !slots.footer }
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
.ui-modal {
  position: fixed;
  inset: 0;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: var(--space-8);
  overflow: auto;
  z-index: 1050;
}

.ui-modal--centered {
  align-items: center;
}

/* Backdrop */
.ui-modal__backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

/* Container */
.ui-modal__container {
  position: relative;
  display: flex;
  flex-direction: column;
  width: 100%;
  max-height: calc(100vh - var(--space-16));
  background: var(--bg-secondary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-xl);
  box-shadow: var(--shadow-xl);
}

.ui-modal__container--sm {
  max-width: 400px;
}

.ui-modal__container--md {
  max-width: 540px;
}

.ui-modal__container--lg {
  max-width: 720px;
}

.ui-modal__container--xl {
  max-width: 960px;
}

.ui-modal__container--full {
  max-width: 100%;
  height: 100%;
  max-height: 100vh;
  border-radius: 0;
}

/* Header */
.ui-modal__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  padding: var(--space-6);
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

.ui-modal__title {
  margin: 0;
  font: var(--font-display);
  font-size: var(--text-xl);
  font-weight: 600;
  color: var(--text-primary);
}

.ui-modal__close {
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

.ui-modal__close:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.ui-modal__close svg {
  width: 20px;
  height: 20px;
}

/* Body */
.ui-modal__body {
  flex: 1;
  padding: var(--space-6);
  overflow-y: auto;
  color: var(--text-secondary);
}

.ui-modal__body--no-header {
  border-top-left-radius: var(--radius-xl);
  border-top-right-radius: var(--radius-xl);
}

.ui-modal__body--no-footer {
  border-bottom-left-radius: var(--radius-xl);
  border-bottom-right-radius: var(--radius-xl);
}

/* Footer */
.ui-modal__footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-3);
  padding: var(--space-6);
  border-top: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

/* Scrollbar */
.ui-modal__body::-webkit-scrollbar {
  width: 6px;
}

.ui-modal__body::-webkit-scrollbar-track {
  background: transparent;
}

.ui-modal__body::-webkit-scrollbar-thumb {
  background: var(--border-default);
  border-radius: var(--radius-sm);
}

.ui-modal__body::-webkit-scrollbar-thumb:hover {
  background: var(--border-strong);
}

/* Responsive */
@media (max-width: 767px) {
  .ui-modal {
    padding: var(--space-4);
  }

  .ui-modal__container {
    max-height: calc(100vh - var(--space-8));
  }

  .ui-modal__header,
  .ui-modal__body,
  .ui-modal__footer {
    padding: var(--space-4);
  }
}

/* Transitions */
.ui-modal-enter-active,
.ui-modal-leave-active {
  transition: opacity var(--duration-base);
}

.ui-modal-enter-active .ui-modal__container,
.ui-modal-leave-active .ui-modal__container {
  transition: all var(--duration-base) ease;
}

.ui-modal-enter-from {
  opacity: 0;
}

.ui-modal-enter-from .ui-modal__container {
  opacity: 0;
  transform: scale(0.95);
}

.ui-modal-leave-to {
  opacity: 0;
}

.ui-modal-leave-to .ui-modal__container {
  opacity: 0;
  transform: scale(0.98);
}

.ui-modal-enter-to,
.ui-modal-leave-from {
  opacity: 1;
}

.ui-modal-enter-to .ui-modal__container,
.ui-modal-leave-from .ui-modal__container {
  opacity: 1;
  transform: scale(1);
}
</style>
