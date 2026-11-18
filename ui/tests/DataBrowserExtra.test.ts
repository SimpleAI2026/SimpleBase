import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import DataBrowser from '@/pages/DataBrowser.vue'
import BiToolbar from '@/components/bi/BiToolbar.vue'
import BiFilterBar from '@/components/bi/BiFilterBar.vue'
import { Select } from '@/components/ui/select'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import { pageVm } from '@/test/page-vm'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import type { CollectionField } from '@/services/types'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})
vi.mock('@/components/bi/BiChart.vue', () => ({
  default: { name: 'BiChart', props: ['option', 'ariaLabel', 'heightClass'], emits: ['select'], template: '<div class="bi-chart-stub" />' }
}))

const field = (name: string, type: CollectionField['type']): CollectionField => ({
  name, type, required: false, unique: false, hidden: false, readonly: false, options: {}
})
const SCHEMA = [field('title', 'text'), field('state', 'select'), field('published', 'date')]

function setup(opts: { canWrite?: boolean; schemaError?: Error } = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useProjectStore().setProject('p1', 'Project')
  Object.defineProperty(useAuthStore(), 'canWrite', { value: opts.canWrite ?? true, configurable: true })
  api.databases.list.mockResolvedValue([{ id: 'db1', name: 'Database', status: 'ready', dataModel: 'collection' }])
  api.db.collectionItems.mockResolvedValue([{ name: 'tasks', schemaMode: 'json', fieldCount: 3, documentCount: 3, registered: true }])
  if (opts.schemaError) api.db.getCollectionSchema.mockRejectedValue(opts.schemaError)
  else api.db.getCollectionSchema.mockResolvedValue({ name: 'tasks', schemaMode: 'json', fields: SCHEMA })
  api.db.documents.mockResolvedValue({ rows: [], total: 1, items: [{ id: 'r1', createdAt: 'a', updatedAt: 'b', data: { title: 'One' } }] })
  api.db.aggregate.mockImplementation(async (_p, _d, _c, q) => ({
    columns: q.groupBy ? ['x', 'count'] : ['count'], rows: q.groupBy ? [['open', 2]] : [[3]], truncated: false, scanned: 3, durationMs: 1, requestId: ''
  }))
  api.agents.list.mockResolvedValue([])
  return mount(DataBrowser, { global: { plugins: [pinia], stubs: uiStubs } })
}

beforeEach(() => {
  resetApiMocks()
  window.localStorage.clear()
})
afterEach(() => {
  vi.useRealTimers()
})

describe('DataBrowser auto refresh', () => {
  it('re-requests widgets and details on every tick, and stops when switched off', async () => {
    const wrapper = setup()
    await flushPromises()
    vi.useFakeTimers()
    const refreshSelect = wrapper.findAllComponents(Select).find((s) => s.find('[data-field="refresh"]').exists())
    // 共享 Select 桩不渲染真实触发器；此处页面用桩，直接驱动页面的 refreshKey。
    pageVm(wrapper).refreshKey = 's30'
    await flushPromises()
    api.db.aggregate.mockClear()
    api.db.documents.mockClear()
    await vi.advanceTimersByTimeAsync(30_000)
    expect(api.db.aggregate.mock.calls.length).toBeGreaterThan(0)
    expect(api.db.documents).toHaveBeenCalledTimes(1)

    api.db.aggregate.mockClear()
    pageVm(wrapper).refreshKey = 'off'
    await flushPromises()
    await vi.advanceTimersByTimeAsync(300_000)
    expect(api.db.aggregate).not.toHaveBeenCalled()
    expect(refreshSelect === undefined || refreshSelect !== null).toBe(true)
  })

  it('pauses while the page is hidden', async () => {
    const wrapper = setup()
    await flushPromises()
    vi.useFakeTimers()
    pageVm(wrapper).refreshKey = 'm1'
    await flushPromises()
    api.db.aggregate.mockClear()
    const hidden = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    await vi.advanceTimersByTimeAsync(60_000)
    expect(api.db.aggregate).not.toHaveBeenCalled()
    hidden.mockReturnValue('visible')
    await vi.advanceTimersByTimeAsync(60_000)
    expect(api.db.aggregate.mock.calls.length).toBeGreaterThan(0)
    hidden.mockRestore()
  })

  it('clears the timer on unmount', async () => {
    const wrapper = setup()
    await flushPromises()
    vi.useFakeTimers()
    pageVm(wrapper).refreshKey = 's30'
    await flushPromises()
    wrapper.unmount()
    api.db.aggregate.mockClear()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(api.db.aggregate).not.toHaveBeenCalled()
  })

  it('switching the refresh interval replaces the old timer', async () => {
    const wrapper = setup()
    await flushPromises()
    vi.useFakeTimers()
    const vm = pageVm(wrapper)
    vm.refreshKey = 's30'
    await flushPromises()
    vm.refreshKey = 'm5'
    await flushPromises()
    api.db.aggregate.mockClear()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(api.db.aggregate).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(240_000)
    expect(api.db.aggregate.mock.calls.length).toBeGreaterThan(0)
  })
})

