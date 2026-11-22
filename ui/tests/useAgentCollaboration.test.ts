// planv6.7 v2 §6：协作轮次的客户端状态索引。
// 覆盖：多目标任务分别落气泡、按 run+call 索引审批、人工帧、取消/重试/回复。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { useAgentCollaboration } from '@/composables/useAgentCollaboration'
import type { AgentStreamHandlers, AgentTask, AgentTurn, ChatMsg } from '@/services/types'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

function task(id: string, seq: number, targetId: string, status: AgentTask['status'] = 'pending'): AgentTask {
  return {
    id,
    turnId: 'turn-1',
    seq,
    targetKind: 'agent',
    targetId,
    instruction: `step ${seq}`,
    status,
    attempt: 0,
    dependsOn: [],
    version: 1,
    createdAt: 't'
  }
}

function turnOf(tasks: AgentTask[], status: AgentTurn['status'] = 'running'): AgentTurn {
  return {
    id: 'turn-1',
    threadId: 'th-1',
    status,
    version: 1,
    createdAt: 't',
    updatedAt: 't',
    tasks,
    lastEventSeq: 0
  }
}

/** 取出 mock 的流式 handlers（发起后由 composable 传入）。 */
function streamHandlers(call = 0): AgentStreamHandlers {
  const args = api.agentThreads.streamTurn.mock.calls[call]
  return args[3] as AgentStreamHandlers
}

