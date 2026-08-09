// ==================== 组件库统一出口 ====================
// 自研 UI 组件已统一到 './ui'，请优先从这里或 '@/components/ui' 引入，
// 避免在视图中直接使用 Element Plus 的 el-* 组件。
// 用法：import { UiButton, UiForm, UiFormItem } from '@/components'

// 自研 UI 组件（基础/表单/数据/反馈/导航/布局/图标）
export * from './ui'

// 视觉背景组件
export { default as AppBackground } from './AppBackground.vue'

// 类型
export type * from './types'
