import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { useProjectStore } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'
import { sampleAgent, sampleCron, sampleGoFn, readyDb, uiStubs } from '@/test/helpers'
import CronJobModal from '@/components/modal/CronJobModal.vue'
import GoFunctionModal from '@/components/modal/GoFunctionModal.vue'
import SqlWorkModal from '@/components/modal/SqlWorkModal.vue'
import DocumentListModal from '@/components/modal/DocumentListModal.vue'
import CreateCollectionModal from '@/components/modal/CreateCollectionModal.vue'
import CreateProjectModal from '@/components/modal/CreateProjectModal.vue'
import DocumentKvModal from '@/components/modal/DocumentKvModal.vue'
import SbModal from '@/components/modal/SbModal.vue'
import AgentScheduleModal from '@/components/ai/AgentScheduleModal.vue'
import AiChat from '@/components/ai/AiChat.vue'
import AiChatComposer from '@/components/ai/AiChatComposer.vue'
import CollectionPanel from '@/components/databases/CollectionPanel.vue'
import CronJobRunsModal from '@/components/modal/CronJobRunsModal.vue'
import GlobalProjectSwitcher from '@/components/GlobalProjectSwitcher.vue'
import NavMenu from '@/components/NavMenu.vue'
import ConnectionPanel from '@/components/settings/ConnectionPanel.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
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
  useProjectStore().setProject('dev-shop')
  return pinia
}

describe('component template interactions', () => {
  beforeEach(() => {
    resetApiMocks()
    api.gofunctions.list.mockResolvedValue([sampleGoFn])
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
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


})
