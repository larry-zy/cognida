import { http } from '@/utils/request'
import type { ChatRequest, ChatResponse } from '@/types'

export const chatApi = {
  /**
   * 非流式聊天
   */
  chat(data: ChatRequest) {
    return http.post<ChatResponse>('/chat', data)
  },

  /**
   * 带认证的聊天
   */
  chatWithAuth(data: ChatRequest) {
    return http.post<ChatResponse>('/chat/auth', data)
  }
}
