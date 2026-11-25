import { afterEach, describe, expect, it, vi } from 'vitest'
import { streamAgentRun, streamAgentTurn } from '@/services/agent-stream'
import type { AgentStreamHandlers, LlmStreamConnection } from '@/services/types'

function sseResponse(frames: Record<string, unknown>[]): Response {
  const raw = frames.map((frame, index) => `id: ${index + 1}\ndata: ${JSON.stringify(frame)}\n\n`).join('')
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(new TextEncoder().encode(raw))
      controller.close()
    }
  })
  return new Response(body, {
    status: 200,
    headers: { 'content-type': 'text/event-stream', 'x-turn-id': 'turn-1' }
  })
}

/** 不自动关闭的 SSE 响应：先推首批帧，后续帧由测试决定何时推入；记录读取端是否 cancel。 */
function heldResponse(frames: Record<string, unknown>[]): {
  response: Response
  push: (more: Record<string, unknown>[]) => void
  cancelled: () => boolean
} {
  const encoder = new TextEncoder()
  const encode = (list: Record<string, unknown>[]) =>
    list.map((frame, index) => `data: ${JSON.stringify(frame)}\n\n`).join('')
  let source!: ReadableStreamDefaultController<Uint8Array>
  let cancelled = false
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      source = controller
      controller.enqueue(encoder.encode(encode(frames)))
    },
    cancel() {
      cancelled = true
    }
  })
  return {
    response: new Response(body, {
      status: 200,
      headers: { 'content-type': 'text/event-stream', 'x-run-id': 'run-1' }
    }),
    push: (more) => {
      try {
        source.enqueue(encoder.encode(encode(more)))
      } catch {
        /* 流已被 cancel：正是期望的结果 */
      }
    },
    cancelled: () => cancelled
  }
}

describe('agent turn v3 stream', () => {
  const connections: LlmStreamConnection[] = []
  afterEach(() => {
    for (const connection of connections.splice(0)) connection.close()
    vi.unstubAllGlobals()
  })

  it('解析 plan 与 task.run_id', async () => {
    const onPlan = vi.fn()
    const onTask = vi.fn()
    const onRun = vi.fn()
    const handlers: AgentStreamHandlers = { onPlan, onTask, onRun, onError: vi.fn() }
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(sseResponse([
      { type: 'plan', turn_id: 'turn-1', confirm_required: false, decision: {
        action: 'create', goal: 'goal', complexity: 'simple',
        dispatches: [{ ref: 'd1', target_kind: 'agent', target_id: 'a1', message: 'work', depends_on: ['d0'] }]
      } },
      { type: 'task', turn_id: 'turn-1', task_id: 'turn-1-d1', run_id: 'run-1', agent_id: 'a1', status: 'running' },
      { type: 'end', turn_id: 'turn-1', reason: 'done' }
    ])))
    connections.push(streamAgentTurn('p1', 'thread-1', { content: 'go', mentions: [] }, handlers))
    await vi.waitFor(() => expect(onRun).toHaveBeenCalledWith('run-1', 'a1'))
    expect(onPlan.mock.calls[0][0].decision.dispatches[0]).toMatchObject({
      ref: 'd1', targetId: 'a1', dependsOn: ['d0']
    })
    expect(onTask.mock.calls[0][0]).toMatchObject({ taskId: 'turn-1-d1', runId: 'run-1' })
  })

  it('waiting_human 正常暂停，不报 streamFailed 或写终态', async () => {
    const handlers: AgentStreamHandlers = { onTurn: vi.fn(), onError: vi.fn(), onEnd: vi.fn() }
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(sseResponse([
      { type: 'turn', turn_id: 'turn-1', status: 'waiting_human' }
    ])))
    connections.push(streamAgentTurn('p1', 'thread-1', { content: 'help', mentions: [] }, handlers))
    await vi.waitFor(() => expect(handlers.onTurn).toHaveBeenCalledWith(expect.objectContaining({
      turnId: 'turn-1', status: 'waiting_human'
    })))
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(handlers.onError).not.toHaveBeenCalled()
    expect(handlers.onEnd).not.toHaveBeenCalled()
  })

  // 以下三例钉住 close() 的语义：只对「仍在途」的请求 abort。
  // 协程已得出终态后再 abort 取消不了任何东西，只会让页面里克隆了响应体的第三方
  // fetch 钩子（浏览器扩展的请求检查）收到 AbortError 并以 error 级别打进控制台。
  it('HTTP 非 2xx：onError 里 close() 不会回头 abort 已经读完的请求', async () => {
    let signal: AbortSignal | undefined
    vi.stubGlobal('fetch', vi.fn().mockImplementation((_url: string, init: RequestInit) => {
      signal = init.signal as AbortSignal
      return Promise.resolve(new Response(
        JSON.stringify({ error: { code: 'internal_error', message: 'boom' } }),
        { status: 500, headers: { 'content-type': 'application/json' } }
      ))
    }))
    let connection!: LlmStreamConnection
    const onError = vi.fn((_e: unknown) => connection.close())
    connection = streamAgentTurn('p1', 'thread-1', { content: 'go', mentions: [] }, { onError })
    connections.push(connection)
    await vi.waitFor(() => expect(onError).toHaveBeenCalledTimes(1))
    expect(onError.mock.calls[0][0]).toMatchObject({ message: 'boom', code: 'internal_error' })
    connection.close()
    expect(signal?.aborted).toBe(false)
  })

  it('收到 error 帧：onError 里 close() 不 abort，读取协作式停止，后续帧不再分发', async () => {
    let signal: AbortSignal | undefined
    const held = heldResponse([
      { type: 'run', run_id: 'run-1' },
      { type: 'error', code: 'llm_timeout', message: '模型响应超时' }
    ])
    vi.stubGlobal('fetch', vi.fn().mockImplementation((_url: string, init: RequestInit) => {
      signal = init.signal as AbortSignal
      return Promise.resolve(held.response)
    }))
    let connection!: LlmStreamConnection
    const onError = vi.fn((_e: unknown) => connection.close())
    const onToken = vi.fn()
    connection = streamAgentRun('p1', 'thread-1', { content: 'go', mentions: [] }, { onError, onToken })
    connections.push(connection)
    await vi.waitFor(() => expect(onError).toHaveBeenCalledTimes(1))
    expect(onError.mock.calls[0][0]).toMatchObject({ message: '模型响应超时', code: 'llm_timeout' })
    await vi.waitFor(() => expect(held.cancelled()).toBe(true))
    expect(signal?.aborted).toBe(false)
    // 服务端在 error 之后还会补 usage / end：流已停，这些帧不再到达调用方。
    held.push([{ type: 'token', content: 'late' }, { type: 'end', reason: 'error' }])
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(onToken).not.toHaveBeenCalled()
  })

  it('流仍在途：close() 照常 abort', async () => {
    let signal: AbortSignal | undefined
    const held = heldResponse([{ type: 'run', run_id: 'run-1' }, { type: 'token', content: 'a' }])
    vi.stubGlobal('fetch', vi.fn().mockImplementation((_url: string, init: RequestInit) => {
      signal = init.signal as AbortSignal
      return Promise.resolve(held.response)
    }))
    const onToken = vi.fn()
    const connection = streamAgentRun('p1', 'thread-1', { content: 'go', mentions: [] }, { onToken })
    connections.push(connection)
    await vi.waitFor(() => expect(onToken).toHaveBeenCalledWith('a'))
    expect(signal?.aborted).toBe(false)
    connection.close()
    expect(signal?.aborted).toBe(true)
  })
})
