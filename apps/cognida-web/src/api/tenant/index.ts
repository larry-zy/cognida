import { http } from '@/utils/request'
import type { Tenant, CreateTenantRequest, UpdateTenantRequest } from '@/types'

export const tenantApi = {
  /**
   * 创建租户
   */
  create(data: CreateTenantRequest) {
    return http.post<Tenant>('/tenants', data)
  },

  /**
   * 获取租户列表
   */
  list() {
    return http.get<Tenant[]>('/tenants')
  },

  /**
   * 获取租户详情
   */
  getById(id: number) {
    return http.get<Tenant>(`/tenants/${id}`)
  },

  /**
   * 更新租户
   */
  update(id: number, data: UpdateTenantRequest) {
    return http.put<Tenant>(`/tenants/${id}`, data)
  },

  /**
   * 删除租户
   */
  delete(id: number) {
    return http.delete(`/tenants/${id}`)
  },

  /**
   * 重新生成API密钥
   */
  regenerateApiKey(id: number) {
    return http.post<{ api_key: string }>(`/tenants/${id}/api-key`)
  },

  /**
   * 获取存储用量
   */
  getStorageUsage(id: number) {
    return http.get<{ used: number; limit: number }>(`/tenants/${id}/storage`)
  }
}
