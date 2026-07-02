<template>
  <div :class="itemClasses" :style="itemStyle">
    <label
      v-if="label || $slots.label"
      :for="itemId"
      class="ui-form-item__label"
      :class="{ 'ui-form-item__label--marked': showMarker }"
      :style="labelStyle"
    >
      <slot name="label">{{ label }}</slot>
      <span v-if="isRequired" class="ui-form-item__required" aria-hidden="true">*</span>
    </label>

    <div class="ui-form-item__content">
      <slot />

      <transition name="ui-form-item-error">
        <div v-if="displayError" class="ui-form-item__error">
          <svg
            class="ui-form-item__error-icon"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
          >
            <circle cx="12" cy="12" r="10" />
            <line x1="12" y1="8" x2="12" y2="12" />
            <line x1="12" y1="16" x2="12.01" y2="16" />
          </svg>
          <span class="ui-form-item__error-text">{{ displayError }}</span>
        </div>
      </transition>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  computed,
  inject,
  onMounted,
  onBeforeUnmount,
  ref,
  toRaw
} from 'vue'
import {
  formContextKey,
  type FormRule,
  type FormItemContext
} from './form-context'

interface Props {
  /** 标签文本 */
  label?: string
  /** 对应 model 的字段名，用于取值与取 rules */
  prop?: string
  /** 强制必填标记（同时参与 required 校验） */
  required?: boolean
  /** 字段级规则，覆盖 form 级 rules[prop] */
  rules?: FormRule[]
  /** 外部错误信息，优先展示 */
  error?: string
  /** 标签宽度，覆盖 form.labelWidth（left 布局生效） */
  labelWidth?: string
}

const props = withDefaults(defineProps<Props>(), {
  required: false
})

const form = inject(formContextKey, null)

const itemId = `ui-form-item-${Math.random().toString(36).substr(2, 9)}`

/** 校验产生的错误信息 */
const validateError = ref('')
/** 字段初始值快照，用于 resetField */
let initialValue: unknown

const size = computed(() => form?.size ?? 'md')
const labelPosition = computed(() => form?.labelPosition ?? 'top')

/** 合并 form 级与字段级规则 */
const resolvedRules = computed<FormRule[]>(() => {
  const formRules =
    props.prop && form?.rules ? form.rules[props.prop] ?? [] : []
  const ownRules = props.rules ?? []
  return [...formRules, ...ownRules]
})

/** 是否存在 required 规则（用于展示 * 与 marker） */
const isRequired = computed(
  () => props.required || resolvedRules.value.some((r) => r.required)
)

/** 优先展示外部 error，其次校验错误 */
const displayError = computed(() => props.error || validateError.value)

/** 聚焦态由内部输入控件的 focusin/focusout 驱动，用于点亮青色 marker */
const focused = ref(false)

const showMarker = computed(
  () => isRequired.value || focused.value || !!displayError.value
)

const itemClasses = computed(() => [
  'ui-form-item',
  `ui-form-item--${size.value}`,
  `ui-form-item--label-${labelPosition.value}`,
  {
    'ui-form-item--error': !!displayError.value,
    'ui-form-item--required': isRequired.value
  }
])

/** left 布局下的整体网格样式 */
const itemStyle = computed(() => {
  if (labelPosition.value !== 'left') return {}
  const width = props.labelWidth || form?.labelWidth
  if (!width) return {}
  return { '--ui-form-item-label-width': width } as Record<string, string>
})

const labelStyle = computed(() => {
  if (labelPosition.value !== 'left') return {}
  const width = props.labelWidth || form?.labelWidth
  return width ? { width } : {}
})

/** 取当前字段值 */
function getFieldValue(): unknown {
  if (!props.prop || !form) return undefined
  return form.model[props.prop]
}

/** 校验单条规则，返回错误信息（空串表示通过） */
async function runRule(rule: FormRule, value: unknown): Promise<string> {
  const isEmpty =
    value === undefined ||
    value === null ||
    value === '' ||
    (Array.isArray(value) && value.length === 0)

  // required
  if (rule.required && isEmpty) {
    return rule.message || `${props.label || props.prop || ''}不能为空`
  }

  // 非必填且为空时，跳过后续规则
  if (isEmpty && !rule.required) return ''

  // type 校验
  if (rule.type) {
    if (rule.type === 'array' && !Array.isArray(value)) {
      return rule.message || '格式不正确'
    }
    if (rule.type === 'number' && typeof value !== 'number') {
      return rule.message || '请输入数字'
    }
    if (rule.type === 'email') {
      const emailRe = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
      if (typeof value !== 'string' || !emailRe.test(value)) {
        return rule.message || '邮箱格式不正确'
      }
    }
  }

  // min / max
  if (rule.min !== undefined || rule.max !== undefined) {
    let compareValue: number | undefined
    if (typeof value === 'number') {
      compareValue = value
    } else if (typeof value === 'string' || Array.isArray(value)) {
      compareValue = value.length
    }
    if (compareValue !== undefined) {
      if (rule.min !== undefined && compareValue < rule.min) {
        return rule.message || `不能小于 ${rule.min}`
      }
      if (rule.max !== undefined && compareValue > rule.max) {
        return rule.message || `不能大于 ${rule.max}`
      }
    }
  }

  // pattern
  if (rule.pattern) {
    if (typeof value !== 'string' || !rule.pattern.test(value)) {
      return rule.message || '格式不正确'
    }
  }

  // 自定义校验器
  if (rule.validator) {
    const result = await rule.validator(rule, value)
    if (result === false) {
      return rule.message || '校验失败'
    }
    if (typeof result === 'string') {
      return result
    }
  }

  return ''
}

