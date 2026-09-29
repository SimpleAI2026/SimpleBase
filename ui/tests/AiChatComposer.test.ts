import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { uiStubs } from '../../test/helpers'
import AiChatComposer from './AiChatComposer.vue'

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
