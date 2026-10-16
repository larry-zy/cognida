import { http } from '@/utils/request'
import { withIdempotency } from '@/utils/idempotent'
export interface EvaluationProfile {
  kind: 'qa' | 'rag' | 'agent' | 'sql'
  source_refs?: string[]
  relevant_chunk_ids?: string[]
  expected_tools?: string[]
  expected_steps?: string[]
  datasource_id?: string
  dialect?: string
  gold_sql?: string
}
export interface DatasetSplit {
  algorithm: 'none' | 'source_hash_v1' | 'source_time_v1'
  seed?: string
  train_percent?: number
  validation_percent?: number
  train_before?: string
  validation_before?: string
  near_duplicate_threshold?: number
}
export interface DatasetVersion {
  id: string; dataset_id: string; version: number; name: string; profile: string
  manifest_digest: string; created_at: string; source_refs: string[]
  samples: { id: string; review_version: number; sample_hash: string; source_ref: string; split: string }[]
  qa_pairs?: { question: string; reference_answer: string }[]
  split: DatasetSplit
}
export interface PublishVersion {
  dataset_id: string; name: string; profile: string
  samples: { task_id: string; item_id: string; expected_version: number }[]
  split: DatasetSplit
}
export const datasetVersionsApi = {
  publish: (input: PublishVersion) => withIdempotency('dataset-version', input, key => http.post<DatasetVersion>('/evaluation/dataset-versions', input, { headers: { 'Idempotency-Key': key } })),
  list: (cursor = '', dataset_id = '') => http.get<{ items: DatasetVersion[]; next_cursor: string; has_more: boolean }>('/evaluation/dataset-versions', { params: { cursor, dataset_id, page_size: 50 } }),
  read: (id: string) => http.get<DatasetVersion>(`/evaluation/dataset-versions/${encodeURIComponent(id)}`),
}
