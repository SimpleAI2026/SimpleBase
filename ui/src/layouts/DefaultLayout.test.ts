import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resetApiMocks } from '../test/api-mock'
import { clickText, mountWithApp } from '../test/helpers'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return {
    api: m.api,
    get isMock() {
      return true
    }
  }
})

vi.mock('../components/NavMenu.vue', () => ({
  default: { template: '<nav class="nav-stub">menu</nav>' }
}))
vi.mock('../components/SettingsModal.vue', () => ({
  default: { template: '<div class="settings-modal" />' }
}))
vi.mock('../components/GlobalProjectSwitcher.vue', () => ({
  default: { template: '<div class="switcher" />' }
}))

import DefaultLayout from './DefaultLayout.vue'
import { useAuthStore } from '../stores/auth'

describe('DefaultLayout', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('shows the route title, mock badge, and opens settings', async () => {
    const reload = vi.fn()
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: { ...window.location, reload }
    })
    const { wrapper, pinia } = await mountWithApp(DefaultLayout, { path: '/' })
    expect(wrapper.text()).toContain('监控大盘')
    expect(wrapper.text()).toContain('Mock 数据')
    expect(wrapper.text()).toContain('使用文档')
    const auth = useAuthStore(pinia)
    const settingsBtn = wrapper.findAll('button').find((b) => b.html().includes('Settings') || b.attributes('title') === undefined)
    await clickText(wrapper, 'sb').catch(() => undefined)
    const iconBtns = wrapper.findAll('button')
    await iconBtns[iconBtns.length - 2].trigger('click')
    expect(auth.settingsOpen).toBe(true)
    auth.markUnauthorized()
    await flushPromises()
    await iconBtns[iconBtns.length - 1].trigger('click')
    expect(reload).toHaveBeenCalled()
  })

  it('falls back to SimpleBase when the route has no name', async () => {
    const { wrapper } = await mountWithApp(DefaultLayout, { path: '/unnamed' })
    expect(wrapper.text()).toContain('SimpleBase')
  })

  it('opens the settings modal from the ?settings= query deep-link', async () => {
    const { pinia, router } = await mountWithApp(DefaultLayout, { path: '/?settings=1' })
    const auth = useAuthStore(pinia)
    expect(auth.settingsOpen).toBe(true)
    expect(router.currentRoute.value.query.settings).toBeUndefined()

    await router.push({ path: '/', query: { settings: 'appearance' } })
    await flushPromises()
    expect(auth.settingsTab).toBe('appearance')
  })
})
