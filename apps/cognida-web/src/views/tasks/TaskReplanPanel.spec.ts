import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import type { GovernedTask, PlanNode, PlanSpec, SavedPlan, TaskSummary } from '@/api/taskcontrol'
const api = vi.hoisted(() => ({ replan: vi.fn() }))
vi.mock('@/api/taskcontrol', () => ({ taskControlApi: api }))
import TaskReplanPanel from './TaskReplanPanel.vue'
const budget = { max_nodes: 4, max_concurrency: 1, max_attempts: 4, max_calls: 10, max_cost_micros: 0, timeout_seconds: 300 }
const node = (id: string): PlanNode => ({ id, capability: 'lifecycle.data-cleaning', capability_version: 1, depends_on: id === 'first' ? [] : ['first'], inputs: [{ asset: { asset_id: 'source', version: 1 }, kind: 'raw_asset' }], output_kinds: ['cleaned_asset'], tools: ['clean'], permissions: ['read'], max_attempts: 1, max_calls_per_attempt: 2, max_cost_micros_per_attempt: 0, timeout_seconds: 20 })
const plan = (): PlanSpec => ({ schema_version: 1, budget, nodes: [node('first'), node('future')] })
const task = (): GovernedTask => ({ id: 'task', goal: 'same task', input_refs: [], budget, version: 4, current_plan_version: 1, execution_plan_version: 1, status: 'running', result_refs: [], created_at: '', updated_at: '' })
const saved = (): SavedPlan => ({ task_id: 'task', version: 1, snapshot: plan(), content_hash: 'hash', created_at: '' })
const summary = (): TaskSummary => ({ task_id: 'task', task_version: 4, current_plan_version: 1, planned_nodes: 2, status: 'running', children: [{ id: 'child', node_id: 'first', plan_version: 1, attempt: 1, domain: 'lifecycle', domain_id: 'stage', dispatch_status: 'bound', domain_status: 'running', receipt_present: true }], result_refs: [], warnings: [], execution_enabled: true })
const props = () => ({ task: task(), savedPlan: saved(), summary: summary(), capabilities: [], scope: 'owner1', busy: false })
const button = (wrapper: ReturnType<typeof mount>, label: string) => wrapper.findAll('button').find(x => x.text() === label)!
async function editing() { const wrapper = mount(TaskReplanPanel, { props: props(), global: { stubs: { TaskPlanPreview: true } } }); await button(wrapper, '编辑未执行步骤').trigger('click'); await wrapper.get('[aria-label="future 单次时限"]').setValue(15); await wrapper.get('[aria-label="调整原因"]').setValue('缩短后续步骤时限'); await wrapper.get('input[type=checkbox]').setValue(true); return wrapper }
describe('explicit running replans', () => {
 beforeEach(() => vi.clearAllMocks())
 it('locks reserved nodes and requires an explicit reviewed mutation with unchanged approved budget', async () => {
  const wrapper = await editing(); expect(wrapper.find('[aria-label="first 单次时限"]').exists()).toBe(false)
  let finish!: (x: unknown) => void; api.replan.mockReturnValue(new Promise(resolve => { finish = resolve }))
  await button(wrapper, '确认重规划').trigger('click'); await button(wrapper, '正在保存新版本…').trigger('click')
  expect(api.replan).toHaveBeenCalledTimes(1); const args = api.replan.mock.calls[0]!
  expect(args.slice(0, 2)).toEqual(['task', 4]); expect(args[2].budget).toEqual(budget); expect(args[2].nodes[0]).toEqual(node('first')); expect(args[2].nodes[1].timeout_seconds).toBe(15)
  finish({ data: { plan: { ...saved(), version: 2 } } }); await flushPromises(); expect(wrapper.emitted('committed')).toHaveLength(1); expect(wrapper.emitted('refresh')).toHaveLength(1); wrapper.unmount()
 })
 it('blocks changes to a node reserved while editing and allows restoring that exact node', async () => {
  const wrapper = await editing(); const next = summary(); next.children.push({ ...next.children[0]!, id: 'second', node_id: 'future' }); await wrapper.setProps({ summary: next })
  expect(wrapper.text()).toContain('编辑期间有节点被预留'); expect(button(wrapper, '确认重规划').attributes('disabled')).toBeDefined()
  await button(wrapper, '恢复已锁定节点').trigger('click'); expect(wrapper.text()).not.toContain('编辑期间有节点被预留'); expect(api.replan).not.toHaveBeenCalled(); wrapper.unmount()
 })
 it('does not submit stale, inconsistent or cancelled tasks', async () => {
  const wrapper = await editing(); await wrapper.setProps({ savedPlan: { ...saved(), version: 2 } }); expect(wrapper.text()).toContain('任务已有其他计划版本'); expect(button(wrapper, '确认重规划').attributes('disabled')).toBeDefined()
  await wrapper.setProps({ task: { ...task(), status: 'cancel_requested' } }); expect(wrapper.text()).toContain('任务已进入不可重规划'); expect(api.replan).not.toHaveBeenCalled(); wrapper.unmount()
 })
 it('keeps unknown commit results read-only and never automatically retries', async () => {
  api.replan.mockRejectedValue(new Error('connection lost')); const wrapper = await editing(); await button(wrapper, '确认重规划').trigger('click'); await flushPromises()
  expect(wrapper.text()).toContain('提交结果尚未确认'); await button(wrapper, '核对最新状态').trigger('click'); expect(api.replan).toHaveBeenCalledTimes(1); expect(wrapper.emitted('refresh')).toHaveLength(2); expect(button(wrapper, '确认重规划').attributes('disabled')).toBeDefined(); wrapper.unmount()
 })
 it('aborts identity changes and ignores their late successful replies', async () => {
  let finish!: (x: unknown) => void; api.replan.mockReturnValue(new Promise(resolve => { finish = resolve })); const wrapper = await editing(); await button(wrapper, '确认重规划').trigger('click'); const signal = api.replan.mock.calls[0]![4] as AbortSignal
  await wrapper.setProps({ scope: 'owner2' }); expect(signal.aborted).toBe(true); finish({ data: { plan: { ...saved(), version: 2 } } }); await flushPromises(); expect(wrapper.emitted('committed')).toBeUndefined(); expect(wrapper.find('[aria-label="调整原因"]').exists()).toBe(false); wrapper.unmount()
 })
 it('clears local drafts when permission is denied and surfaces current source failure', async () => {
  api.replan.mockRejectedValue({ response: { status: 403 } }); const wrapper = await editing(); await button(wrapper, '确认重规划').trigger('click'); await flushPromises(); expect(wrapper.emitted('inaccessible')).toHaveLength(1); expect(wrapper.find('[aria-label="调整原因"]').exists()).toBe(false); wrapper.unmount()
 })
 it('rejects changes to global budget through the advanced editor before a request', async () => {
  const wrapper = await editing(); const edited = plan(); edited.budget = { ...budget, max_calls: 20 }; await wrapper.get('[aria-label="计划 JSON"]').setValue(JSON.stringify(edited)); await button(wrapper, '应用到待审阅计划').trigger('click'); expect(wrapper.text()).toContain('累计预算必须保持原值'); expect(api.replan).not.toHaveBeenCalled(); wrapper.unmount()
 })
 it.each([
  ['missing tools', { tools: undefined }], ['invalid permissions', { permissions: {} }], ['missing output kinds', { output_kinds: null }],
  ['null input', { inputs: [null] }], ['invalid asset', { inputs: [{ kind: 'raw_asset', asset: null }] }],
  ['label schema object', { parameters: { governance: { label_schema: {} } } }],
  ['null label', { parameters: { governance: { label_schema: [null] } } }],
  ['keyword string', { parameters: { governance: { label_schema: [{ id: 'x', name: 'X', keywords: 'word' }] } } }],
  ['labels string', { parameters: { governance: { labels: 'label' } } }],
 ])('keeps the real preview intact for malformed advanced JSON: %s', async (_name, fields) => {
  const wrapper = mount(TaskReplanPanel, { props: props() })
  await button(wrapper, '编辑未执行步骤').trigger('click')
  const edited = plan(); Object.assign(edited.nodes[1]!, fields)
  await wrapper.get('[aria-label="计划 JSON"]').setValue(JSON.stringify(edited))
  await button(wrapper, '应用到待审阅计划').trigger('click')
  expect(wrapper.text()).toContain('计划结构不完整')
  expect(wrapper.findAll('.plan-nodes > li')).toHaveLength(2)
  expect(wrapper.get('[aria-label="future 单次时限"]').element).toHaveProperty('value', '20')
  expect(api.replan).not.toHaveBeenCalled()
  wrapper.unmount()
 })

})
