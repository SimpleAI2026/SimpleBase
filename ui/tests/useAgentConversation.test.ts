import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, nextTick, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { api, resetApiMocks } from '@/test/api-mock'
import { toast } from 'vue-sonner'
import { useAgentConversation } from '@/composables/useAgentConversation'
import ConversationView from '@/components/agent/ConversationView.vue'
import type { ChatMsg } from '@/services/types'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

function makeConv() {
  const messages: ChatMsg[] = []
  const conv = useAgentConversation({
    projectId: () => 'p1',
    threadId: () => 'th-1',
    messages: () => messages
  })
  return { conv, messages }
}

describe('useAgentConversation', () => {
  beforeEach(() => {
    resetApiMocks()
    api.agentThreads.streamRun.mockReturnValue({ close: vi.fn() })
    api.agentThreads.cancel.mockResolvedValue({} as never)
  })

  it('pushes user bubble and streams reply tokens', () => {
    const { conv, messages } = makeConv()
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      handlers = h as Record<string, (...args: unknown[]) => void>
      return { close: vi.fn() }
    })
    conv.start({ content: '你好', mentions: [{ agent_id: 'a1' }] }, {})
    expect(messages).toHaveLength(2)
    expect(messages[0]).toMatchObject({ role: 'user', content: '你好' })
    expect(conv.phase.value).toBe('thinking')
    handlers.onRun?.('run-9')
    expect(conv.runId.value).toBe('run-9')
    handlers.onToken?.('回')
    handlers.onToken?.('答')
    expect(conv.phase.value).toBe('streaming')
    expect(messages[1].content).toBe('回答')
    handlers.onUsage?.({ prompt_tokens: 3, completion_tokens: 2, duration_ms: 900, tool_calls: 0 } as never)
    handlers.onEnd?.('stop')
    expect(conv.phase.value).toBe('done')
    expect(conv.statusText.value).toContain('完成')
  })

  it('renders each token, thinking chunk and tool update before the run ends', async () => {
    let handlers: Parameters<typeof api.agentThreads.streamRun>[3]
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      handlers = h
      return { close: vi.fn() }
    })
    const wrapper = mount(defineComponent({
      setup() {
        const messages = ref<ChatMsg[]>([])
        const conv = useAgentConversation({ projectId: () => 'p1', threadId: () => 'th-1', messages: () => messages.value })
        conv.start({ content: '查询', mentions: [{ agent_id: 'a1' }] }, {})
        return () => h(ConversationView, { messages: messages.value, sending: conv.sending.value })
      }
    }))
    for (const [token, expected] of [['第', '第'], ['二', '第二'], ['段', '第二段']]) {
      handlers!.onToken?.(token)
      await nextTick()
      expect(wrapper.text()).toContain(expected)
    }
    handlers!.onThinking?.(100, '先分析')
    await nextTick()
    expect(wrapper.text()).toContain('先分析')
    handlers!.onToolCallDelta?.('list_databases', '{', 'c1')
    await nextTick()
    expect(wrapper.text()).toContain('list_databases')
    handlers!.onToolCall?.('list_databases', '{}', 'c1')
    await nextTick()
    expect(wrapper.get('[data-status="pending"]').text()).toContain('list_databases')
    handlers!.onToolResult?.('list_databases', '[]', 'c1', 1)
    await nextTick()
    expect(wrapper.find('[data-tool-summary="list_databases"]').exists()).toBe(true)
    handlers!.onEnd?.('stop')
    await nextTick()
    expect(wrapper.find('[data-typing-dots]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps tool phase during thinking heartbeat (BUG-09)', () => {
    const { conv } = makeConv()
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      handlers = h as Record<string, (...args: unknown[]) => void>
      return { close: vi.fn() }
    })
    conv.start({ content: '查库', mentions: [{ agent_id: 'a1' }] }, {})
    handlers.onRun?.('run-1')
    handlers.onToolCall?.('list_databases', '{}', 'c1')
    expect(conv.phase.value).toBe('tool')
    // 工具执行期间收到 thinking 心跳：phase 保持 tool 不回落（BUG-01/09）。
    handlers.onThinking?.(3000, '')
    expect(conv.phase.value).toBe('tool')
    handlers.onToolProgress?.('list_databases', 4000, 'c1')
    expect(conv.phase.value).toBe('tool')
  })

  it('maps error to user text and records failedRunId for retry (BUG-03)', () => {
    const { conv, messages } = makeConv()
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      handlers = h as Record<string, (...args: unknown[]) => void>
      return { close: vi.fn() }
    })
    conv.start({ content: '你好', mentions: [{ agent_id: 'a1' }] }, {})
    handlers.onRun?.('run-x')
    handlers.onError?.(Object.assign(new Error('rate limit'), { code: 'llm_rate_limited' }))
    expect(conv.phase.value).toBe('error')
    expect(conv.failedRunId.value).toBe('run-x')
    expect(messages[1].error).toContain('限流')
    // 重试：请求携带 retry_of_run_id 且不追加 user 气泡（失败 assistant 由页面层移除）。
    const before = messages.length
    messages.pop() // 页面层 retryLast 会先移除失败气泡
    conv.start({ content: '', mentions: [], retry_of_run_id: 'run-x' }, {})
    expect(messages).toHaveLength(before) // 1 user + 1 新 assistant，无重复 user
    expect(api.agentThreads.streamRun).toHaveBeenCalledTimes(2)
    const [, , req] = api.agentThreads.streamRun.mock.calls[1] as unknown as [string, string, Record<string, unknown>]
    expect(req.retry_of_run_id).toBe('run-x')
  })

  it('dedupes tool cards by call id and fills results', () => {
    const { conv, messages } = makeConv()
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      handlers = h as Record<string, (...args: unknown[]) => void>
      return { close: vi.fn() }
    })
    conv.start({ content: '查', mentions: [{ agent_id: 'a1' }] }, {})
    handlers.onRun?.('run-1')
    handlers.onToolCall?.('readonly_sql', '{}', 'c1')
    handlers.onToolCall?.('readonly_sql', '{}', 'c1') // 重复帧
    expect(messages[1].toolCalls).toHaveLength(1)
    handlers.onToolResult?.('readonly_sql', '{"rows":[]}', 'c1', 42)
    expect(messages[1].toolCalls?.[0]).toMatchObject({ content: '{"rows":[]}', duration_ms: 42 })
  })

  it('preserves confirmation after request failure and lets the user retry', async () => {
    const { conv } = makeConv()
    let handlers: Parameters<typeof api.agentThreads.streamRun>[3]
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      handlers = h
      return { close: vi.fn() }
    })
    api.agentThreads.confirm.mockRejectedValueOnce(new Error('confirm-offline'))
    api.agentThreads.confirm.mockResolvedValueOnce(undefined)
    conv.start({ content: '测试', mentions: [] }, {})
    handlers!.onRun?.('run-1')
    handlers!.onConfirmationRequired?.('call-1', ['echo'], '确认')
    await conv.resolveConfirm(true)
    expect(toast.error).toHaveBeenCalledWith('confirm-offline')
    expect(conv.confirmation.value?.callId).toBe('call-1')
    expect(conv.phase.value).toBe('awaiting_confirm')
    await conv.resolveConfirm(false)
    expect(api.agentThreads.confirm).toHaveBeenLastCalledWith('p1', 'run-1', 'call-1', false)
    expect(conv.confirmation.value).toBeNull()
  })

  it('warns when canceling a run fails instead of silently claiming it stopped', async () => {
    const { conv } = makeConv()
    api.agentThreads.cancel.mockRejectedValueOnce(new Error('cancel-offline'))
    conv.start({ content: '测试', mentions: [] }, {})
    conv.runId.value = 'run-2'
    conv.stop()
    await Promise.resolve()
    expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('cancel-offline'))
  })

  it('stop closes connection and cancels active run', () => {
    const { conv } = makeConv()
    const close = vi.fn()
    api.agentThreads.streamRun.mockReturnValue({ close })
    conv.start({ content: '你好', mentions: [{ agent_id: 'a1' }] }, {})
    conv.runId.value = 'run-z'
    conv.stop()
    expect(close).toHaveBeenCalled()
    expect(api.agentThreads.cancel).toHaveBeenCalledWith('p1', 'run-z')
  })

  // run 事件带 agent_id 时直接定身份（固定助手会话的 run 帧带回答者）。
  it('fills speaker identity from run event carrying agent_id', () => {
    const { conv, messages } = makeConv()
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      handlers = h as Record<string, (...args: unknown[]) => void>
      return { close: vi.fn() }
    })
    conv.start({ content: '问题', mentions: [{ agent_id: 'a1' }] }, {})
    handlers.onRun?.('run-1', 'a-db')
    expect(messages[1].agentId).toBe('a-db')
  })

  // run 帧带 agent_id 时用 moduleOf 回填模块（头像与侧栏图标对齐）。
  it('fills speaker module from moduleOf', () => {
    const messages: ChatMsg[] = []
    const conv = useAgentConversation({
      projectId: () => 'p1',
      threadId: () => 'th-1',
      messages: () => messages,
      moduleOf: (id) => (id === 'a-db' ? 'database' : undefined)
    })
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      handlers = h as Record<string, (...args: unknown[]) => void>
      return { close: vi.fn() }
    })
    conv.start({ content: '查表', mentions: [] }, {})
    handlers.onRun?.('run-2', 'a-db')
    expect(messages[1].agentId).toBe('a-db')
    expect(messages[1].module).toBe('database')
    handlers.onEnd?.('stop')
  })

  // attach 回放同样生效（run 事件带 agent_id）。
  it('applies speaker identity during attach replay', () => {
    const { conv, messages } = makeConv()
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.attachRun.mockImplementation((_p, _r, h) => {
      handlers = h as Record<string, (...args: unknown[]) => void>
      return { close: vi.fn() }
    })
    conv.attach('run-7', {})
    handlers.onRun?.('run-7', 'a-s3')
    expect(messages[0].agentId).toBe('a-s3')
  })
})

