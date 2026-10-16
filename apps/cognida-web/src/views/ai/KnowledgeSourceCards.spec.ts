import {describe,it,expect} from 'vitest'
import {mount} from '@vue/test-utils'
import type {AgentStep,RAGContext} from '@/types'
import {parseAgentSteps} from '@/composables/useAgentChat'
import KnowledgeSourceCards from './KnowledgeSourceCards.vue'
import {knowledgeSources} from './knowledgeSources'
const chunk={chunk_id:'c1',knowledge_id:'doc',knowledge_base_id:'kb',content:'ODBCINI requires an explicit environment setting.',source:'IBM source',score:0.82}
const step=(changes:Partial<AgentStep>={}):AgentStep=>({id:'call1',step:1,type:'tool_result',tool_id:'call1',tool_name:'rag_query',status:'success',tool_output:JSON.stringify({chunks:[chunk]}),timestamp:0,...changes})
const mountCards=(props:any)=>mount(KnowledgeSourceCards,{props,global:{stubs:{RouterLink:{props:['to'],template:'<a><slot /></a>'}}}})
describe('real knowledge retrieval receipts',()=>{
 it('renders deduplicated actual retrieved chunks without claiming answer verification',()=>{const w=mountCards({steps:[step(),step({id:'call2'})]});expect(w.findAll('details')).toHaveLength(1);expect(w.text()).toContain('ODBCINI');expect(w.text()).toContain('检索分数 0.820');expect(w.text()).toContain('不代表回答中的每项结论都已核验');expect(w.text()).not.toContain('已按相关度重排');expect(w.find('a').exists()).toBe(true)})
 it('restores cards from persisted successful tool steps, not assistant text',()=>{const steps=parseAgentSteps(JSON.stringify([{...step(),type:'tool_call',tool_output:undefined},{...step(),type:'tool_result'}]));expect(steps).toHaveLength(1);const w=mountCards({steps});expect(w.text()).toContain('IBM source');expect(w.text()).toContain('文档 doc')})
 it.each([{status:'error'},{error:'source denied'},{type:'tool_call'},{tool_name:'model_citation'},{tool_output:'not JSON'},{tool_output:JSON.stringify({chunks:[{...chunk,knowledge_id:''}]})}])('does not fabricate a reference from invalid or unsuccessful receipt %j',changes=>{expect(knowledgeSources([step(changes as Partial<AgentStep>)])).toEqual([])})
 it('preserves legacy structured source context when old records have no tool receipt',()=>{const legacy={contexts_with_score:[{content:'Legacy authorized API context',chunk_id:'old',source:'legacy source',score:0.4}]}as RAGContext;const w=mountCards({legacy});expect(w.text()).toContain('Legacy authorized API context');expect(w.find('a').exists()).toBe(false);expect(knowledgeSources([step()],legacy)).toHaveLength(1)})
 it('renders source and document content as escaped text without executing links or HTML',()=>{const dangerous={...chunk,content:'<img src=x onerror="alert(1)">',source:'javascript:alert(1)'};const w=mountCards({steps:[step({tool_output:JSON.stringify({chunks:[dangerous]})})]});expect(w.find('img').exists()).toBe(false);expect(w.text()).toContain(dangerous.content);expect(w.find('a[href^="javascript:"]').exists()).toBe(false)})
 it('clears cards when message steps change and keeps same chunk IDs in different scopes distinct',async()=>{const w=mountCards({steps:[step({tool_output:JSON.stringify({chunks:[chunk,{...chunk,knowledge_base_id:'kb2'}]})})]});expect(w.findAll('details')).toHaveLength(2);await w.setProps({steps:[]});expect(w.find('section').exists()).toBe(false)})
})
