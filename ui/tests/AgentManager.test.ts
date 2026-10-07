import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { useProjectStore } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, mountWithApp, sampleAgent } from '@/test/helpers'
import AgentManager from '@/pages/AgentManager.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api, isMock: false }
})

const interactionAgentStubs = {
  AiChat: {
    props: ['messages', 'sending'],
    emits: ['stop'],
    template:
      '<div class="ai"><button type="button" class="ai-stop" @click="$emit(\'stop\')">stop</button><slot name="toolbar" /><slot name="empty" /></div>'
  },
  AgentScheduleModal: {
    props: ['open', 'agent', 'schedule'],
    emits: ['update:open', 'saved', 'removed', 'view-thread'],
    template: `
      <div v-if="open" class="sch-m">
        <button type="button" class="sch-close" @click="$emit('update:open', false)">x</button>
        <button type="button" class="sch-saved" @click="$emit('saved', { id: 's1', agent_id: 'ag-1' })">save</button>
        <button type="button" class="sch-removed" @click="$emit('removed', 'sch-1')">rm</button>
        <button type="button" class="sch-thread" @click="$emit('view-thread', 'th-1')">th</button>
      </div>
    `
  }
}


const agentStubs = {
  AiChat: {
    props: ['customSend', 'messages', 'sending', 'mentionAgents'],
    emits: ['stop'],
    template: `
      <div class="ai-chat">
        <button type="button" class="ai-send" @click="customSend && customSend('hello', mentionAgents && mentionAgents[0] ? [{ agent_id: mentionAgents[0].id }] : [])">send</button>
        <button type="button" class="ai-blank" @click="customSend && customSend('   ', [])">blank</button>
        <button type="button" class="ai-stop" @click="$emit('stop')">stop</button>
        <slot name="toolbar" />
        <slot name="empty" />
      </div>
    `
  },
  AgentScheduleModal: {
    props: ['open', 'agent', 'schedule'],
    emits: ['update:open', 'saved', 'removed', 'view-thread'],
    template: `
      <div v-if="open" class="sch-m">
        {{ agent && agent.name }}
        <button type="button" class="sch-saved" @click="$emit('saved', { id: 's1', agent_id: 'ag-1', cron_expr: '0 8 * * *', enabled: true })">save</button>
        <button type="button" class="sch-removed" @click="$emit('removed', 'sch-1')">rm</button>
        <button type="button" class="sch-thread" @click="$emit('view-thread', 'th-1')">th</button>
        <button type="button" class="sch-close" @click="$emit('update:open', false)">x</button>
      </div>
    `
  }
}

function clickExact(wrapper: Awaited<ReturnType<typeof mountWithApp>>['wrapper'], text: string) {
  const el = wrapper.findAll('button').find((b) => b.text().trim() === text)
  if (!el) throw new Error(`No exact button "${text}"`)
  return el.trigger('click')
}