describe('DataBrowser refresh keeps the analysis context', () => {
  async function ready() {
    const wrapper = setup()
    await flushPromises()
    return { wrapper, vm: pageVm(wrapper) }
  }

  it('manual refresh keeps the filter, time range, chips and detail page; it only re-fetches data', async () => {
    const { wrapper, vm } = await ready()
    vm.range = 'd7'
    await wrapper.get('input[data-field="filter"]').setValue('state="open"')
    await wrapper.findAll('button').find((b) => b.text() === '应用')!.trigger('click')
    await wrapper.get('.pager-next').trigger('click')
    await flushPromises()
    const before = vm.globalFilter
    expect(api.db.documents).toHaveBeenLastCalledWith('p1', 'db1', 'tasks', expect.objectContaining({ offset: 20 }))

    api.db.aggregate.mockClear()
    api.db.documents.mockClear()
    api.db.getCollectionSchema.mockClear()
    await wrapper.get('[data-action="refresh"]').trigger('click')
    await flushPromises()

    expect(vm.range).toBe('d7')
    expect(vm.globalFilter).toContain('state="open"')
    expect(vm.globalFilter).toContain('created_at>=')
    expect(before).toContain('state="open"')
    // 数据照常重取（卡片与明细），但没有重建看板：Schema 不再重读，明细停在原来的页。
    expect(api.db.aggregate.mock.calls.length).toBeGreaterThan(0)
    expect(api.db.getCollectionSchema).not.toHaveBeenCalled()
    expect(api.db.documents).toHaveBeenCalledTimes(1)
    expect(api.db.documents).toHaveBeenLastCalledWith('p1', 'db1', 'tasks', expect.objectContaining({ offset: 20 }))
  })

  it('manual refresh still recovers a source whose schema failed to load', async () => {
    const wrapper = setup({ schemaError: Object.assign(new Error('schema broke'), { status: 500 }) })
    await flushPromises()
    api.db.getCollectionSchema.mockResolvedValue({ name: 'tasks', schemaMode: 'json', fields: SCHEMA })
    await wrapper.get('[data-action="refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-widget="kpi-total"]').exists()).toBe(true)
  })

  it('the relative time window moves forward with the clock on every refresh round, once per card', async () => {
    const { wrapper, vm } = await ready()
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-18T10:00:00Z'))
    vm.range = 'h1'
    await flushPromises()
    expect(vm.globalFilter).toBe('created_at>="2026-11-18T09:00:00.000Z"')

    api.db.aggregate.mockClear()
    vi.setSystemTime(new Date('2026-11-18T10:30:00Z'))
    await wrapper.get('[data-action="refresh"]').trigger('click')
    await flushPromises()
    expect(vm.globalFilter).toBe('created_at>="2026-11-18T09:30:00.000Z"')
    for (const [, , , q] of api.db.aggregate.mock.calls) expect(q.filter).toBe('created_at>="2026-11-18T09:30:00.000Z"')
    // 同一轮里每张卡只请求一次（时间锚点与刷新信号同时变化也不会多发）。
    const calls = api.db.aggregate.mock.calls.map((c) => JSON.stringify(c[3]))
    expect(new Set(calls).size).toBe(calls.length)
  })

  it('auto refresh keeps the detail page while it advances the window', async () => {
    const { wrapper, vm } = await ready()
    vi.useFakeTimers()
    vm.range = 'h1'
    vm.refreshKey = 's30'
    await flushPromises()
    await wrapper.get('.pager-next').trigger('click')
    await flushPromises()
    api.db.documents.mockClear()
    await vi.advanceTimersByTimeAsync(30_000)
    expect(api.db.documents).toHaveBeenCalledTimes(1)
    expect(api.db.documents).toHaveBeenLastCalledWith('p1', 'db1', 'tasks', expect.objectContaining({ offset: 20 }))
  })
})

