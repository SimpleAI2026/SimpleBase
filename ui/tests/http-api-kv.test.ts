import { beforeEach, describe, expect, it, vi } from 'vitest'

const http = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  patch: vi.fn(),
  delete: vi.fn(),
  request: vi.fn()
}))

vi.mock('@/services/http', () => ({
  http,
  baseURL: 'http://api.test',
  getApiKey: () => 'test-key'
}))

import { httpApi } from '@/services/http-api'

const pid = 'proj 1'

describe('httpApi.kv (项目级单端点)', () => {
  beforeEach(() => {
    Object.values(http).forEach((fn) => fn.mockReset())
    Object.values(http).forEach((fn) => fn.mockResolvedValue({ data: {} }))
  })

  it('exec：cmd 与类型化写入都打到 POST /v1/projects/:pid/kv', async () => {
    await httpApi.kv.exec(pid, { type: 'cmd', argvs: ['GET', 'k'] })
    expect(http.post.mock.calls[0][0]).toBe('/v1/projects/proj%201/kv')
    expect(http.post.mock.calls[0][1]).toEqual({ type: 'cmd', argvs: ['GET', 'k'] })

    await httpApi.kv.exec(pid, {
      type: 'String',
      args: { key: 'k', value: 'v', ttl_ms: 100 }
    })
    expect(http.post.mock.calls[1][0]).toBe('/v1/projects/proj%201/kv')
    expect(http.post.mock.calls[1][1]).toEqual({
      type: 'String',
      args: { key: 'k', value: 'v', ttl_ms: 100 }
    })
  })

  it('exec 返回响应 body（Redis 回复的 JSON 编码）', async () => {
    http.post.mockResolvedValueOnce({ data: 'OK' })
    expect(await httpApi.kv.exec(pid, { type: 'cmd', argvs: ['SET', 'k', 'v'] })).toBe('OK')

    http.post.mockResolvedValueOnce({ data: null })
    expect(await httpApi.kv.exec(pid, { type: 'cmd', argvs: ['GET', 'missing'] })).toBeNull()

    http.post.mockResolvedValueOnce({ data: ['0', ['a:1']] })
    expect(await httpApi.kv.exec(pid, { type: 'cmd', argvs: ['SCAN', '0'] })).toEqual([
      '0',
      ['a:1']
    ])
  })

  it('execBatch：逐条顺序执行', async () => {
    ;(http.post as any)
      .mockImplementationOnce(() => Promise.resolve({ data: 1 }))
      .mockImplementationOnce(() => Promise.resolve({ data: 2 }))

    const out = await httpApi.kv.execBatch(pid, [
      { type: 'cmd', argvs: ['INCR', 'a'] },
      { type: 'cmd', argvs: ['INCR', 'a'] }
    ])
    expect(out).toEqual([1, 2])
    expect(http.post).toHaveBeenCalledTimes(2)
  })
})
