<template>
  <div :class="wrapperClasses" v-click-outside="handleClickOutside">
    <label v-if="label" :for="selectId" class="ui-select__label">
      {{ label }}
      <span v-if="required" class="ui-select__required">*</span>
    </label>

    <div class="ui-select__container">
      <div
        :class="triggerClasses"
        tabindex="0"
        @click="toggleDropdown"
        @keydown="handleKeydown"
      >
        <span v-if="$slots.prefix" class="ui-select__prefix">
          <slot name="prefix" />
        </span>

        <span class="ui-select__value">
          <template v-if="selectedLabel">
            {{ selectedLabel }}
          </template>
          <span v-else class="ui-select__placeholder">
            {{ placeholder }}
          </span>
        </span>

        <span class="ui-select__suffix">
          <svg
            :class="{ 'ui-select__arrow--open': isOpen }"
            class="ui-select__arrow"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
          >
            <polyline points="6 9 12 15 18 9" />
          </svg>
        </span>
      </div>

      <Teleport to="body">
        <Transition name="ui-select-dropdown">
          <div
            v-if="isOpen"
            :class="dropdownClasses"
            :style="dropdownStyle"
          >
            <div v-if="filterable" class="ui-select__filter">
              <UiInput
                v-model="searchQuery"
                :placeholder="filterPlaceholder"
                size="sm"
              >
                <template #prefix>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <circle cx="11" cy="11" r="8" />
                    <path d="m21 21-4.35-4.35" />
                  </svg>
                </template>
              </UiInput>
            </div>

            <div class="ui-select__options">
              <div
                v-for="option in filteredOptions"
                :key="option.value"
                :class="[
                  'ui-select__option',
                  {
                    'ui-select__option--selected': isSelected(option),
                    'ui-select__option--disabled': option.disabled,
                    'ui-select__option--multiple': multiple
                  }
                ]"
                @click.stop="selectOption(option)"
                @mouseenter="hoveredIndex = filteredOptions.indexOf(option)"
              >
                <slot name="option" :option="option" :selected="isSelected(option)">
                  <span v-if="multiple && isSelected(option)" class="ui-select__check">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <polyline points="20 6 9 17 4 12" />
                    </svg>
                  </span>
                  <span class="ui-select__option-label">{{ option.label }}</span>
                  <span v-if="option.hint" class="ui-select__option-hint">{{ option.hint }}</span>
                </slot>
              </div>

              <div v-if="filteredOptions.length === 0" class="ui-select__empty">
                <slot name="empty">{{ emptyText }}</slot>
              </div>
            </div>
          </div>
        </Transition>
      </Teleport>
    </div>

    <div v-if="hint || error" class="ui-select__hint">
      <span v-if="error" class="ui-select__error">{{ error }}</span>
      <span v-else-if="hint" class="ui-select__hint-text">{{ hint }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import UiInput from './UiInput.vue'

// 扩展 HTMLElement 类型以支持 clickOutside 属性
declare global {
  interface HTMLElement {
    clickOutside?: (event: MouseEvent) => void
  }
}

interface SelectOption {
  label: string
  value: string | number
  disabled?: boolean
  hint?: string
}

interface Props {
  modelValue?: string | number | (string | number)[]
  options?: SelectOption[]
  label?: string
  placeholder?: string
  hint?: string
  error?: string
  disabled?: boolean
  required?: boolean
  clearable?: boolean
  filterable?: boolean
  filterPlaceholder?: string
  emptyText?: string
  multiple?: boolean
  id?: string
  size?: 'sm' | 'md' | 'lg'
  placement?: 'bottom' | 'top'
}

const props = withDefaults(defineProps<Props>(), {
  options: () => [],
  placeholder: '请选择',
  filterPlaceholder: '搜索',
  emptyText: '暂无数据',
  disabled: false,
  required: false,
  clearable: false,
  filterable: false,
  multiple: false,
  size: 'md',
  placement: 'bottom'
})

const emit = defineEmits<{
  'update:modelValue': [value: string | number | (string | number)[]]
  'change': [value: string | number | (string | number)[]]
  'focus': [event: FocusEvent]
  'blur': [event: FocusEvent]
  'visible-change': [visible: boolean]
}>()

const selectId = props.id || `ui-select-${Math.random().toString(36).substr(2, 9)}`
const isOpen = ref(false)
const searchQuery = ref('')
const hoveredIndex = ref(-1)
const triggerRef = ref<HTMLElement>()
const dropdownRef = ref<HTMLElement>()
const dropdownPosition = ref({ top: '0px', left: '0px', width: '0px' })

const wrapperClasses = computed(() => [
  'ui-select-wrapper',
  `ui-select-wrapper--${props.size}`
])

const triggerClasses = computed(() => [
  'ui-select__trigger',
  `ui-select__trigger--${props.size}`,
  {
    'ui-select__trigger--disabled': props.disabled,
    'ui-select__trigger--error': props.error,
    'ui-select__trigger--open': isOpen.value
  }
])

const dropdownClasses = computed(() => [
  'ui-select__dropdown',
  `ui-select__dropdown--${props.placement}`,
  `ui-select__dropdown--${props.size}`
])

const dropdownStyle = computed(() => ({
  ...dropdownPosition.value,
  zIndex: '1050'
}))

const selectedLabel = computed(() => {
  if (props.multiple && Array.isArray(props.modelValue)) {
    return props.modelValue.length > 0
      ? `已选择 ${props.modelValue.length} 项`
      : ''
  }

  const option = props.options.find(opt => opt.value === props.modelValue)
  return option?.label || ''
})

const filteredOptions = computed(() => {
  if (!props.filterable || !searchQuery.value) {
    return props.options
  }

  const query = searchQuery.value.toLowerCase()
  return props.options.filter(opt =>
    opt.label.toLowerCase().includes(query)
  )
})

function isSelected(option: SelectOption): boolean {
  if (props.multiple && Array.isArray(props.modelValue)) {
    return props.modelValue.includes(option.value)
  }
  return props.modelValue === option.value
}

function toggleDropdown() {
  if (props.disabled) return

  isOpen.value = !isOpen.value

  if (isOpen.value) {
    nextTick(updateDropdownPosition)
    searchQuery.value = ''
    hoveredIndex.value = -1
  }

  emit('visible-change', isOpen.value)
}

function selectOption(option: SelectOption) {
  if (option.disabled) return

  let newValue: string | number | (string | number)[]

  if (props.multiple) {
    const currentValue = (props.modelValue || []) as (string | number)[]
    const index = currentValue.indexOf(option.value)

    if (index > -1) {
      newValue = currentValue.filter(v => v !== option.value)
    } else {
      newValue = [...currentValue, option.value]
    }
  } else {
    newValue = option.value
    isOpen.value = false
  }

  emit('update:modelValue', newValue)
  emit('change', newValue)
}

function handleKeydown(event: KeyboardEvent) {
  switch (event.key) {
    case 'Enter':
    case ' ':
      event.preventDefault()
      toggleDropdown()
      break
    case 'Escape':
      if (isOpen.value) {
        event.preventDefault()
        isOpen.value = false
      }
      break
    case 'ArrowDown':
      event.preventDefault()
      if (!isOpen.value) {
        isOpen.value = true
      } else {
        navigateOptions(1)
      }
      break
    case 'ArrowUp':
      event.preventDefault()
      if (isOpen.value) {
        navigateOptions(-1)
      }
      break
  }
}

function navigateOptions(direction: number) {
  const options = filteredOptions.value.filter(opt => !opt.disabled)
  if (options.length === 0) return

  hoveredIndex.value += direction

  if (hoveredIndex.value < 0) {
    hoveredIndex.value = options.length - 1
  } else if (hoveredIndex.value >= options.length) {
    hoveredIndex.value = 0
  }

  const option = options[hoveredIndex.value]
  selectOption(option)
}

function updateDropdownPosition() {
  if (!triggerRef.value || !dropdownRef.value) return

  const triggerRect = triggerRef.value.getBoundingClientRect()
  const dropdownHeight = dropdownRef.value.offsetHeight

  let top = '0px'
  if (props.placement === 'bottom') {
    top = `${triggerRect.bottom + 4}px`
  } else {
    top = `${triggerRect.top - dropdownHeight - 4}px`
  }

  dropdownPosition.value = {
    top,
    left: `${triggerRect.left}px`,
    width: `${triggerRect.width}px`
  }
}

function handleClickOutside() {
  if (isOpen.value) {
    isOpen.value = false
    emit('visible-change', false)
  }
}

const vClickOutside = {
  mounted(el: HTMLElement, binding: any) {
    el.clickOutside = (event: MouseEvent) => {
      if (!el.contains(event.target as Node)) {
        binding.value()
      }
    }
    document.addEventListener('click', el.clickOutside)
  },
  unmounted(el: HTMLElement) {
    document.removeEventListener('click', el.clickOutside as any)
  }
}

onMounted(() => {
  window.addEventListener('scroll', updateDropdownPosition)
  window.addEventListener('resize', updateDropdownPosition)
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', updateDropdownPosition)
  window.removeEventListener('resize', updateDropdownPosition)
})

watch(isOpen, (val) => {
  if (val) {
    nextTick(updateDropdownPosition)
  }
})
</script>

<style scoped>
.ui-select-wrapper {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.ui-select__label {
  font-size: var(--text-sm);
  font-weight: 500;
  color: var(--text-secondary);
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.ui-select__required {
  color: var(--danger);
}

.ui-select__container {
  position: relative;
}

.ui-select__trigger {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
  font-family: var(--font-body);
  font-size: var(--text-base);
  color: var(--text-primary);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  cursor: pointer;
  user-select: none;
  transition: all var(--duration-fast);
}

.ui-select__trigger:hover:not(.ui-select__trigger--disabled) {
  border-color: var(--border-strong);
}

.ui-select__trigger:focus,
.ui-select__trigger--open {
  outline: none;
  border-color: var(--primary);
}

.ui-select__trigger--disabled {
  background: var(--bg-secondary);
  color: var(--text-muted);
  cursor: not-allowed;
}

.ui-select__trigger--error {
  border-color: var(--danger);
}

.ui-select__value {
  flex: 1;
  min-width: 0;
}

.ui-select__placeholder {
  color: var(--text-muted);
}

.ui-select__prefix {
  display: flex;
  align-items: center;
  color: var(--text-muted);
}

.ui-select__suffix {
  display: flex;
  align-items: center;
}

.ui-select__arrow {
  width: 16px;
  height: 16px;
  color: var(--text-muted);
  transition: transform var(--duration-fast);
}

.ui-select__arrow--open {
  transform: rotate(180deg);
}

/* Sizes */
.ui-select__trigger--sm {
  padding: var(--space-2) var(--space-3);
  font-size: var(--text-sm);
}

.ui-select__trigger--lg {
  padding: var(--space-4);
  font-size: var(--text-lg);
}

/* Dropdown */
.ui-select__dropdown {
  position: fixed;
  background: var(--bg-secondary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-xl);
  max-height: 256px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.ui-select__filter {
  padding: var(--space-3);
  border-bottom: 1px solid var(--border-subtle);
}

.ui-select__options {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-2);
}

.ui-select__option {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: all var(--duration-fast);
}

.ui-select__option:hover:not(.ui-select__option--disabled) {
  background: var(--bg-hover);
}

.ui-select__option--selected {
  background: var(--bg-active);
  color: var(--primary);
}

.ui-select__option--disabled {
  color: var(--text-muted);
  cursor: not-allowed;
  opacity: 0.6;
}

.ui-select__check {
  display: flex;
  align-items: center;
  color: var(--primary);
}

.ui-select__check svg {
  width: 16px;
  height: 16px;
}

.ui-select__option-label {
  flex: 1;
}

.ui-select__option-hint {
  font-size: var(--text-sm);
  color: var(--text-muted);
}

.ui-select__empty {
  padding: var(--space-6);
  text-align: center;
  color: var(--text-muted);
}

/* Hint */
.ui-select__hint {
  display: flex;
  align-items: center;
  min-height: 20px;
}

.ui-select__hint-text {
  font-size: var(--text-sm);
  color: var(--text-muted);
}

.ui-select__error {
  font-size: var(--text-sm);
  color: var(--danger);
}

/* Dropdown transition */
.ui-select-dropdown-enter-active,
.ui-select-dropdown-leave-active {
  transition: all var(--duration-base) ease;
}

.ui-select-dropdown-enter-from {
  opacity: 0;
  transform: translateY(-8px);
}

.ui-select-dropdown-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

.ui-select-dropdown-enter-to,
.ui-select-dropdown-leave-from {
  opacity: 1;
  transform: translateY(0);
}
</style>
