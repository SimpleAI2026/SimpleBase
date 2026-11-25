// 协作轮次的客户端状态索引。
// 覆盖：多目标任务分别落气泡、按 run+call 索引审批、人工帧、取消/重试/回复。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { useAgentCollaboration } from '@/composables/useAgentCollaboration'
import type { AgentPlanDecision, AgentStreamHandlers, AgentTask, AgentTurn, ChatMsg } from '@/services/types'

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

function planDecision(): AgentPlanDecision {
  return {
    action: 'create', goal: '核对订单', complexity: 'simple', steps: [],
    dispatches: [{
      ref: 'd1', targetKind: 'agent', targetId: 'a1', message: '查订单',
      dependsOn: [], contextFrom: [], stepId: '', risk: 'destructive', expectedOutput: ''
    }],
    confirm: { required: true, reasons: ['destructive'] },
    progress: { percent: 0, note: '' }, findings: [], unknowns: [],
    shouldContinue: false, shouldSummarize: false, sources: []
  }
}

/** 两个助手各一个任务的决策（a1 / a2）。 */
function planDecisionTwo(): AgentPlanDecision {
  const one = planDecision()
  return {
    ...one,
    confirm: { required: false, reasons: [] },
    dispatches: [
      { ...one.dispatches[0], ref: 'd1', targetId: 'a1', message: '查订单', risk: 'read' },
      { ...one.dispatches[0], ref: 'd2', targetId: 'a2', message: '核对文件', risk: 'read' }
    ]
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

  it('Plan 帧生成任务并把 token 落到各自气泡；气泡按 taskId 绑定，不覆盖同助手的历史气泡', () => {
    // 回归 CB-FE-03：历史里 a1 回答过 → 新 token 不能写进旧气泡。
    const history: ChatMsg = { id: 'old', role: 'assistant', content: '旧回答', agentId: 'a1' }
    messages.push(history)
    const collab = setup()
    collab.start('th-1', '核对', [])
    const h = streamHandlers()
    h.onPlan?.({ turnId: 'turn-1', decision: planDecisionTwo(), confirmRequired: false })
    expect(collab.tasks.value.map((t) => t.id)).toEqual(['turn-1-d1', 'turn-1-d2'])
    // Plan 帧为每个助手任务建了独立气泡（带 taskId / turnId）。
    const bubbleOf = (taskId: string) => messages.find((m) => m.taskId === taskId)
    expect(bubbleOf('turn-1-d1')).toMatchObject({ role: 'assistant', agentId: 'a1', turnId: 'turn-1' })
    expect(bubbleOf('turn-1-d2')).toMatchObject({ role: 'assistant', agentId: 'a2', turnId: 'turn-1' })
    // 任务 1 收流：帧上下文来自 run 事件。
    h.onFrameContext?.({ runId: 'r1', agentId: 'a1' })
    h.onTask?.({ taskId: 'turn-1-d1', agentId: 'a1', index: 1, total: 2, status: 'running' })
    h.onToken?.('助手一')
    expect(bubbleOf('turn-1-d1')?.content).toBe('助手一')
    expect(history.content).toBe('旧回答')
    // 任务 2 收流：新气泡，不覆盖任务 1。
    h.onFrameContext?.({ runId: 'r2', agentId: 'a2' })
    h.onTask?.({ taskId: 'turn-1-d2', agentId: 'a2', index: 2, total: 2, status: 'running' })
    h.onToken?.('助手二')
    expect(bubbleOf('turn-1-d2')?.content).toBe('助手二')
    expect(bubbleOf('turn-1-d1')?.content).toBe('助手一')
    // 回归 CB-FE-04：任务完成不再另插一条「结果气泡」。
    h.onTask?.({ taskId: 'turn-1-d1', agentId: 'a1', index: 1, total: 2, status: 'completed' })
    expect(collab.tasks.value.find((t) => t.id === 'turn-1-d1')?.status).toBe('completed')
    expect(messages.filter((m) => m.role === 'assistant')).toHaveLength(3)
  })

  it('同一助手在一轮里有两个任务：各写各的气泡', () => {
    const collab = setup()
    collab.start('th-1', '核对', [])
    const h = streamHandlers()
    const decision = planDecisionTwo()
    decision.dispatches[1].targetId = 'a1'
    h.onPlan?.({ turnId: 'turn-1', decision, confirmRequired: false })
    // 同一助手两个任务只能靠 run_id 区分：真实帧里每个子 run 都带自己的 run_id。
    h.onFrameContext?.({ runId: 'r1', agentId: 'a1' })
    h.onTask?.({ taskId: 'turn-1-d1', agentId: 'a1', runId: 'r1', index: 1, total: 2, status: 'running' })
    h.onToken?.('第一段')
    h.onTask?.({ taskId: 'turn-1-d1', agentId: 'a1', index: 1, total: 2, status: 'completed' })
    // 第二个子 run 有自己的 run_id：帧上下文随之切换（与真实 SSE 一致）。
    h.onFrameContext?.({ runId: 'r2', agentId: 'a1' })
    h.onTask?.({ taskId: 'turn-1-d2', agentId: 'a1', runId: 'r2', index: 2, total: 2, status: 'running' })
    h.onToken?.('第二段')
    const ofA1 = messages.filter((m) => m.agentId === 'a1')
    expect(ofA1.map((m) => m.content)).toEqual(['第一段', '第二段'])
  })

  it('快照恢复：仍在执行的任务补回气泡，已完成任务不补（服务端已有它的消息）', () => {
    const collab = setup()
    collab.syncTasks([task('k-run', 1, 'a1', 'running'), task('k-done', 2, 'a2', 'completed')])
    expect(messages.filter((m) => m.taskId === 'k-run')).toHaveLength(1)
    expect(messages.filter((m) => m.taskId === 'k-done')).toHaveLength(0)
  })

  it('Plan 决策中：turn 帧 planning 置 planning，run 帧后清除', () => {
    const collab = setup()
    collab.start('th-1', '核对', [])
    const h = streamHandlers()
    h.onTurn?.({ turnId: 'turn-1', status: 'planning', index: 0, total: 0 })
    expect(collab.planning.value).toBe(true)
    h.onPlan?.({ turnId: 'turn-1', decision: planDecisionTwo(), confirmRequired: false })
    h.onTurn?.({ turnId: 'turn-1', status: 'running', index: 0, total: 0 })
    expect(collab.planning.value).toBe(false)
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
    expect(api.agentTurns.act).toHaveBeenCalledWith('p1', 'turn-1', { action: 'retry_task', taskId: 'k1', expectedVersion: 1 })
  })

  it('Plan 帧生成结构化任务并进入整轮确认', () => {
    const collab = setup()
    collab.start('th-1', '核对', [])
    streamHandlers().onPlan?.({ turnId: 'turn-1', decision: planDecision(), confirmRequired: true })
    expect(collab.tasks.value.map((item) => item.id)).toEqual(['turn-1-d1'])
    expect(collab.tasks.value[0]).toMatchObject({ ref: 'd1', risk: 'destructive', instruction: '查订单' })
    expect(collab.pendingPlan.value?.decision.goal).toBe('核对订单')
    expect(collab.turn.value?.status).toBe('awaiting_plan_confirm')
    expect(collab.running.value).toBe(false)
  })

  it('批准 Plan 携带版本并从最新序号续接', async () => {
    const fresh = turnOf([task('turn-1-d1', 1, 'a1', 'pending')], 'running')
    fresh.lastEventSeq = 7
    api.agentTurns.confirmPlan.mockResolvedValue(fresh)
    const collab = setup()
    collab.start('th-1', '核对', [])
    streamHandlers().onPlan?.({ turnId: 'turn-1', decision: planDecision(), confirmRequired: true })
    if (collab.turn.value) collab.turn.value.version = 3
    await collab.confirmPlan(true, [{ ref: 'd1', message: '改后任务' }])
    expect(api.agentTurns.confirmPlan).toHaveBeenCalledWith('p1', 'turn-1', {
      approve: true, edits: [{ ref: 'd1', message: '改后任务' }], expectedVersion: 3
    })
    expect(api.agentThreads.attachTurn).toHaveBeenCalledWith('p1', 'turn-1', 7, expect.any(Object))
    expect(collab.pendingPlan.value).toBeNull()
  })

  it('快照刷新恢复待确认 Plan', async () => {
    const fresh = turnOf([], 'awaiting_plan_confirm')
    fresh.plan = { version: 3, decision: planDecision(), confirmState: 'required', replans: 0 }
    api.agentTurns.get.mockResolvedValue(fresh)
    const collab = setup()
    await collab.refresh('turn-1')
    expect(collab.pendingPlan.value?.turnId).toBe('turn-1')
    expect(collab.pendingPlan.value?.decision.goal).toBe('核对订单')
    expect(collab.running.value).toBe(false)
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

  describe('工具卡合并（与单助手会话共用 utils/tool-cards）', () => {
    /** 启动一轮并让 a1 的任务进入 running，返回 handlers 与「当前任务的工具卡」读取器。 */
    function runningTask() {
      const collab = setup()
      collab.start('th-1', 'x', [])
      const h = streamHandlers()
      h.onPlan?.({ turnId: 'turn-1', decision: planDecision(), confirmRequired: false })
      h.onFrameContext?.({ runId: 'r1', agentId: 'a1' })
      h.onTask?.({ taskId: 'turn-1-d1', agentId: 'a1', index: 1, total: 1, status: 'running' })
      const cards = () => messages.find((m) => m.taskId === 'turn-1-d1')?.toolCalls ?? []
      return { collab, h, cards }
    }

    it('参数增量只在「生成中」追加：卡片进入 running 后到达的迟到 delta 被忽略', () => {
      const { h, cards } = runningTask()
      h.onToolCallDelta?.('sql', '{"a"', 'c1')
      h.onToolCallDelta?.('sql', ':1}', 'c1')
      expect(cards()).toEqual([{ call_id: 'c1', name: 'sql', arguments: '{"a":1}', status: 'generating' }])
      h.onToolStart?.('sql', 'c1')
      h.onToolCallDelta?.('sql', 'LATE', 'c1')
      expect(cards()[0]).toMatchObject({ arguments: '{"a":1}', status: 'running' })
    })

    it('onToolStart 无 call_id：落到同名卡而不是再新增一张（修复重复卡）', () => {
      const { h, cards } = runningTask()
      h.onToolCallDelta?.('list_databases', '{}', 'c1')
      h.onToolStart?.('list_databases')
      expect(cards()).toHaveLength(1)
      expect(cards()[0]).toMatchObject({ call_id: 'c1', status: 'running' })
    })

    it('乱序结果按 call_id 各自落卡；无 call_id 的结果落到最后一张同名且尚无结果的卡', () => {
      const { h, cards } = runningTask()
      h.onToolStart?.('q', 'c1')
      h.onToolStart?.('q', 'c2')
      h.onToolResult?.('q', 'second', 'c2', 20)
      h.onToolResult?.('q', 'first', 'c1', 10)
      expect(cards().map((c) => c.content)).toEqual(['first', 'second'])

      h.onToolStart?.('q', 'c3')
      h.onToolStart?.('q', 'c4')
      h.onToolResult?.('q', 'no-id', undefined, 1)
      expect(cards().map((c) => c.content)).toEqual(['first', 'second', undefined, 'no-id'])
    })

    it('确认请求把对应卡置为 awaiting_confirm 并带上 argv；未知 call_id 不新增卡', () => {
      const { h, cards } = runningTask()
      h.onToolStart?.('sandbox_exec', 'c1')
      h.onConfirmationRequired?.('c1', ['rm', '-rf'], '删除？')
      expect(cards()[0]).toMatchObject({ status: 'awaiting_confirm', argv: ['rm', '-rf'] })
      h.onConfirmationRequired?.('ghost', ['x'], '?')
      expect(cards()).toHaveLength(1)
    })
  })
})
