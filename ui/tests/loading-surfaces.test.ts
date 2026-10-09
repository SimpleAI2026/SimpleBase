import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, mountWithApp, readyDb, sampleAgent, uiStubs } from '@/test/helpers'
import { useProjectStore } from '@/stores/project'
import { LOAD_PLACEHOLDER_DELAY_MS } from '@/composables/useLoadState'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

const catalog = vi.hoisted(() => ({ loadMarkdown: vi.fn() }))
vi.mock('@/docs/catalog', async () => {
  const actual = await vi.importActual<typeof import('@/docs/catalog')>('@/docs/catalog')
  return { ...actual, loadMarkdown: catalog.loadMarkdown }
})

import Sandboxes from '@/pages/Sandboxes.vue'
import AgentManager from '@/pages/AgentManager.vue'
import GoFunctions from '@/pages/GoFunctions.vue'
import CronJobs from '@/pages/CronJobs.vue'
import Databases from '@/pages/Databases.vue'
import Users from '@/pages/Users.vue'
import Logs from '@/pages/Logs.vue'
import S3Manager from '@/pages/S3Manager.vue'
import GoFuncVersionsModal from '@/components/modal/GoFuncVersionsModal.vue'
import GoFuncTestModal from '@/components/modal/GoFuncTestModal.vue'
import CronJobModal from '@/components/modal/CronJobModal.vue'
import AgentScheduleModal from '@/components/ai/AgentScheduleModal.vue'
import SandboxDrawer from '@/components/sandbox/SandboxDrawer.vue'
import DocsArticle from '@/components/docs/DocsArticle.vue'
import SettingsPanel from '@/components/settings/SettingsPanel.vue'
import SqlWorkModal from '@/components/modal/SqlWorkModal.vue'

interface Deferred<T> {
  promise: Promise<T>
  resolve: (value: T) => void
  reject: (reason?: unknown) => void
}

function hold<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const available = {
  available: true, backend: 'fake', images: ['python:3.12-slim'], defaultImage: 'python:3.12-slim',
  cpusMax: 4, memoryMiBMax: 4096, maxFileBytes: 1048576, maxOutputBytes: 65536,
  maxPerProject: 5, execTimeoutMaxS: 300, networkOptions: ['none']
}
const sandbox = {
  id: 'sbx-1', name: 'ci-test', cloudName: 'sbx-1', source: 'api', status: 'running', image: 'python:3.12-slim',
  cpus: 1, memoryMiB: 256, network: 'none', idleTimeoutS: 300, maxDurationS: 1800,
  createdAt: '2026-10-04T00:00:00Z', lastActiveAt: '2026-10-04T00:10:00Z'
}
const fnRecord = {
  id: 'gf-1', name: 'hello', file: 'hello.go', description: '', activeVersion: 1, latestVersion: 1,
  published: true, exports: ['Hello'], createdAt: 't', updatedAt: 't'
}
const schedule = {
  id: 'sch-1', agent_id: 'ag-1', thread_id: 'th-1', prompt: 'check', cron_expr: '0 8 * * *',
  enabled: true, last_run_at: '', next_run_at: '', created_at: 't', updated_at: 't'
}
const sandboxStubs = {
  SandboxCreateModal: { template: '<div />' },
  SandboxDrawer: { template: '<div />' }
}

async function docsRouter() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/docs/:module?/:slug?', name: 'docs', component: { template: '<div />' } }]
  })
  await router.push('/docs')
  await router.isReady()
  return router
}

