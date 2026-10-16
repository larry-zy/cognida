import type {AgentStep,RAGContext} from '@/types'
export interface KnowledgeSource {chunk_id:string;knowledge_id?:string;knowledge_base_id?:string;source:string;content:string;score:number}
const record=(value:unknown):value is Record<string,unknown>=>Boolean(value)&&typeof value==='object'&&!Array.isArray(value)
function chunk(value:unknown,requireScope:boolean):KnowledgeSource|undefined {
 if(!record(value)||typeof value.chunk_id!=='string'||!value.chunk_id||typeof value.content!=='string'||!value.content||typeof value.score!=='number'||!Number.isFinite(value.score))return
 if(requireScope&&(typeof value.knowledge_id!=='string'||!value.knowledge_id||typeof value.knowledge_base_id!=='string'||!value.knowledge_base_id))return
 return {chunk_id:value.chunk_id,content:value.content,score:value.score,source:typeof value.source==='string'?value.source:'',...(typeof value.knowledge_id==='string'?{knowledge_id:value.knowledge_id}:{}),...(typeof value.knowledge_base_id==='string'?{knowledge_base_id:value.knowledge_base_id}:{})}
}
/** Only structured, successful retrieval tool receipts or the legacy API context.
 * Never interpret answer Markdown, thoughts or a model-provided citation as evidence. */
export function knowledgeSources(steps:AgentStep[]=[],legacy?:RAGContext):KnowledgeSource[]{
 const result=new Map<string,KnowledgeSource>()
 const add=(source:KnowledgeSource|undefined)=>{if(source){const key=`${source.knowledge_base_id||''}:${source.knowledge_id||''}:${source.chunk_id}`;if(!result.has(key))result.set(key,source)}}
 for(const step of steps){
  if(step.type!=='tool_result'||step.tool_name!=='rag_query'||step.status!=='success'||step.error||typeof step.tool_output!=='string')continue
  try{const output:unknown=JSON.parse(step.tool_output);if(record(output)&&Array.isArray(output.chunks))for(const value of output.chunks)add(chunk(value,true))}catch{/* An invalid receipt remains visible in the tool timeline; it cannot become a citation. */}
 }
 // Legacy context is used only when no structured receipt is present, avoiding
 // duplicate cards that differ solely because old context lacked document IDs.
 if(result.size===0&&Array.isArray(legacy?.contexts_with_score))for(const value of legacy.contexts_with_score)add(chunk(value,false))
 return [...result.values()]
}
