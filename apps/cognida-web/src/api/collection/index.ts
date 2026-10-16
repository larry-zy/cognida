import { http } from '@/utils/request'

/**
 * Collection is deliberately a separate bounded context from the interactive
 * Data Agent.  Keep its transport types here so collection changes do not
 * leak into the shared chat/agent models.
 */
export type CollectionTaskStatus = 'draft' | 'running' | 'completed' | 'failed' | 'cancelled'
export type CollectionItemStatus = 'pending' | 'running' | 'candidate' | 'accepted' | 'skipped' | 'rejected' | 'failed' | 'cancelled'

export interface CreateCollectionTaskRequest {
  knowledge_base_id: string
  sources: string[]
  max_response_bytes?: number
  /** Optional client supplied key for a safe retry of create. */
  idempotency_key?: string
}

export interface CollectionItemOutcome {
  id: string
  run_id: string
  source_id: string
  source_url: string
  canonical_url?: string
  status: CollectionItemStatus
  error_reason?: string
  content_hash?: string
  asset_ref_id?: string
  knowledge_id?: string
  raw_object_key?: string
  normalized_object_key?: string
  raw_hash?: string
  raw_size?: number
  normalized_size?: number
  http_status?: number
  content_type?: string
  created_at?: string
  updated_at?: string
}

export interface CollectionRun {
  id: string
  status: Exclude<CollectionTaskStatus, 'draft'>
  error_message?: string
  started_at?: string
  completed_at?: string
  created_at?: string
}

export interface CollectionTask {
  mode?: string
  id: string
  knowledge_base_id: string
  status: CollectionTaskStatus
  error_message?: string
  created_at?: string
  updated_at?: string
}

export interface CollectionTaskDetailResponse {
  task: CollectionTask
  sources: Array<{ id: string, url: string, created_at?: string }>
  run?: CollectionRun
  items?: CollectionItemOutcome[]
  summary?: {
    outcome: 'success' | 'partial' | 'failed' | 'running' | 'cancelled'
    total: number
    accepted: number
    candidates?: number
    skipped: number
    rejected: number
    failed: number
    pending: number
    usable: number
  }
}

export interface CollectionTaskListResponse {
  tasks: CollectionTask[]
  total?: number
}

/** HTTP client for the governed, URL-only collection workflow. */
export const collectionApi = {
  create(data: CreateCollectionTaskRequest) {
    const { idempotency_key, ...body } = data
    return http.post<CollectionTask>('/collection-tasks', body, {
      headers: { 'Idempotency-Key': idempotency_key || crypto.randomUUID() }
    })
  },

  list(params?: { page?: number; page_size?: number; status?: CollectionTaskStatus }) {
    return http.get<CollectionTaskListResponse>('/collection-tasks', { params })
  },

  getRun(id: string) { return http.get<CollectionTaskDetailResponse>(`/collection-runs/${encodeURIComponent(id)}`) },

  getById(id: string) {
    return http.get<CollectionTaskDetailResponse>(`/collection-tasks/${id}`)
  },

  start(id: string) {
    return http.post<{ message: string }>(`/collection-tasks/${id}/start`)
  },

  cancel(id: string) {
    return http.post<{ message: string }>(`/collection-tasks/${id}/cancel`)
  }
}
