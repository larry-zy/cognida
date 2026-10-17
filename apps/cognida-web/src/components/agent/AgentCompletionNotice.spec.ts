import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AgentCompletionNotice from './AgentCompletionNotice.vue'
describe('actual Agent terminal receipt', () => {
  it('distinguishes incomplete answers and failed persistence without claiming success', () => {
    const w = mount(AgentCompletionNotice, { props: { completion: { partial: true, persistence_failed: true, terminated_by: 'max_iter' } } })
    expect(w.get('[role=status]').text()).toContain('本轮未完成')
    expect(w.text()).toContain('已达到执行轮次上限')
    expect(w.text()).toContain('对话保存失败')
    expect(w.text()).toContain('刷新或离开后可能无法恢复')
  })
  it('does not invent failure for successful or historical receipts and safely preserves unknown reasons', async () => {
    const w = mount(AgentCompletionNotice)
    expect(w.find('[role=status]').exists()).toBe(false)
    await w.setProps({ completion: { partial: false, persistence_failed: false } })
    expect(w.find('[role=status]').exists()).toBe(false)
    await w.setProps({ completion: { partial: true, persistence_failed: false, terminated_by: '<img src=x onerror=alert(1)>' } })
    expect(w.text()).toContain('<img src=x onerror=alert(1)>'); expect(w.find('img').exists()).toBe(false)
  })
})
