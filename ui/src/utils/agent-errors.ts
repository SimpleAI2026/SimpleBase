// agent-errors.ts：云助手错误码 → 用户文案（planv4.1 BUG-05）。
// 后端错误码见 internal/cloudagent/errors.go；未知码回退原始 message。

export interface AgentError {
  code?: string
  message?: string
}

const LABELS: Record<string, string> = {
  llm_not_configured: '模型服务未配置，请先在设置中配置 LLM 供应商',
  llm_auth_failed: '模型服务鉴权失败，请检查 API Key',
  llm_rate_limited: '模型服务限流，请稍后重试',
  llm_timeout: '模型响应超时，请稍后重试',
  llm_upstream_error: '模型服务暂时不可用，请稍后重试',
  llm_model_not_allowed: '模型不在允许列表中，请检查模型配置',
  agent_thread_busy: '当前会话仍在运行中，请稍候或先停止',
  quota_exceeded: '模型调用配额已用完',
  invalid_retry: '无法重试该会话（仅最近一次失败的运行可重试）',
  bad_request: '请求参数有误',
  unauthorized: '登录状态已过期，请重新登录',
  forbidden: '没有执行该操作的权限',
  not_found: '资源不存在',
  internal_error: '服务内部错误，请稍后重试'
}

/** 把后端错误（或任意抛出）转成用户可读文案。 */
export function agentErrorText(e: AgentError | unknown, fallback = '运行失败'): string {
  if (!e) return fallback
  if (typeof e === 'string') return LABELS[e] || e
  const err = e as AgentError
  if (err.code && LABELS[err.code]) return LABELS[err.code]
  if (err.message) return err.message
  return fallback
}

/** 是否为可重试的错误（失败/超时/上游错误）。 */
export function isRetryableError(e: AgentError | unknown): boolean {
  const err = e as AgentError
  return !!err?.code && ['llm_timeout', 'llm_upstream_error', 'llm_rate_limited', 'internal_error'].includes(err.code)
}
