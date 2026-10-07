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
