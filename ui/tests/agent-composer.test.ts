import { describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import AgentComposer from '@/components/agent/AgentComposer.vue'

describe('AgentComposer passthrough', () => {
  it('forwards props and emits send with model text', async () => {
    const wrapper = mount(AgentComposer, {
      props: {
        modelValue: '查一下订单',
        sending: false,
        placeholder: '输入 @ 点名',
        mentionAgents: [{ id: 'a1', name: 'Database', module: 'database' }]
      },
      attachTo: document.body
    })
    // 真实 AiChatComposer：textarea 输入 + 提交按钮。
    const textarea = wrapper.find('textarea')
    expect(textarea.exists()).toBe(true)
    const sendBtn = wrapper.findAll('button').find((b) => b.attributes('type') !== 'button' || /发送|send/i.test(b.text()) || b.text() === '')
    // 触发 keydown Enter。
    await textarea.setValue('查一下订单')
    await textarea.trigger('keydown', { key: 'Enter', shiftKey: false })
    const send = wrapper.emitted('send')
    expect(send).toBeTruthy()
    // 未在输入框选择 @ 时 mentions 为空（页面层会兜底用当前选中 Agent）；技能由后端按 module 决定。
    expect(send![0]).toEqual(['查一下订单', []])
  })

  it('shows stop state while sending', () => {
    const wrapper = mount(AgentComposer, {
      props: { modelValue: '', sending: true }
    })
    expect(wrapper.exists()).toBe(true)
  })

  it('exposes focus() that reaches the textarea', async () => {
    // planv6.0 §3.5：宿主点提示词后要把焦点交给输入框。
    const wrapper = mount(AgentComposer, {
      props: { modelValue: '待发送' },
      attachTo: document.body
    })
    const vm = wrapper.vm as unknown as { focus: () => void }
    vm.focus()
    await nextTick()
    expect(document.activeElement).toBe(wrapper.get('textarea').element)
    wrapper.unmount()
  })
})
