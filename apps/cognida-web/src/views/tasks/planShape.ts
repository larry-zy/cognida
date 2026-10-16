import type { PlanSpec } from '@/api/taskcontrol'

const record = (value: unknown): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value)
const strings = (value: unknown): value is string[] => Array.isArray(value) && value.every(item => typeof item === 'string')
const integer = (value: unknown): value is number => typeof value === 'number' && Number.isSafeInteger(value) && value >= 0

// This guards rendering an untrusted local JSON draft. Execution authorization,
// dependency semantics and capability limits remain server-side validations.
export function isRenderablePlan(value: unknown): value is PlanSpec {
  if (!record(value) || value.schema_version !== 1 || !record(value.budget)) return false
  if (!['max_nodes', 'max_concurrency', 'max_attempts', 'max_calls', 'max_cost_micros', 'timeout_seconds'].every(key => integer(value.budget && (value.budget as Record<string, unknown>)[key]))) return false
  if (!Array.isArray(value.nodes) || value.nodes.length < 1 || value.nodes.length > 32) return false
  return value.nodes.every(node => {
    if (!record(node) || typeof node.id !== 'string' || typeof node.capability !== 'string') return false
    if (!['capability_version', 'max_attempts', 'max_calls_per_attempt', 'max_cost_micros_per_attempt', 'timeout_seconds'].every(key => integer(node[key]))) return false
    if (!['depends_on', 'output_kinds', 'tools', 'permissions'].every(key => strings(node[key]))) return false
    if (!Array.isArray(node.inputs) || !node.inputs.every(input => record(input) && typeof input.kind === 'string' &&
      (input.from_node === undefined || typeof input.from_node === 'string') &&
      (input.asset === undefined || (record(input.asset) && typeof input.asset.asset_id === 'string' && integer(input.asset.version))))) return false
    if (node.parameters === undefined) return true
    if (!record(node.parameters) || !Object.values(node.parameters).every(record)) return false
    const hf=node.parameters.huggingface
    if(hf!==undefined){
      if(!record(hf)||!record(hf.request))return false
      const request=hf.request
      if(!['repo_id','revision','config','split'].every(key=>typeof request[key]==='string')||!strings(request.files)||!integer(request.max_rows)||!integer(request.max_bytes)||!record(request.field_mapping))return false
      if(request.field_mapping.columns!==undefined&&(!Array.isArray(request.field_mapping.columns)||!request.field_mapping.columns.every(column=>record(column)&&typeof column.source==='string'&&typeof column.target==='string')))return false
    }
    const sft=node.parameters.sft
    if(sft!==undefined&&(!record(sft)||typeof sft.name!=='string'||typeof sft.standard_version_id!=='string'||typeof sft.format!=='string'||!record(sft.columns)||!Object.values(sft.columns).every(value=>typeof value==='string')))return false
    const governance = node.parameters.governance
    if (!governance) return true
    if (!record(governance)) return false
    if (governance.labels !== undefined && !strings(governance.labels)) return false
    if (governance.qa_mapping !== undefined && (!record(governance.qa_mapping) || typeof governance.qa_mapping.question_column !== 'string' || typeof governance.qa_mapping.reference_answer_column !== 'string')) return false
    if (governance.label_schema !== undefined && (!Array.isArray(governance.label_schema) || !governance.label_schema.every(label => record(label) && typeof label.id === 'string' && typeof label.name === 'string' &&
      (label.keywords === undefined || strings(label.keywords)) && (label.terms === undefined || strings(label.terms))))) return false
    return true
  })
}
