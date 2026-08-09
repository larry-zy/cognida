import { http } from '@/utils/request'
import type {
  Session,
  CreateSessionRequest,
  UpdateSessionRequest,
  SessionDetail
} from '@/types'

// 后端实际返回的会话列表格式（对齐 Go SessionListResponse：sessions/total/page/size）
interface BackendSessionListResponse {
  sessions: Session[]
  total: number
  page: number
  size: number
}

// 前端期望的会话列表响应格式
interface FrontendSessionListResponse {
  sessions: Session[]
  total: number
  page: number
  size: number
}

export const sessionApi = {
  /**
   * 创建会话
   */
  create(data: CreateSessionRequest) {
    return http.post<Session>('/sessions', data)
  },

  /**
   * 获取会话详情
   */
  getById(id: string) {
    return http.get<Session>(`/sessions/${id}`)
  },

  /**
   * 获取会话完整详情（包含消息）
   */
  getDetail(id: string) {
    return http.get<SessionDetail>(`/sessions/${id}/detail`)
  },

  /**
   * 获取会话列表（适配后端响应格式）
   */
  async list(params?: { page?: number; page_size?: number; status?: number }): Promise<{ data: FrontendSessionListResponse }> {
    const response = await http.get<BackendSessionListResponse>('/sessions', { params })
    // 转换后端格式为前端格式。
    // 后端 SessionListResponse 的字段是 sessions/size（历史上这里误配为 items/page_size，
    // 导致 sessions 恒为空数组 → 刷新后会话列表整块消失）。兼容旧字段名做兜底。
    const d = (response?.data ?? {}) as Partial<BackendSessionListResponse> & { items?: Session[]; page_size?: number }
    return {
      data: {
        sessions: d.sessions ?? d.items ?? [],
        total: d.total ?? 0,
        page: d.page ?? 1,
        size: d.size ?? d.page_size ?? 20
      }
    }
  },

  /**
   * 更新会话
   */
  update(id: string, data: UpdateSessionRequest) {
    return http.put<Session>(`/sessions/${id}`, data)
  },

  /**
   * 删除会话
   */
  delete(id: string) {
    return http.delete(`/sessions/${id}`)
  },

  /**
   * 归档会话
   */
  archive(id: string) {
    return http.post(`/sessions/${id}/archive`)
  },

  /**
   * 激活会话
   */
  activate(id: string) {
    return http.post(`/sessions/${id}/activate`)
  }
}