describe('useAgentConversation edge branches', () => {
  beforeEach(() => {
    resetApiMocks()
    api.agentThreads.streamRun.mockReturnValue({ close: vi.fn() })
    api.agentThreads.cancel.mockResolvedValue({} as never)
  })

  it('tool progress keeps elapsed ticking and stays in tool phase', () => {
    const { conv } = makeConv()
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      handlers = h as Record<string, (...args: unknown[]) => void>
      return { close: vi.fn() }
    })
    conv.start({ content: '查', mentions: [{ agent_id: 'a' }] }, {})
    handlers.onRun?.('r1')
    handlers.onToolCall?.('search_logs', '{}', 'c1')
    handlers.onToolProgress?.('search_logs', 7000, 'c1')
    expect(conv.phase.value).toBe('tool')
    expect(conv.elapsedMs.value).toBe(7000)
  })

  it('marks canceled end and keeps reply.canceled', () => {
    const { conv, messages } = makeConv()
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      handlers = h as Record<string, (...args: unknown[]) => void>
      return { close: vi.fn() }
    })
    conv.start({ content: 'hi', mentions: [{ agent_id: 'a' }] }, {})
    handlers.onRun?.('r1')
    handlers.onEnd?.('canceled')
    expect(conv.phase.value).toBe('canceled')
    expect(messages[1].canceled).toBe(true)
    expect(conv.statusText.value).toContain('已停止')
  })

  it('routes tool result by name when call id missing', () => {
    const { conv, messages } = makeConv()
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      handlers = h as Record<string, (...args: unknown[]) => void>
      return { close: vi.fn() }
    })
    conv.start({ content: 'hi', mentions: [{ agent_id: 'a' }] }, {})
    handlers.onRun?.('r1')
    handlers.onToolCall?.('list_objects', '{}') // 无 call id
    handlers.onToolResult?.('list_objects', '[{}]', undefined, 5)
    expect(messages[1].toolCalls?.[0]).toMatchObject({ content: '[{}]', duration_ms: 5 })
  })

  it('stop without active run does not call cancel', () => {
    const { conv } = makeConv()
    conv.stop()
    expect(api.agentThreads.cancel).not.toHaveBeenCalled()
  })

  it('stop(false) detaches without canceling the backend run', () => {
    const { conv } = makeConv()
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      handlers = h as Record<string, (...args: unknown[]) => void>
      return { close: vi.fn() }
    })
    conv.start({ content: 'hi', mentions: [{ agent_id: 'a' }] }, {})
    handlers.onRun?.('r1')
    conv.stop(false)
    expect(api.agentThreads.cancel).not.toHaveBeenCalled()
    expect(conv.phase.value).toBe('idle')
  })

  it('attach replays a running run into a new reply; gap error drops the bubble', () => {
    const { conv, messages } = makeConv()
    messages.push({ role: 'user', content: 'q' })
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.attachRun.mockImplementation((_p, _id, h) => {
      handlers = h as Record<string, (...args: unknown[]) => void>
      return { close: vi.fn() }
    })
    const onEnd = vi.fn()
    conv.attach('r9', { onEnd })
    expect(api.agentThreads.attachRun).toHaveBeenCalledWith('p1', 'r9', expect.any(Object))
    expect(conv.runId.value).toBe('r9')
    expect(conv.sending.value).toBe(true)
    handlers.onToken?.('ok')
    expect(messages[1].content).toBe('ok')
    handlers.onEnd?.('stop')
    expect(conv.phase.value).toBe('done')
    expect(onEnd).toHaveBeenCalled()

    const onError = vi.fn()
    conv.attach('r10', { onError })
    handlers.onError?.(Object.assign(new Error('gap'), { code: 'stream_gap' }))
    expect(conv.phase.value).toBe('idle')
    expect(messages).toHaveLength(2)
    expect(onError).toHaveBeenCalled()
  })

  it('thinking content accumulates into reply.thinking', () => {
    const { conv, messages } = makeConv()
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      handlers = h as Record<string, (...args: unknown[]) => void>
      return { close: vi.fn() }
    })
    conv.start({ content: 'hi', mentions: [{ agent_id: 'a' }] }, {})
    handlers.onRun?.('r1')
    handlers.onThinking?.(100, '先分析')
    handlers.onThinking?.(200, '再查')
    expect(messages[1].thinking).toBe('先分析再查')
    expect(conv.elapsedMs.value).toBe(200)
  })

  // planv6.0 前端简化 C2：锁定工具卡匹配规则（按 call_id / 无 id 回退）、数组重新赋值与回调时序。
  describe('tool card matching (C2 characterization)', () => {
    function startWith(outer: Record<string, (...args: unknown[]) => void> = {}) {
      const ctx = makeConv()
      let handlers: Record<string, (...args: unknown[]) => void> = {}
      api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
        handlers = h as Record<string, (...args: unknown[]) => void>
        return { close: vi.fn() }
      })
      ctx.conv.start({ content: 'hi', mentions: [{ agent_id: 'a' }] }, outer)
      return { ...ctx, handlers, cards: () => ctx.messages[1].toolCalls || [] }
    }

    it('delta: id accumulates only while generating; missing id always appends', () => {
      const { handlers, cards } = startWith()
      handlers.onToolCallDelta('q', '{"a"', 'c1')
      handlers.onToolCallDelta('q', ':1}', 'c1')
      expect(cards()).toEqual([{ call_id: 'c1', name: 'q', arguments: '{"a":1}', status: 'generating' }])
      handlers.onToolCall('q', '{"a":1}', 'c1')
      const before = cards()
      handlers.onToolCallDelta('q', 'late', 'c1')
      expect(cards()).toBe(before)
      expect(cards()[0]).toMatchObject({ arguments: '{"a":1}', status: 'pending' })
      handlers.onToolCallDelta('n', 'x')
      handlers.onToolCallDelta('n', 'y')
      expect(cards().slice(1)).toEqual([
        { call_id: undefined, name: 'n', arguments: 'x', status: 'generating' },
        { call_id: undefined, name: 'n', arguments: 'y', status: 'generating' }
      ])
    })

    it('call: replaces args/name, upgrades only generating/empty status, appends without id', () => {
      const { handlers, cards } = startWith()
      handlers.onToolStart('q', 'c1')
      handlers.onToolCall('q2', 'A', 'c1')
      expect(cards()[0]).toMatchObject({ name: 'q2', arguments: 'A', status: 'running' })
      handlers.onToolCall('q', 'B')
      handlers.onToolCall('q', 'B')
      expect(cards()).toHaveLength(3)
      expect(cards()[2]).toEqual({ call_id: undefined, name: 'q', arguments: 'B', status: 'pending' })
    })

    it('start: without id marks the last same-name card; unmatched appends a running card', () => {
      const { handlers, cards } = startWith()
      handlers.onToolCall('q', '1', 'c1')
      handlers.onToolCall('q', '2', 'c2')
      handlers.onToolStart('q')
      expect(cards().map((c) => c.status)).toEqual(['pending', 'running'])
      handlers.onToolStart('other', 'cx')
      expect(cards()[2]).toEqual({ call_id: 'cx', name: 'other', status: 'running' })
    })

    it('result: without id fills the last same-name card lacking content, else pushes', () => {
      const { handlers, cards } = startWith()
      handlers.onToolCall('q', '1', 'c1')
      handlers.onToolCall('q', '2', 'c2')
      handlers.onToolResult('q', 'R2', undefined, 3, false, true)
      handlers.onToolResult('q', 'R1', undefined, 4, true)
      expect(cards().map((c) => [c.content, c.status, c.truncated, c.is_error])).toEqual([
        ['R1', 'error', false, true],
        ['R2', 'success', true, false]
      ])
      handlers.onToolResult('q', 'R3')
      expect(cards()[2]).toEqual({
        call_id: undefined, name: 'q', content: 'R3', duration_ms: undefined,
        is_error: false, truncated: false, status: 'success'
      })
    })

    it('confirmation: matches by exact id (no fallback) and keeps unknown ids card-less', () => {
      const { conv, handlers, cards } = startWith()
      handlers.onToolCall('cli', '{}', 'c1')
      handlers.onConfirmationRequired('nope', ['ls'], 'm')
      expect(conv.phase.value).toBe('awaiting_confirm')
      expect(cards()[0].status).toBe('pending')
      handlers.onConfirmationRequired('c1', ['rm', '-rf'], 'm')
      expect(cards()[0]).toMatchObject({ status: 'awaiting_confirm', argv: ['rm', '-rf'] })
      handlers.onToolCall('cli', '{}', 'c1')
      expect(conv.phase.value).toBe('awaiting_confirm')
      expect(cards()[0].status).toBe('awaiting_confirm')
    })

    it('updates reply (new array reference) before forwarding to outer handlers', () => {
      const seen: string[] = []
      const ctx = { cards: (): { status?: string }[] => [] }
      const outer = {
        onToolCall: () => { seen.push('call:' + ctx.cards().map((c) => c.status).join()) },
        onToolStart: () => { seen.push('start:' + ctx.cards().map((c) => c.status).join()) },
        onToolResult: () => { seen.push('result:' + ctx.cards().map((c) => c.status).join()) }
      }
      const { handlers, cards } = startWith(outer)
      ctx.cards = cards
      handlers.onToolCall('q', '{}', 'c1')
      const afterCall = cards()
      handlers.onToolStart('q', 'c1')
      expect(cards()).not.toBe(afterCall)
      const afterStart = cards()
      handlers.onToolResult('q', 'ok', 'c1', 1)
      expect(cards()).not.toBe(afterStart)
      expect(seen).toEqual(['call:pending', 'start:running', 'result:success'])
    })
  })
})
