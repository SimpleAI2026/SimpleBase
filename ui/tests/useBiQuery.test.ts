import { beforeEach, describe, expect, it, vi } from 'vitest'
import { BiQueue, MAX_CONCURRENT_QUERIES, useBiQuery } from '@/composables/useBiQuery'
import { api, resetApiMocks } from '@/test/api-mock'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

const target = { projectId: 'p1', databaseId: 'db1', collection: 'orders' }
const result = (n: number) => ({ columns: ['count'], rows: [[n]], truncated: false, scanned: n, durationMs: 1, requestId: '' })

function deferred<T>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

describe('BiQueue', () => {
  it('never runs more than the limit at once and keeps FIFO order', async () => {
    const q = new BiQueue(2)
    const gates = Array.from({ length: 5 }, () => deferred<void>())
    const started: number[] = []
    let peak = 0
    const tasks = gates.map((g, i) => q.run(async () => {
      started.push(i)
      peak = Math.max(peak, q.inFlight)
      await g.promise
      return i
    }))
    await Promise.resolve()
    expect(q.inFlight).toBe(2)
    expect(q.queued).toBe(3)
    expect(started).toEqual([0, 1])
    gates[0].resolve()
    await tasks[0]
    await Promise.resolve()
    expect(started).toEqual([0, 1, 2])
    for (const g of gates) g.resolve()
    expect(await Promise.all(tasks)).toEqual([0, 1, 2, 3, 4])
    expect(peak).toBeLessThanOrEqual(2)
    expect(q.inFlight).toBe(0)
    expect(q.queued).toBe(0)
  })

  it('releases its slot when a task throws', async () => {
    const q = new BiQueue(1)
    await expect(q.run(async () => { throw new Error('boom') })).rejects.toThrow('boom')
    expect(q.inFlight).toBe(0)
    expect(await q.run(async () => 'ok')).toBe('ok')
  })

  it('defaults to four concurrent queries', () => {
    expect(MAX_CONCURRENT_QUERIES).toBe(4)
  })
})

describe('useBiQuery', () => {
  beforeEach(() => resetApiMocks())

  it('loads a result and forwards the query', async () => {
    api.db.aggregate.mockResolvedValue(result(3))
    const q = useBiQuery('failed', new BiQueue(4))
    const query = { metrics: ['count'], groupBy: 'status' }
    await q.run(target, query)
    expect(api.db.aggregate).toHaveBeenCalledWith('p1', 'db1', 'orders', query)
    expect(q.result.value).toEqual(result(3))
    expect(q.pending.value).toBe(false)
    expect(q.error.value).toBe('')
  })

  it('reports pending while in flight', async () => {
    const d = deferred<ReturnType<typeof result>>()
    api.db.aggregate.mockReturnValue(d.promise)
    const q = useBiQuery('failed', new BiQueue(4))
    const run = q.run(target, { metrics: ['count'] })
    expect(q.pending.value).toBe(true)
    d.resolve(result(1))
    await run
    expect(q.pending.value).toBe(false)
  })

  it('keeps the newest response when an older one resolves late', async () => {
    const old = deferred<ReturnType<typeof result>>()
    const fresh = deferred<ReturnType<typeof result>>()
    api.db.aggregate.mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise)
    const q = useBiQuery('failed', new BiQueue(4))
    const first = q.run(target, { metrics: ['count'] })
    const second = q.run(target, { metrics: ['count'] })
    fresh.resolve(result(2))
    await second
    old.resolve(result(1))
    await first
    expect(q.result.value).toEqual(result(2))
    expect(q.pending.value).toBe(false)
  })

  it('surfaces errors with the backend code and clears the stale result', async () => {
    api.db.aggregate.mockResolvedValueOnce(result(1))
    const q = useBiQuery('加载失败', new BiQueue(4))
    await q.run(target, { metrics: ['count'] })
    api.db.aggregate.mockRejectedValueOnce(Object.assign(new Error('filter scan limit exceeded'), { code: 'filter_scan_limit_exceeded', status: 400 }))
    await q.run(target, { metrics: ['count'] })
    expect(q.result.value).toBeNull()
    expect(q.error.value).toContain('filter scan limit exceeded')
    expect(q.errorCode.value).toBe('filter_scan_limit_exceeded')
    expect(q.pending.value).toBe(false)
  })

  it('falls back to the supplied message when the error has none', async () => {
    api.db.aggregate.mockRejectedValueOnce(new Error(''))
    const q = useBiQuery('加载失败', new BiQueue(4))
    await q.run(target, { metrics: ['count'] })
    expect(q.error.value).toBe('加载失败')
    api.db.aggregate.mockRejectedValueOnce('plain string')
    await q.run(target, { metrics: ['count'] })
    expect(q.error.value).toBe('加载失败')
    expect(q.errorCode.value).toBe('')
  })

  it('cancel drops an in-flight response', async () => {
    const d = deferred<ReturnType<typeof result>>()
    api.db.aggregate.mockReturnValue(d.promise)
    const q = useBiQuery('failed', new BiQueue(4))
    const run = q.run(target, { metrics: ['count'] })
    q.cancel()
    expect(q.pending.value).toBe(false)
    d.resolve(result(9))
    await run
    expect(q.result.value).toBeNull()
  })

  it('a late failure after cancel does not set an error', async () => {
    const d = deferred<ReturnType<typeof result>>()
    api.db.aggregate.mockReturnValue(d.promise)
    const q = useBiQuery('failed', new BiQueue(4))
    const run = q.run(target, { metrics: ['count'] })
    q.cancel()
    d.reject(new Error('late'))
    await run
    expect(q.error.value).toBe('')
  })

  it('shares the concurrency gate across queries', async () => {
    const queue = new BiQueue(1)
    const gate = deferred<ReturnType<typeof result>>()
    api.db.aggregate.mockReturnValueOnce(gate.promise).mockResolvedValueOnce(result(2))
    const a = useBiQuery('f', queue)
    const b = useBiQuery('f', queue)
    const ra = a.run(target, { metrics: ['count'] })
    const rb = b.run(target, { metrics: ['count'] })
    await Promise.resolve()
    expect(api.db.aggregate).toHaveBeenCalledTimes(1)
    gate.resolve(result(1))
    await Promise.all([ra, rb])
    expect(api.db.aggregate).toHaveBeenCalledTimes(2)
    expect(b.result.value).toEqual(result(2))
  })

  it('a stale queued query is skipped: it never reaches the API and hands its slot on', async () => {
    const queue = new BiQueue(1)
    const gate = deferred<ReturnType<typeof result>>()
    api.db.aggregate.mockReturnValueOnce(gate.promise).mockResolvedValueOnce(result(3))
    const a = useBiQuery('f', queue)
    const b = useBiQuery('f', queue)
    const c = useBiQuery('f', queue)
    const ra = a.run(target, { metrics: ['count'] })
    // b 排队后又被作废（比如用户换了筛选）：轮到它时不能再发请求。
    const rb = b.run(target, { metrics: ['count'], filter: 'old' })
    b.cancel()
    const rc = c.run(target, { metrics: ['count'], filter: 'new' })
    gate.resolve(result(1))
    await Promise.all([ra, rb, rc])
    expect(api.db.aggregate).toHaveBeenCalledTimes(2)
    expect(api.db.aggregate.mock.calls.map((call) => call[3].filter)).toEqual([undefined, 'new'])
    expect(b.result.value).toBeNull()
    expect(b.error.value).toBe('')
    expect(c.result.value).toEqual(result(3))
    expect(queue.inFlight).toBe(0)
    expect(queue.queued).toBe(0)
  })

  it('a superseded run of the same query replaces its queued predecessor instead of doubling the load', async () => {
    const queue = new BiQueue(1)
    const gate = deferred<ReturnType<typeof result>>()
    api.db.aggregate.mockReturnValueOnce(gate.promise).mockResolvedValue(result(5))
    const blocker = useBiQuery('f', queue)
    const q = useBiQuery('f', queue)
    const rBlock = blocker.run(target, { metrics: ['count'] })
    const first = q.run(target, { metrics: ['count'], filter: 'a' })
    const second = q.run(target, { metrics: ['count'], filter: 'b' })
    gate.resolve(result(1))
    await Promise.all([rBlock, first, second])
    expect(api.db.aggregate.mock.calls.map((call) => call[3].filter)).toEqual([undefined, 'b'])
  })
})

