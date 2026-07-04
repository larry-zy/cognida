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

// 交互组件（Button/Confirm/Form/Filter/Pagination）触发的动作。
// Pagination 由 A2UIRenderer 内部消化（按 surface+token 回源取页）；
// 其余动作向上抛给宿主视图（Phase 4/5 的操作类工具与确认流消费）。
export interface A2UIAction {
  // 动作名：组件 props.action 里声明的 name（Pagination 固定为 'paginate'）
  name: string
  // 来源 surface（Renderer 补齐）
  surface?: string
  // 动作参数：Form 提交值 / Filter 选中值 / Confirm 的 pending_action_id 等
  params?: Record<string, any>
}

// 递归节点通过 inject 拿到「id→节点」映射与 dataModel，避免逐层透传 props。
export interface A2UIContext {
  nodes: Ref<Record<string, A2UIComponent>>
  dataModel: Ref<A2UIDataModel>
  // 交互回调入口：节点触发动作统一走这里（由 A2UIRenderer 注入）
  onAction: (action: A2UIAction) => void
}

export const A2UI_CTX: InjectionKey<A2UIContext> = Symbol('a2ui-context')
