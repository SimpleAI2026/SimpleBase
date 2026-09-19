import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { resolvedTheme, useSettingsStore } from './settings'

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

  it('manages project defaults and provider credentials', () => {
    const store = useSettingsStore()
    store.setProjectDefaults('p1', { defaultProvider: 'openai', temperature: 0.2 })
    expect(store.defaultsFor('p1').temperature).toBe(0.2)
    expect(store.defaultsFor('missing')).toEqual({})

    store.upsertProviderConfig('p1', 'openai', {
      enabled: true,
      defaultModel: 'gpt-4o-mini',
      credentials: { api_key: 'sk-test' }
    })
    store.upsertProviderConfig('p1', 'openai', { credentials: { organization: 'org' } })
    expect(store.configsFor('p1').openai.credentials.api_key).toBe('sk-test')
    expect(store.isProviderConfigured('p1', 'openai')).toBe(true)
    expect(store.isProviderConfigured('p1', 'azure_openai')).toBe(false)
    expect(store.isProviderConfigured('p1', 'unknown')).toBe(false)

    store.setDefaultProvider('p1', 'openai')
    expect(store.defaultsFor('p1').defaultModel).toBe('gpt-4o-mini')

    store.clearProviderCredentials('p1', 'openai')
    expect(store.configsFor('p1').openai.credentials).toEqual({})
    store.clearProviderCredentials('p1', 'missing')
    expect(store.isProviderConfigured('p1', 'openai')).toBe(false)
  })

  it('falls back for unknown stored theme and custom providers', () => {
    localStorage.setItem(
      'sb_settings_v1',
      JSON.stringify({
        theme: 'rainbow',
        projectDefaults: { p: { defaultModel: 'm' } },
        providerConfigs: { p: { custom: { enabled: true, credentials: { api_key: 'k' }, updatedAt: 't' } } }
      })
    )
    setActivePinia(createPinia())
    const store = useSettingsStore()
    expect(store.theme).toBe('light')
    expect(store.isProviderConfigured('p', 'custom')).toBe(true)
    store.setDefaultProvider('p', 'no-such')
    expect(store.defaultsFor('p').defaultProvider).toBe('no-such')
  })
})
