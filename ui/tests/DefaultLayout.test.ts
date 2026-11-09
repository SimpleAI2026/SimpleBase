import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resetApiMocks } from '@/test/api-mock'
import { mountWithApp } from '@/test/helpers'
import { clearTokens, setTokens } from '@/services/http'

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
    setTokens('a', 'r')
  })

  it('does not mount project data or pages before authentication', async () => {
    clearTokens()
    const { wrapper, pinia } = await mountWithApp(DefaultLayout, { path: '/' })
    expect(wrapper.find('.switcher').exists()).toBe(false)
    expect(wrapper.find('.nav-stub').exists()).toBe(true)
    expect(wrapper.find('.settings-modal').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('dash')

    useAuthStore(pinia).applyTokens({
      accessToken: 'a',
      refreshToken: 'r',
      user: {
        id: 'u1',
        username: 'root',
        role: 'superadminl1',
        displayName: 'root',
        email: '',
        status: 'active',
        mustChangePassword: false
      }
    })
    await flushPromises()
    expect(wrapper.find('.switcher').exists()).toBe(true)
    expect(wrapper.find('.nav-stub').exists()).toBe(true)
    expect(wrapper.text()).toContain('dash')
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
    // 按无障碍标签定位，避免按钮增删（语言下拉）影响位置断言
    await wrapper.get('button[aria-label="设置"]').trigger('click')
    expect(auth.settingsOpen).toBe(true)
    await wrapper.get('button[aria-label="刷新页面"]').trigger('click')
    expect(reload).toHaveBeenCalled()
    auth.markUnauthorized()
    await flushPromises()
    expect(auth.settingsTab).toBe('connection')
    expect(wrapper.find('.switcher').exists()).toBe(false)
  })

  it('provides a mobile navigation trigger in the header', async () => {
    const { wrapper } = await mountWithApp(DefaultLayout, { path: '/' })
    const trigger = wrapper.get('header button[aria-label="打开导航"]')
    expect(trigger.classes()).toContain('md:hidden')
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
