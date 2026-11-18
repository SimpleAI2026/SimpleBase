import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useBiDataSource, type LoadedSource } from '@/composables/useBiDataSource'
import { api, resetApiMocks } from '@/test/api-mock'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

const col = (name: string, type: string, extra: Record<string, unknown> = {}) => ({ name, type, nullable: true, ...extra })
const field = (name: string, type: string) => ({ name, type, required: false, unique: false, hidden: false, readonly: false, options: {} })

function deferred<T>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

function setup(project = 'p1') {
  const state = { project }
  const cleared = vi.fn()
  const loaded: LoadedSource[] = []
  const fieldErrors: string[] = []
  const ds = useBiDataSource({
    projectId: () => state.project,
    onClear: cleared,
    onLoaded: (info) => loaded.push(info),
    onFieldsError: (m) => fieldErrors.push(m)
  })
  return { ds, state, cleared, loaded, fieldErrors }
}

const DB_DOC = { id: 'doc1', name: 'Docs', status: 'ready', dataModel: 'collection' }
const DB_SQL = { id: 'sql1', name: 'SQL', status: 'ready', dataModel: 'sql' }

describe('useBiDataSource', () => {
  beforeEach(() => {
    resetApiMocks()
    api.databases.list.mockResolvedValue([DB_DOC])
    api.db.collectionItems.mockResolvedValue([
      { name: 'tasks', schemaMode: 'json', fieldCount: 1, documentCount: 3, registered: true },
      { name: 'users', schemaMode: 'view', fieldCount: 1, documentCount: 1, registered: true }
    ])
    api.db.getCollectionSchema.mockResolvedValue({ name: 'tasks', schemaMode: 'json', fields: [field('title', 'text')] })
    api.databases.schema.mockResolvedValue({
      tables: [
        { name: 'events', columns: [col('id', 'BIGINT'), col('happened_at', 'TIMESTAMP'), col('secret', 'VARCHAR', { sensitive: true })] },
        { name: 'plain', columns: [col('label', 'VARCHAR')] }
      ]
    })
  })

  it('collection database: lists collections, selects the first, loads its registered schema', async () => {
    const { ds, loaded } = setup()
    await ds.loadDatabases()
    expect(ds.databaseId.value).toBe('doc1')
    expect(ds.collections.value.map((c) => c.name)).toEqual(['tasks', 'users'])
    expect(ds.tables.value).toEqual([])
    expect(ds.source.value).toEqual({ kind: 'collection', name: 'tasks' })
    expect(ds.registered.value).toBe(true)
    expect(ds.timeFields.value).toEqual([])
    expect(loaded).toHaveLength(1)
    expect(loaded[0]).toMatchObject({ projectId: 'p1', databaseId: 'doc1', registered: true, timeFields: [] })
    expect(api.databases.schema).not.toHaveBeenCalled()
  })

  it('collection without a registered schema (404) degrades instead of failing', async () => {
    api.db.getCollectionSchema.mockRejectedValue(Object.assign(new Error('nf'), { status: 404 }))
    const { ds, loaded, fieldErrors } = setup()
    await ds.loadDatabases()
    expect(ds.registered.value).toBe(false)
    expect(ds.fields.value).toEqual([])
    expect(loaded[0].registered).toBe(false)
    expect(fieldErrors).toEqual([])
  })

  it('a non-404 schema failure is reported and does not call onLoaded', async () => {
    api.db.getCollectionSchema.mockRejectedValue(Object.assign(new Error('schema broke'), { status: 500 }))
    const { ds, loaded, fieldErrors } = setup()
    await ds.loadDatabases()
    expect(fieldErrors).toEqual(['schema broke'])
    expect(loaded).toEqual([])
  })

  it('SQL database: lists tables, uses their real columns as fields and picks time columns — no field request', async () => {
    api.databases.list.mockResolvedValue([DB_SQL])
    const { ds, loaded } = setup()
    await ds.loadDatabases()
    expect(ds.isSqlDatabase.value).toBe(true)
    expect(ds.tables.value.map((t) => t.name)).toEqual(['events', 'plain'])
    expect(ds.collections.value).toEqual([])
    expect(ds.source.value).toEqual({ kind: 'table', name: 'events' })
    expect(ds.fields.value.map((f) => [f.name, f.type])).toEqual([['id', 'number'], ['happened_at', 'date']])
    expect(ds.registered.value).toBe(true)
    expect(ds.timeFields.value).toEqual(['happened_at'])
    expect(loaded[0]).toMatchObject({ registered: true, timeFields: ['happened_at'], source: { kind: 'table', name: 'events' } })
    expect(api.db.collectionItems).not.toHaveBeenCalled()
    expect(api.db.getCollectionSchema).not.toHaveBeenCalled()
  })

  it('selecting another table reloads its fields; selecting the same source is a no-op', async () => {
    api.databases.list.mockResolvedValue([DB_SQL])
    const { ds, loaded } = setup()
    await ds.loadDatabases()
    ds.selectSource({ kind: 'table', name: 'events' })
    expect(loaded).toHaveLength(1)
    ds.selectSource({ kind: 'table', name: 'plain' })
    await Promise.resolve()
    expect(ds.source.value).toEqual({ kind: 'table', name: 'plain' })
    expect(ds.timeFields.value).toEqual([])
    expect(loaded).toHaveLength(2)
  })

  it('an empty database leaves no source and exposes hasSources=false', async () => {
    api.db.collectionItems.mockResolvedValue([])
    const { ds, loaded } = setup()
    await ds.loadDatabases()
    expect(ds.source.value).toBeNull()
    expect(ds.hasSources.value).toBe(false)
    expect(loaded).toEqual([])
    api.databases.list.mockResolvedValue([DB_SQL])
    api.databases.schema.mockResolvedValue({ tables: [] })
    await ds.loadDatabases()
    expect(ds.source.value).toBeNull()
    expect(ds.hasSources.value).toBe(false)
  })

  it('reports a database list failure and an empty project', async () => {
    api.databases.list.mockRejectedValueOnce(new Error('offline'))
    const { ds } = setup()
    await ds.loadDatabases()
    expect(ds.databasesError.value).toBe('offline')
    expect(ds.databases.value).toEqual([])
    const none = setup('')
    await none.ds.loadDatabases()
    expect(none.ds.databases.value).toEqual([])
    expect(api.databases.list).toHaveBeenCalledTimes(1)
  })

  it('reports a source list failure (collections and tables) with the shared message', async () => {
    api.db.collectionItems.mockRejectedValueOnce(new Error('collections offline'))
    const a = setup()
    await a.ds.loadDatabases()
    expect(a.ds.sourcesError.value).toBe('collections offline')
    api.databases.list.mockResolvedValue([DB_SQL])
    api.databases.schema.mockRejectedValueOnce(new Error('schema offline'))
    const b = setup()
    await b.ds.loadDatabases()
    expect(b.ds.sourcesError.value).toBe('schema offline')
    expect(b.ds.source.value).toBeNull()
  })

  it('only keeps databases that are ready', async () => {
    api.databases.list.mockResolvedValue([DB_DOC, { ...DB_SQL, status: 'creating' }])
    const { ds } = setup()
    await ds.loadDatabases()
    expect(ds.databases.value.map((d) => d.id)).toEqual(['doc1'])
  })

  it('reloading with preserveSelection keeps the chosen source; if it disappeared it falls back to the first', async () => {
    const { ds } = setup()
    await ds.loadDatabases()
    ds.selectSource({ kind: 'collection', name: 'users' })
    await Promise.resolve()
    await ds.reloadSources()
    expect(ds.source.value).toEqual({ kind: 'collection', name: 'users' })
    api.db.collectionItems.mockResolvedValue([{ name: 'tasks', schemaMode: 'json', fieldCount: 1, documentCount: 1, registered: true }])
    await ds.reloadSources()
    expect(ds.source.value).toEqual({ kind: 'collection', name: 'tasks' })
  })

  it('a view collection is detected so write actions can be disabled', async () => {
    const { ds } = setup()
    await ds.loadDatabases()
    expect(ds.isView.value).toBe(false)
    ds.selectSource({ kind: 'collection', name: 'users' })
    expect(ds.isView.value).toBe(true)
    api.databases.list.mockResolvedValue([DB_SQL])
    await ds.loadDatabases()
    expect(ds.isView.value).toBe(false)
  })

  it('selectDatabase ignores unknown ids and the current id', async () => {
    api.databases.list.mockResolvedValue([DB_DOC, DB_SQL])
    const { ds } = setup()
    await ds.loadDatabases()
    ds.selectDatabase('')
    ds.selectDatabase('nope')
    ds.selectDatabase('doc1')
    expect(ds.databaseId.value).toBe('doc1')
    ds.selectDatabase('sql1')
    expect(ds.databaseId.value).toBe('sql1')
  })

  it('clears everything below the source whenever the source changes', async () => {
    const { ds, cleared } = setup()
    await ds.loadDatabases()
    const before = cleared.mock.calls.length
    ds.selectSource({ kind: 'collection', name: 'users' })
    await Promise.resolve()
    expect(cleared.mock.calls.length).toBeGreaterThan(before)
  })

  it('drops a stale source list when the database changed while it was loading', async () => {
    api.databases.list.mockResolvedValue([DB_DOC, DB_SQL])
    const { ds } = setup()
    await ds.loadDatabases()
    const slow = deferred<unknown>()
    api.db.collectionItems.mockReturnValueOnce(slow.promise as never)
    const pending = ds.loadSources()
    ds.selectDatabase('sql1')
    await ds.loadSources()
    slow.resolve([{ name: 'STALE', schemaMode: 'json', fieldCount: 0, documentCount: 0, registered: true }])
    await pending
    expect(ds.databaseId.value).toBe('sql1')
    expect(ds.collections.value.map((c) => c.name)).not.toContain('STALE')
    expect(ds.tables.value.map((t) => t.name)).toContain('events')
  })

  it('drops a stale field response after switching sources quickly', async () => {
    const { ds, loaded } = setup()
    await ds.loadDatabases()
    const slow = deferred<unknown>()
    api.db.getCollectionSchema.mockImplementation((_p, _d, name) =>
      name === 'users' ? (slow.promise as never) : Promise.resolve({ name, schemaMode: 'json', fields: [field('title', 'text')] })
    )
    ds.selectSource({ kind: 'collection', name: 'users' })
    await Promise.resolve()
    ds.selectSource({ kind: 'collection', name: 'tasks' })
    await Promise.resolve()
    const count = loaded.length
    slow.resolve({ name: 'users', schemaMode: 'view', fields: [field('STALE', 'text')] })
    await Promise.resolve()
    await Promise.resolve()
    expect(loaded.length).toBe(count)
    expect(ds.fields.value.map((f) => f.name)).not.toContain('STALE')
  })
})
