import { http } from '@/utils/request'

/**
 * Collection is deliberately a separate bounded context from the interactive
 * Data Agent.  Keep its transport types here so collection changes do not
 * leak into the shared chat/agent models.
 */
export type CollectionTaskStatus = 'draft' | 'running' | 'completed' | 'failed' | 'cancelled'
export type CollectionItemStatus = 'pending' | 'running' | 'accepted' | 'skipped' | 'rejected' | 'failed' | 'cancelled'

export interface CreateCollectionTaskRequest {
  knowledge_base_id: string
  sources: string[]
  max_response_bytes?: number
  /** Optional client supplied key for a safe retry of create. */
  idempotency_key?: string
}

export interface CollectionItemOutcome {
  id: string
  source_url: string
  canonical_url?: string
  status: CollectionItemStatus
  error_reason?: string
  content_hash?: string
  knowledge_id?: string
  created_at?: string
  updated_at?: string
}

export interface CollectionRun {
  id: string
  status: CollectionTaskStatus
  started_at?: string
  finished_at?: string
  created_at?: string
}

export interface CollectionTask {
  id: string
  knowledge_base_id: string
  status: CollectionTaskStatus
  seed_urls?: string[]
  source_count?: number
  completed_count?: number
  failed_count?: number
  cancelled_at?: string
  created_at?: string
  updated_at?: string
}

export interface CollectionTaskDetail extends CollectionTask {
  runs?: CollectionRun[]
  items?: CollectionItemOutcome[]
}

export interface CollectionTaskDetailResponse {
  task: CollectionTask
  sources: Array<{ id: string, url: string, created_at?: string }>
  run?: CollectionRun
  items?: CollectionItemOutcome[]
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

  getById(id: string) {
    return http.get<CollectionTaskDetailResponse>(`/collection-tasks/${id}`)
  },

  start(id: string) {
    return http.post<CollectionTask>(`/collection-tasks/${id}/start`)
  },

  cancel(id: string) {
    return http.post<CollectionTask>(`/collection-tasks/${id}/cancel`)
  }
}
