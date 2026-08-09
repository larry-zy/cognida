// 组件通用类型定义
import type { Component } from 'vue'

export type Size = 'xs' | 'sm' | 'md' | 'lg' | 'xl'

export type Variant =
  | 'default'
  | 'primary'
  | 'secondary'
  | 'success'
  | 'warning'
  | 'danger'
  | 'info'
  | 'ghost'
  | 'gradient'
  | 'glass'

export type Color =
  | 'primary'
  | 'secondary'
  | 'success'
  | 'warning'
  | 'danger'
  | 'info'
  | 'accent'

export type Align = 'left' | 'center' | 'right'

export type Shape = 'square' | 'rounded' | 'pill' | 'circle'

// 组件 Props 类型
export interface BaseProps {
  class?: any
  style?: any
}

export interface ClickableProps extends BaseProps {
  disabled?: boolean
  loading?: boolean
}

export interface IconProps {
  icon?: string | Component
}

// 组件事件类型
export interface ComponentEvents {
  click?: [event: Event]
  change?: [value: any]
  focus?: [event: FocusEvent]
  blur?: [event: FocusEvent]
}