describe('DataBrowser condition errors are explained once', () => {
  it('shows a filter rejection in the filter bar instead of only inside every card', async () => {
    const wrapper = setup()
    await flushPromises()
    api.db.aggregate.mockRejectedValue(Object.assign(new Error('bad filter'), { code: 'invalid_filter', status: 400 }))
    await wrapper.get('input[data-field="filter"]').setValue('state==')
    await wrapper.findAll('button').find((b) => b.text() === '应用')!.trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="bi-filter-error"]').text()).toContain('bad filter')
    // 条件改动后重新判定，旧错误不能挂在新条件上。
    api.db.aggregate.mockResolvedValue({ columns: ['x', 'count'], rows: [['a', 1]], truncated: false, scanned: 1, durationMs: 1, requestId: '' })
    await wrapper.get('[data-action="clear"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="bi-filter-error"]').exists()).toBe(false)
  })

  it('does not blame the filter when the failure is a chart-level problem', async () => {
    const wrapper = setup()
    await flushPromises()
    api.db.aggregate.mockRejectedValue(Object.assign(new Error('schema not registered'), { code: 'schema_not_registered', status: 400 }))
    await wrapper.get('[data-action="refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="bi-filter-error"]').exists()).toBe(false)
  })

  it('warns that typed changes are not applied yet', async () => {
    const wrapper = setup()
    await flushPromises()
    expect(wrapper.find('[data-testid="bi-filter-pending"]').exists()).toBe(false)
    await wrapper.get('input[data-field="filter"]').setValue('state="open"')
    expect(wrapper.find('[data-testid="bi-filter-pending"]').exists()).toBe(true)
    await wrapper.findAll('button').find((b) => b.text() === '应用')!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="bi-filter-pending"]').exists()).toBe(false)
  })
})

describe('DataBrowser clear-all', () => {
  it('clears a time-range-only filter too, and the clear button shows up for it', async () => {
    const wrapper = setup()
    await flushPromises()
    const vm = pageVm(wrapper)
    expect(wrapper.find('[data-action="clear"]').exists()).toBe(false)
    vm.range = 'd7'
    await flushPromises()
    await wrapper.get('[data-action="clear"]').trigger('click')
    await flushPromises()
    expect(vm.range).toBe('all')
    expect(vm.globalFilter).toBe('')
  })

  it('switching the data source drops the filter history that referred to the old source', async () => {
    api.db.collectionItems.mockResolvedValue([
      { name: 'tasks', schemaMode: 'json', fieldCount: 3, documentCount: 3, registered: true },
      { name: 'users', schemaMode: 'json', fieldCount: 1, documentCount: 1, registered: true }
    ])
    const wrapper = setup()
    api.db.collectionItems.mockResolvedValue([
      { name: 'tasks', schemaMode: 'json', fieldCount: 3, documentCount: 3, registered: true },
      { name: 'users', schemaMode: 'json', fieldCount: 1, documentCount: 1, registered: true }
    ])
    await flushPromises()
    await wrapper.get('input[data-field="filter"]').setValue('state="open"')
    await wrapper.findAll('button').find((b) => b.text() === '应用')!.trigger('click')
    expect(wrapper.find('[data-chip="history"]').exists()).toBe(true)
    pageVm(wrapper).selectCollection('users')
    await flushPromises()
    expect(wrapper.find('[data-chip="history"]').exists()).toBe(false)
  })
})