/**
 * 执行校验。可指定 trigger 仅跑对应触发时机的规则（缺省跑全部）
 */
async function validate(trigger?: 'blur' | 'change'): Promise<boolean> {
  // 无 prop 或无规则时视为通过
  if (!props.prop || resolvedRules.value.length === 0) {
    validateError.value = ''
    return true
  }

  const rules = trigger
    ? resolvedRules.value.filter((r) => !r.trigger || r.trigger === trigger)
    : resolvedRules.value

  if (rules.length === 0) return true

  const value = getFieldValue()
  for (const rule of rules) {
    const message = await runRule(rule, value)
    if (message) {
      validateError.value = message
      return false
    }
  }
  validateError.value = ''
  return true
}

function clearValidate() {
  validateError.value = ''
}

function resetField() {
  if (props.prop && form) {
    form.model[props.prop] = deepClone(initialValue)
  }
  clearValidate()
}

function focus() {
  const el = document.getElementById(itemId)
  const target =
    el && (el.matches('input, textarea, select') || false)
      ? el
      : document
          .querySelector(`.ui-form-item--error [id="${itemId}"]`)
          ?.closest('.ui-form-item')
          ?.querySelector<HTMLElement>('input, textarea, select, [tabindex]')
  if (target && 'focus' in target) {
    ;(target as HTMLElement).focus()
  } else {
    // 兜底：定位到本 FormItem 内首个可聚焦控件
    focusFirstControl()
  }
}

function focusFirstControl() {
  const root = rootByLabel()
  const control = root?.querySelector<HTMLElement>(
    'input, textarea, select, [tabindex]'
  )
  control?.focus()
}

function rootByLabel(): HTMLElement | null {
  const label = document.querySelector<HTMLElement>(`label[for="${itemId}"]`)
  return label?.closest('.ui-form-item') ?? null
}

/** 简易深拷贝，用于快照/还原字段初始值 */
function deepClone<T>(val: T): T {
  if (val === null || typeof val !== 'object') return val
  if (Array.isArray(val)) return val.map((v) => deepClone(v)) as unknown as T
  const out: Record<string, unknown> = {}
  for (const key of Object.keys(val as Record<string, unknown>)) {
    out[key] = deepClone((val as Record<string, unknown>)[key])
  }
  return out as T
}

const fieldContext: FormItemContext = {
  get prop() {
    return props.prop
  },
  validate,
  clearValidate,
  resetField,
  focus
}

function onFocusin() {
  focused.value = true
}
function onFocusout() {
  focused.value = false
  // 失焦时跑 blur 触发的规则
  if (props.prop) validate('blur')
}

let rootEl: HTMLElement | null = null

onMounted(() => {
  // 记录初始值快照
  initialValue = deepClone(toRaw(getFieldValue()))
  form?.registerField(fieldContext)
  rootEl = rootByLabel() || document.getElementById(itemId)?.closest('.ui-form-item') || null
  if (rootEl) {
    rootEl.addEventListener('focusin', onFocusin)
    rootEl.addEventListener('focusout', onFocusout)
  }
})

onBeforeUnmount(() => {
  form?.unregisterField(fieldContext)
  if (rootEl) {
    rootEl.removeEventListener('focusin', onFocusin)
    rootEl.removeEventListener('focusout', onFocusout)
  }
})

defineExpose({
  validate,
  clearValidate,
  resetField,
  focus
})
</script>

<style scoped>
.ui-form-item {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

/* left 布局：标签与内容并排 */
.ui-form-item--label-left {
  flex-direction: row;
  align-items: flex-start;
  gap: var(--space-4);
}

.ui-form-item--label-left .ui-form-item__label {
  padding-top: var(--space-2);
  flex-shrink: 0;
  justify-content: flex-end;
  text-align: right;
}

.ui-form-item--label-left .ui-form-item__content {
  flex: 1;
  min-width: 0;
}

/* 微标签风：小字号 + 字距 + 加粗 + 次级文字色 */
.ui-form-item__label {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  font-family: var(--font-body);
  font-size: 0.75rem;
  font-weight: 600;
  letter-spacing: 0.04em;
  color: var(--text-secondary);
  line-height: 1.4;
  transition: color var(--duration-fast) var(--ease-default);
}

/* 英文/大写标签场景：使用工具类由调用方按需叠加 */
.ui-form-item__label--upper {
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

/* 识别性细节：青色竖条 marker（2px 宽、约 10px 高） */
.ui-form-item__label--marked::before {
  content: '';
  position: absolute;
  left: calc(var(--space-2) * -1);
  top: 50%;
  transform: translateY(-50%);
  width: 2px;
  height: 10px;
  background: var(--primary);
  border-radius: var(--radius-full);
}

.ui-form-item--label-left .ui-form-item__label--marked::before {
  top: calc(var(--space-2) + 0.7em);
}

.ui-form-item__required {
  color: var(--danger);
  font-weight: 600;
}

.ui-form-item__content {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

/* 错误态：danger 色、小字号 */
.ui-form-item__error {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--danger);
  font-size: 0.75rem;
  line-height: 1.4;
}

.ui-form-item__error-icon {
  width: 12px;
  height: 12px;
  flex-shrink: 0;
}

.ui-form-item__error-text {
  letter-spacing: 0.02em;
}

/* 错误信息 slide-down 淡入 */
.ui-form-item-error-enter-active,
.ui-form-item-error-leave-active {
  transition:
    opacity var(--duration-fast) var(--ease-default),
    transform var(--duration-fast) var(--ease-default);
}

.ui-form-item-error-enter-from,
.ui-form-item-error-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
