import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { getApiKey } from '../services/http'
import { useAuthStore } from './auth'

describe('useAuthStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('reads the default key and ignores empty updates', () => {
    const store = useAuthStore()
    expect(store.apiKey).toBe('sb_live_dev_key_12345')
    store.updateKey('   ')
    expect(store.apiKey).toBe('sb_live_dev_key_12345')
  })

  it('persists a trimmed key and clears unauthorized state', () => {
    const store = useAuthStore()
    store.markUnauthorized()
    expect(store.lastUnauthorizedAt).toBeGreaterThan(0)
    expect(store.keyDrawerOpen).toBe(true)
    store.updateKey('  sb_live_new  ')
    expect(store.apiKey).toBe('sb_live_new')
    expect(getApiKey()).toBe('sb_live_new')
    expect(store.lastUnauthorizedAt).toBe(0)
  })

  it('opens and closes the key drawer', () => {
    const store = useAuthStore()
    store.openDrawer()
    expect(store.keyDrawerOpen).toBe(true)
    store.closeDrawer()
    expect(store.keyDrawerOpen).toBe(false)
  })
})