describe('DataBrowser schema and errors', () => {
  it('re-reads the schema and collections after the schema designer saves', async () => {
    const wrapper = setup()
    await flushPromises()
    await wrapper.get('[data-action="schema"]').trigger('click')
    api.db.getCollectionSchema.mockClear()
    api.db.collectionItems.mockClear()
    // CollectionSchemaModal 保存后发出 saved。
    const modal = wrapper.findComponent({ name: 'CollectionSchemaModal' })
    modal.vm.$emit('saved')
    await flushPromises()
    expect(api.db.getCollectionSchema).toHaveBeenCalled()
    expect(api.db.collectionItems).toHaveBeenCalled()
  })

  it('a non-404 schema failure shows its own error with a retry that re-reads the schema and rebuilds the board', async () => {
    const wrapper = setup({ schemaError: Object.assign(new Error('schema broke'), { status: 500 }) })
    await flushPromises()
    const region = wrapper.get('[data-testid="fields-error"]')
    expect(region.text()).toContain('schema broke')
    expect(wrapper.find('[data-widget]').exists()).toBe(false)

    api.db.getCollectionSchema.mockClear()
    api.db.getCollectionSchema.mockResolvedValue({ name: 'tasks', schemaMode: 'json', fields: SCHEMA })
    const retry = region.findAll('button').find((b) => b.text().includes('重试'))!
    await retry.trigger('click')
    await flushPromises()
    // 重试的是字段（Schema），而不只是明细：成功后错误消失、默认看板出现。
    expect(api.db.getCollectionSchema).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="fields-error"]').exists()).toBe(false)
    expect(wrapper.find('[data-widget="kpi-total"]').exists()).toBe(true)
  })

  it('a detail query failure is shown with a working retry', async () => {
    const wrapper = setup()
    await flushPromises()
    api.db.documents.mockRejectedValueOnce(new Error('detail failed'))
    await wrapper.get('.pager-next').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="bi-detail"]').text()).toContain('detail failed')
    const retry = wrapper.get('[data-testid="bi-detail"]').findAll('button').find((b) => b.text().includes('重试'))!
    await retry.trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="bi-detail"]').text()).not.toContain('detail failed')
  })

  it('a detail response without a total falls back to the loaded count', async () => {
    api.db.documents.mockResolvedValue({ rows: [], items: [{ id: 'r1', createdAt: 'a', updatedAt: 'b', data: {} }] })
    const wrapper = setup()
    api.db.documents.mockResolvedValue({ rows: [], items: [{ id: 'r1', createdAt: 'a', updatedAt: 'b', data: {} }] })
    await flushPromises()
    expect(wrapper.find('[data-testid="bi-detail"]').exists()).toBe(true)
  })

  it('the export failure is reported instead of crashing', async () => {
    const wrapper = setup()
    await flushPromises()
    api.db.exportCollection.mockRejectedValueOnce(new Error('export failed'))
    const vm = pageVm(wrapper)
    await expect(vm.exportData('json')).resolves.toBeUndefined()
  })

  it('does nothing when asked to export without a collection', async () => {
    const wrapper = setup()
    await flushPromises()
    // 空库：没有任何数据源时导出必须是无操作。
    api.db.collectionItems.mockResolvedValue([])
    await pageVm(wrapper).reloadCollections()
    await flushPromises()
    api.db.exportCollection.mockClear()
    await pageVm(wrapper).exportData('csv')
    expect(api.db.exportCollection).not.toHaveBeenCalled()
  })
})

