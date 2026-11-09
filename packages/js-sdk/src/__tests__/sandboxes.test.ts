import { describe, expect, it, vi } from 'vitest'
import { createClient } from '../index.js'

describe('sandboxes API', () => {
  it('encodes project, sandbox and file paths and injects idempotency key', async () => {
    const fetchMock = vi.fn(async (_url: string, opts: RequestInit) => {
      if (opts.method === 'POST') return new Response(JSON.stringify({ id: 'x/y', status: 'pending' }))
      if (opts.method === 'GET') return new Response(JSON.stringify({ content: 'ok', encoding: 'utf8', truncated: false }))
      return new Response(null, { status: 204 })
    })
    const client = createClient({ url: 'http://example.test', projectId: 'p/a', apiKey: 'key', fetch: fetchMock as unknown as typeof fetch })
    await client.sandboxes.create({ name: 'test' }, 'idem-key')
    await client.sandboxes.files.read('x/y', '/workspace/a b.txt')
    await client.sandboxes.files.write('x/y', '/workspace/a b.txt', 'hello')
    const first = fetchMock.mock.calls[0]
    expect(first[0]).toContain('/v1/projects/p%2Fa/sandboxes')
    expect((first[1].headers as Record<string, string>)['Idempotency-Key']).toBe('idem-key')
    expect(fetchMock.mock.calls[1][0]).toContain('/x%2Fy/files/content?path=%2Fworkspace%2Fa+b.txt')
    expect(fetchMock.mock.calls[2][1].body).toBe(JSON.stringify({ content: 'hello' }))
  })

  it('returns exit codes and maps domain errors', async () => {
    const fetchMock = vi.fn(async (url: string) => {
      if (url.endsWith('/exec')) return new Response(JSON.stringify({ exit_code: 3, stdout: '', stderr: '', timed_out: false }), { status: 200 })
      return new Response(JSON.stringify({ error: { code: 'sandbox_invalid_spec', message: 'invalid spec' } }), { status: 400 })
    })
    const client = createClient({ url: 'http://example.test', projectId: 'p', apiKey: 'key', fetch: fetchMock as unknown as typeof fetch })
    const result = await client.sandboxes.exec('x', { command: 'exit 3' })
    expect(result.exit_code).toBe(3)
    await expect(client.sandboxes.run({ command: '' })).rejects.toMatchObject({ code: 'sandbox_invalid_spec', status: 400 })
  })

  it('paginates the list with cursor', async () => {
    const fetchMock = vi.fn(async (url: string) => new Response(JSON.stringify(
      url.includes('cursor=a') ? { sandboxes: [{ id: 'b' }] } : { sandboxes: [{ id: 'a' }], next_cursor: 'a' }
    )))
    const client = createClient({ url: 'http://example.test', projectId: 'p', apiKey: 'key', fetch: fetchMock as unknown as typeof fetch })
    const first = await client.sandboxes.listPage({ status: 'running', limit: 1 })
    expect(first.next_cursor).toBe('a')
    const second = await client.sandboxes.listPage({ limit: 1, cursor: first.next_cursor })
    expect(second.sandboxes[0].id).toBe('b')
    expect(second.next_cursor).toBeUndefined()
    expect((await client.sandboxes.list()).map((s) => s.id)).toEqual(['a'])
    expect(fetchMock.mock.calls[0][0]).toContain('status=running')
    expect(fetchMock.mock.calls[0][0]).not.toContain('cursor')
    expect(fetchMock.mock.calls[1][0]).toContain('cursor=a')
  })
})
