import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PublishDatasetVersion from './PublishDatasetVersion.vue'
import ApplyKnowledgeLabels from './ApplyKnowledgeLabels.vue'
import EvaluationProfileEditor from './EvaluationProfileEditor.vue'
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({authEpoch:1,user:{id:1},currentTenant:{id:1}}) }))
const mocks = vi.hoisted(() => ({ publish: vi.fn(), get: vi.fn(), action: vi.fn() }))
vi.mock('@/api/evaluation/versions', () => ({ datasetVersionsApi: { publish: mocks.publish } }))
vi.mock('@/api/governance', () => ({ governanceApi: { action: mocks.action, get: mocks.get } }))
const task = { id: 'task', name: 'QA', type: 'qa', source_ref_id: 'source:1', labels: [] } as any
const approved = { id: 'approved', content: 'Reviewed question', version: 4, review_status: 'approved' } as any
beforeEach(() => { vi.resetAllMocks(); mocks.publish.mockResolvedValue({ data: { id: 'version', version: 1, name: 'frozen', manifest_digest: 'digest', samples: [approved] } }); mocks.action.mockResolvedValue({ data: { id: 'receipt', labels: ['sales'] } }) })
describe('reviewed publishing', () => {
 it('publishes only explicitly selected approved revisions', async () => {
  const w = mount(PublishDatasetVersion, { props: { task, items: [approved, { ...approved, id: 'pending', review_status: 'pending' }] } })
  expect(w.findAll('input[type=checkbox]')).toHaveLength(1)
  await w.findAll('input')[0].setValue('dataset'); await w.findAll('input')[1].setValue('frozen')
  await w.find('input[type=checkbox]').setValue(true); await w.find('form').trigger('submit'); await flushPromises()
  expect(mocks.publish).toHaveBeenCalledWith(expect.objectContaining({ dataset_id: 'dataset', profile: 'qa', samples: [{ task_id: 'task', item_id: 'approved', expected_version: 4 }], split: { algorithm: 'none', near_duplicate_threshold: 0.85 } }))
  expect(w.text()).toContain('清单摘要：digest')
 })
 it('clears selection when review ceases to be approved', async () => {
  const w = mount(PublishDatasetVersion, { props: { task, items: [approved] } }); await w.find('input[type=checkbox]').setValue(true); await w.setProps({ items: [{ ...approved, version: 5, review_status: 'pending' }] }); expect(w.text()).toContain('已选择 0 项'); expect(w.find('button').attributes('disabled')).toBeDefined()
 })
 it('does not apply a pending result or arbitrary manual asset to knowledge', async () => {
  const w = mount(ApplyKnowledgeLabels, { props: { result: { ...approved, review_status: 'pending' } as any, sourceRef: 'source:1' } }); expect(w.find('button').attributes('disabled')).toBeDefined()
  await w.setProps({ result: approved }); mocks.get.mockResolvedValue({ data: undefined }); await w.find('button').trigger('click'); await flushPromises(); expect(mocks.action).not.toHaveBeenCalled(); expect(w.find('[role=alert]').text()).toContain('不是')
 })
 it('applies the actual knowledge source and displays its receipt', async () => {
  mocks.get.mockResolvedValue({ data: {result_id:'approved',review_version:4,source_ref_id:'source:1',knowledge_id:'doc',content_digest:'digest'} }); const w = mount(ApplyKnowledgeLabels, { props: { result: approved, sourceRef: 'source:1' } }); await w.find('button').trigger('click'); await flushPromises(); expect(mocks.action).toHaveBeenCalledWith('/tagging-results/approved/apply', { knowledge_id: 'doc', expected_version: 4 }); expect(w.text()).toContain('receipt')
 })
 it('does not dispatch an apply after the component is unmounted during source lookup', async () => {
  let release!: (v:any)=>void; mocks.get.mockReturnValueOnce(new Promise(resolve=>{release=resolve}))
  const w=mount(ApplyKnowledgeLabels,{props:{result:approved,sourceRef:'source:1'}}); await w.find('button').trigger('click'); w.unmount()
  release({data:{result_id:'approved',review_version:4,source_ref_id:'source:1',knowledge_id:'doc',content_digest:'digest'}}); await flushPromises(); expect(mocks.action).not.toHaveBeenCalled()
 })
 it('blocks mismatched review targets and rejected authorization without applying', async()=>{
  for(const target of [{result_id:'other',review_version:4,source_ref_id:'source:1',knowledge_id:'doc'},{result_id:'approved',review_version:3,source_ref_id:'source:1',knowledge_id:'doc'},{result_id:'approved',review_version:4,source_ref_id:'old:1',knowledge_id:'doc'}]){
   mocks.get.mockResolvedValue({data:target});const w=mount(ApplyKnowledgeLabels,{props:{result:approved,sourceRef:'source:1'}});await w.get('button').trigger('click');await flushPromises();expect(mocks.action).not.toHaveBeenCalled();w.unmount()
  }
  mocks.get.mockRejectedValue(new Error('来源无权读取'));const w=mount(ApplyKnowledgeLabels,{props:{result:approved,sourceRef:'source:1'}});await w.get('button').trigger('click');await flushPromises();expect(w.text()).toContain('来源无权读取');expect(mocks.action).not.toHaveBeenCalled();w.unmount()
 })
 it('discards a target after a different reviewed result is selected',async()=>{
  let release!:(v:any)=>void;mocks.get.mockReturnValueOnce(new Promise(resolve=>{release=resolve}));const w=mount(ApplyKnowledgeLabels,{props:{result:approved,sourceRef:'source:1'}});await w.get('button').trigger('click');await w.setProps({result:{...approved,id:'new'}});release({data:{result_id:'approved',review_version:4,source_ref_id:'source:1',knowledge_id:'doc'}});await flushPromises();expect(mocks.action).not.toHaveBeenCalled();w.unmount()
 })
 it('changing profile discards unrelated mapping fields for new review', async () => {
  const w = mount(EvaluationProfileEditor, { props: { modelValue: { kind: 'rag', source_refs: ['source:1'], relevant_chunk_ids: ['chunk'] } } }); await w.find('select').setValue('sql'); expect(w.emitted('update:modelValue')?.[0]).toEqual([{ kind: 'sql' }])
 })
})

