import { flushPromises } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '../stores/auth'
import { mountWithApp } from '../test/helpers'
import SettingsModal from './SettingsModal.vue'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return { api: m.api, isMock: false }
})

describe('SettingsModal', () => {
  it('opens through the auth store with 900px max-width and switches tabs', async () => {
    const { wrapper, pinia } = await mountWithApp(SettingsModal)
    const auth = useAuthStore(pinia)
    expect(wrapper.find('.sb-modal').exists()).toBe(false)
    auth.openSettings()
    await flushPromises()
    const modal = wrapper.get('.sb-modal')
    expect(modal.attributes('data-title')).toBe('设置')
    expect(modal.attributes('data-max-width')).toBe('900')
    expect(wrapper.text()).toContain('连接')
    expect(wrapper.text()).toContain('外观')

    auth.markUnauthorized()
    await flushPromises()
    expect(auth.settingsTab).toBe('connection')
    expect(auth.settingsOpen).toBe(true)

    const vm = wrapper.vm as any
    vm.onTab('appearance')
    expect(auth.settingsTab).toBe('appearance')
    vm.onTab('models')
    vm.onTab('providers')
    vm.onTab('nope')
    vm.onOpen(true)
    vm.onOpen(false)
    expect(auth.settingsOpen).toBe(false)
  })
})
