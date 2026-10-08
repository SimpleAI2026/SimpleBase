import { beforeEach, describe, expect, it, vi } from 'vitest'

const { requestUse, responseUse, axiosPost, instanceRequest } = vi.hoisted(() => ({
  requestUse: vi.fn(),
  responseUse: vi.fn(),
  axiosPost: vi.fn(),
  instanceRequest: vi.fn()
}))

vi.mock('axios', () => ({
  default: {
    post: axiosPost,
    create: () => ({
      request: instanceRequest,
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
    axiosPost.mockReset()
    instanceRequest.mockReset()
    vi.resetModules()
    localStorage.clear()
  })

  it('reads and writes the API key', async () => {
    const httpMod = await import('@/services/http')
    expect(httpMod.getApiKey()).toBe('sb_live_dev_key_12345')
    httpMod.setApiKey('k2')
    expect(httpMod.getApiKey()).toBe('k2')
    expect(httpMod.baseURL).toBeDefined()
  })

  it('injects Authorization and rejects HTML success bodies', async () => {
    await import('@/services/http')
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
    vi.doMock('@/stores/auth', () => ({
      useAuthStore: () => ({ markUnauthorized: mark })
    }))
    const httpMod = await import('@/services/http')
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

  it('refreshes the access token once and retries the request on 401', async () => {
    const httpMod = await import('@/services/http')
    httpMod.setTokens('old-access', 'old-refresh')
    axiosPost.mockResolvedValueOnce({ data: { access_token: 'new-access' } })
    instanceRequest.mockResolvedValueOnce({ data: { ok: true } })
    const errFn = responseUse.mock.calls[1][1]
    const res = await errFn({ response: { status: 401 }, config: {} })
    expect(res).toEqual({ data: { ok: true } })
    expect(axiosPost.mock.calls[0][1]).toEqual({ refresh_token: 'old-refresh' })
    expect(instanceRequest.mock.calls[0][0].headers.Authorization).toBe('Bearer new-access')
    // refresh 响应未返回新 refresh_token 时沿用旧值
    expect(httpMod.getRefreshToken()).toBe('old-refresh')
    expect(httpMod.getAccessToken()).toBe('new-access')
  })

  it('marks unauthorized when refresh fails or is unavailable', async () => {
    const mark = vi.fn()
    vi.doMock('@/stores/auth', () => ({
      useAuthStore: () => ({ markUnauthorized: mark })
    }))
    const httpMod = await import('@/services/http')
    const extra = vi.fn()
    httpMod.setUnauthorizedHandler(extra)
    const errFn = responseUse.mock.calls[1][1]

    httpMod.setTokens('a1', 'r1')
    axiosPost.mockRejectedValueOnce(new Error('refresh down'))
    await expect(errFn({ response: { status: 401, data: { message: 'expired' } } })).rejects.toThrow('expired')
    expect(httpMod.getAccessToken()).toBe('')
    expect(extra).toHaveBeenCalledTimes(1)

    httpMod.setTokens('a2', 'r2')
    axiosPost.mockResolvedValueOnce({ data: {} })
    await expect(errFn({ response: { status: 401 }, config: {} })).rejects.toThrow('网络请求失败')
    expect(extra).toHaveBeenCalledTimes(2)

    localStorage.setItem('sb_access_token', 'only-access')
    localStorage.removeItem('sb_refresh_token')
    await expect(errFn({ response: { status: 401 }, config: { __retried: false } })).rejects.toThrow()
    expect(axiosPost).toHaveBeenCalledTimes(2)
    await vi.waitFor(() => expect(mark).toHaveBeenCalled())
  })

  it('shares one in-flight refresh between concurrent 401s', async () => {
    const httpMod = await import('@/services/http')
    httpMod.setTokens('a', 'r')
    let resolve!: (v: unknown) => void
    axiosPost.mockReturnValueOnce(new Promise((r) => { resolve = r }))
    instanceRequest.mockResolvedValue({ data: 1 })
    const errFn = responseUse.mock.calls[1][1]
    const p1 = errFn({ response: { status: 401 }, config: {} })
    const p2 = errFn({ response: { status: 401 }, config: { headers: { X: '1' } } })
    resolve({ data: { access_token: 'n', refresh_token: 'nr' } })
    await Promise.all([p1, p2])
    expect(axiosPost).toHaveBeenCalledTimes(1)
    expect(instanceRequest).toHaveBeenCalledTimes(2)
    expect(httpMod.getRefreshToken()).toBe('nr')
  })
})
