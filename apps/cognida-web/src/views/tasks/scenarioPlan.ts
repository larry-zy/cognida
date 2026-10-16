import type { Capability, NodeParameters, PlanNode, PlanSpec } from '@/api/taskcontrol'
export function hfSFTPlan(capabilities: Capability[], request: NonNullable<NodeParameters['huggingface']>, sft: NonNullable<NodeParameters['sft']>): PlanSpec {
  const ids=['huggingface.import','lifecycle.data-cleaning','lifecycle.data-preparation','governance.sft']
  const names=['import','clean','prepare','sft']
  const inputKinds=['','raw_asset','cleaned_asset','cleaned_asset']
  const nodes:PlanNode[]=ids.map((id,index)=>{
    const cap=capabilities.find(item=>item.id===id&&item.execution)
    if(!cap?.execution)throw new Error('完整组合需要 HF 导入、清洗、准备与 SFT 四项已接通能力。')
    const calls=cap.execution.base_calls+cap.execution.calls_per_input*(index?1:0)
    if(!Number.isSafeInteger(calls)||calls<1)throw new Error('能力预算无效，请重新读取。')
    return {id:names[index]!,capability:cap.id,capability_version:cap.version,depends_on:index?[names[index-1]!]:[],inputs:index?[{from_node:names[index-1]!,kind:inputKinds[index]!}]:[],output_kinds:[...cap.output_kinds],tools:[...cap.tools],permissions:[...cap.permissions],max_attempts:1,max_calls_per_attempt:calls,max_cost_micros_per_attempt:0,timeout_seconds:Math.min(300,cap.execution.max_timeout_seconds),...(index===0?{parameters:{huggingface:request}}:index===3?{parameters:{sft}}:{})}
  })
  return {schema_version:1,budget:{max_nodes:4,max_concurrency:1,max_attempts:4,max_calls:nodes.reduce((total,node)=>total+node.max_calls_per_attempt,0),max_cost_micros:0,timeout_seconds:nodes.reduce((total,node)=>total+node.timeout_seconds,0)},nodes}
}

/** Preserve user-selected fixed fields in the actual model input; never truncate a contract. */
export function scenarioGoal(summary: string, parameters: NodeParameters, chain = false): string {
  const instruction = chain
    ? '按 huggingface.import → lifecycle.data-cleaning → lifecycle.data-preparation → governance.sft 依次处理，每节点仅尝试一次；后续节点使用前节点产物。仅构建 SFT 草稿，不自动审核、发布或训练。'
    : '仅执行所选能力，保持以下固定参数；SFT 只构建草稿，不自动审核、发布或训练。'
  const goal = `${summary}\n${instruction}\n固定参数：${JSON.stringify(parameters)}`
  if ([...goal].length > 4000) throw new Error('固定参数与目标超过 4,000 字符，请精简名称或字段映射后重新预览；不会截断来源约束。')
  return goal
}
