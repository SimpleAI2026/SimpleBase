import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useBiDetail, DETAIL_PAGE_SIZE } from '@/composables/useBiDetail'
import { api, resetApiMocks } from '@/test/api-mock'
import type { BiSource } from '@/utils/bi-source'
import type { CollectionField } from '@/services/types'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

const field = (name: string): CollectionField => ({ name, type: 'text', required: false, unique: false, options: {}, hidden: false, readonly: false })

function setup(initial: BiSource | null, filter = '') {
  const state = { projectId: 'p1', databaseId: 'db1', source: initial, filter }
  const detail = useBiDetail({ scope: () => state, fields: () => [field('title')], filter: () => state.filter })
  return { state, detail }
}

function deferred<T>() {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((res) => { resolve = res })
  return { promise, resolve }
}

describe('useBiDetail', () => {
  beforeEach(() => resetApiMocks())

  it('does nothing without a project, database or source', async () => {
    const { detail } = setup(null)
    await detail.load()
    expect(api.db.documents).not.toHaveBeenCalled()
    expect(api.databases.tableRows).not.toHaveBeenCalled()
    expect(detail.loading.value).toBe(false)
  })

  it('collection: calls the documents endpoint and normalises items to columns + rows', async () => {
    api.db.documents.mockResolvedValue({ rows: [], total: 41, items: [{ id: 'r1', createdAt: 'c', updatedAt: 'u', data: { title: 'A' } }] })
    const { detail } = setup({ kind: 'collection', name: 'tasks' }, 'a=1')
    await detail.load()
    expect(api.db.documents).toHaveBeenCalledWith('p1', 'db1', 'tasks', { limit: DETAIL_PAGE_SIZE, offset: 0, filter: 'a=1', count: true })
    expect(detail.columns.value).toEqual(['id', 'created_at', 'updated_at', 'title'])
    expect(detail.rows.value[0]).toMatchObject({ id: 'r1', title: 'A' })
    expect(detail.total.value).toBe(41)
    expect(detail.mono.value).toEqual(['id', 'created_at', 'updated_at'])
  })

  it('collection: a missing total falls back to offset + loaded count', async () => {
    api.db.documents.mockResolvedValue({ rows: [], items: [{ id: 'r1', createdAt: 'c', updatedAt: 'u', data: {} }] })
    const { detail } = setup({ kind: 'collection', name: 'tasks' })
    detail.page.value = 3
    await detail.load()
    expect(detail.total.value).toBe(2 * DETAIL_PAGE_SIZE + 1)
  })

  it('table: calls the table rows endpoint; an empty filter is sent as undefined', async () => {
    api.databases.tableRows.mockResolvedValue({ table: 'events', columns: [{ name: 'id', type: 'BIGINT' }, { name: 'kind', type: 'VARCHAR' }], rows: [[1, 'click']], limit: 20, offset: 0, total: 9 })
    const { detail } = setup({ kind: 'table', name: 'events' })
    await detail.load()
    expect(api.databases.tableRows).toHaveBeenCalledWith('p1', 'db1', 'events', { limit: DETAIL_PAGE_SIZE, offset: 0, filter: undefined })
    expect(api.db.documents).not.toHaveBeenCalled()
    expect(detail.columns.value).toEqual(['id', 'kind'])
    expect(detail.rows.value).toEqual([{ id: 1, kind: 'click' }])
    expect(detail.total.value).toBe(9)
    expect(detail.mono.value).toEqual([])
  })

  it('table: passes the global filter and the page offset', async () => {
    api.databases.tableRows.mockResolvedValue({ table: 't', columns: [], rows: [], limit: 20, offset: 40, total: 0 })
    const { detail } = setup({ kind: 'table', name: 't' }, 'kind="click"')
    detail.goTo(3)
    await Promise.resolve()
    expect(api.databases.tableRows).toHaveBeenCalledWith('p1', 'db1', 't', { limit: 20, offset: 40, filter: 'kind="click"' })
  })

  it('failure keeps the message and a blank message falls back to the generic one', async () => {
    api.databases.tableRows.mockRejectedValueOnce(new Error('boom'))
    const { detail } = setup({ kind: 'table', name: 't' })
    await detail.load()
    expect(detail.error.value).toBe('boom')
    api.databases.tableRows.mockRejectedValueOnce(new Error(''))
    await detail.load()
    expect(detail.error.value).toBe('数据加载失败')
    expect(detail.loading.value).toBe(false)
  })

  it('discards a late response after the source changed (stale-response guard)', async () => {
    const slow = deferred<unknown>()
    api.databases.tableRows.mockReturnValueOnce(slow.promise as never)
    const { state, detail } = setup({ kind: 'table', name: 'old' })
    const pending = detail.load()
    state.source = { kind: 'table', name: 'new' }
    api.databases.tableRows.mockResolvedValueOnce({ table: 'new', columns: [{ name: 'a', type: 'INT' }], rows: [[1]], limit: 20, offset: 0, total: 1 })
    await detail.load()
    slow.resolve({ table: 'old', columns: [{ name: 'z', type: 'INT' }], rows: [['STALE']], limit: 20, offset: 0, total: 1 })
    await pending
    expect(detail.columns.value).toEqual(['a'])
    expect(JSON.stringify(detail.rows.value)).not.toContain('STALE')
  })

  it('a same-named collection and table are different sources for the stale guard', async () => {
    const slow = deferred<unknown>()
    api.db.documents.mockReturnValueOnce(slow.promise as never)
    const { state, detail } = setup({ kind: 'collection', name: 'events' })
    const pending = detail.load()
    state.source = { kind: 'table', name: 'events' }
    api.databases.tableRows.mockResolvedValueOnce({ table: 'events', columns: [{ name: 'k', type: 'INT' }], rows: [[1]], limit: 20, offset: 0, total: 1 })
    await detail.load()
    slow.resolve({ rows: [], total: 1, items: [{ id: 'STALE', createdAt: '', updatedAt: '', data: {} }] })
    await pending
    expect(detail.columns.value).toEqual(['k'])
  })

  it('clear() wipes the table, resets the page and invalidates an in-flight request', async () => {
    const slow = deferred<unknown>()
    api.db.documents.mockReturnValueOnce(slow.promise as never)
    const { detail } = setup({ kind: 'collection', name: 'tasks' })
    detail.page.value = 4
    const pending = detail.load()
    detail.clear()
    slow.resolve({ rows: [], total: 5, items: [{ id: 'x', createdAt: '', updatedAt: '', data: {} }] })
    await pending
    expect(detail.columns.value).toEqual([])
    expect(detail.rows.value).toEqual([])
    expect(detail.total.value).toBe(0)
    expect(detail.page.value).toBe(1)
    expect(detail.loading.value).toBe(false)
  })

  it('reset() only rewinds the page', () => {
    const { detail } = setup({ kind: 'collection', name: 'tasks' })
    detail.page.value = 5
    detail.reset()
    expect(detail.page.value).toBe(1)
  })
})
