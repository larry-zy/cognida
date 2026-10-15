import { afterEach, describe, expect, it } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import UiModal from './UiModal.vue'

const wrappers: VueWrapper[] = []
function modal(props = {}, content = '<input aria-label="名称" /><button>保存</button>') {
  const wrapper = mount(UiModal, {
    attachTo: document.body,
    props: { modelValue: true, title: '创建采集任务', ...props },
    slots: { default: content },
  })
  wrappers.push(wrapper)
  return wrapper
}
function key(value: string, shiftKey = false) {
  const event = new KeyboardEvent('keydown', { key: value, shiftKey, bubbles: true, cancelable: true })
  document.dispatchEvent(event)
  return event
}
afterEach(() => {
  wrappers.splice(0).reverse().forEach(wrapper => wrapper.unmount())
  document.body.innerHTML = ''
  document.body.style.overflow = ''
})

describe('UiModal keyboard interaction', () => {
  it('names and focuses the dialog on initial open, wraps Tab and restores the opener', async () => {
    document.body.style.overflow = 'scroll'
    const opener = document.createElement('button')
    document.body.append(opener)
    opener.focus()
    const wrapper = modal()
    await flushPromises()
    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    expect(document.getElementById(dialog.getAttribute('aria-labelledby')!)?.textContent).toBe('创建采集任务')
    expect(dialog.getAttribute('aria-modal')).toBe('true')
    expect(document.activeElement).toBe(dialog)
    expect(document.body.style.overflow).toBe('hidden')

    const buttons = dialog.querySelectorAll('button')
    expect(key('Tab').defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(buttons[0])
    key('Tab', true)
    expect(document.activeElement).toBe(buttons[1])
    key('Tab')
    expect(document.activeElement).toBe(buttons[0])

    await wrapper.setProps({ modelValue: false })
    await flushPromises()
    expect(document.activeElement).toBe(opener)
    expect(document.body.style.overflow).toBe('scroll')
  })

  it('only closes the top dialog on Escape and preserves scroll lock for its parent', async () => {
    const parent = modal()
    await flushPromises()
    const parentInput = document.querySelector<HTMLInputElement>('input')!
    parentInput.focus()
    const child = modal({ title: '确认发布' })
    await flushPromises()
    key('Escape')
    expect(child.emitted('update:modelValue')).toEqual([[false]])
    expect(parent.emitted('update:modelValue')).toBeUndefined()
    await child.setProps({ modelValue: false })
    await flushPromises()
    expect(document.activeElement).toBe(parentInput)
    expect(document.body.style.overflow).toBe('hidden')
    key('Escape')
    expect(parent.emitted('update:modelValue')).toEqual([[false]])
  })

  it('does not emit close for hidden dialogs or when Escape dismissal is disabled', async () => {
    const hidden = modal({ modelValue: false })
    const busy = modal({ closeOnEsc: false, closable: false, maskClosable: false }, '<button disabled>正在提交</button>')
    await flushPromises()
    key('Escape')
    expect(busy.emitted('close')).toBeUndefined()
    expect(hidden.emitted('close')).toBeUndefined()
    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    key('Tab')
    expect(document.activeElement).toBe(dialog)
    document.querySelector<HTMLElement>('.ui-modal__backdrop')!.click()
    expect(busy.emitted('close')).toBeUndefined()
  })

  it('restores state on unmount and ignores hidden or disabled focus targets', async () => {
    const wrapper = modal({ closable: false }, '<button hidden>隐藏</button><input autofocus aria-label="名称" /><button disabled>禁用</button><button style="display:none">不可见</button>')
    await flushPromises()
    const input = document.querySelector('input')!
    expect(document.activeElement).toBe(input)
    key('Tab')
    expect(document.activeElement).toBe(input)
    wrapper.unmount()
    wrappers.pop()
    expect(document.body.style.overflow).toBe('')
    key('Escape')
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('keeps focus inside the dialog when technical details are collapsed', async () => {
    modal({}, '<button>刷新</button><details><summary tabindex="0">运行信息</summary><a href="#knowledge">知识库</a></details>')
    await flushPromises()
    const close = document.querySelector<HTMLElement>('.ui-modal__close')!
    const summary = document.querySelector<HTMLElement>('summary')!
    close.focus()
    key('Tab', true)
    expect(document.activeElement).toBe(summary)
    key('Tab')
    expect(document.activeElement).toBe(close)
    document.querySelector('details')!.open = true
    key('Tab', true)
    expect(document.activeElement).toBe(document.querySelector('a'))
  })
})
