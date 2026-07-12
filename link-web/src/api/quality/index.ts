import { http } from '@/utils/request'

// ==================== 类型定义 ====================

/** 质检类型 */
export type QualityCheckType = 'structured' | 'unstructured' | 'cleaning'

/** 结构化评估 - 质量问题 */
export interface QualityIssue {
  severity: string
  dimension: string
  field: string
  description: string
  count: number
  sample?: string[]
}

/** 结构化评估 - 单维度得分 */
export interface DimensionScore {
  name: string
  score: number
  passed: boolean
  issues?: QualityIssue[]
}

/** 结构化数据质量报告 */
export interface StructuredReport {
  overall_score: number
  record_count: number
  dimensions: DimensionScore[]
  issues: QualityIssue[]
  metadata?: Record<string, string>
}

/** 文本质量问题 */
export interface TextIssue {
  type: string
  severity: string
  description: string
  snippet?: string
  suggestion?: string
}

/** 非结构化文本单维度得分 */
export interface UnstructuredDimensionScore {
  name: string
  score: number
  passed: boolean
}

/** 非结构化文本质量报告 */
export interface UnstructuredReport {
  overall_score: number
  dimensions: UnstructuredDimensionScore[]
  text_stats?: Record<string, string>
  issues: TextIssue[]
}

/** 单次清洗操作 */
export interface CleaningOperation {
  type: string
  field: string
  count: number
  description: string
}

/** 数据清洗结果 */
export interface CleaningReport {
  original_count: number
  cleaned_count: number
  removed_count: number
  operations: CleaningOperation[]
  cleaned_csv?: string
}

/** 支持的质量维度说明 */
export interface DimensionInfo {
  name: string
  description: string
  supports_structured: boolean
  supports_unstructured: boolean
}

/** 历史质检记录（列表项，不含完整报告） */
export interface CheckRecordItem {
  id: string
  type: QualityCheckType
  source_name: string
  overall_score: number
  record_count: number
  summary: string
  created_at: string
}

/** 历史质检记录详情（含完整报告快照） */
export interface CheckRecordDetail extends CheckRecordItem {
  report: StructuredReport | UnstructuredReport | CleaningReport | null
}

/** 分页返回 */
export interface PageResult<T> {
  total: number
  items: T[]
  page: number
  size: number
}

// ==================== 请求参数 ====================

export interface StructuredRequest {
  /** 结构化数据文本；format=csv 时为 CSV，format=json 时为对象数组 JSON */
  csv_data: string
  source_name?: string
  dimensions?: string[]
  /** 输入格式，默认 csv；json 表示 csv_data 内是对象数组 */
  format?: 'csv' | 'json'
}

export interface UnstructuredRequest {
  text: string
  source_name?: string
  dimensions?: string[]
}

export interface CleanRequest {
  csv_data: string
  source_name?: string
}

export interface DatasourceEvaluateRequest {
  datasource_id: string
  table: string
  sample_size?: number
  dimensions?: string[]
}

export interface RecordListParams {
  page?: number
  page_size?: number
  type?: QualityCheckType | ''
}

// ==================== API ====================

/**
 * 数据质量管理中心相关 API
 */
export const qualityApi = {
  /** 结构化数据质量评估（粘贴 CSV 文本） */
  evaluateStructured(data: StructuredRequest) {
    return http.post<StructuredReport>('/quality/evaluate/structured', data)
  },

  /** 结构化数据质量评估（上传 CSV 文件） */
  evaluateStructuredFile(formData: FormData) {
    return http.post<StructuredReport>('/quality/evaluate/structured', formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
  },

  /** 数据源直评：从外部数据源指定表抽样后评估结构化质量（取数在 Go 侧完成） */
  evaluateDatasource(data: DatasourceEvaluateRequest) {
    return http.post<StructuredReport>('/quality/evaluate/datasource', data)
  },

  /** 非结构化文本质量评估 */
  evaluateUnstructured(data: UnstructuredRequest) {
    return http.post<UnstructuredReport>('/quality/evaluate/unstructured', data)
  },

  /** 数据清洗（粘贴 CSV 文本）。cleaners 通过 query 传递，与后端 handler 约定一致。 */
  clean(data: CleanRequest, cleaners?: string[]) {
    const config = cleaners && cleaners.length
      ? { params: { cleaners: cleaners.join(',') } }
      : undefined
    return http.post<CleaningReport>('/quality/clean', data, config)
  },

  /** 数据清洗（上传 CSV 文件，cleaners 走 PostForm） */
  cleanFile(formData: FormData) {
    return http.post<CleaningReport>('/quality/clean', formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
  },

  /** 支持的质量维度 */
  listDimensions() {
    return http.get<{ dimensions: DimensionInfo[] }>('/quality/dimensions')
  },

  /** 历史质检记录列表 */
  listRecords(params?: RecordListParams) {
    return http.get<PageResult<CheckRecordItem>>('/quality/records', { params })
  },

  /** 历史质检记录详情（含完整报告） */
  getRecord(id: string) {
    return http.get<CheckRecordDetail>(`/quality/records/${id}`)
  },

  /** 删除历史质检记录 */
  deleteRecord(id: string) {
    return http.delete(`/quality/records/${id}`)
  }
}

export default qualityApi
