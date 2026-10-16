import { http } from '@/utils/request'
import type { RefID } from '@/api/lifecycle'
export interface Cell { type: 'text' | 'number' | 'boolean' | 'null'; text: string }
export interface OperationRequest { action: 'etl' | 'insert' | 'update' | 'delete' | 'rollback'; target: string; expected_version: number; input_ref?: RefID; values?: Record<string, Cell>; where?: Record<string, Cell>; rollback_version?: number }
export interface ManagedTable { columns: string[]; types: Record<string, string>; rows: Cell[][] }
export interface OperationJob { id: string; status: string; version: number; plan: { request: OperationRequest; hash: string; sql: string; parameters: Cell[]; before_version_id: string; after: ManagedTable; after_digest: string; estimated_affected: number; source_refs: RefID[] }; error?: string; expires_at: string; confirmed_by?: number; committed_version?: number; affected_rows: number; output_ref?: RefID; created_at: string }
export interface OperationTarget { id: string; name: string; version: number; active_version_id: string }
export interface OperationVersion { id: string; version: number; operation_id: string; table: ManagedTable; digest: string; source_refs: RefID[] }
const path = (id: string) => `/operations/jobs/${encodeURIComponent(id)}`
export const operationApi = {
  targets: (signal?: AbortSignal) => http.get<{ items: OperationTarget[] }>('/operations/targets', { signal }),
  list: (cursor = '', signal?: AbortSignal) => http.get<{ items: OperationJob[]; next_cursor?: string }>('/operations/jobs', { params: { cursor }, signal }),
  get: (id: string, signal?: AbortSignal) => http.get<OperationJob>(path(id), { signal }),
  prepare: (input: OperationRequest, key: string, signal?: AbortSignal) => http.post<OperationJob>('/operations/plans', input, { signal, headers: { 'Idempotency-Key': key } }),
  confirm: (job: OperationJob, signal?: AbortSignal) => http.post<OperationJob>(`${path(job.id)}/confirm`, { expected_version: job.version, plan_hash: job.plan.hash, action: job.plan.request.action }, { signal, headers: { 'Idempotency-Key': `confirm:${job.id}:${job.plan.hash}` } }),
  cancel: (id: string, signal?: AbortSignal) => http.post<OperationJob>(`${path(id)}/cancel`, {}, { signal, headers: { 'Idempotency-Key': `cancel:${id}` } }),
  version: (name: string, version: number, signal?: AbortSignal) => http.get<OperationVersion>(`/operations/targets/${encodeURIComponent(name)}/versions/${version}`, { signal }),
}
