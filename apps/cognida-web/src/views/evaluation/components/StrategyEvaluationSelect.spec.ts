import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { reactive } from 'vue'
import StrategyEvaluationSelect from './StrategyEvaluationSelect.vue'
const api=vi.hoisted(()=>({versions:vi.fn()}))
vi.mock('@/api/governance/strategies',()=>({strategyApi:api}))
const state=reactive({authEpoch:1,user:{id:1},currentTenant:{id:1}})
vi.mock('@/stores/auth',()=>({useAuthStore:()=>state}))
beforeEach(()=>{vi.resetAllMocks();state.authEpoch=1;api.versions.mockResolvedValue({data:{items:[{id:'fixed',version:2}],next_cursor:'next'}})})
describe('fixed evaluation policy selector',()=>{
 it('selects an immutable version with cursor pagination without activating',async()=>{const w=mount(StrategyEvaluationSelect,{props:{type:'qa',modelValue:'',open:true}});await w.find('select').setValue('cleaning');await flushPromises();await w.findAll('select')[1]!.setValue('fixed');expect(w.emitted('update:modelValue')?.at(-1)).toEqual(['fixed']);await w.find('button').trigger('click');await flushPromises();expect(api.versions).toHaveBeenLastCalledWith('cleaning.default','next');w.unmount()})
 it('discards late versions on close and constrains RAG to retrieval',async()=>{let finish!:(x:any)=>void;api.versions.mockReturnValueOnce(new Promise(r=>finish=r));const w=mount(StrategyEvaluationSelect,{props:{type:'qa',modelValue:'',open:true}});await w.find('select').setValue('cleaning');await w.setProps({open:false,type:'rag'});finish({data:{items:[{id:'old-scope',version:1}]}});await flushPromises();expect(w.text()).not.toContain('old-scope');expect(w.find('select').text()).toContain('retrieval');expect(w.find('select').text()).not.toContain('cleaning');w.unmount()})
})
