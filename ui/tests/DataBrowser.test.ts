import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DataBrowser from '@/pages/DataBrowser.vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import { useProjectStore } from '@/stores/project'
import type { CollectionField, DocumentItem } from '@/services/types'

interface DataBrowserVm {
  rows: DocumentItem[]
  databaseId: string
  collection: string
  setCell: (id: string, column: string, value: string | number | boolean) => void
  appendField: (name: string) => void
  applyQuery: () => void
  goToPage: (page: number) => void
  toggleColumn: (column: string, visible: boolean) => void
  selectDatabase: (value: string) => void
  selectCollection: (name: string) => void
  reloadCollections: () => Promise<void>
  refresh: () => Promise<void>
  saveRow: (row: DocumentItem) => Promise<void>
  exportData: (format: 'csv' | 'json') => Promise<void>
  load: () => Promise<void>
  loadSchema: () => Promise<void>
  loadDatabases: (preserveSelection?: boolean) => Promise<void>
  loadCollections: (preserveSelection?: boolean) => Promise<void>
}

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

const field = (name: string, type: CollectionField['type'], values?: string[]): CollectionField => ({
  name, type, required: false, unique: false, hidden: false, readonly: false,
  options: values ? { values } : {}
})

function setup(view = false, realSelect = false) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useProjectStore().setProject('p1', 'Project')
  api.databases.list.mockResolvedValue([{ id: 'db1', name: 'Database', status: 'ready', dataModel: 'collection' }])
  api.db.collectionItems.mockResolvedValue([{ name: 'tasks', schemaMode: view ? 'view' : 'json', fieldCount: 8, documentCount: 1, registered: true }])
  api.db.getCollectionSchema.mockResolvedValue({
    name: 'tasks', schemaMode: view ? 'view' : 'json', fields: [
      field('title', 'text'), field('priority', 'number'), field('active', 'bool'), field('published', 'date'),
      field('email', 'email'), field('website', 'url'), field('state', 'select', ['open', 'closed']), field('payload', 'json')
    ]
  })
  api.db.documents.mockResolvedValue({
    rows: [], total: 1,
    items: [{
      id: 'r1', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
      data: { title: 'One', priority: 1, active: true, published: '2026-01-01T00:00:00Z', email: 'a@example.com', website: 'https://example.com', state: 'open', payload: { owner: 'Ada' } }
    }]
  })
  api.db.exportCollection.mockResolvedValue(new Blob(['id,title']))
  const stubs = realSelect
    ? { ...uiStubs, Select: false, SelectTrigger: false, SelectValue: false, SelectContent: false, SelectGroup: false, SelectItem: false, teleport: false }
    : uiStubs
  return mount(DataBrowser, { global: { plugins: [pinia], stubs }, attachTo: realSelect ? document.body : undefined })
}