describe('reviewed publication evidence bridge',()=>{
 const selector={props:['modelValue'],emits:['update:modelValue','selected'],template:`<div><button type="button" @click="$emit('update:modelValue','published:2');$emit('selected',{asset_id:'published',version:2,kind:'published_asset'})">选择发布二</button><button type="button" @click="$emit('update:modelValue','other:3');$emit('selected',{asset_id:'other',version:3,kind:'published_asset'})">选择发布三</button></div>`}
 async function setup(){const w=mount(ApplyKnowledgeLabels,{props:{result:approved,sourceRef:'source:1'},global:{stubs:{ScenarioAssetSelect:selector}}});await w.get('input[type=checkbox]').setValue(true);await w.findAll('button').find(b=>b.text()==='选择发布二')!.trigger('click');return w}
 const apply=(w:ReturnType<typeof mount>)=>w.findAll('button').find(b=>b.text()==='应用已审核标签')!
 it('binds explicit publication to both target verification and the single apply write',async()=>{mocks.get.mockResolvedValue({data:{result_id:'approved',review_version:4,source_ref_id:'source:1',knowledge_id:'doc',content_digest:'digest',publication_ref_id:'published:2'}});const w=await setup();await apply(w).trigger('click');await flushPromises();expect(mocks.get).toHaveBeenCalledWith('/tagging-results/approved/apply-target?expected_version=4&publication_ref_id=published%3A2');expect(mocks.action).toHaveBeenCalledWith('/tagging-results/approved/apply',{knowledge_id:'doc',expected_version:4,publication_ref_id:'published:2'});w.unmount()})
 it('rejects a target referring to a different publication',async()=>{mocks.get.mockResolvedValue({data:{result_id:'approved',review_version:4,source_ref_id:'source:1',knowledge_id:'doc',publication_ref_id:'wrong:1'}});const w=await setup();await apply(w).trigger('click');await flushPromises();expect(mocks.action).not.toHaveBeenCalled();expect(w.text()).toContain('不是可应用标签');w.unmount()})
 it('discards a pending target if selected publication changes',async()=>{let release!:(v:unknown)=>void;mocks.get.mockReturnValue(new Promise(r=>release=r));const w=await setup();await apply(w).trigger('click');await w.findAll('button').find(b=>b.text()==='选择发布三')!.trigger('click');release({data:{result_id:'approved',review_version:4,source_ref_id:'source:1',knowledge_id:'doc',publication_ref_id:'published:2'}});await flushPromises();expect(mocks.action).not.toHaveBeenCalled();w.unmount()})
})
