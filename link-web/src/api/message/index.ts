import { http } from '@/utils/request'
import type { Message, CreateMessageRequest } from '@/types'

export const messageApi = {
  /**
   * 创建消息
   */
  create(data: CreateMessageRequest) {
    return http.post<Message>('/messages', data)
  },

  /**
   * 获取消息列表
   */
  list(params?: { session_id?: string; page?: number; size?: number }) {
    return http.get<{ messages: Message[]; total: number }>('/messages', { params })
  },

  /**
   * 获取消息详情
   */
  getById(id: string) {
    return http.get<Message>(`/messages/${id}`)
  },

  /**
   * 更新消息
   */
  update(id: string, data: Partial<CreateMessageRequest>) {
    return http.put<Message>(`/messages/${id}`, data)
  },

  /**
   * 删除消息
   */
  delete(id: string) {
    return http.delete(`/messages/${id}`)
  }
}
