import { describe, expect, it, beforeEach, vi } from 'vitest'
import { httpApi } from './http-api'
import { clearTokens, setTokens, getAccessToken } from './http'

// 拦截真实 http，直接测映射与 token 写入
vi.mock('./http', async () => {
  const actual = await vi.importActual<typeof import('./http')>('./http')
  return {
    ...actual,
    http: {
      get: vi.fn(),
      post: vi.fn(),
      put: vi.fn(),
      patch: vi.fn(),
      delete: vi.fn(),
      request: vi.fn(),
      interceptors: actual.http.interceptors
    }
  }
})

const mocked = await import('./http')
const httpMock = mocked.http as unknown as {
  get: ReturnType<typeof vi.fn>
  post: ReturnType<typeof vi.fn>
  put: ReturnType<typeof vi.fn>
  patch: ReturnType<typeof vi.fn>
  delete: ReturnType<typeof vi.fn>
}

describe('httpApi auth/users mapping', () => {
  beforeEach(() => {
    clearTokens()
    httpMock.get.mockReset()
    httpMock.post.mockReset()
    httpMock.put.mockReset()
    httpMock.patch.mockReset()
    httpMock.delete.mockReset()
  })

  it('login maps snake_case and stores tokens', async () => {
    httpMock.post.mockResolvedValue({
      data: {
        token_type: 'Bearer',
        access_token: 'AT',
        expires_in: 100,
        refresh_token: 'RT',
        user: {
          id: 'u1',
          username: 'simplebase2026',
          role: 'superadminl1',
          display_name: 'S',
          email: '',
          status: 'active',
          must_change_password: false,
          created_at: 't',
          last_login_at: 't2'
        }
      }
    })
    const pair = await httpApi.auth.login({ username: 'simplebase2026', password: 'x' })
    expect(pair.accessToken).toBe('AT')
    expect(pair.user.displayName).toBe('S')
    expect(pair.user.mustChangePassword).toBe(false)
    expect(getAccessToken()).toBe('AT')
  })

  it('refresh maps and stores tokens', async () => {
    httpMock.post.mockResolvedValue({
      data: {
        access_token: 'AT2',
        refresh_token: 'RT2',
        expires_in: 1,
        user: { id: 'u', username: 'a', role: 'user', status: 'active' }
      }
    })
    const pair = await httpApi.auth.refresh('RT')
    expect(pair.accessToken).toBe('AT2')
    expect(getAccessToken()).toBe('AT2')
  })

  it('logout clears tokens', async () => {
    setTokens('a', 'r')
    httpMock.post.mockResolvedValue({ data: undefined })
    await httpApi.auth.logout('r')
    expect(getAccessToken()).toBe('')
  })

  it('me maps projects', async () => {
    httpMock.get.mockResolvedValue({
      data: {
        id: 'u1',
        username: 'x',
        role: 'user',
        status: 'active',
        projects: [{ id: 'p1', name: 'P', owner: true }]
      }
    })
    const me = await httpApi.auth.me()
    expect(me.projects[0].id).toBe('p1')
    expect(me.projects[0].owner).toBe(true)
  })

  it('changePassword put', async () => {
    httpMock.put.mockResolvedValue({ data: undefined })
    await httpApi.auth.changePassword('o', 'n')
    expect(httpMock.put).toHaveBeenCalled()
  })

  it('users list/create/update/remove', async () => {
    httpMock.get.mockResolvedValue({
      data: {
        users: [
          {
            id: 'u1',
            username: 'a',
            role: 'admin',
            status: 'active',
            display_name: 'A',
            project_count: 2
          }
        ],
        next_cursor: 'c'
      }
    })
    const list = await httpApi.users.list()
    expect(list.users[0].projectCount).toBe(2)
    expect(list.nextCursor).toBe('c')

    httpMock.post.mockResolvedValue({
      data: { id: 'u2', username: 'b', role: 'user', status: 'active' }
    })
    const created = await httpApi.users.create({ username: 'b', password: '12345678', role: 'user' })
    expect(created.username).toBe('b')

    httpMock.patch.mockResolvedValue({
      data: { id: 'u2', username: 'b', role: 'admin', status: 'disabled' }
    })
    const updated = await httpApi.users.update('u2', { role: 'admin', status: 'disabled' })
    expect(updated.role).toBe('admin')
    expect(updated.status).toBe('disabled')

    httpMock.delete.mockResolvedValue({ data: undefined })
    await httpApi.users.remove('u2')
    expect(httpMock.delete).toHaveBeenCalled()
  })
})
