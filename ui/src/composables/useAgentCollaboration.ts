// useAgentCollaboration：协作轮次的状态索引。
//
// 为什么单独一个 composable：
//   一轮会产生多个任务、多个子 run、可能还有人任务。旧的 useAgentConversation
//   只有单个 runId + 单个 confirmation，无法表达这种结构，硬塞进去会让两处状态
//   互相覆盖。这里把「轮次 / 任务 / 子 run / 审批」分层索引，页面只读派生状态。
//
// 关键点：
//   - 子 run 的 token/tool 事件按 runId 归到对应任务，不再拼成一整段正文；
//   - 审批按 `runId + callId` 索引，多个助手同时等确认时不会互相覆盖；
//   - 事件按 seq 去重（重连会重放），保证任务状态单调前进。
import { computed, reactive, ref } from 'vue'
import { api } from '@/services/api'
import { applyToolConfirmation, applyToolDelta, applyToolResult, applyToolStart } from '@/utils/tool-cards'
import type {
  AgentMentionTarget,
  AgentPlanDecision,
  AgentPlanEdit,
  AgentPlanEvent,
  AgentTask,
  AgentTaskEvent,
  AgentStreamHandlers,
  AgentTurn,
  AgentThreadKind,
  AgentTurnRequest,
  ChatMsg,
  LlmStreamConnection
} from '@/services/types'

/** 单个任务在界面上的运行态。 */
export interface TaskRuntime {
  task: AgentTask
  /** 该任务的流式正文（多个子 run 会追加到同一任务）。 */
  content: string
  thinking: string
  toolCalls: NonNullable<ChatMsg['toolCalls']>
  runId?: string
  error?: string
}

/** 待确认的工具调用（按 run + call 索引）。 */
export interface PendingConfirmation {
  runId: string
  callId: string
  argv: string[]
  message: string
}

export interface PendingPlan {
  turnId: string
  decision: AgentPlanDecision
}

export interface UseAgentCollaborationOptions {
  projectId: () => string
  /** 现有会话消息（由页面持有）：工具卡与正文仍写回这里。 */
  messages: () => ChatMsg[]
  /** 轮次结束回调（页面据此刷新会话列表）。 */
  onTurnEnd?: () => void
  onError?: (e: unknown) => void
}

