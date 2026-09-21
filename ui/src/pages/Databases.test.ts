import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { ADMIN_PROJECT_ID } from '../stores/project'
import { api, resetApiMocks } from '../test/api-mock'
import { clickText, closedDb, mountWithApp, readyDb } from '../test/helpers'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return { api: m.api, isMock: false }
})

import Databases from './Databases.vue'

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
  }
}

function expandButtons(wrapper: Awaited<ReturnType<typeof mountWithApp>>['wrapper']) {
  return wrapper.findAll('button').filter((b) => (b.attributes('class') || '').includes('rounded-full'))
}

describe('Databases (数据库管理)', () => {
  beforeEach(() => {
    resetApiMocks()
    api.databases.list.mockResolvedValue([readyDb, closedDb, deletingDb])
    api.db.collections.mockResolvedValue(['users'])
  })

  it('lists databases with status labels and write actions', async () => {
    const { wrapper } = await mountWithApp(Databases, { stubs: dbStubs })
    expect(wrapper.text()).toContain('DuckLake 数据库、SQL 工作台与集合文档')
    expect(wrapper.text()).toContain('数据库列表')
    expect(wrapper.text()).toContain('demo')
    expect(wrapper.text()).toContain('就绪')
    expect(wrapper.text()).toContain('已关闭')
    expect(wrapper.text()).toContain('删除中')
    expect(wrapper.text()).toContain('新建数据库')
    expect(wrapper.text()).toContain('SQL')
    expect(wrapper.text()).toContain('打开')
    expect(wrapper.text()).toContain('关闭')
    expect(wrapper.text()).toContain('新建集合')
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
    expect(api.databases.create).toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalled()
    expect(wrapper.find('.sb-modal').exists()).toBe(false)
  })

  it('surfaces create / open / close / delete errors', async () => {
    api.databases.create.mockRejectedValueOnce(new Error('create-fail'))
    api.databases.open.mockRejectedValueOnce(new Error('open-fail'))
    api.databases.close.mockRejectedValueOnce(new Error('close-fail'))
    api.databases.remove.mockRejectedValueOnce(new Error('rm-fail'))
    const { wrapper } = await mountWithApp(Databases, { stubs: dbStubs })

    await clickText(wrapper, '新建数据库')
    await wrapper.get('#db-name').setValue('okdb')
    await wrapper.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('create-fail')

    await clickText(wrapper, '打开')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('open-fail')

    await clickText(wrapper, '关闭')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('close-fail')

    await clickText(wrapper, '删除')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('rm-fail')
  })

  it('opens / closes / deletes a ready database and pages the table', async () => {
    const { wrapper } = await mountWithApp(Databases, { stubs: dbStubs })
    await clickText(wrapper, '打开')
    await flushPromises()
    expect(api.databases.open).toHaveBeenCalledWith(expect.any(String), 'db-1')
    expect(toast.success).toHaveBeenCalledWith('demo 已打开')

    await clickText(wrapper, '关闭')
    await flushPromises()
    expect(api.databases.close).toHaveBeenCalledWith(expect.any(String), 'db-1')

    await clickText(wrapper, '删除')
    await flushPromises()
    expect(api.databases.remove).toHaveBeenCalledWith(expect.any(String), 'db-1')

    await wrapper.get('.pager-next').trigger('click')
    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(api.databases.list.mock.calls.length).toBeGreaterThan(1)
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
    expect(wrapper.get('.coll-panel').attributes('data-readonly')).toBe('1')
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

    const { useProjectStore } = await import('../stores/project')
    useProjectStore(pinia).setProject('00000000-0000-0000-0000-000000000003')
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
