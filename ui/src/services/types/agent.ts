// 云助手域契约：助手 / 会话 / 标签 / 协作轮次 / 审批 / MCP / 流式事件。

export interface LlmMessage {
  role: 'system' | 'user' | 'assistant' | 'tool' | string
  content: string
  /** 仅 assistant 消息：模型发起过的工具调用（OpenAI 形状） */
  tool_calls?: LlmToolCall[]
  /** 仅 tool 消息：对应哪一次调用 */
  tool_call_id?: string
}

/** LlmToolCall 是 OpenAI 的工具调用形状；Arguments 是 JSON 字符串，不是对象。 */
export interface LlmToolCall {
  id: string
  type: string
  function: {
    name: string
    arguments: string
  }
}

/** LlmTool 是 OpenAI 的工具声明。 */
export interface LlmTool {
  type: 'function'
  function: {
    name: string
    description?: string
    parameters?: Record<string, unknown>
  }
}

export interface LlmTokenUsage {
  prompt_tokens?: number
  completion_tokens?: number
  total_tokens?: number
}

export interface LlmStreamConnection {
  close: () => void
}

export interface AgentModuleInfo {
  id: string
  name: string
  description: string
  default_tools: string[]
  team_supported: boolean
  /** 仅 sandbox 模块返回：云沙盒是否已启用 */
  sandbox_available?: boolean
}

export interface CloudAgent {
  id: string
  name: string
  module: string
  description: string
  system_prompt: string
  tool_ids: string[]
  model_override?: string
  /** 内置助手标识（general/database/s3/logs/sandbox）；空为自定义。 */
  builtin_key?: string
  team_enabled: boolean
  created_at: string
  updated_at: string
}

export interface AgentThread {
  id: string
  title: string
  /** 会话归属的助手。空串为未绑定。 */
  agent_id?: string
  /** 会话所属的工作台标签 ID。 */
  tag_id?: string
  /** 群会话的调度模式（'collaborative'：消息先经 Plan 栅栏）；空表示固定助手会话。 */
  dispatch_mode?: AgentDispatchMode
  /** 会话类型：普通 / 告警 / 统计 / 定时；未升级的旧会话为空。 */
  thread_kind?: AgentThreadKind
  /** 首页「一句话建应用」搭建会话的模板 id；普通会话为空。 */
  created_at: string
  updated_at: string
  last_message_preview?: string
}

export interface AgentThreadPage {
  threads: AgentThread[]
  next_cursor: string
}

export interface AgentMention {
  agent_id: string
}

export interface AgentToolCallCard {
  call_id?: string
  name?: string
  content?: string
  arguments?: string
  duration_ms?: number
  /** 工具执行失败。 */
  is_error?: boolean
  /** 结果超长被截断。 */
  truncated?: boolean
  /** 卡片阶段：generating / pending / awaiting_confirm / running / success / error。 */
  status?: string
  argv?: string[]
}

export interface ChatMsg {
  /** 服务端消息 id（历史消息）；实时气泡为空。 */
  id?: string
  role: 'user' | 'assistant' | 'tool'
  content: string
  toolCalls?: AgentToolCallCard[]
  error?: string
  canceled?: boolean
  thinking?: string
  /** 发出该条助手消息的 Agent module，用于头像与侧栏图标对齐。 */
  module?: string
  /** 实际回答助手 ID。 */
  agentId?: string
  /** 群轮次里该气泡对应的任务与轮次：气泡按 taskId 绑定，同一助手多个任务不串气泡。 */
  taskId?: string
  turnId?: string
}

export interface AgentMessage {
  id: string
  role: string
  content: string
  agent_id?: string
  mentions?: AgentMention[]
  tool_calls?: AgentToolCallCard[]
  run_id?: string
  /** 所属 run 状态：刷新后还原「已停止/失败」。 */
  run_status?: string
  error_code?: string
  created_at: string
}

export interface AgentTagPrompt {
  id: string
  title: string
  content: string
}

/** 标签总览助手条的展示项：id + 名称 + module 图标即可。 */
export interface MentionAgentLike {
  id: string
  name: string
  module?: string
}

