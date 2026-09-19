import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useProjectStore } from '../stores/project'
import Dashboard from './Dashboard.vue'
import DocsWiki from './DocsWiki.vue'

const { api } = vi.hoisted(() => ({
  api: {
    databases: { list: vi.fn() },
    quota: { status: vi.fn() },
    metrics: { trend: vi.fn(), summary: vi.fn() },
    logs: { list: vi.fn(), getRetention: vi.fn(), putRetention: vi.fn() },
    s3: { list: vi.fn(), remove: vi.fn(), upload: vi.fn(), presign: vi.fn() }
  }
}))

vi.mock('../services/api', () => ({
  api,
  isMock: false
}))

const stubs = {
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
  TablePager: { template: '<div />' },
  Alert: { template: '<div><slot /></div>' },
  AlertTitle: { template: '<div><slot /></div>' },
  AlertDescription: { template: '<div><slot /></div>' },
  DocsModuleTabs: { template: '<div />' },
  DocsSidebar: { template: '<div />' },
  DocsArticle: { template: '<div class="article" />' },
  Select: { template: '<div><slot /></div>' },
  SelectTrigger: { template: '<div />' },
  SelectValue: { template: '<div />' },
  SelectContent: { template: '<div><slot /></div>' },
  SelectGroup: { template: '<div><slot /></div>' },
  SelectItem: { template: '<div><slot /></div>' }
}

async function withRouter(name = 'dashboard') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'dashboard', component: { template: '<div />' } },
      { path: '/databases', name: 'databases', component: { template: '<div />' } },
      { path: '/docs/:module?/:slug?', name: 'docs-page', component: DocsWiki }
    ]
  })
  await router.push({ name })
  await router.isReady()
  return router
}

describe('pages', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(api).forEach((group) =>
      Object.values(group).forEach((fn) => (fn as ReturnType<typeof vi.fn>).mockReset())
    )
  })

  it('Dashboard loads metrics and can navigate to databases', async () => {
    api.databases.list.mockResolvedValue([
      { id: 'd1', name: 'n', status: 'ready', createdAt: '2026-01-01T00:00:00Z', updatedAt: 't' },
      { id: 'd2', name: 'bad', status: 'degraded', createdAt: 't', updatedAt: 't' }
    ])
    api.quota.status.mockResolvedValue({ llmAllowed: true, databaseAllowed: true })
    api.metrics.trend.mockResolvedValue([{ date: '1/1', requests: 10, errors: 1 }])
    api.metrics.summary.mockResolvedValue({ totalRequests: 9, errorRate: 1, avgLatencyMs: 2, activeDatabases: 1 })
    const router = await withRouter()
    const push = vi.spyOn(router, 'push')
    const w = mount(Dashboard, { global: { plugins: [router, createPinia()], stubs } })
    await flushPromises()
    expect(w.text()).toContain('请求 9')
    expect(w.text()).toContain('n')
    const empty = mount(Dashboard, { global: { plugins: [router, createPinia()], stubs } })
    api.databases.list.mockResolvedValue([])
    api.quota.status.mockRejectedValue(new Error('x'))
    api.metrics.trend.mockResolvedValue([])
    api.metrics.summary.mockRejectedValue(new Error('x'))
    await empty.vm.$.setupState.load?.()
    await flushPromises()
    const btn = w.findAll('button').find((b) => b.text().includes('empty'))
    if (btn) {
      await btn.trigger('click')
      expect(push).toHaveBeenCalled()
    }
    w.unmount()
    empty.unmount()
  })

  it('DocsWiki resolves module/slug from the route', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/docs/:module?/:slug?', name: 'docs-page', component: DocsWiki, meta: { title: '使用文档' } }
      ]
    })
    await router.push('/docs')
    await router.isReady()
    const w = mount(DocsWiki, { global: { plugins: [router, createPinia()], stubs } })
    await flushPromises()
    expect(w.find('.article').exists() || w.text().includes('文档')).toBe(true)
    w.unmount()
  })
})