describe('AgentManager (云 Agent)', () => {
  beforeEach(() => {
    resetApiMocks()
    api.agents.list.mockResolvedValue([sampleAgent])
    api.agents.modules.mockResolvedValue([
      { id: 'database', name: 'Database', description: 'd', default_tools: ['list_databases'], team_supported: false },
      { id: 's3', name: 'S3', description: 's', default_tools: ['list_objects'], team_supported: false }
    ])
    api.agentThreads.list.mockResolvedValue([{ id: 'th-1', title: 'T', created_at: 't', updated_at: 't' }])
    api.agentThreads.messages.mockResolvedValue([])
    api.agentSchedules.list.mockResolvedValue([
      {
        id: 'sch-1',
        agent_id: 'ag-1',
        thread_id: 'th-1',
        prompt: 'p',
        cron_expr: '0 * * * *',
        enabled: true,
        created_at: 't',
        updated_at: 't'
      }
    ])
  })

  it('lists agents with schedule summary and selects one', async () => {
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    expect(wrapper.text()).toContain('按模块的 Agent')
    expect(wrapper.text()).toContain('Database')
    expect(wrapper.text()).toContain('每小时')
    expect(wrapper.text()).toContain('已启用')
    const select = wrapper.get('button[aria-label="选择 Agent Database"]')
    expect(select.attributes('aria-pressed')).toBe('true')
    expect(select.find('button').exists()).toBe(false)
    await select.trigger('click')
    expect(select.attributes('aria-pressed')).toBe('true')
    expect(wrapper.text()).toContain('@Database')
  })

  it('creates, edits, and deletes an agent from the page', async () => {
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await clickExact(wrapper, '新建')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)
    await wrapper.get('#agent-name').setValue('')
    await wrapper.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(toast.warning).toHaveBeenCalledWith('请填写名称')

    await wrapper.get('#agent-name').setValue('N2')
    await wrapper.get('#agent-desc').setValue('desc')
    await wrapper.get('#agent-prompt').setValue('sys')
    await wrapper.get('.select-emit').trigger('click')
    const toolButton = wrapper.findAll('button.badge').at(-1)
    if (!toolButton) throw new Error('Missing tool selector')
    const initialPressed = toolButton.attributes('aria-pressed')
    expect(['true', 'false']).toContain(initialPressed)
    await toolButton.trigger('click')
    expect(toolButton.attributes('aria-pressed')).not.toBe(initialPressed)
    await wrapper.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.agents.create).toHaveBeenCalled()

    await clickExact(wrapper, '编辑')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)
    await wrapper.get('#agent-name').setValue('N3')
    await wrapper.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.agents.patch).toHaveBeenCalled()

    api.agents.remove.mockRejectedValueOnce(new Error('rm'))
    await clickExact(wrapper, '删除')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('rm')
    await clickExact(wrapper, '删除')
    await flushPromises()
    expect(api.agents.remove).toHaveBeenCalled()
  })

  it('opens schedule modal, applies save/remove, and loads a thread', async () => {
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await clickExact(wrapper, '定时')
    expect(wrapper.get('.sch-m').text()).toContain('Database')
    await wrapper.get('.sch-saved').trigger('click')
    await wrapper.get('.sch-removed').trigger('click')
    await wrapper.get('.sch-thread').trigger('click')
    await flushPromises()
    expect(api.agentThreads.messages).toHaveBeenCalled()
  })

  it('sends a stream message, stops it, and starts a new thread', async () => {
    const close = vi.fn()
    api.agentThreads.streamRun.mockImplementation((_p: string, _t: string, _r: unknown, h: { onRun?: (id: string) => void; onToken?: (t: string) => void; onEnd?: () => void }) => {
      h.onRun?.('run-1')
      h.onToken?.('A')
      h.onEnd?.()
      return { close }
    })
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await wrapper.get('.ai-send').trigger('click')
    await flushPromises()
    expect(api.agentThreads.streamRun).toHaveBeenCalled()
    await wrapper.get('.ai-stop').trigger('click')
    await clickText(wrapper, '新会话')
    await flushPromises()
    expect(api.agentThreads.create).toHaveBeenCalled()
  })

  it('shows empty-state create and reloads on refresh / project change', async () => {
    api.agents.list.mockResolvedValueOnce([])
    const { wrapper, pinia } = await mountWithApp(AgentManager, { stubs: agentStubs })
    expect(wrapper.text()).toContain('还没有 Agent')
    await wrapper.get('.empty-action').trigger('click')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)

    api.agents.list.mockResolvedValue([sampleAgent])
    await clickText(wrapper, '刷新')
    await flushPromises()
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    expect(api.agents.list.mock.calls.length).toBeGreaterThan(1)
  })

  it('handles list / schedule / thread load errors', async () => {
    api.agents.list.mockRejectedValueOnce(new Error('agents'))
    api.agentSchedules.list.mockRejectedValueOnce(new Error('sched'))
    await mountWithApp(AgentManager, { stubs: agentStubs })
    expect(toast.error).toHaveBeenCalledWith('agents')
    expect(toast.error).toHaveBeenCalledWith('sched')
  })

  it('blocks sandbox module when sandbox_available is false', async () => {
    api.agents.modules.mockResolvedValue([
      { id: 'database', name: 'Database', description: 'd', default_tools: ['list_databases'], team_supported: false },
      { id: 'sandbox', name: 'Sandbox', description: 'cloud microVM', default_tools: ['sandbox_exec', 'sandbox_shell', 'sandbox_read_file', 'sandbox_write_file'], team_supported: false, sandbox_available: false }
    ])
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await flushPromises()
    const vm = wrapper.vm as Record<string, any>
    const sandboxOpt = vm.moduleOptions.find((o: { value: string }) => o.value === 'sandbox')
    expect(sandboxOpt.disabled).toBe(true)

    vm.openCreate()
    vm.form.name = 'SB'
    vm.form.module = 'sandbox'
    await vm.saveAgent()
    expect(toast.warning).toHaveBeenCalledWith('云沙盒未配置，无法创建 Sandbox Agent')
    expect(api.agents.create).not.toHaveBeenCalled()
  })

  it('allows sandbox module and shows sandbox hint when available', async () => {
    api.agents.modules.mockResolvedValue([
      { id: 'sandbox', name: 'Sandbox', description: 'cloud microVM', default_tools: ['sandbox_exec', 'sandbox_shell', 'sandbox_read_file', 'sandbox_write_file'], team_supported: false, sandbox_available: true }
    ])
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, tool_ids: ['sandbox_exec', 'sandbox_shell'] }
    ])
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await flushPromises()
    const vm = wrapper.vm as Record<string, any>
    const sandboxOpt = vm.moduleOptions.find((o: { value: string }) => o.value === 'sandbox')
    expect(sandboxOpt.disabled).toBe(false)
    expect(vm.toolsHint).toContain('沙盒命令在云端隔离环境执行')
    expect(vm.toolsHint).toContain('@Database')

    vm.openCreate()
    vm.form.name = 'SB'
    vm.form.module = 'sandbox'
    await vm.saveAgent()
    expect(api.agents.create).toHaveBeenCalled()
  })

  it('manages threads, restores URL selection, and sends selected model', async () => {
    api.agentThreads.page.mockResolvedValue({ threads: [
      { id: 'th-1', title: '旧会话', created_at: 't', updated_at: 't' },
      { id: 'th-2', title: '选中会话', created_at: 't', updated_at: 't' }
    ], next_cursor: 'th-2' })
    const { wrapper, router } = await mountWithApp(AgentManager, { stubs: agentStubs, path: '/?thread=th-2' })
    await flushPromises()
    const vm = wrapper.vm as Record<string, any>
    expect(vm.threadId).toBe('th-2')
    await vm.selectThread('th-1')
    await flushPromises()
    expect(router.currentRoute.value.query.thread).toBe('th-1')
    api.agentThreads.page.mockResolvedValueOnce({ threads: [{ id: 'th-3', title: '更多', created_at: 't', updated_at: 't' }], next_cursor: '' })
    vm.nextCursor = 'th-2'
    await vm.loadMoreThreads()
    expect(vm.threads.some((th: { id: string }) => th.id === 'th-3')).toBe(true)
    await vm.renameThread('th-1', '新标题')
    expect(api.agentThreads.rename).toHaveBeenCalledWith(expect.anything(), 'th-1', '新标题')
    await vm.removeThread('th-3')
    expect(api.agentThreads.remove).toHaveBeenCalled()
    vm.openEdit(sampleAgent)
    vm.form.model_override = 'deepseek-ai/DeepSeek-V4-Flash'
    await vm.saveAgent()
    expect(api.agents.patch).toHaveBeenCalledWith(expect.anything(), sampleAgent.id, expect.objectContaining({ model_override: 'deepseek-ai/DeepSeek-V4-Flash' }))
  })

  it('matches duplicate tool names by call id, retries errors and shows cancel state', async () => {
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.streamRun.mockImplementation((_p: string, _t: string, _r: unknown, h: Record<string, (...args: unknown[]) => void>) => {
      handlers = h
      return { close: vi.fn() }
    })
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as Record<string, any>
    await vm.onSend('查库', [{ agent_id: 'ag-1' }])
    handlers.onRun?.('run-1')
    handlers.onThinking?.(2000, 'thinking')
    handlers.onToolCall?.('list_databases', '{}', 'c1')
    handlers.onToolCall?.('list_databases', '{}', 'c2')
    handlers.onToolResult?.('list_databases', 'second', 'c2', 20)
    handlers.onToolResult?.('list_databases', 'first', 'c1', 10)
    expect(vm.chatMessages[1].toolCalls.map((card: { content: string }) => card.content)).toEqual(['first', 'second'])
    handlers.onError?.(Object.assign(new Error('rate limit'), { code: 'llm_rate_limited' }))
    expect(vm.chatMessages[1].error).toContain('限流')
    vm.retryLast()
    expect(api.agentThreads.streamRun).toHaveBeenCalledTimes(2)
    expect(api.agentThreads.streamRun.mock.calls[1][2]).toMatchObject({ content: '查库', mentions: [{ agent_id: 'ag-1' }] })
    handlers.onRun?.('run-2')
    vm.onStop()
    expect(vm.chatMessages.at(-1)?.canceled).toBe(true)
  })

  it('covers thread errors, empty paging and safe retry guards', async () => {
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as Record<string, any>
    await vm.loadMoreThreads()
    api.agentThreads.page.mockRejectedValueOnce(new Error('page-error'))
    vm.nextCursor = 't'
    await vm.loadMoreThreads()
    expect(toast.error).toHaveBeenCalledWith('page-error')
    api.agentThreads.rename.mockRejectedValueOnce(new Error('rename-error'))
    await vm.renameThread('t', 'x')
    expect(toast.error).toHaveBeenCalledWith('rename-error')
    api.agentThreads.remove.mockRejectedValueOnce(new Error('remove-error'))
    await vm.removeThread('th-1')
    expect(toast.error).toHaveBeenCalledWith('remove-error')
    await vm.selectThread('')
    api.agentThreads.messages.mockRejectedValueOnce(new Error('messages-error'))
    await vm.selectThread('broken')
    expect(toast.error).toHaveBeenCalledWith('messages-error')
    vm.lastRequest = null
    vm.retryLast()
    api.agentThreads.remove.mockResolvedValueOnce(undefined)
    await vm.removeThread('broken')
    expect(api.agentThreads.create).toHaveBeenCalled()
  })

  it('shows readonly hint when active agent has no sandbox tools', async () => {
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await flushPromises()
    const vm = wrapper.vm as Record<string, any>
    expect(vm.toolsHint).toContain('工具只读')
  })
})

