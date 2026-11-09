import { describe, expect, it, vi } from 'vitest'
import { agentErrorText, isRetryableError } from '@/utils/agent-errors'

describe('agent-errors', () => {
  it('maps known codes to labels', () => {
    expect(agentErrorText({ code: 'llm_auth_failed' })).toContain('鉴权失败')
    expect(agentErrorText({ code: 'invalid_retry' })).toContain('无法重试')
    expect(agentErrorText({ code: 'agent_thread_busy' })).toContain('运行中')
  })

  it('falls back to message or default', () => {
    expect(agentErrorText({ code: 'unknown_x', message: 'boom' })).toBe('boom')
    expect(agentErrorText({ code: 'unknown_x' }, 'fb')).toBe('fb')
    expect(agentErrorText(null, 'fb')).toBe('fb')
    expect(agentErrorText('llm_timeout')).toContain('超时')
  })

  it('classifies retryable errors', () => {
    expect(isRetryableError({ code: 'llm_timeout' })).toBe(true)
    expect(isRetryableError({ code: 'llm_upstream_error' })).toBe(true)
    expect(isRetryableError({ code: 'llm_auth_failed' })).toBe(false)
    expect(isRetryableError(null)).toBe(false)
  })
})
