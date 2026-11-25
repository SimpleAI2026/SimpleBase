import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, installJsdomPolyfills, mountWithApp, openPopover, pickSelectOption, popoverEl, realPopoverStubs, realSelectStubs, selectTriggers } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

import Logs from '@/pages/Logs.vue'

installJsdomPolyfills()

/** 保留策略在卡片头的真实 Popover 里（弹层 teleport 到 body）：挂到 document，并保留共享桩给其余控件。 */
const mountLogs = () => mountWithApp(Logs, { attachTo: document.body, stubs: { ...realPopoverStubs, ...realSelectStubs, Button: false } })
const retentionInput = () => popoverEl<HTMLInputElement>('input[type="number"]')!

describe('Logs (日志管理)', () => {
  beforeEach(() => {
    resetApiMocks()
    vi.useFakeTimers()
    api.logs.list.mockResolvedValue([
      {
        id: 'l1',
        projectId: 'p',
        level: 'error',
        logger: 'http',
        message: 'boom',
        requestId: 'r1',
        occurredAt: '2024-01-01T00:00:00Z'
      }
    ])
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('loads events and retention, then saves a new keep-days value', async () => {
    const { wrapper } = await mountLogs()
    expect(wrapper.text()).toContain('boom')
    expect(wrapper.findAll('th').find((el) => el.text() === '消息')?.classes()).toContain('text-left')
    expect(wrapper.findAll('td').find((el) => el.text() === 'boom')?.classes()).toContain('text-left')
    // 保留策略在真实 Popover 里：先打开，再读「更新于」、改天数、保存
    await openPopover(wrapper)
    expect(document.body.textContent).toContain('更新于')
    const keep = retentionInput()
    keep.value = '7'
    keep.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    Array.from(document.body.querySelectorAll<HTMLElement>('button')).find((b) => b.textContent?.trim() === '保存')!.click()
    await flushPromises()
    expect(api.logs.putRetention).toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalledWith('已保存保留策略')
  })

  it('X6：趋势在日志表格上方，保留策略收进卡片头弹层', async () => {
    const { wrapper } = await mountLogs()
    const html = wrapper.html()
    expect(html.indexOf('trend-chart')).toBeGreaterThan(-1)
    expect(html.indexOf('trend-chart')).toBeLessThan(html.indexOf('<table'))
    expect(wrapper.findAll('.card')).toHaveLength(1)
    const trigger = wrapper.get('[data-action="retention"]')
    expect(trigger.text()).toContain('保留策略')
    // 收进卡片头弹层：未打开时页面里没有天数输入框，打开后才出现
    expect(retentionInput()).toBeNull()
    await openPopover(wrapper, '.card-action')
    expect(retentionInput()).not.toBeNull()
  })

  it('handles load/save errors and skips work without a project', async () => {
    api.logs.list.mockRejectedValueOnce(new Error('log fail'))
    await mountLogs()
    expect(toast.error).toHaveBeenCalledWith('log fail')

    api.logs.list.mockResolvedValue([])
    api.logs.putRetention.mockRejectedValueOnce('x')
    const { wrapper, pinia } = await mountLogs()
    await openPopover(wrapper)
    const save = () => Array.from(document.body.querySelectorAll<HTMLElement>('button')).find((b) => b.textContent?.trim() === '保存')!
    save().click()
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('保存失败')

    const { useProjectStore } = await import('@/stores/project')
    const store = useProjectStore(pinia)
    store.projectId = ''
    await flushPromises()
    // 没有项目：页面显示空态，保留策略弹层随之不存在，不应再发起保存
    api.logs.putRetention.mockClear()
    document.body.querySelectorAll<HTMLElement>('button').forEach((b) => { if (b.textContent?.trim() === '保存') b.click() })
    await flushPromises()
    expect(api.logs.putRetention).not.toHaveBeenCalled()
  })

  it('polls when auto-refresh is enabled and stops on unmount', async () => {
    const { wrapper } = await mountWithApp(Logs)
    const before = api.logs.list.mock.calls.length
    await wrapper.get('[data-slot="switch"]').trigger('click')
    await vi.advanceTimersByTimeAsync(10000)
    await flushPromises()
    expect(api.logs.list.mock.calls.length).toBeGreaterThan(before)
    await wrapper.get('[data-slot="switch"]').trigger('click')
    wrapper.unmount()
  })

  it('applies filters and keepDays fallback, and reloads on project change', async () => {
    const { wrapper, pinia } = await mountLogs()
    // 真实 Select：级别选「错误」，查询请求带 level=error
    await pickSelectOption(selectTriggers(wrapper)[0], '错误')
    const keyword = wrapper.get('.ig-input')
    await keyword.setValue('err')
    const locals = wrapper.findAll('input[type="datetime-local"]')
    await locals[0].setValue('2024-01-01T00:00')
    await locals[1].setValue('2024-01-02T00:00')
    await clickText(wrapper, '查询')
    await flushPromises()
    const q = api.logs.list.mock.calls.at(-1)?.[1]
    expect(q.level).toBe('error')
    expect(q.q).toBe('err')
    expect(q.from).toBeTruthy()
    expect(q.limit).toBe(200)
    await wrapper.get('.pager-next').trigger('click')

    await openPopover(wrapper)
    const keep = retentionInput()
    keep.value = 'not-a-number'
    keep.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    const { useProjectStore } = await import('@/stores/project')
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    expect(api.logs.list.mock.calls.length).toBeGreaterThan(2)
  })

  it('renders empty state and warn/info levels', async () => {
    api.logs.list.mockResolvedValueOnce([])
    api.logs.getRetention.mockResolvedValueOnce({ scope: 'p', keepDays: 3, updatedAt: '' })
    const empty = await mountWithApp(Logs)
    expect(empty.wrapper.text()).toContain('暂无匹配日志')
    expect(empty.wrapper.text()).toContain('0 条')
    empty.wrapper.unmount()

    api.logs.list.mockResolvedValue([
      { id: 'w1', projectId: 'p', level: 'warn', logger: 'cron', message: 'slow', occurredAt: '2024-01-01T00:00:00Z' },
      { id: 'i1', projectId: 'p', level: 'info', logger: 'http', message: 'ok', occurredAt: '2024-01-01T00:00:01Z' }
    ])
    const { wrapper } = await mountWithApp(Logs)
    expect(wrapper.text()).toContain('slow')
    expect(wrapper.text()).toContain('ok')
    expect(wrapper.text()).toContain('警告')
    expect(wrapper.text()).toContain('信息')
    expect(wrapper.text()).toContain('2 条')
    expect(wrapper.findAll('.card-title').map((n) => n.text()).filter((t) => t === '日志')).toHaveLength(1)
  })
  it('handles non-Error responses', async () => {
    const logs = await mountLogs()
    const lvm = logs.wrapper.vm as Record<string, any>
    // 真实 Select：选「错误」再选回「全部」，级别筛选清空
    await pickSelectOption(selectTriggers(logs.wrapper)[0], '错误')
    expect(lvm.level).toBe('error')
    await pickSelectOption(selectTriggers(logs.wrapper)[0], '全部')
    expect(lvm.level).toBeUndefined()
    lvm.keepDaysText = 'nope'
    expect(lvm.keepDays).toBe(14)
    lvm.keepDaysText = ''
    expect(lvm.keepDays).toBe(14)
    lvm.keepDaysText = '0'
    expect(lvm.keepDays).toBe(1)
    lvm.keepDaysText = '999'
    expect(lvm.keepDays).toBe(365)
    api.logs.list.mockRejectedValueOnce('load-logs')
    await lvm.load()
    logs.wrapper.unmount()

  })

  it('exports CSV and reports export errors', async () => {
    Object.assign(URL, { createObjectURL: vi.fn(() => 'blob:test'), revokeObjectURL: vi.fn() })
    const { wrapper } = await mountWithApp(Logs)
    const vm = wrapper.vm as unknown as { exportCSV: () => Promise<void> }
    await vm.exportCSV()
    expect(api.logs.export).toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(0)
    expect(URL.revokeObjectURL).toHaveBeenCalled()
    api.logs.export.mockRejectedValueOnce(new Error('export'))
    await vm.exportCSV()
    expect(toast.error).toHaveBeenCalledWith('export')
  })

  it('Logs load, save retention, and poll', async () => {
    const { wrapper: w } = await mountWithApp(Logs)
    await flushPromises()
    expect(api.logs.list).toHaveBeenCalled()
    const vm = w.vm as any
    vm.keepDaysText = '21'
    await vm.saveRetention()
    expect(api.logs.putRetention).toHaveBeenCalled()
    vm.autoRefresh = true
    await flushPromises()
    vm.autoRefresh = false
    await flushPromises()
    vm.keyword = 'needle'
    vm.from = '2024-01-01T00:00'
    await vm.clearFilters()
    expect(vm.keyword).toBe('')
    expect(vm.from).toBe('')
    api.logs.list.mockRejectedValueOnce(new Error('fail'))
    await vm.load()
    api.logs.putRetention.mockRejectedValueOnce(new Error('nope'))
    await vm.saveRetention()
    w.unmount()
  })


})
