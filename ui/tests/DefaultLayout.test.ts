import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resetApiMocks } from '@/test/api-mock'
import { mountWithApp } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return {
    api: m.api,
  }
})

vi.mock('@/components/NavMenu.vue', () => ({
  default: { template: '<nav class="nav-stub">menu</nav>' }
}))
vi.mock('@/components/SettingsModal.vue', () => ({
  default: { template: '<div class="settings-modal" />' }
}))
vi.mock('@/components/GlobalProjectSwitcher.vue', () => ({
  default: { template: '<div class="switcher" />' }
}))
vi.mock('@/components/UserMenu.vue', () => ({
  default: { template: '<div class="user-menu" />' }
}))
vi.mock('@/components/modal/LoginModal.vue', () => ({
  default: { template: '<div class="login-modal" />' }
}))

import DefaultLayout from '@/layouts/DefaultLayout.vue'
import { useAuthStore } from '@/stores/auth'

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
    expect(wrapper.text()).toContain('使用文档')
    const auth = useAuthStore(pinia)
    const iconBtns = wrapper.findAll('button')
    // 按钮顺序：… → 设置 → 刷新（UserMenu 在未登录时不渲染）
    await iconBtns[iconBtns.length - 2].trigger('click')
    expect(auth.settingsOpen).toBe(true)
    auth.markUnauthorized()
    await flushPromises()
    expect(auth.settingsTab).toBe('connection')
    await iconBtns[iconBtns.length - 1].trigger('click')
    expect(reload).toHaveBeenCalled()
  })

  it('opens settings from the settings query deep-link', async () => {
    // 旧书签 ?settings=appearance 已下线：只打开弹窗，回退默认 connection tab
    const { wrapper, pinia, router } = await mountWithApp(DefaultLayout, { path: '/?settings=appearance' })
    const auth = useAuthStore(pinia)
    expect(auth.settingsOpen).toBe(true)
    expect(auth.settingsTab).toBe('connection')
    expect(router.currentRoute.value.query.settings).toBeUndefined()
    wrapper.unmount()

    const generic = await mountWithApp(DefaultLayout, { path: '/?settings=1' })
    expect(useAuthStore(generic.pinia).settingsOpen).toBe(true)
    generic.wrapper.unmount()

    const listed = await mountWithApp(DefaultLayout, { path: '/' })
    await listed.router.push({ path: '/', query: { settings: ['models'] } })
    await flushPromises()
    expect(useAuthStore(listed.pinia).settingsOpen).toBe(true)
    expect(useAuthStore(listed.pinia).settingsTab).toBe('models')
    listed.wrapper.unmount()
  })

  it('falls back to SimpleBase when the route has no name', async () => {
    const { wrapper } = await mountWithApp(DefaultLayout, { path: '/unnamed' })
    expect(wrapper.text()).toContain('SimpleBase')
  })

  it('toggles theme from the header icon button', async () => {
    const { wrapper, pinia } = await mountWithApp(DefaultLayout, { path: '/' })
    const { useSettingsStore } = await import('@/stores/settings')
    const settings = useSettingsStore(pinia)
    expect(settings.effectiveTheme).toBe('light')
    const btn = wrapper.get('button[aria-label="切换为深色主题"]')
    await btn.trigger('click')
    expect(settings.theme).toBe('dark')
    await flushPromises()
    const btnBack = wrapper.get('button[aria-label="切换为浅色主题"]')
    await btnBack.trigger('click')
    expect(settings.theme).toBe('light')
    wrapper.unmount()
  })

  it('first click resolves a stored system theme to an explicit mode', async () => {
    const { wrapper, pinia } = await mountWithApp(DefaultLayout, { path: '/' })
    const { useSettingsStore } = await import('@/stores/settings')
    const settings = useSettingsStore(pinia)
    settings.setTheme('system')
    await flushPromises()
    const label = settings.effectiveTheme === 'dark' ? '切换为浅色主题' : '切换为深色主题'
    await wrapper.get(`button[aria-label="${label}"]`).trigger('click')
    expect(settings.theme).not.toBe('system')
    expect(['light', 'dark']).toContain(settings.theme)
    wrapper.unmount()
  })
})