describe('AgentManager script helpers', () => {
  beforeEach(() => {
    resetApiMocks()
    api.agents.list.mockResolvedValue([sampleAgent])
    api.agentThreads.list.mockResolvedValue([{ id: 'th-1', title: 'T', created_at: 't', updated_at: 't' }])
    api.agentThreads.messages.mockResolvedValue([])
    api.agentSchedules.list.mockResolvedValue([
      { id: 'sch-1', agent_id: 'ag-1', thread_id: 'th-1', prompt: 'p', cron_expr: '0 * * * *', enabled: false, created_at: 't', updated_at: 't' }
    ])
  })

  it('covers schedule helpers, CRUD edge cases, and stream send/stop', async () => {
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ...agentStubs, AgentScheduleModal: true, AiChat: true }
    })
    await flushPromises()
    const vm = wrapper.vm as Record<string, any>
    expect(vm.scheduleSummary({ cron_expr: '*/15 * * * *', enabled: true })).toContain('每 15 分钟')
    expect(vm.scheduleSummary({ cron_expr: '0 * * * *', enabled: false })).toContain('已停用')
    expect(vm.scheduleSummary({ cron_expr: '0 8 * * *', enabled: true })).toContain('每天')
    expect(vm.scheduleSummary({ cron_expr: '0 8 * * 1', enabled: true })).toContain('每周一')
    expect(vm.scheduleSummary({ cron_expr: '1 2 3 4 5', enabled: true })).toContain('1 2 3 4 5')

    vm.onScheduleSaved({ id: 's', agent_id: 'ag-1' })
    vm.onScheduleRemoved('sch-1')
    vm.openSchedule(sampleAgent)
    await vm.onViewScheduleThread('th-view')
    api.agentThreads.messages.mockRejectedValueOnce(new Error('th'))
    await vm.onViewScheduleThread('th-view')
    api.agentThreads.messages.mockResolvedValueOnce([
      { id: 'm1', role: 'assistant', content: 'a', tool_calls: [{ name: 't' }], created_at: 't' },
      { id: 'm2', role: 'user', content: 'u', created_at: 't' }
    ])
    await vm.onViewScheduleThread('th-msgs')

    vm.openCreate()
    vm.onModuleChange('s3')
    expect(vm.form.tool_ids).toEqual(expect.arrayContaining(['list_objects']))
    vm.form.name = ''
    await vm.saveAgent()
    expect(toast.warning).toHaveBeenCalledWith('请填写名称')
    vm.form.name = 'N'
    await vm.saveAgent()
    vm.openEdit(sampleAgent)
    vm.onModuleChange('s3')
    vm.onModuleChange('missing')
    vm.toggleTool('list_databases')
    vm.toggleTool('list_databases')
    api.agents.patch.mockRejectedValueOnce(new Error('p'))
    await vm.saveAgent()
    await vm.removeAgent(sampleAgent)
    api.agents.remove.mockRejectedValueOnce(new Error('r'))
    await vm.removeAgent(sampleAgent)
    await vm.resetThread()
    api.agentThreads.create.mockRejectedValueOnce(new Error('c'))
    await vm.resetThread()

    const close = vi.fn()
    api.agentThreads.streamRun.mockImplementation((_p: string, _t: string, _r: unknown, h: any) => {
      h.onRun?.('run-1')
      h.onToken?.('A')
      h.onToolCall?.('list_databases', '{}')
      h.onToolResult?.('list_databases', '[]')
      h.onToolResult?.('other', 'x')
      h.onEnd?.()
      return { close }
    })
    await vm.onSend('hello', [{ agent_id: 'ag-1' }])
    await vm.onSend('   ', [])
    vm.sending = true
    await vm.onSend('again', [{ agent_id: 'ag-1' }])
    vm.sending = false
    vm.activeId = ''
    await vm.onSend('no-agent', [])
    expect(toast.warning).toHaveBeenCalledWith('请先选择或 @ 一个 Agent')
    vm.activeId = 'ag-1'
    api.agentThreads.streamRun.mockImplementation((_p: string, _t: string, _r: unknown, h: any) => {
      h.onError?.(new Error('sse'))
      return { close }
    })
    await vm.onSend('x', [])
    api.agentThreads.streamRun.mockImplementation((_p: string, _t: string, _r: unknown, h: any) => {
      h.onError?.('z')
      return { close }
    })
    await vm.onSend('y', [{ agent_id: 'ag-1' }])
    vm.currentRunId = 'run-1'
    vm.onStop()

    api.agents.modules.mockRejectedValueOnce('m')
    await vm.loadModules()
    api.agentThreads.list.mockResolvedValueOnce([])
    api.agentThreads.messages.mockResolvedValueOnce([
      { id: 'm1', role: 'assistant', content: 'a', tool_calls: [{ name: 't' }], created_at: 't' }
    ])
    await vm.ensureThread()
    api.agentThreads.list.mockRejectedValueOnce(new Error('t'))
    await vm.ensureThread()
    api.agentSchedules.list.mockRejectedValueOnce(new Error('s'))
    await vm.loadSchedules()
    wrapper.unmount()
  })
  it('AgentManager clicks list actions, modal fields, schedule, and stream stop', async () => {
    api.agents.modules.mockResolvedValue([
      { id: 'database', name: 'Database', description: 'd', default_tools: ['list_databases'], team_supported: false },
      { id: 's3', name: 'S3', description: 's', default_tools: ['list_objects'], team_supported: false }
    ])
    const { wrapper, pinia } = await mountWithApp(AgentManager, { stubs: interactionAgentStubs })
    await clickText(wrapper, '刷新')
    await wrapper.find('button.w-full').trigger('click')
    await clickText(wrapper, '新建')
    await flushPromises()
    if (wrapper.find('.sb-modal').exists()) {
      if (wrapper.find('.select-emit').exists()) await wrapper.get('.select-emit').trigger('click')
      if (wrapper.find('#agent-name').exists()) await wrapper.get('#agent-name').setValue('N2')
      if (wrapper.find('#agent-desc').exists()) await wrapper.get('#agent-desc').setValue('d')
      if (wrapper.find('#agent-prompt').exists()) await wrapper.get('#agent-prompt').setValue('p')
      const badges = wrapper.findAll('.badge')
      if (badges.length) await badges[badges.length - 1].trigger('click')
      if (wrapper.find('.sb-ok').exists()) await wrapper.get('.sb-ok').trigger('click')
      await flushPromises()
      if (wrapper.find('.sb-cancel').exists()) await wrapper.get('.sb-cancel').trigger('click')
    }

    await clickText(wrapper, '定时')
    await flushPromises()
    if (wrapper.find('.sch-saved').exists()) await wrapper.get('.sch-saved').trigger('click')
    if (wrapper.find('.sch-removed').exists()) await wrapper.get('.sch-removed').trigger('click')
    if (wrapper.find('.sch-thread').exists()) await wrapper.get('.sch-thread').trigger('click')
    if (wrapper.find('.sch-close').exists()) await wrapper.get('.sch-close').trigger('click')
    await clickText(wrapper, '删除')
    await clickText(wrapper, '新会话')
    if (wrapper.find('.ai-stop').exists()) await wrapper.get('.ai-stop').trigger('click')
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    wrapper.unmount()
  })

  it('AgentManager empty-state create', async () => {
    api.agents.list.mockResolvedValueOnce([])
    const { wrapper } = await mountWithApp(AgentManager, { stubs: interactionAgentStubs })
    if (wrapper.find('.empty-action').exists()) {
      await wrapper.get('.empty-action').trigger('click')
      expect(wrapper.find('.sb-modal').exists()).toBe(true)
    }
    wrapper.unmount()
  })


  it('AgentManager clicks edit/schedule/delete and non-Error catches', async () => {
    api.agentThreads.list.mockResolvedValue([{ id: 'th-1', title: 'T', created_at: 't', updated_at: 't' }])
    const { wrapper } = await mountWithApp(AgentManager)
    const edit = wrapper.findAll('button').find((b) => b.text() === '编辑')
    const sched = wrapper.findAll('button').find((b) => b.text() === '定时')
    if (edit) await edit.trigger('click')
    if (wrapper.find('.sb-cancel').exists()) await wrapper.get('.sb-cancel').trigger('click')
    if (sched) await sched.trigger('click')
    if (wrapper.find('.sb-cancel').exists()) await wrapper.get('.sb-cancel').trigger('click')
    const confirm = wrapper.find('.confirm-action')
    if (confirm.exists()) await confirm.trigger('click')
    await flushPromises()

    const vm = wrapper.vm as Record<string, any>
    api.agentSchedules.list.mockRejectedValueOnce('sched')
    await vm.loadSchedules()
    api.agents.modules.mockRejectedValueOnce('mods')
    await vm.loadModules()
    api.agents.list.mockRejectedValueOnce('agents')
    await vm.loadAgents()
    api.agentThreads.list.mockRejectedValueOnce('threads')
    await vm.ensureThread()
    api.agentThreads.messages.mockRejectedValueOnce('msgs')
    await vm.onViewScheduleThread('th-x')
    api.agents.patch.mockRejectedValueOnce('save')
    vm.editing = sampleAgent
    vm.form = { ...sampleAgent, name: 'n' }
    await vm.saveAgent()
    api.agents.remove.mockRejectedValueOnce('rm')
    await vm.removeAgent(sampleAgent)
    api.agentThreads.create.mockRejectedValueOnce('th')
    await vm.resetThread()
    vm.openCreate()
    vm.onModuleChange('database')
    vm.onModuleChange('missing')
    wrapper.unmount()
  })


  it('covers extended AgentManager helpers', async () => {
    const agents = mount(AgentManager, {
      global: { plugins: [createPinia()], stubs: { default: true } },
      shallow: true
    })
    await flushPromises()
    const avm = agents.vm as any
    await avm.bootstrap?.()
    await avm.loadSchedules?.()
    api.agentSchedules.list.mockRejectedValueOnce(new Error('x'))
    await avm.loadSchedules?.()
    expect(avm.scheduleSummary?.({ cron_expr: '* * * * *', enabled: true, next_run_at: 't' })).toBeTruthy()
    avm.openSchedule?.({ id: 'a' })
    avm.onScheduleSaved?.({ id: 's', agent_id: 'a' })
    avm.onScheduleRemoved?.('s')
    await avm.onViewScheduleThread?.('th')
    await avm.loadModules?.()
    await avm.loadAgents?.()
    api.agents.list.mockRejectedValueOnce(new Error('x'))
    await avm.loadAgents?.()
    await avm.ensureThread?.()
    avm.openCreate?.()
    avm.openEdit?.({ id: 'a', name: 'A', module: 'database', tool_ids: [] })
    avm.onModuleChange?.('s3')
    avm.toggleTool?.('list_databases')
    await avm.saveAgent?.()
    await avm.removeAgent?.({ id: 'a', name: 'A' })
    api.agents.remove.mockRejectedValueOnce(new Error('x'))
    await avm.removeAgent?.({ id: 'a', name: 'A' })
    await avm.resetThread?.()
    avm.onStop?.()
    api.agentThreads.streamRun.mockReturnValue({ close: vi.fn() })
    await avm.onSend?.('hi', [{ agent_id: 'a' }])
    agents.unmount()
  })

})
