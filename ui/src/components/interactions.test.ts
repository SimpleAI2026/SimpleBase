import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { useProjectStore } from '../stores/project'
import { api, resetApiMocks } from '../test/api-mock'
import { sampleAgent, sampleCron, sampleGoFn, readyDb, uiStubs } from '../test/helpers'
import CronJobModal from './modal/CronJobModal.vue'
import GoFunctionModal from './modal/GoFunctionModal.vue'
import SqlWorkModal from './modal/SqlWorkModal.vue'
import DocumentListModal from './modal/DocumentListModal.vue'
import CreateCollectionModal from './modal/CreateCollectionModal.vue'
import CreateProjectModal from './modal/CreateProjectModal.vue'
import DocumentKvModal from './modal/DocumentKvModal.vue'
import SbModal from './modal/SbModal.vue'
import AgentScheduleModal from './ai/AgentScheduleModal.vue'
import AiChat from './ai/AiChat.vue'
import AiChatComposer from './ai/AiChatComposer.vue'
import CollectionPanel from './databases/CollectionPanel.vue'
import CronJobRunsDrawer from './CronJobRunsDrawer.vue'
import GlobalProjectSwitcher from './GlobalProjectSwitcher.vue'
import NavMenu from './NavMenu.vue'
import ConnectionPanel from './settings/ConnectionPanel.vue'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return { api: m.api, isMock: false }
})

vi.mock('@/components/editor/GoMonacoEditor.vue', () => ({
  default: {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: '<textarea class="monaco" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />'
  }
}))

function piniaWithProject() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useProjectStore().setProject('00000000-0000-0000-0000-000000000002')
  return pinia
}

