// 云助手 SSE 流：逐帧解析、run / turn 发起、续接与断线重连。唯一含 fetch 与重连状态机的模块。

import { orElse, orUndefined, type WireRecord } from '@/services/wire'
import type {
  AgentRunRequest,
  AgentStreamHandlers,
  AgentTurnRequest,
  LlmStreamConnection
} from '@/services/types'
import { t } from '@/locales'
import { baseURL, getAccessToken, getApiKey } from '@/services/http'
import { projectPath } from '@/services/paths'
import { isAgentTaskStatus, toAgentPlanDecision, toAgentRunMetrics } from '@/services/converters/agent'

/**
 * 逐帧解析 SSE 的 `data:` 行并回调 JSON 对象。
 *
 * `onDone` 仅供 OpenAI 兼容流使用：它的结束哨兵是字面量 `[DONE]`（不是 JSON），
 * 解析会抛错并被静默忽略，调用方因此永远等不到结束信号、界面一直转圈。
 * 云助手 run 流仍以 `{"type":"end"}` 收尾，不传 onDone，行为与之前完全一致。
 *
 * `shouldStop` 在每个数据块分发完后询问调用方是否已得出终态（如收到 error 帧）。
 * 为真时用 `reader.cancel()` 协作式停止读取，而不是从外部 abort 整个 fetch：
 * 页面里若有第三方包了 fetch 并 clone 了响应体（浏览器扩展的请求检查等），
 * abort 会让那份读取以 AbortError 失败并被打到控制台；cancel 只结束自己这一支。
 */
async function readSse(
  resp: Response,
  onEvent: (event: WireRecord) => void,
  onDone?: () => void,
  shouldStop: () => boolean = () => false
): Promise<void> {
  const reader = resp.body!.getReader()
  const decoder = new TextDecoder()
  let buf = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buf += decoder.decode(value, { stream: true })
    let idx: number
    while ((idx = buf.indexOf('\n\n')) >= 0) {
      const raw = buf.slice(0, idx)
      buf = buf.slice(idx + 2)
      let eventId = ''
      for (const line of raw.split('\n')) {
        if (line.startsWith(':')) continue
        if (line.startsWith('id:')) {
          eventId = line.slice(3).trim()
          continue
        }
        if (!line.startsWith('data:')) continue
        const data = line.slice(5).trim()
        if (!data) continue
        if (onDone && data === '[DONE]') {
          onDone()
          continue
        }
        try {
          const parsed = JSON.parse(data) as WireRecord
          if (eventId) parsed._sse_id = eventId
          onEvent(parsed)
        } catch {
          /* 忽略非 JSON 帧 */
        }
      }
    }
    if (shouldStop()) {
      await reader.cancel().catch(() => undefined)
      return
    }
  }
}

/** 发起一轮：走 /turns，支持多目标 @。 */
export function streamAgentTurn(
  projectId: string,
  threadId: string,
  req: AgentTurnRequest,
  handlers: AgentStreamHandlers
): LlmStreamConnection {
  return openAgentRunStream(projectId, handlers, '', (signal) =>
    fetch(`${baseURL}${projectPath(projectId, 'agent-threads', threadId, 'turns')}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${getAccessToken() || getApiKey()}`
      },
      body: JSON.stringify({
        content: req.content,
        mentions: req.mentions,
        stream: true,
        ...(req.idempotencyKey ? { idempotency_key: req.idempotencyKey } : {}),
        ...(req.threadKind ? { thread_kind: req.threadKind } : {})
      }),
      signal
    }), true
  )
}

export function streamAgentRun(
  projectId: string,
  threadId: string,
  req: AgentRunRequest,
  handlers: AgentStreamHandlers
): LlmStreamConnection {
  return openAgentRunStream(projectId, handlers, '', (signal) =>
    fetch(`${baseURL}${projectPath(projectId, 'agent-threads', threadId, 'runs')}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${getAccessToken() || getApiKey()}`
      },
      body: JSON.stringify({
        content: req.content,
        mentions: req.mentions,
        stream: true,
        ...(req.retry_of_run_id ? { retry_of_run_id: req.retry_of_run_id } : {}),
        // 数据看板生成图表：必须显式透传，否则被这里的字段白名单静默丢弃，助手拿不到字段清单。
      }),
      signal
    })
  )
}

