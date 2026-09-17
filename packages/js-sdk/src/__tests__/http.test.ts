import { describe, it, expect, vi } from 'vitest'
import { createClient } from '../index.js'
import { SimpleBaseError } from '../errors.js'

describe('createClient', () => {
  it('requires url/apiKey/projectId', () => {
    expect(() => createClient({ url: '', apiKey: 'k', projectId: 'p' } as any)).toThrow(/url/)
  })

  it('injects Authorization header', async () => {
    const fetchMock = vi.fn(async () => {
      return new Response(JSON.stringify({ databases: [] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    })
    const sb = createClient({
      url: 'http://example.test',
      apiKey: 'sb_live_dev_key_12345',
      projectId: '00000000-0000-0000-0000-000000000002',
      fetch: fetchMock as unknown as typeof fetch
    })
    await sb.databases.list()
    expect(fetchMock).toHaveBeenCalled()
    const call = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    const headers = call[1].headers as Record<string, string>
    expect(headers.Authorization).toBe('Bearer sb_live_dev_key_12345')
  })

  it('maps error JSON to SimpleBaseError', async () => {
    const fetchMock = vi.fn(async () => {
      return new Response(JSON.stringify({ code: 'invalid_api_key', message: 'invalid api key' }), {
        status: 401,
        headers: { 'Content-Type': 'application/json' }
      })
    })
    const sb = createClient({
      url: 'http://example.test',
      apiKey: 'bad',
      projectId: 'p',
      fetch: fetchMock as unknown as typeof fetch
    })
    await expect(sb.databases.list()).rejects.toMatchObject({
      name: 'SimpleBaseError',
      code: 'invalid_api_key',
      status: 401
    } satisfies Partial<SimpleBaseError>)
  })

  it('requires databaseId for sql helpers', () => {
    const sb = createClient({
      url: 'http://example.test',
      apiKey: 'k',
      projectId: 'p'
    })
    expect(() => sb.sql.query('select 1')).toThrow(/databaseId/)
  })
})
