import { describe, expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'
import { mount } from '@vue/test-utils'
import type { Message } from '@/types'
const api = vi.hoisted(() => ({ getDetail: vi.fn(), list: vi.fn(), delete: vi.fn() }))
vi.mock('@/api/session', () => ({ sessionApi: api }))
vi.mock('@/utils/toast', () => ({ default: { error: vi.fn() } }))
vi.mock('@/utils/confirm', () => ({ ElMessageBox: { confirm: vi.fn() } }))
import { useChatSessions } from '../useChatSessions'
import { useAgentChat } from '../useAgentChat'
function deferred<T>() { let resolve!: (value: T) => void; return { promise: new Promise<T>(r => { resolve = r }), resolve: (value: T) => resolve(value) } }
const message = (content: string): Message => ({ id: content, session_id: 'a', role: 'assistant', content, created_at: '', token_count: 1 })
function harness() {
  const messages = ref<Message[]>([]), currentSessionId = ref(''), inputMessage = ref('')
  let sessions!: ReturnType<typeof useChatSessions<Message>>, chat!: ReturnType<typeof useAgentChat<Message>>
  const after = vi.fn()
  const wrapper = mount(defineComponent({ setup() {
    const shared = { messages, currentSessionId, inputMessage, textareaRef: ref<HTMLTextAreaElement>() }
    sessions = useChatSessions({ ...shared, filter: () => true, mapMessage: m => m, searchTitleFallback: '', deleteConfirmMessage: '', loadFailMessage: '', onAfterSwitch: after, onBeforeSwitch: () => chat.stopStreaming() })
    chat = useAgentChat({ ...shared, messagesContainer: ref(), onSendStart: () => sessions.invalidateHistory(), createStream: async function* () { yield { event: 'done', answer: 'new answer' } }, buildAssistantMessage: message })
    return () => null
  } }))
  return { messages, inputMessage, sessions, chat, wrapper, after }
}
describe('session history request generations', () => {
  it('cannot overwrite a newly sent and completed turn with older history', async () => {
    const history = deferred<unknown>(); api.getDetail.mockReturnValueOnce(history.promise)
    const h = harness(); const loading = h.sessions.switchSession('a')
    h.inputMessage.value = 'new question'; await h.chat.sendMessage()
    history.resolve({ data: { messages: [message('old history')] } }); await loading
    expect(h.messages.value.map(m => m.content)).toEqual(['new question', 'new answer'])
    expect(h.after).not.toHaveBeenCalled(); h.wrapper.unmount()
  })
  it('rejects the first A response after A to B to A navigation', async () => {
    const first = deferred<unknown>(), second = deferred<unknown>(), third = deferred<unknown>()
    api.getDetail.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise).mockReturnValueOnce(third.promise)
    const h = harness()
    const a = h.sessions.switchSession('a'), b = h.sessions.switchSession('b'), latest = h.sessions.switchSession('a')
    third.resolve({ data: { messages: [message('latest A')] } }); await latest
    first.resolve({ data: { messages: [message('stale A')] } }); second.resolve({ data: { messages: [message('stale B')] } }); await Promise.all([a, b])
    expect(h.messages.value.map(m => m.content)).toEqual(['latest A']); h.wrapper.unmount()
  })
  it('ignores history after leaving the page', async () => {
    const history = deferred<unknown>(); api.getDetail.mockReturnValueOnce(history.promise)
    const h = harness(); const pending = h.sessions.switchSession('a'); h.wrapper.unmount()
    history.resolve({ data: { messages: [message('private history')] } }); await pending
    expect(h.messages.value).toEqual([]); expect(h.after).not.toHaveBeenCalled()
  })
})
