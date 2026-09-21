import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useProjectStore } from './stores/project'
import { api, resetApiMocks } from './test/api-mock'
import {
  closedDb,
  mountWithApp,
  readyDb,
  sampleAgent,
  sampleCron,
  sampleGoFn,
  uiStubs
} from './test/helpers'
import AgentManager from './pages/AgentManager.vue'
import CronJobs from './pages/CronJobs.vue'
import Databases from './pages/Databases.vue'
import GoFunctions from './pages/GoFunctions.vue'
import Logs from './pages/Logs.vue'
import S3Manager from './pages/S3Manager.vue'
import Settings from './pages/Settings.vue'
import CronJobModal from './components/modal/CronJobModal.vue'
import GoFunctionModal from './components/modal/GoFunctionModal.vue'
import SqlWorkModal from './components/modal/SqlWorkModal.vue'
import DocumentListModal from './components/modal/DocumentListModal.vue'
import SbModal from './components/modal/SbModal.vue'
import AgentScheduleModal from './components/ai/AgentScheduleModal.vue'
import CronJobRunsDrawer from './components/CronJobRunsDrawer.vue'
import ApiKeyDrawer from './components/ApiKeyDrawer.vue'
import GlobalProjectSwitcher from './components/GlobalProjectSwitcher.vue'

vi.mock('./services/api', async () => {
  const m = await import('./test/api-mock')
  return { api: m.api, isMock: false }
})

vi.mock('@/components/editor/GoMonacoEditor.vue', () => ({
  default: {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template:
      '<textarea class="monaco" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />'
  }
}))

function vmOf(wrapper: { vm: unknown }) {
  return wrapper.vm as Record<string, any>
}

