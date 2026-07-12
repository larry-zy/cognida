import { http } from '@/utils/request'

// ==================== 类型定义 ====================

/** 数据源类型 */
export type DatasourceType = 'mysql' | 'postgres'

/** 数据源状态 */
export type DatasourceStatus = 'active' | 'inactive' | 'error' | 'need_credentials'

/**
 * 脱敏后的数据源（服务端永远不返回密码字段）。
 */
export interface Datasource {
  id: string
  name: string
  type: DatasourceType
  host: string
  port: number
  database_name: string
  username: string
  status: DatasourceStatus
  last_health_check_at?: string
  created_at: string
  updated_at: string
}

/** 数据源分页列表响应 */
export interface DatasourceListResponse {
  items: Datasource[]
  total: number
  page: number
  page_size: number
}

/**
 * 创建数据源请求体。
 * 创建时 password 必填；extra 为可选扩展字段（JSON 对象）。
 */
export interface CreateDatasourceRequest {
  name: string
  type: DatasourceType
  host: string
  port: number
  database_name: string
  username: string
  password: string
  extra?: Record<string, unknown>
}

/**
 * 更新数据源请求体。
 * 空字符串 password 表示"不修改密码"（后端语义：empty = keep old）。
 */
export interface UpdateDatasourceRequest {
  name?: string
  type?: DatasourceType
  host?: string
  port?: number
  database_name?: string
  username?: string
  /** 留空 = 保留旧密码 */
  password?: string
  extra?: Record<string, unknown>
}

/**
 * 测试连接请求体（二选一）：
 * - 已保存的数据源：传 datasource_id（无需密码）
 * - 表单即时测试：传完整连接信息（包含密码）
 */
export type TestDatasourceRequest =
  | { datasource_id: string }
  | {
      name: string
      type: DatasourceType
      host: string
      port: number
      database_name: string
      username: string
      password: string
    }

/**
 * 测试连接结果。
 * 连接失败是业务结果（ok=false + message），不会产生 HTTP 错误。
 */
export interface TestDatasourceResult {
  ok: boolean
  message?: string
}

/** 表列表响应 */
export interface DatasourceTablesResponse {
  tables: string[]
}

/** 表结构详情（列信息等，后端可按需扩展） */
export interface DatasourceTableDetail {
  table: string
  columns?: Array<{
    name: string
    type: string
    nullable?: boolean
    key?: string
    default?: string
    comment?: string
  }>
  [key: string]: unknown
}

// ==================== API ====================

export const datasourceApi = {
  /**
   * 分页获取数据源列表
   */
  list(params?: { page?: number; page_size?: number }) {
    return http.get<DatasourceListResponse>('/datasources', { params })
  },

  /**
   * 获取数据源详情（脱敏，不含密码）
   */
  getById(id: string) {
    return http.get<Datasource>(`/datasources/${id}`)
  },

  /**
   * 创建数据源。
   * 名称冲突时后端返回 400，message = "数据源名称已存在"。
   */
  create(data: CreateDatasourceRequest) {
    return http.post<Datasource>('/datasources', data)
  },

  /**
   * 更新数据源。
   * 传空字符串 password = 保留旧密码（后端语义）。
   */
  update(id: string, data: UpdateDatasourceRequest) {
    return http.put<Datasource>(`/datasources/${id}`, data)
  },

  /**
   * 删除数据源
   */
  delete(id: string) {
    return http.delete<{ deleted: boolean }>(`/datasources/${id}`)
  },

  /**
   * 测试连接。
   * 传 datasource_id 测试已保存数据源；
   * 传完整表单字段测试未保存的连接配置。
   * 返回 { ok, message } — 失败是业务结果，不是 HTTP 错误。
   */
  testConnection(data: TestDatasourceRequest) {
    return http.post<TestDatasourceResult>('/datasources/test', data)
  },

  /**
   * 获取数据源下的表列表
   */
  getTables(id: string) {
    return http.get<DatasourceTablesResponse>(`/datasources/${id}/tables`)
  },

  /**
   * 获取指定表的结构详情
   */
  getTableDetail(id: string, table: string) {
    return http.get<DatasourceTableDetail>(`/datasources/${id}/tables/${encodeURIComponent(table)}`)
  }
}

export default datasourceApi
