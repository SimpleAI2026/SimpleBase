import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { useProjectStore } from '../stores/project'
import { api, resetApiMocks } from '../test/api-mock'
import { sampleAgent, uiStubs } from '../test/helpers'
import AgentManager from './AgentManager.vue'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return { api: m.api, isMock: false }
})

async function mountPage() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', name: 'agents', component: { template: '<div />' } }]
  })
  await router.push('/')
  await router.isReady()
  const pinia = createPinia()
  setActivePinia(pinia)
  useProjectStore().setProject('00000000-0000-0000-0000-000000000002')
  api.agents.list.mockResolvedValue([sampleAgent])
  api.agentThreads.list.mockResolvedValue([{ id: 'th-1', title: 'T', created_at: 't', updated_at: 't' }])
  api.agentThreads.messages.mockResolvedValue([])
  api.agentSchedules.list.mockResolvedValue([
    { id: 'sch-1', agent_id: 'ag-1', thread_id: 'th-1', prompt: 'p', cron_expr: '0 * * * *', enabled: false, created_at: 't', updated_at: 't' }
  ])
  return mount(AgentManager, {
    global: { plugins: [router, pinia], stubs: { ...uiStubs, AiChat: true, AgentScheduleModal: true } }
  })
}

describe('AgentManager script', () => {
  beforeEach(() => resetApiMocks())

  it('covers schedule helpers, CRUD, and stream send/stop', async () => {
    const w = await mountPage()
    await flushPromises()
    const vm = w.vm as any
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

    vm.openCreate()
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
    await vm.ensureThread()
    api.agentThreads.list.mockRejectedValueOnce(new Error('t'))
    await vm.ensureThread()
    api.agentSchedules.list.mockRejectedValueOnce(new Error('s'))
    await vm.loadSchedules()
    w.unmount()
  })
})
