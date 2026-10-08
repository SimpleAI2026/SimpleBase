import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useProjectStore } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'
import { sampleCron, uiStubs } from '@/test/helpers'
import CronJobRunsModal from '@/components/modal/CronJobRunsModal.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

/** 带关闭按钮的 SbModal stub，用于触发 update:open */
const modalStubs = {
  ...uiStubs,
  SbModal: {
    props: ['open', 'title', 'description', 'maxWidth', 'hideFooter'],
    emits: ['update:open'],
    template:
      '<div v-if="open" class="sb-modal"><button type="button" class="modal-close" @click="$emit(\'update:open\', false)">x</button><slot /></div>'
  }
}

describe('CronJobRunsModal', () => {
  beforeEach(() => {
    resetApiMocks()
    setActivePinia(createPinia())
    useProjectStore().setProject('dev-shop')
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
        finishedAt: '2024-01-01T00:00:01Z',
        createdAt: 't'
      }
    ])
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore().setProject('dev-shop')
    const w = mount(CronJobRunsModal, {
      props: { open: false, job: sampleCron },
      global: { plugins: [pinia], stubs: modalStubs }
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

    // 打开（非 autoTrigger）只加载不触发
    await w.setProps({ open: true })
    await flushPromises()
    expect(api.cronjobs.runs).toHaveBeenCalled()
    expect(api.cronjobs.trigger).not.toHaveBeenCalled()

    // 工具栏按钮直点：刷新 / 立即执行
    await w.findAll('button').find((b) => b.text().includes('刷新'))?.trigger('click')
    await flushPromises()
    await w.findAll('button').find((b) => b.text().includes('立即执行'))?.trigger('click')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(api.cronjobs.trigger).toHaveBeenCalled()

    vm.statusFilter = 'failed'
    expect(vm.filteredRuns.length).toBe(0)
    vm.statusFilter = 'all'

    // 复制按钮、状态筛选、弹窗关闭
    for (const b of w.findAll('button').filter((x) => x.text().includes('复制'))) {
      await b.trigger('click')
    }
    expect(navigator.clipboard.writeText).toHaveBeenCalled()
    await w.get('.select-emit').trigger('click')
    expect(vm.statusFilter).toBe('info')
    vm.statusFilter = 'all'
    await w.get('.modal-close').trigger('click')
    expect(w.emitted('update:open')).toBeTruthy()

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

  it('autoTrigger 打开即触发并轮询，关闭后停止轮询', async () => {
    api.cronjobs.runs.mockResolvedValue([{ id: 'r', status: 'running' }])
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore().setProject('dev-shop')
    const w = mount(CronJobRunsModal, {
      props: { open: false, job: sampleCron, autoTrigger: true },
      global: { plugins: [pinia], stubs: uiStubs }
    })
    await flushPromises()
    expect(api.cronjobs.trigger).not.toHaveBeenCalled()

    await w.setProps({ open: true })
    await flushPromises()
    expect(api.cronjobs.trigger).toHaveBeenCalled()

    // 记录仍 running：保持打开时继续轮询
    const before = api.cronjobs.runs.mock.calls.length
    await vi.advanceTimersByTimeAsync(1600)
    expect(api.cronjobs.runs.mock.calls.length).toBeGreaterThan(before)

    // 关闭后立即停止轮询
    const callsAtClose = api.cronjobs.runs.mock.calls.length
    await w.setProps({ open: false })
    await vi.advanceTimersByTimeAsync(10000)
    expect(api.cronjobs.runs.mock.calls.length).toBe(callsAtClose)
    w.unmount()
  })

  it('CronJobRunsModal copy, filter, modal close', async () => {
    api.cronjobs.runs.mockResolvedValue([
      {
        id: 'r1',
        jobId: 'cj-1',
        trigger: 'scheduled',
        status: 'failed',
        error: 'boom',
        durationMs: 2,
        responseJson: '{"ok":1}',
        finishedAt: '2024-01-01T00:00:00Z',
        createdAt: 't'
      }
    ])
    const w = mount(CronJobRunsModal, {
      props: { open: true, job: sampleCron },
      global: {
        plugins: [createPinia()],
        stubs: {
          ...uiStubs,
          SbModal: {
            props: ['open', 'title', 'description', 'maxWidth', 'hideFooter'],
            emits: ['update:open'],
            template:
              '<div v-if="open" class="sb-modal"><button type="button" class="modal-close" @click="$emit(\'update:open\', false)">x</button><slot /></div>'
          }
        }
      }
    })
    await flushPromises()
    const copies = w.findAll('button').filter((b) => b.text().includes('复制'))
    for (const c of copies) await c.trigger('click')
    if (w.find('.select-emit').exists()) await w.get('.select-emit').trigger('click')
    if (w.find('.modal-close').exists()) await w.get('.modal-close').trigger('click')
    expect(w.emitted('update:open')).toBeTruthy()
    w.unmount()
  })

  it('CronJobRunsModal copy clicks, load/trigger fallbacks, empty job', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore().setProject('dev-shop')
    api.cronjobs.runs.mockResolvedValue([
      {
        id: 'r1',
        jobId: 'cj-1',
        trigger: 'scheduled',
        status: 'failed',
        error: 'boom',
        durationMs: 2,
        responseJson: 'not-json',
        finishedAt: '2024-01-01T00:00:00Z',
        createdAt: 't'
      }
    ])
    const w = mount(CronJobRunsModal, {
      props: { open: true, job: sampleCron },
      global: { plugins: [pinia], stubs: uiStubs }
    })
    await flushPromises()
    for (const b of w.findAll('button')) {
      if (b.text().includes('复制')) await b.trigger('click')
    }
    const vm = w.vm as Record<string, any>
    api.cronjobs.runs.mockRejectedValueOnce('load')
    await vm.load()
    api.cronjobs.trigger.mockRejectedValueOnce('trig')
    await vm.trigger()
    w.unmount()

    const empty = mount(CronJobRunsModal, {
      props: { open: true },
      global: { plugins: [pinia], stubs: uiStubs }
    })
    await flushPromises()
    empty.unmount()
  })


})
