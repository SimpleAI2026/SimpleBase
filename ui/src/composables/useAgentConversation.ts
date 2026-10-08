import { computed, ref } from 'vue'
import { api } from '@/services/api'
import { agentErrorText } from '@/utils/agent-errors'
import type { AgentRunMetrics, AgentStreamHandlers, LlmStreamConnection } from '@/services/types'
import type { ChatMsg } from './useAiChat'

/** 运行阶段（BUG-01：前端必须追踪流式回调，工具期间不回落）。 */
export type AgentPhase = 'idle' | 'thinking' | 'streaming' | 'tool' | 'done' | 'error' | 'canceled'

export interface ConversationOptions {
  projectId: () => string
  threadId: () => string
  /** 消息数组（外部持有）。 */
  messages: () => ChatMsg[]
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

  let conn: LlmStreamConnection | null = null
  let timer: ReturnType<typeof setInterval> | null = null
  let started = 0

  const sending = computed(() => phase.value === 'thinking' || phase.value === 'streaming' || phase.value === 'tool')

  const statusText = computed(() => {
    if (phase.value === 'thinking') return `思考中 · ${Math.floor(elapsedMs.value / 1000)}s`
    if (phase.value === 'tool') return `执行工具 ${toolName.value}…`
    if (phase.value === 'streaming') return `回复中 · ${Math.floor(elapsedMs.value / 1000)}s`
    if (phase.value === 'done' && metrics.value) {
      const secs = (metrics.value.duration_ms / 1000).toFixed(1)
      const tokens = metrics.value.prompt_tokens + metrics.value.completion_tokens
      return `完成 · ${secs}s · ${tokens} tokens · ${metrics.value.tool_calls} 次工具`
    }
    if (phase.value === 'canceled') return '已停止'
    if (phase.value === 'error') return lastError.value?.text || '运行失败'
    return ''
  })

  function start(
    body: { content: string; mentions: { agent_id: string }[]; retry_of_run_id?: string },
    handlers: AgentStreamHandlers & { onReply?: (reply: ChatMsg) => void }
  ) {
    stop(false)
    phase.value = 'thinking'
    elapsedMs.value = 0
    toolName.value = ''
    metrics.value = null
    runId.value = ''
    lastError.value = null
    failedRunId.value = ''
    started = Date.now()
    timer = setInterval(() => { elapsedMs.value = Date.now() - started }, 1000)

    const messages = opts.messages()
    // 重试时由调用方准备好消息列表（已移除失败气泡）；新消息在此追加。
    if (!body.retry_of_run_id) {
      messages.push({ role: 'user', content: body.content })
    }
    const reply: ChatMsg = { role: 'assistant', content: '', toolCalls: [] }
    messages.push(reply)
    handlers.onReply?.(reply)

    conn = api.agentThreads.streamRun(opts.projectId(), opts.threadId(), {
      ...body,
      stream: true
    }, {
      onRun: (id) => { runId.value = id; handlers.onRun?.(id) },
      onThinking: (ms, content) => {
        elapsedMs.value = ms
        // BUG-09：工具执行期间的 thinking 心跳不应把 phase 拉回 thinking。
        if (phase.value !== 'tool') phase.value = 'thinking'
        if (content) reply.thinking = (reply.thinking || '') + content
        handlers.onThinking?.(ms, content)
      },
      onToken: (text) => { phase.value = 'streaming'; reply.content += text; handlers.onToken?.(text) },
      onToolCall: (name, args, callId) => {
        toolName.value = name
        phase.value = 'tool'
        if (callId && reply.toolCalls?.some((c) => c.call_id === callId)) return
        reply.toolCalls = [...(reply.toolCalls || []), { call_id: callId, name, arguments: args }]
        handlers.onToolCall?.(name, args, callId)
      },
      onToolResult: (name, content, callId, durationMs) => {
        // 工具结果到达后保持 tool → thinking（等下一个 token），而非立即 streaming。
        phase.value = 'thinking'
        const cards = reply.toolCalls || []
        const target = callId
          ? cards.find((c) => c.call_id === callId)
          : [...cards].reverse().find((c) => c.name === name && !c.content)
        if (target) { target.content = content; target.duration_ms = durationMs }
        else cards.push({ call_id: callId, name, content, duration_ms: durationMs })
        reply.toolCalls = [...cards]
        handlers.onToolResult?.(name, content, callId, durationMs)
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
        const text = agentErrorText(err)
        phase.value = 'error'
        lastError.value = { code: err?.code, text }
        failedRunId.value = runId.value
        reply.error = text
        cleanup()
        handlers.onError?.(e)
      }
    })
  }

  function cleanup() {
    if (timer) clearInterval(timer)
    timer = null
    conn = null
  }

  function stop(cancel = true) {
    const active = conn
    active?.close()
    if (cancel && runId.value && active) {
      void api.agentThreads.cancel(opts.projectId(), runId.value).catch(() => undefined)
    }
    if (active && phase.value !== 'error' && phase.value !== 'done') phase.value = 'canceled'
    cleanup()
  }

  return { phase, statusText, metrics, runId, sending, lastError, failedRunId, elapsedMs, start, stop }
}
