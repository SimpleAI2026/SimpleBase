// 云助手域转换：助手 / 会话 / 消息 / 标签 / 轮次 / 任务 / 审批 / MCP（后端 snake_case → UI camelCase）。

import type {
  AgentApproval,
  AgentApprovalStatus,
  AgentBuiltinTagKey,
  AgentDispatchMode,
  AgentHumanTask,
  AgentMessage,
  AgentPlanDecision,
  AgentPlanSnapshot,
  AgentRunMetrics,
  AgentTag,
  AgentTagInput,
  AgentTask,
  AgentTaskStatus,
  AgentThread,
  AgentThreadKind,
  AgentTurn,
  AgentUsageEventWire,
  AgentWorkspaceCapabilities,
  CloudAgent,
  McpBuiltinServer,
  McpServer,
  McpTestResult,
  McpToolPolicy
} from '@/services/types'
import { AGENT_TASK_STATUSES } from '@/services/types'
import { http } from '@/services/http'
import { listOf, orUndefined, type WireRecord } from '@/services/wire'

export function toCloudAgent(raw: WireRecord): CloudAgent {
  return {
    id: String(raw?.id ?? ''),
    name: String(raw?.name ?? ''),
    module: String(raw?.module ?? ''),
    description: String(raw?.description ?? ''),
    system_prompt: String(raw?.system_prompt ?? ''),
    tool_ids: listOf(raw?.tool_ids, String),
    model_override: orUndefined(raw, 'model_override'),
    builtin_key: orUndefined(raw, 'builtin_key'),
    team_enabled: !!raw?.team_enabled,
    created_at: String(raw?.created_at ?? ''),
    updated_at: String(raw?.updated_at ?? '')
  }
}

export function isAgentTaskStatus(value: unknown): value is AgentTaskStatus {
  return AGENT_TASK_STATUSES.some((status) => status === value)
}

/**
 * 存储里的 auto 在读时统一映射为 collaborative（群会话）。
 * 旧会话因此获得群路由与多助手能力，界面不再出现「一轮一助手」的旧语义。
 */
export function toAgentThread(raw: WireRecord): AgentThread {
  return {
    id: String(raw?.id ?? ''),
    title: String(raw?.title ?? ''),
    agent_id: raw?.agent_id ? String(raw.agent_id) : undefined,
    tag_id: raw?.tag_id ? String(raw.tag_id) : undefined,
    dispatch_mode: isDispatchMode(raw?.dispatch_mode) ? raw.dispatch_mode : undefined,
    thread_kind: isThreadKind(raw?.thread_kind) ? raw.thread_kind : undefined,
    created_at: String(raw?.created_at ?? ''),
    updated_at: String(raw?.updated_at ?? ''),
    last_message_preview: raw?.last_message_preview ? String(raw.last_message_preview) : undefined
  }
}

export function toAgentWorkspaceCapabilities(raw: WireRecord): AgentWorkspaceCapabilities {
  return {
    protocolVersion: Number(raw?.protocol_version || 0),
    collaboration: raw?.collaboration === true,
    maxTasks: Number(raw?.max_tasks || 0),
    maxDepth: Number(raw?.max_depth || 0),
    maxInstruction: Number(raw?.max_instruction || 0),
    confirmPolicies: listOf(raw?.confirm_policy, String),
    threadKinds: Array.isArray(raw?.thread_kinds) ? raw.thread_kinds.filter(isThreadKind) : []
  }
}

/** 调度模式守卫：群会话只有 collaborative。 */
function isDispatchMode(v: unknown): v is AgentDispatchMode {
  return v === 'collaborative'
}

/** 会话类型守卫。 */
function isThreadKind(v: unknown): v is AgentThreadKind {
  return v === 'chat' || v === 'alert' || v === 'statistics' || v === 'scheduled'
}

/** 内置群 key 守卫。 */
function isBuiltinTagKey(v: unknown): v is AgentBuiltinTagKey {
  return v === 'general' || v === 'alerts' || v === 'stats' || v === 'scheduled'
}

