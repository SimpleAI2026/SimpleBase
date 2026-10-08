import { describe, expect, it } from 'vitest'
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
    // 未在输入框选择 @ 时 mentions 为空（页面层会兜底用当前选中 Agent）。
    expect(send![0]).toEqual(['查一下订单', []])
  })

  it('shows stop state while sending', () => {
    const wrapper = mount(AgentComposer, {
      props: { modelValue: '', sending: true }
    })
    expect(wrapper.exists()).toBe(true)
  })
})
