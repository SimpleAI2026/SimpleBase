import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { toast } from 'vue-sonner'
import { ADMIN_PROJECT_ID, useProjectStore } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, creatingDb, degradedDb, mountWithApp, readyDb, uiStubs } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

import Databases from '@/pages/Databases.vue'

const interactionCollectionStub = {
  CollectionPanel: {
    props: ['database', 'readonly'],
    emits: ['view-data', 'add-document', 'create-collection'],
    template: `
      <div class="coll-panel">
        <button type="button" class="view-data" @click="$emit('view-data', 'users')">view</button>
        <button type="button" class="add-doc" @click="$emit('add-document', 'users')">add</button>
        <button type="button" class="new-coll" @click="$emit('create-collection')">new</button>
      </div>
    `
  },
  SqlWorkModal: {
    props: ['open'],
    emits: ['update:open'],
    template:
      '<div v-if="open" class="sql-m"><button type="button" class="sql-close" @click="$emit(\'update:open\', false)">x</button></div>'
  },
  CreateCollectionModal: {
    props: ['open'],
    emits: ['created', 'update:open'],
    template:
      '<div v-if="open" class="cc-m"><button type="button" class="cc-created" @click="$emit(\'created\', \'users\')">ok</button><button type="button" class="cc-close" @click="$emit(\'update:open\', false)">x</button></div>'
  },
  DocumentListModal: {
    props: ['open'],
    emits: ['add-document', 'update:open'],
    template:
      '<div v-if="open" class="dl-m"><button type="button" class="dl-add" @click="$emit(\'add-document\')">add</button><button type="button" class="dl-close" @click="$emit(\'update:open\', false)">x</button></div>'
  },
  DocumentKvModal: {
    props: ['open'],
    emits: ['created', 'update:open'],
    template:
      '<div v-if="open" class="kv-m"><button type="button" class="kv-created" @click="$emit(\'created\')">ok</button><button type="button" class="kv-close" @click="$emit(\'update:open\', false)">x</button></div>'
  }
}


const deletingDb = {
  id: 'db-del',
  name: 'going',
  status: 'deleting' as const,
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z'
}

const dbStubs = {
  CollectionPanel: {
    props: ['database', 'readonly', 'reloadToken'],
    emits: ['view-data', 'add-document', 'create-collection'],
    template: `
      <div class="coll-panel" :data-readonly="readonly ? '1' : '0'" :data-reload="reloadToken">
        <button type="button" class="view-data" @click="$emit('view-data', 'users')">查看数据</button>
        <button type="button" class="add-doc" @click="$emit('add-document', 'users')">新增文档</button>
        <button type="button" class="new-coll" @click="$emit('create-collection')">新建集合</button>
      </div>
    `
  },
  SqlWorkModal: {
    props: ['open', 'readonly', 'database'],
    emits: ['update:open'],
    template:
      '<div v-if="open" class="sql-m" :data-readonly="readonly ? \'1\' : \'0\'">{{ database && database.name }}<button type="button" class="sql-close" @click="$emit(\'update:open\', false)">x</button></div>'
  },
  CreateCollectionModal: {
    props: ['open'],
    emits: ['created', 'update:open'],
    template:
      '<div v-if="open" class="cc-m"><button type="button" class="cc-created" @click="$emit(\'created\', \'users\')">ok</button></div>'
  },
  DocumentListModal: {
    props: ['open', 'readonly', 'collection'],
    emits: ['add-document', 'update:open'],
    template:
      '<div v-if="open" class="dl-m">{{ collection }}<button type="button" class="dl-add" @click="$emit(\'add-document\')">add</button><button type="button" class="dl-close" @click="$emit(\'update:open\', false)">x</button></div>'
  },
  DocumentKvModal: {
    props: ['open', 'collection'],
    emits: ['created', 'update:open'],
    template:
      '<div v-if="open" class="kv-m">{{ collection }}<button type="button" class="kv-created" @click="$emit(\'created\')">ok</button></div>'
  },
  SchemaPanel: {
    props: ['database', 'readonly'],
    template: '<div class="schema-panel" :data-readonly="readonly ? \'1\' : \'0\'">{{ database.name }}</div>'
  }
}

