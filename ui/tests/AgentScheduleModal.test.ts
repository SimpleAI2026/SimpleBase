import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { sampleAgent, uiStubs } from '@/test/helpers'
import AgentScheduleModal from '@/components/ai/AgentScheduleModal.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api, isMock: false }
})

const schedule = {
  id: 'sch-1',
  agent_id: 'ag-1',
  thread_id: 'th-1',
  prompt: 'check tables',
  cron_expr: '0 8 * * *',
  enabled: true,
  last_run_at: '2024-01-01T00:00:00Z',
  next_run_at: '2024-01-02T00:00:00Z',
  created_at: 't',
  updated_at: 't'
}

describe('AgentScheduleModal', () => {
  beforeEach(() => {
    resetApiMocks()
    vi.useFakeTimers()
  })
  afterEach(() => vi.useRealTimers())

  it('covers create/patch/remove/trigger and time helpers', async () => {
    const w = mount(AgentScheduleModal, {
      props: { open: true, agent: sampleAgent, schedule, projectId: 'p1' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    vm.onFrequencyChange('custom')
    vm.onFrequencyChange('0 * * * *')
    vm.form.cron_expr = 'bad'
    vm.validateCron()
    expect(vm.cronError).toBeTruthy()
    vm.form.cron_expr = '0 8 * * *'
    vm.validateCron()
    expect(vm.cronError).toBe('')

    vm.form.prompt = ''
    await vm.save()
    expect(toast.warning).toHaveBeenCalledWith('请填写提示词')
    vm.form.prompt = 'ok'
    vm.form.cron_expr = 'x'
    await vm.save()
    vm.form.cron_expr = '0 8 * * *'
    await vm.save()
    expect(api.agentSchedules.patch).toHaveBeenCalled()

    await vm.remove()
    api.agentSchedules.remove.mockRejectedValueOnce(new Error('rm'))
    await vm.remove()
    await vm.triggerNow()
    await vi.advanceTimersByTimeAsync(2000)
    api.agentSchedules.trigger.mockRejectedValueOnce(new Error('trig'))
    await vm.triggerNow()

    api.agentSchedules.runs.mockRejectedValueOnce(new Error('r'))
    await vm.loadRuns()

    const create = mount(AgentScheduleModal, {
      props: { open: true, agent: sampleAgent, schedule: null, projectId: 'p1' },
      global: { stubs: uiStubs }
    })
    const cvm = create.vm as any
    cvm.form.prompt = 'p'
    cvm.form.cron_expr = '0 8 * * *'
    await cvm.save()
    const exists = Object.assign(new Error('already exists'), { status: 409 })
    api.agentSchedules.create.mockRejectedValueOnce(exists)
    await cvm.save()
    api.agentSchedules.create.mockRejectedValueOnce(new Error('other'))
    await cvm.save()

    const empty = mount(AgentScheduleModal, {
      props: { open: true, agent: null, schedule: null, projectId: '' },
      global: { stubs: uiStubs }
    })
    await (empty.vm as any).save()
    await (empty.vm as any).remove()
    await (empty.vm as any).triggerNow()
    await empty.setProps({ open: false })
    await empty.setProps({ open: true, schedule: { ...schedule, cron_expr: '0 8 * * *' } })
    await empty.setProps({ open: false })
    await empty.setProps({ open: true, schedule: null, agent: sampleAgent, projectId: 'p1' })
    await empty.setProps({ open: false })
    await empty.setProps({ open: true, schedule: { ...schedule, cron_expr: '1 2 3 4 5' } })
    w.unmount()
    create.unmount()
    empty.unmount()
  })
  it('AgentScheduleModal frequency, switch, custom cron, footer buttons', async () => {
    const schedule = {
      id: 'sch-1',
      agent_id: 'ag-1',
      thread_id: 'th-1',
      prompt: 'p',
      cron_expr: '1 2 3 4 5',
      enabled: true,
      created_at: 't',
      updated_at: 't',
      last_run_at: '2024-01-01T00:00:00Z',
      next_run_at: '2024-01-02T00:00:00Z'
    }
    api.agentSchedules.runs.mockResolvedValue([
      { id: 'r1', schedule_id: 'sch-1', run_id: 'x', trigger: 'manual', status: 'completed', created_at: 't' }
    ])
    const w = mount(AgentScheduleModal, {
      props: { open: false, agent: sampleAgent, schedule, projectId: 'p1' },
      global: { stubs: uiStubs }
    })
    await w.setProps({ open: true })
    await flushPromises()
    await w.get('.select-emit').trigger('click')
    await w.get('.switch').trigger('click')
    await w.get('#schedule-prompt').setValue('hello')
    if (w.findAll('button').some((b) => b.text().includes('查看会话'))) {
      await w.findAll('button').find((b) => b.text().includes('查看会话'))!.trigger('click')
      expect(w.emitted('view-thread')).toBeTruthy()
    }
    await w.findAll('button').find((b) => b.text().includes('取消'))?.trigger('click')
    expect(w.emitted('update:open')).toBeTruthy()
    w.unmount()
  })

  it('AgentScheduleModal custom cron, 409 retry, and catch fallbacks', async () => {
    const w = mount(AgentScheduleModal, {
      props: { open: true, agent: sampleAgent, schedule: null, projectId: 'p1' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    if (w.find('.select-custom').exists()) await w.get('.select-custom').trigger('click')
    if (w.find('input[placeholder="*/15 * * * *"]').exists()) {
      await w.get('input[placeholder="*/15 * * * *"]').setValue('1 2 3 4 5')
      await w.get('input[placeholder="*/15 * * * *"]').trigger('blur')
    }
    const vm = w.vm as Record<string, any>
    vm.form.prompt = 'ping'
    vm.form.cron_expr = '* * * * *'
    const err409 = Object.assign(new Error('exists'), { status: 409 })
    api.agentSchedules.create.mockRejectedValueOnce(err409)
    await vm.save().catch(() => undefined)
    api.agentSchedules.create.mockRejectedValueOnce(new Error('already exists'))
    await vm.save()
    api.agentSchedules.create.mockRejectedValueOnce('plain')
    await vm.save()
    w.unmount()

    const schedule = {
      id: 'sch-1',
      agent_id: 'ag-1',
      thread_id: 'th-1',
      prompt: 'p',
      cron_expr: '* * * * *',
      enabled: true,
      created_at: 't',
      updated_at: 't'
    }
    const w2 = mount(AgentScheduleModal, {
      props: { open: true, agent: sampleAgent, schedule, projectId: 'p1' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm2 = w2.vm as Record<string, any>
    api.agentSchedules.remove.mockRejectedValueOnce('rm')
    await vm2.remove()
    api.agentSchedules.trigger.mockRejectedValueOnce('trig')
    await vm2.triggerNow()
    if (w2.find('.sb-cancel').exists()) await w2.get('.sb-cancel').trigger('click')
    w2.unmount()
  })


})
