import { http } from '@/utils/request'

// 调用链单个 span。
// 注意：后端已加 json tag，字段为 snake_case。
// attributes / events 为 JSON 字符串，前端展示前需自行 parse。
export interface Span {
  span_id: string
  parent_span_id: string
  name: string
  kind: string
  start_time: string
  end_time: string
  duration_ms: number
  status_code: string
  status_msg: string
  agent_name: string
  attributes: string
  events: string
}

// 预构建的 span 树节点（后端已按 parent_span_id 组装好层级）。
export interface SpanNode {
  span: Span
  children: SpanNode[]
}

// trace 列表项（概要）。
export interface TraceSummary {
  trace_id: string
  root_name: string
  request_id: string
  session_id: string
  agent_name: string
  span_count: number
  start_time: string
  end_time: string
  duration_ms: number
  has_error: boolean
}

// trace 列表分页响应。
export interface TracePage {
  list: TraceSummary[]
  total: number
  page: number
  page_size: number
}

// 单条 trace 详情（全部 span + 预构建树）。
export interface TraceDetail {
  trace_id: string
  spans: Span[]
  tree: SpanNode[]
}

// trace 列表查询参数。
export interface TraceQuery {
  page?: number
  page_size?: number
  request_id?: string
  session_id?: string
  agent_name?: string
  trace_id?: string
  only_error?: boolean
  start_time?: string
  end_time?: string
}

export const traceApi = {
  /** 分页查询调用链列表 */
  listTraces(params?: TraceQuery) {
    return http.get<TracePage>('/traces', { params })
  },

  /** 查询单条调用链的全部 span 及树结构 */
  getTrace(traceID: string) {
    return http.get<TraceDetail>(`/traces/${traceID}`)
  },
}

// 兼容按需直接引入的写法。
export const listTraces = traceApi.listTraces
export const getTrace = traceApi.getTrace
