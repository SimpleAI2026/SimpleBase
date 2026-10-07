import { ref, computed, getCurrentInstance, onUnmounted } from 'vue'
import { api } from '@/services/api'
import type { AgentRunMetrics, AgentStreamHandlers, LlmStreamConnection } from '@/services/types'

export type AgentRunPhase = 'idle' | 'thinking' | 'streaming' | 'tool' | 'done' | 'error' | 'canceled'

export function useAgentRun(projectId: () => string) {
  const phase = ref<AgentRunPhase>('idle')
  const elapsedMs = ref(0)
  const toolName = ref('')
  const metrics = ref<AgentRunMetrics | null>(null)
  const runId = ref('')
  let conn: LlmStreamConnection | null = null
  let timer: ReturnType<typeof setInterval> | null = null
  let started = 0

  const statusText = computed(() => {
    if (phase.value === 'thinking') return `思考中 · ${Math.floor(elapsedMs.value / 1000)}s`
    if (phase.value === 'tool') return `调用 ${toolName.value}…`
    if (phase.value === 'streaming') return `回复中 · ${Math.floor(elapsedMs.value / 1000)}s`
    if (phase.value === 'done' && metrics.value) return `完成 · ${(metrics.value.duration_ms / 1000).toFixed(1)}s · ${metrics.value.prompt_tokens + metrics.value.completion_tokens} tokens · ${metrics.value.tool_calls} 次工具`
    if (phase.value === 'canceled') return '已停止'
    if (phase.value === 'error') return '运行失败'
    return ''
  })

  function start(connect: (handlers: AgentStreamHandlers) => LlmStreamConnection, handlers: AgentStreamHandlers) {
    stop(false)
    phase.value = 'thinking'
    elapsedMs.value = 0
    toolName.value = ''
    metrics.value = null
    runId.value = ''
    started = Date.now()
    timer = setInterval(() => { elapsedMs.value = Date.now() - started }, 1000)
    conn = connect({
      ...handlers,
      onRun: (id) => { runId.value = id; handlers.onRun?.(id) },
      onThinking: (ms, content) => { elapsedMs.value = ms; phase.value = 'thinking'; handlers.onThinking?.(ms, content) },
      onToken: (text) => { phase.value = 'streaming'; handlers.onToken?.(text) },
      onToolCall: (name, args, id) => { toolName.value = name; phase.value = 'tool'; handlers.onToolCall?.(name, args, id) },
      onToolResult: (name, content, id, ms) => { phase.value = 'thinking'; handlers.onToolResult?.(name, content, id, ms) },
      onUsage: (value) => { metrics.value = value; handlers.onUsage?.(value) },
      onEnd: (reason) => { phase.value = reason === 'canceled' ? 'canceled' : 'done'; cleanup(); handlers.onEnd?.(reason) },
      onError: (error) => { phase.value = 'error'; cleanup(); handlers.onError?.(error) }
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
    if (cancel && runId.value && active) void api.agentThreads.cancel(projectId(), runId.value).catch(() => undefined)
    if (active) phase.value = 'canceled'
    cleanup()
  }

  if (getCurrentInstance()) onUnmounted(() => stop())
  return { phase, statusText, metrics, runId, start, stop }
}
