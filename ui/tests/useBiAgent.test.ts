import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { api, resetApiMocks } from '@/test/api-mock'
import { DATA_AGENT_KEY, buildAgentFields, threadKey, useBiAgent, type BiAgentScope } from '@/composables/useBiAgent'
import type { AgentStreamHandlers, BiChartType, CloudAgent, CollectionField } from '@/services/types'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

const scope: BiAgentScope = { projectId: 'p1', databaseId: 'db1', collection: 'orders' }
const f = (name: string, type: CollectionField['type'], hidden = false): CollectionField => ({
  name, type, required: false, unique: false, options: {}, hidden, readonly: false
})
const fields = [f('amount', 'number'), f('status', 'select'), f('secret', 'text', true)]
const agent = (over: Partial<CloudAgent> = {}): CloudAgent => ({
  id: 'ag-db', name: 'Database', module: 'database', description: '', system_prompt: '', tool_ids: [],
  builtin_key: DATA_AGENT_KEY, team_enabled: false, writable: false, created_at: '', updated_at: '', ...over
})
const chart = (o: Record<string, unknown>) => `<!-- sb:chart ${JSON.stringify({ version: 1, ...o })} -->`

function memoryStorage(initial: Record<string, string> = {}): Storage {
  const data = new Map(Object.entries(initial))
  return {
    get length() { return data.size }, clear: () => data.clear(), getItem: (k) => data.get(k) ?? null,
    key: (i) => [...data.keys()][i] ?? null, removeItem: (k) => void data.delete(k), setItem: (k, v) => void data.set(k, v)
  }
}

/** 捕获 streamRun 的请求与 handlers，由测试手动驱动流事件。 */
function captureStream() {
  const calls: Array<{ req: Record<string, unknown>; handlers: AgentStreamHandlers }> = []
  api.agentThreads.streamRun.mockImplementation((_p: string, _t: string, req: Record<string, unknown>, handlers: AgentStreamHandlers) => {
    calls.push({ req, handlers })
    return { close: vi.fn() }
  })
  return calls
}

function setup(preferred: BiChartType = 'auto', storage = memoryStorage(), s: BiAgentScope = scope) {
  const bi = useBiAgent({ scope: () => s, fields: () => fields, preferredType: () => preferred, storage })
  return { bi, storage }
}

beforeEach(() => {
  resetApiMocks()
  api.agents.list.mockResolvedValue([agent()])
  api.agentThreads.create.mockResolvedValue({ id: 'th-new', title: 't', created_at: '', updated_at: '' })
  api.agentThreads.messages.mockResolvedValue([])
  api.agentThreads.cancel.mockResolvedValue({})
})

describe('buildAgentFields', () => {
  it('drops hidden fields and appends the builtin time columns', () => {
    expect(buildAgentFields(fields)).toEqual([
      { name: 'amount', type: 'number' }, { name: 'status', type: 'select' },
      { name: 'created_at', type: 'date' }, { name: 'updated_at', type: 'date' }
    ])
  })
  it('does not duplicate a registered created_at', () => {
    const out = buildAgentFields([f('created_at', 'date')])
    expect(out.filter((x) => x.name === 'created_at')).toHaveLength(1)
  })
})

describe('useBiAgent assistant lookup', () => {
  it('finds the built-in data assistant by builtin_key, ignoring custom ones', async () => {
    api.agents.list.mockResolvedValue([agent({ id: 'custom', builtin_key: '' , name: 'Database'}), agent({ id: 'real' })])
    const { bi } = setup()
    expect((await bi.ensureAgent())?.id).toBe('real')
    expect(bi.agentMissing.value).toBe(false)
  })

  it('reports a missing assistant without creating one', async () => {
    api.agents.list.mockResolvedValue([agent({ id: 'g', builtin_key: 'general' })])
    const { bi } = setup()
    expect(await bi.ensureAgent()).toBeNull()
    expect(bi.agentMissing.value).toBe(true)
    await bi.send('hi')
    expect(api.agents.create).not.toHaveBeenCalled()
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
  })

  it('caches the assistant across sends', async () => {
    captureStream()
    const { bi } = setup()
    await bi.send('a')
    await bi.send('b')
    expect(api.agents.list).toHaveBeenCalledTimes(1)
  })
})