function expandButtons(wrapper: Awaited<ReturnType<typeof mountWithApp>>['wrapper']) {
  return wrapper.findAll('button').filter((b) => (b.attributes('class') || '').includes('rounded-full'))
}

describe('Databases (数据库管理)', () => {
  beforeEach(() => {
    resetApiMocks()
    api.databases.list.mockResolvedValue([readyDb, creatingDb, degradedDb, deletingDb])
    api.db.collections.mockResolvedValue(['users'])
  })

  it('lists databases with status labels and write actions', async () => {
    const { wrapper } = await mountWithApp(Databases, { stubs: dbStubs })
    expect(wrapper.text()).toContain('DuckLake 数据库：集合文档或 SQL 表，新建后即可使用')
    expect(wrapper.text()).toContain('数据库列表')
    expect(wrapper.text()).toContain('demo')
    expect(wrapper.text()).toContain('就绪')
    expect(wrapper.text()).toContain('创建中')
    expect(wrapper.text()).toContain('降级')
    expect(wrapper.text()).toContain('删除中')
    expect(wrapper.text()).not.toContain('已关闭')
    expect(wrapper.text()).not.toContain('打开')
    expect(wrapper.text()).not.toContain('关闭')
    expect(wrapper.text()).toContain('新建数据库')
    expect(wrapper.text()).toContain('SQL')
    expect(wrapper.text()).toContain('新建集合')
    const sqlButtons = wrapper.findAll('button').filter((b) => b.text().includes('SQL'))
    expect(sqlButtons[0].attributes('disabled')).toBeUndefined()
    expect(sqlButtons[1].attributes('disabled')).toBeDefined()
    expect(sqlButtons[2].attributes('disabled')).toBeDefined()
    await expandButtons(wrapper)[0].trigger('click')
    expect(wrapper.get('[colspan="7"]').classes()).toContain('text-left')
  })

  it('creates a database after validating the name', async () => {
    const { wrapper } = await mountWithApp(Databases, { stubs: dbStubs })
    await clickText(wrapper, '新建数据库')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)

    await wrapper.get('.sb-ok').trigger('click')
    expect(toast.warning).toHaveBeenCalledWith('请输入数据库名称')

    await wrapper.get('#db-name').setValue('bad/name')
    await wrapper.get('.sb-ok').trigger('click')
    expect(toast.warning).toHaveBeenCalledWith('名称不能包含 / \\ 或控制字符')

    await wrapper.get('#db-name').setValue('x'.repeat(64))
    expect(wrapper.text()).toContain('名称不能超过 63 字节')
    await wrapper.get('.sb-ok').trigger('click')
    expect(toast.warning).toHaveBeenCalledWith('名称不能超过 63 字节')

    await wrapper.get('#db-name').setValue('okdb')
    await wrapper.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.databases.create).toHaveBeenCalledWith(expect.any(String), {
      name: 'okdb',
      dataModel: 'collection',
      initSql: undefined
    })
    expect(toast.success).toHaveBeenCalledWith(expect.stringContaining('就绪'))
    expect(wrapper.find('.sb-modal').exists()).toBe(false)
  })

  it('creates a SQL database with init SQL and manages schema instead of collections', async () => {
    const sqlDb = {
      id: 'db-sql',
      name: 'sqldb',
      status: 'ready' as const,
      createdAt: '2024-01-01T00:00:00Z',
      updatedAt: '2024-01-01T00:00:00Z',
      dataModel: 'sql' as const
    }
    api.databases.list.mockResolvedValue([readyDb, sqlDb])
    const { wrapper } = await mountWithApp(Databases, { stubs: { ...dbStubs, FieldGroup: false } })
    expect(wrapper.text()).toContain('集合')
    expect(wrapper.html()).toContain('justify-center')
    expect(wrapper.html()).not.toContain('text-right')

    await clickText(wrapper, '新建数据库')
    const group = wrapper.get('[data-slot="field-group"]')
    expect(group.classes()).toContain('gap-5')
    expect(group.text()).toContain('名称')
    expect(group.text()).toContain('数据类型')
    expect(wrapper.find('#db-init-sql').exists()).toBe(false)
    await clickText(wrapper, 'SQL 数据')
    expect(wrapper.find('#db-init-sql').exists()).toBe(true)
    expect(group.text()).toContain('初始化 SQL')
    expect(wrapper.get('[data-slot="field-group"]').classes()).toContain('gap-5')
    await clickText(wrapper, '集合文档')
    expect(wrapper.find('#db-init-sql').exists()).toBe(false)
    await clickText(wrapper, 'SQL 数据')
    await wrapper.get('#db-init-sql').setValue('  CREATE TABLE t (id INTEGER);  ')
    await wrapper.get('#db-name').setValue('orders')
    await wrapper.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.databases.create).toHaveBeenCalledWith(expect.any(String), {
      name: 'orders',
      dataModel: 'sql',
      initSql: 'CREATE TABLE t (id INTEGER);'
    })

    const cards = wrapper.get('[aria-label="数据库移动端列表"]').findAll('article')
    const sqlCard = cards.find((card) => card.text().includes('sqldb'))!
    expect(sqlCard.text()).toContain('SQL')
    expect(sqlCard.findAll('button').some((button) => button.text().trim() === '新建集合')).toBe(false)
    await sqlCard.findAll('button').find((button) => button.text() === '查看数据')!.trigger('click')
    expect(sqlCard.find('.schema-panel').exists()).toBe(true)
    expect(sqlCard.find('.coll-panel').exists()).toBe(false)

    const collectionCard = cards.find((card) => card.text().includes('demo'))!
    expect(collectionCard.text()).toContain('集合')
    expect(collectionCard.findAll('button').some((button) => button.text().trim() === '新建集合')).toBe(true)
    await collectionCard.findAll('button').find((button) => button.text() === '查看数据')!.trigger('click')
    expect(collectionCard.find('.coll-panel').exists()).toBe(true)
    expect(wrapper.find('.schema-panel').exists()).toBe(true)
  })

  it('surfaces create and delete errors', async () => {
    api.databases.create.mockRejectedValueOnce(new Error('create-fail'))
    api.databases.remove.mockRejectedValueOnce(new Error('rm-fail'))
    const { wrapper } = await mountWithApp(Databases, { stubs: dbStubs })

    await clickText(wrapper, '新建数据库')
    await wrapper.get('#db-name').setValue('okdb')
    await wrapper.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('create-fail')

    await clickText(wrapper, '删除')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('rm-fail')
  })

  it('deletes a ready database and pages the table', async () => {
    const { wrapper } = await mountWithApp(Databases, { stubs: dbStubs })
    await clickText(wrapper, 'SQL')
    expect(wrapper.get('.sql-m').text()).toContain('demo')
    await wrapper.get('.sql-close').trigger('click')

    await clickText(wrapper, '新建集合')
    expect(wrapper.find('.cc-m').exists()).toBe(true)
    await wrapper.get('.cc-created').trigger('click')

    await clickText(wrapper, '删除')
    await flushPromises()
    expect(api.databases.remove).toHaveBeenCalledWith(expect.any(String), 'db-1')

    await wrapper.get('.pager-next').trigger('click')
    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(api.databases.list.mock.calls.length).toBeGreaterThan(1)
  })

  it('keeps the mobile database card actions and expanded content accessible', async () => {
    const { wrapper } = await mountWithApp(Databases, { stubs: dbStubs })
    const cards = wrapper.get('[aria-label="数据库移动端列表"]')
    expect(cards.findAll('article')).toHaveLength(4)
    const readyCard = cards.findAll('article')[0]
    expect(readyCard.text()).toContain('demo')
    expect(readyCard.text()).toContain('就绪')
    const expand = readyCard.findAll('button').find((button) => button.text() === '查看数据')!
    expect(expand.attributes('aria-expanded')).toBe('false')
    await expand.trigger('click')
    await flushPromises()
    expect(expand.attributes('aria-expanded')).toBe('true')
    expect(readyCard.find('.coll-panel').exists()).toBe(true)
    expect(expandButtons(wrapper)[0].attributes('aria-label')).toContain('demo')
    expect(expandButtons(wrapper)[0].attributes('aria-expanded')).toBe('true')
  })

  it('expands collections, opens SQL, and wires document modals', async () => {
    const { wrapper } = await mountWithApp(Databases, { stubs: dbStubs })
    const expand = expandButtons(wrapper)[0]
    await expand.trigger('click')
    await flushPromises()
    expect(wrapper.find('.coll-panel').exists()).toBe(true)

    await wrapper.get('.view-data').trigger('click')
    expect(wrapper.get('.dl-m').text()).toContain('users')
    await wrapper.get('.dl-add').trigger('click')
    expect(wrapper.find('.kv-m').exists()).toBe(true)
    await wrapper.get('.dl-close').trigger('click')

    await wrapper.get('.add-doc').trigger('click')
    expect(wrapper.find('.kv-m').exists()).toBe(true)
    await wrapper.get('.kv-created').trigger('click')
    expect(wrapper.find('.dl-m').exists()).toBe(true)

    await wrapper.get('.new-coll').trigger('click')
    expect(wrapper.find('.cc-m').exists()).toBe(true)
    await wrapper.get('.cc-created').trigger('click')
    expect(wrapper.get('.coll-panel').attributes('data-reload')).not.toBe('0')

    await clickText(wrapper, 'SQL')
    expect(wrapper.get('.sql-m').text()).toContain('demo')
    await wrapper.get('.sql-close').trigger('click')

    await clickText(wrapper, '新建集合')
    expect(wrapper.find('.cc-m').exists()).toBe(true)

    await expand.trigger('click')
    expect(wrapper.find('.coll-panel').exists()).toBe(false)
  })

  it('does not expand a database that is not ready', async () => {
    const { wrapper } = await mountWithApp(Databases, { stubs: dbStubs })
    const expand = expandButtons(wrapper)[1]
    expect(expand.attributes('disabled')).toBeDefined()
    await expand.trigger('click')
    expect(wrapper.find('.coll-panel').exists()).toBe(false)
  })

  it('hides write actions on the admin project but keeps SQL and expand', async () => {
    const { wrapper } = await mountWithApp(Databases, {
      projectId: ADMIN_PROJECT_ID,
      stubs: dbStubs
    })
    expect(wrapper.text()).toContain('系统数据库')
    expect(wrapper.text()).toContain('受保护')
    expect(wrapper.text()).toContain('只读且不可删除')
    expect(wrapper.findAll('button').some((b) => b.text().includes('新建数据库'))).toBe(false)
    expect(wrapper.findAll('button').some((b) => b.text().trim() === '删除')).toBe(false)
    expect(wrapper.findAll('button').some((b) => b.text().trim() === '打开')).toBe(false)
    expect(wrapper.findAll('button').some((b) => b.text().trim() === '关闭')).toBe(false)
    expect(wrapper.findAll('button').some((b) => b.text().trim() === '新建集合')).toBe(false)
    expect(wrapper.text()).toContain('SQL')
    await expandButtons(wrapper)[0].trigger('click')
    await flushPromises()
    expect(wrapper.get('.schema-panel').attributes('data-readonly')).toBe('1')
    expect(wrapper.find('.coll-panel').exists()).toBe(false)
    await clickText(wrapper, 'SQL')
    expect(wrapper.get('.sql-m').attributes('data-readonly')).toBe('1')
    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(api.databases.list).toHaveBeenCalled()
  })

  it('opens create from empty state and reloads on project change', async () => {
    api.databases.list.mockResolvedValueOnce([])
    const { wrapper, pinia } = await mountWithApp(Databases, { stubs: dbStubs })
    expect(wrapper.text()).toContain('暂无数据库')
    await wrapper.get('.empty-action').trigger('click')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)

    const { useProjectStore } = await import('@/stores/project')
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    expect(api.databases.list.mock.calls.length).toBeGreaterThan(1)
  })

  it('shows an admin empty state without a create action', async () => {
    api.databases.list.mockResolvedValueOnce([])
    const { wrapper } = await mountWithApp(Databases, {
      projectId: ADMIN_PROJECT_ID,
      stubs: dbStubs
    })
    expect(wrapper.text()).toContain('暂无系统库')
    expect(wrapper.find('.empty-action').exists()).toBe(false)
  })
})

