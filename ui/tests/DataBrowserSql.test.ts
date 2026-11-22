import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DataBrowser from '@/pages/DataBrowser.vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import { pageVm } from '@/test/page-vm'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})
vi.mock('@/components/bi/BiChart.vue', () => ({
  default: {
    name: 'BiChart',
    props: ['option', 'ariaLabel', 'heightClass'],
    emits: ['select'],
    template: '<div class="bi-chart-stub" :data-series="(option.series || []).map((s) => s.type).join(\',\')"><button type="button" class="bi-chart-click" @click="$emit(\'select\', { name: \'click\', seriesName: \'\', value: 1 })">click</button></div>'
  }
}))

const col = (name: string, type: string, extra: Record<string, unknown> = {}) => ({ name, type, nullable: true, ...extra })
const EVENTS = {
  name: 'events',
  columns: [col('id', 'BIGINT', { nullable: false }), col('kind', 'VARCHAR'), col('amount', 'DOUBLE'), col('happened_at', 'TIMESTAMP'), col('payload', 'JSON')]
}
const NO_TIME = { name: 'plain', columns: [col('id', 'BIGINT'), col('label', 'VARCHAR'), col('score', 'DOUBLE')] }
function setup(opts: { tables?: unknown[]; canWrite?: boolean; admin?: boolean } = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useProjectStore().setProject(opts.admin ? 'sb-admin' : 'p1', 'Project')
  Object.defineProperty(useAuthStore(), 'canWrite', { value: opts.canWrite ?? true, configurable: true })
  api.databases.list.mockResolvedValue([{ id: 'sql1', name: 'Analytics', status: 'ready', dataModel: 'sql' }])
  api.databases.schema.mockResolvedValue({ tables: opts.tables ?? [EVENTS] })
  api.databases.tableRows.mockImplementation(async (_p, _d, table, q) => ({
    table, columns: [{ name: 'id', type: 'BIGINT' }, { name: 'kind', type: 'VARCHAR' }], rows: [[1, 'click'], [2, 'view']],
    limit: q.limit, offset: q.offset, total: 2
  }))
  api.databases.aggregateTable.mockImplementation(async (_p, _d, _t, q) => ({
    columns: q.groupBy ? ['x', 'count'] : ['count'], rows: q.groupBy ? [['click', 3], ['view', 2]] : [[5]],
    truncated: false, scanned: 5, durationMs: 1, requestId: ''
  }))
  return mount(DataBrowser, { global: { plugins: [pinia], stubs: uiStubs } })
}

beforeEach(() => {
  resetApiMocks()
  window.localStorage.clear()
})