describe('useBiAgent thread handling', () => {
  it('creates a titled thread bound to the assistant and remembers it per collection', async () => {
    captureStream()
    const { bi, storage } = setup()
    await bi.send('hi')
    expect(api.agentThreads.create).toHaveBeenCalledWith('p1', 'BI · orders', 'ag-db')
    expect(storage.getItem(threadKey(scope))).toBe('th-new')
  })

  it('reuses a stored thread when it still exists', async () => {
    captureStream()
    const { bi } = setup('auto', memoryStorage({ [threadKey(scope)]: 'th-old' }))
    await bi.send('hi')
    expect(api.agentThreads.messages).toHaveBeenCalledWith('p1', 'th-old')
    expect(api.agentThreads.create).not.toHaveBeenCalled()
    expect(api.agentThreads.streamRun).toHaveBeenCalledWith('p1', 'th-old', expect.anything(), expect.anything())
  })

  it('recreates the thread on 404', async () => {
    captureStream()
    api.agentThreads.messages.mockRejectedValue(Object.assign(new Error('nf'), { status: 404 }))
    const { bi, storage } = setup('auto', memoryStorage({ [threadKey(scope)]: 'th-gone' }))
    await bi.send('hi')
    expect(api.agentThreads.create).toHaveBeenCalledTimes(1)
    expect(storage.getItem(threadKey(scope))).toBe('th-new')
  })

  it('does not spawn new threads on non-404 probe failures', async () => {
    api.agentThreads.messages.mockRejectedValue(Object.assign(new Error('boom'), { status: 500 }))
    const { bi } = setup('auto', memoryStorage({ [threadKey(scope)]: 'th-old' }))
    await bi.send('hi')
    expect(api.agentThreads.create).not.toHaveBeenCalled()
    expect(bi.failure.value).toBe('boom')
  })

  it('keeps separate threads per collection', () => {
    expect(threadKey(scope)).not.toBe(threadKey({ ...scope, collection: 'users' }))
    expect(threadKey(scope)).not.toBe(threadKey({ ...scope, databaseId: 'db2' }))
  })

  it('"new chat" forgets the thread and clears the pending area', async () => {
    captureStream()
    const { bi, storage } = setup()
    await bi.send('hi')
    await bi.newChat()
    expect(storage.getItem(threadKey(scope))).toBeNull()
    expect(bi.messages.value).toEqual([])
    expect(bi.pending.value).toEqual([])
    await bi.send('again')
    expect(api.agentThreads.create).toHaveBeenCalledTimes(2)
  })
})

describe('useBiAgent request body', () => {
  it('sends bi_context with schema fields and the assistant mention', async () => {
    const calls = captureStream()
    const { bi } = setup('stacked_bar')
    await bi.send('近 30 天金额')
    expect(calls).toHaveLength(1)
    expect(calls[0].req).toMatchObject({
      content: '近 30 天金额',
      mentions: [{ agent_id: 'ag-db' }],
      stream: true,
      bi_context: {
        database_id: 'db1', collection: 'orders', preferred_type: 'stacked_bar',
        fields: [
          { name: 'amount', type: 'number' }, { name: 'status', type: 'select' },
          { name: 'created_at', type: 'date' }, { name: 'updated_at', type: 'date' }
        ]
      }
    })
    // 隐藏字段永远不会发给助手。
    expect(JSON.stringify(calls[0].req)).not.toContain('secret')
  })

  it('omits preferred_type for auto', async () => {
    const calls = captureStream()
    const { bi } = setup('auto')
    await bi.send('x')
    expect(calls[0].req.bi_context).not.toHaveProperty('preferred_type')
  })

  it('ignores blank input', async () => {
    const calls = captureStream()
    const { bi } = setup()
    await bi.send('   ')
    expect(calls).toHaveLength(0)
    expect(api.agents.list).not.toHaveBeenCalled()
  })
})

