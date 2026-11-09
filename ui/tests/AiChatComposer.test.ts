import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { uiStubs } from '@/test/helpers'
import AiChatComposer from '@/components/ai/AiChatComposer.vue'

const agents = [
  { id: 'a1', name: 'Database', module: 'database' },
  { id: 'a2', name: 'S3', module: 's3' }
]

describe('AiChatComposer', () => {
  it('sends plain text without any mention candidate and never treats a skill alias as a mention', async () => {
    const w = mount(AiChatComposer, {
      props: { modelValue: '@数据库 查询统计表', mentionAgents: [] }, global: { stubs: uiStubs }
    })
    await w.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    expect(w.emitted('send')?.[0]).toEqual([[]])
    await w.setProps({ modelValue: '直接查询统计表' })
    await w.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    expect(w.emitted('send')?.[1]).toEqual([[]])
    await w.setProps({ modelValue: '@未知助手 请解释' })
    await w.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    expect(w.emitted('send')?.[2]).toEqual([[]])
    await w.setProps({ modelValue: '@数据库' })
    await w.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    expect(w.emitted('send')?.[3]).toEqual([[]])
    w.unmount()
  })

  it('offers only agent mentions and inserts a mention token', async () => {
    const w = mount(AiChatComposer, {
      props: {
        modelValue: '@键', mentionAgents: [{ id: 'kv', name: '键值助手', module: 'kv' }]
      },
      global: { stubs: uiStubs }
    })
    const vm = w.vm as unknown as { parseMentionQuery: (text: string) => void; pickMention: (agent: { id: string; name: string; module?: string }) => void }
    vm.parseMentionQuery('@键')
    await flushPromises()
    expect(w.text()).toContain('助手')
    // `@` 菜单里只有助手分组，不再出现技能分组。
    expect(w.text()).not.toContain('技能')
    vm.pickMention({ id: 'kv', name: '键值助手', module: 'kv' })
    expect(w.emitted('update:modelValue')?.at(-1)?.[0]).toBe('@键值助手 ')
    expect(w.emitted('send')).toBeUndefined()
    w.unmount()
  })

  it('does not open the mention menu for a skill-only alias', async () => {
    const w = mount(AiChatComposer, {
      props: { modelValue: '@沙', mentionAgents: [] },
      global: { stubs: uiStubs }
    })
    const vm = w.vm as unknown as { parseMentionQuery: (text: string) => void; mentionOpen: boolean }
    vm.parseMentionQuery('@沙')
    await flushPromises()
    expect(vm.mentionOpen).toBe(false)
    expect(w.text()).not.toContain('技能')
    w.unmount()
  })

  it('keeps a selected agent id for ambiguous names and drops the binding when the draft changes', async () => {
    const sameName = [
      { id: 'a1', name: '报告助手', module: 'general' },
      { id: 'a2', name: '报告助手', module: 'database' },
      { id: 'database', name: '数据库', module: 'database' }
    ]
    const w = mount(AiChatComposer, {
      props: { modelValue: '@报', mentionAgents: sameName },
      global: { stubs: uiStubs }
    })
    const vm = w.vm as unknown as {
      pickMention: (agent: typeof sameName[number]) => void
      mentionsForSend: (text: string) => { agent_id: string }[]
    }
    vm.pickMention(sameName[1])
    await w.setProps({ modelValue: '@报告助手 ' })
    expect(vm.mentionsForSend('@报告助手 ')).toEqual([{ agent_id: 'a2' }])
    await w.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    expect(w.emitted('send')?.[0]).toEqual([[{ agent_id: 'a2' }]])
    const ta = w.get('textarea')
    await ta.setValue('@报告助手 请生成报告')
    expect(vm.mentionsForSend('@报告助手 请生成报告')).toEqual([{ agent_id: 'a2' }])
    // 「数据库」在这里是助手名，不再是技能别名：命中助手而非技能。
    expect(vm.mentionsForSend('@数据库 ')).toEqual([{ agent_id: 'database' }])
    await w.setProps({ modelValue: '@数据库 ' })
    vm.pickMention(sameName[2])
    await w.setProps({ modelValue: '@数据库 ' })
    expect(vm.mentionsForSend('@数据库 ')).toEqual([{ agent_id: 'database' }])
    await w.setProps({ modelValue: '@数据库 @报告助手 ' })
    // 两个助手名都在候选里；「报告助手」有两个同名助手，未点选就不算点名。
    expect(vm.mentionsForSend('@数据库 @报告助手 ')).toEqual([{ agent_id: 'database' }])
    await w.setProps({ modelValue: '@数据库x ' })
    expect(vm.mentionsForSend('@数据库x ')).toEqual([])
    await w.setProps({ modelValue: '' })
    expect(vm.mentionsForSend('@报告助手 ')).toEqual([])
    w.unmount()
  })

  it('parses a selected agent whose name contains spaces, but not an ambiguous manual name', async () => {
    const spaced = { id: 'spaced', name: '统计 报表助手', module: 'database' }
    const w = mount(AiChatComposer, {
      props: { modelValue: '分析 @统', mentionAgents: [spaced] }, global: { stubs: uiStubs }
    })
    const vm = w.vm as unknown as {
      pickMention: (agent: typeof spaced) => void
      mentionsForSend: (text: string) => { agent_id: string }[]
    }
    vm.pickMention(spaced)
    await w.setProps({ modelValue: '分析 @统计 报表助手 生成摘要' })
    expect(vm.mentionsForSend('分析 @统计 报表助手 生成摘要')).toEqual([{ agent_id: 'spaced' }])
    await w.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    expect(w.emitted('send')?.[0]).toEqual([[{ agent_id: 'spaced' }]])
    await w.setProps({ modelValue: '分析 @统计 报表助手 再检查' })
    await w.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    expect(w.emitted('send')?.[1]).toEqual([[{ agent_id: 'spaced' }]])
    await w.setProps({ modelValue: '分析 @统计 报表助手' })
    await w.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    expect(w.emitted('send')?.[2]).toEqual([[{ agent_id: 'spaced' }]])
    await w.setProps({ modelValue: '' })
    expect(vm.mentionsForSend('@统计 报表助手 生成摘要')).toEqual([])
    w.unmount()
  })

  it('asks for a candidate when a spaced agent name is typed rather than selected', async () => {
    const w = mount(AiChatComposer, {
      props: { modelValue: '@统计 报表助手 生成摘要', mentionAgents: [{ id: 'spaced', name: '统计 报表助手', module: 'database' }] },
      global: { stubs: uiStubs }
    })
    await w.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    expect(w.emitted('send')).toBeUndefined()
    expect(w.get('[role="alert"]').text()).toContain('请选择候选')
    await w.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    expect(w.emitted('send')).toBeUndefined()
    await w.get('textarea').setValue('继续分析')
    expect(w.find('[role="alert"]').exists()).toBe(false)
    w.unmount()
  })

  it('asks for an explicit candidate when manual text matches duplicate agent names', async () => {
    const w = mount(AiChatComposer, {
      props: { modelValue: '@报告助手 生成摘要', mentionAgents: [
        { id: 'one', name: '报告助手', module: 'general' }, { id: 'two', name: '报告助手', module: 'database' }
      ] }, global: { stubs: uiStubs }
    })
    await w.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    expect(w.emitted('send')).toBeUndefined()
    expect(w.text()).toContain('请选择候选')
    w.unmount()
  })

  it('keeps built-in agent mentions across locale switches', async () => {
    const w = mount(AiChatComposer, {
      props: { modelValue: '@数据库助手 查询', mentionAgents: [{ id: 'database', name: 'Database Assistant', legacyName: 'Database', builtinKey: 'database', module: 'database' }] },
      global: { stubs: uiStubs }
    })
    const vm = w.vm as unknown as { mentionsForSend: (text: string) => { agent_id: string }[] }
    expect(vm.mentionsForSend('@数据库助手 查询')).toEqual([{ agent_id: 'database' }])
    expect(vm.mentionsForSend('@Database 查询')).toEqual([{ agent_id: 'database' }])
    // 别名「数据库」不再隐式命中助手：不带「助手」后缀就不是助手名。
    expect(vm.mentionsForSend('@数据库 查询')).toEqual([])
    w.unmount()
  })

  it('resolves @KV助手 only to the existing built-in agent and rejects collisions', async () => {
    const kv = { id: 'kv-id', name: 'Key-Value Assistant', builtinKey: 'kv', module: 'kv' }
    const w = mount(AiChatComposer, {
      props: { modelValue: '@KV助手 查询缓存', mentionAgents: [kv] }, global: { stubs: uiStubs }
    })
    const vm = w.vm as unknown as { mentionsForSend: (text: string) => { agent_id: string }[] }
    expect(vm.mentionsForSend('@KV助手 查询缓存')).toEqual([{ agent_id: 'kv-id' }])
    await w.setProps({ mentionAgents: [kv, { id: 'custom', name: 'KV助手', module: 'general' }] })
    await w.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    expect(w.emitted('send')).toBeUndefined()
    expect(w.get('[role="alert"]').text()).toContain('请选择候选')
    w.unmount()
  })

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
    expect(vm.mentionsForSend('plain')).toEqual([])
    expect(vm.mentionsForSend('@Bot @Bot')).toEqual([{ agent_id: 'a1' }])
    expect(vm.mentionsForSend('@定时任务 @KV')).toEqual([])
    vm.parseMentionQuery('@')
    expect(vm.filteredMentions.length).toBeGreaterThan(0)
    w.unmount()
  })
})
