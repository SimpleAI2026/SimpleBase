import { describe, expect, it, vi } from 'vitest'
import { createClient } from '../index.js'

describe('storage site API', () => {
  it('reads site mode and toggles public paths', async () => {
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      const path = String(url)
      if (path.endsWith('/s3/site')) {
        return new Response(
          JSON.stringify({ baseUrl: 'http://example.test/sites/p', publishable: true, mode: 'path' }),
          { status: 200 }
        )
      }
      if (path.endsWith('/s3/public') && (init?.method || 'GET') === 'GET') {
        return new Response(
          JSON.stringify({
            site: { publishable: true, mode: 'path' },
            paths: [{ path: 'web/', kind: 'folder', publicUrl: 'http://example.test/sites/p/web/' }]
          }),
          { status: 200 }
        )
      }
      if (path.endsWith('/s3/public') && init?.method === 'PUT') {
        return new Response(JSON.stringify({ public: true, publicUrl: 'http://example.test/sites/p/web/' }), {
          status: 200
        })
      }
      return new Response('{}', { status: 200 })
    })
    const sb = createClient({
      url: 'http://example.test',
      apiKey: 'k',
      projectId: 'p',
      fetch: fetchMock as unknown as typeof fetch
    })

    const site = await sb.storage.getSite()
    expect(site.mode).toBe('path')
    expect(site.publishable).toBe(true)

    const publicState = await sb.storage.listPublic()
    expect(publicState.paths[0].kind).toBe('folder')

    const updated = await sb.storage.setPublic({ key: 'web/', kind: 'folder', public: true })
    expect(updated.public).toBe(true)

    const putCall = fetchMock.mock.calls[2] as unknown as [string, RequestInit]
    expect(String(putCall[0])).toContain('/s3/public')
    expect(putCall[1].method).toBe('PUT')
    expect(JSON.parse(String(putCall[1].body))).toEqual({ key: 'web/', kind: 'folder', public: true })
  })
})
