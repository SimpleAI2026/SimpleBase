import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '../stores/auth'
import { resetApiMocks } from '../test/api-mock'
import { mountWithApp } from '../test/helpers'
import SettingsModal from './SettingsModal.vue'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return { api: m.api, isMock: false }
})

describe('SettingsModal', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('opens as a 900px SbModal with tabs and closes from the shell', async () => {
    const { wrapper, pinia } = await mountWithApp(SettingsModal)
    const auth = useAuthStore(pinia)
    expect(wrapper.find('.sb-modal').exists()).toBe(false)
    auth.openSettings()
    await flushPromises()
    const modal = wrapper.get('.sb-modal')
    expect(modal.attributes('data-title')).toBe('设置')
    expect(modal.attributes('data-max-width')).toBe('900')
    expect(modal.attributes('data-hide-footer')).toBe('true')
    expect(wrapper.text()).toContain('连接')
    expect(wrapper.text()).toContain('外观')
    expect(wrapper.text()).toContain('模型')
    expect(wrapper.text()).toContain('供应商')

    await wrapper.get('.tab-emit').trigger('click')
    expect(auth.settingsTab).toBe('connection')

    const vm = wrapper.vm as any
    vm.onTab('appearance')
    expect(auth.settingsTab).toBe('appearance')
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

  it('forces the connection tab on 401', async () => {
    const { wrapper, pinia } = await mountWithApp(SettingsModal)
    const auth = useAuthStore(pinia)
    auth.openSettings({ tab: 'appearance' })
    auth.markUnauthorized()
    await flushPromises()
    expect(auth.settingsOpen).toBe(true)
    expect(auth.settingsTab).toBe('connection')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)
  })
})
