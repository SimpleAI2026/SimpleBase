import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { useAuthStore } from '@/stores/auth'
import { ADMIN_PROJECT_ID, useProjectStore } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, mountWithApp } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api, isMock: false }
})

import GlobalProjectSwitcher from '@/components/GlobalProjectSwitcher.vue'

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

  it('lists server projects only, selects one, and opens create', async () => {
    const { wrapper, pinia } = await mountWithApp(GlobalProjectSwitcher, { stubs: modalStub })
    const store = useProjectStore(pinia)
    store.remember('orphan-id')
    await flushPromises()
    expect(wrapper.text()).not.toContain('orphan-id')
    const items = wrapper.findAll('.cmd-item')
    expect(items.length).toBeGreaterThan(0)
    await items[items.length - 1].trigger('click')
    await flushPromises()
    expect(toast.success).toHaveBeenCalled()

    await items[0].trigger('click')
    await clickText(wrapper, '新建项目')
    await flushPromises()
    expect(store.createModalOpen).toBe(true)
    api.projects.list.mockResolvedValue([
      { id: 'dev-shop', name: '商城', createdAt: 't' },
      { id: 'p-b', name: 'Beta', createdAt: 't' },
      { id: 'p-new', name: 'New', createdAt: 't' }
    ])
    await wrapper.get('.created').trigger('click')
    await flushPromises()
    expect(store.id).toBe('p-new')
    expect(api.projects.list).toHaveBeenCalled()
  })

  it('marks managed databases with a star and does not show group headings', async () => {
    api.projects.list.mockResolvedValue([
      { id: 'dev-shop', name: '商城', createdAt: 't' },
      { id: ADMIN_PROJECT_ID, name: 'admin', createdAt: 't', managed: true }
    ])
    const { wrapper, pinia } = await mountWithApp(GlobalProjectSwitcher, { stubs: modalStub })
    const auth = useAuthStore(pinia)
    auth.user = {
      id: 'u1',
      username: 'root',
      role: 'superadminl1',
      displayName: 'root',
      email: '',
      status: 'active',
      mustChangePassword: false
    }
    await flushPromises()
    expect(wrapper.text()).toContain('★')
    expect(wrapper.text()).toContain('（只读）')
    expect(wrapper.text()).not.toContain('admin 管理数据库')
    expect(wrapper.text()).not.toContain('全部项目')
    expect(wrapper.text()).not.toContain('我的项目')
  })
})
