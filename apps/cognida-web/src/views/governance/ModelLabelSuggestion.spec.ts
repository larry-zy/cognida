import {describe,it,expect,vi,beforeEach} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import ModelLabelSuggestion from './ModelLabelSuggestion.vue'
const mocks=vi.hoisted(()=>({action:vi.fn()}))
vi.mock('@/api/governance',()=>({governanceApi:{action:mocks.action}}))
vi.mock('@/stores/auth',()=>({useAuthStore:()=>({authEpoch:1,user:{id:1},currentTenant:{id:1}})}))
const item={id:'i',version:3,review_status:'pending',provenance:{source:'rules'},content:'evidence'} as any
beforeEach(()=>vi.resetAllMocks())
describe('model annotation suggestions',()=>{
 it('sends current revision and guidelines, returns only a pending draft',async()=>{mocks.action.mockResolvedValue({data:{...item,version:4,provenance:{source:'model'}}});const w=mount(ModelLabelSuggestion,{props:{kind:'annotation',taskId:'task',item}});await w.find('textarea').setValue('Literal evidence only');await w.find('button').trigger('click');await flushPromises();expect(mocks.action).toHaveBeenCalledWith('/annotation-tasks/task/items/i/suggest',{expected_version:3,guidelines:'Literal evidence only'});expect(w.emitted('updated')?.[0]?.[0]).toMatchObject({review_status:'pending',version:4})})
 it('locks human and reviewed records',async()=>{const w=mount(ModelLabelSuggestion,{props:{kind:'tagging',taskId:'task',item:{...item,provenance:{source:'human'}}}});expect(w.find('button').attributes('disabled')).toBeDefined();await w.setProps({item:{...item,review_status:'approved'}});expect(w.find('button').attributes('disabled')).toBeDefined();expect(mocks.action).not.toHaveBeenCalled()})
 it('does not display a late result after switching items',async()=>{let done!:(v:any)=>void;mocks.action.mockReturnValue(new Promise(resolve=>done=resolve));const w=mount(ModelLabelSuggestion,{props:{kind:'annotation',taskId:'task',item}});await w.find('button').trigger('click');await w.setProps({item:{...item,id:'next'}});done({data:{...item,version:4}});await flushPromises();expect(w.emitted('updated')).toBeUndefined()})
})
