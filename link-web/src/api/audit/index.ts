import { http } from '@/utils/request'

// 审计日志实体。
// 注意：后端返回的是 Go 领域实体，未加 json tag，故字段为 PascalCase。
export interface AuditLog {
  ID: number
  TenantID: number | null
  UserID: number | null
  Module: string
  Action: string
  ResourceType: string
  ResourceID: string | null
  ResourceName: string | null
  Details: string
  Status: string
  ErrorMessage: string | null
  IPAddress: string | null
  RequestID: string | null
  DurationMs: number | null
  CreatedAt: string
}

export interface AuditLogPage {
  list: AuditLog[]
  total: number
  page: number
  page_size: number
}

export interface AuditStats {
  Total: number
  ByModule: Record<string, number>
  ByDay: Array<{ Date: string; Count: number }>
}

export interface AuditLogQuery {
  page?: number
  page_size?: number
  request_id?: string
  module?: string
  action?: string
  resource_type?: string
  status?: string
  keywords?: string
  user_id?: string | number
  start_time?: string
  end_time?: string
}

export const auditApi = {
  /** 分页查询审计日志 */
  getAuditLogs(params?: AuditLogQuery) {
    return http.get<AuditLogPage>('/audit-logs', { params })
  },

  /** 审计统计（总量 / 按模块 / 按日） */
  getAuditStats(days = 7) {
    return http.get<AuditStats>('/audit-logs/stats', { params: { days } })
  },

  /** 按 ID 查询单条审计详情 */
  getAuditLog(id: number) {
    return http.get<AuditLog>(`/audit-logs/${id}`)
  },
}
