import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { api, resetApiMocks, setIsMock } from '@/test/api-mock'
import { clickText, creatingDb, mountWithApp, readyDb } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return {
    api: m.api,
    get isMock() {
      return m.getIsMock()
    }
  }
})

import Dashboard from '@/pages/Dashboard.vue'

const legacyPageStubs = {
  ProjectScope: { template: '<div><slot /></div>' },
  PageContainer: { template: '<div><slot /></div>' },
  Card: { template: '<div><slot /></div>' },
  CardHeader: { template: '<div><slot /></div>' },
  CardTitle: { template: '<div><slot /></div>' },
  CardDescription: { template: '<div><slot /></div>' },
  CardContent: { template: '<div><slot /></div>' },
  CardAction: { template: '<div><slot /></div>' },
  Table: { template: '<table><slot /></table>' },
  TableHeader: { template: '<thead><slot /></thead>' },
  TableBody: { template: '<tbody><slot /></tbody>' },
  TableRow: { template: '<tr><slot /></tr>' },
  TableHead: { template: '<th><slot /></th>' },
  TableCell: { template: '<td><slot /></td>' },
  TableEmpty: { template: '<tr><slot /></tr>' },
  Badge: { template: '<span><slot /></span>' },
  Button: { template: '<button @click="$emit(\'click\')"><slot /></button>' },
  Skeleton: { template: '<div />' },
  Spinner: { template: '<div />' },
  SbEmptyState: { template: '<button @click="$emit(\'action\')">empty</button>' },
  TablePager: {
    props: ['page'],
    emits: ['update:page'],
    template: '<button type="button" class="pager-next" @click="$emit(\'update:page\', (page || 1) + 1)">next</button>'
  },
  Alert: { template: '<div><slot /></div>' },
  AlertTitle: { template: '<div><slot /></div>' },
  AlertDescription: { template: '<div><slot /></div>' },
  DocsSidebar: { template: '<div />' },
  DocsArticle: { template: '<div class="article" />' },
  Select: { template: '<div><slot /></div>' },
  SelectTrigger: { template: '<div />' },
  SelectValue: { template: '<div />' },
  SelectContent: { template: '<div><slot /></div>' },
  SelectGroup: { template: '<div><slot /></div>' },
  SelectItem: { template: '<div><slot /></div>' }
}


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
    api.databases.list.mockResolvedValue([readyDb, creatingDb, degradedDb])
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
    expect(wrapper.text()).toContain('对象存储')
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

  it('仅趋势失败时也标记 trendFailed 且有资源时不置空', async () => {
    api.metrics.trend.mockRejectedValueOnce(new Error('trend-only'))
    const { wrapper } = await mountWithApp(Dashboard)
    expect(wrapper.text()).toContain('趋势加载失败')
    // 数据库/函数计数正常展示
    expect(wrapper.text()).toContain('数据库总数')
    wrapper.unmount()
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

  it('shows P50/P90/P99 latency percentiles with overflow and seconds formatting', async () => {
    api.metrics.summary.mockResolvedValue({
      totalRequests: 100,
      errorRate: 0,
      avgLatencyMs: 50,
      activeDatabases: 1,
      latencyP50Ms: 18.4,
      latencyP90Ms: 1520,
      latencyP99Ms: 30000,
      latencySampleCount: 100,
      latencyOverflowMs: 30000
    })
    const { wrapper } = await mountWithApp(Dashboard)
    const block = wrapper.get('[data-testid="latency-percentiles"]').text()
    expect(block).toContain('接口耗时分位数')
    expect(block).toContain('全项目所有接口 · 近 24 小时 · 估算值')
    expect(block).toContain('样本 100')
    expect(block).toContain('P50')
    expect(block).toContain('18ms')
    expect(block).toContain('1.52s')
    expect(block).toContain('≥30s')
  })

  it('shows empty state when no latency samples (老数据/字段缺失)', async () => {
    const { wrapper } = await mountWithApp(Dashboard)
    const block = wrapper.get('[data-testid="latency-percentiles"]').text()
    expect(block).toContain('暂无近 24 小时接口样本')
    expect(block).not.toContain('0ms')
  })

  it('reloads when the project changes', async () => {
    const { wrapper, pinia } = await mountWithApp(Dashboard)
    await flushPromises()
    const before = api.databases.list.mock.calls.length
    const { useProjectStore } = await import('@/stores/project')
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    expect(api.databases.list.mock.calls.length).toBeGreaterThan(before)
  })
  it('Dashboard resource summary and refresh', async () => {
    api.metrics.summary.mockResolvedValue({ totalRequests: 2, errorRate: 0, avgLatencyMs: 1, activeDatabases: 1 })
    api.metrics.trend.mockResolvedValue([{ date: '1/1', requests: 2, errors: 0 }])
    api.databases.list.mockResolvedValue([readyDb])
    api.s3.list.mockResolvedValue([{ key: 'a', size: 1 }])
    api.gofunctions.list.mockResolvedValue([])
    api.cronjobs.list.mockResolvedValue([])
    api.agents.list.mockResolvedValue([])
    const { wrapper } = await mountWithApp(Dashboard)
    expect(wrapper.text()).toContain('资源类型')
    await clickText(wrapper, '刷新数据')
    expect(wrapper.text()).toContain('对象存储')
    wrapper.unmount()
  })
  it('Dashboard loads metrics and can navigate to databases', async () => {
    api.databases.list.mockResolvedValue([
      { id: 'd1', name: 'n', status: 'ready', createdAt: '2026-01-01T00:00:00Z', updatedAt: 't' },
      { id: 'd2', name: 'bad', status: 'degraded', createdAt: 't', updatedAt: 't' }
    ])
    api.s3.list.mockResolvedValue([])
    api.gofunctions.list.mockResolvedValue([])
    api.cronjobs.list.mockResolvedValue([])
    api.agents.list.mockResolvedValue([])
    api.quota.status.mockResolvedValue({ llmAllowed: true, databaseAllowed: true })
    api.metrics.trend.mockResolvedValue([{ date: '1/1', requests: 10, errors: 1 }])
    api.metrics.summary.mockResolvedValue({ totalRequests: 9, errorRate: 1, avgLatencyMs: 2, activeDatabases: 1 })
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'dashboard', component: { template: '<div />' } },
        { path: '/databases', name: 'databases', component: { template: '<div />' } }
      ]
    })
    await router.push('/')
    await router.isReady()
    const w = mount(Dashboard, { global: { plugins: [router, createPinia()], stubs: legacyPageStubs } })
    await flushPromises()
    expect(w.text()).toContain('请求 9')
    expect(w.text()).toContain('资源类型')
    expect(w.text()).toContain('数据库')

    api.databases.list.mockRejectedValueOnce(new Error('db'))
    api.quota.status.mockResolvedValue({ llmAllowed: false, databaseAllowed: true })
    api.metrics.trend.mockRejectedValueOnce(new Error('t'))
    api.metrics.summary.mockResolvedValue({ totalRequests: 1, errorRate: 0, avgLatencyMs: 1, activeDatabases: 0 })
    await w.vm.$.setupState.load?.()
    await flushPromises()
    expect(w.text()).toContain('受限')

    api.databases.list.mockRejectedValueOnce(new Error('db'))
    api.quota.status.mockRejectedValueOnce(new Error('q'))
    api.metrics.trend.mockResolvedValueOnce([{ date: '1/2', requests: 0, errors: 0 }])
    api.metrics.summary.mockRejectedValueOnce(new Error('s'))
    await w.vm.$.setupState.load?.()
    await flushPromises()

    const empty = mount(Dashboard, { global: { plugins: [router, createPinia()], stubs: legacyPageStubs } })
    api.databases.list.mockResolvedValue([])
    api.s3.list.mockResolvedValue([])
    api.gofunctions.list.mockResolvedValue([])
    api.cronjobs.list.mockResolvedValue([])
    api.agents.list.mockResolvedValue([])
    api.quota.status.mockRejectedValue(new Error('x'))
    api.metrics.trend.mockResolvedValue([])
    api.metrics.summary.mockRejectedValue(new Error('x'))
    await empty.vm.$.setupState.load?.()
    await flushPromises()
    w.unmount()
    empty.unmount()
  })


})
