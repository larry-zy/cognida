import { http, ApiResponse } from '@/utils/request'
import type {
  GraphData,
  GraphSearchRequest,
  AddNodeRequest,
  AddRelationRequest,
  UpdateNodeRequest,
  UpdateRelationRequest,
  NodeDetailResponse,
  RelationTypeOption,
  GraphStatsResponse
} from '@/types'

/**
 * 图谱相关API
 */
export const graphApi = {
  /**
   * 获取知识库图谱数据
   */
  getGraph(kbId: string): Promise<ApiResponse<GraphData>> {
    return http.get<GraphData>(`/knowledge-bases/${kbId}/graph`)
  },

  /**
   * 获取图谱统计信息（平均度/最大度/孤立节点/连通分量等）
   */
  getStats(kbId: string): Promise<ApiResponse<GraphStatsResponse>> {
    return http.get<GraphStatsResponse>(`/graph/stats`, { params: { kb_id: kbId } })
  },

  /**
   * 搜索节点
   */
  searchNode(kbId: string, data: GraphSearchRequest): Promise<ApiResponse<GraphData>> {
    return http.post<GraphData>(`/knowledge-bases/${kbId}/graph/search`, data)
  },

  /**
   * 获取节点详情
   */
  getNodeDetail(kbId: string, nodeId: string): Promise<ApiResponse<NodeDetailResponse>> {
    return http.get<NodeDetailResponse>(`/knowledge-bases/${kbId}/graph/nodes/${nodeId}`)
  },

  /**
   * 添加节点
   */
  addNode(kbId: string, data: AddNodeRequest): Promise<ApiResponse> {
    return http.post(`/knowledge-bases/${kbId}/graph/nodes`, data)
  },

  /**
   * 添加关系
   */
  addRelation(kbId: string, data: AddRelationRequest): Promise<ApiResponse> {
    return http.post(`/knowledge-bases/${kbId}/graph/relations`, data)
  },

  /**
   * 更新节点
   */
  updateNode(kbId: string, nodeId: string, data: UpdateNodeRequest): Promise<ApiResponse> {
    return http.put(`/knowledge-bases/${kbId}/graph/nodes/${nodeId}`, data)
  },

  /**
   * 更新关系
   */
  updateRelation(kbId: string, relationId: string, data: UpdateRelationRequest): Promise<ApiResponse> {
    return http.put(`/knowledge-bases/${kbId}/graph/relations/${relationId}`, data)
  },

  /**
   * 删除图谱
   */
  deleteGraph(kbId: string): Promise<ApiResponse> {
    return http.delete(`/knowledge-bases/${kbId}/graph`)
  },

  /**
   * 删除节点
   */
  deleteNode(kbId: string, nodeId: string): Promise<ApiResponse> {
    return http.delete(`/knowledge-bases/${kbId}/graph/nodes/${nodeId}`)
  },

  /**
   * 删除关系
   */
  deleteRelation(kbId: string, relationId: string): Promise<ApiResponse> {
    return http.delete(`/knowledge-bases/${kbId}/graph/relations/${relationId}`)
  },

  /**
   * 获取关系类型选项
   */
  getRelationTypes(kbId: string): Promise<ApiResponse<RelationTypeOption[]>> {
    return http.get<RelationTypeOption[]>(`/knowledge-bases/${kbId}/graph/relation-types`)
  }
}

export default graphApi
