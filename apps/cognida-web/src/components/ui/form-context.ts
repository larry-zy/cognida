import type { InjectionKey } from 'vue'

/**
 * 单条校验规则。兼容 Element Plus 风格写法，例如：
 * `{ required: true, message: '请选择', trigger: 'change' }`
 */
export interface FormRule {
  /** 是否必填 */
  required?: boolean
  /** 校验失败时展示的文案 */
  message?: string
  /** 触发时机 */
  trigger?: 'blur' | 'change'
  /** 最小长度（string / array）或最小值（number） */
  min?: number
  /** 最大长度（string / array）或最大值（number） */
  max?: number
  /** 正则校验 */
  pattern?: RegExp
  /** 值类型约束 */
  type?: 'string' | 'number' | 'email' | 'array'
  /**
   * 自定义校验器。
   * 返回 true 表示通过；返回字符串表示失败并作为错误信息；
   * 也可返回 Promise 以支持异步校验。
   */
  validator?: (
    rule: FormRule,
    value: unknown
  ) => boolean | string | Promise<boolean | string>
}

/** 已注册到 Form 的字段（由 FormItem 暴露给 Form 调用） */
export interface FormItemContext {
  /** 字段对应 model 的 key */
  prop?: string
  /** 执行校验，返回是否通过 */
  validate: (trigger?: 'blur' | 'change') => Promise<boolean>
  /** 清除校验状态 */
  clearValidate: () => void
  /** 重置字段值到初始值并清除校验 */
  resetField: () => void
  /** 聚焦到该字段（用于校验失败时定位） */
  focus: () => void
}

/** Form 向下提供的上下文 */
export interface FormContext {
  /** 表单数据对象（reactive） */
  model: Record<string, any>
  /** 表单级校验规则 */
  rules?: Record<string, FormRule[]>
  /** 标签位置 */
  labelPosition: 'top' | 'left'
  /** 标签宽度（left 布局生效） */
  labelWidth?: string
  /** 是否整体禁用 */
  disabled: boolean
  /** 尺寸 */
  size: 'sm' | 'md' | 'lg'
  /** 注册字段 */
  registerField: (field: FormItemContext) => void
  /** 注销字段 */
  unregisterField: (field: FormItemContext) => void
}

/** provide / inject key */
export const formContextKey: InjectionKey<FormContext> = Symbol('UiFormContext')