export function toAgentMessage(raw: WireRecord): AgentMessage {
  return {
    id: String(raw?.id ?? ''),
    role: String(raw?.role ?? ''),
    content: String(raw?.content ?? ''),
    agent_id: orUndefined(raw, 'agent_id'),
    mentions: listOf(raw?.mentions),
    tool_calls: listOf(raw?.tool_calls),
    run_id: orUndefined(raw, 'run_id'),
    run_status: orUndefined(raw, 'run_status'),
    error_code: orUndefined(raw, 'error_code'),
    created_at: String(raw?.created_at ?? '')
  }
}

/** 工作台标签（snake → camel，数组兜底 []）。 */
export function toAgentTag(raw: WireRecord): AgentTag {
  const prompts = Array.isArray(raw?.prompts)
    ? raw.prompts.map((p: WireRecord) => ({
        id: String(p?.id ?? ''),
        title: String(p?.title ?? ''),
        content: String(p?.content ?? '')
      }))
    : []
  const threadKinds = Array.isArray(raw?.thread_kinds)
    ? raw.thread_kinds.filter(isThreadKind)
    : []
  return {
    id: String(raw?.id ?? ''),
    builtinKey: isBuiltinTagKey(raw?.builtin_key) ? raw.builtin_key : undefined,
    name: String(raw?.name ?? ''),
    agentIds: listOf(raw?.agent_ids, String),
    prompts,
    unavailableAgentIds: listOf(raw?.unavailable_agent_ids, String),
    routingPrompt: String(raw?.routing_prompt ?? ''),
    fallbackAgentId: raw?.fallback_agent_id ? String(raw.fallback_agent_id) : undefined,
    humanPolicy: raw?.human_policy === 'disabled' ? 'disabled' : undefined,
    confirmPolicy: raw?.confirm_policy === 'always' || raw?.confirm_policy === 'never' ? raw.confirm_policy : 'auto',
    threadKinds,
    version: Number(raw?.version ?? 0),
    updatedAt: raw?.updated_at ? String(raw.updated_at) : undefined
  }
}

/** 标签输入（camel → snake）。 */
export function fromAgentTagInput(input: AgentTagInput): Record<string, unknown> {
  const body: Record<string, unknown> = {
    agent_ids: input.agentIds,
    prompts: input.prompts.map((p) => ({ id: p.id, title: p.title, content: p.content }))
  }
  if (input.name !== undefined) body.name = input.name
  if (input.expectedVersion !== undefined) body.expected_version = input.expectedVersion
  if (input.routingPrompt !== undefined) body.routing_prompt = input.routingPrompt
  if (input.fallbackAgentId !== undefined) body.fallback_agent_id = input.fallbackAgentId
  if (input.humanPolicy !== undefined) body.human_policy = input.humanPolicy
  if (input.confirmPolicy !== undefined) body.confirm_policy = input.confirmPolicy
  return body
}

export function toAgentApproval(raw: WireRecord): AgentApproval {
  return {
    id: String(raw?.id ?? ''),
    run_id: String(raw?.run_id ?? ''),
    call_id: String(raw?.call_id ?? ''),
    thread_id: orUndefined(raw, 'thread_id'),
    agent_id: orUndefined(raw, 'agent_id'),
    tool_name: orUndefined(raw, 'tool_name'),
    argv: listOf(raw?.argv, String),
    message: orUndefined(raw, 'message'),
    status: String(raw?.status ?? 'pending') as AgentApprovalStatus,
    initiator_id: orUndefined(raw, 'initiator_id'),
    resolved_by: orUndefined(raw, 'resolved_by'),
    created_at: String(raw?.created_at ?? ''),
    expires_at: orUndefined(raw, 'expires_at'),
    resolved_at: orUndefined(raw, 'resolved_at')
  }
}