describe('useBiAgent results', () => {
  async function run(content: string) {
    const calls = captureStream()
    const { bi } = setup()
    await bi.send('go')
    const h = calls[0].handlers
    h.onToken?.(content)
    h.onEnd?.('stop')
    await nextTick()
    return { bi, calls }
  }

  it('puts valid blocks in the pending area and shows the prose without them', async () => {
    const text = `好的。\n${chart({ title: '状态占比', type: 'pie', metrics: ['count'], group_by: 'status', donut: true })}`
    const { bi } = await run(text)
    expect(bi.pending.value).toHaveLength(1)
    expect(bi.pending.value[0].widget).toMatchObject({ title: '状态占比', type: 'pie', groupBy: 'status', donut: true })
    expect(bi.pending.value[0].widget.id).toMatch(/^ai-/)
    expect(bi.notes.value).toBe('好的。')
    expect(bi.rejected.value).toEqual([])
    expect(bi.noBlock.value).toBe(false)
  })

  it('rejects blocks with unknown fields or incompatible types, keeping the good ones', async () => {
    const text = [
      chart({ title: 'good', type: 'bar', metrics: ['count'], group_by: 'status' }),
      chart({ title: 'ghost', type: 'bar', metrics: ['count'], group_by: 'nope' }),
      chart({ title: 'bad stack', type: 'stacked_bar', metrics: ['avg:amount'], group_by: 'status', series: 'status' })
    ].join('\n')
    const { bi } = await run(text)
    expect(bi.pending.value.map((p) => p.widget.title)).toEqual(['good'])
    expect(bi.rejected.value).toEqual([
      { title: 'ghost', reason: 'field_unknown' },
      { title: 'bad stack', reason: 'stacked_needs_additive' }
    ])
  })

  it('flags a reply with no usable block', async () => {
    const { bi } = await run('数据不适合出图。')
    expect(bi.pending.value).toEqual([])
    expect(bi.noBlock.value).toBe(true)
    expect(bi.notes.value).toBe('数据不适合出图。')
  })

  it('nothing reaches the dashboard until the user confirms: pending is local state only', async () => {
    const { bi } = await run(chart({ title: 'x', type: 'auto', metrics: ['count'] }))
    expect(bi.pending.value).toHaveLength(1)
    bi.dropPending(bi.pending.value[0].widget.id)
    expect(bi.pending.value).toEqual([])
  })

  it('a new send clears the previous results', async () => {
    const calls = captureStream()
    const { bi } = setup()
    await bi.send('one')
    calls[0].handlers.onToken?.(chart({ title: 'a', type: 'auto', metrics: ['count'] }))
    calls[0].handlers.onEnd?.('stop')
    expect(bi.pending.value).toHaveLength(1)
    await bi.send('two')
    expect(bi.pending.value).toEqual([])
    expect(bi.noBlock.value).toBe(false)
  })
})

describe('useBiAgent failures, retry, cancel', () => {
  it('shows a translated error and offers retry that re-sends bi_context', async () => {
    const calls = captureStream()
    const { bi } = setup('pie')
    await bi.send('go')
    calls[0].handlers.onRun?.('run-1')
    calls[0].handlers.onError?.(Object.assign(new Error('x'), { code: 'quota_exceeded' }))
    expect(bi.failure.value).toContain('配额')
    expect(bi.failedRunId.value).toBe('run-1')

    await bi.retry()
    expect(calls).toHaveLength(2)
    expect(calls[1].req).toMatchObject({ retry_of_run_id: 'run-1', content: '' })
    expect(calls[1].req.bi_context).toMatchObject({ collection: 'orders', preferred_type: 'pie' })
  })

  it('retry is a no-op without a failed run', async () => {
    const calls = captureStream()
    const { bi } = setup()
    await bi.retry()
    expect(calls).toHaveLength(0)
  })

  it('stop cancels the backend run', async () => {
    const calls = captureStream()
    const { bi } = setup()
    await bi.send('go')
    calls[0].handlers.onRun?.('run-9')
    bi.stop()
    expect(api.agentThreads.cancel).toHaveBeenCalledWith('p1', 'run-9')
  })

  it('"fix with agent" sends the backend error as a follow-up', async () => {
    const calls = captureStream()
    const { bi } = setup()
    await bi.send('go')
    calls[0].handlers.onEnd?.('stop')
    await bi.fixWithAgent('趋势', 'filter scan limit exceeded')
    expect(calls).toHaveLength(2)
    expect(calls[1].req.content).toContain('趋势')
    expect(calls[1].req.content).toContain('filter scan limit exceeded')
  })
})

describe('useBiAgent scope changes (epoch)', () => {
  it('reset discards late callbacks from the previous collection', async () => {
    const calls = captureStream()
    const { bi } = setup()
    await bi.send('go')
    bi.reset()
    calls[0].handlers.onToken?.(chart({ title: 'late', type: 'auto', metrics: ['count'] }))
    calls[0].handlers.onEnd?.('stop')
    await nextTick()
    expect(bi.pending.value).toEqual([])
    expect(bi.failure.value).toBe('')
  })

  it('reset during the assistant lookup stops the send before any request', async () => {
    const calls = captureStream()
    let release!: (v: CloudAgent[]) => void
    api.agents.list.mockReturnValue(new Promise<CloudAgent[]>((r) => { release = r }))
    const { bi } = setup()
    const sending = bi.send('go')
    bi.reset()
    release([agent()])
    await sending
    await flushPromises()
    expect(calls).toHaveLength(0)
    expect(api.agentThreads.create).not.toHaveBeenCalled()
  })

  it('a late failure after reset is ignored', async () => {
    const calls = captureStream()
    const { bi } = setup()
    await bi.send('go')
    bi.reset()
    calls[0].handlers.onError?.(new Error('late'))
    expect(bi.failure.value).toBe('')
  })
})

