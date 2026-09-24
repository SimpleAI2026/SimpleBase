import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
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
    api.s3.list.mockResolvedValue([
      { key: 'a.txt', size: 1, lastModified: 't' },
      { key: 'b.txt', size: 2, lastModified: 't' }
    ])
    api.gofunctions.list.mockResolvedValue([{ name: 'f1' }, { name: 'f2' }, { name: 'f3' }])
    api.cronjobs.list.mockResolvedValue([{ name: 'job1' }])
    api.agents.list.mockResolvedValue([{ id: 'a1' }, { id: 'a2' }])
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

  it('loads metric cards, request trend, and resource summary counts', async () => {
    const { wrapper } = await mountWithApp(Dashboard)
    expect(wrapper.text()).toContain('系统运行状态与项目资源概览')
    expect(wrapper.text()).toContain('数据库总数')
    expect(wrapper.text()).toContain('就绪数据库')
    expect(wrapper.text()).toContain('异常数据库')
    expect(wrapper.text()).toContain('配额状态')
    expect(wrapper.text()).toContain('正常')
    expect(wrapper.text()).toContain('请求 9')
    expect(wrapper.text()).toContain('错误率 1%')
    // 资源汇总：只展示类型与数量，不列明细名称
    expect(wrapper.text()).toContain('资源类型')
    expect(wrapper.text()).toContain('数据库')
    expect(wrapper.text()).toContain('S3 对象存储')
    expect(wrapper.text()).toContain('云函数')
    expect(wrapper.text()).toContain('定时任务')
    expect(wrapper.text()).toContain('云 Agent')
    expect(wrapper.text()).not.toContain('demo')
    expect(wrapper.text()).not.toContain('flaky')
    expect(wrapper.find('.trend-legend').text()).toContain('请求')
    expect(wrapper.find('.trend-legend').text()).toContain('错误')
    // 柱状/折线切换控件（echarts 容器）
    expect(wrapper.find('.trend-mode-switch').exists()).toBe(true)
    expect(wrapper.find('.trend-canvas').exists()).toBe(true)
    expect(wrapper.find('[role="img"]').exists()).toBe(true)
  })

  it('switches between bar and line chart modes', async () => {
    const { wrapper } = await mountWithApp(Dashboard)
    await flushPromises()
    expect(wrapper.find('.trend-chart').exists()).toBe(true)
    // uiStubs 的 ToggleGroup 在点击时 emit update:modelValue → line
    await wrapper.get('.tg-line').trigger('click')
    await flushPromises()
    expect(wrapper.find('.trend-chart').exists()).toBe(true)
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
    expect(wrapper.text()).toContain('趋势加载失败')
    expect(wrapper.text()).not.toContain('暂无趋势数据')
    expect(wrapper.text()).not.toContain('请求 9')
    expect(toast.error).toHaveBeenCalledWith('部分数据加载失败')
  })

  it('shows zero counts when no resources exist', async () => {
    api.databases.list.mockResolvedValue([])
    api.s3.list.mockResolvedValue([])
    api.gofunctions.list.mockResolvedValue([])
    api.cronjobs.list.mockResolvedValue([])
    api.agents.list.mockResolvedValue([])
    api.metrics.trend.mockResolvedValue([])
    const { wrapper } = await mountWithApp(Dashboard)
    expect(wrapper.text()).toContain('暂无趋势数据')
    expect(wrapper.text()).toContain('资源类型')
    expect(wrapper.text()).toContain('云 Agent')
  })

  it('reloads when the project changes', async () => {
    const { wrapper, pinia } = await mountWithApp(Dashboard)
    await flushPromises()
    const before = api.databases.list.mock.calls.length
    const { useProjectStore } = await import('../stores/project')
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    expect(api.databases.list.mock.calls.length).toBeGreaterThan(before)
  })
})
