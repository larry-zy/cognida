import { http } from '@/utils/request'
import { withIdempotency } from '@/utils/idempotent'
import type { RefID } from '@/api/lifecycle'
export interface APICredential { id: string; name: string; origin: string; kind: 'bearer' | 'api_key'; enabled: boolean }
export interface SourceConfig {
  url: string; credential_id: string; items_pointer: string; record_key: string
  watermark_field?: string; watermark_parameter?: string; deleted_field?: string
  pagination: { mode: 'none' | 'page' | 'cursor'; parameter?: string; page_size_parameter?: string; page_size?: number; next_pointer?: string; snapshot_pointer?: string; snapshot_parameter?: string }
  limits: { max_pages: number; max_records: number; max_bytes: number; max_page_bytes: number; max_requests: number; timeout_seconds: number }
}
export interface APISource { id: string; name: string; version: number; enabled: boolean; config: SourceConfig; watermark?: string; active_run?: string }
export interface APIIngestionRun { id: string; source_id: string; source_version: number; version: number; status: string; error?: string; consistency: string; snapshot_digest?: string; pages: { number: number; digest: string; bytes: number; records: number; acquired_at: string }[]; record_count: number; duplicate_count: number; deleted_count: number; bytes: number; requests: number; watermark_before?: string; watermark_after?: string; output_refs: RefID[]; retryable: boolean; deadline: string }
interface Page<T> { items: T[]; next_cursor?: string }
const encoded = encodeURIComponent
const write = <T>(path: string, data: unknown, signal?: AbortSignal) => withIdempotency(path, data, key => http.post<T>(path, data, { signal, headers: { 'Idempotency-Key': key } }))
export const apiSourceApi = {
  credentials: (cursor = '', signal?: AbortSignal) => http.get<Page<APICredential>>('/api-credentials', { params: { cursor, limit: 50 }, signal }),
  createCredential: async (data: { name: string; origin: string; kind: string; secret: string }, signal?: AbortSignal) => {
    // The shared retry-key cache must never retain credential plaintext.
    const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(JSON.stringify(data)))
    const fingerprint = Array.from(new Uint8Array(digest), b => b.toString(16).padStart(2, '0')).join('')
    return withIdempotency('/api-credentials', { fingerprint }, key => http.post<APICredential>('/api-credentials', data, { signal, headers: { 'Idempotency-Key': key } }))
  },
  disableCredential: (id: string, signal?: AbortSignal) => write(`/api-credentials/${encoded(id)}/disable`, {}, signal),
  sources: (cursor = '', signal?: AbortSignal) => http.get<Page<APISource>>('/api-sources', { params: { cursor, limit: 50 }, signal }),
  create: (data: { name: string; config: SourceConfig }, signal?: AbortSignal) => write<APISource>('/api-sources', data, signal),
  update: (id: string, data: { name: string; expected_version: number; config: SourceConfig }, signal?: AbortSignal) => http.put<APISource>(`/api-sources/${encoded(id)}`, data, { signal }),
  enabled: (id: string, version: number, enabled: boolean, signal?: AbortSignal) => write<APISource>(`/api-sources/${encoded(id)}/enabled`, { expected_version: version, enabled }, signal),
  start: (id: string, version: number, key: string, signal?: AbortSignal) => http.post<APIIngestionRun>(`/api-sources/${encoded(id)}/runs`, { version }, { headers: { 'Idempotency-Key': key }, signal }),
  run: (id: string, signal?: AbortSignal) => http.get<APIIngestionRun>(`/api-ingestion-runs/${encoded(id)}`, { signal }),
  cancel: (id: string, signal?: AbortSignal) => write<APIIngestionRun>(`/api-ingestion-runs/${encoded(id)}/cancel`, {}, signal),
  resume: (id: string, signal?: AbortSignal) => write<APIIngestionRun>(`/api-ingestion-runs/${encoded(id)}/resume`, {}, signal),
}