export function toMcpServer(raw: WireRecord): McpServer {
  const policy = raw?.tools_policy
  return {
    id: String(raw?.id ?? ''),
    name: String(raw?.name ?? ''),
    transport: String(raw?.transport ?? 'http'),
    url: String(raw?.url ?? ''),
    timeout_s: Number(raw?.timeout_s || 60),
    enabled: Boolean(raw?.enabled),
    tools_policy: policy && typeof policy === 'object' ? (policy as Record<string, McpToolPolicy>) : {},
    header_names: listOf(raw?.header_names, String),
    has_credential: Boolean(raw?.has_credential),
    name_conflict: raw?.name_conflict ? true : undefined,
    created_at: String(raw?.created_at ?? ''),
    updated_at: String(raw?.updated_at ?? '')
  }
}

/** MCP 连通性测试结果：自定义服务与内置服务共用。 */
export function toMcpTestResult(raw: WireRecord): McpTestResult {
  return {
    ok: Boolean(raw?.ok),
    tools: listOf(raw?.tools, String),
    error: raw?.error ? String(raw.error) : undefined
  }
}

export function toMcpBuiltinServer(raw: WireRecord): McpBuiltinServer {
  const tools = listOf(raw?.tools)
  return {
    name: String(raw?.name ?? ''),
    enabled: Boolean(raw?.enabled),
    transport: String(raw?.transport ?? 'http'),
    url: String(raw?.url ?? ''),
    tools: tools.map((x: WireRecord) => ({
      name: String(x?.name ?? ''),
      read_only: Boolean(x?.read_only),
      destructive: Boolean(x?.destructive)
    }))
  }
}

/** usage 帧 → 运行指标。`||` 兜底仅覆盖 falsy；非数字字符串仍得 NaN（保持现状）。 */
export function toAgentRunMetrics(obj: AgentUsageEventWire): AgentRunMetrics {
  return {
    duration_ms: Number(obj.duration_ms || 0),
    prompt_tokens: Number(obj.prompt_tokens || 0),
    completion_tokens: Number(obj.completion_tokens || 0),
    reasoning_tokens: Number(obj.reasoning_tokens || 0),
    tool_calls: Number(obj.tool_calls || 0)
  }
}

export function toAgentPlanDecision(raw: WireRecord): AgentPlanDecision {
  const confirm = raw?.confirm && typeof raw.confirm === 'object' ? raw.confirm as WireRecord : {}
  const progress = raw?.progress && typeof raw.progress === 'object' ? raw.progress as WireRecord : {}
  return {
    action: String(raw?.action ?? ''),
    goal: String(raw?.goal ?? ''),
    complexity: String(raw?.complexity ?? ''),
    steps: listOf(raw?.steps, (step: WireRecord) => ({
      id: String(step?.id ?? ''), title: String(step?.title ?? ''),
      status: String(step?.status ?? ''), note: String(step?.note ?? '')
    })),
    dispatches: listOf(raw?.dispatches, (dispatch: WireRecord) => ({
      ref: String(dispatch?.ref ?? ''),
      targetKind: dispatch?.target_kind === 'human' ? 'human' : 'agent',
      targetId: String(dispatch?.target_id ?? ''),
      message: String(dispatch?.message ?? ''),
      dependsOn: listOf(dispatch?.depends_on, String),
      contextFrom: listOf(dispatch?.context_from, String),
      stepId: String(dispatch?.step_id ?? ''),
      risk: String(dispatch?.risk ?? ''),
      expectedOutput: String(dispatch?.expected_output ?? '')
    })),
    confirm: { required: Boolean(confirm.required), reasons: listOf(confirm.reasons, String) },
    progress: { percent: Number(progress.percent || 0), note: String(progress.note || '') },
    findings: listOf(raw?.findings, String),
    unknowns: listOf(raw?.unknowns, String),
    shouldContinue: raw?.should_continue === true,
    shouldSummarize: raw?.should_summarize === true,
    sources: listOf(raw?.sources, (source: WireRecord) => ({
      kind: String(source?.kind ?? ''), ref: String(source?.ref ?? ''), usedFor: String(source?.used_for ?? '')
    }))
  }
}