describe('DataBrowser', () => {
  const downloads: string[] = []

  beforeEach(() => {
    resetApiMocks()
    downloads.length = 0
    Object.assign(URL, { createObjectURL: vi.fn(() => 'blob:test'), revokeObjectURL: vi.fn() })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      downloads.push(this.download)
    })
  })

  it('queries, toggles columns, patches edited fields and exports', async () => {
    const wrapper = setup()
    await flushPromises()
    expect(wrapper.text()).toContain('tasks')
    expect(wrapper.find('input[type="number"]').exists()).toBe(true)
    expect(wrapper.find('input[type="email"]').exists()).toBe(true)
    expect(wrapper.find('input[type="url"]').exists()).toBe(true)
    expect(wrapper.find('input[type="datetime-local"]').exists()).toBe(true)
    await wrapper.find('tbody input[type="text"]').setValue('Changed')
    await wrapper.findAll('button').find((button) => button.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(api.db.patch).toHaveBeenCalledWith('p1', 'db1', 'tasks', 'r1', { title: 'Changed' })

    const titleToggle = wrapper.findAll('label').find((label) => label.text().endsWith('title'))!
    await titleToggle.get('button').trigger('click')
    expect(wrapper.findAll('th').some((header) => header.text() === 'title')).toBe(false)
    const dataVm = wrapper.vm as unknown as DataBrowserVm
    dataVm.toggleColumn('title', true)
    await wrapper.findAll('button').find((button) => button.text() === '导出 CSV')!.trigger('click')
    await wrapper.findAll('button').find((button) => button.text() === '导出 JSON')!.trigger('click')
    await flushPromises()
    expect(api.db.exportCollection).toHaveBeenCalledWith('p1', 'db1', 'tasks', 'csv', expect.any(Object))
    expect(api.db.exportCollection).toHaveBeenCalledWith('p1', 'db1', 'tasks', 'json', expect.any(Object))
    expect(downloads).toEqual(['tasks.csv', 'tasks.json'])
    expect(wrapper.text()).not.toContain('导出 NDJSON')
    await wrapper.findAll('button').find((button) => button.text() === '查询')!.trigger('click')
    await flushPromises()
    expect(api.db.documents).toHaveBeenCalled()
  })

  it('covers typed edits, pagination, no-op saves and error paths', async () => {
    const wrapper = setup()
    await flushPromises()
    const vm = wrapper.vm as unknown as DataBrowserVm
    const row = vm.rows[0]
    vm.setCell(row.id, 'priority', '4')
    vm.setCell(row.id, 'published', '2026-02-01T10:00')
    vm.setCell(row.id, 'payload', '{"owner":"Grace"}')
    vm.setCell(row.id, 'payload', '{bad')
    await vm.saveRow(row)
    expect(api.db.patch).toHaveBeenCalled()
    await vm.saveRow(row)
    api.db.patch.mockRejectedValueOnce(new Error('save'))
    vm.setCell(row.id, 'title', 'Again')
    await vm.saveRow(row)
    vm.appendField('title')
    vm.appendField('priority')
    vm.applyQuery()
    vm.goToPage(2)
    vm.goToPage(0)
    vm.selectCollection('other')
    vm.selectDatabase('')
    await vm.reloadCollections()
    await vm.exportData('json')
    api.db.exportCollection.mockRejectedValueOnce(new Error('export'))
    await vm.exportData('csv')
    api.db.documents.mockRejectedValueOnce(new Error('load'))
    await vm.load()
    api.db.getCollectionSchema.mockRejectedValueOnce(Object.assign(new Error('schema'), { status: 500 }))
    await vm.load()
    api.db.getCollectionSchema.mockRejectedValueOnce(Object.assign(new Error('missing'), { status: 404 }))
    await vm.load()
    vm.setCell(row.id, 'priority', '')
    vm.setCell(row.id, 'published', '')
    vm.goToPage(999)
    useProjectStore().projectId = ''
    await vm.load()
    await vm.loadSchema()
    await vm.loadDatabases()
    await vm.loadCollections()
    await flushPromises()
    expect(api.db.getCollectionSchema.mock.calls.length).toBeGreaterThanOrEqual(3)
  })

  it('wires database, query, collection, modal and cell events', async () => {
    const wrapper = setup()
    await flushPromises()
    await wrapper.get('.select-emit').trigger('click')
    await flushPromises()
    const filter = wrapper.get('input[placeholder^="例如 status"]')
    await filter.setValue('title="One"')
    await wrapper.get('input[placeholder="例如 -created_at,title"]').setValue('title')
    await wrapper.get('aside button').trigger('click')
    await wrapper.findAll('main button').find((button) => button.text() === 'title')!.trigger('click')
    await wrapper.findAll('button').find((button) => button.text() === '查询')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text() === 'title="One" && title')!.trigger('click')
    for (const label of ['Schema 设计器', '新建视图', '导入数据']) {
      await wrapper.findAll('button').find((button) => button.text() === label)!.trigger('click')
      await wrapper.get('.sb-modal-close').trigger('click')
    }
    await wrapper.findAll('button').find((button) => button.text() === '导出 JSON')!.trigger('click')
    await wrapper.get('tbody .switch').trigger('click')
    await wrapper.get('tbody .select-emit').trigger('click')
    await wrapper.get('tbody textarea').setValue('{"changed":true}')
    await flushPromises()
    expect(api.db.exportCollection).toHaveBeenCalled()
  })

  it('keeps the newest database selection when an older collection request resolves late', async () => {
    const wrapper = setup()
    await flushPromises()
    const vm = wrapper.vm as unknown as DataBrowserVm
    api.databases.list.mockResolvedValue([
      { id: 'db1', name: 'Database 1', status: 'ready', dataModel: 'collection' },
      { id: 'db2', name: 'Database 2', status: 'ready', dataModel: 'collection' }
    ])
    await vm.loadDatabases(true)

    let resolveOld!: (items: { name: string }[]) => void
    api.db.collectionItems.mockImplementation((_project, database) => {
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

  it('refreshes the full resource chain and recovers from resource errors', async () => {
    api.databases.list.mockRejectedValueOnce(new Error('offline'))
    const wrapper = setup()
    await flushPromises()
    const vm = wrapper.vm as unknown as DataBrowserVm
    expect(wrapper.text()).toContain('offline')

    api.databases.list.mockResolvedValue([{ id: 'db1', name: 'Database', status: 'ready', dataModel: 'collection' }])
    await vm.refresh()
    await flushPromises()
    expect(wrapper.text()).toContain('tasks')
    expect(vm.databaseId).toBe('db1')

    const databaseCalls = api.databases.list.mock.calls.length
    const collectionCalls = api.db.collectionItems.mock.calls.length
    await vm.refresh()
    await flushPromises()
    expect(api.databases.list.mock.calls.length).toBeGreaterThan(databaseCalls)
    expect(api.db.collectionItems.mock.calls.length).toBeGreaterThan(collectionCalls)
    expect(vm.collection).toBe('tasks')

    api.db.collectionItems.mockRejectedValueOnce(new Error('collections offline'))
    await vm.loadCollections(true)
    expect(wrapper.text()).toContain('collections offline')
    await vm.reloadCollections()
    expect(wrapper.text()).toContain('tasks')
  })

  it('keeps logical views read-only', async () => {
    const wrapper = setup(true)
    await flushPromises()
    expect(wrapper.text()).toContain('VIEW')
    expect(wrapper.findAll('button').some((button) => button.text() === '保存')).toBe(false)
    expect(wrapper.find('input[type="number"]').exists()).toBe(false)
  })

  it('selects SQL databases and loads tables instead of collections', async () => {
    const wrapper = setup()
    await flushPromises()
    const vm = wrapper.vm as unknown as DataBrowserVm
    api.databases.list.mockResolvedValue([
      { id: 'db1', name: 'Documents', status: 'ready', dataModel: 'collection' },
      { id: 'sql1', name: 'Analytics', status: 'ready', dataModel: 'sql' }
    ])
    api.databases.schema.mockResolvedValue({ tables: [{ name: 'events', columns: [] }] })
    await vm.loadDatabases(true)
    const collectionCalls = api.db.collectionItems.mock.calls.length

    vm.selectDatabase('sql1')
    await flushPromises()

    expect(vm.databaseId).toBe('sql1')
    expect(api.db.collectionItems).toHaveBeenCalledTimes(collectionCalls)
    expect(api.databases.schema).toHaveBeenCalledWith('p1', 'sql1')
    expect(wrapper.get('[data-testid="schema-panel"]').text()).toContain('events')
  })

  it('changes databases through the real Select interaction', async () => {
    const wrapper = setup(false, true)
    await flushPromises()
    const vm = wrapper.vm as unknown as DataBrowserVm
    api.databases.list.mockResolvedValue([
      { id: 'db1', name: 'Documents', status: 'ready', dataModel: 'collection' },
      { id: 'db2', name: 'Orders', status: 'ready', dataModel: 'collection' }
    ])
    await vm.loadDatabases(true)

    await wrapper.get('[role="combobox"]').trigger('keydown', { key: 'Enter' })
    await flushPromises()
    const option = [...document.body.querySelectorAll('[role="option"]')].find((item) => item.textContent?.includes('Orders'))
    expect(option).toBeDefined()
    option!.dispatchEvent(new MouseEvent('pointerup', { bubbles: true }))
    await flushPromises()

    expect(vm.databaseId).toBe('db2')
    expect(api.db.collectionItems).toHaveBeenLastCalledWith('p1', 'db2')
    wrapper.unmount()
  })
})
