import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useProjectStore } from '../stores/project'
import Logs from './Logs.vue'
import GoFunctions from './GoFunctions.vue'
import CronJobs from './CronJobs.vue'
import S3Manager from './S3Manager.vue'
import Databases from './Databases.vue'
import AgentManager from './AgentManager.vue'
import SettingsPanel from '../components/settings/SettingsPanel.vue'
import { uiStubs } from '../test/helpers'

const { api } = vi.hoisted(() => {
  const fn = () => vi.fn()
  return {
    api: {
      gofunctions: { list: fn(), create: fn(), get: fn(), update: fn(), remove: fn() },
      cronjobs: { list: fn(), create: fn(), get: fn(), update: fn(), remove: fn(), runs: fn(), trigger: fn() },
      projects: { list: fn(), create: fn() },
      metrics: { summary: fn(), trend: fn() },
      databases: { list: fn(), create: fn(), get: fn(), open: fn(), close: fn(), remove: fn() },
      sql: { query: fn(), execute: fn(), batch: fn() },
      db: { collections: fn(), createCollection: fn(), rows: fn(), insert: fn(), update: fn(), remove: fn() },
      s3: { list: fn(), presign: fn(), remove: fn(), upload: fn() },
      logs: { list: fn(), getRetention: fn(), putRetention: fn() },
      quota: { status: fn() },
      llm: { providers: fn(), chat: fn(), stream: fn() },
      llmSettings: { get: fn(), put: fn() },
      agents: { modules: fn(), list: fn(), create: fn(), get: fn(), patch: fn(), remove: fn() },
      agentThreads: {
        list: fn(),
        create: fn(),
        get: fn(),
        remove: fn(),
        messages: fn(),
        run: fn(),
        streamRun: fn(),
        cancel: fn()
      },
      agentSchedules: { list: fn(), create: fn(), patch: fn(), remove: fn(), runs: fn(), trigger: fn() }
    }
  }
})

vi.mock('../services/api', () => ({ api, isMock: true }))
vi.mock('axios', () => ({
  default: {
    create: () => ({
      interceptors: { request: { use: vi.fn() }, response: { use: vi.fn() } },
      get: vi.fn(),
      post: vi.fn(),
      put: vi.fn(),
      patch: vi.fn(),
      delete: vi.fn()
    }),
    post: vi.fn()
  }
}))

const uiStubs = {
  ProjectScope: { template: '<div><slot /></div>' },
  PageContainer: { template: '<div><slot /></div>' },
  Card: { template: '<div><slot /></div>' },
  CardHeader: { template: '<div><slot /></div>' },
  CardTitle: { template: '<div><slot /></div>' },
  CardDescription: { template: '<div><slot /></div>' },
  CardContent: { template: '<div><slot /></div>' },
  CardAction: { template: '<div><slot /></div>' },
  Table: { template: '<table><slot /></table>' },
  TableHeader: { template: '<thead><slot /></thead>' },
  TableBody: { template: '<tbody><slot /></tbody>' },
  TableRow: { template: '<tr><slot /></tr>' },
  TableHead: { template: '<th><slot /></th>' },
  TableCell: { template: '<td><slot /></td>' },
  TableEmpty: { template: '<tr><slot /></tr>' },
  Badge: { template: '<span @click="$emit(\'click\')"><slot /></span>' },
  Button: { template: '<button @click="$emit(\'click\')"><slot /></button>' },
  Input: { template: '<input />' },
  InputGroup: { template: '<div><slot /></div>' },
  InputGroupAddon: { template: '<div />' },
  InputGroupInput: { template: '<input />' },
  Select: { template: '<div><slot /></div>' },
  SelectTrigger: { template: '<div />' },
  SelectValue: { template: '<div />' },
  SelectContent: { template: '<div><slot /></div>' },
  SelectGroup: { template: '<div><slot /></div>' },
  SelectItem: { template: '<div><slot /></div>' },
  Switch: { template: '<button />' },
  Progress: { template: '<div />' },
  Spinner: { template: '<div />' },
  Skeleton: { template: '<div />' },
  Alert: { template: '<div><slot /></div>' },
  AlertTitle: { template: '<div><slot /></div>' },
  AlertDescription: { template: '<div><slot /></div>' },
  Tooltip: { template: '<div><slot /></div>' },
  TooltipProvider: { template: '<div><slot /></div>' },
  TooltipTrigger: { template: '<div><slot /></div>' },
  TooltipContent: { template: '<div><slot /></div>' },
  ConfirmAction: { template: '<div @click="$emit(\'confirm\')"><slot /></div>' },
  SbEmptyState: { template: '<button @click="$emit(\'action\')">empty</button>' },
  TablePager: { template: '<div />' },
  SbModal: { template: '<div><slot /></div>' },
  GoFunctionModal: { template: '<div />' },
  CronJobModal: { template: '<div />' },
  CronJobRunsDrawer: { template: '<div />' },
  CreateProjectModal: { template: '<div />' },
  CreateCollectionModal: { template: '<div />' },
  DocumentListModal: { template: '<div />' },
  DocumentKvModal: { template: '<div />' },
  SqlWorkModal: { template: '<div />' },
  AiChat: { template: '<div />' },
  AiChatComposer: { template: '<div />' },
  AgentScheduleModal: { template: '<div />' },
  Field: { template: '<div><slot /></div>' },
  FieldGroup: { template: '<div><slot /></div>' },
  FieldLabel: { template: '<div><slot /></div>' },
  ToggleGroup: { template: '<div><slot /></div>' },
  ToggleGroupItem: { template: '<button><slot /></button>' },
  Slider: { template: '<div />' },
  Textarea: { template: '<textarea />' },
  CollectionPanel: { template: '<div />' },
  GlobalProjectSwitcher: { template: '<div />' }
}

