import { describe, expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { useAgentChat } from '../useAgentChat'
import type { AgentStreamEvent, Message } from '@/types'

vi.mock('@/utils/toast', () => ({ default: { error: vi.fn() } }))

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(r => { resolve = r })
  return { promise, resolve }
}

function harness(createStream: (content: string, signal: AbortSignal) => AsyncGenerator<AgentStreamEvent>) {
  const messages = ref<Message[]>([])
  const currentSessionId = ref('old-session')
  const inputMessage = ref('first question')
  const onEvent = vi.fn()
  const onSendEnd = vi.fn()
  let chat!: ReturnType<typeof useAgentChat<Message>>
  const wrapper = mount(defineComponent({
    setup() {
      chat = useAgentChat<Message>({
        messages, currentSessionId, inputMessage,
        textareaRef: ref(), messagesContainer: ref(), createStream, onEvent, onSendEnd,
        buildAssistantMessage: content => ({ id: 'answer', session_id: currentSessionId.value, role: 'assistant', content, token_count: 1, created_at: '' })
      })
      return () => null
    }
  }))
  return { chat, wrapper, messages, currentSessionId, inputMessage, onEvent, onSendEnd }
}

describe('chat stream lifecycle', () => {
  it('retains the exact answer and actual partial/persistence terminal metadata', async () => {
    const h = harness(async function* () {
      yield { event: 'done', answer: 'partial exact answer', partial: true, persistence_failed: true, terminated_by: 'max_iter', iterations: 8, max_reached: true }
    })
    await h.chat.sendMessage()
    expect(h.messages.value[1]).toMatchObject({ content: 'partial exact answer', completion: { partial: true, persistence_failed: true, terminated_by: 'max_iter', iterations: 8, max_reached: true } })
    expect(h.chat.isStreaming.value).toBe(false)
    h.wrapper.unmount()
  })

  it('ignores late old-session events and does not reset a newer stream', async () => {
    const first = deferred<AgentStreamEvent>()
    const second = deferred<AgentStreamEvent>()
    const signals: AbortSignal[] = []
    const h = harness(async function* (_content, signal) {
      signals.push(signal)
      yield await (signals.length === 1 ? first.promise : second.promise)
    })
    const oldSend = h.chat.sendMessage()
    h.chat.stopStreaming()
    expect(signals[0].aborted).toBe(true)
    h.currentSessionId.value = 'new-session'
    h.messages.value = []
    h.inputMessage.value = 'new question'
    const newSend = h.chat.sendMessage()
    first.resolve({ event: 'done', session_id: 'old-session', answer: 'late old answer' } as AgentStreamEvent)
    await oldSend
    expect(h.currentSessionId.value).toBe('new-session')
    expect(h.chat.isStreaming.value).toBe(true)
    expect(h.messages.value.map(m => m.content)).toEqual(['new question'])
    expect(h.onEvent).not.toHaveBeenCalled()
    second.resolve({ event: 'done', answer: 'new answer' } as AgentStreamEvent)
    await newSend
    expect(h.messages.value.map(m => m.content)).toEqual(['new question', 'new answer'])
    expect(h.chat.isStreaming.value).toBe(false)
    h.wrapper.unmount()
  })

  it('cancels on unmount and never commits a late answer', async () => {
    const event = deferred<AgentStreamEvent>()
    let signal!: AbortSignal
    const h = harness(async function* (_content, s) { signal = s; yield await event.promise })
    const send = h.chat.sendMessage()
    h.wrapper.unmount()
    expect(signal.aborted).toBe(true)
    event.resolve({ event: 'done', answer: 'late answer' } as AgentStreamEvent)
    await send
    expect(h.messages.value).toHaveLength(1)
    expect(h.onSendEnd).toHaveBeenCalledTimes(1)
  })
})
