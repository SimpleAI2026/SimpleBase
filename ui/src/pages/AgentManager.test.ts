import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { useProjectStore } from '../stores/project'
import { api, resetApiMocks } from '../test/api-mock'
import { clickText, mountWithApp, sampleAgent } from '../test/helpers'
import AgentManager from './AgentManager.vue'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return { api: m.api, isMock: false }
})

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
    expect(wrapper.text()).toContain('按模块的只读 Agent')
    expect(wrapper.text()).toContain('Database')
    expect(wrapper.text()).toContain('每小时')
    expect(wrapper.text()).toContain('已启用')
    await wrapper.find('button.w-full').trigger('click')
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
    const badges = wrapper.findAll('.badge')
    if (badges.length) await badges[badges.length - 1].trigger('click')
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
})
