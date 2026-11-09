import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { i18n, setI18nLocale, t } from '@/locales'
import { useSettingsStore } from '@/stores/settings'
import { localeOptions, useLocale } from '@/composables/useLocale'
import { resetApiMocks } from '@/test/api-mock'
import { mountWithApp, uiStubs } from '@/test/helpers'
import { setTokens } from '@/services/http'
import LocaleSelect from '@/components/LocaleSelect.vue'

/**
 * 界面语言：默认中文、不做浏览器探测，顶栏下拉手动切换并持久化，
 * 切换时同步 i18n 实例与 <html lang>。下拉只展示「中 / EN」文字图标，不展示完整语言名。
 */
vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})
vi.mock('@/components/NavMenu.vue', () => ({ default: { template: '<nav />' } }))
vi.mock('@/components/SettingsModal.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/GlobalProjectSwitcher.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/UserMenu.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/modal/LoginModal.vue', () => ({ default: { template: '<div />' } }))

import DefaultLayout from '@/layouts/DefaultLayout.vue'

/** useLocale 依赖 useI18n() 的注入上下文，用一个宿主组件取到它。 */
function mountUseLocale() {
  let api: ReturnType<typeof useLocale>
  const Host = defineComponent({
    setup() {
      api = useLocale()
      return () => null
    }
  })
  const wrapper = mount(Host, { global: { plugins: [i18n] } })
  return { api: api!, wrapper }
}

describe('useLocale', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    setI18nLocale('zh-CN')
    useSettingsStore().setLocale('zh-CN')
  })

  it('暴露语言选项与当前值，且标签用各自语言书写', () => {
    const { api, wrapper } = mountUseLocale()
    expect(api.current.value).toBe('zh-CN')
    expect(api.options.map((o) => o.value)).toEqual(['zh-CN', 'en-US'])
    expect(api.options.map((o) => o.label)).toEqual(['简体中文', 'English'])
    expect(localeOptions).toHaveLength(2)
    wrapper.unmount()
  })

  it('setLocale 同步 store、i18n 实例与 <html lang>', () => {
    const { api, wrapper } = mountUseLocale()
    api.setLocale('en-US')
    expect(api.current.value).toBe('en-US')
    expect(useSettingsStore().locale).toBe('en-US')
    expect(i18n.global.locale.value).toBe('en-US')
    expect(document.documentElement.lang).toBe('en')
    expect(t('common.ok')).toBe('OK')

    api.setLocale('zh-CN')
    expect(document.documentElement.lang).toBe('zh-CN')
    expect(t('common.ok')).toBe('确定')
    wrapper.unmount()
  })
})

describe('LocaleSelect', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    setI18nLocale('zh-CN')
    useSettingsStore().setLocale('zh-CN')
  })

  it('触发器带完整语言名的 aria-label，且图标对读屏隐藏', () => {
    // 真实 SelectValue 才会渲染触发器内的徽标；这里用真实 ui 组件而非桩
    const w = mount(LocaleSelect)
    const trigger = w.get('[aria-label="界面语言"]')
    const badge = trigger.get('[data-locale="zh-CN"]')
    expect(badge.text()).toBe('中')
    expect(badge.attributes('aria-hidden')).toBe('true')
    expect(trigger.text()).not.toContain('简体中文')
    w.unmount()
  })

  it('英文时触发器显示 EN', () => {
    useSettingsStore().setLocale('en-US')
    const w = mount(LocaleSelect)
    const trigger = w.get('[role="combobox"]')
    expect(trigger.get('[data-locale="en-US"]').text()).toBe('EN')
    // aria-label 随界面语言变化，语言名不在触发器文字里
    expect(trigger.attributes('aria-label')).toBe('Interface language')
    w.unmount()
  })

  it('选项为「中 / EN」文字图标，语言名仅以 sr-only 提供', () => {
    const w = mount(LocaleSelect, { global: { stubs: uiStubs } })
    expect(w.findAll('[data-locale]').map((n) => n.text())).toEqual(['中', 'EN'])
    expect(w.findAll('.sr-only').map((n) => n.text())).toEqual(['简体中文', 'English'])
    w.unmount()
  })
})

describe('顶栏语言下拉', () => {
  beforeEach(() => {
    resetApiMocks()
    setTokens('a', 'r')
    setI18nLocale('zh-CN')
    useSettingsStore().setLocale('zh-CN')
  })

  it('切换后持久化并刷新 <html lang>', async () => {
    const { wrapper } = await mountWithApp(DefaultLayout, { path: '/' })
    const settings = useSettingsStore()
    // 桌面顶栏与移动端侧栏各一份，取第一份驱动
    const select = wrapper.findComponent(LocaleSelect)
    expect(select.exists()).toBe(true)

    const vm = select.vm as unknown as { onChange: (v: string) => void }
    vm.onChange('en-US')
    await flushPromises()
    expect(settings.locale).toBe('en-US')
    expect(i18n.global.locale.value).toBe('en-US')
    expect(document.documentElement.lang).toBe('en')
    expect(JSON.parse(localStorage.getItem('sb_settings_v1') || '{}').locale).toBe('en-US')

    vm.onChange('zh-CN')
    await flushPromises()
    expect(settings.locale).toBe('zh-CN')
    wrapper.unmount()
  })

  it('折叠态：侧栏底部有一个展开用的 SidebarTrigger，且仅折叠时可见', async () => {
    const { wrapper } = await mountWithApp(DefaultLayout, { path: '/' })
    const footer = wrapper.findAll('div').find((d) => d.classes().includes('group-data-[collapsible=icon]:flex'))
    expect(footer).toBeTruthy()
    // 默认隐藏（hidden），只有 group-data-[collapsible=icon] 命中时才 flex
    expect(footer!.classes()).toContain('hidden')
    expect(footer!.findAll('button')).toHaveLength(1)
    wrapper.unmount()
  })
})