export function toAgentPlanSnapshot(raw: WireRecord): AgentPlanSnapshot | undefined {
  if (!raw || typeof raw !== 'object') return undefined
  const decision = raw.decision && typeof raw.decision === 'object' ? raw.decision as WireRecord : {}
  return {
    version: Number(raw.version ?? 0),
    decision: toAgentPlanDecision(decision),
    confirmState: String(raw.confirm_state ?? ''),
    replans: Number(raw.replans ?? 0)
  }
}

/** 轮次：snake → camel；任务数组兜底 []。 */
export function toAgentTurn(raw: WireRecord): AgentTurn {
  const tasks = listOf(raw?.tasks, toAgentTask)
  return {
    id: String(raw?.id ?? ''),
    threadId: String(raw?.thread_id ?? ''),
    tagId: raw?.tag_id ? String(raw.tag_id) : undefined,
    status: String(raw?.status ?? '') as AgentTurn['status'],
    errorCode: raw?.error_code ? String(raw.error_code) : undefined,
    plan: raw?.plan && typeof raw.plan === 'object' ? toAgentPlanSnapshot(raw.plan as WireRecord) : undefined,
    version: Number(raw?.version ?? 0),
    createdAt: String(raw?.created_at ?? ''),
    updatedAt: String(raw?.updated_at ?? ''),
    tasks,
    lastEventSeq: Number(raw?.last_event_seq ?? 0)
  }
}

function toAgentTask(raw: WireRecord): AgentTask {
  return {
    id: String(raw?.id ?? ''),
    turnId: String(raw?.turn_id ?? ''),
    seq: Number(raw?.seq ?? 0),
    ref: raw?.ref ? String(raw.ref) : undefined,
    targetKind: raw?.target_kind === 'human' ? 'human' : 'agent',
    targetId: String(raw?.target_id ?? ''),
    instruction: String(raw?.instruction ?? ''),
    status: String(raw?.status ?? '') as AgentTask['status'],
    attempt: Number(raw?.attempt ?? 0),
    runId: raw?.run_id ? String(raw.run_id) : undefined,
    assigneeId: raw?.assignee_id ? String(raw.assignee_id) : undefined,
    replyText: raw?.reply_text ? String(raw.reply_text) : undefined,
    replyOutcome: raw?.reply_outcome === 'unable' ? 'unable' : raw?.reply_outcome === 'continue' ? 'continue' : undefined,
    repliedBy: raw?.replied_by ? String(raw.replied_by) : undefined,
    dependsOn: listOf(raw?.depends_on, String),
    contextFrom: listOf(raw?.context_from, String),
    stepId: raw?.step_id ? String(raw.step_id) : undefined,
    risk: raw?.risk ? String(raw.risk) : undefined,
    expectedOutput: raw?.expected_output ? String(raw.expected_output) : undefined,
    result: raw?.result ? String(raw.result) : undefined,
    errorCode: raw?.error_code ? String(raw.error_code) : undefined,
    version: Number(raw?.version ?? 0),
    createdAt: String(raw?.created_at ?? '')
  }
}

/** 人工待办：snake → camel。 */
export function toAgentHumanTask(raw: WireRecord): AgentHumanTask {
  return {
    taskId: String(raw?.task_id ?? ''),
    turnId: String(raw?.turn_id ?? ''),
    threadId: String(raw?.thread_id ?? ''),
    threadTitle: raw?.thread_title ? String(raw.thread_title) : undefined,
    tagId: raw?.tag_id ? String(raw.tag_id) : undefined,
    instruction: String(raw?.instruction ?? ''),
    status: String(raw?.status ?? ''),
    assigneeId: String(raw?.assignee_id ?? ''),
    version: Number(raw?.version ?? 0),
    createdAt: String(raw?.created_at ?? '')
  }
}