describe('loading surfaces', () => {
  beforeEach(() => {
    resetApiMocks()
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    catalog.loadMarkdown.mockReset()
  })

  it('keeps cloud sandboxes off the empty copy until the list settles', async () => {
    const caps = hold<typeof available>()
    const listed = hold<(typeof sandbox)[]>()
    api.sandboxes.capabilities.mockReturnValue(caps.promise)
    api.sandboxes.list.mockReturnValue(listed.promise)
    const { wrapper } = await mountWithApp(Sandboxes, { stubs: sandboxStubs })
    expect(wrapper.text()).toContain('正在加载')
    expect(wrapper.html()).toContain('aria-busy="true"')
    expect(wrapper.find('.spinner').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('还没有云沙盒')
    expect(wrapper.text()).not.toContain('未配置云沙盒')
    await vi.advanceTimersByTimeAsync(LOAD_PLACEHOLDER_DELAY_MS)
    expect(wrapper.find('.skeleton').exists()).toBe(true)
    expect(wrapper.find('[aria-hidden="true"]').exists()).toBe(true)
    caps.resolve(available)
    await flushPromises()
    expect(wrapper.text()).not.toContain('还没有云沙盒')
    listed.resolve([])
    await flushPromises()
    expect(wrapper.text()).toContain('还没有云沙盒')
    wrapper.unmount()
  })

  it('retries a failed sandbox load and keeps rows when refresh fails', async () => {
    api.sandboxes.capabilities.mockRejectedValueOnce(new Error('caps down'))
    const { wrapper } = await mountWithApp(Sandboxes, { stubs: sandboxStubs })
    expect(wrapper.text()).toContain('加载失败')
    expect(wrapper.text()).not.toContain('还没有云沙盒')
    api.sandboxes.capabilities.mockResolvedValue(available)
    api.sandboxes.list.mockResolvedValue([sandbox])
    await clickText(wrapper, '重试')
    await flushPromises()
    expect(wrapper.text()).toContain('ci-test')
    const again = hold<(typeof sandbox)[]>()
    api.sandboxes.list.mockReturnValue(again.promise)
    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(wrapper.text()).toContain('ci-test')
    expect(wrapper.html()).toContain('aria-busy="true"')
    again.reject(new Error('刷新失败'))
    await flushPromises()
    expect(wrapper.text()).toContain('ci-test')
    expect(wrapper.text()).not.toContain('还没有云沙盒')
    expect(toast.error).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('hides the agent empty state until the list settles', async () => {
    const listed = hold<unknown[]>()
    api.agents.modules.mockResolvedValue([])
    api.agents.list.mockReturnValue(listed.promise)
    api.agentThreads.page.mockResolvedValue({ threads: [], next_cursor: '' })
    api.agentThreads.create.mockResolvedValue({ id: 'th-1', title: '云助手', updated_at: 't' })
    api.agentThreads.messages.mockResolvedValue([])
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: true, AgentComposer: true, AgentScheduleModal: true }
    })
    expect(wrapper.text()).not.toContain('还没有 Agent')
    expect(wrapper.html()).toContain('aria-busy="true"')
    await vi.advanceTimersByTimeAsync(LOAD_PLACEHOLDER_DELAY_MS)
    expect(wrapper.find('.skeleton').exists()).toBe(true)
    listed.resolve([])
    await flushPromises()
    expect(wrapper.text()).toContain('还没有 Agent')
    wrapper.unmount()
  })

  it('keeps the conversation placeholder until the next thread history returns', async () => {
    api.agents.modules.mockResolvedValue([])
    api.agents.list.mockResolvedValue([sampleAgent])
    api.agentThreads.page.mockResolvedValue({
      threads: [
        { id: 'th-1', title: '第一句', updated_at: 't' },
        { id: 'th-2', title: '第二句', updated_at: 't' }
      ],
      next_cursor: ''
    })
    api.agentThreads.messages.mockResolvedValue([{ id: 'm1', role: 'user', content: '旧消息', created_at: 't' }])
    const { wrapper } = await mountWithApp(AgentManager)
    expect(wrapper.text()).toContain('旧消息')
    const next = hold<unknown[]>()
    api.agentThreads.messages.mockReturnValue(next.promise)
    const vm = wrapper.vm as unknown as { selectThread: (id: string) => Promise<void> }
    const pending = vm.selectThread('th-2')
    await flushPromises()
    expect(wrapper.text()).not.toContain('用 @ 点名')
    expect(wrapper.html()).toContain('aria-busy="true"')
    next.resolve([])
    await pending
    await flushPromises()
    expect(wrapper.text()).toContain('用 @ 点名')
    wrapper.unmount()
  })

  it('shows an empty version list only after a successful fetch, and an alert on failure', async () => {
    const versions = hold<{ activeVersion: number; versions: unknown[] }>()
    api.gofunctions.listVersions.mockReturnValue(versions.promise)
    const versionsModal = await mountWithApp(GoFuncVersionsModal, { props: { open: true, record: fnRecord } })
    expect(versionsModal.wrapper.text()).not.toContain('暂无版本')
    expect(versionsModal.wrapper.text()).toContain('正在加载')
    versions.resolve({ activeVersion: 0, versions: [] })
    await flushPromises()
    expect(versionsModal.wrapper.text()).toContain('暂无版本')
    versionsModal.wrapper.unmount()

    api.gofunctions.listVersions.mockRejectedValueOnce(new Error('load fail'))
    const failed = await mountWithApp(GoFuncVersionsModal, { props: { open: true, record: fnRecord } })
    expect(failed.wrapper.text()).toContain('加载失败')
    expect(failed.wrapper.text()).not.toContain('暂无版本')
    api.gofunctions.listVersions.mockResolvedValue({ activeVersion: 0, versions: [] })
    await clickText(failed.wrapper, '重试')
    await flushPromises()
    expect(failed.wrapper.text()).toContain('暂无版本')
    failed.wrapper.unmount()
  })

  it('keeps the version sidebar in a skeleton while the test modal loads', async () => {
    const versions = hold<{ activeVersion: number; versions: unknown[] }>()
    api.gofunctions.listVersions.mockReturnValue(versions.promise)
    const { wrapper } = await mountWithApp(GoFuncTestModal, { props: { open: true, record: fnRecord } })
    expect(wrapper.find('.skeleton').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('该版本无导出函数')
    versions.resolve({ activeVersion: 1, versions: [{ version: 1, exports: ['Hello'], note: '', createdAt: 't', active: true }] })
    await flushPromises()
    expect(wrapper.text()).toContain('Hello')
    wrapper.unmount()
  })

  it('shows schedule history only after runs return', async () => {
    const runs = hold<unknown[]>()
    api.agentSchedules.runs.mockReturnValue(runs.promise)
    const scheduleModal = mount(AgentScheduleModal, {
      props: { open: false, agent: sampleAgent, schedule, projectId: 'p1' },
      global: { stubs: uiStubs }
    })
    await scheduleModal.setProps({ open: true })
    await flushPromises()
    expect(scheduleModal.text()).not.toContain('暂无执行记录')
    expect(scheduleModal.text()).toContain('正在加载')
    runs.resolve([])
    await flushPromises()
    expect(scheduleModal.text()).toContain('暂无执行记录')
    const kept = hold<unknown[]>()
    api.agentSchedules.runs.mockResolvedValueOnce([{ id: 'r1', status: 'success', started_at: 't', finished_at: 't' }])
    const vm = scheduleModal.vm as unknown as { loadRuns: () => Promise<void> }
    await vm.loadRuns()
    await flushPromises()
    api.agentSchedules.runs.mockReturnValue(kept.promise)
    const refreshing = vm.loadRuns()
    await flushPromises()
    kept.reject(new Error('runs down'))
    await refreshing
    expect(scheduleModal.text()).not.toContain('暂无执行记录')
    scheduleModal.unmount()
  })

  it('shows an empty sandbox directory only after the file list returns', async () => {
    const files = hold<unknown[]>()
    api.sandboxes.files.list.mockReturnValue(files.promise)
    const drawer = mount(SandboxDrawer, {
      props: { open: true, projectId: 'p', sandbox, maxFileBytes: 1048576 },
      global: { stubs: uiStubs }
    })
    await clickText(drawer, '文件')
    await flushPromises()
    expect(drawer.text()).not.toContain('目录为空')
    expect(drawer.text()).toContain('正在加载')
    files.resolve([])
    await flushPromises()
    expect(drawer.text()).toContain('目录为空')
    api.sandboxes.files.list.mockResolvedValueOnce([{ name: 'a.txt', path: '/workspace/a.txt', kind: 'file', size: 1 }])
    await clickText(drawer, '刷新')
    await flushPromises()
    expect(drawer.text()).toContain('a.txt')
    const kept = hold<unknown[]>()
    api.sandboxes.files.list.mockReturnValue(kept.promise)
    await clickText(drawer, '刷新')
    await flushPromises()
    expect(drawer.text()).toContain('a.txt')
    kept.reject(new Error('files down'))
    await flushPromises()
    expect(drawer.text()).toContain('a.txt')
    expect(drawer.text()).not.toContain('目录为空')
    drawer.unmount()
  })

  it('does not claim there are no functions while the cron picker is loading', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore().setProject('dev-shop')
    const fns = hold<unknown[]>()
    api.gofunctions.list.mockReturnValue(fns.promise)
    const cron = mount(CronJobModal, {
      props: { open: false },
      global: { plugins: [pinia], stubs: uiStubs }
    })
    await cron.setProps({ open: true })
    await flushPromises()
    expect(cron.text()).not.toContain('还没有云函数')
    expect(cron.find('.skeleton').exists()).toBe(true)
    fns.resolve([])
    await flushPromises()
    expect(cron.text()).toContain('还没有云函数')
    cron.unmount()
  })

  it('holds the docs body until markdown resolves', async () => {
    const body = hold<string | null>()
    catalog.loadMarkdown.mockReturnValue(body.promise)
    const router = await docsRouter()
    const article = mount(DocsArticle, {
      props: {
        moduleId: 'm',
        moduleTitle: 'M',
        page: { moduleId: 'm', slug: 'x', title: 'X', order: 1, filePath: 'm/x.md' }
      },
      global: { plugins: [router], stubs: uiStubs }
    })
    await flushPromises()
    expect(article.text()).toContain('正在加载')
    expect(article.html()).toContain('aria-busy="true"')
    expect(article.text()).not.toContain('这篇文档没有正文')
    body.resolve('   ')
    await flushPromises()
    expect(article.text()).toContain('这篇文档没有正文')
    article.unmount()
  })

  it('hides provider badges until credentials settle', async () => {
    const creds = hold<unknown[]>()
    api.llmProviderCreds.list.mockReturnValue(creds.promise)
    const { wrapper } = await mountWithApp(SettingsPanel, { props: { section: 'providers' } })
    expect(wrapper.text()).not.toContain('未配置')
    expect(wrapper.text()).toContain('正在加载')
    creds.resolve([])
    await flushPromises()
    expect(wrapper.text()).toContain('未配置')
    wrapper.unmount()
  })

  it('shows a query skeleton and a button spinner instead of an empty result', async () => {
    const query = hold<{ columns: string[]; rowCount: number; rows: unknown[]; durationMs: number; requestId: string }>()
    api.sql.query.mockReturnValue(query.promise)
    const modal = mount(SqlWorkModal, {
      props: { open: true, projectId: 'p', database: readyDb, readonly: false },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = modal.vm as unknown as { sqlText: string; mode: string; run: () => Promise<void> }
    vm.mode = 'query'
    vm.sqlText = 'SELECT 1'
    const pending = vm.run()
    await flushPromises()
    expect(modal.text()).not.toContain('暂时未查询到数据')
    expect(modal.find('.spinner').exists()).toBe(true)
    expect(modal.html()).toContain('aria-busy="true"')
    await vi.advanceTimersByTimeAsync(LOAD_PLACEHOLDER_DELAY_MS)
    expect(modal.find('.skeleton').exists()).toBe(true)
    query.resolve({ columns: [], rowCount: 0, rows: [], durationMs: 1, requestId: 'r' })
    await pending
    await flushPromises()
    expect(modal.text()).toContain('暂时未查询到数据')
    modal.unmount()
  })

  it.each([
    ['云函数', GoFunctions, () => api.gofunctions.list, '还没有云函数', [fnRecord], 'hello.go', []],
    ['定时任务', CronJobs, () => api.cronjobs.list, '还没有定时任务', [{ id: 'c1', name: 'nightly', scheduleKind: 'cron', cronExpr: '0 2 * * *', enabled: true, funcFile: 'hello.go', funcExport: 'Hello', status: 'idle' }], 'nightly', []],
    ['数据库', Databases, () => api.databases.list, '暂无数据库', [{ id: 'db-1', name: 'demo', status: 'ready', dataModel: 'sql', createdAt: 't' }], 'demo', []],
    ['用户', Users, () => api.users.list, '暂无用户', { users: [{ id: 'u1', username: 'ada', role: 'member', status: 'active', projectCount: 1 }] }, 'ada', { users: [] }],
    ['日志', Logs, () => api.logs.list, '暂无匹配日志', [{ id: 'l1', message: 'hello-log', level: 'info', logger: 'api', requestId: 'r', time: 't' }], 'hello-log', []],
    ['对象', S3Manager, () => api.s3.list, '暂无对象', [{ key: 'a.txt', size: 1, lastModified: 't' }], 'a.txt', []]
  ] as const)('list page %s waits for data before the empty copy', async (_label, page, apiFn, emptyCopy, row, needle, empty) => {
    const pending = hold<unknown>()
    apiFn().mockReturnValue(pending.promise)
    const { wrapper } = await mountWithApp(page)
    expect(wrapper.text()).not.toContain(emptyCopy)
    expect(wrapper.text()).toContain('正在加载')
    expect(wrapper.find('.spinner').exists()).toBe(true)
    await vi.advanceTimersByTimeAsync(LOAD_PLACEHOLDER_DELAY_MS)
    expect(wrapper.find('.skeleton').exists()).toBe(true)
    pending.resolve(empty)
    await flushPromises()
    expect(wrapper.text()).toContain(emptyCopy)

    apiFn().mockResolvedValueOnce(row)
    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(wrapper.text()).toContain(needle)
    const kept = hold<unknown>()
    apiFn().mockReturnValue(kept.promise)
    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(wrapper.text()).toContain(needle)
    expect(wrapper.html()).toContain('aria-busy="true"')
    kept.reject(new Error('refresh failed'))
    await flushPromises()
    expect(wrapper.text()).toContain(needle)
    expect(wrapper.text()).not.toContain(emptyCopy)
    wrapper.unmount()
  })
})
