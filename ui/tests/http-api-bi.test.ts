import { beforeEach, describe, expect, it, vi } from 'vitest'

const http = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  patch: vi.fn(),
  delete: vi.fn()
}))

vi.mock('@/services/http', () => ({
  http,
  baseURL: 'http://api.test',
  getApiKey: () => 'test-key',
  getAccessToken: () => ''
}))

import { httpApi } from '@/services/http-api'
import { toAggregateParams, toAggregateResult } from '@/services/http-api-converters'
import type { BiAgentContext } from '@/services/types'

const pid = 'proj 1'

describe('toAggregateParams', () => {
  it('turns arrays and camelCase into the backend query string keys', () => {
    expect(toAggregateParams({ metrics: ['count', 'sum:amount'], groupBy: 'placed:day', series: 'status', filter: 'a=1', sort: '-count', limit: 20 })).toEqual({
      metrics: 'count,sum:amount', group_by: 'placed:day', series: 'status', filter: 'a=1', sort: '-count', limit: 20
    })
  })

  it('drops empty values instead of sending blank keys', () => {
    expect(toAggregateParams({ metrics: [], groupBy: '', series: undefined, filter: '', sort: undefined })).toEqual({})
    expect(toAggregateParams({})).toEqual({})
  })

  it('keeps limit 0 out only when undefined, not when numeric', () => {
    expect(toAggregateParams({ limit: 0 })).toEqual({ limit: 0 })
  })

  it('builds scatter points params', () => {
    expect(toAggregateParams({ mode: 'points', x: 'amount', y: 'qty', size: 'qty', series: 'status', limit: 500 })).toEqual({
      mode: 'points', x: 'amount', y: 'qty', size: 'qty', series: 'status', limit: 500
    })
  })
})

describe('toAggregateResult', () => {
  it('maps snake_case to camelCase', () => {
    expect(toAggregateResult({ columns: ['x', 'count'], rows: [['a', 1]], truncated: true, scanned: 12, duration_ms: 7, request_id: 'r1' })).toEqual({
      columns: ['x', 'count'], rows: [['a', 1]], truncated: true, scanned: 12, durationMs: 7, requestId: 'r1'
    })
  })

  it('is defensive about malformed bodies', () => {
    for (const bad of [undefined, null, 'oops', 5, []]) {
      expect(toAggregateResult(bad)).toEqual({ columns: [], rows: [], truncated: false, scanned: 0, durationMs: 0, requestId: '' })
    }
    expect(toAggregateResult({ columns: [1, 2], rows: 'x', truncated: 'yes' }).columns).toEqual(['1', '2'])
    expect(toAggregateResult({ columns: [], rows: 'x', truncated: 'yes' })).toMatchObject({ rows: [], truncated: false })
  })
})

describe('httpApi.db.aggregate', () => {
  beforeEach(() => {
    Object.values(http).forEach((fn) => fn.mockReset())
  })

  it('GETs the aggregate endpoint with encoded path segments and converted params', async () => {
    http.get.mockResolvedValueOnce({ data: { columns: ['count'], rows: [[3]], truncated: false, scanned: 3, duration_ms: 1, request_id: 'r' } })
    const result = await httpApi.db.aggregate(pid, 'db/1', 'my orders', { metrics: ['count'], groupBy: 'status' })
    const [url, config] = http.get.mock.calls[0]
    expect(String(url)).toMatch(/\/databases\/db%2F1\/data\/collections\/my%20orders\/aggregate$/)
    expect(config).toEqual({ params: { metrics: 'count', group_by: 'status' } })
    expect(result).toMatchObject({ columns: ['count'], rows: [[3]], scanned: 3, requestId: 'r' })
  })

  it('is a read: it never uses a write verb', async () => {
    http.get.mockResolvedValueOnce({ data: {} })
    await httpApi.db.aggregate(pid, 'db', 'c', { metrics: ['count'] })
    expect(http.post).not.toHaveBeenCalled()
    expect(http.put).not.toHaveBeenCalled()
    expect(http.patch).not.toHaveBeenCalled()
    expect(http.delete).not.toHaveBeenCalled()
  })

  it('propagates request failures', async () => {
    http.get.mockRejectedValueOnce(Object.assign(new Error('scan limit'), { code: 'filter_scan_limit_exceeded' }))
    await expect(httpApi.db.aggregate(pid, 'db', 'c', { metrics: ['count'] })).rejects.toMatchObject({ code: 'filter_scan_limit_exceeded' })
  })
})