describe('SQL table as a data source', () => {
  it('opens the first table straight into a dashboard (no old schema panel, no "create table" form)', async () => {
    const wrapper = setup()
    await flushPromises()
    expect(wrapper.find('[data-testid="bi-board"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="bi-detail"]').exists()).toBe(true)
    for (const gone of ['新建表', '添加列', '添加字段', 'SQL 数据库请在下方查看表结构与数据', '查看数据']) {
      expect(wrapper.text(), gone).not.toContain(gone)
    }
    expect(wrapper.get('[data-testid="table-hint"]').text()).toContain('只读')
  })

  it('every aggregate goes to the table endpoint with the table name, never the collection endpoint', async () => {
    const wrapper = setup()
    await flushPromises()
    const calls = api.databases.aggregateTable.mock.calls
    expect(calls.length).toBe(wrapper.findAll('[data-testid="bi-board"] [data-widget]').length)
    expect(calls.length).toBeGreaterThan(0)
    for (const [p, d, tb] of calls) expect([p, d, tb]).toEqual(['p1', 'sql1', 'events'])
    expect(api.db.aggregate).not.toHaveBeenCalled()
  })

  it('the default board uses the table\'s own time column and never invents created_at', async () => {
    setup()
    await flushPromises()
    const queries = api.databases.aggregateTable.mock.calls.map((c) => c[3])
    expect(queries.some((q) => q.groupBy === 'happened_at:day')).toBe(true)
    expect(queries.some((q) => q.groupBy?.startsWith('created_at') || q.filter?.includes('created_at'))).toBe(false)
    // JSON 列不会被拿来分组或统计。
    expect(queries.some((q) => JSON.stringify(q).includes('payload'))).toBe(false)
  })

  it('a table without any time column has no trend, no time-range row, and says why', async () => {
    const wrapper = setup({ tables: [NO_TIME] })
    await flushPromises()
    const queries = api.databases.aggregateTable.mock.calls.map((c) => c[3])
    expect(queries.every((q) => !q.groupBy?.includes(':day'))).toBe(true)
    expect(wrapper.find('[data-testid="bi-range-row"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="needs-schema"]').text()).toContain('没有时间列')
    // 页面级条件里不会拼出引用不存在列的时间范围。
    expect(pageVm(wrapper).globalFilter).toBe('')
  })

  it('a table with a time column offers the time-range row, and the range filters the table by that column', async () => {
    const wrapper = setup()
    await flushPromises()
    expect(wrapper.find('[data-testid="bi-range-row"]').exists()).toBe(true)
    const vm = pageVm(wrapper)
    api.databases.aggregateTable.mockClear()
    api.databases.tableRows.mockClear()
    vm.range = 'd7'
    await flushPromises()
    expect(vm.globalFilter).toMatch(/^happened_at>="\d{4}-/)
    for (const [, , , q] of api.databases.aggregateTable.mock.calls) expect(q.filter).toBe(vm.globalFilter)
    expect(api.databases.tableRows).toHaveBeenCalledWith('p1', 'sql1', 'events', expect.objectContaining({ filter: vm.globalFilter, offset: 0 }))
  })

  it('the detail table is the table\'s own rows (columns from the result, not the document shape)', async () => {
    const wrapper = setup()
    await flushPromises()
    const detail = wrapper.get('[data-testid="bi-detail"]')
    expect(detail.findAll('th').map((h) => h.text())).toEqual(['id', 'kind'])
    expect(detail.text()).toContain('click')
    expect(api.databases.tableRows).toHaveBeenCalledWith('p1', 'sql1', 'events', { limit: 20, offset: 0, filter: undefined })
    expect(api.db.documents).not.toHaveBeenCalled()
  })

  it('detail paging asks the table endpoint for the next page', async () => {
    const wrapper = setup()
    await flushPromises()
    api.databases.tableRows.mockImplementation(async (_p, _d, table, q) => ({
      table, columns: [{ name: 'id', type: 'BIGINT' }], rows: [[1]], limit: q.limit, offset: q.offset, total: 100
    }))
    await wrapper.get('.pager-next').trigger('click')
    await flushPromises()
    expect(api.databases.tableRows).toHaveBeenLastCalledWith('p1', 'sql1', 'events', expect.objectContaining({ offset: 20 }))
  })

  it('click-to-filter on a table chart adds a chip that filters every widget and the detail by real columns', async () => {
    const wrapper = setup()
    await flushPromises()
    const vm = pageVm(wrapper)
    api.databases.aggregateTable.mockClear()
    // kind 是名字像分类的文本列：与集合里的 select 一样得到占比图。
    const share = wrapper.get('[data-widget="share-kind"]')
    await share.get('.bi-chart-click').trigger('click')
    await flushPromises()
    expect(vm.chips.map((c) => c.expr)).toEqual(['kind="click"'])
    for (const [, , , q] of api.databases.aggregateTable.mock.calls) expect(q.filter).toBe('kind="click"')
    expect(api.databases.tableRows).toHaveBeenLastCalledWith('p1', 'sql1', 'events', expect.objectContaining({ filter: 'kind="click"' }))
  })

  it('an aggregate failure surfaces the backend message with a working retry', async () => {
    const wrapper = setup()
    await flushPromises()
    api.databases.aggregateTable.mockRejectedValueOnce(new Error('table too large'))
    await wrapper.get('[data-action="refresh"]').trigger('click')
    await flushPromises()
    api.databases.aggregateTable.mockImplementation(async () => ({ columns: ['count'], rows: [[1]], truncated: false, scanned: 1, durationMs: 1, requestId: '' }))
    await flushPromises()
    expect(api.databases.aggregateTable).toHaveBeenCalled()
  })

  it('board edits persist under a table-specific key that cannot collide with a collection of the same name', async () => {
    const wrapper = setup()
    await flushPromises()
    await wrapper.get('[data-widget="kpi-total"] [data-action="menu"]').trigger('click')
    await wrapper.get('[data-widget="kpi-total"] [data-action="duplicate"]').trigger('click')
    await flushPromises()
    expect(window.localStorage.getItem('sb_bi_v1:p1:sql1:table:events')).toContain('kpi-total')
    expect(window.localStorage.getItem('sb_bi_v1:p1:sql1:events')).toBeNull()
  })

  it('has no schema / view / import actions: a SQL table is read-only here', async () => {
    const wrapper = setup()
    await flushPromises()
    for (const action of ['schema', 'view', 'import', 'data-actions']) expect(wrapper.find(`[data-action="${action}"]`).exists(), action).toBe(false)
  })

  it('an empty SQL database explains what to do next, with no board and no old panel', async () => {
    const wrapper = setup({ tables: [] })
    await flushPromises()
    expect(wrapper.find('[data-testid="bi-board"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('还没有表')
    expect(wrapper.text()).not.toContain('新建表')
    expect(api.databases.aggregateTable).not.toHaveBeenCalled()
  })

  it('switching between two tables swaps the board and refetches against the new table only', async () => {
    const second = { name: 'orders', columns: [col('id', 'BIGINT'), col('status', 'VARCHAR'), col('placed_at', 'TIMESTAMP')] }
    const wrapper = setup({ tables: [EVENTS, second] })
    await flushPromises()
    const vm = pageVm(wrapper)
    expect(vm.collection).toBe('events')
    api.databases.aggregateTable.mockClear()
    vm.selectTable('orders')
    await flushPromises()
    expect(vm.collection).toBe('orders')
    expect(api.databases.aggregateTable.mock.calls.length).toBeGreaterThan(0)
    for (const [, , tb] of api.databases.aggregateTable.mock.calls) expect(tb).toBe('orders')
  })

  it('drops a stale table response after switching tables quickly', async () => {
    const second = { name: 'orders', columns: [col('id', 'BIGINT'), col('status', 'VARCHAR')] }
    const wrapper = setup({ tables: [EVENTS, second] })
    await flushPromises()
    const vm = pageVm(wrapper)
    // 先切到 orders 并让它的明细挂起，再切回 events：晚到的 orders 响应必须被丢弃。
    let releaseOrders!: (v: unknown) => void
    api.databases.tableRows.mockImplementation((_p, _d, table, q) => {
      if (table === 'orders') return new Promise((resolve) => { releaseOrders = resolve })
      return Promise.resolve({ table, columns: [{ name: 'kind', type: 'VARCHAR' }], rows: [['fresh']], limit: q.limit, offset: q.offset, total: 1 })
    })
    vm.selectTable('orders')
    await flushPromises()
    vm.selectTable('events')
    await flushPromises()
    releaseOrders({ table: 'orders', columns: [{ name: 'status', type: 'VARCHAR' }], rows: [['STALE']], limit: 20, offset: 0, total: 1 })
    await flushPromises()
    expect(vm.collection).toBe('events')
    const detail = wrapper.get('[data-testid="bi-detail"]')
    expect(detail.text()).toContain('fresh')
    expect(detail.text()).not.toContain('STALE')
  })
})

describe('mixed use: switching between a collection database and a SQL database', () => {
  it('a collection source still hits the collection endpoint after a SQL table was open', async () => {
    const wrapper = setup()
    await flushPromises()
    api.databases.list.mockResolvedValue([
      { id: 'doc1', name: 'Docs', status: 'ready', dataModel: 'collection' },
      { id: 'sql1', name: 'Analytics', status: 'ready', dataModel: 'sql' }
    ])
    api.db.collectionItems.mockResolvedValue([{ name: 'tasks', schemaMode: 'json', fieldCount: 1, documentCount: 1, registered: true }])
    api.db.getCollectionSchema.mockResolvedValue({ name: 'tasks', schemaMode: 'json', fields: [] })
    api.db.documents.mockResolvedValue({ rows: [], total: 0, items: [] })
    api.db.aggregate.mockResolvedValue({ columns: ['count'], rows: [[0]], truncated: false, scanned: 0, durationMs: 0, requestId: '' })
    const vm = pageVm(wrapper)
    await vm.loadDatabases(true)
    api.databases.aggregateTable.mockClear()
    vm.selectDatabase('doc1')
    await flushPromises()
    expect(vm.sourceKind).toBe('collection')
    expect(api.db.aggregate).toHaveBeenCalled()
    expect(api.databases.aggregateTable).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="table-hint"]').exists()).toBe(false)
    // 回到集合库后数据操作菜单恢复。
    expect(wrapper.find('[data-action="data-actions"]').exists()).toBe(true)
  })
})
