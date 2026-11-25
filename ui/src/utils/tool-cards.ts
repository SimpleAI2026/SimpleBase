import type { AgentToolCallCard } from '@/services/types'

/**
 * 工具卡合并：单助手会话与协作轮次共用的纯函数（不改入参数组，返回新数组）。
 *
 * 匹配规则：有 call_id 时只按 id 精确匹配；无 id 时才用调用方给的回退条件，
 * 取最后一张满足条件的卡。确认事件不走这里（按 id 精确匹配）。
 */

type Fallback = (card: AgentToolCallCard) => boolean

function findCard(cards: AgentToolCallCard[], callId: string | undefined, fallback?: Fallback): AgentToolCallCard | undefined {
  if (callId) return cards.find((c) => c.call_id === callId)
  if (!fallback) return undefined
  return [...cards].reverse().find(fallback)
}

/** 参数流式增量：仅在卡片仍处于「生成中 / 无状态」时追加；已进入后续状态的卡原样返回同一个数组。 */
export function applyToolDelta(cards: AgentToolCallCard[], name: string, argsDelta: string, callId?: string): AgentToolCallCard[] {
  const target = findCard(cards, callId)
  if (!target) return [...cards, { call_id: callId, name, arguments: argsDelta, status: 'generating' }]
  if (target.status !== 'generating' && target.status) return cards
  target.arguments = (target.arguments || '') + argsDelta
  target.status = 'generating'
  return [...cards]
}

/** 完整调用：参数以此为准；生成中的卡进入 pending，找不到则新增。 */
export function applyToolCall(cards: AgentToolCallCard[], name: string, args: string, callId?: string): AgentToolCallCard[] {
  const target = findCard(cards, callId)
  if (!target) return [...cards, { call_id: callId, name, arguments: args, status: 'pending' }]
  target.arguments = args
  target.name = name
  if (target.status === 'generating' || !target.status) target.status = 'pending'
  return [...cards]
}

/** 开始执行：无 id 时按同名卡兜底，找不到则新增 running 卡。 */
export function applyToolStart(cards: AgentToolCallCard[], name: string, callId?: string): AgentToolCallCard[] {
  const target = findCard(cards, callId, (c) => c.name === name)
  if (!target) return [...cards, { call_id: callId, name, status: 'running' }]
  target.status = 'running'
  return [...cards]
}

export interface ToolResultInfo {
  content: string
  durationMs?: number
  isError?: boolean
  truncated?: boolean
}

/** 执行结果：无 id 时按「同名且尚无结果」兜底，找不到则新增（乱序到达的结果也不丢）。 */
export function applyToolResult(cards: AgentToolCallCard[], name: string, info: ToolResultInfo, callId?: string): AgentToolCallCard[] {
  const status = info.isError ? 'error' : 'success'
  const target = findCard(cards, callId, (c) => c.name === name && !c.content)
  if (!target) {
    return [...cards, {
      call_id: callId,
      name,
      content: info.content,
      duration_ms: info.durationMs,
      is_error: !!info.isError,
      truncated: !!info.truncated,
      status
    }]
  }
  target.content = info.content
  target.duration_ms = info.durationMs
  target.is_error = !!info.isError
  target.truncated = !!info.truncated
  target.status = status
  return [...cards]
}

/** 等待确认：只按 call_id 精确匹配，找不到就不动。 */
export function applyToolConfirmation(cards: AgentToolCallCard[], callId: string, argv: string[]): AgentToolCallCard[] {
  const target = cards.find((c) => c.call_id === callId)
  if (!target) return cards
  target.status = 'awaiting_confirm'
  target.argv = argv
  return [...cards]
}