describe('streamRun bi_context passthrough', () => {
  const bi: BiAgentContext = {
    database_id: '0b6f1c1e-7a3d-4a0e-9a53-2d6a7e2f9c11', collection: 'orders', preferred_type: 'stacked_bar',
    fields: [{ name: 'amount', type: 'number' }]
  }

  function captureBody() {
    const fetchMock = vi.fn().mockResolvedValue({ ok: false, status: 500, body: null, json: async () => ({}) })
    vi.stubGlobal('fetch', fetchMock)
    return fetchMock
  }

  it('sends bi_context in the request body (the body is a field allow-list, so it must be explicit)', async () => {
    const fetchMock = captureBody()
    httpApi.agentThreads.streamRun(pid, 'th', { content: '画图', mentions: [{ agent_id: 'a1' }], bi_context: bi }, { onError: vi.fn() })
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalled())
    const body = JSON.parse(fetchMock.mock.calls[0][1].body as string)
    expect(body.bi_context).toEqual(bi)
    expect(body).toMatchObject({ content: '画图', mentions: [{ agent_id: 'a1' }], stream: true })
    vi.unstubAllGlobals()
  })

  it('omits the key entirely when there is no bi_context', async () => {
    const fetchMock = captureBody()
    httpApi.agentThreads.streamRun(pid, 'th', { content: 'x', mentions: [] }, { onError: vi.fn() })
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalled())
    const body = JSON.parse(fetchMock.mock.calls[0][1].body as string)
    expect(body).not.toHaveProperty('bi_context')
    vi.unstubAllGlobals()
  })

  it('still sends retry and starter fields next to bi_context', async () => {
    const fetchMock = captureBody()
    httpApi.agentThreads.streamRun(pid, 'th', { content: '', mentions: [], retry_of_run_id: 'r9', bi_context: bi }, { onError: vi.fn() })
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalled())
    const body = JSON.parse(fetchMock.mock.calls[0][1].body as string)
    expect(body).toMatchObject({ retry_of_run_id: 'r9', bi_context: { collection: 'orders' } })
    expect(body).not.toHaveProperty('starter_stage')
    vi.unstubAllGlobals()
  })
})

describe('httpApi.databases.aggregateTable', () => {
  beforeEach(() => {
    Object.values(http).forEach((fn) => fn.mockReset())
  })

  it('GETs the table aggregate endpoint with an encoded table name and the same param conversion', async () => {
    http.get.mockResolvedValueOnce({ data: { columns: ['x', 'count'], rows: [['a', 1]], truncated: false, scanned: 4, duration_ms: 2, request_id: 'r' } })
    const result = await httpApi.databases.aggregateTable(pid, 'db/1', 'my table', { metrics: ['count', 'sum:amount'], groupBy: 'placed_at:day', filter: 'a=1', limit: 20 })
    const [url, config] = http.get.mock.calls[0]
    expect(String(url)).toMatch(/\/databases\/db%2F1\/schema\/tables\/my%20table\/aggregate$/)
    expect(config).toEqual({ params: { metrics: 'count,sum:amount', group_by: 'placed_at:day', filter: 'a=1', limit: 20 } })
    expect(result).toMatchObject({ columns: ['x', 'count'], rows: [['a', 1]], scanned: 4, requestId: 'r' })
  })

  it('is a read and never uses a write verb', async () => {
    http.get.mockResolvedValueOnce({ data: {} })
    await httpApi.databases.aggregateTable(pid, 'db', 't', { metrics: ['count'] })
    for (const verb of ['post', 'put', 'patch', 'delete'] as const) expect(http[verb]).not.toHaveBeenCalled()
  })

  it('builds scatter points params the same way as the collection endpoint', async () => {
    http.get.mockResolvedValueOnce({ data: {} })
    await httpApi.databases.aggregateTable(pid, 'db', 't', { mode: 'points', x: 'a', y: 'b', size: 'c', series: 'd', limit: 300 })
    expect(http.get.mock.calls[0][1]).toEqual({ params: { mode: 'points', x: 'a', y: 'b', size: 'c', series: 'd', limit: 300 } })
  })

  it('propagates request failures', async () => {
    http.get.mockRejectedValueOnce(Object.assign(new Error('nope'), { code: 'unknown_field' }))
    await expect(httpApi.databases.aggregateTable(pid, 'db', 't', { metrics: ['count'] })).rejects.toMatchObject({ code: 'unknown_field' })
  })
})

