import { http } from '@/utils/request'

export const modelApi = {
  /**
   * 获取模型列表
   */
  getList(type?: string) {
    return http.get<any>('/models', { params: { type } })
  },

  /**
   * 获取单个模型
   */
  getById(id: string) {
    return http.get<any>(`/models/${id}`)
  }
}
