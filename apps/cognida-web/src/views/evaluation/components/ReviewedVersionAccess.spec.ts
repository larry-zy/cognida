import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import ReviewedDatasetVersions from './ReviewedDatasetVersions.vue'
import CreateEvaluationDialog from './CreateEvaluationDialog.vue'
const mocks = vi.hoisted(() => ({ list: vi.fn(), read: vi.fn(), auth: null as any }))
vi.mock('@/api/evaluation/versions', () => ({ datasetVersionsApi: { list: mocks.list, read: mocks.read } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => mocks.auth }))
vi.mock('@/api/evaluation', () => ({ evaluationApi: { getGraders: vi.fn().mockResolvedValue({ data: { graders: [] } }) } }))
function deferred<T>() { let resolve!: (v: T) => void; let reject!: (e: Error) => void; const promise = new Promise<T>((yes, no) => {resolve=yes;reject=no}); return {promise,resolve,reject} }
const item=(id:string)=>({id,name:id,version:1,profile:'agent',manifest_digest:'digest'})
beforeEach(() => {vi.clearAllMocks();mocks.auth=reactive({authEpoch:1,user:{id:1},currentTenant:{id:1}});mocks.list.mockResolvedValue({data:{items:[],next_cursor:''}})})
describe('fixed version identity and cursor access', () => {
 it('reloads after identity changes while an old list request remains pending', async () => {
  const old=deferred<any>();mocks.list.mockReturnValueOnce(old.promise).mockResolvedValueOnce({data:{items:[item('new-owner')]}})
  const w=mount(ReviewedDatasetVersions);mocks.auth.authEpoch++;await flushPromises();expect(w.text()).toContain('new-owner');expect(w.find('button').attributes('disabled')).toBeUndefined()
  old.resolve({data:{items:[item('old-owner')]}});await flushPromises();expect(w.text()).not.toContain('old-owner');w.unmount()
 })
 it('ignores stale download errors after identity changes', async () => {
  mocks.list.mockResolvedValue({data:{items:[item('current')]}});const w=mount(ReviewedDatasetVersions);await flushPromises();const pending=deferred<any>();mocks.read.mockReturnValueOnce(pending.promise)
  await w.findAll('button')[1].trigger('click');mocks.auth.authEpoch++;await flushPromises();pending.reject(new Error('old-account-error'));await flushPromises();expect(w.text()).not.toContain('old-account-error');w.unmount()
 })
 it('loads later pages and ignores responses from a closed modal or old identity', async () => {
  const first=deferred<any>();mocks.list.mockReturnValueOnce(first.promise).mockResolvedValueOnce({data:{items:[item('new')],next_cursor:'next'}}).mockResolvedValueOnce({data:{items:[item('later')],next_cursor:''}})
  const slot={template:'<div><slot/><slot name="footer"/></div>'}
  const w=mount(CreateEvaluationDialog,{props:{modelValue:true,datasets:[],knowledgeBases:[],chatModels:[],agents:[],datasources:[],creating:false},global:{stubs:{UiModal:slot,UiForm:slot,UiFormItem:slot,UiSelect:true,UiCheckbox:true,UiTextarea:true,UiButton:true}}})
  mocks.auth.authEpoch++;await flushPromises();expect(w.find('select').text()).toContain('new');const more=w.findAll('button').find(b=>b.text().includes('加载更多固定版本'))!;await more.trigger('click');await flushPromises();expect(mocks.list).toHaveBeenLastCalledWith('next');expect(w.find('select').text()).toContain('later')
  await w.setProps({modelValue:false});first.resolve({data:{items:[item('old')]}});await flushPromises();expect(w.text()).not.toContain('old');w.unmount()
 })
})