/** 续接进行中的 run：从头回放事件缓存（after=0）并跟随直到 end（切换界面后回到会话时使用）。 */
export function attachAgentRun(projectId: string, runId: string, handlers: AgentStreamHandlers): LlmStreamConnection {
  return openAgentRunStream(projectId, handlers, runId, (signal) => fetchAgentRunEvents(projectId, runId, '', signal))
}

/** 从 last_event_seq 续接轮次；动作返回 running 后也复用此入口。 */
export function attachAgentTurn(projectId: string, turnId: string, after: number, handlers: AgentStreamHandlers): LlmStreamConnection {
  return openAgentRunStream(projectId, handlers, '', (signal) => fetchAgentTurnEvents(projectId, turnId, String(after), signal), true)
}

function fetchAgentTurnEvents(projectId: string, turnId: string, after: string, signal: AbortSignal) {
  return fetch(`${baseURL}${projectPath(projectId, 'agent-turns', turnId, 'events')}?after=${encodeURIComponent(after)}`, {
    method: 'GET',
    headers: { Authorization: `Bearer ${getAccessToken() || getApiKey()}` },
    signal
  })
}

function fetchAgentRunEvents(projectId: string, runId: string, after: string, signal: AbortSignal) {
  return fetch(
    `${baseURL}${projectPath(projectId, 'agent-runs', runId, 'events')}?after=${encodeURIComponent(after)}`,
    {
      method: 'GET',
      headers: { Authorization: `Bearer ${getAccessToken() || getApiKey()}` },
      signal
    }
  )
}

