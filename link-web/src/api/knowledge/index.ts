import { http } from '@/utils/request'
import type {
  KnowledgeBase,
  CreateKnowledgeBaseRequest,
  UpdateKnowledgeBaseRequest,
  KnowledgeBaseStats,
  Knowledge,
  KnowledgeStatus,
  Chunk,
  ChunkListResponse,
  SearchRequest,
  SearchResponse
} from '@/types'

/**
 * 知识库相关API
 */
export const knowledgeApi = {
  /**
   * 创建知识库
   */
  create(data: CreateKnowledgeBaseRequest) {
    return http.post<KnowledgeBase>('/knowledge-bases', data)
  },

  /**
   * 获取知识库列表
   */
  getList() {
    return http.get<KnowledgeBase[]>('/knowledge-bases')
  },

  /**
   * 获取知识库详情
   */
  getDetail(id: string) {
    return http.get<KnowledgeBase>(`/knowledge-bases/${id}`)
  },

  /**
   * 更新知识库
   */
  update(id: string, data: UpdateKnowledgeBaseRequest) {
    return http.put<KnowledgeBase>(`/knowledge-bases/${id}`, data)
  },

  /**
   * 删除知识库
   */
  delete(id: string) {
    return http.delete(`/knowledge-bases/${id}`)
  },

  /**
   * 获取知识库统计信息
   */
  getStats(id: string) {
    return http.get<KnowledgeBaseStats>(`/knowledge-bases/${id}/stats`)
  },

  /**
   * 上传知识库文件
   * @param onProgress 上传进度回调（0-100）
   */
  uploadFile(
    kbId: string,
    formData: FormData,
    onProgress?: (percent: number) => void
  ) {
    return http.post<{ knowledge_id: string; status: string; title?: string }>(
      `/knowledge-bases/${kbId}/knowledge/file`,
      formData,
      {
        headers: {
          'Content-Type': 'multipart/form-data'
        },
        onUploadProgress: (e) => {
          if (!onProgress || !e.total) return
          onProgress(Math.round((e.loaded / e.total) * 100))
        }
      }
    )
  },

  /**
   * 上传前预检：文件是否已存在于知识库（防止重传相同文件）
   */
  checkFile(kbId: string, fileHash: string) {
    return http.post<{ duplicate: boolean; knowledge_id?: string; title?: string }>(
      `/knowledge-bases/${kbId}/knowledge/check`,
      { file_hash: fileHash }
    )
  },

  /**
   * 获取知识库文档列表
   */
  getKnowledgeList(kbId: string, params?: { page?: number; page_size?: number; status?: string }) {
    return http.get<Knowledge[]>(`/knowledge-bases/${kbId}/knowledge`, { params })
  },

  /**
   * 获取单个文档详情
   */
  getKnowledge(kbId: string, knowledgeId: string) {
    return http.get<Knowledge>(`/knowledge-bases/${kbId}/knowledge/${knowledgeId}`)
  },

  /**
   * 删除文档
   */
  deleteKnowledge(kbId: string, knowledgeId: string) {
    return http.delete(`/knowledge-bases/${kbId}/knowledge/${knowledgeId}`)
  },

  /**
   * 获取文档处理状态
   */
  getKnowledgeStatus(kbId: string, knowledgeId: string) {
    return http.get<KnowledgeStatus>(`/knowledge-bases/${kbId}/knowledge/${knowledgeId}/status`)
  },

  /**
   * 获取分块列表
   */
  getChunks(kbId: string, params?: { page?: number; size?: number; knowledge_id?: string }) {
    return http.get<ChunkListResponse>(`/knowledge-bases/${kbId}/chunks`, { params })
  },

  /**
   * 获取单个分块详情
   */
  getChunk(kbId: string, chunkId: string) {
    return http.get<Chunk>(`/knowledge-bases/${kbId}/chunks/${chunkId}`)
  },

  /**
   * 更新分块
   */
  updateChunk(kbId: string, chunkId: string, data: { content?: string; tags?: string[] }) {
    return http.put<Chunk>(`/knowledge-bases/${kbId}/chunks/${chunkId}`, data)
  },

  /**
   * 删除分块
   */
  deleteChunk(kbId: string, chunkId: string) {
    return http.delete(`/knowledge-bases/${kbId}/chunks/${chunkId}`)
  },

  /**
   * 搜索知识库
   */
  search(data: SearchRequest) {
    return http.post<SearchResponse>('/knowledge/search', data)
  }
}

export default knowledgeApi
