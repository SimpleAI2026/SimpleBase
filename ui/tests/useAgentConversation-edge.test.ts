import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { useAgentConversation } from '@/composables/useAgentConversation'
import type { ChatMsg } from '@/composables/useAiChat'

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
