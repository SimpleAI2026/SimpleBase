import { describe, expect, it, vi } from 'vitest'
import { useAgentRun } from '@/composables/useAgentRun'
import type { AgentStreamHandlers } from '@/services/types'

describe('useAgentRun', () => {
  it('tracks thinking, streaming, tools, usage and cancellation', () => {
    let callbacks: AgentStreamHandlers = {}
    const run = useAgentRun(() => 'p')
    const close = vi.fn()
    run.start((handlers) => { callbacks = handlers; return { close } }, {})
    expect(run.phase.value).toBe('thinking')
    callbacks.onThinking?.(2000)
    expect(run.statusText.value).toContain('2s')
    callbacks.onToken?.('hello')
    expect(run.phase.value).toBe('streaming')
    callbacks.onToolCall?.('readonly_sql', '{}', 'c1')
    expect(run.statusText.value).toContain('readonly_sql')
    callbacks.onToolResult?.('readonly_sql', '[]', 'c1')
    expect(run.phase.value).toBe('thinking')
    callbacks.onUsage?.({ duration_ms: 1000, prompt_tokens: 10, completion_tokens: 5, reasoning_tokens: 0, tool_calls: 1 })
    callbacks.onEnd?.('stop')
    expect(run.statusText.value).toContain('15 tokens')
    run.start((handlers) => { callbacks = handlers; return { close } }, {})
    run.stop(false)
    expect(run.phase.value).toBe('canceled')
    expect(close).toHaveBeenCalled()
  })
  it('shows idle, running and failure status for all phases', () => {
    let callbacks: AgentStreamHandlers = {}
    const run = useAgentRun(() => 'p')
    expect(run.statusText.value).toBe('')
    run.start((handlers) => { callbacks = handlers; return { close: vi.fn() } }, {})
    callbacks.onToken?.('hello')
    expect(run.statusText.value).toContain('回复中')
    callbacks.onError?.(new Error('fail'))
    expect(run.statusText.value).toBe('运行失败')
    run.start((handlers) => { callbacks = handlers; return { close: vi.fn() } }, {})
    callbacks.onEnd?.('stop')
    expect(run.phase.value).toBe('done')
    expect(run.statusText.value).toBe('')
    run.start((handlers) => { callbacks = handlers; return { close: vi.fn() } }, {})
    callbacks.onEnd?.('canceled')
    expect(run.statusText.value).toBe('已停止')
  })
})