describe('useBiAgent preparation guard and stale state', () => {
  it('a second send while the thread is still being created is ignored (no duplicate threads)', async () => {
    const calls = captureStream()
    let release!: (v: { id: string; title: string; created_at: string; updated_at: string }) => void
    api.agentThreads.create.mockReturnValue(new Promise((r) => { release = r }))
    const { bi } = setup()
    const first = bi.send('one')
    await flushPromises()
    expect(bi.sending.value).toBe(true)
    await bi.send('two')
    release({ id: 'th-new', title: 't', created_at: '', updated_at: '' })
    await first
    await flushPromises()
    expect(api.agentThreads.create).toHaveBeenCalledTimes(1)
    expect(calls).toHaveLength(1)
    expect(calls[0].req.content).toBe('one')
  })

  it('preparation failure releases the busy flag so the user can retry', async () => {
    api.agentThreads.create.mockRejectedValueOnce(new Error('boom'))
    const { bi } = setup()
    await bi.send('hi')
    expect(bi.failure.value).toBe('boom')
    expect(bi.sending.value).toBe(false)
  })

  it('reset clears the previous failure, timer and retry id so a new scope never shows the old error', async () => {
    const calls = captureStream()
    const { bi } = setup()
    await bi.send('go')
    calls[0].handlers.onRun?.('run-1')
    calls[0].handlers.onError?.(new Error('quota'))
    await nextTick()
    expect(bi.failedRunId.value).toBe('run-1')
    bi.reset()
    expect(bi.failedRunId.value).toBe('')
    expect(bi.failure.value).toBe('')
    expect(bi.statusText.value).toBe('')
    expect(bi.phase.value).toBe('idle')
    expect(bi.sending.value).toBe(false)
  })

  it('reset during preparation releases the busy flag too', async () => {
    captureStream()
    let release!: (v: CloudAgent[]) => void
    api.agents.list.mockReturnValue(new Promise<CloudAgent[]>((r) => { release = r }))
    const { bi } = setup()
    const sending = bi.send('go')
    expect(bi.sending.value).toBe(true)
    bi.reset()
    expect(bi.sending.value).toBe(false)
    release([agent()])
    await sending
    expect(bi.sending.value).toBe(false)
  })
})

describe('useBiAgent block feedback', () => {
  async function run(text: string) {
    const calls = captureStream()
    const { bi } = setup()
    await bi.send('go')
    calls[0].handlers.onToken?.(text)
    calls[0].handlers.onEnd?.('stop')
    await nextTick()
    return bi
  }

  it('reports unparsable blocks next to the good ones instead of dropping them silently', async () => {
    const bi = await run(`<!-- sb:chart {坏 -->\n${chart({ title: 'good', type: 'bar', metrics: ['count'], group_by: 'status' })}`)
    expect(bi.pending.value.map((p) => p.widget.title)).toEqual(['good'])
    expect(bi.rejected.value).toEqual([{ title: '—', reason: 'bad_json' }])
    expect(bi.noBlock.value).toBe(false)
  })

  it('a block whose filter is too long is rejected, never added with the filter missing', async () => {
    const bi = await run(chart({ title: 'wide', type: 'bar', metrics: ['count'], group_by: 'status', filter: 'a'.repeat(3000) }))
    expect(bi.pending.value).toEqual([])
    expect(bi.rejected.value).toEqual([{ title: 'wide', reason: 'filter_too_long' }])
  })

  it('updatePending replaces a preview in place and leaves the others untouched', async () => {
    const bi = await run([
      chart({ title: 'a', type: 'bar', metrics: ['count'], group_by: 'status' }),
      chart({ title: 'b', type: 'pie', metrics: ['count'], group_by: 'status' })
    ].join('\n'))
    const first = bi.pending.value[0].widget
    bi.updatePending({ ...first, title: 'renamed' })
    expect(bi.pending.value.map((p) => p.widget.title)).toEqual(['renamed', 'b'])
    expect(bi.pending.value[0].widget.id).toBe(first.id)
  })
})

/** 可切换 scope 的 setup：模拟页面从 tasks 切到 users 再切回。 */
function swappable(initial: BiAgentScope) {
  let current = initial
  const bi = useBiAgent({ scope: () => current, fields: () => fields, preferredType: () => 'auto', storage: memoryStorage() })
  return { bi, switchTo: (next: BiAgentScope) => { current = next } }
}

