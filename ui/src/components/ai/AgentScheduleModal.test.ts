import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '../../test/api-mock'
import { sampleAgent, uiStubs } from '../../test/helpers'
import AgentScheduleModal from './AgentScheduleModal.vue'

vi.mock('../../services/api', async () => {
  const m = await import('../../test/api-mock')
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
    expect(vm.statusVariant('completed')).toBe('default')
    expect(vm.statusVariant('failed')).toBe('destructive')
    expect(vm.statusVariant('running')).toBe('secondary')
    expect(vm.statusVariant('queued')).toBe('secondary')
    expect(vm.statusVariant('x')).toBe('outline')
    expect(vm.formatTime('')).toBe('')
    expect(vm.formatTime('bad')).toBe('')
    expect(vm.formatTime('2024-01-01T00:00:00Z')).toContain('UTC')
    expect(vm.shortTime('')).toBe('—')
    expect(vm.shortTime('bad')).toBe('—')
    expect(vm.shortTime('2024-01-15T08:05:00Z')).toMatch(/\d{2}-\d{2}/)
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
    await empty.setProps({ open: true, schedule: { ...schedule, cron_expr: '1 2 3 4 5 6' } })
    w.unmount()
    create.unmount()
    empty.unmount()
  })
})
