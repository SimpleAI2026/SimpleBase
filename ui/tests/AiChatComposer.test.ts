import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { uiStubs } from '@/test/helpers'
import AiChatComposer from '@/components/ai/AiChatComposer.vue'

const agents = [
  { id: 'a1', name: 'Database', module: 'database' },
  { id: 'a2', name: 'S3', module: 's3' }
]

describe('AiChatComposer', () => {
  it('covers mention parsing, picking, send keys, and autosize', async () => {
    const w = mount(AiChatComposer, {
      props: { modelValue: 'hello @Da', mentionAgents: agents },
      global: { stubs: uiStubs }
    })
    const vm = w.vm as any
    vm.parseMentionQuery('nope')
    vm.parseMentionQuery('hi @Da')
    vm.parseMentionQuery('hi @Da more')
    vm.pickMention(agents[0])
    vm.pickMention(agents[0])
    expect(vm.mentionsForSend('@Database and @S3')).toEqual(
      expect.arrayContaining([{ agent_id: 'a1' }, { agent_id: 'a2' }])
    )
    const ta = w.get('textarea')
    await ta.trigger('input')
    await ta.trigger('keydown', { key: 'Enter', shiftKey: true })
    await ta.trigger('keydown', { key: 'Enter', shiftKey: false })
    await ta.trigger('keydown', { key: 'Escape' })
    vm.autosize()
    await w.setProps({ modelValue: 'changed', sending: true })
    vm.onInput({ target: { value: '@' } } as unknown as Event)
    w.unmount()
  })
})

  it('AiChatComposer mention parsing and send', async () => {
    const w = mount(AiChatComposer, {
      props: {
        modelValue: 'hi @Bot',
        mentionAgents: [{ id: 'a1', name: 'Bot', module: 'db' }]
      },
      global: { stubs: uiStubs }
    })
    const vm = w.vm as any
    expect(vm.mentionsForSend('hi @Bot')).toEqual([{ agent_id: 'a1' }])
    vm.parseMentionQuery('hello')
    vm.parseMentionQuery('hi @Bo')
    vm.parseMentionQuery('hi @Bo more')
    vm.pickMention({ id: 'a1', name: 'Bot' })
    vm.pickMention({ id: 'a1', name: 'Bot' })
    vm.onInput({ target: { value: 'x @Z' } })
    vm.onKeydown({ key: 'Escape', preventDefault: vi.fn() })
    vm.mentionOpen = true
    vm.onKeydown({ key: 'Escape', preventDefault: vi.fn() })
    vm.onKeydown({ key: 'Enter', shiftKey: true, preventDefault: vi.fn() })
    const prevent = vi.fn()
    vm.onKeydown({ key: 'Enter', shiftKey: false, preventDefault: prevent })
    await flushPromises()
    await w.setProps({ modelValue: 'plain' })
    vm.pickMention({ id: 'z', name: 'Zed' })
    vm.pickMention({ id: 'cron', name: '定时任务', module: 'cron' })
    expect(vm.mentionsForSend('plain').map((m: { agent_id: string }) => m.agent_id)).toEqual(['a1', 'z'])
    expect(vm.skillsForSend('@定时任务 @KV')).toEqual(['cron', 'kv'])
    vm.parseMentionQuery('@')
    expect(vm.filteredMentions.length).toBeGreaterThan(0)
    w.unmount()
  })
