import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { resolvedTheme, useSettingsStore } from '@/stores/settings'

describe('settings store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('resolves theme modes including system', () => {
    expect(resolvedTheme('light')).toBe('light')
    expect(resolvedTheme('dark')).toBe('dark')
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: (query: string) => ({
        matches: query.includes('dark'),
        media: query,
        addEventListener: () => undefined,
        removeEventListener: () => undefined
      })
    })
    expect(resolvedTheme('system')).toBe('dark')
  })

  it('recovers from invalid stored JSON and persists theme', () => {
    localStorage.setItem('sb_settings_v1', '{bad')
    setActivePinia(createPinia())
    const store = useSettingsStore()
    expect(store.theme).toBe('light')
    store.setTheme('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(JSON.parse(localStorage.getItem('sb_settings_v1') || '{}').theme).toBe('dark')
  })

  it('manages project defaults', () => {
    const store = useSettingsStore()
    store.setProjectDefaults('p1', { defaultProvider: 'openai', temperature: 0.2 })
    expect(store.defaultsFor('p1').temperature).toBe(0.2)
    expect(store.defaultsFor('p1').defaultProvider).toBe('openai')
    expect(store.defaultsFor('missing')).toEqual({})

    // 连接信息与服务端默认值同份缓存，掩码只用于展示
    store.setProjectDefaults('p1', { baseUrl: 'https://relay.example.com/v1', apiKeyMasked: 'sk-...7890', hasApiKey: true })
    expect(store.defaultsFor('p1').baseUrl).toBe('https://relay.example.com/v1')
    expect(store.defaultsFor('p1').apiKeyMasked).toBe('sk-...7890')
    expect(store.defaultsFor('p1').hasApiKey).toBe(true)
    // 局部 patch 不冲掉已有字段
    expect(store.defaultsFor('p1').temperature).toBe(0.2)
  })

  it('applies persisted locale to i18n and <html lang>', () => {
    const store = useSettingsStore()
    store.setLocale('zh-CN')
    // applyLocaleToI18n：启动时把已持久化的语言同步到 i18n 与 <html lang>
    document.documentElement.removeAttribute('lang')
    store.applyLocaleToI18n()
    expect(document.documentElement.getAttribute('lang')).toBe('zh-CN')
    store.setLocale('en-US')
    // htmlLang('en-US') 取主语言段（locales/index.ts §htmlLang）
    expect(document.documentElement.getAttribute('lang')).toBe('en')
  })

  it('loads missing nested maps and tolerates a legacy payload', () => {
    localStorage.setItem('sb_settings_v1', JSON.stringify({ theme: 'light' }))
    setActivePinia(createPinia())
    const store = useSettingsStore()
    expect(store.defaultsFor('p')).toEqual({})

    // 旧版本存在 providerConfigs（浏览器明文 Key 时代）：读取时忽略，不崩
    localStorage.setItem(
      'sb_settings_v1',
      JSON.stringify({ theme: 'light', providerConfigs: { p: { openai: { credentials: { api_key: 'k' } } } } })
    )
    setActivePinia(createPinia())
    const legacy = useSettingsStore()
    expect(legacy.defaultsFor('p')).toEqual({})
  })

  it('persists projectDefaults so they survive a reload', () => {
    localStorage.setItem('sb_settings_v1', JSON.stringify({ theme: 'light' }))
    setActivePinia(createPinia())
    const store = useSettingsStore()
    store.setProjectDefaults('p1', { defaultProvider: 'openai', defaultModel: 'gpt-4o-mini' })
    const saved = JSON.parse(localStorage.getItem('sb_settings_v1') || '{}')
    expect(saved.projectDefaults.p1.defaultProvider).toBe('openai')
    // 旧的 providerConfigs 字段不再写回
    expect(saved.providerConfigs).toBeUndefined()
    // 模拟 reload：新 store 从 localStorage 恢复
    setActivePinia(createPinia())
    const reloaded = useSettingsStore()
    expect(reloaded.defaultsFor('p1').defaultProvider).toBe('openai')
    expect(reloaded.defaultsFor('p1').defaultModel).toBe('gpt-4o-mini')
  })

  it('falls back for unknown stored theme', () => {
    localStorage.setItem(
      'sb_settings_v1',
      JSON.stringify({
        theme: 'rainbow',
        projectDefaults: { p: { defaultModel: 'm' } }
      })
    )
    setActivePinia(createPinia())
    const store = useSettingsStore()
    expect(store.theme).toBe('light')
    expect(store.defaultsFor('p').defaultModel).toBe('m')
  })
})
