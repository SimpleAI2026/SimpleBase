import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getApiKey, getAccessToken, getRefreshToken, clearTokens, setTokens } from '../services/http'
import { useAuthStore } from './auth'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return {
    api: m.api,
    get isMock() {
      return true
    }
  }
})

const mockApi = await import('../test/api-mock')

describe('useAuthStore roles and login', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    clearTokens()
    mockApi.resetApiMocks()
  })

  it('defaults to unauthenticated with API key channel writable', () => {
    const store = useAuthStore()
    expect(store.isAuthenticated).toBe(false)
    expect(store.role).toBe('')
    expect(store.isSuper).toBe(false)
    expect(store.isAdminRole).toBe(false)
    expect(store.isUser).toBe(false)
    expect(store.canWrite).toBe(true)
    expect(store.canCreateProject).toBe(true)
    expect(store.canManageUsers).toBe(false)
    expect(store.canViewUsers).toBe(false)
  })

  it('login applies tokens and role getters', async () => {
    const store = useAuthStore()
    mockApi.api.auth.login = vi.fn(async () => ({
      tokenType: 'Bearer',
      accessToken: 'at-1',
      expiresIn: 7200,
      refreshToken: 'rt-1',
      user: {
        id: 'u1',
        username: 'simplebase2026',
        role: 'superadminl1' as const,
        displayName: 'Super',
        email: '',
        status: 'active' as const,
        mustChangePassword: false
      }
    }))
    await store.login('simplebase2026', 'simplebase2026')
    expect(store.isSuper).toBe(true)
    expect(store.canManageUsers).toBe(true)
    expect(store.canViewUsers).toBe(true)
    expect(store.canWrite).toBe(true)
    expect(store.displayName).toBe('Super')
    expect(getAccessToken()).toBe('at-1')
    expect(getRefreshToken()).toBe('rt-1')
    expect(store.loginOpen).toBe(false)
  })

  it('admin is read-only and can view users', async () => {
    const store = useAuthStore()
    store.applyTokens({
      accessToken: 'a',
      refreshToken: 'r',
      user: {
        id: 'u2',
        username: 'admin',
        role: 'admin',
        displayName: '',
        email: '',
        status: 'active',
        mustChangePassword: false
      }
    })
    expect(store.isAdminRole).toBe(true)
    expect(store.canWrite).toBe(false)
    expect(store.canCreateProject).toBe(false)
    expect(store.canManageUsers).toBe(false)
    expect(store.canViewUsers).toBe(true)
  })

  it('user role is writable but cannot manage users', async () => {
    const store = useAuthStore()
    store.applyTokens({
      accessToken: 'a',
      refreshToken: 'r',
      user: {
        id: 'u3',
        username: 'alice',
        role: 'user',
        displayName: '',
        email: '',
        status: 'active',
        mustChangePassword: true
      }
    })
    expect(store.isUser).toBe(true)
    expect(store.canWrite).toBe(true)
    expect(store.canCreateProject).toBe(true)
    expect(store.canManageUsers).toBe(false)
    expect(store.mustChangePassword).toBe(true)
  })

  it('logout clears tokens and opens login', async () => {
    const store = useAuthStore()
    mockApi.api.auth.logout = vi.fn(async () => undefined)
    store.applyTokens({
      accessToken: 'a',
      refreshToken: 'r',
      user: {
        id: 'u1',
        username: 'x',
        role: 'user',
        displayName: '',
        email: '',
        status: 'active',
        mustChangePassword: false
      }
    })
    await store.logout()
    expect(store.isAuthenticated).toBe(false)
    expect(getAccessToken()).toBe('')
    expect(store.loginOpen).toBe(true)
  })

  it('bootstrap without token opens login', async () => {
    const store = useAuthStore()
    const ok = await store.bootstrap()
    expect(ok).toBe(false)
    expect(store.loginOpen).toBe(true)
  })

  it('bootstrap with token loads me', async () => {
    setTokens('at', 'rt')
    const store = useAuthStore()
    mockApi.api.auth.me = vi.fn(async () => ({
      id: 'u1',
      username: 'simplebase2026',
      role: 'superadminl1' as const,
      displayName: 'S',
      email: '',
      status: 'active' as const,
      mustChangePassword: false,
      projects: []
    }))
    const ok = await store.bootstrap()
    expect(ok).toBe(true)
    expect(store.isSuper).toBe(true)
  })

  it('bootstrap failure clears tokens and opens login', async () => {
    setTokens('bad', 'bad')
    const store = useAuthStore()
    mockApi.api.auth.me = vi.fn(async () => {
      throw new Error('401')
    })
    const ok = await store.bootstrap()
    expect(ok).toBe(false)
    expect(getAccessToken()).toBe('')
    expect(store.loginOpen).toBe(true)
  })

  it('changePassword clears mustChangePassword', async () => {
    const store = useAuthStore()
    mockApi.api.auth.changePassword = vi.fn(async () => undefined)
    store.mustChangePassword = true
    await store.changePassword('old12345', 'new12345')
    expect(store.mustChangePassword).toBe(false)
  })

  it('markUnauthorized opens login and settings', () => {
    const store = useAuthStore()
    store.markUnauthorized()
    expect(store.lastUnauthorizedAt).toBeGreaterThan(0)
    expect(store.loginOpen).toBe(true)
    expect(store.settingsOpen).toBe(true)
    expect(store.settingsTab).toBe('connection')
  })
})