/** 群（标签）是协作群：成员助手 + 路由提示词 + 兜底 + 人工策略。 */
export interface AgentTag {
  id: string
  /** 内置群：通用 / 告警 / 统计 / 定时。 */
  builtinKey?: AgentBuiltinTagKey
  name: string
  agentIds: string[]
  prompts: AgentTagPrompt[]
  /** 配置里已归档/不存在的助手，编辑器标「助手已不可用」。 */
  unavailableAgentIds: string[]
  /** 群路由提示词：只决定「谁来做、做什么」，不授予任何权限。 */
  routingPrompt: string
  /** 兜底助手（必须是成员）；空表示无兜底，路由失败时请人澄清。 */
  fallbackAgentId?: string
  /** 人工策略：'disabled' 表示该群不产生人工任务。 */
  humanPolicy?: 'disabled'
  /** Plan 确认策略；auto 由破坏性、任务数和依赖深度触发。 */
  confirmPolicy?: 'auto' | 'always' | 'never'
  /** 该群可创建的会话类型（内置群有默认值）。 */
  threadKinds: AgentThreadKind[]
  version: number
  updatedAt?: string
}

/** 内置群 key。 */
export type AgentBuiltinTagKey = 'general' | 'alerts' | 'stats' | 'scheduled'

export interface AgentTagInput {
  name?: string
  agentIds: string[]
  prompts: AgentTagPrompt[]
  routingPrompt?: string
  fallbackAgentId?: string
  humanPolicy?: 'disabled'
  confirmPolicy?: 'auto' | 'always' | 'never'
  expectedVersion?: number
}

/** 会话类型：普通 / 告警 / 统计 / 定时。 */
export type AgentThreadKind = 'chat' | 'alert' | 'statistics' | 'scheduled'

export interface AgentWorkspaceCapabilities {
  protocolVersion: number
  collaboration: boolean
  maxTasks: number
  maxDepth: number
  maxInstruction: number
  confirmPolicies: string[]
  threadKinds: AgentThreadKind[]
}

export interface AgentThreadCreateOptions {
  title?: string
  agentId?: string
  tagId?: string
  dispatchMode?: AgentDispatchMode
  threadKind?: AgentThreadKind
  source?: 'workspace' | 'starter'
}

/** 会话的调度模式：群会话只有 collaborative 一种（一轮一助手的 auto 已下线）。 */
export type AgentDispatchMode = 'collaborative'

export interface AgentThreadPageQuery {
  agentId?: string
  tagId?: string
  limit?: number
  cursor?: string
}

export interface AgentPlanConfirm {
  required: boolean
  reasons: string[]
}

export interface AgentPlanProgress {
  percent: number
  note: string
}

export interface AgentPlanStep {
  id: string
  title: string
  status: string
  note: string
}

export interface AgentPlanDispatch {
  ref: string
  targetKind: AgentTaskTargetKind
  targetId: string
  message: string
  dependsOn: string[]
  contextFrom: string[]
  stepId: string
  risk: string
  expectedOutput: string
}

export interface AgentPlanSource {
  kind: string
  ref: string
  usedFor: string
}

/** Plan 帧的结构化决策；UI 字段统一 camelCase。 */
export interface AgentPlanDecision {
  action: 'create' | 'update' | 'complete' | 'replan' | string
  goal: string
  complexity: 'simple' | 'complex' | string
  steps: AgentPlanStep[]
  dispatches: AgentPlanDispatch[]
  confirm: AgentPlanConfirm
  progress: AgentPlanProgress
  findings: string[]
  unknowns: string[]
  shouldContinue: boolean
  shouldSummarize: boolean
  sources: AgentPlanSource[]
}

export interface AgentPlanSnapshot {
  version: number
  decision: AgentPlanDecision
  confirmState: string
  replans: number
}

export interface AgentPlanEvent {
  turnId: string
  decision: AgentPlanDecision
  confirmRequired: boolean
  source?: string
}

export interface AgentPlanConfirmationEvent {
  turnId: string
  state: 'required' | 'approved' | 'rejected' | string
  edited: boolean
  version?: number
}

export interface AgentPlanEdit {
  ref: string
  message?: string
  drop?: boolean
}

export interface AgentPlanConfirmationRequest {
  approve: boolean
  edits?: AgentPlanEdit[]
  expectedVersion?: number
}

export type AgentTaskTargetKind = 'agent' | 'human'

/** 任务状态（与后端 systemdb 常量一致）。 */
export const AGENT_TASK_STATUSES = [
  'pending',
  'running',
  'waiting_human',
  'completed',
  'failed',
  'blocked',
  'canceled',
  'interrupted'
] as const

export type AgentTaskStatus = (typeof AGENT_TASK_STATUSES)[number]

export interface AgentTask {
  id: string
  turnId: string
  seq: number
  ref?: string
  targetKind: AgentTaskTargetKind
  targetId: string
  instruction: string
  status: AgentTaskStatus
  attempt: number
  runId?: string
  assigneeId?: string
  replyText?: string
  replyOutcome?: 'continue' | 'unable'
  repliedBy?: string
  dependsOn: string[]
  contextFrom?: string[]
  stepId?: string
  risk?: string
  expectedOutput?: string
  result?: string
  errorCode?: string
  version: number
  createdAt: string
}

