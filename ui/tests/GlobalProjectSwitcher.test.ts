import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { useAuthStore } from '@/stores/auth'
import { ADMIN_PROJECT_ID, useProjectStore } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, installJsdomPolyfills, mountWithApp, realPopoverStubs, uiStubs } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

import GlobalProjectSwitcher from '@/components/GlobalProjectSwitcher.vue'

const popoverStubs = { ...realPopoverStubs }
const trigger = (w: VueWrapper) => w.get('[data-slot="popover-trigger"]')
const optionEls = () => Array.from(document.body.querySelectorAll<HTMLElement>('[data-slot="command-item"]'))

const modalStub = {
  CreateProjectModal: {
    props: ['open'],
    emits: ['created', 'update:open'],
    template:
      '<div v-if="open" class="cpm"><button type="button" class="created" @click="$emit(\'created\', { id: \'p-new\', name: \'New\', createdAt: \'t\' })">c</button></div>'
  }
}

describe('GlobalProjectSwitcher', () => {
  installJsdomPolyfills()
  beforeEach(() => {
    resetApiMocks()
    api.projects.list.mockResolvedValue([
      { id: 'dev-shop', name: '商城', createdAt: 't' },
      { id: 'p-b', name: 'Beta', createdAt: 't' }
    ])
  })

  it('lists server projects only, selects one, and opens create', async () => {
    const { wrapper, pinia } = await mountWithApp(GlobalProjectSwitcher, { stubs: { ...popoverStubs, ...modalStub }, attachTo: document.body })
    const store = useProjectStore(pinia)
    store.remember('orphan-id')
    await flushPromises()
    // 真实 Popover：先点触发器打开弹层，选项在 body 里（teleport）
    await trigger(wrapper).trigger('click')
    await flushPromises()
    expect(document.body.textContent).not.toContain('orphan-id')
    let items = optionEls()
    expect(items.map((el) => el.textContent?.trim())).toEqual([expect.stringContaining('dev-shop'), expect.stringContaining('p-b')])
    items[items.length - 1].click()
    await flushPromises()
    expect(store.id).toBe('p-b')
    expect(toast.success).toHaveBeenCalled()
    // 选中后弹层关闭
    expect(trigger(wrapper).attributes('aria-expanded')).toBe('false')

    await trigger(wrapper).trigger('click')
    await flushPromises()
    items = optionEls()
    items[0].click()
    await flushPromises()
    expect(store.id).toBe('dev-shop')
    await trigger(wrapper).trigger('click')
    await flushPromises()
    ;Array.from(document.body.querySelectorAll<HTMLElement>('button')).find((b) => b.textContent?.includes('新建项目'))!.click()
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
    const { wrapper, pinia } = await mountWithApp(GlobalProjectSwitcher, { stubs: { ...popoverStubs, ...modalStub }, attachTo: document.body })
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
    // 项目列表在真实 Popover 里：先打开再读 body
    await trigger(wrapper).trigger('click')
    await flushPromises()
    const text = document.body.textContent ?? ''
    expect(text).not.toContain('★')
    expect(text).toContain('（只读）')
    expect(text).not.toContain('admin 管理数据库')
    expect(text).not.toContain('全部项目')
    expect(text).not.toContain('我的项目')
  })
  it('handles close interactions', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore(pinia).setProject('dev-shop')
    const sw = mount(GlobalProjectSwitcher, { global: { plugins: [pinia], stubs: uiStubs } })
    const store = useProjectStore(pinia)
    store.openCreateModal()
    await flushPromises()
    const modal = sw.findComponent({ name: 'CreateProjectModal' })
    await modal.vm.$emit('update:open', false)
    expect(store.createModalOpen).toBe(false)
    sw.unmount()
  })

})

  it('GlobalProjectSwitcher popover open/close and create modal close', async () => {
    api.projects.list.mockResolvedValue([])
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore(pinia).setProject('dev-shop')
    const store = useProjectStore(pinia)
    store.history = ['orphan']
    store.projectId = 'missing-current'
    store.projectName = 'Ghost'
    const w = mount(GlobalProjectSwitcher, { attachTo: document.body, global: { plugins: [pinia], stubs: { ...uiStubs, ...popoverStubs } } })
    await flushPromises()
    // 真实 Popover：触发器开合；列表为空时显示「无匹配」，历史里的孤儿项不出现
    expect(trigger(w).attributes('aria-expanded')).toBe('false')
    await trigger(w).trigger('click')
    await flushPromises()
    expect(trigger(w).attributes('aria-expanded')).toBe('true')
    expect(optionEls()).toHaveLength(0)
    expect(document.body.textContent).not.toContain('orphan')
    await trigger(w).trigger('click')
    await flushPromises()
    expect(trigger(w).attributes('aria-expanded')).toBe('false')
    w.unmount()
  })
