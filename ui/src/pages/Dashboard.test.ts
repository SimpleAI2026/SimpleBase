import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks, setIsMock } from '../test/api-mock'
import { clickText, closedDb, mountWithApp, readyDb } from '../test/helpers'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return {
    api: m.api,
    get isMock() {
      return m.getIsMock()
    }
  }
})

import Dashboard from './Dashboard.vue'

const degradedDb = {
  id: 'db-deg',
  name: 'flaky',
  status: 'degraded' as const,
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z'
}

describe('Dashboard (监控大盘)', () => {
  beforeEach(() => {
    resetApiMocks()
    setIsMock(false)
    api.databases.list.mockResolvedValue([readyDb, closedDb, degradedDb])
    api.quota.status.mockResolvedValue({ llmAllowed: true, databaseAllowed: true })
    api.metrics.trend.mockResolvedValue([
      { date: '01-01', requests: 10, errors: 1 },
      { date: '01-02', requests: 0, errors: 0 }
    ])
    api.metrics.summary.mockResolvedValue({
      totalRequests: 9,
      errorRate: 1,
      avgLatencyMs: 12.6,
      activeDatabases: 1
    })
  })

  it('loads metric cards, request trend, and database overview', async () => {
    const { wrapper } = await mountWithApp(Dashboard)
    expect(wrapper.text()).toContain('系统运行状态与数据库概览')
    expect(wrapper.text()).toContain('数据库总数')
    expect(wrapper.text()).toContain('就绪数据库')
    expect(wrapper.text()).toContain('异常数据库')
    expect(wrapper.text()).toContain('配额状态')
    expect(wrapper.text()).toContain('正常')
    expect(wrapper.text()).toContain('请求 9')
    expect(wrapper.text()).toContain('错误率 1%')
    expect(wrapper.text()).toContain('demo')
    expect(wrapper.text()).toContain('flaky')
    expect(wrapper.find('[title="请求 10"]').exists()).toBe(true)
    expect(wrapper.find('[title="错误 1"]').exists()).toBe(true)
  })

  it('shows Mock badge and refreshes data from the toolbar', async () => {
    setIsMock(true)
    const { wrapper } = await mountWithApp(Dashboard)
    expect(wrapper.text()).toContain('Mock')
    const before = api.metrics.summary.mock.calls.length
    await clickText(wrapper, '刷新数据')
    await flushPromises()
    expect(api.metrics.summary.mock.calls.length).toBeGreaterThan(before)
  })

  it('marks quota as 受限 when LLM or database quota is denied', async () => {
    api.quota.status.mockResolvedValue({ llmAllowed: false, databaseAllowed: true })
    const { wrapper } = await mountWithApp(Dashboard)
    expect(wrapper.text()).toContain('受限')
  })

  it('keeps rendering when some metric APIs reject', async () => {
    api.databases.list.mockRejectedValueOnce(new Error('db'))
    api.quota.status.mockRejectedValueOnce(new Error('quota'))
    api.metrics.trend.mockRejectedValueOnce(new Error('trend'))
    api.metrics.summary.mockRejectedValueOnce(new Error('summary'))
    const { wrapper } = await mountWithApp(Dashboard)
    expect(wrapper.text()).toContain('数据库总数')
    expect(wrapper.text()).toContain('-')
    expect(wrapper.text()).not.toContain('请求 9')
  })

  it('navigates to databases from the empty-state action', async () => {
    api.databases.list.mockResolvedValue([])
    api.metrics.trend.mockResolvedValue([])
    const { wrapper, router } = await mountWithApp(Dashboard)
    expect(wrapper.text()).toContain('暂无趋势数据')
    expect(wrapper.text()).toContain('暂无数据库')
    const push = vi.spyOn(router, 'push')
    await wrapper.get('.empty-action').trigger('click')
    expect(push).toHaveBeenCalledWith({ name: 'databases' })
  })

  it('pages the database table and reloads when the project changes', async () => {
    const { wrapper, pinia } = await mountWithApp(Dashboard)
    await wrapper.get('.pager-next').trigger('click')
    const { useProjectStore } = await import('../stores/project')
    const before = api.databases.list.mock.calls.length
    useProjectStore(pinia).setProject('00000000-0000-0000-0000-000000000003')
    await flushPromises()
    expect(api.databases.list.mock.calls.length).toBeGreaterThan(before)
  })
})
