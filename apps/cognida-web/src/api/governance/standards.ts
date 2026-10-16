import { http } from '@/utils/request'
import { withIdempotency } from '@/utils/idempotent'
import type { Page } from './index'

export type StandardTaskType = 'classification' | 'named_entity' | 'relation' | 'qa' | 'tagging'
export interface StandardLabel {
  id: string; name: string; definition: string
  include_criteria: string[]; exclude_criteria: string[]
  keywords: string[]; terms: string[]; exclude_keywords: string[]
  examples: { text: string; decision: 'include' | 'exclude'; explanation: string }[]
}
export interface StandardInput {
  name: string; task_type: StandardTaskType; guidelines: string; labels: StandardLabel[]
  constraints: { max_labels: number; max_entities: number; max_relations: number; allow_empty: boolean }
}
export interface LabelStandard extends StandardInput {
  id: string; version: number; status: string; latest_version_id?: string; created_at?: string; updated_at?: string
}
export interface StandardVersion extends StandardInput {
  id: string; version: number; standard_id: string; standard_revision: number; status: string; content_digest: string; created_at?: string
}
export const standardTypeLabels: Record<StandardTaskType, string> = {
  classification: '分类', named_entity: '命名实体', relation: '关系', qa: '问答', tagging: '知识打标',
}
export const standardsApi = {
  list: (cursor = '') => http.get<Page<LabelStandard>>('/label-standards', { params: { cursor, page_size: 50 } }),
  get: (id: string) => http.get<LabelStandard>(`/label-standards/${encodeURIComponent(id)}`),
  create: (standard: StandardInput) => withIdempotency('label-standard', standard, key => http.post<LabelStandard>('/label-standards', standard, { headers: { 'Idempotency-Key': key } })),
  update: (id: string, expected_version: number, standard: StandardInput) => withIdempotency(`label-standard/${id}/update`, { expected_version, standard }, key => http.put<LabelStandard>(`/label-standards/${encodeURIComponent(id)}`, { expected_version, standard }, { headers: { 'Idempotency-Key': key } })),
  publish: (id: string, expected_version: number) => withIdempotency(`label-standard/${id}/publish`, { expected_version }, key => http.post<StandardVersion>(`/label-standards/${encodeURIComponent(id)}/publish`, { expected_version }, { headers: { 'Idempotency-Key': key } })),
  versions: (id: string, cursor = '') => http.get<Page<StandardVersion>>(`/label-standards/${encodeURIComponent(id)}/versions`, { params: { cursor, page_size: 50 } }),
  version: (id: string) => http.get<StandardVersion>(`/label-standard-versions/${encodeURIComponent(id)}`),
}