export function useAgentCollaboration(opts: UseAgentCollaborationOptions) {
  /** 当前轮次快照（null 表示没有活动轮次）。 */
  const turn = ref<AgentTurn | null>(null)
  const pendingPlan = ref<PendingPlan | null>(null)
  const confirmingPlan = ref(false)
  /** 任务运行态（key = taskId）。 */
  const runtimes = reactive<Record<string, TaskRuntime>>({})
  /** 待确认（key = `${runId}:${callId}`）。 */
  const confirmations = reactive<Record<string, PendingConfirmation>>({})
  /** 本轮是否正在跑（含 Plan 决策期）。 */
  const running = ref(false)
  /** 已处理的最大事件序号（重连去重）。 */
  let lastSeq = 0
  /** 当前连接。 */
  let connection: LlmStreamConnection | null = null
  /** 已发送的幂等键：同一次发送重试时复用，避免重复建轮次。 */
  let pendingKey = ''
  /** 当前帧归属（由 onFrameContext 维护）：多助手串行时定位任务。 */
  let currentRunId = ''
  let currentAgentId = ''

  const tasks = computed(() => turn.value?.tasks ?? [])
  /** Plan 栅栏决策中（输入区状态条显示「Plan 决策中…」）。 */
  const planning = computed(() => turn.value?.status === 'planning' && running.value)
  const waitingHuman = computed(() => tasks.value.some((t) => t.status === 'waiting_human'))
  const confirmationList = computed(() => Object.values(confirmations))
  const progress = computed(() => {
    const list = tasks.value
    const done = list.filter((t) => t.status === 'completed').length
    return { done, total: list.length }
  })

  /** 任务对应的助手/人目标（页面用名称渲染身份）。 */
  function taskBySeq(seq: number): AgentTask | undefined {
    return tasks.value.find((t) => t.seq === seq)
  }

  function runtimeOf(taskId: string): TaskRuntime | undefined {
    return runtimes[taskId]
  }

  function ensureRuntime(task: AgentTask): TaskRuntime {
    if (!runtimes[task.id]) {
      runtimes[task.id] = { task, content: '', thinking: '', toolCalls: [] }
    }
    runtimes[task.id].task = task
    return runtimes[task.id]
  }

  /**
   * 建立或更新一条任务（帧驱动的唯一入口）。
   *
   * 轮次快照是异步的，帧可能先到；这里用帧里的最小字段建占位任务，
   * 后续 syncTasks 用服务端快照覆盖，内容（正文/工具卡）保留在 runtime 里。
   */
  function upsertTask(seed: Pick<AgentTask, 'id' | 'seq' | 'targetKind' | 'targetId' | 'instruction' | 'status'>): AgentTask {
    const existing = tasks.value.find((t) => t.id === seed.id)
    if (existing) {
      // 只补空字段，不覆盖已由快照填好的状态。
      if (!existing.targetId) existing.targetId = seed.targetId
      if (!existing.instruction) existing.instruction = seed.instruction
      ensureRuntime(existing)
      return existing
    }
    const task: AgentTask = {
      id: seed.id,
      turnId: turn.value?.id ?? '',
      seq: seed.seq,
      targetKind: seed.targetKind,
      targetId: seed.targetId,
      instruction: seed.instruction,
      status: seed.status,
      attempt: 0,
      dependsOn: [],
      version: 0,
      createdAt: new Date().toISOString()
    }
    ensureTurnContainer(task.turnId)
    turn.value!.tasks.push(task)
    ensureRuntime(task)
    return task
  }

  /** 快照未到时建立最小轮次容器（拿到真实快照后由 syncTasks 覆盖）。 */
  function ensureTurnContainer(turnId: string): void {
    if (turn.value) return
    turn.value = {
      id: turnId,
      threadId: '',
      status: 'running',
      version: 0,
      createdAt: '',
      updatedAt: '',
      tasks: [],
      lastEventSeq: 0
    }
  }

  /** 由 SSE 帧更新任务状态（不依赖快照刷新）。 */
  function applyTaskEvent(ev: AgentTaskEvent): void {
    const target = ev.taskId
      ? upsertTask({
          id: ev.taskId,
          seq: ev.index,
          targetKind: ev.targetKind ?? 'agent',
          targetId: ev.agentId ?? '',
          instruction: ev.message ?? '',
          status: ev.status
        })
      : tasks.value.find((t) => t.seq === ev.index)
    if (!target) return
    if (ev.agentId && !target.targetId) target.targetId = ev.agentId
    if (ev.ref) target.ref = ev.ref
    if (ev.dependsOn) target.dependsOn = ev.dependsOn
    if (ev.result) target.result = ev.result
    if (ev.runId) target.runId = ev.runId
    target.status = ev.status as AgentTask['status']
    const runtime = ensureRuntime(target)
    runtime.task = target
    if (ev.runId) runtime.runId = ev.runId
    // 气泡在任务出现时即按 taskId 建好（applyPlan / 这里），token 实时写入；
    // 任务结束不再另插一条「结果气泡」（修复 CB-FE-04 的重复气泡）。
    if (target.targetKind === 'agent' && ev.status !== 'pending') ensureBubble(target)
  }

  /**
   * 发起一轮（多目标 @）。
   *
   * 消息流结构：用户消息 → 每个任务一条 assistant 气泡（带身份）→ 汇总。
   * 气泡在任务首次出现时创建，后续 token / 工具事件都写进对应任务的气泡，
   * 不会把多个助手的输出拼成一段。
   */
  function start(threadId: string, content: string, mentions: AgentMentionTarget[], threadKind?: AgentThreadKind): void {
    const projectId = opts.projectId()
    if (!projectId || !threadId) return
    detach()
    running.value = true
    lastSeq = 0
    turn.value = null
    pendingPlan.value = null
    currentRunId = ''
    currentAgentId = ''
    clearRuntimes()
    const req: AgentTurnRequest = {
      content,
      mentions,
      idempotencyKey: pendingKey || crypto.randomUUID(),
      threadKind
    }
    pendingKey = req.idempotencyKey || ''
    const body = opts.messages()
    body.push({ role: 'user', content })
    connection = api.agentThreads.streamTurn(projectId, threadId, req, buildHandlers())
  }

  function applyPlan(ev: AgentPlanEvent): void {
    ensureTurnContainer(ev.turnId)
    const current = turn.value!
    if (!current.id) current.id = ev.turnId
    const previous = current.tasks
    current.plan = {
      version: current.plan?.version ?? 0,
      decision: ev.decision,
      confirmState: ev.confirmRequired ? 'required' : '',
      replans: current.plan?.replans ?? 0
    }
    if (ev.confirmRequired) {
      current.status = 'awaiting_plan_confirm'
      pendingPlan.value = { turnId: current.id, decision: ev.decision }
      running.value = false
    } else {
      pendingPlan.value = null
      if (current.status === 'awaiting_plan_confirm') current.status = 'running'
    }
    if (!ev.decision.dispatches.length && (ev.decision.action === 'update' || ev.decision.action === 'complete')) return
    current.tasks = ev.decision.dispatches.map((dispatch, index) => {
      const old = previous.find((task) => task.ref === dispatch.ref)
      if (old?.status === 'completed') return old
      const task: AgentTask = {
        id: `${current.id}-${dispatch.ref}`,
        turnId: current.id,
        seq: index + 1,
        ref: dispatch.ref,
        targetKind: dispatch.targetKind,
        targetId: dispatch.targetId,
        instruction: dispatch.message,
        status: 'pending',
        attempt: 0,
        dependsOn: dispatch.dependsOn,
        contextFrom: dispatch.contextFrom,
        stepId: dispatch.stepId,
        risk: dispatch.risk,
        expectedOutput: dispatch.expectedOutput,
        version: 0,
        createdAt: new Date().toISOString()
      }
      ensureRuntime(task)
      if (task.targetKind === 'agent') ensureBubble(task)
      return task
    })
  }

  /** buildHandlers 把 SSE 帧映射到「轮次 / 任务 / 子 run」三层状态。 */
  function buildHandlers(): AgentStreamHandlers {
    return {
      // 帧归属：多助手串行时靠它把 token / 工具 / 审批路由到正确任务。
      onFrameContext: (ctx) => {
        if (ctx.runId) currentRunId = ctx.runId
        if (ctx.agentId) currentAgentId = ctx.agentId
      },
      onPlan: (ev) => applyPlan(ev),
      onPlanConfirmation: (ev) => {
        if (ev.turnId) ensureTurnContainer(ev.turnId)
        if (ev.state === 'required') {
          running.value = false
          if (turn.value) {
            turn.value.status = 'awaiting_plan_confirm'
            if (turn.value.plan) {
              turn.value.plan.confirmState = 'required'
              pendingPlan.value = { turnId: turn.value.id, decision: turn.value.plan.decision }
            }
          }
        } else {
          pendingPlan.value = null
          if (turn.value?.plan) turn.value.plan.confirmState = ev.state
        }
      },
      onTask: (ev) => applyTaskEvent(ev),
      onHuman: (ev) => {
        const task = upsertTask({
          id: ev.taskId ?? `seq-${ev.index}`,
          seq: ev.index,
          targetKind: 'human',
          targetId: ev.humanId ?? '',
          instruction: ev.message ?? '',
          status: 'waiting_human'
        })
        ensureRuntime(task).task = task
        running.value = false
      },
      onTurn: (ev) => {
        if (ev.turnId) ensureTurnContainer(ev.turnId)
        if (!turn.value) return
        turn.value.status = ev.status as AgentTurn['status']
        running.value = ev.status === 'running' || ev.status === 'planning'
        if (ev.status === 'awaiting_plan_confirm' && turn.value.plan) {
          pendingPlan.value = { turnId: turn.value.id, decision: turn.value.plan.decision }
        }
      },
      onThinking: (_ms, text) => {
        const rt = activeRuntime()
        if (rt && text) rt.thinking += text
      },
      onToken: (text) => {
        const rt = activeRuntime()
        if (!rt) return
        rt.content += text
        const bubble = bubbleOf(rt.task)
        if (bubble) bubble.content = rt.content
      },
      onToolCallDelta: (name, argsDelta, callId) => {
        const rt = activeRuntime()
        if (!rt) return
        rt.toolCalls = applyToolDelta(rt.toolCalls, name, argsDelta, callId)
        syncToolCards(rt)
      },
      onToolStart: (name, callId) => {
        const rt = activeRuntime()
        if (!rt) return
        rt.toolCalls = applyToolStart(rt.toolCalls, name, callId)
        syncToolCards(rt)
      },
      onToolResult: (name, content, callId, durationMs, isError, truncated) => {
        const rt = activeRuntime()
        if (!rt) return
        rt.toolCalls = applyToolResult(rt.toolCalls, name, { content, durationMs, isError, truncated }, callId)
        syncToolCards(rt)
      },
      onConfirmationRequired: (callId, argv, message) => {
        // 审批按 run + call 索引：多个助手同时等确认时互不覆盖。
        const rt = activeRuntime()
        const runId = rt?.runId || ''
        confirmations[`${runId}:${callId}`] = { runId, callId, argv, message }
        if (rt) {
          const next = applyToolConfirmation(rt.toolCalls, callId, argv)
          if (next !== rt.toolCalls) {
            rt.toolCalls = next
            syncToolCards(rt)
          }
        }
      },
      onConfirmationResolved: (callId) => {
        for (const key of Object.keys(confirmations)) {
          if (confirmations[key].callId === callId) delete confirmations[key]
        }
      },
      onRun: (runId) => {
        const rt = activeRuntime()
        if (rt) rt.runId = runId
      },
      onEnd: () => {
        running.value = false
        pendingKey = ''
        // 结束后拉一次快照：以服务端任务状态为准（含 failed/blocked）。
        const id = turn.value?.id
        if (id) void refresh(id)
        opts.onTurnEnd?.()
      },
      onError: (e) => {
        detach()
        running.value = false
        pendingKey = ''
        opts.onError?.(e)
      }
    }
  }

  /**
   * activeRuntime 返回当前帧所属任务的运行态。
   *
   * 优先用帧上下文（agentId / runId）精确匹配；都没有时才回退到
   * 「最后一个 running 任务」，保证单助手/无上下文的旧帧仍能工作。
   */
  function activeRuntime(): TaskRuntime | undefined {
    if (currentRunId) {
      const byRun = Object.values(runtimes).find((rt) => rt.runId === currentRunId)
      if (byRun) return byRun
    }
    if (currentAgentId) {
      const list = tasks.value.filter((t) => t.targetId === currentAgentId && t.targetKind === 'agent')
      const candidate = list[list.length - 1]
      if (candidate) return ensureRuntime(candidate)
    }
    const runningTasks = tasks.value.filter((t) => t.status === 'running')
    const last = runningTasks[runningTasks.length - 1]
    if (last) return ensureRuntime(last)
    const all = Object.values(runtimes)
    return all[all.length - 1]
  }

  /**
   * bubbleOf 找到任务对应的会话气泡：按 taskId 绑定（修复 CB-FE-03）。
   * 不能按 agentId 找：同一助手在历史里回答过、或同一轮有两个任务指向同一助手时，
   * 按 agentId 会把新 token 写进旧气泡。
   */
  function bubbleOf(task: AgentTask): ChatMsg | undefined {
    return opts.messages().find((m) => m.role === 'assistant' && m.taskId === task.id)
  }

  /** ensureBubble 为任务补一条 assistant 气泡（没有就建）；已流式到达的正文 / 工具卡一并带上。 */
  function ensureBubble(task: AgentTask): ChatMsg {
    const found = bubbleOf(task)
    if (found) return found
    const rt = runtimes[task.id]
    const bubble: ChatMsg = {
      role: 'assistant',
      content: rt?.content ?? '',
      toolCalls: rt?.toolCalls.length ? [...rt.toolCalls] : undefined,
      agentId: task.targetId,
      taskId: task.id,
      turnId: task.turnId
    }
    opts.messages().push(bubble)
    return bubble
  }

  /** syncToolCards 把任务内的工具卡同步到气泡（触发重渲染）。 */
  function syncToolCards(rt: TaskRuntime): void {
    const bubble = bubbleOf(rt.task)
    if (bubble) bubble.toolCalls = [...rt.toolCalls]
  }

  function clearRuntimes(): void {
    for (const key of Object.keys(runtimes)) delete runtimes[key]
    for (const key of Object.keys(confirmations)) delete confirmations[key]
  }

  /** 清理连接（切换会话/页面卸载）。只断开 SSE，不取消后端轮次。 */
  function detach(): void {
    connection?.close()
    connection = null
  }

  /** 从最新已消费序号续接后台轮次。 */
  function attach(turnId: string, after = lastSeq): void {
    const projectId = opts.projectId()
    if (!projectId || !turnId) return
    detach()
    running.value = true
    connection = api.agentThreads.attachTurn(projectId, turnId, after, buildHandlers())
  }

  /** 批准 / 编辑后批准 / 拒绝整轮 Plan；与单次工具确认完全分离。 */
  async function confirmPlan(approve: boolean, edits: AgentPlanEdit[] = []): Promise<void> {
    const current = turn.value
    const projectId = opts.projectId()
    if (!current || !projectId || confirmingPlan.value) return
    confirmingPlan.value = true
    detach()
    try {
      const fresh = await api.agentTurns.confirmPlan(projectId, current.id, {
        approve, edits, expectedVersion: current.version
      })
      turn.value = fresh
      syncTasks(fresh.tasks)
      pendingPlan.value = fresh.status === 'awaiting_plan_confirm' && fresh.plan
        ? { turnId: fresh.id, decision: fresh.plan.decision }
        : null
      if (fresh.status === 'running') attach(fresh.id, fresh.lastEventSeq)
    } catch (e) {
      opts.onError?.(e)
    } finally {
      confirmingPlan.value = false
    }
  }

  /** 停止整轮：断开连接并请求服务端取消。 */
  async function cancel(): Promise<void> {
    detach()
    running.value = false
    const t = turn.value
    const projectId = opts.projectId()
    if (!t || !projectId) return
    try {
      turn.value = await api.agentTurns.act(projectId, t.id, { action: 'cancel' })
    } catch (e) {
      opts.onError?.(e)
    }
  }

  function taskVersion(taskId: string): number | undefined {
    return tasks.value.find((task) => task.id === taskId)?.version
  }

  /** 重试单个失败任务。 */
  async function retryTask(taskId: string): Promise<void> {
    const t = turn.value
    const projectId = opts.projectId()
    if (!t || !projectId) return
    detach()
    try {
      turn.value = await api.agentTurns.act(projectId, t.id, { action: 'retry_task', taskId, expectedVersion: taskVersion(taskId) })
      syncTasks(turn.value.tasks)
      running.value = true
      attach(t.id, turn.value.lastEventSeq)
      opts.onTurnEnd?.()
    } catch (e) {
      opts.onError?.(e)
    }
  }

  /** 人工回复（继续 / 无法处理）。 */
  async function replyHuman(taskId: string, outcome: 'continue' | 'unable', text?: string): Promise<void> {
    const projectId = opts.projectId()
    if (!projectId) return
    const task = tasks.value.find((t) => t.id === taskId)
    detach()
    try {
      turn.value = await api.agentTurns.reply(projectId, taskId, {
        outcome,
        text,
        expectedVersion: task?.version
      })
      syncTasks(turn.value.tasks)
      running.value = turn.value.status === 'running' || turn.value.status === 'planning'
      if (running.value) attach(turn.value.id, turn.value.lastEventSeq)
      opts.onTurnEnd?.()
    } catch (e) {
      opts.onError?.(e)
    }
  }

  /** 刷新轮次快照（重连/回前台）。 */
  async function refresh(turnId: string): Promise<AgentTurn | null> {
    const projectId = opts.projectId()
    if (!projectId) return null
    try {
      const fresh = await api.agentTurns.get(projectId, turnId)
      turn.value = fresh
      syncTasks(fresh.tasks)
      pendingPlan.value = fresh.status === 'awaiting_plan_confirm' && fresh.plan
        ? { turnId: fresh.id, decision: fresh.plan.decision }
        : null
      running.value = fresh.status === 'running' || fresh.status === 'planning'
      if (running.value) attach(fresh.id, fresh.lastEventSeq)
      return fresh
    } catch (e) {
      opts.onError?.(e)
      return null
    }
  }

  /**
   * 用快照覆盖本地任务（以服务端为准，保留已流式正文）。
   *
   * 轮次快照可能还没拉到（/turns 是异步的），但任务帧已经先到：
   * 这种情况下按任务建一个最小轮次容器，保证 tasks / progress 派生立即可用。
   */
  function syncTasks(list: AgentTask[]): void {
    const current = turn.value
    if (!current) {
      turn.value = {
        id: list[0]?.turnId || '',
        threadId: '',
        status: 'running',
        version: 0,
        createdAt: '',
        updatedAt: '',
        tasks: list,
        lastEventSeq: 0
      }
    } else {
      current.tasks = list
    }
    for (const task of list) {
      ensureRuntime(task)
      // 切回会话 / 重连后，仍在执行的任务没有服务端消息（run 未结束），补回它的实时气泡。
      if (task.targetKind === 'agent' && task.status !== 'pending' && task.status !== 'completed') ensureBubble(task)
    }
  }

  /** 事件序号去重：返回 true 表示该帧应当被处理。 */
  function acceptSeq(seq: number | undefined): boolean {
    if (!seq) return true
    if (seq <= lastSeq) return false
    lastSeq = seq
    return true
  }

  return {
    turn,
    pendingPlan,
    confirmingPlan,
    runtimes,
    confirmations,
    running,
    planning,
    tasks,
    waitingHuman,
    confirmationList,
    progress,
    start,
    detach,
    attach,
    cancel,
    confirmPlan,
    retryTask,
    replyHuman,
    refresh,
    syncTasks,
    applyTaskEvent,
    runtimeOf,
    taskBySeq,
    acceptSeq,
    ensureRuntime
  }
}