async function mountPage(comp: object) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'dashboard', component: { template: '<div />' } },
      { path: '/databases', name: 'databases', component: { template: '<div />' } },
      { path: '/settings', name: 'settings', component: { template: '<div />' } }
    ]
  })
  await router.push('/')
  await router.isReady()
  const pinia = createPinia()
  setActivePinia(pinia)
  useProjectStore().setProject('00000000-0000-0000-0000-000000000002', '商城后台')
  return mount(comp, { global: { plugins: [router, pinia], stubs: uiStubs } })
}

describe('page coverage', () => {
  beforeEach(() => {
    Object.values(api).forEach((group) =>
      Object.values(group).forEach((fn) => (fn as ReturnType<typeof vi.fn>).mockReset().mockResolvedValue([]))
    )
    api.logs.list.mockResolvedValue([{ id: '1', projectId: 'p', level: 'info', logger: 'l', message: 'm', occurredAt: '2026-01-01T00:00:00Z' }])
    api.logs.getRetention.mockResolvedValue({ scope: 'p', keepDays: 7, updatedAt: '2026-01-01T00:00:00Z' })
    api.gofunctions.list.mockResolvedValue([
      { id: 'g', name: 'hello', file: 'hello.go', exports: ['Hello'], createdAt: 't', updatedAt: 't' }
    ])
    api.cronjobs.list.mockResolvedValue([
      {
        id: 'j',
        name: 'night',
        description: '',
        scheduleKind: 'cron',
        cronExpr: '* * * * *',
        funcFile: 'hello',
        funcExport: 'Hello',
        inputJson: '{}',
        enabled: true,
        lastStatus: 'completed',
        lastError: '',
        runCount: 1,
        targetMissing: false,
        createdAt: 't',
        updatedAt: 't'
      }
    ])
    api.s3.list.mockResolvedValue([{ key: 'a.txt', size: 1, lastModified: 't' }])
    api.s3.presign.mockResolvedValue({ url: 'http://x' })
    api.databases.list.mockResolvedValue([{ id: 'd', name: 'n', status: 'ready', createdAt: 't', updatedAt: 't' }])
    api.llmSettings.get.mockResolvedValue({ defaultProvider: 'openai', defaultModel: 'm', temperature: 0.2, maxTokens: 10 })
    api.llm.providers.mockResolvedValue(['openai'])
    api.agents.modules.mockResolvedValue([{ id: 'database', name: 'DB', description: '', default_tools: [], team_supported: false }])
    api.agents.list.mockResolvedValue([
      { id: 'a', name: 'A', module: 'database', description: '', system_prompt: '', tool_ids: [], team_enabled: false, created_at: 't', updated_at: 't' }
    ])
    api.agentThreads.list.mockResolvedValue([{ id: 'th', title: 'T', created_at: 't', updated_at: 't' }])
    api.agentThreads.messages.mockResolvedValue([])
    api.agentSchedules.list.mockResolvedValue([])
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: vi.fn().mockResolvedValue(undefined) }, configurable: true })
    vi.stubGlobal('open', vi.fn())
  })

  it('Logs load, save retention, and poll', async () => {
    const w = await mountPage(Logs)
    await flushPromises()
    expect(api.logs.list).toHaveBeenCalled()
    const vm = w.vm as any
    vm.keepDaysText = '21'
    await vm.saveRetention()
    expect(api.logs.putRetention).toHaveBeenCalled()
    vm.autoRefresh = true
    await flushPromises()
    vm.autoRefresh = false
    await flushPromises()
    vm.keyword = 'needle'
    vm.from = '2024-01-01T00:00'
    await vm.clearFilters()
    expect(vm.keyword).toBe('')
    expect(vm.from).toBe('')
    api.logs.list.mockRejectedValueOnce(new Error('fail'))
    await vm.load()
    api.logs.putRetention.mockRejectedValueOnce(new Error('nope'))
    await vm.saveRetention()
    w.unmount()
  })

  it('GoFunctions create/edit/view/remove/copy', async () => {
    const w = await mountPage(GoFunctions)
    await flushPromises()
    const vm = w.vm as any
    vm.openCreate()
    vm.openEdit({ name: 'hello', file: 'hello.go', exports: ['Hello'] })
    vm.openView({ name: 'hello' })
    await vm.remove({ name: 'hello', file: 'hello.go' })
    vm.copyInvokePath({ name: 'hello', exports: [] })
    vm.copyInvokePath({ name: 'hello', exports: ['Hello'] }, 'Hello')
    api.gofunctions.list.mockRejectedValueOnce('bad')
    await vm.load()
    api.gofunctions.remove.mockRejectedValueOnce(new Error('x'))
    await vm.remove({ name: 'hello', file: 'hello.go' })
    w.unmount()
  })

  it('CronJobs toggle/trigger/remove and schedule text', async () => {
    const w = await mountPage(CronJobs)
    await flushPromises()
    const vm = w.vm as any
    const rec = {
      id: 'j',
      name: 'night',
      scheduleKind: 'interval',
      intervalSeconds: 120,
      enabled: true
    }
    expect(vm.scheduleText({ scheduleKind: 'cron', cronExpr: '* * * * *' })).toContain('*')
    expect(vm.scheduleText({ scheduleKind: 'interval', intervalSeconds: 86400 })).toContain('天')
    expect(vm.scheduleText({ scheduleKind: 'interval', intervalSeconds: 3600 })).toContain('小时')
    expect(vm.scheduleText({ scheduleKind: 'interval', intervalSeconds: 120 })).toContain('分钟')
    expect(vm.scheduleText({ scheduleKind: 'interval', intervalSeconds: 61 })).toContain('秒')
    expect(w.text()).toContain('成功')
    api.cronjobs.update.mockResolvedValue({ ...rec, enabled: false })
    await vm.toggleEnabled(rec)
    api.cronjobs.update.mockRejectedValueOnce(new Error('x'))
    await vm.toggleEnabled(rec)
    await vm.trigger(rec)
    api.cronjobs.trigger.mockRejectedValueOnce(new Error('x'))
    await vm.trigger(rec)
    await vm.remove(rec)
    api.cronjobs.remove.mockRejectedValueOnce(new Error('x'))
    await vm.remove(rec)
    vm.openCreate()
    vm.openEdit(rec)
    vm.openRuns(rec)
    api.cronjobs.list.mockRejectedValueOnce('bad')
    await vm.load()
    w.unmount()
  })

  it('S3Manager list/open/remove/upload validation', async () => {
    const w = await mountPage(S3Manager)
    await flushPromises()
    const vm = w.vm as any
    expect(vm.validateKey('')).toBeTruthy()
    expect(vm.validateKey('a'.repeat(1025))).toBeTruthy()
    expect(vm.validateKey('a\0b')).toBeTruthy()
    expect(vm.validateKey('/abs')).toBeTruthy()
    expect(vm.validateKey('../x')).toBeTruthy()
    expect(vm.validateKey('ok.txt')).toBe('')
    await vm.open('a.txt')
    api.s3.presign.mockRejectedValueOnce(new Error('x'))
    await vm.open('a.txt')
    await vm.remove('a.txt')
    api.s3.remove.mockRejectedValueOnce(new Error('x'))
    await vm.remove('a.txt')
    api.s3.list.mockRejectedValueOnce(new Error('x'))
    await vm.load()
    vm.handleBeforeUpload(new File(['x'], '../bad'))
    vm.handleBeforeUpload(new File([new Uint8Array(2)], 'ok.txt'))
    await flushPromises()
    w.unmount()
  })

  it('Databases and Settings mount and load', async () => {
    const db = await mountPage(Databases)
    await flushPromises()
    expect(api.databases.list).toHaveBeenCalled()
    const dbVm = db.vm as any
    const item = { id: 'd', name: 'n', status: 'ready' }
    expect(dbVm.isReady(item)).toBe(true)
    expect(dbVm.isOpenable({ status: 'closed' })).toBe(true)
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
    await dbVm.openDb(item)
    api.databases.open.mockRejectedValueOnce(new Error('x'))
    await dbVm.openDb(item)
    await dbVm.closeDb(item)
    api.databases.close.mockRejectedValueOnce(new Error('x'))
    await dbVm.closeDb(item)
    await dbVm.removeDb(item)
    api.databases.remove.mockRejectedValueOnce(new Error('x'))
    await dbVm.removeDb(item)
    db.unmount()

    const settings = mount(SettingsPanel, {
      props: { section: 'providers' },
      global: { plugins: [createPinia()], stubs: { ...uiStubs, default: true } },
      shallow: true
    })
    await flushPromises()
    const svm = settings.vm as any
    svm.onTheme('dark')
    svm.onTheme(['light'])
    await svm.loadServerDefaults()
    api.llmSettings.get.mockRejectedValueOnce(new Error('x'))
    await svm.loadServerDefaults()
    await svm.patchDefaults({ temperature: 0.1 })
    api.llmSettings.put.mockRejectedValueOnce(new Error('x'))
    await svm.patchDefaults({})
    svm.localCfg('openai')
    svm.configured('openai')
    svm.mask('sk-abcdefghijk')
    svm.mask('')
    await svm.setDefault('openai')
    svm.openEditor('openai')
    svm.saveEditor()
    svm.clearEditor()
    settings.unmount()

    const agents = mount(AgentManager, {
      global: { plugins: [createPinia()], stubs: { ...uiStubs, default: true } },
      shallow: true
    })
    await flushPromises()
    const avm = agents.vm as any
    await avm.bootstrap?.()
    await avm.loadSchedules?.()
    api.agentSchedules.list.mockRejectedValueOnce(new Error('x'))
    await avm.loadSchedules?.()
    expect(avm.scheduleSummary?.({ cron_expr: '* * * * *', enabled: true, next_run_at: 't' })).toBeTruthy()
    avm.openSchedule?.({ id: 'a' })
    avm.onScheduleSaved?.({ id: 's', agent_id: 'a' })
    avm.onScheduleRemoved?.('s')
    await avm.onViewScheduleThread?.('th')
    await avm.loadModules?.()
    await avm.loadAgents?.()
    api.agents.list.mockRejectedValueOnce(new Error('x'))
    await avm.loadAgents?.()
    await avm.ensureThread?.()
    avm.openCreate?.()
    avm.openEdit?.({ id: 'a', name: 'A', module: 'database', tool_ids: [] })
    avm.onModuleChange?.('s3')
    avm.toggleTool?.('list_databases')
    await avm.saveAgent?.()
    await avm.removeAgent?.({ id: 'a', name: 'A' })
    api.agents.remove.mockRejectedValueOnce(new Error('x'))
    await avm.removeAgent?.({ id: 'a', name: 'A' })
    await avm.resetThread?.()
    avm.onStop?.()
    api.agentThreads.streamRun.mockReturnValue({ close: vi.fn() })
    await avm.onSend?.('hi', [{ agent_id: 'a' }])
    agents.unmount()
  })
})