describe('useAgentCollaboration', () => {
  let messages: ChatMsg[]

  beforeEach(() => {
    resetApiMocks()
    messages = []
  })

  function setup() {
    return useAgentCollaboration({
      projectId: () => 'p1',
      messages: () => messages
    })
  }

  it('发起一轮：先落用户消息，并把请求交给 /turns', () => {
    const collab = setup()
    collab.start('th-1', '核对订单', [{ kind: 'agent', id: 'a1' }])
    expect(messages[0]).toMatchObject({ role: 'user', content: '核对订单' })
    expect(api.agentThreads.streamTurn).toHaveBeenCalledWith(
      'p1',
      'th-1',
      expect.objectContaining({ content: '核对订单', mentions: [{ kind: 'agent', id: 'a1' }] }),
      expect.any(Object)
    )
    expect(collab.running.value).toBe(true)
  })

  it('幂等键复用：同一次发送重试不换 key', () => {
    const collab = setup()
    collab.start('th-1', 'a', [])
    const first = api.agentThreads.streamTurn.mock.calls[0][2].idempotencyKey
    collab.start('th-1', 'a', [])
    const second = api.agentThreads.streamTurn.mock.calls[1][2].idempotencyKey
    expect(second).toBe(first)
  })

  it('路由帧自带 task_id：不拉快照也能建任务并把 token 落到对应气泡', () => {
    // 回归：帧里缺 task_id 时前端建不出任务，token 会被丢弃 → 气泡为空。
    const collab = setup()
    collab.start('th-1', '核对', [])
    const h = streamHandlers()
    h.onRoute?.({
      status: 'done',
      steps: [
        { kind: 'agent', agentId: 'a1', instruction: 's1', taskId: 'k1' },
        { kind: 'agent', agentId: 'a2', instruction: 's2', taskId: 'k2' }
      ]
    })
    expect(collab.tasks.value.map((t) => t.id)).toEqual(['k1', 'k2'])
    // 任务 1 收流：帧上下文来自 run 事件。
    h.onFrameContext?.({ runId: 'r1', agentId: 'a1' })
    h.onTask?.({ taskId: 'k1', agentId: 'a1', index: 1, total: 2, status: 'running' })
    h.onToken?.('助手一')
    expect(messages.find((m) => m.agentId === 'a1')?.content).toBe('助手一')
    // 任务 2 收流：新气泡，不覆盖任务 1。
    h.onFrameContext?.({ runId: 'r2', agentId: 'a2' })
    h.onTask?.({ taskId: 'k2', agentId: 'a2', index: 2, total: 2, status: 'running' })
    h.onToken?.('助手二')
    expect(messages.find((m) => m.agentId === 'a2')?.content).toBe('助手二')
    expect(messages.find((m) => m.agentId === 'a1')?.content).toBe('助手一')
    // 任务完成后正文回落到消息（刷新前也能看到）。
    h.onTask?.({ taskId: 'k1', agentId: 'a1', index: 1, total: 2, status: 'completed' })
    expect(collab.tasks.value.find((t) => t.id === 'k1')?.status).toBe('completed')
  })

  it('审批按 run+call 索引，多助手不互相覆盖', () => {
    const collab = setup()
    collab.start('th-1', 'x', [])
    const h = streamHandlers()
    collab.syncTasks([task('k1', 1, 'a1', 'running'), task('k2', 2, 'a2', 'running')])
    collab.runtimes.k1 = { task: collab.tasks.value[0], content: '', thinking: '', toolCalls: [], runId: 'r1' }
    h.onFrameContext?.({ runId: 'r1', agentId: 'a1' })
    h.onConfirmationRequired?.('c1', ['rm', '-rf'], '删除')
    collab.runtimes.k2 = { task: collab.tasks.value[1], content: '', thinking: '', toolCalls: [], runId: 'r2' }
    h.onFrameContext?.({ runId: 'r2', agentId: 'a2' })
    h.onConfirmationRequired?.('c1', ['drop'], '删表')
    expect(Object.keys(collab.confirmations)).toHaveLength(2)
    expect(collab.confirmationList.value.map((c) => c.runId).sort()).toEqual(['r1', 'r2'])
    // 解决其中一个只移除它。
    h.onConfirmationResolved?.('c1', true)
    expect(Object.keys(collab.confirmations)).toHaveLength(0)
  })

  it('人工帧把任务标记为等人并停止运行', () => {
    const collab = setup()
    collab.start('th-1', 'x', [])
    const h = streamHandlers()
    h.onHuman?.({ taskId: 'h1', index: 1, message: '确认一下' })
    expect(collab.tasks.value[0].status).toBe('waiting_human')
    expect(collab.waitingHuman.value).toBe(true)
    expect(collab.running.value).toBe(false)
  })

  it('task 帧 completed 后把正文落回消息（只落一次）', () => {
    const collab = setup()
    collab.start('th-1', 'x', [])
    const t = task('k1', 1, 'a1', 'running')
    collab.syncTasks([t])
    const h = streamHandlers()
    collab.runtimes.k1 = { task: collab.tasks.value[0], content: '结果', thinking: '', toolCalls: [] }
    h.onTask?.({ taskId: 'k1', index: 1, total: 1, status: 'completed' })
    const bubbles = messages.filter((m) => m.role === 'assistant' && m.agentId === 'a1')
    expect(bubbles).toHaveLength(1)
    expect(bubbles[0].content).toBe('结果')
  })

  it('取消整轮调用 actions=cancel', async () => {
    api.agentTurns.act.mockResolvedValue(turnOf([], 'canceled'))
    const collab = setup()
    collab.start('th-1', 'x', [])
    await collab.refresh('turn-1')
    await collab.cancel()
    expect(api.agentTurns.act).toHaveBeenCalledWith('p1', 'turn-1', { action: 'cancel' })
    expect(collab.running.value).toBe(false)
  })

  it('重试单个任务与人工回复带上版本号', async () => {
    api.agentTurns.act.mockResolvedValue(turnOf([task('k1', 1, 'a1', 'failed')]))
    api.agentTurns.reply.mockResolvedValue(turnOf([task('h1', 1, 'u1', 'completed')]))
    const collab = setup()
    collab.start('th-1', 'x', [])
    collab.syncTasks([task('k1', 1, 'a1', 'failed'), { ...task('h1', 2, 'u1'), targetKind: 'human', version: 4 }])
    // 先回复人工任务（retry 会用返回快照覆盖本地轮次，需在其之前断言）。
    await collab.replyHuman('h1', 'continue', '好了')
    expect(api.agentTurns.reply).toHaveBeenCalledWith('p1', 'h1', {
      outcome: 'continue', text: '好了', expectedVersion: 4
    })
    collab.syncTasks([task('k1', 1, 'a1', 'failed')])
    await collab.retryTask('k1')
    expect(api.agentTurns.act).toHaveBeenCalledWith('p1', 'turn-1', { action: 'retry_task', taskId: 'k1' })
  })

  it('事件序号去重：重复连接不会重复应用', () => {
    const collab = setup()
    expect(collab.acceptSeq(3)).toBe(true)
    expect(collab.acceptSeq(3)).toBe(false)
    expect(collab.acceptSeq(2)).toBe(false)
    expect(collab.acceptSeq(4)).toBe(true)
    expect(collab.acceptSeq(undefined)).toBe(true)
  })

  it('进度派生：completed / total', () => {
    const collab = setup()
    collab.syncTasks([
      task('k1', 1, 'a1', 'completed'),
      task('k2', 2, 'a2', 'completed'),
      task('k3', 3, 'a3', 'running')
    ])
    expect(collab.progress.value).toEqual({ done: 2, total: 3 })
  })
})
