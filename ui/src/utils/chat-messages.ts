import { t } from '@/locales'
import type { AgentMessage, ChatMsg } from '@/services/types'
import { errorMessage } from '@/utils/format'

/**
 * 持久化消息 → 聊天气泡：run 状态还原失败 / 取消态；assistant 按其归属助手补 module 与路由结论。
 * `agentModule` 由调用方按 `m.agent_id` 解析（这里不依赖助手列表）。
 */
export function toChatMessage(m: AgentMessage, agentModule?: string): ChatMsg {
  return {
    id: m.id,
    role: m.role === 'assistant' ? 'assistant' : 'user',
    content: m.content,
    module: m.role === 'assistant' ? agentModule : undefined,
    agentId: m.role === 'assistant' ? m.agent_id : undefined,
    toolCalls: m.tool_calls,
    error: m.error_code ? errorMessage({ code: m.error_code } as Error & { code?: string }, t('pages.agents.runFailed')) : undefined,
    canceled: m.run_status === 'canceled'
  }
}

/** 批量转换：`moduleOf(agentId)` 解析每条消息归属助手的 module。 */
export function toChatMessages(list: AgentMessage[], moduleOf: (agentId: string) => string | undefined): ChatMsg[] {
  return list.map((m) => toChatMessage(m, moduleOf(m.agent_id ?? '')))
}
