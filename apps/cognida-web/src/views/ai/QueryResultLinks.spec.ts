import { describe, expect, it } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import type { AgentStep } from '@/types'
import QueryResultLinks from './QueryResultLinks.vue'
const result = (output: string, status: 'success' | 'error' = 'success'): AgentStep => ({ id: 'step', step: 1, timestamp: 0, type: 'tool_result', tool_name: 'sql_execute', tool_output: output, status })
describe('query result import actions', () => {
  it('uses only successful observed handles, deduplicates them and binds the current session', async () => {
    const wrapper = mount(QueryResultLinks, { props: { sessionId: 'session-a', steps: [result('{"result_id":"result-a"}'), result('{"result_id":"result-a"}'), result('{"result_id":"bad"}', 'error'), result('incomplete json'), result('{"result_id":34}')] }, global: { stubs: { RouterLink: RouterLinkStub } } })
    const links = wrapper.findAllComponents(RouterLinkStub)
    expect(links).toHaveLength(1)
    expect(links[0].props('to')).toEqual({ path: '/pipelines', query: { source: 'result', session_id: 'session-a', result_id: 'result-a', name: '查询结果 1' } })
    expect(wrapper.text()).toContain('过期后请重新查询')
    await wrapper.setProps({ sessionId: '', steps: [] })
    expect(wrapper.findAllComponents(RouterLinkStub)).toHaveLength(0)
    wrapper.unmount()
  })
  it('never creates an action for assistant prose or an unbound result', () => {
    const wrapper = mount(QueryResultLinks, { props: { sessionId: '', steps: [result('{"result_id":"result-a"}')] }, global: { stubs: { RouterLink: RouterLinkStub } } })
    expect(wrapper.findAllComponents(RouterLinkStub)).toHaveLength(0)
    wrapper.unmount()
  })
})
