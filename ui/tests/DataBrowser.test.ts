import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DataBrowser from '@/pages/DataBrowser.vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import { pageVm, type PageVm as Vm } from '@/test/page-vm'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import type { CollectionField } from '@/services/types'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

// 图表容器依赖 canvas / ResizeObserver，页面编排测试不关心绘制，只关心传进去的 option。
vi.mock('@/components/bi/BiChart.vue', () => ({
  default: {
    name: 'BiChart',
    props: ['option', 'ariaLabel', 'heightClass'],
    emits: ['select'],
    template: '<div class="bi-chart-stub" :data-series="(option.series || []).map((s) => s.type).join(\',\')"><button type="button" class="bi-chart-click" @click="$emit(\'select\', { name: \'open\', seriesName: \'\', value: 1 })">click</button></div>'
  }
}))

const field = (name: string, type: CollectionField['type'], values?: string[]): CollectionField => ({
  name, type, required: false, unique: false, hidden: false, readonly: false, options: values ? { values } : {}
})

const SCHEMA = [field('title', 'text'), field('priority', 'number'), field('state', 'select', ['open', 'closed']), field('published', 'date')]

interface SetupOpts {
  view?: boolean
  registered?: boolean
  canWrite?: boolean
  admin?: boolean
  realSelect?: boolean
}

