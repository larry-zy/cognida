<template>
  <div ref="triggerRef" :class="wrapperClasses">
    <slot />

    <Teleport to="body">
      <Transition name="ui-tooltip">
        <div
          v-if="visible"
          :class="tooltipClasses"
          :style="tooltipStyle"
          role="tooltip"
        >
          <div class="ui-tooltip__arrow" :style="arrowStyle"></div>
          <div class="ui-tooltip__content">
            <slot name="content">{{ content }}</slot>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount, nextTick } from 'vue'

interface Props {
  content?: string
  placement?: 'top' | 'bottom' | 'left' | 'right'
  trigger?: 'hover' | 'click' | 'focus'
  disabled?: boolean
  delay?: number
  arrow?: boolean
  maxWidth?: string | number
}

const props = withDefaults(defineProps<Props>(), {
  placement: 'top',
  trigger: 'hover',
  disabled: false,
  delay: 200,
  arrow: true,
  maxWidth: 300
})

const emit = defineEmits<{
  'show': []
  'hide': []
}>()

const triggerRef = ref<HTMLElement>()
const visible = ref(false)
const tooltipPosition = ref({ top: '0px', left: '0px' })
const arrowPosition = ref({ left: '', top: '' })
let timer: number | null = null

const wrapperClasses = computed(() => [
  'ui-tooltip-wrapper',
  { 'ui-tooltip-wrapper--disabled': props.disabled }
])

const tooltipClasses = computed(() => [
  'ui-tooltip',
  `ui-tooltip--${props.placement}`
])

const tooltipStyle = computed(() => ({
  ...tooltipPosition.value,
  maxWidth: typeof props.maxWidth === 'number' ? `${props.maxWidth}px` : props.maxWidth,
  zIndex: '1070'
}))

const arrowStyle = computed(() => arrowPosition.value)

function show() {
  if (props.disabled) return

  timer = window.setTimeout(() => {
    visible.value = true
    emit('show')
    nextTick(updatePosition)
  }, props.delay)
}

function hide() {
  if (timer) {
    clearTimeout(timer)
    timer = null
  }

  visible.value = false
  emit('hide')
}

function updatePosition() {
  if (!triggerRef.value) return

  const triggerRect = triggerRef.value.getBoundingClientRect()
  const scrollX = window.pageXOffset
  const scrollY = window.pageYOffset

  // Tooltip dimensions (will be calculated after render)
  const tooltipWidth = 150
  const tooltipHeight = 40
  const gap = 8

  let top = 0
  let left = 0
  let arrowLeft = ''
  let arrowTop = ''

  switch (props.placement) {
    case 'top':
      top = triggerRect.top + scrollY - tooltipHeight - gap
      left = triggerRect.left + scrollX + (triggerRect.width - tooltipWidth) / 2
      arrowTop = '100%'
      arrowLeft = '50%'
      break
    case 'bottom':
      top = triggerRect.bottom + scrollY + gap
      left = triggerRect.left + scrollX + (triggerRect.width - tooltipWidth) / 2
      arrowTop = '0'
      arrowLeft = '50%'
      break
    case 'left':
      top = triggerRect.top + scrollY + (triggerRect.height - tooltipHeight) / 2
      left = triggerRect.left + scrollX - tooltipWidth - gap
      arrowTop = '50%'
      arrowLeft = '100%'
      break
    case 'right':
      top = triggerRect.top + scrollY + (triggerRect.height - tooltipHeight) / 2
      left = triggerRect.right + scrollX + gap
      arrowTop = '50%'
      arrowLeft = '0'
      break
  }

  tooltipPosition.value = { top: `${top}px`, left: `${left}px` }
  arrowPosition.value = { left: arrowLeft, top: arrowTop }
}

function addEventListeners() {
  if (!triggerRef.value) return

  if (props.trigger === 'hover') {
    triggerRef.value.addEventListener('mouseenter', show)
    triggerRef.value.addEventListener('mouseleave', hide)
  } else if (props.trigger === 'click') {
    triggerRef.value.addEventListener('click', toggle)
  } else if (props.trigger === 'focus') {
    triggerRef.value.addEventListener('focus', show)
    triggerRef.value.addEventListener('blur', hide)
  }
}

function removeEventListeners() {
  if (!triggerRef.value) return

  if (props.trigger === 'hover') {
    triggerRef.value.removeEventListener('mouseenter', show)
    triggerRef.value.removeEventListener('mouseleave', hide)
  } else if (props.trigger === 'click') {
    triggerRef.value.removeEventListener('click', toggle)
  } else if (props.trigger === 'focus') {
    triggerRef.value.removeEventListener('focus', show)
    triggerRef.value.removeEventListener('blur', hide)
  }
}

function toggle() {
  visible.value ? hide() : show()
}

onMounted(() => {
  addEventListeners()
})

onBeforeUnmount(() => {
  removeEventListeners()
  if (timer) {
    clearTimeout(timer)
  }
})
</script>

<style scoped>
.ui-tooltip-wrapper {
  position: relative;
  display: inline-block;
}

.ui-tooltip {
  position: absolute;
  background: var(--bg-elevated);
  color: var(--text-primary);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  font-size: var(--text-sm);
  line-height: 1.5;
  box-shadow: var(--shadow-lg);
  white-space: nowrap;
  pointer-events: none;
}

.ui-tooltip__arrow {
  position: absolute;
  width: 8px;
  height: 8px;
  background: var(--bg-elevated);
  transform: translate(-50%, -50%) rotate(45deg);
}

.ui-tooltip--top .ui-tooltip__arrow {
  bottom: -4px;
}

.ui-tooltip--bottom .ui-tooltip__arrow {
  top: -4px;
}

.ui-tooltip--left .ui-tooltip__arrow {
  right: -4px;
}

.ui-tooltip--right .ui-tooltip__arrow {
  left: -4px;
}

.ui-tooltip__content {
  position: relative;
  z-index: 1;
}

/* Transition */
.ui-tooltip-enter-active,
.ui-tooltip-leave-active {
  transition: all var(--duration-fast) ease;
}

.ui-tooltip-enter-from,
.ui-tooltip-leave-to {
  opacity: 0;
  transform: scale(0.9);
}

.ui-tooltip-enter-to,
.ui-tooltip-leave-from {
  opacity: 1;
  transform: scale(1);
}
</style>
