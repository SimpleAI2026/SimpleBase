import { beforeEach, describe, expect, it, vi } from 'vitest'

const http = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn(), request: vi.fn() }))
vi.mock('@/services/http', () => ({
  http,
  baseURL: 'http://api.test',
  getApiKey: () => 'key',
  getAccessToken: () => ''
}))
import { httpApi } from '@/services/http-api'

function ok(data: unknown) {
  return Promise.resolve({ data })
}

describe('capability gap HTTP mappings', () => {
  beforeEach(() => Object.values(http).forEach((fn) => fn.mockReset()))

  it('maps collections, schemas, document pages and writes', async () => {
    http.get.mockResolvedValueOnce(ok({ items: [{ name: 'users', field_count: 2, document_count: 3, registered: true }] }))
    expect((await httpApi.db.collectionItems('p', 'd'))[0]).toEqual({ name: 'users', schemaMode: 'json', fieldCount: 2, documentCount: 3, registered: true })
    http.get.mockResolvedValueOnce(ok({ rows: [{ id: 'r1', title: 'A' }], items: [{ id: 'r1', created_at: 'c', updated_at: 'u', data: { title: 'A' } }], next_cursor: 'n', total: 1 }))
    const page = await httpApi.db.documents('p', 'd', 'users', { filter: 'priority>=2', count: true })
    expect(page.items[0]).toEqual({ id: 'r1', createdAt: 'c', updatedAt: 'u', data: { title: 'A' } })
    expect(page.nextCursor).toBe('n')
    http.get.mockResolvedValueOnce(ok({ id: 'r1', created_at: 'c', data: { title: 'A' } }))
    expect((await httpApi.db.document('p', 'd', 'users', 'r/1')).updatedAt).toBe('c')
    http.patch.mockResolvedValueOnce(ok({ id: 'r1' }))
    await httpApi.db.patch('p', 'd', 'users', 'r1', { title: 'B' })
    http.post.mockResolvedValueOnce(ok({ items: [{ id: 'r2', created_at: 'c', data: {} }] }))
    expect((await httpApi.db.batch('p', 'd', 'users', [{}]))[0].id).toBe('r2')
    http.delete.mockResolvedValueOnce(ok(undefined))
    await httpApi.db.truncate('p', 'd', 'users')

    const schema = { id: 's', project_id: 'p', name: 'users', fields: [{ name: 'age', type: 'number', required: true, unique: true, options: { min: 1, max: 9, max_select: 2, max_size: 20, only_int: true, only_domains: ['a.com'] } }] }
    http.get.mockResolvedValueOnce(ok(schema))
    const mapped = await httpApi.db.getCollectionSchema('p', 'd', 'users')
    expect(mapped.projectId).toBe('p')
    expect(mapped.fields[0].options).toMatchObject({ min: 1, max: 9, maxSelect: 2, maxSize: 20, onlyInt: true, onlyDomains: ['a.com'] })
    http.put.mockResolvedValueOnce(ok({ invalid: [{ id: 'r1', field: 'age', reason: 'x' }], scanned: 1, truncated: true }))
    const dry = await httpApi.db.putCollectionSchema('p', 'd', { name: 'users', fields: [] }, true)
    expect(dry).toMatchObject({ scanned: 1, truncated: true })
    expect(http.put.mock.calls.at(-1)?.[2]?.params).toEqual({ dry_run: 1 })
    http.put.mockResolvedValueOnce(ok(schema))
    await httpApi.db.putCollectionSchema('p', 'd', { name: 'users', fields: [] })
    http.delete.mockResolvedValueOnce(ok(undefined))
    await httpApi.db.deleteCollectionSchema('p', 'd', 'users')
  })

  it('maps scoped keys, backups, files and log stats/export', async () => {
    http.post.mockResolvedValueOnce(ok({ id: 'k', project_id: 'p', permissions: ['database:read'], created_at: 'c', secret: 's' }))
    await httpApi.apiKeys.create('p', ['database:read'], { resources: ['collection:orders'], actions: ['read'], hiddenFields: ['secret'], readonlyFields: ['owner'], expiresAt: 'e' })
    expect(http.post.mock.calls[0][1].scope).toEqual({ resources: ['collection:orders'], actions: ['read'], hidden_fields: ['secret'], readonly_fields: ['owner'], expires_at: 'e' })

    const backup = { id: 'b', project_id: 'p', size_bytes: 2, sha256: 'x', engine: 'sqlite', format_version: 1, status: 'ready', created_at: 'c', expires_at: 'e' }
    http.get.mockResolvedValueOnce(ok({ items: [backup] }))
    expect((await httpApi.backups.list('p'))[0]).toMatchObject({ id: 'b', projectId: 'p', sizeBytes: 2, formatVersion: 1, expiresAt: 'e' })
    http.post.mockResolvedValueOnce(ok(backup))
    await httpApi.backups.create('p')
    http.post.mockResolvedValueOnce(ok(undefined))
    await httpApi.backups.restore('p', 'b')
    http.get.mockResolvedValueOnce(ok(new Blob(['x'])))
    await httpApi.backups.download('p', 'b')
    http.delete.mockResolvedValueOnce(ok(undefined))
    await httpApi.backups.remove('p', 'b')

    http.get.mockResolvedValueOnce(ok({ folders: ['docs/'], objects: [{ key: 'a', size: 1 }] }))
    expect(await httpApi.s3.listDetailed('p', 'docs/')).toEqual({ folders: ['docs/'], objects: [{ key: 'a', size: 1 }] })
    http.get.mockResolvedValueOnce(ok(new Blob(['x'])))
    await httpApi.s3.download('p', 'a')
    http.post.mockResolvedValueOnce(ok({ url: '/files/t', expires_at: 'e' }))
    expect(await httpApi.s3.createLink('p', 'a')).toEqual({ url: '/files/t', expiresAt: 'e' })

    http.get.mockResolvedValueOnce(ok({ items: [{ key: '2026-01-01T00:00:00Z', count: 2 }] }))
    expect(await httpApi.logs.stats('p', undefined, 'hour')).toEqual([{ key: '2026-01-01T00:00:00Z', count: 2 }])
    http.get.mockResolvedValueOnce(ok(new Blob(['id'])))
    await httpApi.logs.export('p')
    expect(http.get.mock.calls.at(-1)?.[1]?.responseType).toBe('blob')
  })

  it('normalizes empty and legacy response shapes', async () => {
    http.get.mockResolvedValueOnce(ok({}))
    expect((await httpApi.db.collectionItems('p', 'd')).length).toBe(0)
    http.get.mockResolvedValueOnce(ok({}))
    expect(await httpApi.db.documents('p', 'd', 'c')).toEqual({ rows: [], items: [], nextCursor: undefined, total: undefined })
    http.get.mockResolvedValueOnce(ok({}))
    expect((await httpApi.db.document('p', 'd', 'c', 'r')).updatedAt).toBe('')
    http.get.mockResolvedValueOnce(ok({ fields: [{ name: 'x', type: 'custom', options: {} }] }))
    expect((await httpApi.db.getCollectionSchema('p', 'd', 'c')).fields[0].options).toMatchObject({ maxSelect: undefined, onlyInt: false })
    http.get.mockResolvedValueOnce(ok({ invalid: [] }))
    expect(await httpApi.backups.list('p')).toEqual([])
    http.get.mockResolvedValueOnce(ok({}))
    expect(await httpApi.s3.listDetailed('p')).toEqual({ folders: [], objects: [] })
    http.get.mockResolvedValueOnce(ok({}))
    expect(await httpApi.logs.stats('p')).toEqual([])
    http.put.mockResolvedValueOnce(ok({}))
    expect(await httpApi.db.putCollectionSchema('p', 'd', { name: 'c', fields: [] })).toMatchObject({ name: '', fields: [] })
  })

  it('maps missing fields, malformed optional values and empty write results', async () => {
    http.get.mockResolvedValueOnce(ok({ items: [{}] }))
    expect(await httpApi.db.collectionItems('p', 'd')).toEqual([{ name: '', schemaMode: 'json', fieldCount: 0, documentCount: 0, registered: false }])

    http.get.mockResolvedValueOnce(ok({ items: [{ created_at: 'c' }], rows: {}, next_cursor: '', total: '1' }))
    expect(await httpApi.db.documents('p', 'd', 'c')).toEqual({
      rows: [], items: [{ id: '', createdAt: 'c', updatedAt: 'c', data: {} }], nextCursor: undefined, total: undefined
    })
    http.post.mockResolvedValueOnce(ok({}))
    expect(await httpApi.db.batch('p', 'd', 'c', [])).toEqual([])

    http.get.mockResolvedValueOnce(ok({ fields: [null, {
      options: { min: true, max: {}, pattern: 'x', values: [1], max_select: '2', max_size: '3', except_domains: ['bad.com'], only_domains: ['good.com'] }
    }] }))
    const schema = await httpApi.db.getCollectionSchema('p', 'd', 'c')
    expect(schema.fields[0]).toMatchObject({ name: '', type: 'text', options: { min: undefined, max: undefined } })
    expect(schema.fields[1].options).toMatchObject({ maxSelect: 2, maxSize: 3, values: ['1'], exceptDomains: ['bad.com'], onlyDomains: ['good.com'] })

    http.get.mockResolvedValueOnce(ok({ items: [null] }))
    expect(await httpApi.backups.list('p')).toEqual([{ id: '', projectId: '', sizeBytes: 0, sha256: '', engine: '', formatVersion: 0, status: '', createdAt: '', expiresAt: undefined }])
    http.post.mockResolvedValueOnce(ok(null))
    expect((await httpApi.backups.create('p')).id).toBe('')
    http.post.mockResolvedValueOnce(ok({}))
    expect(await httpApi.s3.createLink('p', 'a')).toEqual({ url: '', expiresAt: '' })
    http.get.mockResolvedValueOnce(ok({ items: [{}] }))
    expect(await httpApi.logs.stats('p')).toEqual([{ key: '', count: 0 }])
    http.post.mockResolvedValueOnce(ok({}))
    expect(await httpApi.llm.chat('p', { messages: [] })).toEqual({ content: '', usage: undefined, model: '', provider: '', finish_reason: undefined })
  })

  it('loads OpenAPI and preserves preview response status', async () => {
    http.get.mockResolvedValueOnce(ok({ openapi: '3.0.3', paths: {} }))
    expect((await httpApi.preview.document()).openapi).toBe('3.0.3')
    http.request.mockResolvedValueOnce({ status: 409, data: { error: { code: 'conflict' } } })
    expect(await httpApi.preview.request('POST', '/v1/test', { value: 1 })).toEqual({ status: 409, body: '{\n  "error": {\n    "code": "conflict"\n  }\n}' })
    expect(http.request.mock.calls[0][0]).toMatchObject({ method: 'POST', url: '/v1/test', data: { value: 1 } })
  })
})