describe('remaining coverage gaps', () => {
  beforeEach(() => {
    resetApiMocks()
    api.databases.list.mockResolvedValue([readyDb, closedDb])
    api.db.collections.mockResolvedValue(['users'])
    api.agents.list.mockResolvedValue([sampleAgent])
    api.agents.modules.mockResolvedValue([
      { id: 'database', name: 'Database', description: 'd', default_tools: ['list_databases'], team_supported: false }
    ])
    api.agentThreads.list.mockResolvedValue([])
    api.agentThreads.messages.mockResolvedValue([])
    api.agentSchedules.list.mockResolvedValue([])
    api.cronjobs.list.mockResolvedValue([sampleCron])
    api.gofunctions.list.mockResolvedValue([sampleGoFn])
    api.logs.list.mockResolvedValue([])
    api.logs.getRetention.mockResolvedValue({ keepDays: 14, updatedAt: '', scope: 'project' })
  })

  it('Databases methods cover not-ready, missing active db, and non-Error catches', async () => {
    const { wrapper } = await mountWithApp(Databases)
    const vm = vmOf(wrapper)
    vm.toggleExpand({ ...closedDb })
    vm.toggleExpand(readyDb)
    vm.toggleExpand(readyDb)
    vm.activeDb = null
    vm.onCollectionCreated()
    vm.activeCollection = ''
    vm.onAddDocumentFromList()

    api.databases.create.mockRejectedValueOnce('create-fail')
    vm.newName = 'okdb'
    await vm.create()
    api.databases.open.mockRejectedValueOnce('open-fail')
    await vm.openDb(readyDb)
    api.databases.close.mockRejectedValueOnce('close-fail')
    await vm.closeDb(readyDb)
    api.databases.remove.mockRejectedValueOnce('rm-fail')
    await vm.removeDb(readyDb)
    vm.createVisible = true
    if (wrapper.find('.sb-cancel').exists()) await wrapper.get('.sb-cancel').trigger('click')
    wrapper.unmount()
  })

  it('AgentManager clicks edit/schedule/delete and non-Error catches', async () => {
    api.agentThreads.list.mockResolvedValue([{ id: 'th-1', title: 'T', created_at: 't', updated_at: 't' }])
    const { wrapper } = await mountWithApp(AgentManager)
    const edit = wrapper.findAll('button').find((b) => b.text() === '编辑')
    const sched = wrapper.findAll('button').find((b) => b.text() === '定时')
    if (edit) await edit.trigger('click')
    if (wrapper.find('.sb-cancel').exists()) await wrapper.get('.sb-cancel').trigger('click')
    if (sched) await sched.trigger('click')
    if (wrapper.find('.sb-cancel').exists()) await wrapper.get('.sb-cancel').trigger('click')
    const confirm = wrapper.find('.confirm-action')
    if (confirm.exists()) await confirm.trigger('click')
    await flushPromises()

    const vm = vmOf(wrapper)
    api.agentSchedules.list.mockRejectedValueOnce('sched')
    await vm.loadSchedules()
    api.agents.modules.mockRejectedValueOnce('mods')
    await vm.loadModules()
    api.agents.list.mockRejectedValueOnce('agents')
    await vm.loadAgents()
    api.agentThreads.list.mockRejectedValueOnce('threads')
    await vm.ensureThread()
    api.agentThreads.messages.mockRejectedValueOnce('msgs')
    await vm.onViewScheduleThread('th-x')
    api.agents.patch.mockRejectedValueOnce('save')
    vm.editing = sampleAgent
    vm.form = { ...sampleAgent, name: 'n' }
    await vm.saveAgent()
    api.agents.remove.mockRejectedValueOnce('rm')
    await vm.removeAgent(sampleAgent)
    api.agentThreads.create.mockRejectedValueOnce('th')
    await vm.resetThread()
    vm.openCreate()
    vm.onModuleChange('database')
    vm.onModuleChange('missing')
    wrapper.unmount()
  })

  it('CronJobs / S3 / Logs / Settings / GoFunctions non-Error paths', async () => {
    const cron = await mountWithApp(CronJobs)
    const cvm = vmOf(cron.wrapper)
    expect(cvm.scheduleText({ scheduleKind: 'interval', intervalSeconds: undefined })).toContain('每')
    api.cronjobs.update.mockRejectedValueOnce('tog')
    await cvm.toggleEnabled(sampleCron)
    api.cronjobs.trigger.mockRejectedValueOnce('trig')
    await cvm.trigger(sampleCron)
    api.cronjobs.remove.mockRejectedValueOnce('rm')
    await cvm.remove(sampleCron)
    cvm.openCreate()
    cvm.modalOpen = false
    cron.wrapper.unmount()

    const s3 = await mountWithApp(S3Manager)
    const svm = vmOf(s3.wrapper)
    if (s3.wrapper.find('.ig-input').exists()) {
      await s3.wrapper.get('.ig-input').setValue('images/')
    }
    api.s3.list.mockRejectedValueOnce('list')
    await svm.load()
    api.s3.remove.mockRejectedValueOnce('rm')
    await svm.remove('k')
    api.s3.presign.mockRejectedValueOnce('url')
    await svm.open('k')
    s3.wrapper.unmount()

    const logs = await mountWithApp(Logs)
    const lvm = vmOf(logs.wrapper)
    if (logs.wrapper.find('.select-empty').exists()) await logs.wrapper.get('.select-empty').trigger('click')
    lvm.keepDaysText = 'nope'
    expect(lvm.keepDays).toBe(14)
    api.logs.list.mockRejectedValueOnce('load-logs')
    await lvm.load()
    logs.wrapper.unmount()

    const settings = await mountWithApp(Settings)
    const st = vmOf(settings.wrapper)
    api.llmSettings.put.mockRejectedValueOnce({ nope: true })
    await st.patchDefaults({ maxTokens: 1 })
    api.llmSettings.put.mockRejectedValueOnce({ nope: true })
    await st.setDefault('openai')
    if (settings.wrapper.find('#max-tokens').exists()) {
      await settings.wrapper.get('#max-tokens').setValue('')
      await flushPromises()
    }
    if (settings.wrapper.find('.sheet-content-close').exists()) {
      await settings.wrapper.get('.sheet-content-close').trigger('click')
    }
    settings.wrapper.unmount()

    const go = await mountWithApp(GoFunctions)
    const gvm = vmOf(go.wrapper)
    api.gofunctions.remove.mockRejectedValueOnce('rm-fn')
    await gvm.remove?.(sampleGoFn).catch(() => undefined)
    go.wrapper.unmount()
  })

  it('CronJobModal interval unit selects, json error, and save catches', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore().setProject('00000000-0000-0000-0000-000000000002')
    const w = mount(CronJobModal, {
      props: { open: true },
      global: { plugins: [pinia], stubs: uiStubs }
    })
    await flushPromises()
    const radios = w.findAll('input[type="radio"]')
    await radios[1].setValue()
    const nums = w.findAll('input[type="number"]')
    if (nums[0]) await nums[0].setValue('2')
    if (w.find('.select-hour').exists()) await w.get('.select-hour').trigger('click')
    if (w.find('.select-day').exists()) await w.get('.select-day').trigger('click')
    const vm = vmOf(w)
    vm.form.inputJson = '{bad'
    expect(vm.inputJsonError).toBeTruthy()
    vm.form.inputJson = ''
    vm.form.name = 'JobA'
    vm.form.funcFile = 'hello'
    vm.form.funcExport = 'Hello'
    vm.form.scheduleKind = 'cron'
    vm.form.cronExpr = '* * * * *'
    api.cronjobs.create.mockRejectedValueOnce('save-fail')
    await vm.save()
    await w.setProps({
      open: true,
      target: { ...sampleCron, scheduleKind: 'interval', intervalSeconds: 7200 }
    })
    await flushPromises()
    api.cronjobs.update.mockRejectedValueOnce(new Error('upd'))
    vm.form.name = sampleCron.name
    vm.form.funcFile = 'hello'
    vm.form.funcExport = 'Hello'
    vm.form.scheduleKind = 'interval'
    await vm.save()
    w.unmount()
  })

  it('SqlWorkModal inputs, pager, and empty query result', async () => {
    api.sql.query.mockResolvedValue({ columns: [], rows: [], rowCount: 0, durationMs: 1, requestId: 'r' })
    const w = mount(SqlWorkModal, {
      props: { open: true, projectId: 'p', database: readyDb },
      global: { stubs: uiStubs }
    })
    await w.get('.tg-execute').trigger('click')
    for (const i of w.findAll('input')) {
      if (i.attributes('type') === 'number') await i.setValue('10')
      else await i.setValue('["1"]')
    }
    await w.get('.tg-batch').trigger('click')
    await w.get('.tg-query').trigger('click')
    for (const i of w.findAll('input')) {
      if (i.attributes('type') === 'number') await i.setValue('5')
      else await i.setValue('[]')
    }
    await w.get('textarea').setValue('SELECT 1')
    api.sql.query.mockResolvedValueOnce({
      columns: ['id'],
      rows: [['1'], ['2']],
      rowCount: 2,
      durationMs: 1,
      requestId: 'r'
    })
    await vmOf(w).run()
    await flushPromises()
    if (w.find('.pager-next').exists()) await w.get('.pager-next').trigger('click')
    w.unmount()
  })

  it('GoFunctionModal monaco, close emit, and save catch', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore().setProject('00000000-0000-0000-0000-000000000002')
    const w = mount(GoFunctionModal, {
      props: { open: true, mode: 'create' },
      global: { plugins: [pinia], stubs: uiStubs }
    })
    if (w.find('.monaco').exists()) {
      await w.get('.monaco').setValue('func Ping() int { return 1 }')
    }
    const vm = vmOf(w)
    api.gofunctions.create.mockRejectedValueOnce('save')
    vm.nameValue = 'Ping'
    await vm.save()
    if (w.find('.sb-cancel').exists()) await w.get('.sb-cancel').trigger('click')
    else await w.findAll('button').find((b) => b.text().includes('取消') || b.text().includes('关闭'))?.trigger('click')
    w.unmount()
  })

  it('AgentScheduleModal custom cron, 409 retry, and catch fallbacks', async () => {
    const w = mount(AgentScheduleModal, {
      props: { open: true, agent: sampleAgent, schedule: null, projectId: 'p1' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    if (w.find('.select-custom').exists()) await w.get('.select-custom').trigger('click')
    if (w.find('input[placeholder="*/15 * * * *"]').exists()) {
      await w.get('input[placeholder="*/15 * * * *"]').setValue('1 2 3 4 5')
      await w.get('input[placeholder="*/15 * * * *"]').trigger('blur')
    }
    const vm = vmOf(w)
    vm.form.prompt = 'ping'
    vm.form.cron_expr = '* * * * *'
    const err409 = Object.assign(new Error('exists'), { status: 409 })
    api.agentSchedules.create.mockRejectedValueOnce(err409)
    await vm.save().catch(() => undefined)
    api.agentSchedules.create.mockRejectedValueOnce(new Error('already exists'))
    await vm.save()
    api.agentSchedules.create.mockRejectedValueOnce('plain')
    await vm.save()
    w.unmount()

    const schedule = {
      id: 'sch-1',
      agent_id: 'ag-1',
      thread_id: 'th-1',
      prompt: 'p',
      cron_expr: '* * * * *',
      enabled: true,
      created_at: 't',
      updated_at: 't'
    }
    const w2 = mount(AgentScheduleModal, {
      props: { open: true, agent: sampleAgent, schedule, projectId: 'p1' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm2 = vmOf(w2)
    api.agentSchedules.remove.mockRejectedValueOnce('rm')
    await vm2.remove()
    api.agentSchedules.trigger.mockRejectedValueOnce('trig')
    await vm2.triggerNow()
    if (w2.find('.sb-cancel').exists()) await w2.get('.sb-cancel').trigger('click')
    w2.unmount()
  })

  it('CronJobRunsDrawer copy clicks, load/trigger fallbacks, empty job', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore().setProject('00000000-0000-0000-0000-000000000002')
    api.cronjobs.runs.mockResolvedValue([
      {
        id: 'r1',
        jobId: 'cj-1',
        trigger: 'scheduled',
        status: 'failed',
        error: 'boom',
        durationMs: 2,
        responseJson: 'not-json',
        finishedAt: '2024-01-01T00:00:00Z',
        createdAt: 't'
      }
    ])
    const w = mount(CronJobRunsDrawer, {
      props: { open: true, job: sampleCron },
      global: { plugins: [pinia], stubs: uiStubs }
    })
    await flushPromises()
    for (const b of w.findAll('button')) {
      if (b.text().includes('复制')) await b.trigger('click')
    }
    const vm = vmOf(w)
    api.cronjobs.runs.mockRejectedValueOnce('load')
    await vm.load()
    api.cronjobs.trigger.mockRejectedValueOnce('trig')
    await vm.trigger()
    w.unmount()

    const empty = mount(CronJobRunsDrawer, {
      props: { open: true },
      global: { plugins: [pinia], stubs: uiStubs }
    })
    await flushPromises()
    empty.unmount()
  })

  it('DocumentListModal / SbModal / ApiKey / Switcher close handlers', async () => {
    api.db.rows.mockResolvedValue([{ id: 'r1', name: 'Ada' }])
    const dl = mount(DocumentListModal, {
      props: { open: true, projectId: 'p', databaseId: 'db', collection: 'users' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    if (dl.find('.confirm-action').exists()) await dl.get('.confirm-action').trigger('click')
    if (dl.find('.sb-cancel').exists()) await dl.get('.sb-cancel').trigger('click')
    dl.unmount()

    const sb = mount(SbModal, {
      props: { open: true, title: 'T' },
      global: { stubs: { ...uiStubs, SbModal: false } }
    })
    await sb.findAll('button')[0].trigger('click')
    sb.unmount()

    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore().setProject('00000000-0000-0000-0000-000000000002')
    const key = mount(ApiKeyDrawer, { global: { plugins: [pinia], stubs: uiStubs } })
    const keyInput = key.find('input')
    if (keyInput.exists()) await keyInput.setValue('k')
    key.unmount()

    const sw = mount(GlobalProjectSwitcher, { global: { plugins: [pinia], stubs: uiStubs } })
    const store = useProjectStore(pinia)
    store.openCreateModal()
    await flushPromises()
    if (sw.find('.sb-cancel').exists()) await sw.get('.sb-cancel').trigger('click')
    sw.unmount()
  })
})
