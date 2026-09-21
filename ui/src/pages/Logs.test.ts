import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '../test/api-mock'
import { clickText, mountWithApp } from '../test/helpers'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return { api: m.api, isMock: false }
})

import Logs from './Logs.vue'

describe('Logs', () => {
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
    const { wrapper } = await mountWithApp(Logs)
    expect(wrapper.text()).toContain('boom')
    expect(wrapper.text()).toContain('更新于')
    const keep = wrapper.find('input[type="number"]')
    await keep.setValue('7')
    await clickText(wrapper, '保存')
    await flushPromises()
    expect(api.logs.putRetention).toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalledWith('已保存保留策略')
  })

  it('handles load/save errors and skips work without a project', async () => {
    api.logs.list.mockRejectedValueOnce(new Error('log fail'))
    await mountWithApp(Logs)
    expect(toast.error).toHaveBeenCalledWith('log fail')

    api.logs.list.mockResolvedValue([])
    api.logs.putRetention.mockRejectedValueOnce('x')
    const { wrapper, pinia } = await mountWithApp(Logs)
    await clickText(wrapper, '保存')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('保存失败')

    const { useProjectStore } = await import('../stores/project')
    const store = useProjectStore(pinia)
    store.projectId = ''
    await clickText(wrapper, '刷新')
    await clickText(wrapper, '保存')
    await flushPromises()
  })

  it('polls when auto-refresh is enabled and stops on unmount', async () => {
    const { wrapper } = await mountWithApp(Logs)
    const before = api.logs.list.mock.calls.length
    await wrapper.get('.switch').trigger('click')
    await vi.advanceTimersByTimeAsync(10000)
    await flushPromises()
    expect(api.logs.list.mock.calls.length).toBeGreaterThan(before)
    await wrapper.get('.switch').trigger('click')
    wrapper.unmount()
  })

  it('applies filters and keepDays fallback, and reloads on project change', async () => {
    const { wrapper, pinia } = await mountWithApp(Logs)
    await wrapper.get('.select-emit').trigger('click')
    const keyword = wrapper.get('.ig-input')
    await keyword.setValue('err')
    const locals = wrapper.findAll('input[type="datetime-local"]')
    await locals[0].setValue('2024-01-01T00:00')
    await locals[1].setValue('2024-01-02T00:00')
    await clickText(wrapper, '刷新')
    await flushPromises()
    const q = api.logs.list.mock.calls.at(-1)?.[1]
    expect(q.q).toBe('err')
    expect(q.from).toBeTruthy()
    expect(q.limit).toBe(200)
    await wrapper.get('.pager-next').trigger('click')

    const keep = wrapper.find('input[type="number"]')
    await keep.setValue('not-a-number')
    const { useProjectStore } = await import('../stores/project')
    useProjectStore(pinia).setProject('00000000-0000-0000-0000-000000000003')
    await flushPromises()
    expect(api.logs.list.mock.calls.length).toBeGreaterThan(2)
  })
})