function setup(opts: SetupOpts = {}) {
  const { view = false, registered = true, canWrite = true } = opts
  const pinia = createPinia()
  setActivePinia(pinia)
  useProjectStore().setProject(opts.admin ? 'sb-admin' : 'p1', 'Project')
  const auth = useAuthStore()
  Object.defineProperty(auth, 'canWrite', { value: canWrite, configurable: true })
  api.databases.list.mockResolvedValue([{ id: 'db1', name: 'Database', status: 'ready', dataModel: 'collection' }])
  api.db.collectionItems.mockResolvedValue([{ name: 'tasks', schemaMode: view ? 'view' : 'json', fieldCount: 4, documentCount: 3, registered }])
  if (registered) {
    api.db.getCollectionSchema.mockResolvedValue({ name: 'tasks', schemaMode: view ? 'view' : 'json', fields: SCHEMA })
  } else {
    api.db.getCollectionSchema.mockRejectedValue(Object.assign(new Error('not found'), { status: 404 }))
  }
  api.db.documents.mockResolvedValue({
    rows: [], total: 1,
    items: [{ id: 'r1', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-02T00:00:00Z', data: { title: 'One', priority: 1, state: 'open', published: '2026-01-01T00:00:00Z' } }]
  })
  api.db.aggregate.mockImplementation(async (_p, _d, _c, q) => {
    if (q.mode === 'points') return { columns: ['x', 'y'], rows: [[1, 2]], truncated: false, scanned: 3, durationMs: 1, requestId: '' }
    const cols = ['x', 'count'].filter((c) => c !== 'x' || q.groupBy)
    const rows = q.groupBy ? [['open', 2], ['closed', 1]] : [[3]]
    return { columns: cols, rows, truncated: false, scanned: 3, durationMs: 1, requestId: '' }
  })
  api.db.exportCollection.mockResolvedValue(new Blob(['id']))
  api.agents.list.mockResolvedValue([])
  const stubs = opts.realSelect
    ? { ...uiStubs, Select: false, SelectTrigger: false, SelectValue: false, SelectContent: false, SelectGroup: false, SelectItem: false, teleport: false }
    : uiStubs
  const wrapper = mount(DataBrowser, { global: { plugins: [pinia], stubs }, attachTo: opts.realSelect ? document.body : undefined })
  return wrapper
}

const buttonText = (wrapper: ReturnType<typeof setup>, text: string) => wrapper.findAll('button').find((b) => b.text().includes(text))

describe('DataBrowser (BI dashboard)', () => {
  const downloads: string[] = []

  beforeEach(() => {
    resetApiMocks()
    downloads.length = 0
    window.localStorage.clear()
    Object.assign(URL, { createObjectURL: vi.fn(() => 'blob:test'), revokeObjectURL: vi.fn() })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      downloads.push(this.download)
    })
  })

  it('renders the default board for a registered collection with one aggregate request per widget', async () => {
    const wrapper = setup()
    await flushPromises()
    const board = wrapper.get('[data-testid="bi-board"]')
    expect(board.findAll('[data-widget]').length).toBeGreaterThanOrEqual(2)
    const calls = api.db.aggregate.mock.calls
    expect(calls.length).toBe(board.findAll('[data-widget]').length)
    // 首屏请求数有上限（planv6.7 验收 1）。
    expect(calls.length).toBeLessThanOrEqual(8)
    // 默认看板必有「总记录」KPI 与按天记录趋势。
    expect(calls.some(([, , , q]) => q.metrics?.join() === 'count' && !q.groupBy)).toBe(true)
    expect(calls.some(([, , , q]) => q.groupBy === 'created_at:day')).toBe(true)
    // 请求都带上了项目 / 库 / 集合。
    for (const [p, d, c] of calls) expect([p, d, c]).toEqual(['p1', 'db1', 'tasks'])
  })

  it('offers the nine chart types in prototype order from a select on the data-source row', async () => {
    const wrapper = setup()
    await flushPromises()
    const toolbar = wrapper.get('[data-testid="bi-toolbar"]')
    expect(toolbar.text()).toContain('图表类型')
    const options = toolbar.findAll('option').map((o) => o.attributes('value'))
    expect(options.filter((v) => ['auto', 'line', 'column', 'bar', 'pie', 'table', 'grouped_bar', 'stacked_bar', 'scatter'].includes(v as string)))
      .toEqual(['auto', 'line', 'column', 'bar', 'pie', 'table', 'grouped_bar', 'stacked_bar', 'scatter'])
    // 旧的按钮组不再出现
    expect(wrapper.findAll('[data-type]')).toHaveLength(0)
  })

  it('shows the detail table first, then the chart board', async () => {
    const wrapper = setup()
    await flushPromises()
    const html = wrapper.html()
    expect(html.indexOf('data-testid="bi-detail"')).toBeGreaterThan(-1)
    expect(html.indexOf('data-testid="bi-detail"')).toBeLessThan(html.indexOf('data-testid="bi-board"'))
  })

  it('renders the read-only detail table and never offers row editing', async () => {
    const wrapper = setup()
    await flushPromises()
    const detail = wrapper.get('[data-testid="bi-detail"]')
    expect(detail.text()).toContain('One')
    expect(detail.find('input').exists()).toBe(false)
    expect(detail.find('textarea').exists()).toBe(false)
    expect(api.db.patch).not.toHaveBeenCalled()
    expect(api.db.documents).toHaveBeenCalledWith('p1', 'db1', 'tasks', expect.objectContaining({ limit: 20, offset: 0, count: true }))
  })

  it('points detail paging at the next page', async () => {
    const wrapper = setup()
    await flushPromises()
    await wrapper.get('.pager-next').trigger('click')
    await flushPromises()
    expect(api.db.documents).toHaveBeenLastCalledWith('p1', 'db1', 'tasks', expect.objectContaining({ offset: 20 }))
  })

  it('applies the time range to every widget and the detail table', async () => {
    const wrapper = setup()
    await flushPromises()
    api.db.aggregate.mockClear()
    api.db.documents.mockClear()
    const vm = pageVm(wrapper)
    await wrapper.get('[data-range="d7"]').trigger('click').catch(() => undefined)
    // ToggleGroup 在共享桩里没有真实选项，直接驱动 range（页面状态）以验证联动。
    vm.range = 'd7'
    await flushPromises()
    expect(vm.globalFilter).toMatch(/^created_at>="\d{4}-/)
    expect(api.db.aggregate.mock.calls.length).toBeGreaterThan(0)
    for (const [, , , q] of api.db.aggregate.mock.calls) expect(q.filter).toBe(vm.globalFilter)
    expect(api.db.documents).toHaveBeenCalledWith('p1', 'db1', 'tasks', expect.objectContaining({ filter: vm.globalFilter, offset: 0 }))
  })

  it('applies a typed filter on Enter / apply and keeps history', async () => {
    const wrapper = setup()
    await flushPromises()
    api.db.aggregate.mockClear()
    await wrapper.get('input[data-field="filter"]').setValue('state="open"')
    await buttonText(wrapper, '应用')!.trigger('click')
    await flushPromises()
    expect(api.db.aggregate.mock.calls.length).toBeGreaterThan(0)
    for (const [, , , q] of api.db.aggregate.mock.calls) expect(q.filter).toBe('state="open"')
    expect(wrapper.find('[data-chip="history"]').text()).toBe('state="open"')
    // 清除条件后恢复。
    api.db.aggregate.mockClear()
    await buttonText(wrapper, '清除条件')!.trigger('click')
    await flushPromises()
    for (const [, , , q] of api.db.aggregate.mock.calls) expect(q.filter).toBeUndefined()
  })

  it('click-to-filter adds a chip, re-queries everything, and the chip can be removed', async () => {
    const wrapper = setup()
    await flushPromises()
    const vm = pageVm(wrapper)
    api.db.aggregate.mockClear()
    // 默认看板里 state 的饼图点击 "open" → 追加 state="open" chip。
    const pie = wrapper.get('[data-widget="share-state"]')
    await pie.get('.bi-chart-click').trigger('click')
    await flushPromises()
    expect(vm.chips.map((c) => c.expr)).toEqual(['state="open"'])
    expect(wrapper.get('[data-testid="bi-chips"]').text()).toContain('state=open')
    for (const [, , , q] of api.db.aggregate.mock.calls) expect(q.filter).toBe('state="open"')
    expect(api.db.documents).toHaveBeenLastCalledWith('p1', 'db1', 'tasks', expect.objectContaining({ filter: 'state="open"' }))

    // 同一个 chip 不重复添加。
    await pie.get('.bi-chart-click').trigger('click')
    await flushPromises()
    expect(vm.chips).toHaveLength(1)

    api.db.aggregate.mockClear()
    await wrapper.get('[data-chip-remove]').trigger('click')
    await flushPromises()
    expect(vm.chips).toHaveLength(0)
    for (const [, , , q] of api.db.aggregate.mock.calls) expect(q.filter).toBeUndefined()
  })

  it('persists board edits per collection and restores them on reload', async () => {
    const first = setup()
    await flushPromises()
    await first.get('[data-widget="kpi-total"] [data-action="menu"]').trigger('click')
    await first.get('[data-widget="kpi-total"] [data-action="duplicate"]').trigger('click')
    await flushPromises()
    const count = first.findAll('[data-testid="bi-board"] [data-widget]').length
    expect(window.localStorage.getItem('sb_bi_v1:p1:db1:tasks')).toContain('kpi-total')
    first.unmount()

    resetApiMocks()
    const second = setup()
    await flushPromises()
    expect(second.findAll('[data-testid="bi-board"] [data-widget]').length).toBe(count)
  })

  it('restoring the default board discards saved edits', async () => {
    const wrapper = setup()
    await flushPromises()
    await wrapper.get('[data-widget="kpi-total"] [data-action="menu"]').trigger('click')
    await wrapper.get('[data-widget="kpi-total"] [data-action="duplicate"]').trigger('click')
    await flushPromises()
    expect(window.localStorage.getItem('sb_bi_v1:p1:db1:tasks')).toBeTruthy()
    const edited = wrapper.findAll('[data-testid="bi-board"] [data-widget]').length
    // 共享 ConfirmAction 桩点击即确认。
    await wrapper.get('[data-action="reset-board"]').trigger('click')
    await flushPromises()
    expect(window.localStorage.getItem('sb_bi_v1:p1:db1:tasks')).toBeNull()
    expect(wrapper.findAll('[data-testid="bi-board"] [data-widget]').length).toBeLessThan(edited)
    expect(wrapper.find('[data-widget="kpi-total"]').exists()).toBe(true)
  })

  it('an unregistered collection only gets the total and the trend, plus a schema prompt', async () => {
    const wrapper = setup({ registered: false })
    await flushPromises()
    const ids = wrapper.findAll('[data-testid="bi-board"] [data-widget]').map((w) => w.attributes('data-widget'))
    expect(ids).toEqual(['kpi-total', 'trend'])
    expect(wrapper.get('[data-testid="needs-schema"]').text()).toContain('登记')
  })

  it('exports with the current global filter and downloads the file', async () => {
    const wrapper = setup()
    await flushPromises()
    const vm = pageVm(wrapper)
    await vm.exportData('csv')
    expect(api.db.exportCollection).toHaveBeenCalledWith('p1', 'db1', 'tasks', 'csv', { filter: undefined })
    expect(downloads).toEqual(['tasks.csv'])
    await vm.onAction('export-json')
    expect(downloads).toEqual(['tasks.csv', 'tasks.json'])
  })

  it('keeps the data actions (schema, view, import, export) reachable', async () => {
    const wrapper = setup()
    await flushPromises()
    for (const action of ['schema', 'view', 'import', 'export-csv', 'export-json']) {
      expect(wrapper.find(`[data-action="${action}"]`).exists(), action).toBe(true)
    }
    await wrapper.get('[data-action="schema"]').trigger('click')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)
  })

  it('logical views and read-only roles cannot reach write actions', async () => {
    const view = setup({ view: true })
    await flushPromises()
    for (const action of ['schema', 'import']) expect(view.get(`[data-action="${action}"]`).attributes('disabled'), action).toBeDefined()
    // 视图可以新建视图（针对库，不针对集合）。
    expect(view.get('[data-action="view"]').attributes('disabled')).toBeUndefined()
    view.unmount()

    resetApiMocks()
    const readonly = setup({ canWrite: false })
    await flushPromises()
    for (const action of ['schema', 'view', 'import']) expect(readonly.get(`[data-action="${action}"]`).attributes('disabled'), action).toBeDefined()
    // 导出是读操作，仍可用。
    expect(readonly.get('[data-action="export-csv"]').attributes('disabled')).toBeUndefined()
  })

  it('the system project is read-only: no write actions and the agent is disabled', async () => {
    const wrapper = setup({ admin: true })
    await flushPromises()
    for (const action of ['schema', 'view', 'import']) expect(wrapper.get(`[data-action="${action}"]`).attributes('disabled'), action).toBeDefined()
    expect(wrapper.get('[data-testid="bi-agent-disabled"]').text()).toContain('系统项目')
    expect(wrapper.get('[data-testid="bi-agent-input"]').attributes('disabled')).toBeDefined()
  })

  it('shows the agent disable reasons for read-only roles and unregistered schemas', async () => {
    const readonly = setup({ canWrite: false })
    await flushPromises()
    expect(readonly.get('[data-testid="bi-agent-disabled"]').text()).toContain('没有写权限')
    readonly.unmount()

    resetApiMocks()
    const bare = setup({ registered: false })
    await flushPromises()
    expect(bare.get('[data-testid="bi-agent-disabled"]').text()).toContain('Schema')
  })

  it('keeps the newest database selection when an older collection request resolves late', async () => {
    const wrapper = setup()
    await flushPromises()
    const vm = pageVm(wrapper)
    api.databases.list.mockResolvedValue([
      { id: 'db1', name: 'Database 1', status: 'ready', dataModel: 'collection' },
      { id: 'db2', name: 'Database 2', status: 'ready', dataModel: 'collection' }
    ])
    await vm.loadDatabases(true)

    let resolveOld!: (items: { name: string }[]) => void
    api.db.collectionItems.mockImplementation((_p, database) => {
      if (database === 'db1') return new Promise((resolve) => { resolveOld = resolve })
      return Promise.resolve([{ name: 'orders', schemaMode: 'json', fieldCount: 1, documentCount: 1, registered: true }])
    })
    const oldLoad = vm.loadCollections(true)
    vm.selectDatabase('db2')
    await flushPromises()
    expect(vm.databaseId).toBe('db2')
    expect(vm.collection).toBe('orders')

    resolveOld([{ name: 'outdated' }])
    await oldLoad
    await flushPromises()
    expect(vm.databaseId).toBe('db2')
    expect(vm.collection).toBe('orders')
    expect(wrapper.text()).not.toContain('outdated')
  })

  it('drops a stale schema response after switching collections', async () => {
    const wrapper = setup()
    await flushPromises()
    const vm = pageVm(wrapper)
    api.db.collectionItems.mockResolvedValue([
      { name: 'tasks', schemaMode: 'json', fieldCount: 4, documentCount: 3, registered: true },
      { name: 'users', schemaMode: 'json', fieldCount: 1, documentCount: 1, registered: true }
    ])
    await vm.reloadCollections()
    await flushPromises()
    expect(vm.collection).toBe('tasks')

    // 先切到 users 并让它的 schema 一直挂起，再切回 tasks：后到的 users 响应必须被丢弃。
    let releaseUsers!: (v: unknown) => void
    api.db.getCollectionSchema.mockImplementation((_p, _d, c) => {
      if (c === 'users') return new Promise((resolve) => { releaseUsers = resolve })
      return Promise.resolve({ name: 'tasks', schemaMode: 'json', fields: SCHEMA })
    })
    vm.selectCollection('users')
    await flushPromises()
    vm.selectCollection('tasks')
    await flushPromises()
    releaseUsers({ name: 'users', schemaMode: 'json', fields: [field('email', 'email')] })
    await flushPromises()

    expect(vm.collection).toBe('tasks')
    // 看板仍是 tasks 的 schema（含 state 的占比图），没有被 users 的 schema 覆盖。
    expect(wrapper.find('[data-widget="share-state"]').exists()).toBe(true)
    expect(wrapper.find('[data-widget="share-email"]').exists()).toBe(false)
  })

  it('refreshes the full resource chain and recovers from resource errors', async () => {
    api.databases.list.mockRejectedValueOnce(new Error('offline'))
    const wrapper = setup()
    await flushPromises()
    const vm = pageVm(wrapper)
    expect(wrapper.text()).toContain('offline')

    api.databases.list.mockResolvedValue([{ id: 'db1', name: 'Database', status: 'ready', dataModel: 'collection' }])
    await vm.refresh()
    await flushPromises()
    expect(wrapper.text()).toContain('tasks')
    expect(vm.databaseId).toBe('db1')

    api.db.collectionItems.mockRejectedValueOnce(new Error('collections offline'))
    await vm.loadCollections(true)
    expect(wrapper.text()).toContain('collections offline')
    await vm.reloadCollections()
    expect(wrapper.text()).toContain('tasks')
  })

  it('SQL databases list their tables as data sources and show the board — never the old schema panel', async () => {
    const wrapper = setup()
    await flushPromises()
    const vm = pageVm(wrapper)
    api.databases.list.mockResolvedValue([
      { id: 'db1', name: 'Documents', status: 'ready', dataModel: 'collection' },
      { id: 'sql1', name: 'Analytics', status: 'ready', dataModel: 'sql' }
    ])
    api.databases.schema.mockResolvedValue({
      tables: [{ name: 'events', columns: [{ name: 'id', type: 'BIGINT' }, { name: 'kind', type: 'VARCHAR' }, { name: 'happened_at', type: 'TIMESTAMP' }] }]
    })
    api.databases.tableRows.mockResolvedValue({ table: 'events', columns: [{ name: 'id', type: 'BIGINT' }, { name: 'kind', type: 'VARCHAR' }], rows: [[1, 'click']], limit: 20, offset: 0, total: 1 })
    await vm.loadDatabases(true)
    const collectionCalls = api.db.collectionItems.mock.calls.length
    api.db.aggregate.mockClear()

    vm.selectDatabase('sql1')
    await flushPromises()

    expect(vm.databaseId).toBe('sql1')
    expect(vm.collection).toBe('events')
    expect(vm.sourceKind).toBe('table')
    // SQL 库走表结构接口列表，不会去请求集合接口。
    expect(api.db.collectionItems).toHaveBeenCalledTimes(collectionCalls)
    expect(api.databases.schema).toHaveBeenCalledWith('p1', 'sql1')
    // 看板出现，聚合走表端点而不是集合端点。
    expect(wrapper.find('[data-testid="bi-board"]').exists()).toBe(true)
    expect(api.databases.aggregateTable).toHaveBeenCalled()
    expect(api.db.aggregate).not.toHaveBeenCalled()
    for (const [p, d, tb] of api.databases.aggregateTable.mock.calls) expect([p, d, tb]).toEqual(['p1', 'sql1', 'events'])
    // 旧的表结构面板与它的提示语已清理。
    expect(wrapper.text()).not.toContain('新建表')
    expect(wrapper.text()).not.toContain('添加列')
    expect(wrapper.text()).not.toContain('SQL 数据库请在下方查看表结构与数据')
    expect(wrapper.find('[data-testid="table-hint"]').exists()).toBe(true)
  })

  it('works with the real Select (database and collection pickers)', async () => {
    const wrapper = setup({ realSelect: true })
    await flushPromises()
    expect(wrapper.find('[data-field="collection"]').exists()).toBe(true)
    // 只有一个 ready 库时不显示库下拉。
    expect(wrapper.find('[data-field="database"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows empty guidance when the project has no databases', async () => {
    resetApiMocks()
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore().setProject('p1', 'Project')
    api.databases.list.mockResolvedValue([])
    const wrapper = mount(DataBrowser, { global: { plugins: [pinia], stubs: uiStubs } })
    await flushPromises()
    expect(wrapper.text()).toContain('当前项目暂无可用数据库')
    expect(wrapper.find('[data-testid="bi-board"]').exists()).toBe(false)
  })
})
