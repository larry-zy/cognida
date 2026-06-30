import { http } from '@/utils/request'
import type {
  EvaluationTask,
  EvaluationDetail,
  CreateEvaluationRequest,
  CreateDatasetRequest,
  UpdateDatasetRequest,
  AddSamplesRequest,
  DatasetInfo,
  DatasetListResponse,
  SampleListResponse,
  QAResult
} from '@/types'

export const evaluationApi = {
  /**
   * 创建测评任务
   */
  create(data: CreateEvaluationRequest) {
    return http.post<EvaluationDetail>('/evaluation/tasks', data)
  },

  /**
   * 获取测评结果
   */
  getResult(taskId: string) {
    return http.get<EvaluationDetail>(`/evaluation/results/${taskId}`)
  },

  /**
   * 列出测评任务
   */
  list(params?: { page?: number; page_size?: number }) {
    return http.get<DatasetListResponse>('/evaluation/tasks', { params })
  },

  /**
   * 获取单个测评任务
   */
  getById(id: string) {
    return http.get<EvaluationTask>(`/evaluation/tasks/${id}`)
  },

  /**
   * 删除测评任务
   */
  delete(id: string) {
    return http.delete(`/evaluation/tasks/${id}`)
  },

  /**
   * 流式获取任务进度 (SSE)
   */
  streamTask(taskId: string): string {
    // 返回 SSE URL
    return `/api/v1/evaluation/tasks/${taskId}/stream`
  },

  /**
   * 获取 QA 级别结果
   */
  getQAResults(taskId: string, params?: { page?: number; page_size?: number }) {
    return http.get<{ results: QAResult[]; total: number }>(
      `/evaluation/results/${taskId}/qa`,
      { params }
    )
  },

  /**
   * 获取任务详情（包含 QA 结果）
   */
  getTaskDetail(taskId: string) {
    return http.get<EvaluationDetail>(`/evaluation/tasks/${taskId}`)
  },

  // ============ 数据集相关 ============

  /**
   * 列出数据集
   */
  listDatasets(params?: { type?: string }) {
    return http.get<{ datasets: DatasetInfo[] }>('/evaluation/datasets', { params })
  },

  /**
   * 获取数据集详情
   */
  getDataset(id: string) {
    return http.get<DatasetInfo>(`/evaluation/datasets/${id}`)
  },

  /**
   * 创建数据库数据集
   */
  createDataset(data: CreateDatasetRequest) {
    return http.post<DatasetInfo>('/evaluation/datasets', data)
  },

  /**
   * 更新数据集
   */
  updateDataset(id: string, data: UpdateDatasetRequest) {
    return http.put<DatasetInfo>(`/evaluation/datasets/${id}`, data)
  },

  /**
   * 删除数据集
   */
  deleteDataset(id: string) {
    return http.delete(`/evaluation/datasets/${id}`)
  },

  /**
   * 获取数据集样本列表
   */
  listSamples(id: string, params?: { page?: number; page_size?: number }) {
    return http.get<SampleListResponse>(`/evaluation/datasets/${id}/samples`, { params })
  },

  /**
   * 添加样本
   */
  addSamples(id: string, data: AddSamplesRequest) {
    return http.post(`/evaluation/datasets/${id}/samples`, data)
  },

  /**
   * 删除样本
   */
  deleteSample(datasetId: string, sampleId: number) {
    return http.delete(`/evaluation/datasets/${datasetId}/samples/${sampleId}`)
  },

  /**
   * 预览数据集样本（文件系统数据集）
   */
  previewDataset(id: string, limit: number = 5) {
    return http.get<{ qapairs: any[] }>(`/evaluation/datasets/${id}/preview?limit=${limit}`)
  }
}
