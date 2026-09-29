import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { useProjectStore } from '../stores/project'
import { api, resetApiMocks } from '../test/api-mock'
import { clickText, mountWithApp } from '../test/helpers'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return { api: m.api, isMock: false }
})

import GlobalProjectSwitcher from './GlobalProjectSwitcher.vue'

const modalStub = {
  CreateProjectModal: {
    props: ['open'],
    emits: ['created', 'update:open'],
    template:
      '<div v-if="open" class="cpm"><button type="button" class="created" @click="$emit(\'created\', { id: \'p-new\', name: \'New\', createdAt: \'t\' })">c</button></div>'
  }
}

describe('GlobalProjectSwitcher', () => {
  beforeEach(() => {
    resetApiMocks()
    api.projects.list.mockResolvedValue([
      { id: 'dev-shop', name: '商城', createdAt: 't' },
      { id: 'p-b', name: 'Beta', createdAt: 't' }
    ])
  })

  it('merges history, selects a project, and opens create', async () => {
    const { wrapper, pinia } = await mountWithApp(GlobalProjectSwitcher, { stubs: modalStub })
    const store = useProjectStore(pinia)
    store.remember('orphan-id')
    await flushPromises()
    const items = wrapper.findAll('.cmd-item')
    expect(items.length).toBeGreaterThan(0)
    await items[items.length - 1].trigger('click')
    await flushPromises()
    expect(toast.success).toHaveBeenCalled()

    await items[0].trigger('click')
    await clickText(wrapper, '新建项目')
    await flushPromises()
    expect(store.createModalOpen).toBe(true)
    await wrapper.get('.created').trigger('click')
    await flushPromises()
    expect(store.id).toBe('p-new')
    expect(api.projects.list).toHaveBeenCalled()
  })
})