describe('httpApi.databases.tableRows filter', () => {
  beforeEach(() => {
    Object.values(http).forEach((fn) => fn.mockReset())
  })

  const body = { table: 't', columns: [], rows: [], limit: 20, offset: 0, total: 0 }

  it('omits the filter key entirely when there is no filter, so the request is unchanged', async () => {
    http.get.mockResolvedValueOnce({ data: body })
    await httpApi.databases.tableRows(pid, 'db', 't', { limit: 20, offset: 0 })
    expect(http.get.mock.calls[0][1]).toEqual({ params: { limit: 20, offset: 0 } })
    http.get.mockResolvedValueOnce({ data: body })
    await httpApi.databases.tableRows(pid, 'db', 't', { limit: 20, offset: 0, filter: '' })
    expect(http.get.mock.calls[1][1]).toEqual({ params: { limit: 20, offset: 0 } })
  })

  it('sends the filter when present', async () => {
    http.get.mockResolvedValueOnce({ data: body })
    await httpApi.databases.tableRows(pid, 'db', 't', { limit: 20, offset: 40, filter: 'kind="click"' })
    expect(http.get.mock.calls[0][1]).toEqual({ params: { limit: 20, offset: 40, filter: 'kind="click"' } })
  })
})

describe('streamRun bi_context source passthrough', () => {
  it('sends source=table inside bi_context untouched', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: false, status: 500, body: null, json: async () => ({}) })
    vi.stubGlobal('fetch', fetchMock)
    const ctx: BiAgentContext = { database_id: '0b6f1c1e-7a3d-4a0e-9a53-2d6a7e2f9c11', collection: 'events', source: 'table', fields: [{ name: 'id', type: 'number' }] }
    httpApi.agentThreads.streamRun(pid, 'th', { content: '画图', mentions: [], bi_context: ctx }, { onError: vi.fn() })
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalled())
    expect(JSON.parse(fetchMock.mock.calls[0][1].body as string).bi_context).toEqual(ctx)
    vi.unstubAllGlobals()
  })
})

describe('aggregate with_total and total mapping', () => {
  it('sends with_total only when asked, and maps the returned total', async () => {
    const { toAggregateParams, toAggregateResult } = await import('@/services/http-api-converters')
    expect(toAggregateParams({ metrics: ['count'], groupBy: 'status', withTotal: true })).toMatchObject({ with_total: 1 })
    expect(toAggregateParams({ metrics: ['count'], groupBy: 'status' })).not.toHaveProperty('with_total')
    const mapped = toAggregateResult({ columns: ['x', 'count'], rows: [['a', 1]], total: 42, truncated: true, scanned: 1 })
    expect(mapped.total).toBe(42)
    // 后端没给（或给了非数字）时不能变成 0：前端要能区分「总量是 0」与「没有总量」。
    expect(toAggregateResult({ columns: [], rows: [] }).total).toBeUndefined()
    expect('total' in toAggregateResult({ columns: [], rows: [], total: null })).toBe(false)
  })
})

