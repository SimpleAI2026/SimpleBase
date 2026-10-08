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

  it('maps sandbox resources, execution and file paths', async () => {
    const row = { id: 'sb-1', name: 'demo', cloud_name: 'vm-1', source: 'api', status: 'pending',
      image: 'python:3.12-slim', cpus: 1, memory_mib: 256, idle_timeout_s: 300, created_at: 'today' }
    http.get.mockResolvedValueOnce({ data: { available: true, backend: 'fake', images: ['python:3.12-slim'],
      default_image: 'python:3.12-slim', network_options: ['none'] } })
    expect((await httpApi.sandboxes.capabilities(pid)).available).toBe(true)
    http.get.mockResolvedValueOnce({ data: { sandboxes: [row] } })
    expect((await httpApi.sandboxes.list(pid))[0].memoryMiB).toBe(256)
    http.post.mockResolvedValueOnce({ data: row })
    await httpApi.sandboxes.create(pid, { name: 'demo' }, 'idem')
    expect(http.post.mock.calls[0][2].headers['Idempotency-Key']).toBe('idem')
    http.post.mockResolvedValueOnce({ data: row })
    await httpApi.sandboxes.start(pid, row.id)
    http.post.mockResolvedValueOnce({ data: row })
    await httpApi.sandboxes.stop(pid, row.id)
    http.post.mockResolvedValueOnce({ data: { stdout: 'ok', exit_code: 3, duration_ms: 2 } })
    expect((await httpApi.sandboxes.exec(pid, row.id, { command: 'exit 3' })).exitCode).toBe(3)
    http.get.mockResolvedValueOnce({ data: { entries: [{ name: 'a.txt', path: '/workspace/a.txt', kind: 'file', size: 2 }] } })
    expect((await httpApi.sandboxes.files.list(pid, row.id, '/workspace'))[0].name).toBe('a.txt')
    http.get.mockResolvedValueOnce({ data: { content: 'hi', encoding: 'utf8' } })
    expect((await httpApi.sandboxes.files.read(pid, row.id, '/workspace/a.txt')).content).toBe('hi')
    http.put.mockResolvedValueOnce({ data: undefined })
    await httpApi.sandboxes.files.write(pid, row.id, '/workspace/a.txt', 'hi')
    http.put.mockResolvedValueOnce({ data: undefined })
    await httpApi.sandboxes.files.upload(pid, row.id, '/workspace/data.bin', new Uint8Array([0, 255]))
    expect(http.put.mock.calls[1][1]).toBeInstanceOf(Blob)
    expect(http.put.mock.calls[1][2].headers['Content-Type']).toBe('application/octet-stream')
    http.delete.mockResolvedValueOnce({ data: undefined })
    await httpApi.sandboxes.files.remove(pid, row.id, '/workspace/a.txt')
    http.delete.mockResolvedValueOnce({ data: undefined })
    await httpApi.sandboxes.remove(pid, row.id)
    expect(http.get.mock.calls[2][1].params.path).toBe('/workspace')
    expect(http.put.mock.calls[0][2].params.path).toBe('/workspace/a.txt')
  })

  it('follows sandbox list cursors until the last page', async () => {
    http.get.mockResolvedValueOnce({ data: { sandboxes: [{ id: 'a' }], next_cursor: 'a' } })
    http.get.mockResolvedValueOnce({ data: { sandboxes: [{ id: 'b' }] } })
    const rows = await httpApi.sandboxes.list(pid, 'running', 'api')
    expect(rows.map((r) => r.id)).toEqual(['a', 'b'])
    expect(http.get).toHaveBeenCalledTimes(2)
    expect(http.get.mock.calls[0][1].params).toEqual({ status: 'running', source: 'api', limit: 100, cursor: undefined })
    expect(http.get.mock.calls[1][1].params.cursor).toBe('a')
    http.get.mockResolvedValueOnce({ data: {} })
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

    http.get.mockResolvedValueOnce({
      data: {
        databases: [{ id: 'd', name: 'n', status: 'ready', created_at: 'a', updated_at: 'b', snapshot: { last_synced_snapshot: 1, sync_lag: 2 } }]
      }
    })
    expect((await httpApi.databases.list(pid))[0].snapshot?.syncLag).toBe(2)
    http.get.mockResolvedValueOnce({ data: {} })
    expect(await httpApi.databases.list(pid)).toEqual([])
    http.post.mockResolvedValueOnce({ data: { id: 'd', name: 'n', status: 'creating' } })
    await httpApi.databases.create(pid, 'n')
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
    http.get.mockResolvedValueOnce({ data: { keep_days: 7, updated_at: 't', scope: 'project' } })
    expect((await httpApi.logs.getRetention(pid)).keepDays).toBe(7)
    http.get.mockResolvedValueOnce({ data: undefined })
    expect((await httpApi.logs.getRetention(pid)).keepDays).toBe(14)
    http.put.mockResolvedValueOnce({ data: {} })
    await httpApi.logs.putRetention(pid, 30)

    http.get.mockResolvedValueOnce({ data: { llm_allowed: true, database_allowed: false } })
    expect((await httpApi.quota.status(pid)).llmAllowed).toBe(true)

    http.post.mockResolvedValueOnce({ data: { content: 'hi' } })
    expect(await httpApi.llm.chat(pid, { messages: [{ role: 'user', content: 'hi' }], maxTokens: 1, temperature: 0 })).toEqual({
      content: 'hi'
    })
    http.get.mockResolvedValueOnce({ data: { default_provider: 'openai', default_model: 'm', max_tokens: 8 } })
    expect((await httpApi.llmSettings.get(pid)).maxTokens).toBe(8)
    http.get.mockResolvedValueOnce({ data: undefined })
    expect(await httpApi.llmSettings.get(pid)).toEqual({})
    http.put.mockResolvedValueOnce({ data: { defaultProvider: 'openai' } })
    await httpApi.llmSettings.put(pid, { defaultProvider: 'openai', defaultModel: 'm', temperature: 1, maxTokens: 2 })
    http.put.mockResolvedValueOnce({ data: {} })
    await httpApi.llmSettings.put(pid, {})

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
    expect((await httpApi.sql.batch(pid, db, { statements: [{ sql: 's' }] })).results[0].index).toBe(0)
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
    await httpApi.llmSettings.put(pid, { defaultProvider: 'x' })
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
            'data: {"delta":"A"}\n\n',
            'data: {"content":"B"}\n\n',
            'data: {"choices":[{"delta":{"content":"C"}}]}\n\n',
            'data: {"type":"end"}\n\n'
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
    expect(chunksSeen.join('')).toBe('ABC')
    conn.close()

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

  it('falls back to defaults when sandbox and function payloads are sparse', async () => {
    http.get.mockResolvedValueOnce({ data: {} })
    const caps = await httpApi.sandboxes.capabilities(pid)
    expect(caps).toMatchObject({ available: false, backend: 'cloud', images: [], cpusMax: 4, networkOptions: ['none'] })
    http.get.mockResolvedValueOnce({ data: { sandboxes: [{}], next_cursor: 'c2' } }).mockResolvedValueOnce({ data: {} })
    const list = await httpApi.sandboxes.list(pid)
    expect(list).toHaveLength(1)
    expect(list[0]).toMatchObject({ id: '', network: 'none', cpus: 0, threadId: undefined, lastError: undefined })
    http.get.mockResolvedValueOnce({ data: { sandboxes: [{ id: 's', thread_id: 't', last_error: 'e', started_at: 'a', last_active_at: 'b', expires_at: 'c' }] } })
    expect((await httpApi.sandboxes.list(pid))[0]).toMatchObject({ threadId: 't', lastError: 'e', expiresAt: 'c' })
    http.post.mockResolvedValueOnce({ data: {} })
    expect(await httpApi.sandboxes.exec(pid, 's', { command: 'x', timeoutS: 5 })).toMatchObject({ exitCode: 0, stdout: '', status: '' })
    http.post.mockResolvedValueOnce({ data: {} })
    await httpApi.sandboxes.create(pid, { name: 'n', start: true })
    expect(http.post.mock.calls.at(-1)?.[2]).toMatchObject({ headers: undefined, timeout: 90000 })
    http.get.mockResolvedValueOnce({ data: {} })
    expect(await httpApi.sandboxes.files.list(pid, 's', '/')).toEqual([])
    http.get.mockResolvedValueOnce({ data: { entries: [{}] } })
    expect((await httpApi.sandboxes.files.list(pid, 's', '/'))[0]).toEqual({ name: '', path: '', kind: '', size: 0 })
    http.get.mockResolvedValueOnce({ data: undefined })
    expect(await httpApi.sandboxes.files.read(pid, 's', '/a')).toEqual({ content: '', encoding: 'utf8', truncated: false })

    http.get.mockResolvedValueOnce({ data: undefined })
    expect(await httpApi.gofunctions.listVersions(pid, 'n')).toEqual({ activeVersion: 0, versions: [] })
    http.post.mockResolvedValueOnce({ data: undefined })
    expect(await httpApi.gofunctions.activate(pid, 'n', 3)).toEqual({ activeVersion: 3 })
    http.post.mockResolvedValueOnce({ data: undefined })
    expect(await httpApi.gofunctions.test(pid, 'n', 2, 'Fn', {})).toMatchObject({
      ok: false, statusCode: 0, durationMs: 0, version: 2, activeVersion: 0, functionName: 'Fn', error: ''
    })
  })
})
