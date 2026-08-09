<template>
  <form :class="formClasses" @submit.prevent>
    <slot />
  </form>
</template>

<script setup lang="ts">
import { computed, provide, reactive, toRefs } from 'vue'
import {
  formContextKey,
  type FormRule,
  type FormContext,
  type FormItemContext
} from './form-context'

interface Props {
  /** 表单数据对象 */
  model: Record<string, any>
  /** 表单级校验规则，键为 model 的字段名 */
  rules?: Record<string, FormRule[]>
  /** 标签位置 */
  labelPosition?: 'top' | 'left'
  /** 标签宽度（left 布局生效），如 '120px' */
  labelWidth?: string
  /** 整体禁用 */
  disabled?: boolean
  /** 尺寸 */
  size?: 'sm' | 'md' | 'lg'
}

const props = withDefaults(defineProps<Props>(), {
  labelPosition: 'top',
  disabled: false,
  size: 'md'
})

const formClasses = computed(() => [
  'ui-form',
  `ui-form--${props.size}`,
  `ui-form--label-${props.labelPosition}`
])

/** 已注册的字段集合 */
const fields: FormItemContext[] = []

function registerField(field: FormItemContext) {
  fields.push(field)
}

function unregisterField(field: FormItemContext) {
  const index = fields.indexOf(field)
  if (index !== -1) fields.splice(index, 1)
}

/** 提供给子 FormItem 的响应式上下文 */
// toRefs(props) 的 ref 只读且随 props 变化（withDefaults 已保证默认值），
// 无需（也不能）再手动回写，否则触发 "target is readonly" 警告。
const context = reactive({
  ...toRefs(props),
  registerField,
  unregisterField
}) as unknown as FormContext

provide(formContextKey, context)

/**
 * 校验指定字段（可传单个 prop）
 */
async function validateField(prop: string): Promise<boolean> {
  const target = fields.filter((f) => f.prop === prop)
  if (target.length === 0) return true
  const results = await Promise.all(target.map((f) => f.validate()))
  return results.every(Boolean)
}

/**
 * 校验全部字段，全部通过 resolve true；
 * 否则 resolve false 并聚焦第一个错误字段
 */
async function validate(): Promise<boolean> {
  const results = await Promise.all(fields.map((f) => f.validate()))
  const allValid = results.every(Boolean)
  if (!allValid) {
    const firstInvalidIndex = results.findIndex((ok) => !ok)
    if (firstInvalidIndex !== -1) {
      fields[firstInvalidIndex]?.focus()
    }
  }
  return allValid
}

/**
 * 重置所有字段到初始值并清除校验状态
 */
function resetFields() {
  fields.forEach((f) => f.resetField())
}

/**
 * 清除校验状态。可指定 prop 或 prop 数组，缺省清除全部
 */
function clearValidate(clearProps?: string | string[]) {
  if (!clearProps) {
    fields.forEach((f) => f.clearValidate())
    return
  }
  const list = Array.isArray(clearProps) ? clearProps : [clearProps]
  fields
    .filter((f) => f.prop !== undefined && list.includes(f.prop))
    .forEach((f) => f.clearValidate())
}

defineExpose({
  validate,
  validateField,
  resetFields,
  clearValidate
})
</script>

<style scoped>
.ui-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  font-family: var(--font-body);
}

.ui-form--sm {
  gap: var(--space-4);
}

.ui-form--lg {
  gap: var(--space-6);
}
</style>