export type AgentTurnStatus =
  | 'planning'
  | 'awaiting_plan_confirm'
  | 'running'
  | 'waiting_human'
  | 'completed'
  | 'failed'
  | 'canceled'
  | 'interrupted'
  | 'rejected'

export interface AgentTurn {
  id: string
  threadId: string
  tagId?: string
  status: AgentTurnStatus
  errorCode?: string
  /** v3 结构化 Plan 与确认状态。 */
  plan?: AgentPlanSnapshot
  version: number
  createdAt: string
  updatedAt: string
  tasks: AgentTask[]
  /** 事件表最大序号，供续传。 */
  lastEventSeq: number
}

/** 人工待办项（跨会话聚合）。 */
export interface AgentHumanTask {
  taskId: string
  turnId: string
  threadId: string
  threadTitle?: string
  tagId?: string
  instruction: string
  status: string
  assigneeId: string
  version: number
  createdAt: string
}

export interface AgentHumanTaskList {
  tasks: AgentHumanTask[]
  pending: number
}

/** 发起一轮的请求体（多目标 @）。 */
export interface AgentTurnRequest {
  content: string
  mentions: AgentMentionTarget[]
  idempotencyKey?: string
  stream?: boolean
  threadKind?: AgentThreadKind
}

/** 结构化 @ 目标：助手或人（人只能是当前登录用户）。 */
export interface AgentMentionTarget {
  kind: AgentTaskTargetKind
  id: string
}

/** 轮次动作：取消整轮 / 重试单个任务 / 恢复。 */
export interface AgentTurnActionRequest {
  action: 'cancel' | 'retry_task' | 'resume'
  taskId?: string
  expectedVersion?: number
}

/** task 帧：哪个任务、什么状态、总共有几步。 */
export interface AgentTaskEvent {
  taskId?: string
  agentId?: string
  targetKind?: AgentTaskTargetKind
  ref?: string
  dependsOn?: string[]
  result?: string
  index: number
  total: number
  status: AgentTaskStatus
  message?: string
  /** 该任务对应的子 run（后续事件按它分组）。 */
  runId?: string
}

/** human 帧：轮次停下等人。 */
export interface AgentHumanEvent {
  /** 对应任务 ID。 */
  taskId?: string
  humanId?: string
  humanName?: string
  index: number
  message?: string
}

/** turn 帧：轮次状态与进度。 */
export interface AgentTurnEvent {
  turnId?: string
  status: AgentTurnStatus | string
  index: number
  total: number
}

/** 群成员写入：加入已有助手 与 新建助手并加入 二选一。 */
export interface AgentGroupMembersInput {
  agentIds?: string[]
  newAgent?: AgentNewMemberInput
  expectedVersion?: number
}

/** 「新建助手并加入」的入参（与 /agents 创建体同构）。 */
export interface AgentNewMemberInput {
  name: string
  module: string
  description?: string
  systemPrompt?: string
  toolIds?: string[]
}

/** 人工回复：continue（回复并继续）与 unable（无法处理）。 */
export interface AgentHumanReplyInput {
  outcome: 'continue' | 'unable'
  text?: string
  expectedVersion?: number
}

export interface AgentRunMetrics {
  duration_ms: number
  prompt_tokens: number
  completion_tokens: number
  reasoning_tokens: number
  tool_calls: number
  error_code?: string
}

/** SSE usage 帧原始字段（经 `Number(x || 0)` 兜底，字段可能缺省或为字符串）。 */
export interface AgentUsageEventWire {
  duration_ms?: unknown
  prompt_tokens?: unknown
  completion_tokens?: unknown
  reasoning_tokens?: unknown
  tool_calls?: unknown
}

export interface AgentRun extends Partial<AgentRunMetrics> {
  id: string
  thread_id: string
  agent_id: string
  status: string
  error?: string
}

/** 正在执行的 run（交互 + 定时任务），用于助手列表 / 会话切换器的「正在回复」指示。 */
export interface ActiveAgentRun {
  run_id: string
  agent_id: string
  thread_id: string
  started_at?: string
}

/** 审批状态机：pending → approved | denied | expired | canceled，终态不可回退。 */
export type AgentApprovalStatus = 'pending' | 'approved' | 'denied' | 'expired' | 'canceled'