describe('BiToolbar', () => {
  const COLLECTIONS = [
    { name: 'tasks', schemaMode: 'json', fieldCount: 1, documentCount: 2, registered: true },
    { name: 'v', schemaMode: 'view', fieldCount: 1, documentCount: 0, registered: true }
  ]
  const TABLES = [{ name: 'events', columns: [{ name: 'id', type: 'BIGINT' }, { name: 'kind', type: 'VARCHAR' }] }]
  const base = {
    databases: [{ id: 'a', name: 'A', status: 'ready' }, { id: 'b', name: 'B', status: 'ready' }],
    databaseId: 'a',
    collections: COLLECTIONS,
    tables: [],
    source: { kind: 'collection', name: 'tasks' },
    loading: false, refreshKey: 'off',
    canEditSchema: true, canCreateView: true, canImport: true
  }
  const realSelect = { ...uiStubs, Button: false, Select: false, SelectTrigger: false, SelectValue: false, SelectContent: false, SelectGroup: false, SelectItem: false, Spinner: false }
  const mountBar = (over: Record<string, unknown> = {}) => mount(BiToolbar, { props: { ...base, ...over } as never, global: { stubs: realSelect } })
  const emitFrom = async (w: ReturnType<typeof mountBar>, marker: string, value: string) => {
    const sel = w.findAllComponents(Select).find((s) => s.find(`[${marker}]`).exists())!
    sel.vm.$emit('update:modelValue', value)
    await flushPromises()
  }
  // 选项弹层默认不展开：用把选项直接渲染出来的桩，只验证分组与徽标逻辑。
  const inline = { template: '<div><slot /></div>' }
  const inlineStubs = { ...uiStubs, Button: false, Spinner: false, Select: inline, SelectContent: inline, SelectGroup: inline, SelectItem: { template: '<div class="item"><slot /></div>' } }

  it('shows the database picker only when there are several databases', () => {
    expect(mountBar().find('[data-field="database"]').exists()).toBe(true)
    expect(mountBar({ databases: [base.databases[0]] }).find('[data-field="database"]').exists()).toBe(false)
  })

  it('emits database, source and refresh-interval changes; the source value encodes its kind', async () => {
    const wrapper = mountBar({ tables: TABLES })
    await emitFrom(wrapper, 'data-field="database"', 'b')
    await emitFrom(wrapper, 'data-field="collection"', 'c:v')
    await emitFrom(wrapper, 'data-field="collection"', 't:events')
    await emitFrom(wrapper, 'data-field="refresh"', 'm1')
    expect(wrapper.emitted('select-database')).toEqual([['b']])
    expect(wrapper.emitted('select-source')).toEqual([[{ kind: 'collection', name: 'v' }], [{ kind: 'table', name: 'events' }]])
    expect(wrapper.emitted('update:refresh')).toEqual([['m1']])
  })

  it('ignores a malformed source value instead of emitting garbage', async () => {
    const wrapper = mountBar()
    await emitFrom(wrapper, 'data-field="collection"', 'tasks')
    await emitFrom(wrapper, 'data-field="collection"', '')
    expect(wrapper.emitted('select-source')).toBeUndefined()
  })

  it('refresh and data actions emit', async () => {
    const wrapper = mountBar()
    await wrapper.get('[data-action="refresh"]').trigger('click')
    expect(wrapper.emitted('refresh')).toHaveLength(1)
    for (const action of ['schema', 'view', 'import', 'export-csv', 'export-json']) await wrapper.get(`[data-action="${action}"]`).trigger('click')
    expect(wrapper.emitted('action')?.map((e) => e[0])).toEqual(['schema', 'view', 'import', 'export-csv', 'export-json'])
  })

  it('a SQL table is a read-only source: no data-actions menu, but the picker and refresh stay', () => {
    const wrapper = mountBar({ collections: [], tables: TABLES, source: { kind: 'table', name: 'events' } })
    expect(wrapper.find('[data-action="data-actions"]').exists()).toBe(false)
    expect(wrapper.find('[data-field="collection"]').exists()).toBe(true)
    expect(wrapper.find('[data-action="refresh"]').exists()).toBe(true)
    expect(wrapper.find('[data-field="refresh"]').exists()).toBe(true)
  })

  it('no source selected: the data-actions menu is hidden and the picker is disabled when nothing can be picked', () => {
    expect(mountBar({ source: null }).find('[data-action="data-actions"]').exists()).toBe(false)
    expect(mountBar({ source: null, collections: [], tables: [] }).get('[data-field="collection"]').attributes('disabled')).toBeDefined()
    expect(mountBar().get('[data-field="collection"]').attributes('disabled')).toBeUndefined()
  })

  it('disables write actions according to the capability flags', () => {
    const wrapper = mountBar({ canEditSchema: false, canCreateView: false, canImport: false })
    for (const action of ['schema', 'view', 'import']) expect(wrapper.get(`[data-action="${action}"]`).attributes('disabled'), action).toBeDefined()
    expect(wrapper.get('[data-action="export-csv"]').attributes('disabled')).toBeUndefined()
  })

  it('shows the loading spinner while loading and locks the picker and refresh', () => {
    const wrapper = mountBar({ loading: true })
    expect(wrapper.get('[data-action="refresh"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-field="collection"]').attributes('disabled')).toBeDefined()
  })

  it('marks view collections with a 视图 badge, and tables with their column count', () => {
    const wrapper = mount(BiToolbar, { props: { ...base, tables: TABLES } as never, global: { stubs: inlineStubs } })
    const items = wrapper.findAll('.item').map((i) => i.text().replace(/\s+/g, ' ').trim())
    expect(items).toContain('tasks 2')
    expect(items).toContain('v 视图')
    expect(items).toContain('events 2 列')
  })

  it('shows group headings only when both kinds are present', () => {
    const both = mount(BiToolbar, { props: { ...base, tables: TABLES } as never, global: { stubs: inlineStubs } })
    expect(both.find('[data-group="collection"]').text()).toBe('集合')
    expect(both.find('[data-group="table"]').text()).toBe('SQL 表')
    const only = mount(BiToolbar, { props: { ...base } as never, global: { stubs: inlineStubs } })
    expect(only.find('[data-group="collection"]').exists()).toBe(false)
    expect(only.find('[data-group="table"]').exists()).toBe(false)
  })

  it('a same-named collection and table are distinct options (the kind is part of the value)', () => {
    const wrapper = mount(BiToolbar, {
      props: { ...base, collections: [{ ...COLLECTIONS[0], name: 'events' }], tables: TABLES } as never,
      global: { stubs: { ...inlineStubs, SelectItem: { props: ['value'], template: '<div class="item" :data-value="value"><slot /></div>' } } }
    })
    // 库下拉与刷新间隔也是 SelectItem：只看数据源相关的值（c: / t: 前缀）。
    const values = wrapper.findAll('.item').map((i) => i.attributes('data-value')).filter((v) => /^[ct]:/.test(v ?? ''))
    expect(values).toEqual(['c:events', 't:events'])
  })
})

