import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import AiChat from '@/components/ai/AiChat.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api, isMock: false }
})

describe('AiChat', () => {
  beforeEach(() => resetApiMocks())

  it('renders error retry and canceled assistant with real message scroller', async () => {
    const w = mount(AiChat, {
      props: { projectId: 'p', customSend: vi.fn(), messages: [{ role: 'assistant', content: 'partial', error: '模型超时', canceled: true, thinking: '思考中' }] },
      global: { stubs: { ...uiStubs, MessageScroller: { template: '<div><slot /></div>' }, AiChatComposer: true, AgentToolCard: true } }
    })
    expect(w.text()).toContain('模型超时')
    expect(w.text()).toContain('已停止')
    expect(w.text()).toContain('思考中')
    await w.findAll('button').find((button) => button.text() === '重试')?.trigger('click')
    expect(w.emitted('retry')).toHaveLength(1)
    w.unmount()
  })

  it('covers custom send, default chat, stop and clear', async () => {
    const customSend = vi.fn().mockResolvedValue(undefined)
    const w = mount(AiChat, {
      props: {
        projectId: 'p',
        customSend,
        sending: false,
        messages: [
          { role: 'user', content: 'hi' },
          { role: 'assistant', content: 'yo', toolCalls: [{ name: 't', arguments: '{}', content: '[]' }], error: '请求失败', canceled: true, thinking: '推理片段' }
        ],
        modelOptions: [{ label: 'm', value: 'm' }]
      },
      global: { stubs: { ...uiStubs, AiChatComposer: true, MessageScroller: true } }
    })
    const vm = w.vm as any
    vm.draft = ''
    await vm.onSend([])
    vm.draft = 'hello'
    await vm.onSend([{ agent_id: 'a1' }])
    expect(customSend).toHaveBeenCalled()
    vm.stop()
    expect(w.emitted('stop')).toBeTruthy()
    vm.clear()
    w.unmount()

    api.llm.chat.mockResolvedValue({ content: 'ok', model: 'm', provider: 'p' })
    const w2 = mount(AiChat, {
      props: { projectId: 'p', streaming: false, model: 'gpt-4o-mini' },
      global: { stubs: { ...uiStubs, AiChatComposer: true, MessageScroller: true } }
    })
    const vm2 = w2.vm as any
    vm2.draft = 'hi'
    await vm2.onSend()
    await flushPromises()
    vm2.stop()
    vm2.clear()
    w2.unmount()
  })
})