/** 一条破坏性工具调用的审批记录（跨 run 的待办）。 */
export interface AgentApproval {
  id: string
  run_id: string
  call_id: string
  thread_id?: string
  agent_id?: string
  tool_name?: string
  /** 脱敏后的参数摘要，后端已做 RedactArgv。 */
  argv: string[]
  message?: string
  status: AgentApprovalStatus
  initiator_id?: string
  resolved_by?: string
  created_at: string
  expires_at?: string
  resolved_at?: string
}

export interface AgentApprovalList {
  approvals: AgentApproval[]
  /** 项目内 pending 总数（不受 status 过滤影响，用于角标）。 */
  pending: number
}

/** 单个 MCP 工具的权限标记（事实来源，Server 自报的 annotations 只作建议）。 */
export interface McpToolPolicy {
  read_only?: boolean
  destructive?: boolean
}

/** 项目级 MCP Server 配置。凭据只单向写，读回来的只有 header 名字。 */
export interface McpServer {
  id: string
  name: string
  transport: string
  url: string
  timeout_s: number
  enabled: boolean
  tools_policy: Record<string, McpToolPolicy>
  /** Header 名（不含值）：有名字就说明配了凭据。 */
  header_names: string[]
  has_credential: boolean
  /** 历史配置占用了内置保留名；Agent 引用时内置 Server 优先。 */
  name_conflict?: boolean
  created_at: string
  updated_at: string
}

/** 内置 MCP 的单个工具（事实来源是后端工具表的 ReadOnly / Destructive）。 */
export interface McpBuiltinTool {
  name: string
  read_only: boolean
  destructive: boolean
}

/** 项目 API 封装的内置 MCP Server：只读展示，仅可开关与测试。 */
export interface McpBuiltinServer {
  name: string
  enabled: boolean
  transport: string
  url: string
  tools: McpBuiltinTool[]
}

/** GET /mcp/servers 的响应：用户配置 + 内置。 */
export interface McpServerList {
  servers: McpServer[]
  builtin: McpBuiltinServer[]
}

export interface McpServerBody {
  name?: string
  url?: string
  /** 省略表示沿用既有值；传 {} 表示清空。 */
  headers?: Record<string, string>
  timeout_s?: number
  enabled?: boolean
  tools_policy?: Record<string, McpToolPolicy>
}

export interface McpTestResult {
  ok: boolean
  tools: string[]
  error?: string
}

export interface AgentRunRequest {
  content: string
  mentions: AgentMention[]
  stream?: boolean
  /** 重试指定失败 run：复用原 user 消息，不重复落库。 */
  retry_of_run_id?: string
}

export interface AgentStreamHandlers {
  onRun?: (runId: string, agentId?: string) => void
  /** v3：结构化 Plan 与整轮确认状态。 */
  onPlan?: (ev: AgentPlanEvent) => void
  onPlanConfirmation?: (ev: AgentPlanConfirmationEvent) => void
  /** v2：任务状态帧（每个任务开始/结束各一次）。 */
  onTask?: (ev: AgentTaskEvent) => void
  /** v2：人工待办帧（轮次停下等人）。 */
  onHuman?: (ev: AgentHumanEvent) => void
  /** v2：轮次状态帧（完成/失败/等人）。 */
  onTurn?: (ev: AgentTurnEvent) => void
  /**
   * v2：每一帧的归属上下文（run_id / agent_id）。
   *
   * 轮次模式下多助手串行，token / 工具 / 审批帧必须知道属于哪个任务；
   * 单助手路径不使用该回调，行为不变。
   */
  onFrameContext?: (ctx: { runId?: string; agentId?: string }) => void
  onThinking?: (elapsedMs: number, content?: string) => void
  onToken?: (text: string) => void
  onToolCall?: (name: string, args: string, callId?: string) => void
  onToolCallDelta?: (name: string, argsDelta: string, callId?: string) => void
  onToolStart?: (name: string, callId?: string) => void
  onConfirmationRequired?: (callId: string, argv: string[], message: string) => void
  onConfirmationResolved?: (callId: string, approved: boolean) => void
  onToolResult?: (name: string, content: string, callId?: string, durationMs?: number, isError?: boolean, truncated?: boolean) => void
  /** 工具执行心跳：长工具期间周期推送。 */
  onToolProgress?: (name: string, elapsedMs: number, callId?: string) => void
  onUsage?: (metrics: AgentRunMetrics) => void
  onEnd?: (reason?: string) => void
  onError?: (e: unknown) => void
}

/** /agents/models 响应。 */
export interface AgentModelsResponse {
  default_model: string
  models: { provider: string; name: string }[]
}