describe('useBiAgent scope parking and resume', () => {

  it('restores the previous preview and messages when the same collection is opened again', async () => {
    const calls = captureStream()
    const { bi, switchTo } = swappable(scope)
    await bi.send('go')
    calls[0].handlers.onToken?.(chart({ title: '保留我', type: 'auto', metrics: ['count'] }))
    calls[0].handlers.onEnd?.('stop')
    await nextTick()
    expect(bi.pending.value).toHaveLength(1)

    // 切到 users：现场暂存，实时状态清空。
    switchTo({ ...scope, collection: 'users' })
    bi.reset()
    expect(bi.pending.value).toEqual([])
    expect(bi.messages.value).toEqual([])
    // users 自己没有现场：sync 不应该把 tasks 的东西恢复出来。
    bi.sync()
    expect(bi.pending.value).toEqual([])

    switchTo(scope)
    bi.reset()
    bi.sync()
    expect(bi.pending.value.map((p) => p.widget.title)).toEqual(['保留我'])
    expect(bi.messages.value.length).toBeGreaterThan(0)
  })

  it('re-attaches a run that was still going when the user navigated away', async () => {
    const calls = captureStream()
    const { bi, switchTo } = swappable(scope)
    await bi.send('go')
    calls[0].handlers.onRun?.('run-42')
    expect(bi.sending.value).toBe(true)

    switchTo({ ...scope, collection: 'users' })
    bi.reset()
    // 离开不等于取消：连接关闭，但后端 run 继续。
    expect(api.agentThreads.cancel).not.toHaveBeenCalled()

    switchTo(scope)
    bi.reset()
    bi.sync()
    expect(api.agentThreads.attachRun).toHaveBeenCalledWith('p1', 'run-42', expect.anything())
  })

  it('never parks one collection under another collection key', async () => {
    captureStream()
    const { bi, switchTo } = swappable(scope)
    await bi.send('go')
    // 现网顺序：先切 source，再触发清理；暂存必须按会话所属的数据源算。
    switchTo({ ...scope, collection: 'users' })
    bi.reset()
    bi.sync()
    expect(bi.pending.value).toEqual([])
    expect(bi.messages.value).toEqual([])
  })

  it('new chat forgets the parked state so coming back does not resurrect it', async () => {
    const calls = captureStream()
    const { bi } = swappable(scope)
    await bi.send('go')
    calls[0].handlers.onToken?.(chart({ title: '旧结果', type: 'auto', metrics: ['count'] }))
    calls[0].handlers.onEnd?.('stop')
    await nextTick()
    await bi.newChat()
    bi.sync()
    expect(bi.pending.value).toEqual([])
  })

  it('a read-only instance is explained in words, not with the backend message', async () => {
    api.agentThreads.create.mockRejectedValue(Object.assign(new Error('this instance is not writable'), { code: 'writer_unavailable', status: 503 }))
    const { bi } = setup()
    await bi.send('hi')
    expect(bi.failure.value).toContain('只读')
    expect(bi.failure.value).not.toContain('not writable')
  })
})

describe('useBiAgent park edge cases', () => {
  it('does not park when the scope never had a session (no thread yet)', async () => {
    const { bi } = setup()
    // 从没发过消息：reset 不应留下任何待恢复现场。
    bi.reset()
    bi.sync()
    expect(bi.pending.value).toEqual([])
    expect(bi.messages.value).toEqual([])
  })

  it('sync without a collection is a no-op', async () => {
    const bi = useBiAgent({ scope: () => ({ projectId: 'p1', databaseId: 'db1', collection: '' }), fields: () => fields, preferredType: () => 'auto', storage: memoryStorage() })
    bi.sync()
    expect(bi.pending.value).toEqual([])
  })

  it('a parked run failing on resume surfaces the failure instead of a silent stop', async () => {
    const calls = captureStream()
    const { bi, switchTo } = swappable(scope)
    await bi.send('go')
    calls[0].handlers.onRun?.('run-9')
    switchTo({ ...scope, collection: 'users' })
    bi.reset()
    switchTo(scope)
    bi.reset()
    bi.sync()
    const attachHandlers = api.agentThreads.attachRun.mock.calls.at(-1)?.[2] as AgentStreamHandlers | undefined
    attachHandlers?.onError?.(new Error('boom') as never)
    await nextTick()
    expect(bi.failure.value).toBe('boom')
  })
})

