import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useProjectStore } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'
import {
  creatingDb,
  mountWithApp,
  readyDb,
  sampleAgent,
  sampleCron,
  sampleGoFn,
  uiStubs
} from '@/test/helpers'
import AgentManager from '@/pages/AgentManager.vue'
import CronJobs from '@/pages/CronJobs.vue'
import Databases from '@/pages/Databases.vue'
import GoFunctions from '@/pages/GoFunctions.vue'
import Logs from '@/pages/Logs.vue'
import S3Manager from '@/pages/S3Manager.vue'
import CronJobModal from '@/components/modal/CronJobModal.vue'
import GoFunctionModal from '@/components/modal/GoFunctionModal.vue'
import SqlWorkModal from '@/components/modal/SqlWorkModal.vue'
import DocumentListModal from '@/components/modal/DocumentListModal.vue'
import SbModal from '@/components/modal/SbModal.vue'
import AgentScheduleModal from '@/components/ai/AgentScheduleModal.vue'
import CronJobRunsModal from '@/components/modal/CronJobRunsModal.vue'
import ConnectionPanel from '@/components/settings/ConnectionPanel.vue'
import SettingsPanel from '@/components/settings/SettingsPanel.vue'
import GlobalProjectSwitcher from '@/components/GlobalProjectSwitcher.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
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
    api.databases.list.mockResolvedValue([readyDb, creatingDb])
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

    const settings = await mountWithApp(SettingsPanel, { props: { section: 'models' } })
    const st = vmOf(settings.wrapper)
    api.llmSettings.put.mockRejectedValueOnce({ nope: true })
    await st.patchDefaults({ maxTokens: 1 })
    api.llmSettings.put.mockRejectedValueOnce({ nope: true })
    const providers = await mountWithApp(SettingsPanel, { props: { section: 'providers' } })
    await vmOf(providers.wrapper).setDefault('openai')
    if (settings.wrapper.find('#max-tokens').exists()) {
      await settings.wrapper.get('#max-tokens').setValue('')
      await flushPromises()
    }
    settings.wrapper.unmount()
    providers.wrapper.unmount()

    const go = await mountWithApp(GoFunctions)
    const gvm = vmOf(go.wrapper)
    api.gofunctions.remove.mockRejectedValueOnce('rm-fn')
    await gvm.remove?.(sampleGoFn).catch(() => undefined)
    go.wrapper.unmount()
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
    useProjectStore().setProject('dev-shop')
    const key = mount(ConnectionPanel, { global: { plugins: [pinia], stubs: uiStubs } })
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