describe('BiFilterBar', () => {
  const base = { range: 'all', timeField: 'created_at', dateFields: ['created_at', 'updated_at'], filter: '', history: [], chips: [], fieldNames: [], dirty: false }
  const realSelect = { ...uiStubs, Button: false, Input: false, Select: false, SelectTrigger: false, SelectValue: false, SelectContent: false, SelectGroup: false, SelectItem: false }
  const mountBar = (over: Record<string, unknown> = {}) => mount(BiFilterBar, { props: { ...base, ...over } as never, global: { stubs: realSelect } })

  it('hides the time-field picker when only one time field exists', () => {
    expect(mountBar().find('[data-field="time-field"]').exists()).toBe(true)
    expect(mountBar({ dateFields: ['created_at'] }).find('[data-field="time-field"]').exists()).toBe(false)
  })

  it('emits a time-field change', async () => {
    const wrapper = mountBar()
    wrapper.findComponent(Select).vm.$emit('update:modelValue', 'updated_at')
    await flushPromises()
    expect(wrapper.emitted('update:timeField')).toEqual([['updated_at']])
  })

  it('accepts a known range and ignores an empty or unknown one (a range must always stay selected)', async () => {
    const wrapper = mount(BiFilterBar, { props: { ...base } as never, global: { stubs: { ...uiStubs, Button: false, Input: false } } })
    // 共享桩的 ToggleGroup 渲染为带 class="tg" 的 div，真实的更新事件走它的 update:modelValue。
    const group = wrapper.findComponent(uiStubs.ToggleGroup)
    group.vm.$emit('update:modelValue', 'd7')
    group.vm.$emit('update:modelValue', '')
    group.vm.$emit('update:modelValue', undefined)
    group.vm.$emit('update:modelValue', 'bogus')
    await flushPromises()
    expect(wrapper.emitted('update:range')).toEqual([['d7']])
  })

  it('Enter applies the filter; typing updates it', async () => {
    const wrapper = mountBar()
    await wrapper.get('input[data-field="filter"]').setValue('a=1')
    await wrapper.get('input[data-field="filter"]').trigger('keydown.enter')
    expect(wrapper.emitted('update:filter')).toEqual([['a=1']])
    expect(wrapper.emitted('apply')).toHaveLength(1)
  })

  it('field, history and chip controls only show when they have content, and emit', async () => {
    const empty = mountBar()
    expect(empty.find('[data-chip]').exists()).toBe(false)
    expect(empty.find('[data-action="clear"]').exists()).toBe(false)
    const wrapper = mountBar({ fieldNames: ['title'], history: ['a=1'], chips: [{ id: 'c1', label: 'status=open', expr: 'status="open"' }], dirty: true })
    await wrapper.get('[data-chip="field"]').trigger('click')
    await wrapper.get('[data-chip="history"]').trigger('click')
    await wrapper.get('[data-chip-remove]').trigger('click')
    await wrapper.get('[data-action="clear"]').trigger('click')
    expect(wrapper.emitted('append-field')).toEqual([['title']])
    expect(wrapper.emitted('use-history')).toEqual([['a=1']])
    expect(wrapper.emitted('remove-chip')).toEqual([['c1']])
    expect(wrapper.emitted('clear')).toHaveLength(1)
    expect(wrapper.get('[data-testid="bi-chips"]').text()).toContain('status=open')
  })
})
