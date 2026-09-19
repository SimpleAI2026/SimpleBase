import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useProjectStore } from '../stores/project'
import { api, resetApiMocks } from '../test/api-mock'
import { sampleCron, uiStubs } from '../test/helpers'
import CronJobRunsDrawer from './CronJobRunsDrawer.vue'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return { api: m.api, isMock: false }
})

describe('CronJobRunsDrawer', () => {
  beforeEach(() => {
    resetApiMocks()
    setActivePinia(createPinia())
    useProjectStore().setProject('00000000-0000-0000-0000-000000000002')
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
    vi.useFakeTimers()
  })

  afterEach(() => vi.useRealTimers())

  it('covers filter helpers, pretty json, copy, load and polling trigger', async () => {
    api.cronjobs.runs.mockResolvedValue([
      {
        id: 'r1',
        jobId: 'cj-1',
        trigger: 'manual',
        status: 'completed',
        error: 'e',
        durationMs: 1,
        responseJson: '{"a":1}',
        createdAt: 't'
      }
    ])
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore().setProject('00000000-0000-0000-0000-000000000002')
    const w = mount(CronJobRunsDrawer, {
      props: { open: true, job: sampleCron },
      global: { plugins: [pinia], stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    expect(vm.runVariant('completed')).toBe('default')
    expect(vm.runVariant('failed')).toBe('destructive')
    expect(vm.runVariant('running')).toBe('secondary')
    expect(vm.runText('completed')).toBe('成功')
    expect(vm.runText('failed')).toBe('失败')
    expect(vm.runText('running')).toBe('执行中')
    expect(vm.runText('')).toBe('未知')
    expect(vm.runIcon('completed')).toBe('●')
    expect(vm.runIcon('failed')).toBe('✕')
    expect(vm.runIcon('x')).toBe('◌')
    expect(vm.prettyJson('{"a":1}')).toContain('a')
    expect(vm.prettyJson('nope')).toBe('nope')
    await vm.copy('x')

    vm.statusFilter = 'failed'
    expect(vm.filteredRuns.length).toBe(0)
    vm.statusFilter = 'all'

    api.cronjobs.runs
      .mockResolvedValueOnce([{ id: 'r', status: 'running' }])
      .mockResolvedValueOnce([{ id: 'r', status: 'completed' }])
    const trig = vm.trigger()
    await vi.advanceTimersByTimeAsync(1500)
    await trig
    expect(api.cronjobs.trigger).toHaveBeenCalled()

    api.cronjobs.runs.mockRejectedValueOnce(new Error('runs'))
    await vm.load()
    api.cronjobs.trigger.mockRejectedValueOnce(new Error('trig'))
    await vm.trigger()
    await w.setProps({ open: false, job: undefined })
    await vm.load()
    await vm.trigger()
    w.unmount()
  })
})
