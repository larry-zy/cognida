import {describe,it,expect,vi,beforeEach} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import CleaningReview from './CleaningReview.vue'
const mocks=vi.hoisted(()=>({post:vi.fn(),get:vi.fn()}))
vi.mock('@/utils/request',()=>({http:mocks}))
vi.mock('@/stores/auth',()=>({useAuthStore:()=>({authEpoch:1,user:{id:1},currentTenant:{id:1}})}))
beforeEach(()=>vi.resetAllMocks())
describe('cleaning diagnosis and exact diff',()=>{
 it('requires an explicit confirmation and invalidates it after an option change',async()=>{mocks.post.mockResolvedValue({data:{plan_digest:'bound',diagnostics:{retention_rate:0.5,before:{row_count:2},after:{row_count:1}},differences:{items:[{source_row:0,before:['00123','9007199254740993.123456789012345678'],operations:['dedup'],removed_by:'dedup'}],total:1,next_cursor:'',columns:['id']}}});const w=mount(CleaningReview,{props:{sourceRef:'source:1',options:{keep:'last'}}});await w.find('button').trigger('click');await flushPromises();expect(mocks.post).toHaveBeenCalledWith('/lifecycle/cleaning/preview',{input_ref:{asset_id:'source',version:1},options:{keep:'last'},cursor:''});expect(w.text()).toContain('9007199254740993.123456789012345678');expect(w.emitted('confirmed')).toBeUndefined();await w.findAll('button').find(b=>b.text().includes('确认此来源'))!.trigger('click');expect(w.emitted('confirmed')?.[0]).toEqual(['bound']);await w.setProps({options:{keep:'first'}});expect(w.emitted('confirmed')?.at(-1)).toEqual(['']);expect(w.text()).not.toContain('方案已确认')})
 it('reads persisted output at its actual version with cursor',async()=>{mocks.get.mockResolvedValueOnce({data:{items:[],total:51,next_cursor:'50',columns:[]}}).mockResolvedValueOnce({data:{items:[],total:51,next_cursor:'',columns:[]}});const w=mount(CleaningReview,{props:{cleanedRef:'output:3'}});await w.find('button').trigger('click');await flushPromises();await w.findAll('button').find(b=>b.text().includes('更多'))!.trigger('click');await flushPromises();expect(mocks.get).toHaveBeenLastCalledWith('/lifecycle/assets/output/cleaning-diff',{params:{version:'3',cursor:'50'}});expect(mocks.post).not.toHaveBeenCalled()})
})