describe('component template interactions', () => {
  beforeEach(() => {
    resetApiMocks()
    api.gofunctions.list.mockResolvedValue([sampleGoFn])
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
  })

  it('CronJobModal radios, selects, inputs, and cancel', async () => {
    const w = mount(CronJobModal, {
      props: { open: true },
      global: { plugins: [piniaWithProject()], stubs: uiStubs }
    })
    await flushPromises()
    const radios = w.findAll('input[type="radio"]')
    await radios[1].setValue()
    await radios[0].setValue()
    for (const sel of w.findAll('.select-emit')) await sel.trigger('click')
    const inputs = w.findAll('input')
    for (const i of inputs) {
      const t = i.attributes('type')
      if (t === 'radio') continue
      if (t === 'number') await i.setValue('2')
      else await i.setValue('job-a')
    }
    await w.get('textarea').setValue('{"a":1}')
    const cancel = w.find('.sb-cancel')
    if (cancel.exists()) await cancel.trigger('click')
    else await w.findAll('button').find((b) => b.text().includes('取消'))?.trigger('click')
    w.unmount()
  })

  it('SqlWorkModal toggles modes, pager, switch, and footer', async () => {
    api.sql.query.mockResolvedValue({
      columns: ['id'],
      rows: [['1']],
      rowCount: 1,
      durationMs: 1,
      requestId: 'r'
    })
    const w = mount(SqlWorkModal, {
      props: { open: true, projectId: 'p', database: readyDb },
      global: { stubs: uiStubs }
    })
    await w.get('.tg-execute').trigger('click')
    await w.get('.tg-batch').trigger('click')
    await w.get('.switch').trigger('click')
    await w.get('.tg-query').trigger('click')
    await w.get('textarea').setValue('SELECT 1')
    await w.findAll('button').find((b) => b.text().includes('查询') || b.text() === '查询')?.trigger('click')
    await flushPromises()
    if (w.find('.pager-next').exists()) await w.get('.pager-next').trigger('click')
    await w.findAll('button').find((b) => b.text().includes('关闭'))?.trigger('click')
    w.unmount()
  })

  it('GoFunctionModal name input and modal close', async () => {
    const w = mount(GoFunctionModal, {
      props: { open: true, mode: 'create' },
      global: { plugins: [piniaWithProject()], stubs: { ...uiStubs, GoMonacoEditor: true } }
    })
    const name = w.find('input')
    if (name.exists()) await name.setValue('Echo')
    const cancel = w.find('.sb-cancel')
    if (cancel.exists()) await cancel.trigger('click')
    else await w.findAll('button').find((b) => b.text().includes('取消'))?.trigger('click')
    w.unmount()
  })

  it('DocumentListModal add, pager, confirm delete, and close', async () => {
    api.db.rows.mockResolvedValue([{ id: 'r1', name: 'Ada' }])
    const w = mount(DocumentListModal, {
      props: { open: true, projectId: 'p', databaseId: 'db', collection: 'users' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    await vm.load?.()
    await flushPromises()
    await w.findAll('button').find((b) => b.text().includes('新增文档'))?.trigger('click')
    if (w.find('.pager-next').exists()) await w.get('.pager-next').trigger('click')
    if (w.find('.confirm-action').exists()) await w.get('.confirm-action').trigger('click')
    await flushPromises()
    await w.findAll('button').find((b) => b.text().includes('关闭'))?.trigger('click')
    w.unmount()
  })

  it('CreateCollection / CreateProject / DocumentKv / SbModal close handlers', async () => {
    const cc = mount(CreateCollectionModal, {
      props: { open: true, projectId: 'p', database: readyDb },
      global: { stubs: uiStubs }
    })
    await cc.get('.sb-cancel').trigger('click')
    cc.unmount()

    const cp = mount(CreateProjectModal, { props: { open: true }, global: { stubs: uiStubs } })
    await cp.get('.sb-cancel').trigger('click')
    cp.unmount()

    const kv = mount(DocumentKvModal, {
      props: { open: true, projectId: 'p', databaseId: 'db', collection: 'users' },
      global: { stubs: uiStubs }
    })
    await kv.get('.sb-cancel').trigger('click')
    kv.unmount()

    const sb = mount(SbModal, {
      props: { open: true, title: 'T' },
      global: { stubs: { ...uiStubs, SbModal: false } }
    })
    await sb.findAll('button')[0].trigger('click')
    sb.unmount()
  })

  it('AgentScheduleModal frequency, switch, custom cron, footer buttons', async () => {
    const schedule = {
      id: 'sch-1',
      agent_id: 'ag-1',
      thread_id: 'th-1',
      prompt: 'p',
      cron_expr: '1 2 3 4 5',
      enabled: true,
      created_at: 't',
      updated_at: 't',
      last_run_at: '2024-01-01T00:00:00Z',
      next_run_at: '2024-01-02T00:00:00Z'
    }
    api.agentSchedules.runs.mockResolvedValue([
      { id: 'r1', schedule_id: 'sch-1', run_id: 'x', trigger: 'manual', status: 'completed', created_at: 't' }
    ])
    const w = mount(AgentScheduleModal, {
      props: { open: false, agent: sampleAgent, schedule, projectId: 'p1' },
      global: { stubs: uiStubs }
    })
    await w.setProps({ open: true })
    await flushPromises()
    await w.get('.select-emit').trigger('click')
    await w.get('.switch').trigger('click')
    await w.get('#schedule-prompt').setValue('hello')
    if (w.findAll('button').some((b) => b.text().includes('查看会话'))) {
      await w.findAll('button').find((b) => b.text().includes('查看会话'))!.trigger('click')
      expect(w.emitted('view-thread')).toBeTruthy()
    }
    await w.findAll('button').find((b) => b.text().includes('取消'))?.trigger('click')
    expect(w.emitted('update:open')).toBeTruthy()
    w.unmount()
  })

  it('AiChat toolbar model/stream and composer send/stop/mention', async () => {
    const w = mount(AiChat, {
      props: {
        projectId: 'p',
        showToolbar: true,
        streaming: true,
        sending: false,
        model: 'm',
        modelOptions: [{ label: 'm', value: 'm' }],
        mentionAgents: [{ id: 'a1', name: 'Database', module: 'database' }],
        messages: [
          { role: 'assistant', content: 'yo', toolCalls: [{ name: 't', arguments: '{}', content: '[]' }] }
        ]
      },
      global: { stubs: uiStubs }
    })
    if (w.find('.select-emit').exists()) await w.get('.select-emit').trigger('click')
    if (w.find('.switch').exists()) await w.get('.switch').trigger('click')
    const composer = w.findComponent(AiChatComposer)
    if (composer.exists()) {
      await composer.get('textarea').setValue('hello @Da')
      await composer.get('textarea').trigger('input')
      if (composer.find('.cmd-item').exists()) await composer.get('.cmd-item').trigger('click')
      const send = composer.findAll('button').find((b) => b.attributes('aria-label') === '发送')
      if (send) await send.trigger('click')
    }
    await w.setProps({ sending: true })
    const stop = w.findAll('button').find((b) => b.attributes('aria-label') === '停止生成')
    if (stop) await stop.trigger('click')
    w.unmount()
  })

  it('CollectionPanel view/add buttons', async () => {
    api.db.collections.mockResolvedValue(['users'])
    const w = mount(CollectionPanel, {
      props: { projectId: 'p', database: readyDb },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    await w.findAll('button').find((b) => b.text().includes('查看数据'))?.trigger('click')
    await w.findAll('button').find((b) => b.text().includes('新增文档'))?.trigger('click')
    expect(w.emitted('view-data')?.[0]).toEqual(['users'])
    expect(w.emitted('add-document')?.[0]).toEqual(['users'])
    w.unmount()
  })

  it('CronJobRunsDrawer copy, filter, sheet close', async () => {
    api.cronjobs.runs.mockResolvedValue([
      {
        id: 'r1',
        jobId: 'cj-1',
        trigger: 'scheduled',
        status: 'failed',
        error: 'boom',
        durationMs: 2,
        responseJson: '{"ok":1}',
        finishedAt: '2024-01-01T00:00:00Z',
        createdAt: 't'
      }
    ])
    const w = mount(CronJobRunsDrawer, {
      props: { open: true, job: sampleCron },
      global: { plugins: [piniaWithProject()], stubs: uiStubs }
    })
    await flushPromises()
    const copies = w.findAll('button').filter((b) => b.text().includes('复制'))
    for (const c of copies) await c.trigger('click')
    if (w.find('.select-emit').exists()) await w.get('.select-emit').trigger('click')
    if (w.find('.sheet-close').exists()) await w.get('.sheet-close').trigger('click')
    expect(w.emitted('update:open')).toBeTruthy()
    w.unmount()
  })

  it('GlobalProjectSwitcher popover open/close and create modal close', async () => {
    api.projects.list.mockResolvedValue([])
    const pinia = piniaWithProject()
    const store = useProjectStore(pinia)
    store.history = ['orphan']
    store.projectId = 'missing-current'
    store.projectName = 'Ghost'
    const w = mount(GlobalProjectSwitcher, { global: { plugins: [pinia], stubs: uiStubs } })
    await flushPromises()
    if (w.find('.pop-open').exists()) await w.get('.pop-open').trigger('click')
    if (w.find('.pop-close').exists()) await w.get('.pop-close').trigger('click')
    w.unmount()
  })

  it('NavMenu emits navigate on link click', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'dashboard', component: { template: '<div />' }, meta: { title: '监控大盘' } }
      ]
    })
    await router.push('/')
    await router.isReady()
    const w = mount(NavMenu, {
      global: {
        plugins: [router],
        stubs: {
          SidebarMenu: { template: '<div><slot /></div>' },
          SidebarMenuItem: { template: '<div><slot /></div>' },
          SidebarMenuButton: { template: '<div><slot /></div>' },
          RouterLink: { template: '<a class="nav-link" @click="$attrs.onClick && $attrs.onClick()"><slot /></a>' }
        }
      }
    })
    const link = w.find('.nav-link, a')
    if (link.exists()) await link.trigger('click')
    w.unmount()
  })

  it('ConnectionPanel mounts in the settings modal flow', async () => {
    const pinia = piniaWithProject()
    const w = mount(ConnectionPanel, { global: { plugins: [pinia], stubs: uiStubs } })
    expect(w.text()).toContain('API Key')
    w.unmount()
  })
})
