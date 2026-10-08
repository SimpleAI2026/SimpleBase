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
})