describe('useBiQuery routes by data source kind', () => {
  beforeEach(() => resetApiMocks())

  it('a collection target (default or explicit) hits the collection endpoint', async () => {
    api.db.aggregate.mockResolvedValue(result(3))
    const q = useBiQuery('失败', new BiQueue(4))
    await q.run(target, { metrics: ['count'] })
    await q.run({ ...target, kind: 'collection' }, { metrics: ['count'] })
    expect(api.db.aggregate).toHaveBeenCalledTimes(2)
    expect(api.databases.aggregateTable).not.toHaveBeenCalled()
  })

  it('a table target hits the table endpoint with the table name, and never the collection one', async () => {
    api.databases.aggregateTable.mockResolvedValue(result(7))
    const q = useBiQuery('失败', new BiQueue(4))
    await q.run({ ...target, collection: 'events', kind: 'table' }, { metrics: ['count'], groupBy: 'kind' })
    expect(api.databases.aggregateTable).toHaveBeenCalledWith('p1', 'db1', 'events', { metrics: ['count'], groupBy: 'kind' })
    expect(api.db.aggregate).not.toHaveBeenCalled()
    expect(q.result.value?.rows).toEqual([[7]])
  })

  it('table failures surface the same way (message, code, cleared result)', async () => {
    api.databases.aggregateTable.mockResolvedValue(result(1))
    const q = useBiQuery('失败', new BiQueue(4))
    await q.run({ ...target, kind: 'table' }, { metrics: ['count'] })
    api.databases.aggregateTable.mockRejectedValueOnce(Object.assign(new Error('too many rows'), { code: 'filter_scan_limit_exceeded' }))
    await q.run({ ...target, kind: 'table' }, { metrics: ['count'] })
    expect(q.error.value).toBe('too many rows')
    expect(q.errorCode.value).toBe('filter_scan_limit_exceeded')
    expect(q.result.value).toBeNull()
  })

  it('a stale table response is discarded after the target changes (token race)', async () => {
    const slow = deferred<ReturnType<typeof result>>()
    api.databases.aggregateTable.mockReturnValueOnce(slow.promise)
    const q = useBiQuery('失败', new BiQueue(4))
    const first = q.run({ ...target, collection: 'old', kind: 'table' }, { metrics: ['count'] })
    api.databases.aggregateTable.mockResolvedValueOnce(result(2))
    await q.run({ ...target, collection: 'new', kind: 'table' }, { metrics: ['count'] })
    slow.resolve(result(99))
    await first
    expect(q.result.value?.rows).toEqual([[2]])
  })
})
