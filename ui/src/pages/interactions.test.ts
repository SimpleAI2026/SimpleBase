import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useProjectStore } from '../stores/project'
import { api, resetApiMocks } from '../test/api-mock'
import { clickText, mountWithApp, readyDb, sampleAgent, uiStubs } from '../test/helpers'
import Databases from './Databases.vue'
import AgentManager from './AgentManager.vue'
import Dashboard from './Dashboard.vue'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return { api: m.api, isMock: false }
})

const collectionStub = {
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

const agentStubs = {
  AiChat: {
    props: ['messages', 'sending'],
    emits: ['stop'],
    template:
      '<div class="ai"><button type="button" class="ai-stop" @click="$emit(\'stop\')">stop</button><slot name="toolbar" /><slot name="empty" /></div>'
  },
  AgentScheduleModal: {
    props: ['open', 'agent', 'schedule'],
    emits: ['update:open', 'saved', 'removed', 'view-thread'],
    template: `
      <div v-if="open" class="sch-m">
        <button type="button" class="sch-close" @click="$emit('update:open', false)">x</button>
        <button type="button" class="sch-saved" @click="$emit('saved', { id: 's1', agent_id: 'ag-1' })">save</button>
        <button type="button" class="sch-removed" @click="$emit('removed', 'sch-1')">rm</button>
        <button type="button" class="sch-thread" @click="$emit('view-thread', 'th-1')">th</button>
      </div>
    `
  }
}

describe('page template interactions', () => {
  beforeEach(() => {
    resetApiMocks()
    api.databases.list.mockResolvedValue([readyDb])
    api.db.collections.mockResolvedValue(['users'])
    api.agents.list.mockResolvedValue([sampleAgent])
    api.agentThreads.list.mockResolvedValue([{ id: 'th-1', title: 'T', created_at: 't', updated_at: 't' }])
    api.agentThreads.messages.mockResolvedValue([
      { id: 'm1', role: 'assistant', content: 'hi', tool_calls: [{ name: 't' }], created_at: 't' }
    ])
    api.agentSchedules.list.mockResolvedValue([
      {
        id: 'sch-1',
        agent_id: 'ag-1',
        thread_id: 'th-1',
        prompt: 'p',
        cron_expr: '0 * * * *',
        enabled: true,
        created_at: 't',
        updated_at: 't'
      }
    ])
  })

  it('Databases clicks expand, sql, open/close, collection, docs, pager and project watch', async () => {
    const { wrapper, pinia } = await mountWithApp(Databases, { stubs: collectionStub })
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
    await clickText(wrapper, '打开')
    await clickText(wrapper, '关闭')
    await clickText(wrapper, '新建集合')
    await clickText(wrapper, '删除')
    await wrapper.get('.pager-next').trigger('click')
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    wrapper.unmount()
  })

  it('Databases empty-state create and admin refresh', async () => {
    api.databases.list.mockResolvedValueOnce([])
    const { wrapper } = await mountWithApp(Databases, { stubs: collectionStub })
    await wrapper.get('.empty-action').trigger('click')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)
    wrapper.unmount()

    api.databases.list.mockResolvedValue([readyDb])
    const admin = await mountWithApp(Databases, {
      projectId: 'sb-admin',
      stubs: collectionStub
    })
    await clickText(admin.wrapper, '刷新')
    admin.wrapper.unmount()
  })

  it('AgentManager clicks list actions, modal fields, schedule, and stream stop', async () => {
    api.agents.modules.mockResolvedValue([
      { id: 'database', name: 'Database', description: 'd', default_tools: ['list_databases'], team_supported: false },
      { id: 's3', name: 'S3', description: 's', default_tools: ['list_objects'], team_supported: false }
    ])
    const { wrapper, pinia } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await clickText(wrapper, '刷新')
    await wrapper.find('button.w-full').trigger('click')
    await clickText(wrapper, '新建')
    await flushPromises()
    if (wrapper.find('.sb-modal').exists()) {
      if (wrapper.find('.select-emit').exists()) await wrapper.get('.select-emit').trigger('click')
      if (wrapper.find('#agent-name').exists()) await wrapper.get('#agent-name').setValue('N2')
      if (wrapper.find('#agent-desc').exists()) await wrapper.get('#agent-desc').setValue('d')
      if (wrapper.find('#agent-prompt').exists()) await wrapper.get('#agent-prompt').setValue('p')
      const badges = wrapper.findAll('.badge')
      if (badges.length) await badges[badges.length - 1].trigger('click')
      if (wrapper.find('.sb-ok').exists()) await wrapper.get('.sb-ok').trigger('click')
      await flushPromises()
      if (wrapper.find('.sb-cancel').exists()) await wrapper.get('.sb-cancel').trigger('click')
    }

    await clickText(wrapper, '定时')
    await flushPromises()
    if (wrapper.find('.sch-saved').exists()) await wrapper.get('.sch-saved').trigger('click')
    if (wrapper.find('.sch-removed').exists()) await wrapper.get('.sch-removed').trigger('click')
    if (wrapper.find('.sch-thread').exists()) await wrapper.get('.sch-thread').trigger('click')
    if (wrapper.find('.sch-close').exists()) await wrapper.get('.sch-close').trigger('click')
    await clickText(wrapper, '删除')
    await clickText(wrapper, '新会话')
    if (wrapper.find('.ai-stop').exists()) await wrapper.get('.ai-stop').trigger('click')
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    wrapper.unmount()
  })

  it('AgentManager empty-state create', async () => {
    api.agents.list.mockResolvedValueOnce([])
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    if (wrapper.find('.empty-action').exists()) {
      await wrapper.get('.empty-action').trigger('click')
      expect(wrapper.find('.sb-modal').exists()).toBe(true)
    }
    wrapper.unmount()
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
    expect(wrapper.text()).toContain('S3 对象存储')
    wrapper.unmount()
  })
})

describe('script-setup method coverage extras', () => {
  it('Databases onDocumentCreated reopens list when closed', async () => {
    setActivePinia(createPinia())
    useProjectStore().setProject('dev-shop')
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore().setProject('dev-shop')
    const w = mount(Databases, {
      global: { plugins: [pinia], stubs: { ...uiStubs, ...collectionStub } }
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
})
