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

const pid = 'proj 1'
const db = 'db 1'

function ok(data: unknown) {
  return Promise.resolve({ data })
}

describe('httpApi', () => {
  beforeEach(() => {
    Object.values(http).forEach((fn) => fn.mockReset())
  })

  it('maps sandbox resources, lifecycle and envd file paths', async () => {
    http.get.mockResolvedValueOnce({ data: { available: true, backend: 'fake', images: ['python:3.12-slim'],
      default_image: 'python:3.12-slim', default_template: 'base', cpus: 2, memory_mib: 512,
      timeout_default_s: 300, timeout_max_s: 3600, max_file_bytes: 1024, max_output_bytes: 2048,
      max_per_project: 5, terminal: true, envd_version: '0.2.0',
      ssh: { enabled: true, listen: '127.0.0.1:2222', host_key_fingerprint: 'SHA256:x' } } })
    const caps = await httpApi.sandboxes.capabilities(pid)
    expect(caps).toMatchObject({ available: true, backend: 'fake', cpus: 2, memoryMiB: 512, terminal: true, envdVersion: '0.2.0' })
    expect(caps.ssh).toEqual({ enabled: true, listen: '127.0.0.1:2222', hostKeyFingerprint: 'SHA256:x' })
    expect(http.get.mock.calls[0][0]).toBe('/e2b/ext/capabilities')
    expect(http.get.mock.calls[0][1].headers['X-SimpleBase-Project']).toBe(pid)

    const row = { sandboxID: 'sb-1', templateID: 'base', state: 'running', startedAt: 'today', endAt: 'tomorrow',
      cpuCount: 2, memoryMB: 512, metadata: { name: 'demo', 'simplebase.source': 'api' },
      x_simplebase: { status: 'running', source: 'api', idleTimeoutS: 300, lastActiveAt: 'now', lastError: 'e' } }
    http.get.mockResolvedValueOnce({ data: [row] })
    const [item] = await httpApi.sandboxes.list(pid, 'running', { team: 'a' })
    expect(item).toMatchObject({ id: 'sb-1', name: 'demo', state: 'running', status: 'running', cpuCount: 2,
      memoryMB: 512, source: 'api', idleTimeoutS: 300, lastActiveAt: 'now', lastError: 'e' })
    const listCfg = http.get.mock.calls[1][1]
    expect(listCfg.params.metadata).toBe('team=a')
    expect(listCfg.params.limit).toBe(100)

    http.get.mockResolvedValueOnce({ data: row })
    expect((await httpApi.sandboxes.get(pid, 'sb/1')).id).toBe('sb-1')
    expect(http.get.mock.calls[2][0]).toBe('/e2b/sandboxes/sb%2F1')

    http.post.mockResolvedValueOnce({ data: row })
    await httpApi.sandboxes.create(pid, { name: 'demo', templateId: 'base', timeoutS: 120 })
    expect(http.post.mock.calls[0][0]).toBe('/e2b/sandboxes')
    expect(http.post.mock.calls[0][1]).toMatchObject({ templateID: 'base', timeout: 120, metadata: { name: 'demo' } })

    http.post.mockResolvedValueOnce({ data: row })
    await httpApi.sandboxes.connect(pid, 'sb-1', 60)
    expect(http.post.mock.calls[1][0]).toBe('/e2b/sandboxes/sb-1/connect')
    expect(http.post.mock.calls[1][1]).toMatchObject({ timeout: 60 })

    http.post.mockResolvedValueOnce({ data: undefined })
    await httpApi.sandboxes.pause(pid, 'sb-1')
    expect(http.post.mock.calls[2][0]).toBe('/e2b/sandboxes/sb-1/pause')

    http.post.mockResolvedValueOnce({ data: { ticket: 'tk', expires_in: 30 } })
    expect(await httpApi.sandboxes.terminalTicket(pid, 'sb-1')).toEqual({ ticket: 'tk', expiresIn: 30 })

    http.post.mockResolvedValueOnce({ data: { entries: [{ name: 'a.txt', path: '/workspace/a.txt',
      type: 'FILE_TYPE_FILE', size: '2', permissions: '-rw-r--r--' }] } })
    const entries = await httpApi.sandboxes.files.list(pid, 'sb-1', '/workspace')
    expect(entries[0]).toMatchObject({ name: 'a.txt', kind: 'file', size: 2, permissions: '-rw-r--r--' })
    expect(http.post.mock.calls[4][0]).toBe('/e2b/envd/filesystem.Filesystem/ListDir')
    expect(http.post.mock.calls[4][1]).toMatchObject({ path: '/workspace', depth: 1 })
    expect(http.post.mock.calls[4][2].headers['E2b-Sandbox-Id']).toBe('sb-1')
    expect(http.post.mock.calls[4][2].headers['Connect-Protocol-Version']).toBe('1')

    const blob = { size: 2 }
    http.get.mockResolvedValueOnce({ data: blob })
    expect(await httpApi.sandboxes.files.read(pid, 'sb-1', '/workspace/a.txt')).toBe(blob)
    expect(http.get.mock.calls[3][1]).toMatchObject({ params: { path: '/workspace/a.txt' }, responseType: 'blob' })

    http.post.mockResolvedValueOnce({ data: undefined })
    await httpApi.sandboxes.files.write(pid, 'sb-1', '/workspace/a.txt', 'hi')
    expect(http.post.mock.calls[5][0]).toBe('/e2b/envd/files')
    expect(http.post.mock.calls[5][1]).toBeInstanceOf(Blob)
    expect(http.post.mock.calls[5][2].headers['Content-Type']).toBe('application/octet-stream')

    http.post.mockResolvedValueOnce({ data: undefined })
    await httpApi.sandboxes.files.write(pid, 'sb-1', '/workspace/d.bin', blob as unknown as Blob)
    expect(http.post.mock.calls[6][1]).toBe(blob)

    http.post.mockResolvedValueOnce({ data: undefined })
    await httpApi.sandboxes.files.remove(pid, 'sb-1', '/workspace/a.txt')
    expect(http.post.mock.calls[7][0]).toBe('/e2b/envd/filesystem.Filesystem/Remove')

    http.delete.mockResolvedValueOnce({ data: undefined })
    await httpApi.sandboxes.remove(pid, 'sb-1')
    expect(http.delete.mock.calls[0][0]).toBe('/e2b/sandboxes/sb-1')
  })

  it('builds the terminal websocket url with the ticket and keeps the base path', () => {
    const url = new URL(httpApi.sandboxes.terminalWebSocketUrl('sb/1', 'tk+/='))
    expect(url.pathname).toBe('/e2b/ext/sandboxes/sb%2F1/terminal')
    expect(url.searchParams.get('ticket')).toBe('tk+/=')
    expect(['ws:', 'wss:']).toContain(url.protocol)
  })

  it('follows the sandbox list X-Next-Token until the last page', async () => {
    http.get.mockResolvedValueOnce({ data: [{ sandboxID: 'a', state: 'running' }], headers: { 'x-next-token': 't1' } })
    http.get.mockResolvedValueOnce({ data: [{ sandboxID: 'b', state: 'paused' }], headers: {} })
    const rows = await httpApi.sandboxes.list(pid, 'running')
    expect(rows.map((r) => r.id)).toEqual(['a', 'b'])
    expect(http.get).toHaveBeenCalledTimes(2)
    expect(http.get.mock.calls[0][1].params).toEqual({ state: 'running', metadata: undefined, limit: 100, nextToken: undefined })
    expect(http.get.mock.calls[1][1].params.nextToken).toBe('t1')
    http.get.mockResolvedValueOnce({ data: [] })
    expect(await httpApi.sandboxes.list(pid)).toEqual([])
  })

  it('maps gofunctions and cron jobs including interval/manual variants', async () => {
    http.get.mockResolvedValueOnce({
      data: { functions: [{ id: 1, name: 'hello', file: 'hello.go', exports: ['Hello'], created_at: 'a', updated_at: 'b' }] }
    })
    const fns = await httpApi.gofunctions.list(pid)
    expect(fns[0].name).toBe('hello')
    expect(http.get.mock.calls[0][0]).toContain(encodeURIComponent(pid))

    http.post.mockResolvedValueOnce({ data: { name: 'n', created_at: 't' } })
    await httpApi.gofunctions.create(pid, { name: 'n', source: 's' })
    http.get.mockResolvedValueOnce({ data: { name: 'n' } })
    await httpApi.gofunctions.get(pid, 'n')
    http.post.mockResolvedValueOnce({ data: { name: 'n', source: 's2' } })
    await httpApi.gofunctions.saveVersion(pid, 'n', { source: 's2', note: 'x', activate: true })
    http.post.mockResolvedValueOnce({ data: { active_version: 1 } })
    await httpApi.gofunctions.activate(pid, 'n', 1)
    http.get.mockResolvedValueOnce({
      data: { active_version: 1, versions: [{ version: 1, exports: ['Hello'], note: '', created_at: 't', active: true }] }
    })
    await httpApi.gofunctions.listVersions(pid, 'n')
    http.post.mockResolvedValueOnce({
      data: { ok: true, status_code: 200, duration_ms: 3, version: 1, active_version: 1, function_name: 'Hello', data: { x: 1 } }
    })
    await httpApi.gofunctions.test(pid, 'n', 1, 'Hello', {})
    http.delete.mockResolvedValueOnce({ data: {} })
    await httpApi.gofunctions.remove(pid, 'n')

    http.get.mockResolvedValueOnce({
      data: {
        jobs: [
          {
            id: 'j',
            schedule_kind: 'interval',
            interval_seconds: 90,
            enabled: 1,
            target_missing: true,
            run_count: '3'
          }
        ]
      }
    })
    const jobs = await httpApi.cronjobs.list(pid)
    expect(jobs[0].scheduleKind).toBe('interval')
    expect(jobs[0].intervalSeconds).toBe(90)

    // planv6.0 前端简化 A1：调度类型只认 interval / once，其余（含缺失、null、大小写不符）回退 cron。
    http.get.mockResolvedValueOnce({
      data: {
        jobs: [
          { schedule_kind: 'interval' },
          { schedule_kind: 'once' },
          { schedule_kind: 'cron' },
          { schedule_kind: 'Interval' },
          { schedule_kind: 'unknown' },
          { schedule_kind: null },
          {}
        ]
      }
    })
    expect((await httpApi.cronjobs.list(pid)).map((job) => job.scheduleKind)).toEqual([
      'interval', 'once', 'cron', 'cron', 'cron', 'cron', 'cron'
    ])

    // planv6.0 前端简化 B1-cron：类型收紧不改变运行时转换（零值、null、数字字符串、空串）。
    http.get.mockResolvedValueOnce({
      data: {
        jobs: [
          { interval_seconds: 0, run_at: '', enabled: 0, run_count: '7', input_json: null, target_missing: 'yes' },
          { interval_seconds: null, run_at: null, last_run_at: '2026-01-01T00:00:00Z', enabled: 'false' }
        ]
      }
    })
    const [zeroJob, nullJob] = await httpApi.cronjobs.list(pid)
    expect(zeroJob).toMatchObject({ intervalSeconds: 0, runAt: undefined, enabled: false, runCount: 7, inputJson: '{}', targetMissing: true })
    expect(nullJob).toMatchObject({ intervalSeconds: undefined, runAt: undefined, lastRunAt: '2026-01-01T00:00:00Z', enabled: true })
    http.get.mockResolvedValueOnce({ data: { runs: [{ trigger: 'Manual', duration_ms: '5', status: null, started_at: '' }] } })
    expect((await httpApi.cronjobs.runs(pid, 'j'))[0]).toMatchObject({ trigger: 'scheduled', durationMs: 5, status: '', startedAt: undefined })

    http.post.mockResolvedValueOnce({ data: { id: 'j', schedule_kind: 'cron' } })
    await httpApi.cronjobs.create(pid, {
      name: 'n',
      description: 'd',
      scheduleKind: 'cron',
      cronExpr: '* * * * *',
      intervalSeconds: 60,
      funcFile: 'hello',
      funcExport: 'Hello',
      inputJson: '{}',
      enabled: true
    })
    http.patch.mockResolvedValueOnce({ data: { id: 'j' } })
    await httpApi.cronjobs.update(pid, 'j', { description: 'x' })
    http.delete.mockResolvedValueOnce({ data: {} })
    await httpApi.cronjobs.remove(pid, 'j')
    http.get.mockResolvedValueOnce({
      data: { runs: [{ id: 'r', trigger: 'manual', status: 'completed', duration_ms: 2 }] }
    })
    const runs = await httpApi.cronjobs.runs(pid, 'j', 10)
    expect(runs[0].trigger).toBe('manual')
    http.get.mockResolvedValueOnce({ data: { runs: null } })
    expect(await httpApi.cronjobs.runs(pid, 'j')).toEqual([])
    http.post.mockResolvedValueOnce({ data: {} })
    await httpApi.cronjobs.trigger(pid, 'j')
  })

  it('maps projects, metrics, databases, sql, and data rows', async () => {
    http.get.mockResolvedValueOnce({ data: { projects: [{ id: 'p', name: 'n', created_at: 't' }] } })
    expect((await httpApi.projects.list())[0].createdAt).toBe('t')
    http.get.mockResolvedValueOnce({ data: { projects: null } })
    expect(await httpApi.projects.list()).toEqual([])
    http.post.mockResolvedValueOnce({ data: { id: 'p', name: 'n' } })
    await httpApi.projects.create({ name: 'n' })
    http.post.mockResolvedValueOnce({ data: { id: 'p', createdAt: 'c' } })
    await httpApi.projects.create({ name: 'n', id: 'p' })

    http.get.mockResolvedValueOnce({ data: { total_requests: 1, error_rate: 2, avg_latency_ms: 3, active_databases: 4 } })
    const legacySummary = await httpApi.metrics.summary(pid)
    expect(legacySummary.totalRequests).toBe(1)
    // 旧后端无分位数字段：保持 null，不转成 0
    expect(legacySummary.latencyP50Ms).toBeNull()
    expect(legacySummary.latencySampleCount).toBe(0)
    expect(legacySummary.latencyOverflowMs).toBe(30000)
    http.get.mockResolvedValueOnce({
      data: {
        total_requests: 5,
        latency_p50_ms: 12.5,
        latency_p90_ms: 80,
        latency_p99_ms: null,
        latency_sample_count: 5,
        latency_overflow_ms: 30000
      }
    })
    const pct = await httpApi.metrics.summary(pid)
    expect(pct.latencyP50Ms).toBe(12.5)
    expect(pct.latencyP90Ms).toBe(80)
    expect(pct.latencyP99Ms).toBeNull()
    expect(pct.latencySampleCount).toBe(5)
    http.get.mockResolvedValueOnce({ data: { latency_p50_ms: 'NaN' } })
    expect((await httpApi.metrics.summary(pid)).latencyP50Ms).toBeNull()
    http.get.mockResolvedValueOnce({ data: undefined })
    expect((await httpApi.metrics.summary(pid)).totalRequests).toBe(0)
    http.get.mockResolvedValueOnce({ data: { points: [{ date: 'd', requests: 1, errors: 0 }] } })
    expect((await httpApi.metrics.trend(pid))[0].date).toBe('d')
    http.get.mockResolvedValueOnce({ data: [{ date: 'x' }] })
    expect((await httpApi.metrics.trend(pid))[0].date).toBe('x')

    // planv6.0 前端简化 B1-metrics：snake 优先于 camel；?? 只跳过 null/undefined；optionalNumber 处理空串/非法值。
    http.get.mockResolvedValueOnce({
      data: {
        total_requests: 0, totalRequests: 9,
        error_rate: null, errorRate: 0.5,
        avg_latency_ms: '7',
        activeDatabases: 2,
        latency_p50_ms: '', latencyP50Ms: 3,
        latency_p90_ms: undefined, latencyP90Ms: '4',
        latency_p99_ms: 'abc',
        latency_sample_count: null,
        latency_overflow_ms: 0
      }
    })
    expect(await httpApi.metrics.summary(pid)).toEqual({
      totalRequests: 0, errorRate: 0.5, avgLatencyMs: 7, activeDatabases: 2,
      latencyP50Ms: null, latencyP90Ms: 4, latencyP99Ms: null,
      latencySampleCount: 0, latencyOverflowMs: 0
    })
    http.get.mockResolvedValueOnce({ data: 'oops' })
    expect((await httpApi.metrics.summary(pid)).latencyOverflowMs).toBe(30000)
    http.get.mockResolvedValueOnce({ data: { points: [null, { date: 1, requests: '2', errors: null }] } })
    expect(await httpApi.metrics.trend(pid)).toEqual([
      { date: '', requests: 0, errors: 0 },
      { date: '1', requests: 2, errors: 0 }
    ])
    http.get.mockResolvedValueOnce({ data: 'oops' })
    expect(await httpApi.metrics.trend(pid)).toEqual([])

    http.get.mockResolvedValueOnce({
      data: {
        databases: [{ id: 'd', name: 'n', status: 'ready', created_at: 'a', updated_at: 'b', snapshot: { last_synced_snapshot: 1, sync_lag: 2 } }]
      }
    })
    const listed = await httpApi.databases.list(pid)
    expect(listed[0].snapshot?.syncLag).toBe(2)
    expect(listed[0].dataModel).toBe('collection')
    http.get.mockResolvedValueOnce({
      data: { databases: [{ id: 'd', name: 'n', status: 'ready', created_at: 'a', updated_at: 'b', data_model: 'sql' }] }
    })
    expect((await httpApi.databases.list(pid))[0].dataModel).toBe('sql')
    http.get.mockResolvedValueOnce({ data: {} })
    expect(await httpApi.databases.list(pid)).toEqual([])
    http.post.mockResolvedValueOnce({ data: { id: 'd', name: 'n', status: 'creating', data_model: 'sql' } })
    const created = await httpApi.databases.create(pid, {
      name: 'n',
      dataModel: 'sql',
      initSql: 'CREATE TABLE t (id INTEGER)'
    })
    expect(created.dataModel).toBe('sql')
    expect(http.post.mock.calls.at(-1)?.[1]).toEqual({
      name: 'n',
      data_model: 'sql',
      init_sql: 'CREATE TABLE t (id INTEGER)'
    })
    http.post.mockResolvedValueOnce({ data: { id: 'd', name: 'plain', status: 'ready' } })
    expect((await httpApi.databases.create(pid, { name: 'plain' })).dataModel).toBe('collection')
    http.get.mockResolvedValueOnce({
      data: {
        tables: [
          { name: 't', columns: [{ name: 'id', type: 'INTEGER', nullable: false }, 1, { name: 2, type: 3 }] }
        ]
      }
    })
    const schema = await httpApi.databases.schema(pid, db)
    expect(schema.tables[0].columns[0]).toEqual({ name: 'id', type: 'INTEGER', nullable: false })
    expect(schema.tables[0].columns[1]).toEqual({ name: '', type: '', nullable: true })
    expect(schema.tables[0].columns[2].name).toBe('')
    expect(http.get.mock.calls.at(-1)?.[0]).toContain('/schema')
    http.get.mockResolvedValueOnce({ data: undefined })
    expect(await httpApi.databases.schema(pid, db)).toEqual({ tables: [] })
    http.get.mockResolvedValueOnce({ data: { tables: [1, { name: 'x' }] } })
    const partial = await httpApi.databases.schema(pid, db)
    expect(partial.tables[0]).toEqual({ name: '', columns: [] })
    expect(partial.tables[1]).toEqual({ name: 'x', columns: [] })
    http.get.mockResolvedValueOnce({
      data: {
        table: 'sys_users',
        columns: [{ name: 'password_hash', type: 'VARCHAR', nullable: false, sensitive: true }, { name: 'id' }],
        rows: [[null, 'u1'], 'bad'],
        limit: 50,
        offset: 0,
        total: 1
      }
    })
    const rows = await httpApi.databases.tableRows(pid, db, 'sys/users', { limit: 50, offset: 0 })
    expect(rows.columns[0].sensitive).toBe(true)
    expect(rows.columns[1].sensitive).toBeUndefined()
    expect(rows.rows).toEqual([[null, 'u1'], []])
    expect(rows.total).toBe(1)
    expect(String(http.get.mock.calls.at(-1)?.[0])).toContain('/schema/tables/sys%2Fusers/rows')
    expect(http.get.mock.calls.at(-1)?.[1]).toEqual({ params: { limit: 50, offset: 0 } })
    http.get.mockResolvedValueOnce({ data: undefined })
    expect(await httpApi.databases.tableRows(pid, db, 't', { limit: 50, offset: 0 })).toEqual({
      table: '',
      columns: [],
      rows: [],
      limit: 0,
      offset: 0,
      total: 0
    })
    http.post.mockResolvedValueOnce({ data: { name: 't', columns: [{ name: 'id', type: 'INTEGER', nullable: false }] } })
    expect(
      (
        await httpApi.databases.createTable(pid, db, {
          name: 't',
          columns: [{ name: 'id', type: 'INTEGER', nullable: false }]
        })
      ).columns[0].nullable
    ).toBe(false)
    expect(http.post.mock.calls.at(-1)?.[0]).toContain('/schema/tables')
    expect(http.post.mock.calls.at(-1)?.[1]).toEqual({
      name: 't',
      columns: [{ name: 'id', type: 'INTEGER', nullable: false }]
    })
    http.post.mockResolvedValueOnce({ data: null })
    expect(await httpApi.databases.createTable(pid, db, { name: 't', columns: [] })).toEqual({
      name: '',
      columns: []
    })
    http.post.mockResolvedValueOnce({ data: { name: 'email', type: 'VARCHAR', nullable: true } })
    expect(
      (await httpApi.databases.addColumn(pid, db, { table: 't', name: 'email', type: 'VARCHAR' })).nullable
    ).toBe(true)
    expect(http.post.mock.calls.at(-1)?.[0]).toContain('/schema/columns')
    http.post.mockResolvedValueOnce({ data: 'nope' })
    expect(
      await httpApi.databases.addColumn(pid, db, { table: 't', name: 'a', type: 'JSON', nullable: false })
    ).toEqual({ name: '', type: '', nullable: true })
    http.delete.mockResolvedValueOnce({ data: {} })
    await httpApi.databases.remove(pid, db)

    http.post.mockResolvedValueOnce({ data: { columns: ['c'], rows: [[1]], row_count: 1, duration_ms: 2, request_id: 'r' } })
    expect((await httpApi.sql.query(pid, db, { sql: 'select 1', maxRows: 5 })).rowCount).toBe(1)
    http.post.mockResolvedValueOnce({ data: {} })
    expect((await httpApi.sql.query(pid, db, { sql: 'select 1' })).columns).toEqual([])
    http.post.mockResolvedValueOnce({ data: { rows_affected: 2, durability: 'ok', duration_ms: 1, request_id: 'r' } })
    expect((await httpApi.sql.execute(pid, db, { sql: 'update x' })).rowsAffected).toBe(2)
    http.post.mockResolvedValueOnce({
      data: {
        results: [{ index: 0, rows_affected: 1, duration_ms: 2, error_code: 'e', error_message: 'm' }],
        durability: 'ok',
        duration_ms: 3,
        request_id: 'r',
        error: { failed_index: 0, code: 'e', message: 'm' }
      }
    })
    const batch = await httpApi.sql.batch(pid, db, { statements: [{ sql: 's' }], transactional: true })
    expect(batch.error?.failedIndex).toBe(0)
    http.post.mockResolvedValueOnce({ data: {} })
    expect((await httpApi.sql.batch(pid, db, { statements: [{ sql: 's' }], transactional: false })).results).toEqual([])
  })

  it('exports a collection as JSON with the current query', async () => {
    const blob = new Blob(['[]'], { type: 'application/json' })
    http.get.mockResolvedValueOnce({ data: blob })
    const result = await httpApi.db.exportCollection(pid, db, 'task items', 'json', {
      filter: 'priority>1',
      sort: '-created_at'
    })
    expect(result).toBe(blob)
    expect(http.get).toHaveBeenCalledWith(
      '/v1/projects/proj%201/databases/db%201/data/collections/task%20items/export',
      { params: { filter: 'priority>1', sort: '-created_at', format: 'json' }, responseType: 'blob' }
    )
  })

  it('covers data, s3, logs, quota, llm, agents and schedules', async () => {
    http.get.mockResolvedValueOnce({ data: { collections: ['u'] } })
    expect(await httpApi.db.collections(pid, db)).toEqual(['u'])
    http.get.mockResolvedValueOnce({ data: {} })
    expect(await httpApi.db.collections(pid, db)).toEqual([])
    http.post.mockResolvedValueOnce({ data: {} })
    await httpApi.db.createCollection(pid, db, 'c')
    http.get.mockResolvedValueOnce({ data: { rows: [{ id: '1' }] } })
    expect(await httpApi.db.rows(pid, db, 'c', { q: 1 })).toEqual([{ id: '1' }])
    http.get.mockResolvedValueOnce({ data: {} })
    expect(await httpApi.db.rows(pid, db, 'c')).toEqual([])
    http.post.mockResolvedValueOnce({ data: { id: '1' } })
    await httpApi.db.insert(pid, db, 'c', { a: 1 })
    http.put.mockResolvedValueOnce({ data: { id: '1' } })
    await httpApi.db.update(pid, db, 'c', '1', { a: 2 })
    http.delete.mockResolvedValueOnce({ data: {} })
    await httpApi.db.remove(pid, db, 'c', '1')

    http.get.mockResolvedValueOnce({ data: [{ key: 'k' }] })
    expect(await httpApi.s3.list(pid, 'pre')).toHaveLength(1)
    http.get.mockResolvedValueOnce({ data: { nope: true } })
    expect(await httpApi.s3.list(pid)).toEqual([])
    http.get.mockResolvedValueOnce({ data: { url: 'u' } })
    expect(await httpApi.s3.presign(pid, 'k')).toEqual({ url: 'u' })
    http.delete.mockResolvedValueOnce({ data: {} })
    await httpApi.s3.remove(pid, 'k')
    http.post.mockResolvedValueOnce({ data: { key: 'k', size: 1 } })
    const onProgress = vi.fn()
    await httpApi.s3.upload(pid, 'k', new File(['x'], 'x.txt'), onProgress)
    const uploadCfg = http.post.mock.calls.at(-1)?.[2] as {
      timeout: number
      onUploadProgress: (e: { loaded: number; total?: number }) => void
    }
    expect(uploadCfg.timeout).toBe(0)
    uploadCfg.onUploadProgress({ loaded: 1, total: 2 })
    expect(onProgress).toHaveBeenCalledWith(50)
    uploadCfg.onUploadProgress({ loaded: 1 })

    http.get.mockResolvedValueOnce({
      data: { events: [{ id: '1', project_id: 'p', level: 'info', logger: 'l', message: 'm', occurred_at: 't' }] }
    })
    expect((await httpApi.logs.list(pid, { level: 'info', q: 'x', from: 'a', to: 'b', limit: 5 }))[0].projectId).toBe('p')
    http.get.mockResolvedValueOnce({ data: [{ ID: '2', ProjectID: 'p', Level: 'warn', Logger: 'l', Message: 'm', OccurredAt: 't' }] })
    expect((await httpApi.logs.list(pid))[0].id).toBe('2')

    // planv6.0 前端简化 B1-log：snake → camel → Pascal 优先级；?? 只跳过 null/undefined；透传字段 null → undefined。
    http.get.mockResolvedValueOnce({
      data: {
        events: [
          {
            id: 0, ID: 'x', project_id: null, projectId: '', ProjectID: 'P',
            level: '', Level: 'warn', Logger: 'L', message: null, Message: 'M',
            fields_json: null, fieldsJson: '{"a":1}', FieldsJSON: 'z',
            request_id: '', RequestID: 'r', occurred_at: undefined, OccurredAt: 'T'
          },
          { fields_json: null, request_id: null },
          null
        ]
      }
    })
    expect(await httpApi.logs.list(pid)).toEqual([
      {
        id: '0', projectId: '', level: '', logger: 'L', message: 'M',
        fieldsJson: '{"a":1}', requestId: '', occurredAt: 'T'
      },
      { id: '', projectId: '', level: '', logger: '', message: '', fieldsJson: undefined, requestId: undefined, occurredAt: '' },
      { id: '', projectId: '', level: '', logger: '', message: '', fieldsJson: undefined, requestId: undefined, occurredAt: '' }
    ])
    http.get.mockResolvedValueOnce({ data: { keep_days: 0, KeepDays: 9, Scope: 'S', UpdatedAt: 'U' } })
    expect(await httpApi.logs.getRetention(pid)).toEqual({ scope: 'S', keepDays: 0, updatedAt: 'U' })
    http.get.mockResolvedValueOnce({ data: { keep_days: null, keepDays: '3' } })
    expect((await httpApi.logs.getRetention(pid)).keepDays).toBe(3)
    http.get.mockResolvedValueOnce({ data: 'oops' })
    expect(await httpApi.logs.getRetention(pid)).toEqual({ scope: '', keepDays: 14, updatedAt: '' })
    http.get.mockResolvedValueOnce({ data: { keep_days: 7, updated_at: 't', scope: 'project' } })
    expect((await httpApi.logs.getRetention(pid)).keepDays).toBe(7)
    http.get.mockResolvedValueOnce({ data: undefined })
    expect((await httpApi.logs.getRetention(pid)).keepDays).toBe(14)
    http.put.mockResolvedValueOnce({ data: {} })
    await httpApi.logs.putRetention(pid, 30)

    http.get.mockResolvedValueOnce({ data: { llm_allowed: true, database_allowed: false } })
    expect((await httpApi.quota.status(pid)).llmAllowed).toBe(true)

    http.post.mockResolvedValueOnce({
      data: {
        id: 'chatcmpl-1',
        object: 'chat.completion',
        model: 'm',
        provider: 'openai',
        choices: [{ index: 0, message: { role: 'assistant', content: 'hi' }, finish_reason: 'stop' }],
        usage: { prompt_tokens: 1, completion_tokens: 2, total_tokens: 3 }
      }
    })
    // OpenAI 形状经服务层展平回界面已有的扁平类型（不透传嵌套结构）。
    expect(await httpApi.llm.chat(pid, { messages: [{ role: 'user', content: 'hi' }], maxTokens: 1, temperature: 0 })).toEqual({
      content: 'hi',
      model: 'm',
      provider: 'openai',
      finish_reason: 'stop',
      usage: { prompt_tokens: 1, completion_tokens: 2, total_tokens: 3 }
    })
    expect(http.post).toHaveBeenLastCalledWith(
      expect.stringContaining('/v1/chat/completions'),
      expect.objectContaining({ messages: [{ role: 'user', content: 'hi' }] })
    )
    http.get.mockResolvedValueOnce({
      data: { default_provider: 'openai', default_model: 'm', base_url: 'http://x/v1', api_key_masked: 'sk-...7890', has_api_key: true, max_tokens: 8 }
    })
    const settings = await httpApi.llmSettings.get(pid)
    expect(settings.maxTokens).toBe(8)
    expect(settings.baseUrl).toBe('http://x/v1')
    expect(settings.apiKeyMasked).toBe('sk-...7890')
    expect(settings.hasApiKey).toBe(true)
    http.get.mockResolvedValueOnce({ data: undefined })
    expect(await httpApi.llmSettings.get(pid)).toEqual({ hasApiKey: false })
    http.put.mockResolvedValueOnce({ data: { defaultProvider: 'openai' } })
    await httpApi.llmSettings.put(pid, { defaultProvider: 'openai', defaultModel: 'm', baseUrl: 'http://x/v1', temperature: 1, maxTokens: 2 }, 'sk-live')
    expect(http.put).toHaveBeenLastCalledWith(expect.stringContaining('/llm/settings'), {
      default_provider: 'openai',
      default_model: 'm',
      api_key: 'sk-live',
      base_url: 'http://x/v1',
      temperature: 1,
      max_tokens: 2
    })
    // 未传 apiKey 时回空串，服务端视为沿用既有值
    http.put.mockResolvedValueOnce({ data: {} })
    await httpApi.llmSettings.put(pid, {})
    expect(http.put).toHaveBeenLastCalledWith(expect.stringContaining('/llm/settings'), {
      default_provider: '',
      default_model: '',
      api_key: '',
      base_url: '',
      temperature: undefined,
      max_tokens: undefined
    })

    http.get.mockResolvedValueOnce({ data: { modules: [{ id: 'm' }] } })
    expect(await httpApi.agents.modules(pid)).toHaveLength(1)
    http.get.mockResolvedValueOnce({ data: {} })
    expect(await httpApi.agents.modules(pid)).toEqual([])
    http.get.mockResolvedValueOnce({
      data: { agents: [{ id: 'a', name: 'n', module: 'm', tool_ids: ['t'], team_enabled: 1, created_at: 'c', updated_at: 'u' }] }
    })
    expect((await httpApi.agents.list(pid))[0].team_enabled).toBe(true)
    http.get.mockResolvedValueOnce({ data: {} })
    expect(await httpApi.agents.list(pid)).toEqual([])
    http.post.mockResolvedValueOnce({ data: { id: 'a' } })
    await httpApi.agents.create(pid, { name: 'n' })
    http.patch.mockResolvedValueOnce({ data: { id: 'a' } })
    await httpApi.agents.patch(pid, 'a', { name: 'n2' })
    http.delete.mockResolvedValueOnce({ data: {} })
    await httpApi.agents.remove(pid, 'a')
    http.get.mockResolvedValueOnce({
      data: { default_model: 'm', models: [{ provider: 'p', name: 'n' }, {}] }
    })
    expect(await httpApi.agents.models(pid)).toEqual({
      default_model: 'm',
      models: [
        { provider: 'p', name: 'n' },
        { provider: '', name: '' }
      ]
    })
    http.get.mockResolvedValueOnce({ data: {} })
    expect(await httpApi.agents.models(pid)).toEqual({ default_model: '', models: [] })

    http.get.mockResolvedValueOnce({ data: { threads: [{ id: 't', title: 'x', created_at: 'c', updated_at: 'u' }] } })
    expect((await httpApi.agentThreads.list(pid))[0].id).toBe('t')
    http.get.mockResolvedValueOnce({ data: {} })
    expect(await httpApi.agentThreads.list(pid)).toEqual([])
    http.get.mockResolvedValueOnce({ data: { threads: [{ id: 't', title: 'x', last_message_preview: '预览' }], next_cursor: 't' } })
    expect((await httpApi.agentThreads.page(pid, 1)).threads[0].last_message_preview).toBe('预览')
    http.get.mockResolvedValueOnce({ data: {} })
    expect((await httpApi.agentThreads.page(pid)).next_cursor).toBe('')
    http.post.mockResolvedValueOnce({ data: { id: 't' } })
    await httpApi.agentThreads.create(pid, 'title')
    http.patch.mockResolvedValueOnce({ data: { id: 't', title: 'new' } })
    expect((await httpApi.agentThreads.rename(pid, 't', 'new')).title).toBe('new')
    http.get.mockResolvedValueOnce({ data: { runs: [{ id: 'r', duration_ms: 20 }] } })
    expect((await httpApi.agentThreads.runs(pid, 't'))[0].duration_ms).toBe(20)
    http.get.mockResolvedValueOnce({ data: {} })
    expect(await httpApi.agentThreads.runs(pid, 't')).toEqual([])
    http.delete.mockResolvedValueOnce({ data: {} })
    await httpApi.agentThreads.remove(pid, 't')
    http.get.mockResolvedValueOnce({
      data: { messages: [{ id: 'm', role: 'user', content: 'c', mentions: [], tool_calls: [], created_at: 't' }] }
    })
    expect((await httpApi.agentThreads.messages(pid, 't'))[0].role).toBe('user')
    http.get.mockResolvedValueOnce({ data: {} })
    expect(await httpApi.agentThreads.messages(pid, 't')).toEqual([])
    http.post.mockResolvedValueOnce({ data: { id: 'r', status: 'canceled' } })
    expect((await httpApi.agentThreads.cancel(pid, 'r')).status).toBe('canceled')

    http.get.mockResolvedValueOnce({
      data: { schedules: [{ id: 's', agent_id: 'a', thread_id: 't', prompt: 'p', cron_expr: '* * * * *', enabled: true }] }
    })
    expect((await httpApi.agentSchedules.list(pid))[0].id).toBe('s')
    http.get.mockResolvedValueOnce({ data: {} })
    expect(await httpApi.agentSchedules.list(pid)).toEqual([])
    http.post.mockResolvedValueOnce({ data: { id: 's' } })
    await httpApi.agentSchedules.create(pid, { agent_id: 'a', prompt: 'p', cron_expr: '* * * * *' })
    http.patch.mockResolvedValueOnce({ data: { id: 's' } })
    await httpApi.agentSchedules.patch(pid, 's', { enabled: false })
    http.delete.mockResolvedValueOnce({ data: {} })
    await httpApi.agentSchedules.remove(pid, 's')
    http.get.mockResolvedValueOnce({ data: { runs: [{ id: 'r', schedule_id: 's', run_id: 'x', trigger: 'manual', status: 'ok' }] } })
    expect((await httpApi.agentSchedules.runs(pid, 's'))[0].trigger).toBe('manual')
    http.get.mockResolvedValueOnce({ data: {} })
    expect(await httpApi.agentSchedules.runs(pid, 's')).toEqual([])
    http.post.mockResolvedValueOnce({ data: {} })
    await httpApi.agentSchedules.trigger(pid, 's')
    http.post.mockResolvedValueOnce({ data: {} })
    expect((await httpApi.sql.execute(pid, db, { sql: 'update x' })).rowsAffected).toBe(0)
    http.post.mockResolvedValueOnce({ data: { results: [{}] } })
    expect((await httpApi.sql.batch(pid, db, { statements: [{ sql: 's' }], transactional: false })).results[0].index).toBe(0)
    http.get.mockResolvedValueOnce({
      data: { functions: [{}] }
    })
    expect((await httpApi.gofunctions.list(pid))[0].exports).toEqual([])
    http.get.mockResolvedValueOnce({
      data: {
        functions: [
          {
            id: 'g',
            name: 'g',
            active_version: 2,
            latest_version: 3,
            published: true,
            exports: ['A'],
            versions: [{ version: 2, exports: ['A'], note: 'n', created_at: 't', active: true, source: 'src' }]
          }
        ]
      }
    })
    expect((await httpApi.gofunctions.list(pid))[0].versions?.[0].active).toBe(true)
    http.get.mockResolvedValueOnce({ data: { jobs: [{}] } })
    const sparseJob = (await httpApi.cronjobs.list(pid))[0]
    expect(sparseJob.scheduleKind).toBe('cron')
    http.get.mockResolvedValueOnce({ data: { runs: [{}] } })
    expect((await httpApi.cronjobs.runs(pid, 'j'))[0].trigger).toBe('scheduled')
    http.get.mockResolvedValueOnce({ data: { events: [{}] } })
    expect((await httpApi.logs.list(pid))[0].id).toBe('')
    http.get.mockResolvedValueOnce({ data: { agents: [{}] } })
    expect((await httpApi.agents.list(pid))[0].tool_ids).toEqual([])
    http.get.mockResolvedValueOnce({ data: { threads: [{}] } })
    expect((await httpApi.agentThreads.list(pid))[0].id).toBe('')
    http.get.mockResolvedValueOnce({ data: { messages: [{}] } })
    expect((await httpApi.agentThreads.messages(pid, 't'))[0].role).toBe('')
    http.get.mockResolvedValueOnce({ data: { schedules: [{}] } })
    expect((await httpApi.agentSchedules.list(pid))[0].id).toBe('')
    http.get.mockResolvedValueOnce({ data: { runs: [{}] } })
    expect((await httpApi.agentSchedules.runs(pid, 's'))[0].id).toBe('')
    http.get.mockResolvedValueOnce({ data: { totalRequests: 3 } })
    expect((await httpApi.metrics.summary(pid)).totalRequests).toBe(3)
    http.get.mockResolvedValueOnce({ data: { points: [{}] } })
    expect((await httpApi.metrics.trend(pid))[0].date).toBe('')
    http.put.mockResolvedValueOnce({ data: { defaultProvider: 'x', defaultModel: 'y', maxTokens: 1 } })
    await httpApi.llmSettings.put(pid, { defaultProvider: 'x' }, 'sk-new')
    http.patch.mockResolvedValueOnce({ data: {} })
    await httpApi.cronjobs.update(pid, 'j', { scheduleKind: 'interval', intervalSeconds: 60, enabled: false })
  })

  it('parses LLM and agent SSE streams including error and abort paths', async () => {
    async function* chunks(parts: string[]) {
      for (const p of parts) {
        yield new TextEncoder().encode(p)
      }
    }
    const makeReader = (parts: string[]) => {
      const it = chunks(parts)[Symbol.asyncIterator]()
      return {
        read: () => it.next().then((r) => ({ done: !!r.done, value: r.value }))
      }
    }

    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    fetchMock.mockResolvedValueOnce({
      ok: true,
      body: {
        getReader: () =>
          makeReader([
            'ignore\n\n',
            'data: not-json\n\n',
            'data: {"choices":[{"delta":{"role":"assistant"},"finish_reason":null}]}\n\n',
            'data: {"choices":[{"delta":{"content":"A"},"finish_reason":null}]}\n\n',
            'data: {"choices":[{"delta":{"content":"B"},"finish_reason":null}]}\n\n',
            // 工具增量分片不属于正文，不应混进 onChunk。
            'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"shell","arguments":""}}]}}]}\n\n',
            'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]}}]}\n\n',
            'data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}\n\n',
            'data: [DONE]\n\n'
          ])
      }
    })
    const chunksSeen: string[] = []
    const ended = vi.fn()
    const conn = httpApi.llm.stream(pid, { messages: [{ role: 'user', content: 'hi' }] }, {
      onChunk: (t) => chunksSeen.push(t),
      onEnd: ended
    })
    await vi.waitFor(() => expect(ended).toHaveBeenCalled())
    expect(chunksSeen.join('')).toBe('AB')
    conn.close()

    // 路径与请求体：OpenAI 兼容端点，必须带 stream:true。
    const firstCall = fetchMock.mock.calls[0]
    expect(String(firstCall[0])).toContain('/v1/chat/completions')
    expect(JSON.parse(firstCall[1].body)).toEqual(
      expect.objectContaining({ stream: true, messages: [{ role: 'user', content: 'hi' }] })
    )

    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 500,
      body: null,
      json: async () => ({ error: { message: 'nope' } })
    })
    const onError = vi.fn()
    httpApi.llm.stream(pid, { messages: [] }, { onError })
    await vi.waitFor(() => expect(onError).toHaveBeenCalled())

    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 502,
      body: null,
      json: async () => {
        throw new Error('bad json')
      }
    })
    const onError2 = vi.fn()
    httpApi.llm.stream(pid, { messages: [] }, { onError: onError2 })
    await vi.waitFor(() => expect(onError2).toHaveBeenCalled())

    fetchMock.mockResolvedValueOnce({
      ok: true,
      headers: { get: () => 'text/event-stream' },
      body: {
        getReader: () =>
          makeReader([
            'data: {"type":"run","run_id":"r1"}\n\n',
            'data: {"type":"token","content":"hi"}\n\n',
            'data: {"type":"chunk","delta":"!"}\n\n',
            'data: {"type":"tool_call","name":"sql","arguments":"{}"}\n\n',
            'data: {"type":"tool_result","name":"sql","content":"ok"}\n\n',
            'data: {"type":"error","message":"boom"}\n\n',
            'data: {"type":"end"}\n\n'
          ])
      }
    })
    const ev: string[] = []
    httpApi.agentThreads.streamRun(pid, 'th', { content: 'x', mentions: [] }, {
      onRun: (id) => ev.push('run:' + id),
      onToken: (t) => ev.push('tok:' + t),
      onToolCall: (n) => ev.push('call:' + n),
      onToolResult: (n) => ev.push('res:' + n),
      onError: () => ev.push('err'),
      onEnd: () => ev.push('end')
    })
    await vi.waitFor(() => expect(ev).toContain('err'))
    expect(ev).not.toContain('end')
    expect(ev).toEqual(expect.arrayContaining(['run:r1', 'tok:hi', 'tok:!', 'call:sql', 'res:sql', 'err']))

    fetchMock.mockResolvedValueOnce({
      ok: true,
      headers: { get: () => 'text/html' },
      body: { getReader: () => makeReader([]) }
    })
    const htmlErr = vi.fn()
    httpApi.agentThreads.streamRun(pid, 'th', { content: 'x', mentions: [] }, { onError: htmlErr })
    await vi.waitFor(() => expect(htmlErr).toHaveBeenCalled())

    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 400,
      headers: { get: () => 'application/json' },
      body: null,
      json: async () => ({ error: { code: 'agent_thread_busy', message: 'busy' } })
    })
    const bad = vi.fn()
    httpApi.agentThreads.streamRun(pid, 'th', { content: 'x', mentions: [] }, { onError: bad })
    await vi.waitFor(() => expect(bad).toHaveBeenCalled())
    expect(bad.mock.calls[0][0].code).toBe('agent_thread_busy')

    fetchMock.mockResolvedValueOnce({
      ok: true,
      body: {
        getReader: () => ({
          read: () => new Promise(() => {})
        })
      }
    })
    const hung = httpApi.llm.stream(pid, { messages: [] }, { onError: vi.fn(), onEnd: vi.fn() })
    hung.close()

    fetchMock.mockRejectedValueOnce(new Error('network'))
    const netErr = vi.fn()
    httpApi.llm.stream(pid, { messages: [] }, { onError: netErr })
    await vi.waitFor(() => expect(netErr).toHaveBeenCalled())

    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 503,
      body: null,
      json: async () => ({ message: 'svc' })
    })
    const llmMsg = vi.fn()
    httpApi.llm.stream(pid, { messages: [] }, { onError: llmMsg })
    await vi.waitFor(() => expect(llmMsg).toHaveBeenCalled())

    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 504,
      body: null,
      json: async () => ({})
    })
    const llmStatus = vi.fn()
    httpApi.llm.stream(pid, { messages: [] }, { onError: llmStatus })
    await vi.waitFor(() => expect(llmStatus).toHaveBeenCalled())

    fetchMock.mockResolvedValueOnce({
      ok: true,
      body: {
        getReader: () =>
          makeReader(['data: \n\n', 'data: {"type":"other"}\n\n', 'data: {"type":"end"}\n\n'])
      }
    })
    const emptyEnded = vi.fn()
    httpApi.llm.stream(pid, { messages: [] }, { onEnd: emptyEnded })
    await vi.waitFor(() => expect(emptyEnded).toHaveBeenCalled())

    fetchMock.mockResolvedValueOnce({
      ok: true,
      headers: { get: () => null },
      body: {
        getReader: () =>
          makeReader([
            'event: ping\n\n',
            'data: \n\n',
            'data: not-json\n\n',
            'data: {"type":"error"}\n\n',
            'data: {"type":"token"}\n\n',
            'data: {"type":"tool_call"}\n\n',
            'data: {"type":"tool_result"}\n\n',
            'data: {"type":"end"}\n\n'
          ])
      }
    })
    const sparse = vi.fn()
    const agentHangClose = httpApi.agentThreads.streamRun(
      pid,
      'th',
      { content: 'x', mentions: [] },
      {
        onError: sparse,
        onToken: vi.fn(),
        onToolCall: vi.fn(),
        onToolResult: vi.fn(),
        onEnd: vi.fn()
      }
    )
    await vi.waitFor(() => expect(sparse).toHaveBeenCalled())
    agentHangClose.close()

    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 418,
      headers: { get: () => 'application/json' },
      body: null,
      json: async () => ({})
    })
    const teapot = vi.fn()
    httpApi.agentThreads.streamRun(pid, 'th', { content: 'x', mentions: [] }, { onError: teapot })
    await vi.waitFor(() => expect(teapot).toHaveBeenCalled())

    fetchMock.mockResolvedValueOnce({
      ok: true,
      headers: { get: () => 'text/event-stream' },
      body: {
        getReader: () => ({
          read: () => new Promise(() => {})
        })
      }
    })
    const hungAgent = httpApi.agentThreads.streamRun(pid, 'th', { content: 'x', mentions: [] }, {
      onError: vi.fn(),
      onEnd: vi.fn()
    })
    hungAgent.close()

    vi.unstubAllGlobals()
  })

  it('covers remaining mapper fallbacks', async () => {
    http.get.mockResolvedValueOnce({ data: { points: null } })
    expect(await httpApi.metrics.trend(pid)).toEqual([])
    http.get.mockResolvedValueOnce({ data: { projects: [{}] } })
    expect((await httpApi.projects.list())[0].id).toBe('')
    http.get.mockResolvedValueOnce({ data: { functions: null } })
    expect(await httpApi.gofunctions.list(pid)).toEqual([])
    http.get.mockResolvedValueOnce({ data: { jobs: null } })
    expect(await httpApi.cronjobs.list(pid)).toEqual([])
    http.get.mockResolvedValueOnce({ data: { nope: true } })
    expect(await httpApi.logs.list(pid)).toEqual([])
  })

  it('reconnects an agent run and posts confirmation', async () => {
    async function* chunks(parts: string[]) {
      for (const p of parts) yield new TextEncoder().encode(p)
    }
    const makeReader = (parts: string[]) => {
      const it = chunks(parts)[Symbol.asyncIterator]()
      return { read: () => it.next().then((r) => ({ done: !!r.done, value: r.value })) }
    }
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    let hold = false
    const held: Array<() => void> = []
    vi.spyOn(globalThis, 'setTimeout').mockImplementation(((fn: TimerHandler) => {
      if (hold) {
        held.push(() => (fn as () => void)())
        return held.length as unknown as ReturnType<typeof setTimeout>
      }
      queueMicrotask(() => (fn as () => void)())
      return 0 as unknown as ReturnType<typeof setTimeout>
    }) as unknown as typeof setTimeout)

    fetchMock.mockResolvedValueOnce({
      ok: true,
      headers: { get: () => 'text/event-stream' },
      body: {
        getReader: () => {
          const encoded = [
            new TextEncoder().encode('id: 7\n: ping\ndata: {"type":"run","run_id":"r9"}\n\n'),
            new TextEncoder().encode('id: 8\ndata: {"type":"tool_call_delta","name":"simplebase","arguments":"{","call_id":"c1"}\n\n')
          ]
          let i = 0
          return {
            read: async () => {
              if (i < encoded.length) return { done: false, value: encoded[i++] }
              throw new Error('socket')
            }
          }
        }
      }
    })
    fetchMock.mockResolvedValueOnce({
      ok: true,
      headers: { get: () => 'text/event-stream' },
      body: {
        getReader: () => makeReader([
          'id: 9\ndata: {"type":"tool_start","name":"simplebase","call_id":"c1"}\n\n',
          'id: 10\ndata: {"type":"confirmation_required","call_id":"c1","argv":["database","delete"],"message":"sure?"}\n\n',
          'id: 11\ndata: {"type":"confirmation_resolved","call_id":"c1","approve":true}\n\n',
          'id: 12\ndata: {"type":"tool_progress","name":"simplebase","elapsed_ms":4,"call_id":"c1"}\n\n',
          'id: 13\ndata: {"type":"tool_result","name":"simplebase","content":"{}","is_error":false,"duration_ms":1,"call_id":"c1"}\n\n',
          'id: 14\ndata: {"type":"thinking","elapsed_ms":2,"content":"..."}\n\n',
          'id: 15\ndata: {"type":"usage","duration_ms":1,"prompt_tokens":1,"completion_tokens":1,"reasoning_tokens":0,"tool_calls":1}\n\n',
          'id: 16\ndata: {"type":"end","reason":"stop"}\n\n'
        ])
      }
    })
    const seen: string[] = []
    httpApi.agentThreads.streamRun(pid, 'th', { content: '删库', mentions: [] }, {
      onRun: () => seen.push('run'),
      onToolCallDelta: () => seen.push('delta'),
      onToolStart: () => seen.push('start'),
      onConfirmationRequired: () => seen.push('confirm'),
      onConfirmationResolved: () => seen.push('resolved'),
      onToolProgress: () => seen.push('progress'),
      onToolResult: (_n, _c, _id, _d, isErr) => seen.push('result:' + String(isErr)),
      onThinking: () => seen.push('think'),
      onUsage: () => seen.push('usage'),
      onEnd: (reason) => seen.push('end:' + reason),
      onError: () => seen.push('err')
    })
    await vi.waitFor(() => expect(seen).toContain('end:stop'))
    expect(seen).not.toContain('err')
    const replay = String(fetchMock.mock.calls[1][0])
    expect(replay).toContain('/agent-runs/r9/events?after=8')
    const firstBody = JSON.parse(String(fetchMock.mock.calls[0][1].body))
    // 技能由后端按助手 module 决定，前端不再传 skills。
    expect(firstBody.skills).toBeUndefined()
    expect(firstBody.content).toBe('删库')

    hold = true
    const heldBefore = held.length
    fetchMock.mockResolvedValueOnce({
      ok: true,
      headers: { get: () => 'text/event-stream' },
      body: {
        getReader: () => {
          let sent = false
          return {
            read: async () => {
              if (!sent) {
                sent = true
                return { done: false, value: new TextEncoder().encode('id: 1\ndata: {"type":"run","run_id":"r-abort"}\n\n') }
              }
              throw new Error('socket')
            }
          }
        }
      }
    })
    const aborted = vi.fn()
    const conn = httpApi.agentThreads.streamRun(pid, 'th', { content: 'x', mentions: [] }, { onError: aborted })
    await vi.waitFor(() => expect(held.length).toBeGreaterThan(heldBefore))
    conn.close()
    held.splice(heldBefore).forEach((fire) => fire())
    await Promise.resolve()
    expect(aborted).not.toHaveBeenCalled()
    hold = false

    for (let n = 0; n < 4; n++) {
      fetchMock.mockResolvedValueOnce({
        ok: n === 0,
        status: n === 0 ? 200 : 500,
        headers: { get: () => 'text/event-stream' },
        body: n === 0
          ? {
              getReader: () => {
                let sent = false
                return {
                  read: async () => {
                    if (!sent) {
                      sent = true
                      return { done: false, value: new TextEncoder().encode('id: 2\ndata: {"type":"run","run_id":"r-fail"}\n\n') }
                    }
                    throw new Error('socket')
                  }
                }
              }
            }
          : null
      })
    }
    const failed = vi.fn()
    httpApi.agentThreads.streamRun(pid, 'th', { content: 'x', mentions: [] }, { onError: failed })
    await vi.waitFor(() => expect(failed).toHaveBeenCalled(), { timeout: 3000 })

    vi.unstubAllGlobals()
    vi.mocked(globalThis.setTimeout).mockRestore()

    http.post.mockResolvedValueOnce({ data: {} })
    await httpApi.agentThreads.confirm(pid, 'run 1', 'call 1', true)
    expect(http.post.mock.calls.at(-1)?.[0]).toContain('/agent-runs/run%201/confirmations/call%201')
    expect(http.post.mock.calls.at(-1)?.[1]).toEqual({ approve: true })
  })

  it('reconnects after premature EOF and requires an end frame from the replay', async () => {
    const timer = vi.spyOn(globalThis, 'setTimeout').mockImplementation(((fn: TimerHandler) => {
      queueMicrotask(() => (fn as () => void)())
      return 0 as unknown as ReturnType<typeof setTimeout>
    }) as unknown as typeof setTimeout)
    const encoder = new TextEncoder()
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        headers: { get: () => 'text/event-stream' },
        body: { getReader: () => {
          let sent = false
          return { read: async () => {
            if (!sent) {
              sent = true
              return { done: false, value: encoder.encode('id: 1\ndata: {"type":"run","run_id":"r1"}\n\n') }
            }
            return { done: true }
          } }
        } }
      })
      .mockResolvedValueOnce({
        ok: true,
        headers: { get: () => 'text/event-stream' },
        body: { getReader: () => {
          let sent = false
          return { read: async () => {
            if (!sent) {
              sent = true
              return { done: false, value: encoder.encode('id: 2\ndata: {"type":"token","content":"ok"}\n\nid: 3\ndata: {"type":"end"}\n\n') }
            }
            return { done: true }
          } }
        } }
      })
    vi.stubGlobal('fetch', fetchMock)
    try {
      const onEnd = vi.fn()
      const onError = vi.fn()
      const onToken = vi.fn()
      httpApi.agentThreads.streamRun(pid, 'th', { content: 'x', mentions: [] }, { onEnd, onError, onToken })
      await vi.waitFor(() => expect(onEnd).toHaveBeenCalledTimes(1))
      expect(onError).not.toHaveBeenCalled()
      expect(onToken).toHaveBeenCalledWith('ok')
      expect(String(fetchMock.mock.calls[1][0])).toContain('/agent-runs/r1/events?after=1')
    } finally {
      vi.unstubAllGlobals()
      timer.mockRestore()
    }
  })

  // planv6.0 前端简化 C1：锁定各事件传给回调的实参（含稀疏/异常字段）及 usage 数值转换。
  it('forwards exact handler arguments for sparse agent stream events', async () => {
    const encoder = new TextEncoder()
    const lines = [
      'data: {"type":"usage","duration_ms":"12","prompt_tokens":null,"completion_tokens":"x","tool_calls":2}\n\n',
      'data: {"type":"usage"}\n\n',
      'data: {"type":"thinking"}\n\n',
      'data: {"type":"chunk","delta":"d"}\n\n',
      'data: {"type":"token","content":""}\n\n',
      'data: {"type":"tool_call_delta"}\n\n',
      'data: {"type":"tool_start","name":"n","call_id":""}\n\n',
      'data: {"type":"tool_call","name":"n","arguments":"{}"}\n\n',
      'data: {"type":"tool_progress","elapsed_ms":"3"}\n\n',
      'data: {"type":"tool_result","is_error":"true","truncated":1}\n\n',
      'data: {"type":"confirmation_required","argv":[1,null]}\n\n',
      'data: {"type":"confirmation_resolved","call_id":"c","approve":"yes"}\n\n',
      'data: {"type":"unknown_kind","x":1}\n\n',
      'data: {"type":"end","reason":5}\n\n'
    ]
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      headers: { get: (key: string) => (key.toLowerCase() === 'content-type' ? 'text/event-stream' : null) },
      body: {
        getReader: () => {
          let i = 0
          return {
            read: async () => (i < lines.length
              ? { done: false, value: encoder.encode(lines[i++]) }
              : { done: true, value: undefined })
          }
        }
      }
    })
    vi.stubGlobal('fetch', fetchMock)
    try {
      const calls: unknown[][] = []
      const rec = (name: string) => (...args: unknown[]) => { calls.push([name, ...args]) }
      const end = vi.fn()
      httpApi.agentThreads.streamRun(pid, 'th', { content: 'x', mentions: [] }, {
        onUsage: rec('usage'),
        onThinking: rec('thinking'),
        onToken: rec('token'),
        onToolCallDelta: rec('delta'),
        onToolStart: rec('start'),
        onToolCall: rec('call'),
        onToolProgress: rec('progress'),
        onToolResult: rec('result'),
        onConfirmationRequired: rec('confirm'),
        onConfirmationResolved: rec('resolved'),
        onError: rec('error'),
        onEnd: end
      })
      await vi.waitFor(() => expect(end).toHaveBeenCalled())
      expect(end).toHaveBeenCalledWith(undefined)
      expect(calls).toEqual([
        // 现状：|| 只兜底 falsy，非数字字符串得到 NaN（按现状锁定，不在简化中修复）。
        ['usage', { duration_ms: 12, prompt_tokens: 0, completion_tokens: NaN, reasoning_tokens: 0, tool_calls: 2 }],
        ['usage', { duration_ms: 0, prompt_tokens: 0, completion_tokens: 0, reasoning_tokens: 0, tool_calls: 0 }],
        ['thinking', 0, undefined],
        ['token', 'd'],
        ['delta', '', '', undefined],
        ['start', 'n', undefined],
        ['call', 'n', '{}', undefined],
        ['progress', '', 3, undefined],
        ['result', '', '', undefined, 0, false, false],
        ['confirm', '', ['1', 'null'], ''],
        ['resolved', 'c', false]
      ])
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('passes truncated through the live agent stream', async () => {
    const encoder = new TextEncoder()
    const lines = [
      'data: {"type":"tool_result","name":"readonly_sql","content":"{}","duration_ms":1,"is_error":false,"truncated":true}\n\n',
      'data: {"type":"end"}\n\n'
    ]
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      headers: { get: (key: string) => (key.toLowerCase() === 'content-type' ? 'text/event-stream' : null) },
      body: {
        getReader: () => {
          let i = 0
          return {
            read: async () => (i < lines.length
              ? { done: false, value: encoder.encode(lines[i++]) }
              : { done: true, value: undefined })
          }
        }
      }
    })
    vi.stubGlobal('fetch', fetchMock)
    try {
      const got: Array<boolean | undefined> = []
      const end = vi.fn()
      httpApi.agentThreads.streamRun(pid, 'th', { content: 'x', mentions: [] }, {
        onToolResult: (_n, _c, _id, _d, _e, truncated) => got.push(truncated),
        onEnd: end
      })
      await vi.waitFor(() => expect(end).toHaveBeenCalled())
      expect(got).toEqual([true])
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('seeds runId from the X-Run-Id header before the first event', async () => {
    // BUG-B5：首个 run 帧之前就断连时，靠响应头才有 runId 可续传。
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        headers: { get: (key: string) => (key.toLowerCase() === 'x-run-id' ? 'run-hdr' : null) },
        body: { getReader: () => ({ read: async () => { throw new Error('socket') } }) }
      })
    vi.stubGlobal('fetch', fetchMock)
    const timer = vi.spyOn(globalThis, 'setTimeout').mockImplementation(((fn: TimerHandler) => {
      queueMicrotask(() => (fn as () => void)())
      return 0 as unknown as ReturnType<typeof setTimeout>
    }) as unknown as typeof setTimeout)
    try {
      const onRun = vi.fn()
      const onError = vi.fn()
      httpApi.agentThreads.streamRun(pid, 'th', { content: 'x', mentions: [] }, { onRun, onError })
      await vi.waitFor(() => expect(onError).toHaveBeenCalled())
      expect(onRun).toHaveBeenCalledWith('run-hdr')
      // 三次重连都失败，但目标 URL 必须带上响应头给出的 runID。
      const replayCalls = fetchMock.mock.calls.map((c) => String(c[0])).filter((u) => u.includes('/agent-runs/'))
      expect(replayCalls.length).toBeGreaterThan(0)
      expect(replayCalls[0]).toContain('/agent-runs/run-hdr/events')
    } finally {
      vi.unstubAllGlobals()
      timer.mockRestore()
    }
  })

  it('reports an incomplete agent stream instead of treating EOF as completion', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      headers: { get: (key: string) => (key.toLowerCase() === 'content-type' ? 'text/event-stream' : null) },
      body: { getReader: () => ({ read: async () => ({ done: true }) }) }
    })
    vi.stubGlobal('fetch', fetchMock)
    try {
      const onEnd = vi.fn()
      const onError = vi.fn()
      httpApi.agentThreads.streamRun(pid, 'th', { content: 'x', mentions: [] }, { onEnd, onError })
      await vi.waitFor(() => expect(onError).toHaveBeenCalledTimes(1))
      expect(onEnd).not.toHaveBeenCalled()
      expect(fetchMock).toHaveBeenCalledTimes(1)
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('falls back to defaults when sandbox and function payloads are sparse', async () => {
    http.get.mockResolvedValueOnce({ data: {} })
    const caps = await httpApi.sandboxes.capabilities(pid)
    expect(caps).toMatchObject({ available: false, backend: '', images: [], cpus: 0, memoryMiB: 0, terminal: false })
    expect(caps.reason).toBeUndefined()
    expect(caps.ssh).toBeUndefined()
    http.get.mockResolvedValueOnce({ data: [{}] })
    const list = await httpApi.sandboxes.list(pid)
    expect(list).toHaveLength(1)
    expect(list[0]).toMatchObject({ id: '', name: '', state: 'paused', status: '', source: '', cpuCount: 0,
      lastActiveAt: undefined, lastError: undefined })
    http.get.mockResolvedValueOnce({ data: [{ sandboxID: 's', state: 'running', metadata: { 'simplebase.source': 'console' } }] })
    expect((await httpApi.sandboxes.list(pid))[0]).toMatchObject({ id: 's', source: 'console', endAt: undefined })
    http.post.mockResolvedValueOnce({ data: {} })
    await httpApi.sandboxes.create(pid, {})
    expect(http.post.mock.calls[0][1]).toMatchObject({ templateID: '', metadata: {} })
    expect(http.post.mock.calls.at(-1)?.[2]).toMatchObject({ timeout: 90000 })
    http.post.mockResolvedValueOnce({ data: { entries: [{}] } })
    expect((await httpApi.sandboxes.files.list(pid, 's', '/'))[0]).toEqual({
      name: '', path: '', kind: 'file', size: 0, permissions: '', modifiedTime: undefined
    })

    http.get.mockResolvedValueOnce({ data: undefined })
    expect(await httpApi.gofunctions.listVersions(pid, 'n')).toEqual({ activeVersion: 0, versions: [] })
    http.post.mockResolvedValueOnce({ data: undefined })
    expect(await httpApi.gofunctions.activate(pid, 'n', 3)).toEqual({ activeVersion: 3 })
    http.post.mockResolvedValueOnce({ data: undefined })
    expect(await httpApi.gofunctions.test(pid, 'n', 2, 'Fn', {})).toMatchObject({
      ok: false, statusCode: 0, durationMs: 0, version: 2, activeVersion: 0, functionName: 'Fn', error: ''
    })
  })

  it('covers agentRuns.active / attachAgentRun and storage endpoints', async () => {
    // agentThreads.active：runs 数组正常映射，非数组回退为空
    http.get.mockResolvedValueOnce({ data: { runs: [{ run_id: 'r1', agent_id: 'ag1' }] } })
    const actives = await httpApi.agentThreads.active(pid)
    expect(actives).toHaveLength(1)
    expect(http.get.mock.calls[0][0]).toBe(`/v1/projects/${encodeURIComponent(pid)}/agent-runs/active`)
    http.get.mockResolvedValueOnce({ data: { runs: null } })
    expect(await httpApi.agentThreads.active(pid)).toEqual([])

    // attachAgentRun：透传 SSE 事件流（fetch 流式解析）
    const sseChunks = [
      `data: ${JSON.stringify({ type: 'run', run_id: 'r1' })}\n\n`,
      `data: ${JSON.stringify({ type: 'token', content: 'hi' })}\n\n`,
      `data: ${JSON.stringify({ type: 'end' })}\n\n`
    ]
    const encoder = new TextEncoder()
    let sseIdx = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve({
          ok: true,
          headers: new Map([['content-type', 'text/event-stream']]),
          body: {
            getReader: () => ({
              read: () =>
                Promise.resolve(
                  sseIdx < sseChunks.length
                    ? { done: false, value: encoder.encode(sseChunks[sseIdx++]) }
                    : { done: true, value: undefined }
                )
            })
          },
          status: 200
        })
      )
    )
    try {
      const events: Record<string, unknown>[] = []
      const conn = httpApi.agentThreads.attachRun(pid, 'r1', {
        onRun: (id) => events.push({ type: 'run', id }),
        onToken: (t) => events.push({ type: 'token', t }),
        onEnd: () => events.push({ type: 'end' })
      })
      await new Promise((r) => setTimeout(r, 20))
      expect(events).toEqual([{ type: 'run', id: 'r1' }, { type: 'token', t: 'hi' }, { type: 'end' }])
      conn.close()
    } finally {
      vi.unstubAllGlobals()
    }

    // storage.get / storage.put
    http.get.mockResolvedValueOnce({
      data: { database: 'local', files: 'local', data_dir: './data', cache_dir: './cache',
        writable_config: true, bucket: '', prefix: '', source: {} }
    })
    const s = await httpApi.storage.get()
    expect(s).toMatchObject({ database: 'local', data_dir: './data', writable_config: true })
    expect(http.get.mock.calls.at(-1)?.[0]).toBe('/v1/system/storage')

    http.put.mockResolvedValueOnce({ data: { ok: true } })
    const putRes = await httpApi.storage.put({ database: 's3', files: 'local', data_dir: './data', confirm_reset: true })
    expect(putRes).toEqual({ ok: true })
    expect(http.put.mock.calls[0][0]).toBe('/v1/system/storage')
  })
})
