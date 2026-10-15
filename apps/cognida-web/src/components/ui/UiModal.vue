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
          ref="dialog"
          :class="containerClasses"
          :style="containerStyle"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="title || $slots.header ? titleId : undefined"
          :aria-label="!title && !$slots.header ? '对话框' : undefined"
          tabindex="-1"
          @click.stop
        >
          <!-- Header -->
          <div v-if="$slots.header || title || closable" class="ui-modal__header">
            <div :id="titleId"><slot name="header">
              <h3 class="ui-modal__title">{{ title }}</h3>
            </slot></div>
            <button
              v-if="closable"
              type="button"
              class="ui-modal__close"
              aria-label="关闭对话框"
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

<script lang="ts">
// All instances share a stack so nested dialogs neither steal focus nor unlock
// scrolling while another dialog remains open.
const openDialogs: HTMLElement[] = []
let savedBodyOverflow = ''
let dialogSequence = 0
</script>

<script setup lang="ts">
import { computed, ref, watch, nextTick, onMounted, onBeforeUnmount, useSlots } from 'vue'

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
const dialog = ref<HTMLElement>()
const titleId = `ui-modal-title-${++dialogSequence}`
let activeDialog: HTMLElement | undefined
let previousFocus: HTMLElement | undefined
let openRevision = 0

const bodyClasses = computed(() => [
  'ui-modal__body',
  { 'ui-modal__body--no-header': !slots.header && !props.title && !props.closable },
  { 'ui-modal__body--no-footer': !slots.footer }
])

function handleClose() {
  if (!props.modelValue) return
  emit('update:modelValue', false)
  emit('close')
}

function handleBackdropClick() {
  if (props.maskClosable) {
    handleClose()
  }
}

function handleKeydown(event: KeyboardEvent) {
  if (!props.modelValue || !activeDialog || openDialogs[openDialogs.length - 1] !== activeDialog) return
  if (event.key === 'Escape') {
    event.stopPropagation()
    if (props.closeOnEsc) {
      event.preventDefault()
      handleClose()
    }
  } else if (event.key === 'Tab') {
    const targets = focusableElements(activeDialog)
    const first = targets[0]
    const last = targets[targets.length - 1]
    if (!first || !last) {
      event.preventDefault()
      activeDialog.focus()
    } else if (event.shiftKey && (document.activeElement === first || !targets.includes(document.activeElement as HTMLElement))) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && (document.activeElement === last || !targets.includes(document.activeElement as HTMLElement))) {
      event.preventDefault()
      first.focus()
    }
  }
}

function focusableElements(container: HTMLElement) {
  return [...container.querySelectorAll<HTMLElement>('a[href], button, input, select, textarea, summary, [tabindex], [contenteditable="true"]')]
    .filter(element => element.tabIndex >= 0 && !element.matches(':disabled') && !element.closest('[hidden], [inert], [aria-hidden="true"]'))
    .filter(element => {
      for (let node: HTMLElement | null = element; node && node !== container; node = node.parentElement) {
        const style = getComputedStyle(node)
        if (style.display === 'none' || style.visibility === 'hidden') return false
        if (node.tagName === 'DETAILS' && !node.hasAttribute('open')) {
          const summary = [...node.children].find(child => child.tagName === 'SUMMARY')
          if (!summary?.contains(element)) return false
        }
      }
      return true
    })
}

function releaseDialog() {
  if (!activeDialog) return
  const wasTop = openDialogs[openDialogs.length - 1] === activeDialog
  const index = openDialogs.indexOf(activeDialog)
  if (index >= 0) openDialogs.splice(index, 1)
  activeDialog = undefined
  if (!openDialogs.length) document.body.style.overflow = savedBodyOverflow
  if (wasTop) {
    const top = openDialogs[openDialogs.length - 1]
    if (previousFocus?.isConnected && (!top || top.contains(previousFocus))) previousFocus.focus()
    else top?.focus()
  }
}

watch(() => props.modelValue, async (value) => {
  const revision = ++openRevision
  if (value) {
    previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
    await nextTick()
    if (revision !== openRevision || !props.modelValue || !dialog.value) return
    activeDialog = dialog.value
    if (!openDialogs.length) savedBodyOverflow = document.body.style.overflow
    openDialogs.push(activeDialog)
    document.body.style.overflow = 'hidden'
    const initial = focusableElements(activeDialog).find(element => element.hasAttribute('autofocus'))
    ;(initial || activeDialog).focus()
    emit('open')
  } else {
    releaseDialog()
  }
}, { immediate: true, flush: 'post' })

onMounted(() => {
  document.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  openRevision++
  document.removeEventListener('keydown', handleKeydown)
  releaseDialog()
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

.ui-modal__close:focus-visible {
  outline: 2px solid var(--accent-primary, #b8a177);
  outline-offset: 2px;
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

@media (prefers-reduced-motion: reduce) {
  .ui-modal-enter-active,
  .ui-modal-leave-active,
  .ui-modal-enter-active .ui-modal__container,
  .ui-modal-leave-active .ui-modal__container { transition: none; }
}
</style>
