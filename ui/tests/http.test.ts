import { beforeEach, describe, expect, it, vi } from 'vitest'

const { requestUse, responseUse } = vi.hoisted(() => ({
  requestUse: vi.fn(),
  responseUse: vi.fn()
}))

vi.mock('axios', () => ({
  default: {
    create: () => ({
      interceptors: {
        request: { use: requestUse },
        response: { use: responseUse }
      }
    })
  }
}))

describe('http helpers and interceptors', () => {
  beforeEach(() => {
    requestUse.mockReset()
    responseUse.mockReset()
    vi.resetModules()
    localStorage.clear()
  })

  it('reads and writes the API key', async () => {
    const httpMod = await import('./http')
    expect(httpMod.getApiKey()).toBe('sb_live_dev_key_12345')
    httpMod.setApiKey('k2')
    expect(httpMod.getApiKey()).toBe('k2')
    expect(httpMod.baseURL).toBeDefined()
  })

  it('injects Authorization and rejects HTML success bodies', async () => {
    await import('./http')
    const reqFn = requestUse.mock.calls[0][0]
    const cfg = await reqFn({ headers: {} })
    expect(cfg.headers.Authorization).toMatch(/^Bearer /)

    const okFn = responseUse.mock.calls[0][0]
    await expect(okFn({ headers: { 'content-type': 'text/html' }, data: '<html/>' })).rejects.toThrow(
      /HTML/
    )
    expect(await okFn({ headers: { 'content-type': 'application/json' }, data: { ok: 1 } })).toEqual({
      headers: { 'content-type': 'application/json' },
      data: { ok: 1 }
    })
    expect(await okFn({ headers: {}, data: { ok: 1 } })).toEqual({ headers: {}, data: { ok: 1 } })

    const reqNoHeaders = requestUse.mock.calls[0][0]
    const cfg2 = await reqNoHeaders({})
    expect(cfg2.headers.Authorization).toMatch(/^Bearer /)

    const passthrough = responseUse.mock.calls[1][0]
    const r = { data: { ok: true } }
    expect(await passthrough(r)).toBe(r)
  })

  it('normalizes API errors and notifies 401 handlers', async () => {
    const mark = vi.fn()
    vi.doMock('../stores/auth', () => ({
      useAuthStore: () => ({ markUnauthorized: mark })
    }))
    const httpMod = await import('./http')
    const extra = vi.fn()
    httpMod.setUnauthorizedHandler(extra)
    const errFn = responseUse.mock.calls[1][1]
    await expect(
      errFn({
        response: { status: 401, data: { error: { message: 'bad key' } } },
        message: 'x'
      })
    ).rejects.toThrow('bad key')
    expect(extra).toHaveBeenCalled()
    await expect(errFn({ message: 'network down' })).rejects.toThrow('network down')
    await expect(errFn({ response: { status: 500, data: { message: 'oops' } } })).rejects.toThrow(
      'oops'
    )
    await expect(errFn({})).rejects.toThrow('网络请求失败')
  })
})
