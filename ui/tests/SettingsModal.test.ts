import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth'
import { resetApiMocks } from '@/test/api-mock'
import { mountWithApp } from '@/test/helpers'
import SettingsModal from '@/components/SettingsModal.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

describe('SettingsModal', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('opens as a 720px SbModal with tabs and closes from the shell', async () => {
    const { wrapper, pinia } = await mountWithApp(SettingsModal)
    const auth = useAuthStore(pinia)
    expect(wrapper.find('.sb-modal').exists()).toBe(false)
    auth.openSettings()
    await flushPromises()
    const modal = wrapper.get('.sb-modal')
    expect(modal.attributes('data-title')).toBe('设置')
    expect(modal.attributes('data-max-width')).toBe('720')
    expect(modal.attributes('data-hide-footer')).toBe('true')
    expect(wrapper.text()).toContain('连接、默认模型与厂商 API Key')
    expect(wrapper.text()).toContain('连接')
    expect(wrapper.text()).not.toContain('外观')
    const scroller = wrapper.get('.overflow-y-auto')
    expect(scroller.classes().join(' ')).toContain('max-h-[min(calc(100vh-14rem),720px)]')
    expect(scroller.classes()).toContain('px-1')
    expect(wrapper.text()).toContain('模型')
    expect(wrapper.text()).toContain('供应商')

    await wrapper.get('.tab-emit').trigger('click')
    expect(auth.settingsTab).toBe('connection')

    const vm = wrapper.vm as any
    vm.onTab('appearance')
    expect(auth.settingsTab).toBe('connection')
    vm.onTab('models')
    expect(auth.settingsTab).toBe('models')
    vm.onTab('providers')
    expect(auth.settingsTab).toBe('providers')
    vm.onTab('nope')
    expect(auth.settingsTab).toBe('providers')
    vm.onOpen(true)
    expect(auth.settingsOpen).toBe(true)
    vm.onOpen(false)
    expect(auth.settingsOpen).toBe(false)
  })

  it('keeps the settings max-width inside the viewport-capped shell', async () => {
    const { wrapper, pinia } = await mountWithApp(SettingsModal, { stubs: { SbModal: false } })
    const auth = useAuthStore(pinia)
    auth.openSettings()
    await flushPromises()
    const content = wrapper.get('.dialog-content')
    const cls = content.attributes('class') || ''
    expect(cls).toContain('max-h-[calc(100vh-2rem)]')
    expect(cls).toContain('flex')
    expect(cls).toContain('sm:max-w-[var(--sb-modal-max-w)]')
    expect(cls).not.toContain('sm:max-w-5xl')
    expect(content.attributes('style') || '').toContain('--sb-modal-max-w: 720px')
    const body = wrapper.get('[data-slot="sb-modal-body"]')
    expect(body.classes()).toContain('overflow-y-auto')
    expect(body.text()).toContain('连接')
    expect(body.text()).toContain('模型')
    expect(wrapper.get('.pr-8').text()).toContain('设置')
    expect(wrapper.find('.dialog-footer').exists()).toBe(false)
  })

  it('401 no longer forces the connection tab (planv5.0 §2.2)', async () => {
    const { wrapper, pinia } = await mountWithApp(SettingsModal)
    const auth = useAuthStore(pinia)
    // 未打开设置时 401：只弹登录框，不强制打开设置弹窗
    auth.markUnauthorized()
    await flushPromises()
    expect(auth.settingsOpen).toBe(false)
    expect(auth.loginOpen).toBe(true)

    // 已打开设置（models）时 401：设置保持原 tab，不被强制切到 connection
    auth.openSettings({ tab: 'models' })
    auth.markUnauthorized()
    await flushPromises()
    expect(auth.settingsOpen).toBe(true)
    expect(auth.settingsTab).toBe('models')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)
  })
})
