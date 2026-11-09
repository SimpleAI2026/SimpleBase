import { describe, expect, it, vi } from 'vitest'
import { createClient } from '../index.js'

describe('collection transfer', () => {
  it('imports raw NDJSON without JSON wrapping', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ dry_run: false, scanned: 1, imported: 1 }), {
      status: 201,
      headers: { 'Content-Type': 'application/json' }
    }))
    const client = createClient({ url: 'http://example.test', apiKey: 'k', projectId: 'p', fetch: fetchMock as unknown as typeof fetch })
    const result = await client.database('db').collection('tasks').import({ format: 'ndjson', body: '{"title":"one"}\n' })
    expect(result.imported).toBe(1)
    const [url, request] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('http://example.test/v1/projects/p/databases/db/data/collections/tasks/import')
    expect(request.body).toBe('{"title":"one"}\n')
    expect((request.headers as Record<string, string>)['Content-Type']).toBe('application/x-ndjson')
  })

  it('returns the streaming export Response and propagates errors', async () => {
    const fetchMock = vi.fn(async () => new Response('id,title\n1,one\n', {
      status: 200,
      headers: { 'Content-Type': 'text/csv', 'Content-Disposition': 'attachment; filename="tasks.csv"' }
    }))
    const client = createClient({ url: 'http://example.test', apiKey: 'k', projectId: 'p', fetch: fetchMock as unknown as typeof fetch })
    const response = await client.database('db').collection('tasks').export({ format: 'csv', fields: ['id', 'title'] })
    expect(await response.text()).toContain('1,one')
    expect(response.headers.get('content-disposition')).toContain('tasks.csv')
    const [url] = fetchMock.mock.calls[0] as unknown as [string]
    expect(url).toContain('format=csv')
    expect(url).toContain('fields=id%2Ctitle')

    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ code: 'invalid_filter', message: 'bad filter' }), { status: 400 }))
    await expect(client.database('db').collection('tasks').export({ format: 'ndjson' })).rejects.toMatchObject({ code: 'invalid_filter', status: 400 })
  })

  it('dry-runs a logical view at the collection metadata endpoint', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ columns: ['id', 'data'], fields: [], items: [], scanned: 0 }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' }
    }))
    const client = createClient({ url: 'http://example.test', apiKey: 'k', projectId: 'p', fetch: fetchMock as unknown as typeof fetch })
    await client.database('db').collection('active').dryRunView('SELECT id, data, created_at, updated_at FROM tasks')
    const [url, request] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('http://example.test/v1/projects/p/databases/db/data/collections/meta/dry-run-view')
    expect(request.method).toBe('POST')
    expect(JSON.parse(String(request.body))).toMatchObject({ name: 'active' })
  })
})
