import { computed, reactive, ref } from 'vue'
import { api } from '@/services/api'
import { t } from '@/locales'
import { toast } from 'vue-sonner'
import { errorMessage } from '@/utils/format'
import { agentErrorText } from '@/utils/agent-errors'
import type { AgentRunMetrics, AgentStreamHandlers, AgentToolCallCard, LlmStreamConnection } from '@/services/types'
import type { ChatMsg } from './useAiChat'

/** 运行阶段（BUG-01：前端必须追踪流式回调，工具期间不回落）。 */
export type AgentPhase = 'idle' | 'thinking' | 'streaming' | 'tool' | 'awaiting_confirm' | 'done' | 'error' | 'canceled'

export interface ConversationOptions {
  projectId: () => string
  threadId: () => string
  /** 消息数组（外部持有）。 */
  messages: () => ChatMsg[]
}

/**
 * 定位工具卡：有 call_id 时按 id 精确匹配；无 id 时若给了回退条件，取最后一张满足条件的卡。
 * 仅供 delta / call / start / result 使用；确认事件按原样精确匹配，不走回退。
 */
function findToolCard(
  cards: AgentToolCallCard[],
  callId: string | undefined,
  fallback?: (card: AgentToolCallCard) => boolean
): AgentToolCallCard | undefined {
  if (callId) return cards.find((c) => c.call_id === callId)
  if (!fallback) return undefined
  return [...cards].reverse().find(fallback)
}

/**
 * useAgentConversation：单会话运行状态层（planv4.1 BUG-01/03/09/10）。
 * - 追踪全部 SSE 回调，工具执行期间 phase 停留在 tool（不跳回 thinking）
 * - 重试走后端 retry_of_run_id（不重复追加 user 消息）
 * - 页面刷新后从 run_status/error_code 还原失败态
 */
