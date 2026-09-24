import { describe, expect, it, beforeEach } from 'vitest'
import {
  clearTokens,
  getAccessToken,
  getApiKey,
  getRefreshToken,
  setApiKey,
  setTokens
} from './http'
import { mockApi } from './mock'

describe('http token helpers', () => {
  beforeEach(() => {
    clearTokens()
  })

  it('stores and clears tokens', () => {
    setTokens('acc', 'ref')
    expect(getAccessToken()).toBe('acc')
    expect(getRefreshToken()).toBe('ref')
    clearTokens()
    expect(getAccessToken()).toBe('')
    expect(getRefreshToken()).toBe('')
  })

  it('api key fallback', () => {
    expect(getApiKey()).toBe('sb_live_dev_key_12345')
    setApiKey('  sb_live_x  ')
    // setApiKey 不 trim（与历史行为一致）
    expect(getApiKey()).toBe('  sb_live_x  ' || getApiKey())
  })
})

describe('mockApi auth and users', () => {
  it('login accepts bootstrap superadmin', async () => {
    const pair = await mockApi.auth.login({ username: 'simplebase2026', password: 'simplebase2026' })
    expect(pair.user.role).toBe('superadminl1')
    expect(pair.accessToken).toBeTruthy()
    expect(pair.refreshToken).toBeTruthy()
  })

  it('login accepts mock admin and user', async () => {
    const a = await mockApi.auth.login({ username: 'admin', password: 'admin1234' })
    expect(a.user.role).toBe('admin')
    const u = await mockApi.auth.login({ username: 'user', password: 'user1234' })
    expect(u.user.role).toBe('user')
  })

  it('login rejects bad credentials', async () => {
    await expect(mockApi.auth.login({ username: 'x', password: 'y' })).rejects.toThrow()
  })

  it('refresh and logout and changePassword', async () => {
    await expect(mockApi.auth.refresh('rt')).rejects.toThrow()
    await expect(mockApi.auth.logout('rt')).resolves.toBeUndefined()
    await expect(mockApi.auth.changePassword('a', 'b')).resolves.toBeUndefined()
  })

  it('me returns super with projects', async () => {
    const me = await mockApi.auth.me()
    expect(me.role).toBe('superadminl1')
    expect(Array.isArray(me.projects)).toBe(true)
  })

  it('users list/create/update/remove', async () => {
    const list = await mockApi.users.list()
    expect(list.users.length).toBeGreaterThan(0)
    const created = await mockApi.users.create({
      username: 'bob',
      password: '12345678',
      role: 'user',
      displayName: 'Bob'
    })
    expect(created.username).toBe('bob')
    const got = await mockApi.users.get('user-super')
    expect(got.username).toBe('simplebase2026')
    const updated = await mockApi.users.update('user-super', { displayName: 'X' })
    expect(updated.displayName).toBe('X')
    await expect(mockApi.users.remove('user-normal')).resolves.toBeUndefined()
  })

  it('users get missing rejects', async () => {
    await expect(mockApi.users.get('nope')).rejects.toThrow()
  })
})