describe('Databases desktop table actions', () => {
  beforeEach(() => {
    resetApiMocks()
    api.databases.list.mockResolvedValue([readyDb, creatingDb])
    api.db.collections.mockResolvedValue(['users'])
  })

  it('wires SQL, collection, delete and expanded panel events from the desktop table', async () => {
    const { wrapper } = await mountWithApp(Databases, { stubs: dbStubs })
    const desktop = wrapper.get('.md\\:block')
    await desktop.findAll('button').find((b) => b.text() === 'SQL')!.trigger('click')
    expect(wrapper.get('.sql-m').text()).toContain('demo')
    await wrapper.get('.sql-close').trigger('click')
    await desktop.findAll('button').find((b) => b.text() === '新建集合')!.trigger('click')
    expect(wrapper.find('.cc-m').exists()).toBe(true)
    await desktop.findAll('.confirm-action')[0].trigger('click')
    await flushPromises()
    expect(api.databases.remove).toHaveBeenCalledWith(expect.any(String), 'db-1')

    await expandButtons(wrapper)[0].trigger('click')
    await flushPromises()
    const panel = wrapper.get('.md\\:block').get('.coll-panel')
    await panel.get('.view-data').trigger('click')
    expect(wrapper.get('.dl-m').text()).toContain('users')
    await wrapper.get('.dl-close').trigger('click')
    await panel.get('.add-doc').trigger('click')
    expect(wrapper.find('.kv-m').exists()).toBe(true)
    await panel.get('.new-coll').trigger('click')
    expect(wrapper.find('.cc-m').exists()).toBe(true)
  })

  it('opens create from the desktop empty state and closes the create modal', async () => {
    api.databases.list.mockResolvedValueOnce([])
    const { wrapper } = await mountWithApp(Databases, { stubs: dbStubs })
    await wrapper.get('.md\\:block').get('.empty-action').trigger('click')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)
    await wrapper.get('.sb-cancel').trigger('click')
    expect(wrapper.find('.sb-modal').exists()).toBe(false)
  })
  it('Databases clicks expand, sql, collection, docs, delete, pager and project watch', async () => {
    const { wrapper, pinia } = await mountWithApp(Databases, { stubs: interactionCollectionStub })
    await clickText(wrapper, '刷新')
    await clickText(wrapper, '新建数据库')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)
    await wrapper.get('#db-name').setValue('okdb')
    await wrapper.get('.sb-ok').trigger('click')
    await flushPromises()
    if (wrapper.find('.sb-cancel').exists()) await wrapper.get('.sb-cancel').trigger('click')

    const expand = wrapper.findAll('button').find((b) => b.attributes('class')?.includes('rounded-full'))
    if (expand) await expand.trigger('click')
    await flushPromises()
    if (wrapper.find('.view-data').exists()) {
      await wrapper.get('.view-data').trigger('click')
      await wrapper.get('.dl-add').trigger('click')
      await wrapper.get('.dl-close').trigger('click')
      await wrapper.get('.add-doc').trigger('click')
      await wrapper.get('.kv-created').trigger('click')
      await wrapper.get('.kv-close').trigger('click')
      await wrapper.get('.new-coll').trigger('click')
      await wrapper.get('.cc-created').trigger('click')
      await wrapper.get('.cc-close').trigger('click')
    }
    await clickText(wrapper, 'SQL')
    if (wrapper.find('.sql-close').exists()) await wrapper.get('.sql-close').trigger('click')
    await clickText(wrapper, '新建集合')
    await clickText(wrapper, '删除')
    await wrapper.get('.pager-next').trigger('click')
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    wrapper.unmount()
  })

  it('Databases empty-state create and admin refresh', async () => {
    api.databases.list.mockResolvedValueOnce([])
    const { wrapper } = await mountWithApp(Databases, { stubs: interactionCollectionStub })
    await wrapper.get('.empty-action').trigger('click')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)
    wrapper.unmount()

    api.databases.list.mockResolvedValue([readyDb])
    const admin = await mountWithApp(Databases, {
      projectId: 'sb-admin',
      stubs: interactionCollectionStub
    })
    await clickText(admin.wrapper, '刷新')
    admin.wrapper.unmount()
  })

  it('Databases onDocumentCreated reopens list when closed', async () => {
    setActivePinia(createPinia())
    useProjectStore().setProject('dev-shop')
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore().setProject('dev-shop')
    const w = mount(Databases, {
      global: { plugins: [pinia], stubs: { ...uiStubs, ...interactionCollectionStub } }
    })
    await flushPromises()
    const vm = w.vm as any
    vm.activeDb = readyDb
    vm.activeCollection = 'users'
    vm.docListOpen = false
    vm.onDocumentCreated()
    expect(vm.docListOpen).toBe(true)
    vm.onAddDocumentFromList()
    expect(vm.kvOpen).toBe(true)
    w.unmount()
  })
  it('Databases methods cover not-ready, missing active db, and non-Error catches', async () => {
    const { wrapper } = await mountWithApp(Databases)
    const vm = wrapper.vm as Record<string, any>
    vm.toggleExpand({ ...creatingDb })
    vm.toggleExpand(readyDb)
    vm.toggleExpand(readyDb)
    vm.activeDb = null
    vm.onCollectionCreated()
    vm.activeCollection = ''
    vm.onAddDocumentFromList()

    api.databases.create.mockRejectedValueOnce('create-fail')
    vm.newName = 'okdb'
    await vm.create()
    api.databases.remove.mockRejectedValueOnce('rm-fail')
    await vm.removeDb(readyDb)
    vm.createVisible = true
    if (wrapper.find('.sb-cancel').exists()) await wrapper.get('.sb-cancel').trigger('click')
    wrapper.unmount()
  })


  it('covers database status and operation boundaries', async () => {
    const { wrapper: db } = await mountWithApp(Databases, { stubs: dbStubs })
    await flushPromises()
    expect(api.databases.list).toHaveBeenCalled()
    const dbVm = db.vm as any
    const item = { id: 'd', name: 'n', status: 'ready' }
    expect(dbVm.isReady(item)).toBe(true)
    expect(dbVm.isReady({ status: 'creating' })).toBe(false)
    expect(dbVm.isReady({ status: 'degraded' })).toBe(false)
    dbVm.toggleExpand(item)
    dbVm.toggleExpand(item)
    dbVm.openCreate()
    dbVm.openSql(item)
    dbVm.openCreateCollection(item)
    dbVm.openDocList(item, 'users')
    dbVm.openKv(item, 'users')
    dbVm.onCollectionCreated()
    dbVm.onAddDocumentFromList()
    dbVm.onDocumentCreated()
    api.databases.create.mockResolvedValue(item)
    dbVm.newName = 'newdb'
    await dbVm.create()
    dbVm.newName = ''
    await dbVm.create()
    dbVm.newName = 'bad/name'
    await dbVm.create()
    dbVm.newName = 'x'.repeat(80)
    await dbVm.create()
    api.databases.create.mockRejectedValueOnce(new Error('x'))
    dbVm.newName = 'failok'
    await dbVm.create()
    await dbVm.removeDb(item)
    api.databases.remove.mockRejectedValueOnce(new Error('x'))
    await dbVm.removeDb(item)
    db.unmount()

  })

})