function openAgentRunStream(
  projectId: string,
  handlers: AgentStreamHandlers,
  initialRunId: string,
  open: (signal: AbortSignal) => Promise<Response>,
  turnMode = false
): LlmStreamConnection {
  const controller = new AbortController()
  let ended = false
  let failed = false
  let paused = false
  // settled：后台协程已得出终态（HTTP 错误 / error 帧 / 读流结束），fetch 响应体不再有在途读取。
  // 此后 close() 必须是空操作——再 abort 取消不了任何东西，只会让挂在 fetch 上的第三方
  // 钩子（克隆了响应体的浏览器扩展等）收到一个 AbortError 并以 error 级别打进控制台。
  let settled = false
  const settle = () => {
    settled = true
  }
  let lastId = ''
  let runId = initialRunId
  let turnId = ''
  const fireEnd = (reason?: string) => {
    if (!ended) {
      ended = true
      if (!failed) handlers.onEnd?.(reason)
    }
  }
  const onEvent = (obj: WireRecord) => {
    if (obj?._sse_id != null && String(obj._sse_id) !== '') {
      const nextId = String(obj._sse_id)
      if (/^\d+$/.test(nextId) && /^\d+$/.test(lastId) && Number(nextId) <= Number(lastId)) return
      lastId = nextId
    }
    if (turnMode && obj?.turn_id) turnId = String(obj.turn_id)
    // v3：轮次模式下每帧携带归属（哪个子 run / 哪个助手），供协作状态索引任务。
    if (obj?.run_id || obj?.agent_id) {
      handlers.onFrameContext?.({
        runId: obj?.run_id ? String(obj.run_id) : undefined,
        agentId: obj?.agent_id ? String(obj.agent_id) : undefined
      })
    }
    if (obj?.type === 'end') {
      fireEnd(typeof obj.reason === 'string' ? obj.reason : undefined)
      return
    }
    if (obj?.type === 'run' && obj.run_id) {
      runId = String(obj.run_id)
      handlers.onRun?.(runId, obj.agent_id ? String(obj.agent_id) : undefined)
      return
    }
    if (obj?.type === 'plan') {
      const decision = obj.decision && typeof obj.decision === 'object' ? obj.decision as WireRecord : {}
      handlers.onPlan?.({
        turnId: turnId || String(obj.turn_id || ''),
        decision: toAgentPlanDecision(decision),
        confirmRequired: obj.confirm_required === true,
        source: obj.source ? String(obj.source) : undefined
      })
      if (obj.confirm_required === true) paused = true
      return
    }
    if (obj?.type === 'plan_confirmation') {
      const state = String(obj.state || '')
      paused = state === 'required'
      handlers.onPlanConfirmation?.({
        turnId: turnId || String(obj.turn_id || ''), state,
        edited: obj.edited === true,
        version: obj.version ? Number(obj.version) : undefined
      })
      return
    }
    if (obj?.type === 'task') {
      handlers.onTask?.({
        taskId: obj.task_id ? String(obj.task_id) : undefined,
        agentId: obj.agent_id ? String(obj.agent_id) : undefined,
        targetKind: obj.target_kind === 'human' ? 'human' : 'agent',
        ref: obj.ref ? String(obj.ref) : undefined,
        dependsOn: Array.isArray(obj.depends_on) ? obj.depends_on.map((ref: unknown) => String(ref)) : undefined,
        result: obj.result ? String(obj.result) : undefined,
        index: Number(obj.index || 0),
        total: Number(obj.total || 0),
        status: isAgentTaskStatus(obj.status) ? obj.status : 'pending',
        message: obj.message ? String(obj.message) : undefined,
        runId: obj.run_id ? String(obj.run_id) : undefined
      })
      if (obj.run_id) handlers.onRun?.(String(obj.run_id), obj.agent_id ? String(obj.agent_id) : undefined)
      return
    }
    if (obj?.type === 'human') {
      handlers.onHuman?.({
        taskId: obj.task_id ? String(obj.task_id) : undefined,
        humanId: obj.human_id ? String(obj.human_id) : undefined,
        humanName: obj.human_name ? String(obj.human_name) : undefined,
        index: Number(obj.index || 0),
        message: obj.message ? String(obj.message) : undefined
      })
      return
    }
    if (obj?.type === 'turn') {
      const status = String(obj.status ?? '')
      if (turnMode) paused = status === 'waiting_human' || status === 'awaiting_plan_confirm'
      handlers.onTurn?.({
        turnId: turnId || String(obj.turn_id || ''),
        status,
        index: Number(obj.index || 0),
        total: Number(obj.total || 0)
      })
      return
    }
    if (obj?.type === 'error') {
      failed = true
      const error = new Error(String(obj.message || t('error.runFailed'))) as Error & { code?: string }
      error.code = typeof obj.code === 'string' ? obj.code : undefined
      // 先标记终态再通知：调用方惯例是在 onError 里 close()，此时不应再去 abort 在途读取，
      // 余下的收尾帧由 readSse 协作式跳过。
      settle()
      handlers.onError?.(error)
      return
    }
    if (obj?.type === 'thinking') {
      handlers.onThinking?.(Number(obj.elapsed_ms || 0), obj.content ? String(obj.content) : undefined)
      return
    }
    if (obj?.type === 'usage') {
      handlers.onUsage?.(toAgentRunMetrics(obj))
      return
    }
    if (obj?.type === 'token' || obj?.type === 'chunk') {
      const text = (obj.content || obj.delta || '') as string
      if (text) handlers.onToken?.(text)
      return
    }
    if (obj?.type === 'tool_call_delta') {
      handlers.onToolCallDelta?.(orElse(obj, 'name', ''), orElse(obj, 'arguments', ''), orUndefined(obj, 'call_id'))
      return
    }
    if (obj?.type === 'tool_start') {
      handlers.onToolStart?.(orElse(obj, 'name', ''), orUndefined(obj, 'call_id'))
      return
    }
    if (obj?.type === 'confirmation_required') {
      const argv = Array.isArray(obj.argv) ? obj.argv.map((part: unknown) => String(part)) : []
      handlers.onConfirmationRequired?.(String(obj.call_id || ''), argv, String(obj.message || ''))
      return
    }
    if (obj?.type === 'confirmation_resolved') {
      handlers.onConfirmationResolved?.(String(obj.call_id || ''), obj.approve === true)
      return
    }
    if (obj?.type === 'tool_call') {
      handlers.onToolCall?.(orElse(obj, 'name', ''), orElse(obj, 'arguments', ''), orUndefined(obj, 'call_id'))
      return
    }
    if (obj?.type === 'tool_progress') {
      handlers.onToolProgress?.(orElse(obj, 'name', ''), Number(obj.elapsed_ms || 0), orUndefined(obj, 'call_id'))
      return
    }
    if (obj?.type === 'tool_result') {
      handlers.onToolResult?.(orElse(obj, 'name', ''), orElse(obj, 'content', ''), orUndefined(obj, 'call_id'), Number(obj.duration_ms || 0), obj.is_error === true, obj.truncated === true)
    }
  }
  ;(async () => {
    try {
      const resp = await open(controller.signal)
      const ct = resp.headers.get('content-type') || ''
      if (ct.includes('text/html')) {
        throw new Error(t('error.htmlResponse'))
      }
      if (!resp.ok || !resp.body) {
        const data = await resp.json().catch(() => null)
        const detail = (data as { error?: { code?: string; message?: string }; message?: string } | null)
        const error = new Error(detail?.error?.message || detail?.message || t('error.requestFailedWithStatus', { status: resp.status })) as Error & { code?: string }
        error.code = detail?.error?.code
        throw error
      }
      // runID 此前只由首个 SSE 帧下发，连接在 run 帧
      // 到达前断开时无法续传。响应头先给一份，后续 run 事件仍会覆盖同一值。
      const headerRunId = resp.headers.get('x-run-id')
      if (headerRunId) {
        runId = headerRunId
        handlers.onRun?.(runId)
      }
      const headerTurnId = resp.headers.get('x-turn-id')
      if (headerTurnId) turnId = headerTurnId
      await readSse(resp, onEvent, undefined, () => failed)
      if (paused && !ended && !failed) return
      if (!ended && !failed) throw new Error(t('error.streamFailed'))
    } catch (e) {
      if (controller.signal.aborted || failed) return
      let delay = 1000
      let err: unknown = e
      for (let attempt = 0; attempt < 3 && (turnMode ? turnId : runId) && !ended && !paused; attempt++) {
        try {
          await new Promise<void>((resolve, reject) => {
            const timer = setTimeout(resolve, delay)
            controller.signal.addEventListener('abort', () => {
              clearTimeout(timer)
              reject(e)
            }, { once: true })
          })
          if (controller.signal.aborted) return
          const resp = turnMode
            ? await fetchAgentTurnEvents(projectId, turnId, lastId, controller.signal)
            : await fetchAgentRunEvents(projectId, runId, lastId, controller.signal)
          if (!resp.ok || !resp.body) throw new Error(t('error.requestFailedWithStatus', { status: resp.status }))
          await readSse(resp, onEvent, undefined, () => failed)
          if (paused && !ended && !failed) return
          if (!ended && !failed) throw new Error(t('error.streamFailed'))
          return
        } catch (next) {
          if (controller.signal.aborted) return
          err = next
          delay *= 2
        }
      }
      if (!controller.signal.aborted && !failed) {
        // 协程到此为止，不会再发起请求：先标记终态，调用方在 onError 里 close() 不会去
        // abort 一个响应体已经读完的 fetch。
        settle()
        handlers.onError?.(err)
      }
    } finally {
      settle()
    }
  })()
  return {
    close: () => {
      if (!settled) controller.abort()
    }
  }
}
