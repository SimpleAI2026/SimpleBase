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
})