export function useAgentConversation(opts: ConversationOptions) {
  const phase = ref<AgentPhase>('idle')
  const elapsedMs = ref(0)
  const toolName = ref('')
  const metrics = ref<AgentRunMetrics | null>(null)
  const runId = ref('')
  const lastError = ref<{ code?: string; text: string } | null>(null)
  /** 待重试的失败 run id（空表示无失败 run）。 */
  const failedRunId = ref('')
  const confirmation = ref<{ callId: string; argv: string[]; message: string } | null>(null)

  let conn: LlmStreamConnection | null = null
  let timer: ReturnType<typeof setInterval> | null = null
  let started = 0

  const sending = computed(() => phase.value === 'thinking' || phase.value === 'streaming' || phase.value === 'tool' || phase.value === 'awaiting_confirm')

  const statusText = computed(() => {
    const secs = Math.floor(elapsedMs.value / 1000)
    if (phase.value === 'thinking') return t('agentPhase.thinking', { s: secs })
    if (phase.value === 'awaiting_confirm') return t('agentPhase.awaitingConfirm')
    if (phase.value === 'tool') return t('agentPhase.tool', { name: toolName.value })
    if (phase.value === 'streaming') return t('agentPhase.streaming', { s: secs })
    if (phase.value === 'done' && metrics.value) {
      return t('agentPhase.done', {
        secs: (metrics.value.duration_ms / 1000).toFixed(1),
        tokens: metrics.value.prompt_tokens + metrics.value.completion_tokens,
        calls: metrics.value.tool_calls
      })
    }
    if (phase.value === 'canceled') return t('agentPhase.canceled')
    if (phase.value === 'error') return lastError.value?.text || t('agentPhase.runFailed')
    return ''
  })

  function begin() {
    phase.value = 'thinking'
    elapsedMs.value = 0
    toolName.value = ''
    metrics.value = null
    runId.value = ''
    lastError.value = null
    failedRunId.value = ''
    confirmation.value = null
    started = Date.now()
    timer = setInterval(() => { elapsedMs.value = Date.now() - started }, 1000)
  }

  type Handlers = AgentStreamHandlers & { onReply?: (reply: ChatMsg) => void }

  function start(
    body: { content: string; mentions: { agent_id: string }[]; module?: string; retry_of_run_id?: string },
    handlers: Handlers
  ) {
    stop(false)
    begin()

    const messages = opts.messages()
    // 重试时由调用方准备好消息列表（已移除失败气泡）；新消息在此追加。
    if (!body.retry_of_run_id) {
      messages.push({ role: 'user', content: body.content })
    }
    const reply = reactive<ChatMsg>({ role: 'assistant', content: '', toolCalls: [], module: body.module })
    messages.push(reply)
    handlers.onReply?.(reply)

    conn = api.agentThreads.streamRun(opts.projectId(), opts.threadId(), {
      ...body,
      stream: true
    }, bindHandlers(reply, handlers, false))
  }

  /**
   * 续接后台仍在运行的 run（切换界面后回到会话）：新建 assistant 气泡，
   * 从头回放事件缓存重建正文 / 工具卡，再跟随到 end。
   */
  function attach(id: string, handlers: Handlers & { module?: string } = {}) {
    stop(false)
    begin()
    runId.value = id
    const reply = reactive<ChatMsg>({ role: 'assistant', content: '', toolCalls: [], module: handlers.module })
    opts.messages().push(reply)
    handlers.onReply?.(reply)
    conn = api.agentThreads.attachRun(opts.projectId(), id, bindHandlers(reply, handlers, true))
  }

  function bindHandlers(reply: ChatMsg, handlers: Handlers, attached: boolean): AgentStreamHandlers {
    return {
      onRun: (id) => { runId.value = id; handlers.onRun?.(id) },
      onThinking: (ms, content) => {
        elapsedMs.value = ms
        // BUG-09：工具执行期间的 thinking 心跳不应把 phase 拉回 thinking。
        if (phase.value !== 'tool' && phase.value !== 'awaiting_confirm') phase.value = 'thinking'
        if (content) reply.thinking = (reply.thinking || '') + content
        handlers.onThinking?.(ms, content)
      },
      onToken: (text) => { phase.value = 'streaming'; reply.content += text; handlers.onToken?.(text) },
      onToolCallDelta: (name, argsDelta, callId) => {
        toolName.value = name
        phase.value = 'tool'
        const cards = reply.toolCalls || []
        const target = findToolCard(cards, callId)
        if (!target) {
          reply.toolCalls = [...cards, { call_id: callId, name, arguments: argsDelta, status: 'generating' }]
        } else if (target.status === 'generating' || !target.status) {
          target.arguments = (target.arguments || '') + argsDelta
          target.status = 'generating'
          reply.toolCalls = [...cards]
        }
        handlers.onToolCallDelta?.(name, argsDelta, callId)
      },
      onToolCall: (name, args, callId) => {
        toolName.value = name
        if (phase.value !== 'awaiting_confirm') phase.value = 'tool'
        const cards = reply.toolCalls || []
        const target = findToolCard(cards, callId)
        if (target) {
          target.arguments = args
          target.name = name
          if (target.status === 'generating' || !target.status) target.status = 'pending'
          reply.toolCalls = [...cards]
        } else {
          reply.toolCalls = [...cards, { call_id: callId, name, arguments: args, status: 'pending' }]
        }
        handlers.onToolCall?.(name, args, callId)
      },
      onToolStart: (name, callId) => {
        toolName.value = name
        phase.value = 'tool'
        const cards = reply.toolCalls || []
        const target = findToolCard(cards, callId, (c) => c.name === name)
        if (target) {
          target.status = 'running'
          reply.toolCalls = [...cards]
        } else {
          reply.toolCalls = [...cards, { call_id: callId, name, status: 'running' }]
        }
        handlers.onToolStart?.(name, callId)
      },
      onConfirmationRequired: (callId, argv, message) => {
        phase.value = 'awaiting_confirm'
        confirmation.value = { callId, argv, message }
        const cards = reply.toolCalls || []
        const target = cards.find((c) => c.call_id === callId)
        if (target) {
          target.status = 'awaiting_confirm'
          target.argv = argv
          reply.toolCalls = [...cards]
        }
        handlers.onConfirmationRequired?.(callId, argv, message)
      },
      onConfirmationResolved: (callId, approved) => {
        if (confirmation.value?.callId === callId) confirmation.value = null
        if (phase.value === 'awaiting_confirm') phase.value = 'tool'
        handlers.onConfirmationResolved?.(callId, approved)
      },
      onToolResult: (name, content, callId, durationMs, isError, truncated) => {
        // 工具结果到达后保持 tool → thinking（等下一个 token），而非立即 streaming。
        phase.value = 'thinking'
        confirmation.value = null
        const cards = reply.toolCalls || []
        const target = findToolCard(cards, callId, (c) => c.name === name && !c.content)
        if (target) {
          target.content = content
          target.duration_ms = durationMs
          target.is_error = !!isError
          target.truncated = !!truncated
          target.status = isError ? 'error' : 'success'
        } else cards.push({ call_id: callId, name, content, duration_ms: durationMs, is_error: !!isError, truncated: !!truncated, status: isError ? 'error' : 'success' })
        reply.toolCalls = [...cards]
        handlers.onToolResult?.(name, content, callId, durationMs, isError, truncated)
      },
      onToolProgress: (name, elapsed, callId) => {
        // 心跳同步运行时长（BUG-04：长工具期间前端可见进度，不回落 thinking）。
        elapsedMs.value = elapsed
        handlers.onToolProgress?.(name, elapsed, callId)
      },
      onUsage: (value) => { metrics.value = value; handlers.onUsage?.(value) },
      onEnd: (reason) => {
        phase.value = reason === 'canceled' ? 'canceled' : 'done'
        if (reason === 'canceled') reply.canceled = true
        cleanup()
        handlers.onEnd?.(reason)
      },
      onError: (e) => {
        const err = e as Error & { code?: string }
        // 续接失败（事件已过期 stream_gap 等）不代表 run 失败：回到空闲，由宿主刷新消息。
        if (attached) {
          phase.value = 'idle'
          const list = opts.messages()
          const idx = list.indexOf(reply)
          if (idx >= 0) list.splice(idx, 1)
          cleanup()
          handlers.onError?.(e)
          return
        }
        const text = agentErrorText(err)
        phase.value = 'error'
        lastError.value = { code: err?.code, text }
        failedRunId.value = runId.value
        reply.error = text
        cleanup()
        handlers.onError?.(e)
      }
    }
  }

  function cleanup() {
    if (timer) clearInterval(timer)
    timer = null
    conn = null
  }

  async function resolveConfirm(approve: boolean) {
    const pending = confirmation.value
    const currentRunId = runId.value
    if (!pending || !currentRunId) return
    try {
      await api.agentThreads.confirm(opts.projectId(), currentRunId, pending.callId, approve)
      if (runId.value === currentRunId && confirmation.value?.callId === pending.callId) {
        confirmation.value = null
        if (phase.value === 'awaiting_confirm') phase.value = 'tool'
      }
    } catch (e) {
      toast.error(errorMessage(e, t('pages.agents.confirmFailed')))
    }
  }

  /**
   * cancel=true：用户点停止，关闭连接并让后端取消 run。
   * cancel=false：仅断开连接（切换会话 / 离开页面），后端 run 继续，回来后可 attach 续接。
   */
  function stop(cancel = true) {
    const active = conn
    active?.close()
    if (cancel && runId.value && active) {
      void api.agentThreads.cancel(opts.projectId(), runId.value).catch((e) => {
        toast.error(`${t('pages.agents.cancelFailed')}：${errorMessage(e, t('pages.agents.cancelFailed'))}`)
      })
    }
    if (active && phase.value !== 'error' && phase.value !== 'done') phase.value = cancel ? 'canceled' : 'idle'
    cleanup()
  }

  return { phase, statusText, metrics, runId, sending, lastError, failedRunId, elapsedMs, confirmation, start, attach, stop, resolveConfirm }
}
