import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mockApi } from './mock'

const PID = '00000000-0000-0000-0000-000000000002'

describe('mockApi', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  async function flush<T>(p: Promise<T>): Promise<T> {
    const done = p
    await vi.runAllTimersAsync()
    return done
  }

  async function flushReject(p: Promise<unknown>, match?: RegExp | string) {
    const assertion = match ? expect(p).rejects.toThrow(match) : expect(p).rejects.toThrow()
    await vi.runAllTimersAsync()
    await assertion
  }

  it('covers projects, databases, sql, and document CRUD', async () => {
    expect((await flush(mockApi.projects.list())).length).toBeGreaterThan(0)
    await flushReject(mockApi.projects.create({ name: '' }), /name/)
    const created = await flush(mockApi.projects.create({ name: 'new-proj-' + Date.now() }))
    await flushReject(mockApi.projects.create({ name: created.name }), /exists/)

    expect((await flush(mockApi.databases.list(PID))).length).toBeGreaterThan(0)
    const db = await flush(mockApi.databases.create(PID, 'extra'))
    expect((await flush(mockApi.databases.get(PID, db.id))).id).toBe(db.id)
    await flushReject(mockApi.databases.get(PID, 'missing'))
    expect((await flush(mockApi.databases.open(PID, db.id))).status).toBe('ready')
    await flush(mockApi.databases.close(PID, db.id))
    await flushReject(mockApi.databases.open(PID, 'missing'))
    await flushReject(mockApi.databases.close(PID, 'missing'))

    const users = await flush(mockApi.sql.query(PID, 'db-default', { sql: 'select * from users' }))
    expect(users.rowCount).toBeGreaterThan(0)
    const schema = await flush(mockApi.sql.query(PID, 'db-default', { sql: 'select * from information_schema.tables' }))
    expect(schema.columns).toContain('table_name')
    expect((await flush(mockApi.sql.query(PID, 'db-default', { sql: 'select 1' }))).rowCount).toBe(0)
    expect((await flush(mockApi.sql.execute(PID, 'db-default', { sql: 'update x' }))).durability).toBeTruthy()
    expect(
      (await flush(mockApi.sql.batch(PID, 'db-default', { statements: [{ sql: 'a' }, { sql: 'b' }], transactional: true })))
        .results
    ).toHaveLength(2)

    expect((await flush(mockApi.db.collections(PID, 'db-default'))).includes('users')).toBe(true)
    await flush(mockApi.db.createCollection(PID, db.id, 'items'))
    await flush(mockApi.db.createCollection(PID, db.id, 'items'))
    const row = await flush(mockApi.db.insert(PID, db.id, 'fresh', { name: 'x' }))
    expect((await flush(mockApi.db.rows(PID, db.id, 'fresh')))[0].id).toBe(row.id)
    expect((await flush(mockApi.db.update(PID, db.id, 'fresh', row.id, { name: 'y' }))).name).toBe('y')
    await flushReject(mockApi.db.update(PID, db.id, 'fresh', 'nope', {}))
    await flush(mockApi.db.remove(PID, db.id, 'fresh', row.id))
    await flush(mockApi.databases.remove(PID, db.id))
    await flush(mockApi.databases.remove(PID, 'already-gone'))
    await flushReject(mockApi.db.collections(PID, 'missing-db'))
  })

  it('covers s3, logs, quota, llm, and settings', async () => {
    expect((await flush(mockApi.s3.list(PID, 'images/'))).every((o) => o.key.startsWith('images/'))).toBe(true)
    expect((await flush(mockApi.s3.presign(PID, 'k'))).url).toContain('mock-cdn')
    const uploaded = await flush(mockApi.s3.upload(PID, 'tmp/a.bin', { size: 12 }))
    expect(uploaded.size).toBe(12)
    await flush(mockApi.s3.remove(PID, 'tmp/a.bin'))

    const logs = await flush(mockApi.logs.list(PID, { level: 'error', q: 'query', from: '2000-01-01', to: '2099-01-01', limit: 5 }))
    expect(Array.isArray(logs)).toBe(true)
    expect((await flush(mockApi.logs.list(PID))).length).toBeGreaterThan(0)
    expect((await flush(mockApi.logs.getRetention(PID))).keepDays).toBe(14)
    await flush(mockApi.logs.putRetention(PID, 21))
    expect((await flush(mockApi.logs.getRetention(PID))).keepDays).toBe(21)
    expect((await flush(mockApi.logs.getRetention('other'))).keepDays).toBe(14)
    expect((await flush(mockApi.quota.status(PID))).llmAllowed).toBe(true)
    expect(await flush(mockApi.llm.providers(PID))).toContain('openai')
    const chat = await flush(mockApi.llm.chat(PID, { messages: [{ role: 'user', content: 'hello world' }] }))
    expect(chat.content).toContain('hello')

    const chunks: string[] = []
    const ended = vi.fn()
    const conn = mockApi.llm.stream(PID, { messages: [{ role: 'user', content: 'sse' }] }, {
      onChunk: (t) => chunks.push(t),
      onEnd: ended
    })
    await vi.runAllTimersAsync()
    expect(chunks.join('').length).toBeGreaterThan(0)
    expect(ended).toHaveBeenCalled()
    const conn2 = mockApi.llm.stream(PID, { messages: [] }, {
      onChunk: () => {
        throw new Error('chunk fail')
      },
      onError: vi.fn()
    })
    await vi.runAllTimersAsync()
    conn.close()
    conn2.close()

    expect((await flush(mockApi.llmSettings.get(PID))).defaultProvider).toBe('openai')
    expect((await flush(mockApi.llmSettings.put(PID, { defaultProvider: 'anthropic', defaultModel: 'c' }))).defaultProvider).toBe(
      'anthropic'
    )
  })

  it('covers gofunctions and cronjobs including validation errors', async () => {
    expect((await flush(mockApi.gofunctions.list(PID)))[0].name).toBe('hello')
    await flushReject(mockApi.gofunctions.create(PID, { name: 'bad name', source: 'x' }))
    await flushReject(mockApi.gofunctions.create(PID, { name: 'hello', source: 'func Hello() {}' }), /exists/)
    await flushReject(mockApi.gofunctions.create(PID, { name: 'emptyfn', source: 'package main' }), /导出/)
    const created = await flush(
      mockApi.gofunctions.create(PID, { name: 'Echo', source: 'func Echo(req T) R {}\nfunc helper() {}' })
    )
    expect(created.exports).toContain('Echo')
    expect((await flush(mockApi.gofunctions.get(PID, 'Echo'))).source).toBeTruthy()
    await flushReject(mockApi.gofunctions.get(PID, 'missing'))
    await flushReject(mockApi.gofunctions.update(PID, 'Echo', 'package main'))
    expect((await flush(mockApi.gofunctions.update(PID, 'Echo', 'func Ping() int { return 1 }'))).exports).toContain('Ping')
    await flushReject(mockApi.gofunctions.update(PID, 'missing', 'func X() {}'))
    await flush(mockApi.gofunctions.remove(PID, 'Echo'))
    await flushReject(mockApi.gofunctions.remove(PID, 'Echo'))

    const jobs = await flush(mockApi.cronjobs.list(PID))
    expect(jobs[0].name).toBe('nightly-hello')
    await flushReject(mockApi.cronjobs.create(PID, { name: 'bad name', funcFile: 'hello', funcExport: 'Hello', scheduleKind: 'cron', cronExpr: '* * * * *' }))
    await flushReject(
      mockApi.cronjobs.create(PID, { name: 'nightly-hello', funcFile: 'hello', funcExport: 'Hello', scheduleKind: 'cron', cronExpr: '* * * * *' }),
      /exists/
    )
    await flushReject(
      mockApi.cronjobs.create(PID, { name: 'NoFn', funcFile: 'missing', funcExport: 'Hello', scheduleKind: 'cron', cronExpr: '* * * * *' }),
      /不存在/
    )
    await flushReject(
      mockApi.cronjobs.create(PID, { name: 'BadExport', funcFile: 'hello', funcExport: 'Nope', scheduleKind: 'cron', cronExpr: '* * * * *' }),
      /未导出/
    )
    await flushReject(
      mockApi.cronjobs.create(PID, { name: 'BadCron', funcFile: 'hello', funcExport: 'Hello', scheduleKind: 'cron', cronExpr: 'bad' }),
      /5 字段/
    )
    await flushReject(
      mockApi.cronjobs.create(PID, { name: 'BadInt', funcFile: 'hello', funcExport: 'Hello', scheduleKind: 'interval', intervalSeconds: 10 }),
      /60/
    )
    const cron = await flush(
      mockApi.cronjobs.create(PID, {
        name: 'ExtraJob',
        description: 'd',
        funcFile: 'hello',
        funcExport: 'Hello',
        scheduleKind: 'cron',
        cronExpr: '0 * * * *',
        inputJson: '{"a":1}'
      })
    )
    const interval = await flush(
      mockApi.cronjobs.create(PID, {
        name: 'EveryMin',
        funcFile: 'hello',
        funcExport: 'Hello',
        scheduleKind: 'interval',
        intervalSeconds: 60
      })
    )
    expect((await flush(mockApi.cronjobs.get(PID, cron.id))).id).toBe(cron.id)
    await flushReject(mockApi.cronjobs.get(PID, 'missing'))
    expect(
      (await flush(mockApi.cronjobs.update(PID, cron.id, { description: 'n', scheduleKind: 'interval', intervalSeconds: 90, enabled: false, inputJson: '{}' }))).scheduleKind
    ).toBe('interval')
    expect(
      (await flush(mockApi.cronjobs.update(PID, interval.id, { scheduleKind: 'cron', cronExpr: '1 * * * *', funcFile: 'hello', funcExport: 'Hello' }))).cronExpr
    ).toBe('1 * * * *')
    await flushReject(mockApi.cronjobs.update(PID, 'missing', {}))
    await flush(mockApi.cronjobs.trigger(PID, cron.id))
    expect((await flush(mockApi.cronjobs.runs(PID, cron.id))).length).toBeGreaterThan(0)
    await flushReject(mockApi.cronjobs.trigger(PID, 'missing'))
    await flush(mockApi.cronjobs.remove(PID, cron.id))
    await flushReject(mockApi.cronjobs.remove(PID, cron.id))
  })

  it('covers agents, threads, and schedules', async () => {
    expect((await flush(mockApi.agents.modules(PID))).length).toBeGreaterThan(0)
    const listed = await flush(mockApi.agents.list(PID))
    expect(listed.length).toBeGreaterThan(0)
    const agent = await flush(mockApi.agents.create(PID, { name: 'Custom', module: 'general', description: 'd', system_prompt: 'p', tool_ids: [] }))
    expect((await flush(mockApi.agents.get(PID, agent.id))).name).toBe('Custom')
    await flushReject(mockApi.agents.get(PID, 'missing'))
    expect((await flush(mockApi.agents.patch(PID, agent.id, { name: 'Custom2' }))).name).toBe('Custom2')
    await flushReject(mockApi.agents.patch(PID, 'missing', {}))

    expect(await flush(mockApi.agentThreads.list(PID))).toEqual([])
    const thread = await flush(mockApi.agentThreads.create(PID, 'T1'))
    expect((await flush(mockApi.agentThreads.get(PID, thread.id))).id).toBe(thread.id)
    await flushReject(mockApi.agentThreads.get(PID, 'missing'))
    const run = await flush(mockApi.agentThreads.run(PID, thread.id, { content: 'hi', mentions: [{ agent_id: agent.id }] }))
    expect(run.message.role).toBe('assistant')
    expect((await flush(mockApi.agentThreads.messages(PID, thread.id))).length).toBeGreaterThan(0)
    expect((await flush(mockApi.agentThreads.cancel(PID, 'r'))).status).toBe('canceled')
    const tokens: string[] = []
    const streamErr = vi.fn()
    const s1 = mockApi.agentThreads.streamRun(PID, thread.id, { content: 'stream', mentions: [] }, {
      onToken: (t) => tokens.push(t),
      onToolCall: vi.fn(),
      onToolResult: vi.fn(),
      onEnd: vi.fn(),
      onError: streamErr
    })
    await vi.runAllTimersAsync()
    expect(tokens.join('')).toContain('Mock')
    const s2 = mockApi.agentThreads.streamRun(PID, thread.id, { content: 'x', mentions: [] }, {
      onToken: () => {
        throw new Error('tok')
      },
      onError: streamErr
    })
    await vi.runAllTimersAsync()
    s1.close()
    s2.close()
    await flush(mockApi.agentThreads.remove(PID, thread.id))

    const sched = await flush(
      mockApi.agentSchedules.create(PID, { agent_id: agent.id, prompt: 'ping', cron_expr: '* * * * *' })
    )
    await flushReject(
      mockApi.agentSchedules.create(PID, { agent_id: agent.id, prompt: 'ping', cron_expr: '* * * * *' }),
      /exists/
    )
    await flushReject(mockApi.agentSchedules.create(PID, { agent_id: 'missing', prompt: 'p', cron_expr: '* * * * *' }))
    expect((await flush(mockApi.agentSchedules.list(PID))).some((s) => s.id === sched.id)).toBe(true)
    expect((await flush(mockApi.agentSchedules.patch(PID, sched.id, { prompt: 'pong', cron_expr: '1 * * * *', enabled: false }))).enabled).toBe(false)
    expect((await flush(mockApi.agentSchedules.patch(PID, sched.id, { enabled: true }))).enabled).toBe(true)
    await flushReject(mockApi.agentSchedules.patch(PID, 'missing', {}))
    await flush(mockApi.agentSchedules.trigger(PID, sched.id))
    expect((await flush(mockApi.agentSchedules.runs(PID, sched.id))).length).toBeGreaterThan(0)
    await vi.advanceTimersByTimeAsync(1500)
    await flushReject(mockApi.agentSchedules.trigger(PID, 'missing'))
    await flush(mockApi.agentSchedules.remove(PID, sched.id))
    await flush(mockApi.agents.remove(PID, agent.id))
  })
})
