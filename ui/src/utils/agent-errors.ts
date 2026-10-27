// agent-errors.ts：云助手错误码 → 用户文案（planv4.1 BUG-05）。
// 后端错误码见 internal/gocloudagent/errors.go；未知码回退原始 message。
//
// 文案统一走 i18n（plan/planv5.0/ui-optimization-v3-plan.md §3.2）：
// 这里只映射 key，不存中文字面量；组件外没有 useI18n 上下文，用 i18n.global.t。
import { t } from '../locales'

export interface AgentError {
  code?: string
  message?: string
}

const CODE_KEYS: Record<string, string> = {
  llm_not_configured: 'error.codes.llm_not_configured',
  llm_auth_failed: 'error.codes.llm_auth_failed',
  llm_rate_limited: 'error.codes.llm_rate_limited',
  llm_timeout: 'error.codes.llm_timeout',
  llm_upstream_error: 'error.codes.llm_upstream_error',
  llm_model_not_allowed: 'error.codes.llm_model_not_allowed',
  agent_thread_busy: 'error.codes.agent_thread_busy',
  quota_exceeded: 'error.codes.quota_exceeded',
  invalid_retry: 'error.codes.invalid_retry',
  bad_request: 'error.codes.bad_request',
  unauthorized: 'error.codes.unauthorized',
  forbidden: 'error.codes.forbidden',
  not_found: 'error.codes.not_found',
  internal_error: 'error.codes.internal_error'
}

/**
 * 把后端错误（或任意抛出）转成用户可读文案。
 * `fallback` 可以是 i18n key（`error.*`）也可以是已翻译文案；
 * 未知码仍回退后端原始 message —— 后端自由文本不翻译。
 */
export function agentErrorText(
  e: AgentError | unknown,
  fallback = 'error.runFailed'
): string {
  if (!e) return t(fallback)
  if (typeof e === 'string') {
    const key = CODE_KEYS[e]
    return key ? t(key) : e
  }
  const err = e as AgentError
  if (err.code && CODE_KEYS[err.code]) return t(CODE_KEYS[err.code])
  if (err.message) return err.message
  return t(fallback)
}

/** 是否为可重试的错误（失败/超时/上游错误）。 */
export function isRetryableError(e: AgentError | unknown): boolean {
  const err = e as AgentError
  return !!err?.code && ['llm_timeout', 'llm_upstream_error', 'llm_rate_limited', 'internal_error'].includes(err.code)
}
