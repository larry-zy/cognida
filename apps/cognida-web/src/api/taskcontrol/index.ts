import { http } from '@/utils/request'
import { withIdempotency } from '@/utils/idempotent'
import type { RefID } from '@/api/lifecycle'

export interface TaskBudget {
  max_nodes: number
  max_concurrency: number
  max_attempts: number
  max_calls: number
  max_cost_micros: number
  timeout_seconds: number
}
export interface PlanInput { asset?: RefID; from_node?: string; kind: string }
export interface PlanNode {
  id: string
  capability: string
  capability_version: number
  depends_on: string[]
  inputs: PlanInput[]
  output_kinds: string[]
  tools: string[]
  permissions: string[]
  max_attempts: number
  max_calls_per_attempt: number
  max_cost_micros_per_attempt: number
  timeout_seconds: number
}
export interface PlanSpec { schema_version: number; budget: TaskBudget; nodes: PlanNode[] }
export interface PlanValidation {
  valid: boolean
  execution_enabled: false
  authorization_scope: string
  origin_revalidated: boolean
  topological_order: string[]
  approval_required: string[]
  input_refs: RefID[]
  reserved_attempts: number
  reserved_calls: number
  reserved_cost_micros: number
  reserved_seconds: number
}
export interface Capability {
  id: string
  version: number
  domain: string
  input_kinds: string[]
  output_kinds: string[]
  tools: string[]
  permissions: string[]
  requires_approval: boolean
  execution?: { profile: string; base_calls: number; calls_per_input: number; max_timeout_seconds: number }
}
export interface DraftInput { goal: string; input_refs: RefID[]; budget: TaskBudget }
export interface GovernedTask extends DraftInput {
  id: string
  version: number
  current_plan_version: number
  execution_plan_version?: number
  started_at?: string
  deadline_at?: string
  finished_at?: string
  status: string
  reason?: string
  result_refs: RefID[]
  created_at: string
  updated_at: string
}
export interface SavedPlan { task_id: string; version: number; snapshot: PlanSpec; content_hash: string; created_at: string }
export interface Clarification { id: string; question: string; options?: string[] }
export interface Answer { question_id: string; question: string; answer: string }
export interface Assessment { ready: boolean; questions: Clarification[]; execution_enabled: false }
export interface Proposal {
  task_id: string
  task_version: number
  decision: { kind: 'clarify' | 'proposal' | 'unsupported'; explanation: string; questions?: Clarification[]; plan?: PlanSpec }
  validation?: PlanValidation
  execution_enabled: false
  planning_calls: number
  saved: false
}
export interface TaskSummary {
  task_id: string
  task_version: number
  status: string
  current_plan_version: number
  planned_nodes: number
  children: { id: string; node_id: string; plan_version: number; attempt: number; domain: string; domain_kind?: string; domain_id: string; dispatch_status: string; domain_status: string; receipt_present: boolean }[]
  result_refs: RefID[]
  warnings: string[]
  execution_enabled: boolean
  terminal?: boolean
  can_cancel?: boolean
  nodes?: { node_id: string; capability: string; status: string; attempt: number; error?: string; output_refs: RefID[]; domain_kind?: string; domain_id?: string }[]
  usage?: { reserved_attempts: number; reserved_calls: number; reserved_cost_micros: number }
}
export interface ExecutionReadiness {
  task_id: string
  task_version: number
  plan_version: number
  execution_enabled: boolean
  allowed: boolean
  blockers: { node_id?: string; code: string; message: string }[]
}
const base = '/governed-tasks'
const taskPath = (id: string) => `${base}/${encodeURIComponent(id)}`
export const taskControlApi = {
  capabilities: (signal?: AbortSignal) => http.get<{ capabilities: Capability[]; execution_enabled: boolean }>(`${base}/capabilities`, { signal }),
  list: (cursor = '', signal?: AbortSignal) => http.get<{ items: GovernedTask[]; next_cursor: string }>(base, { params: { cursor, page_size: 20 }, signal }),
  get: (id: string, signal?: AbortSignal) => http.get<GovernedTask>(taskPath(id), { signal }),
  assess: (data: DraftInput, signal?: AbortSignal) => http.post<Assessment>(`${base}/supervisor/assess`, data, { signal }),
  create: (data: DraftInput, signal?: AbortSignal) => withIdempotency(base, data, key => http.post<GovernedTask>(base, data, { headers: { 'Idempotency-Key': key }, signal })),
  propose: (id: string, expectedVersion: number, answers: Answer[], signal?: AbortSignal) => http.post<Proposal>(`${taskPath(id)}/supervisor/propose`, { expected_version: expectedVersion, answers }, { signal }),
  appendPlan: (id: string, expectedVersion: number, plan: PlanSpec, signal?: AbortSignal) => http.post<{ plan: SavedPlan; validation: PlanValidation }>(`${taskPath(id)}/plans`, { expected_version: expectedVersion, plan }, { signal }),
  plan: (id: string, version: number, signal?: AbortSignal) => http.get<SavedPlan>(`${taskPath(id)}/plans/${version}`, { signal }),
  summary: (id: string, signal?: AbortSignal) => http.get<TaskSummary>(`${taskPath(id)}/supervisor/summary`, { signal }),
  readiness: (id: string, signal?: AbortSignal) => http.get<ExecutionReadiness>(`${taskPath(id)}/execution-readiness`, { signal }),
  start: (id: string, expectedVersion: number, planVersion: number, signal?: AbortSignal) => {
    const path = `${taskPath(id)}/start`, data = { expected_version: expectedVersion, plan_version: planVersion }
    return withIdempotency(path, data, key => http.post<GovernedTask>(path, data, { headers: { 'Idempotency-Key': key }, signal }))
  },
  cancel: (id: string, expectedVersion: number, signal?: AbortSignal) => http.post<GovernedTask>(`${taskPath(id)}/cancel`, { expected_version: expectedVersion }, { signal }),
}
