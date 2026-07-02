import type { InjectionKey, Ref } from 'vue'

// A2UI 规范中的单个组件节点（邻接表扁平模型）。
export interface A2UIComponent {
  id: string
  type: string
  props?: Record<string, any>
  children?: string[]
}

// 后端装配的可信数据信封：数字全部来自真实 sql_execute / data_analysis。
export interface A2UIDataModel {
  table?: { columns: string[]; rows: Record<string, any>[] } | null
  metrics?: Record<string, any>
  series?: { name?: string; labels?: any[]; actual?: number[]; forecast?: number[] } | null
  meta?: Record<string, any>
}

// 完整 UISpec（对齐 Go 端 genui.UISpec 的 JSON 序列化）。
export interface A2UISpec {
  surface?: string
  title?: string
  catalog?: string[]
  genMode?: string
  components: A2UIComponent[]
  dataModel?: A2UIDataModel | null
}

// 递归节点通过 inject 拿到「id→节点」映射与 dataModel，避免逐层透传 props。
export interface A2UIContext {
  nodes: Ref<Record<string, A2UIComponent>>
  dataModel: Ref<A2UIDataModel>
}

export const A2UI_CTX: InjectionKey<A2UIContext> = Symbol('a2ui-context')
