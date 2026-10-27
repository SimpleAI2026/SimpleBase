/** 统一的后端接口类型定义与 API 抽象（契约对照 plan/planv2.0/proto-http.md） */

/**
 * 请求失败错误：在 message 之外保留 HTTP status 与后端业务 code。
 *
 * `http.ts` 的拦截器此前只 reject `new Error(message)`，status 和 code 全部丢失，
 * 调用方无法区分「资源未建立（应当展示为空）」与「真正的失败」（planv6.0 §2.2）。
 */
export interface ApiError extends Error {
  status?: number
  code?: string
}

export interface MetricsSummary {
  totalRequests: number
  errorRate: number
  avgLatencyMs: number
  activeDatabases: number
  /** 近 24h 接口耗时估算分位数（ms）；无样本为 null */
  latencyP50Ms?: number | null
  latencyP90Ms?: number | null
  latencyP99Ms?: number | null
  /** 参与分位数统计的请求数（可能少于 totalRequests：升级前样本无直方图） */
  latencySampleCount?: number
  /** 最高有限桶上界（ms）；分位数 >= 该值时显示为「≥」 */
  latencyOverflowMs?: number
}

export interface TrendPoint {
  date: string
  requests: number
  errors: number
}

/**
 * 监控摘要原始 JSON：后端 snake_case，兼容旧版 camelCase（snake 优先）。
 * 字段全部经 Number / optionalNumber 兜底，故为 unknown。
 */
export interface MetricsSummaryWire {
  total_requests?: unknown
  totalRequests?: unknown
  error_rate?: unknown
  errorRate?: unknown
  avg_latency_ms?: unknown
  avgLatencyMs?: unknown
  active_databases?: unknown
  activeDatabases?: unknown
  latency_p50_ms?: unknown
  latencyP50Ms?: unknown
  latency_p90_ms?: unknown
  latencyP90Ms?: unknown
  latency_p99_ms?: unknown
  latencyP99Ms?: unknown
  latency_sample_count?: unknown
  latencySampleCount?: unknown
  latency_overflow_ms?: unknown
  latencyOverflowMs?: unknown
}

/** 趋势点原始 JSON（字段经 String / Number 兜底）。 */
export interface TrendPointWire {
  date?: unknown
  requests?: unknown
  errors?: unknown
}

export interface DbRow {
  id: string
  [key: string]: unknown
}

export interface CollectionInfo {
  name: string
  schemaMode: 'json' | 'view'
  fieldCount: number
  documentCount: number
  registered: boolean
}

export interface CollectionFieldOptions {
  min?: number | string
  max?: number | string
  pattern?: string
  values?: string[]
  maxSelect?: number
  maxSize?: number
  onlyInt?: boolean
  exceptDomains?: string[]
  onlyDomains?: string[]
}

export interface CollectionField {
  name: string
  type: 'text' | 'number' | 'bool' | 'date' | 'email' | 'url' | 'select' | 'json'
  required: boolean
  unique: boolean
  default?: unknown
  options: CollectionFieldOptions
  hidden: boolean
  readonly: boolean
}

export interface CollectionSchema {
  id?: string
  projectId?: string
  name: string
  schemaMode: 'json' | 'view'
  viewQuery?: string
  fields: CollectionField[]
}

export interface CollectionSchemaInput {
  name: string
  schemaMode?: 'json' | 'view'
  viewQuery?: string
  fields: CollectionField[]
}

export interface ViewDryRunResult {
  columns: string[]
  fields: CollectionField[]
  items: DocumentItem[]
  scanned: number
}

export interface CollectionImportResult {
  dryRun: boolean
  scanned: number
  imported: number
}

export interface FunctionTrigger {
  id: string
  projectId: string
  kind: 'record_change' | 'object_event' | 'http'
  target: string
  events: string[]
  funcFile: string
  funcExport: string
  filter?: string
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export interface FunctionTriggerInput {
  kind: FunctionTrigger['kind']
  target: string
  events: string[]
  funcFile: string
  funcExport: string
  filter?: string
  enabled: boolean
}

export interface DocumentItem {
  id: string
  createdAt: string
  updatedAt: string
  data: Record<string, unknown>
}

export interface DocumentQuery {
  limit?: number
  cursor?: string
  offset?: number
  sort?: string
  filter?: string
  fields?: string
  count?: boolean
}

export interface DocumentPage {
  rows: DbRow[]
  items: DocumentItem[]
  nextCursor?: string
  total?: number
}

export interface SchemaDryRunResult {
  invalid: { id: string; field: string; reason: string }[]
  scanned: number
  truncated: boolean
}

// —— Key-Value 数据服务（key-value-ducklake-plan §3，项目级单端点）——

export type KvType = 'string' | 'list' | 'set' | 'hash' | 'zset'

/** SCAN 回复里的一条 key 元信息（cmd SCAN 展开用） */
export interface KvKeyMeta {
  key: string
  type: KvType
  /** 元素计数；string 类型为 null */
  len: number | null
  /** 剩余 TTL 毫秒；null = 永久 */
  ttl_ms: number | null
  mtime_ms: number
  version: number
}

/** cmd 请求体：argvs[0] 是命令名，其余为参数（全部字符串，数字由服务端解析） */
export interface KvCmdBody {
  type: 'cmd'
  argvs: string[]
}

/** 按类型写入一份数据的 args（key 必填；其余字段按类型取用） */
export interface KvTypedArgs {
  key: string
  /** String：文本值 */
  value?: string
  /** Hash：字段表（至少一个） */
  fields?: Record<string, string>
  /** List / Set：元素数组（非空） */
  elems?: string[]
  /** List：back（默认）| front */
  side?: 'back' | 'front'
  /** ZSet：成员与分数（非空） */
  items?: { elem: string; score: number }[]
  /** 可选：写成功后按毫秒设置过期；0 = 立即过期 */
  ttl_ms?: number
  /** String：仅不存在时写入 */
  nx?: boolean
  /** String：仅已存在时写入 */
  xx?: boolean
  /** String：保留已有 TTL */
  keep_ttl?: boolean
}

export interface KvTypedBody {
  type: 'String' | 'Hash' | 'List' | 'Set' | 'ZSet'
  args: KvTypedArgs
}

/** POST /v1/projects/:projectId/kv 的请求体 */
export type KvExecBody = KvCmdBody | KvTypedBody

export interface S3Object {
  key: string
  size: number
  /** 注意：S3 域是后端 snake_case 命名的例外（proto-http.md §3.4） */
  lastModified?: string
}

export interface BackupItem {
  id: string
  projectId: string
  sizeBytes: number
  sha256: string
  engine: string
  formatVersion: number
  status: string
  createdAt: string
  expiresAt?: string
}

export interface LogEvent {
  id: string
  projectId: string
  level: string
  logger: string
  message: string
  fieldsJson?: string
  requestId?: string
  occurredAt: string
}

export interface LogQuery {
  level?: string
  q?: string
  from?: string
  to?: string
  limit?: number
}

export interface LogRetention {
  scope: string
  keepDays: number
  updatedAt: string
}

/**
 * 日志事件原始 JSON：兼容 snake_case / camelCase / Go 默认 PascalCase（按此优先级）。
 * fields_json / request_id 原样透传（null 归一为 undefined），其余经 String 兜底。
 */
export interface LogEventWire {
  id?: unknown
  ID?: unknown
  project_id?: unknown
  projectId?: unknown
  ProjectID?: unknown
  level?: unknown
  Level?: unknown
  logger?: unknown
  Logger?: unknown
  message?: unknown
  Message?: unknown
  fields_json?: string | null
  fieldsJson?: string | null
  FieldsJSON?: string | null
  request_id?: string | null
  requestId?: string | null
  RequestID?: string | null
  occurred_at?: unknown
  occurredAt?: unknown
  OccurredAt?: unknown
}

/** 日志保留策略原始 JSON（三种命名风格，字段经 String / Number 兜底）。 */
export interface LogRetentionWire {
  scope?: unknown
  Scope?: unknown
  keep_days?: unknown
  keepDays?: unknown
  KeepDays?: unknown
  updated_at?: unknown
  updatedAt?: unknown
  UpdatedAt?: unknown
}

/** 项目级模型连接 + 默认对话参数（服务端 sys_llm_settings）。 */
export interface LlmSettings {
  defaultProvider?: string
  defaultModel?: string
  /** 供应商 Base URL；留空用厂商默认 */
  baseUrl?: string
  /** 已保存 Key 的掩码（如 sk-...abcd），只读展示 */
  apiKeyMasked?: string
  /** 服务端是否已保存 Key；写入时 apiKey 留空表示沿用既有值 */
  hasApiKey?: boolean
  temperature?: number
  maxTokens?: number
}

/* ---------- Projects ---------- */

export interface ProjectItem {
  id: string
  name: string
  /** 项目单文件数据库引擎；物理路径不对外暴露 */
  engine?: 'duckdb' | 'sqlite'
  createdAt: string
  /** admin 管理数据库（系统项目）标记，super/admin 可见 */
  managed?: boolean
}

/* ---------- 项目 API Keys ---------- */

/** API Key 条目（列表返回；不含明文） */
export interface ApiKeyScope {
  resources: string[]
  actions: string[]
  hiddenFields: string[]
  readonlyFields: string[]
  expiresAt?: string
}

export interface ApiKeyItem {
  id: string
  projectId: string
  permissions: string[]
  createdAt: string
  revokedAt?: string
}

export interface S3ListResult {
  folders: string[]
  objects: S3Object[]
}

export interface S3Link {
  url: string
  expiresAt: string
}

export interface LogStat {
  key: string
  count: number
}

/* ---------- Auth / Users（login-auth-plan） ---------- */

export type UserRole = 'superadminl1' | 'admin' | 'user'

export interface AuthUser {
  id: string
  username: string
  role: UserRole
  displayName: string
  email: string
  status: 'active' | 'disabled'
  mustChangePassword: boolean
  createdBy?: string
  createdAt?: string
  lastLoginAt?: string
}

export interface LoginRequest {
  username: string
  password: string
}

export interface TokenPair {
  tokenType: string
  accessToken: string
  expiresIn: number
  refreshToken: string
  user: AuthUser
}

export interface UserItem extends AuthUser {
  projectCount: number
}

export interface UserListResult {
  users: UserItem[]
  nextCursor: string
}

export interface CreateUserRequest {
  username: string
  password: string
  role: Exclude<UserRole, 'superadminl1'>
  displayName?: string
  email?: string
}

export interface UpdateUserRequest {
  role?: UserRole
  displayName?: string
  email?: string
  status?: 'active' | 'disabled'
  password?: string
  mustChangePassword?: boolean
}

/* ---------- Databases（proto-http.md §3.1） ---------- */

/** 数据库资源。成功创建对 UI 是 ready。creating 只在创建过程中短暂出现。 */
export interface DatabaseItem {
  id: string
  name: string
  status: 'creating' | 'ready' | 'degraded' | 'deleting' | 'deleted'
  createdAt: string
  updatedAt: string
  /** 仅详情接口可能返回（服务端注入 SnapshotFor 时） */
  snapshot?: { lastSyncedSnapshot: number; syncLag: number }
  /** 库内用户表总行数；未知/未就绪时省略 */
  documentCount?: number
  /** 数据形态。缺省按集合文档处理，与存量库迁移默认一致 */
  dataModel?: 'collection' | 'sql'
}

/** 创建数据库。省略 dataModel 时服务端按集合文档创建。 */
export interface CreateDatabaseInput {
  name: string
  dataModel?: 'collection' | 'sql'
  initSql?: string
}

/** 表结构列。nullable 省略视为可空。sensitive 仅系统库凭证列。 */
export interface SchemaColumn {
  name: string
  type: string
  nullable?: boolean
  sensitive?: boolean
}

/** 一张表的分页行。rows 与 columns 按下标对齐。 */
export interface SchemaTableRows {
  table: string
  columns: SchemaColumn[]
  rows: unknown[][]
  limit: number
  offset: number
  total: number
}

export interface SchemaTable {
  name: string
  columns: SchemaColumn[]
}

export interface DatabaseSchema {
  tables: SchemaTable[]
}

/* ---------- SQL（proto-http.md §3.2） ---------- */

/** query/execute 单语句请求 */
export interface SqlRequest {
  sql: string
  args?: unknown[]
  /** 仅 query 有效，缺省 1000 */
  maxRows?: number
}

export interface SqlBatchRequest {
  statements: SqlRequest[]
  transactional: boolean
}

/** query 结果。rows 是二维数组，列名需与 columns 按下标 zip */
export interface SqlQueryResult {
  columns: string[]
  rows: unknown[][]
  rowCount: number
  durationMs: number
  requestId: string
}

export interface SqlExecuteResult {
  rowsAffected: number
  durability: string
  durationMs: number
  requestId: string
}

export interface SqlBatchResultItem {
  index: number
  rowsAffected?: number
  durationMs?: number
  errorCode?: string
  errorMessage?: string
}

export interface SqlBatchResult {
  results: SqlBatchResultItem[]
  durability: string
  durationMs: number
  requestId: string
  error?: { failedIndex: number; code: string; message: string }
}

/* ---------- Quota（proto-http.md §3.6） ---------- */

export interface QuotaStatus {
  llmAllowed: boolean
  databaseAllowed: boolean
}

/* ---------- LLM Gateway（proto-http.md §3.5） ---------- */

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

export interface LlmChatRequest {
  model?: string
  messages: LlmMessage[]
  maxTokens?: number
  temperature?: number
}

export interface LlmTokenUsage {
  prompt_tokens?: number
  completion_tokens?: number
  total_tokens?: number
}

export interface LlmChatResponse {
  content: string
  usage?: LlmTokenUsage
  model: string
  provider: string
  finish_reason?: string
}

export interface LlmStreamHandlers {
  onChunk?: (text: string) => void
  onEnd?: () => void
  onError?: (e: unknown) => void
}

export interface LlmStreamConnection {
  close: () => void
}

/* ---------- 云助手（proto-http.md §3.12） ---------- */

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
  /** 内置助手标识（general/database/s3/logs/sandbox）；空为自定义（planv4.1 BUG-11）。 */
  builtin_key?: string
  team_enabled: boolean
  /** 该助手是否允许写（planv6.0 v2 §3.2）：默认 false，不可写时只能执行只读命令。 */
  writable: boolean
  created_at: string
  updated_at: string
}

export interface AgentThread {
  id: string
  title: string
  /** 会话归属的助手（planv6.0 §3.7 修订：每个助手一份会话）。空串为未绑定。 */
  agent_id?: string
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
  /** 工具执行失败（planv4.1 BUG-02）。 */
  is_error?: boolean
  /** 结果超长被截断。 */
  truncated?: boolean
  /** 卡片阶段：generating / pending / awaiting_confirm / running / success / error。 */
  status?: string
  argv?: string[]
}

export interface AgentMessage {
  id: string
  role: string
  content: string
  agent_id?: string
  mentions?: AgentMention[]
  tool_calls?: AgentToolCallCard[]
  run_id?: string
  /** 所属 run 状态（planv4.1 BUG-06）：刷新后还原「已停止/失败」。 */
  run_status?: string
  error_code?: string
  created_at: string
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

/* ---------- Agent Schedule（定时执行） ---------- */

export interface AgentSchedule {
  id: string
  agent_id: string
  agent_name?: string
  thread_id: string
  prompt: string
  cron_expr: string
  enabled: boolean
  last_run_at?: string
  next_run_at?: string
  created_by?: string
  created_at: string
  updated_at: string
}

export interface AgentScheduleBody {
  agent_id: string
  prompt: string
  cron_expr: string
  enabled?: boolean
  thread_id?: string
}

export interface AgentScheduleRun {
  id: string
  schedule_id: string
  run_id: string
  trigger: string
  status: string
  error?: string
  started_at?: string
  finished_at?: string
  created_at: string
}

export interface AgentRunRequest {
  content: string
  mentions: AgentMention[]
  stream?: boolean
  /** 重试指定失败 run（planv4.1 BUG-03）：复用原 user 消息，不重复落库。 */
  retry_of_run_id?: string
}

export interface AgentStreamHandlers {
  onRun?: (runId: string) => void
  onThinking?: (elapsedMs: number, content?: string) => void
  onToken?: (text: string) => void
  onToolCall?: (name: string, args: string, callId?: string) => void
  onToolCallDelta?: (name: string, argsDelta: string, callId?: string) => void
  onToolStart?: (name: string, callId?: string) => void
  onConfirmationRequired?: (callId: string, argv: string[], message: string) => void
  onConfirmationResolved?: (callId: string, approved: boolean) => void
  onToolResult?: (name: string, content: string, callId?: string, durationMs?: number, isError?: boolean, truncated?: boolean) => void
  /** 工具执行心跳（planv4.1 BUG-04）：长工具期间周期推送。 */
  onToolProgress?: (name: string, elapsedMs: number, callId?: string) => void
  onUsage?: (metrics: AgentRunMetrics) => void
  onEnd?: (reason?: string) => void
  onError?: (e: unknown) => void
}

/** /agents/models 响应（planv4.1 BUG-07）。 */
export interface AgentModelsResponse {
  default_model: string
  models: { provider: string; name: string }[]
}

/* ---------- GoFunctions（gofunction-versions-testplan） ---------- */

export interface GoFuncVersionSummary {
  version: number
  exports: string[]
  note: string
  createdAt: string
  active: boolean
  source?: string
}

/** 云函数实体。file = name + ".go" */
export interface GoFunctionItem {
  id: string
  name: string
  file: string
  description: string
  /** 0 = 未发布 */
  activeVersion: number
  latestVersion: number
  published: boolean
  /** 生效版（或最新版）导出 */
  exports: string[]
  versions?: GoFuncVersionSummary[]
  source?: string
  createdAt: string
  updatedAt: string
}

export interface GoFunctionCreate {
  name: string
  source: string
  description?: string
  note?: string
  /** 默认 true：保存后设为生效 */
  activate?: boolean
}

export interface GoFuncVersionCreate {
  source: string
  note?: string
  activate?: boolean
}

export interface GoFuncTestResult {
  ok: boolean
  statusCode: number
  durationMs: number
  version: number
  activeVersion: number
  functionName: string
  data?: unknown
  error?: string
}

/* ---------- CronJobs（proto-http.md §3.14） ---------- */

/** 定时任务调度模式：cron 定时执行 / interval 固定间隔 / once 一次性执行 */
export type CronScheduleKind = 'cron' | 'interval' | 'once'

/** 定时任务目标类型：function 云函数导出函数 / agent 助手 */
export type CronTargetKind = 'function' | 'agent'

/** 助手提示词长度上限（字节，对齐后端 cronJobAgentPromptLimit = 16KiB）。 */
export const CRON_AGENT_PROMPT_MAX_BYTES = 16 * 1024

/** 定时任务资源。目标为云函数导出函数或助手；时间一律 UTC */
export interface CronJobItem {
  id: string
  name: string
  description: string
  /** 目标类型；旧数据缺省视为 function */
  targetKind: CronTargetKind
  /** targetKind=agent 时非空 */
  agentId?: string
  agentThreadId?: string
  agentPrompt?: string
  scheduleKind: CronScheduleKind
  /** scheduleKind=cron 时非空：5 字段 cron 表达式 */
  cronExpr: string
  /** scheduleKind=interval 时非空：秒（60 ~ 2592000） */
  intervalSeconds?: number
  /** scheduleKind=once 时非空：一次性执行时刻（ISO，UTC） */
  runAt?: string
  funcFile: string
  funcExport: string
  /** 固定入参 JSON 原文，默认 "{}" */
  inputJson: string
  enabled: boolean
  lastRunAt?: string
  nextRunAt?: string
  /** 最近一次执行状态：completed / failed / running / ''（未运行） */
  lastStatus: string
  lastError: string
  runCount: number
  /** 目标云函数缺失（文件被删/函数未导出）；执行将失败 */
  targetMissing: boolean
  createdAt: string
  updatedAt: string
}

/** 一次定时任务执行记录 */
export interface CronJobRunItem {
  id: string
  jobId: string
  trigger: 'scheduled' | 'manual'
  status: 'running' | 'completed' | 'failed' | 'canceled'
  error: string
  durationMs: number
  /** 返回值 JSON（截断 4KB）；空串表示无输出 */
  responseJson: string
  startedAt?: string
  finishedAt?: string
  createdAt: string
}

/**
 * 后端定时任务原始 JSON（snake_case，internal/api/cronjob_handler.go 的 cronJobDTO）。
 * 仅供 http-api 映射使用：经 String/Number/Boolean 兜底的字段标为 unknown；
 * 原样透传（`|| undefined`）的时间字段按后端契约标为字符串。
 */
export interface CronJobWire {
  id?: unknown
  name?: unknown
  description?: unknown
  schedule_kind?: unknown
  cron_expr?: unknown
  interval_seconds?: unknown
  run_at?: string | null
  target_kind?: unknown
  agent_id?: unknown
  agent_thread_id?: unknown
  agent_prompt?: unknown
  func_file?: unknown
  func_export?: unknown
  input_json?: unknown
  enabled?: unknown
  last_run_at?: string | null
  next_run_at?: string | null
  last_status?: unknown
  last_error?: unknown
  run_count?: unknown
  target_missing?: unknown
  created_at?: unknown
  updated_at?: unknown
}

/** 后端定时任务执行记录原始 JSON（cronJobRunDTO）。 */
export interface CronJobRunWire {
  id?: unknown
  job_id?: unknown
  trigger?: unknown
  status?: unknown
  error?: unknown
  duration_ms?: unknown
  response_json?: unknown
  started_at?: string | null
  finished_at?: string | null
  created_at?: unknown
}

export interface CronJobCreate {
  name: string
  description: string
  scheduleKind: CronScheduleKind
  cronExpr: string
  intervalSeconds?: number
  /** scheduleKind=once 时必填 */
  runAt?: string
  /** 缺省 function */
  targetKind?: CronTargetKind
  /** targetKind=function 时必填 */
  funcFile?: string
  funcExport?: string
  inputJson?: string
  /** targetKind=agent 时必填 */
  agentId?: string
  agentPrompt?: string
  enabled?: boolean
}

/* ---------- 云沙盒（planv6.2 E2B 兼容 API） ---------- */

export interface SandboxSshInfo {
  enabled: boolean
  listen?: string
  hostKeyFingerprint?: string
}
export interface SandboxCapabilities {
  available: boolean
  backend: string
  /** 不可用时的原因（后端固定文案） */
  reason?: string
  images: string[]
  defaultImage: string
  defaultTemplate?: string
  cpus: number
  memoryMiB: number
  timeoutDefaultS: number
  timeoutMaxS: number
  maxFileBytes: number
  maxOutputBytes: number
  maxPerProject: number
  terminal: boolean
  ssh?: SandboxSshInfo
  envdVersion?: string
}
export interface SandboxItem {
  id: string
  name: string
  templateId: string
  /** E2B 二态 */
  state: 'running' | 'paused'
  /** x_simplebase 扩展里的五态原值：pending|running|stopped|expired|error */
  status: string
  source: string
  cpuCount: number
  memoryMB: number
  startedAt?: string
  endAt?: string
  lastActiveAt?: string
  lastError?: string
  idleTimeoutS: number
  metadata: Record<string, string>
}
export interface SandboxCreate {
  name?: string
  templateId?: string
  timeoutS?: number
  metadata?: Record<string, string>
}
export interface SandboxTerminalTicket {
  ticket: string
  expiresIn: number
}
export interface SandboxFileEntry {
  name: string
  path: string
  kind: 'file' | 'directory'
  /** envd 的 size 是字符串形式的 int64，这里已转为 number */
  size: number
  permissions: string
  modifiedTime?: string
}

export interface OpenAPIParameter {
  name: string
  in: 'path' | 'query'
  required: boolean
}

export interface OpenAPIOperation {
  operationId: string
  'x-tryable': boolean
  parameters?: OpenAPIParameter[]
  requestBody?: { required: boolean; content: Record<string, unknown> }
}

export interface OpenAPIDocument {
  openapi: string
  paths: Record<string, Record<string, OpenAPIOperation>>
}

export interface APIPreviewResult {
  status: number
  body: string
}

/**
 * API 统一抽象：http 实现与 mock 实现均遵循该接口。
 * projectId 一律为方法首个参数，由调用方从 stores/project.ts 读取后显式传入
 * （services 层不 import store，保持无状态可测）。
 */
export interface Api {
  preview: {
    document: () => Promise<OpenAPIDocument>
    request: (method: string, path: string, body?: unknown) => Promise<APIPreviewResult>
  }
  auth: {
    login: (req: LoginRequest) => Promise<TokenPair>
    logout: (refreshToken?: string) => Promise<void>
    me: () => Promise<AuthUser & { projects: { id: string; name?: string; owner: boolean }[] }>
    changePassword: (oldPassword: string, newPassword: string) => Promise<void>
  }
  users: {
    list: (limit?: number, cursor?: string) => Promise<UserListResult>
    create: (req: CreateUserRequest) => Promise<UserItem>
    update: (id: string, req: UpdateUserRequest) => Promise<UserItem>
  }
  triggers: {
    list: (projectId: string) => Promise<FunctionTrigger[]>
    create: (projectId: string, input: FunctionTriggerInput) => Promise<FunctionTrigger>
    patch: (projectId: string, id: string, input: Partial<FunctionTriggerInput>) => Promise<FunctionTrigger>
    remove: (projectId: string, id: string) => Promise<void>
    invoke: (projectId: string, id: string, body: Record<string, unknown>) => Promise<void>
  }
  gofunctions: {
    list: (projectId: string) => Promise<GoFunctionItem[]>
    create: (projectId: string, body: GoFunctionCreate) => Promise<GoFunctionItem>
    get: (projectId: string, name: string) => Promise<GoFunctionItem>
    /** 保存为新版本（可选设为生效） */
    saveVersion: (projectId: string, name: string, body: GoFuncVersionCreate) => Promise<GoFunctionItem>
    remove: (projectId: string, name: string) => Promise<void>
    listVersions: (
      projectId: string,
      name: string
    ) => Promise<{ activeVersion: number; versions: GoFuncVersionSummary[] }>
    activate: (projectId: string, name: string, version: number) => Promise<{ activeVersion: number }>
    /** 调试台试跑指定版本 */
    test: (
      projectId: string,
      name: string,
      version: number,
      functionName: string,
      body: unknown
    ) => Promise<GoFuncTestResult>
  }
  sandboxes: {
    capabilities: (projectId: string) => Promise<SandboxCapabilities>
    list: (projectId: string, state?: string, metadataFilter?: Record<string, string>) => Promise<SandboxItem[]>
    get: (projectId: string, id: string) => Promise<SandboxItem>
    create: (projectId: string, body: SandboxCreate) => Promise<SandboxItem>
    remove: (projectId: string, id: string) => Promise<void>
    connect: (projectId: string, id: string, timeoutS?: number) => Promise<SandboxItem>
    pause: (projectId: string, id: string) => Promise<void>
    terminalTicket: (projectId: string, id: string) => Promise<SandboxTerminalTicket>
    terminalWebSocketUrl: (id: string, ticket: string) => string
    files: {
      list: (projectId: string, id: string, path: string, depth?: number) => Promise<SandboxFileEntry[]>
      read: (projectId: string, id: string, path: string) => Promise<Blob>
      write: (projectId: string, id: string, path: string, content: Blob | string) => Promise<void>
      remove: (projectId: string, id: string, path: string) => Promise<void>
    }
  }
  cronjobs: {
    list: (projectId: string) => Promise<CronJobItem[]>
    create: (projectId: string, body: CronJobCreate) => Promise<CronJobItem>
    /** PATCH：name 不可改；调度变更后服务端重算 nextRunAt */
    update: (projectId: string, jobId: string, body: Partial<CronJobCreate>) => Promise<CronJobItem>
    remove: (projectId: string, jobId: string) => Promise<void>
    runs: (projectId: string, jobId: string, limit?: number) => Promise<CronJobRunItem[]>
    /** 手动立即执行（异步 202；不改排期） */
    trigger: (projectId: string, jobId: string) => Promise<void>
  }
  projects: {
    list: () => Promise<ProjectItem[]>
    create: (req: { name: string; id?: string; engine?: 'duckdb' | 'sqlite'; ownerUserId?: string }) => Promise<ProjectItem>
  }
  /** 项目 API Key 管理（创建/列表/吊销；明文仅创建响应一次性返回） */
  apiKeys: {
    list: (projectId: string) => Promise<ApiKeyItem[]>
    create: (projectId: string, permissions?: string[], scope?: ApiKeyScope) => Promise<ApiKeyItem & { secret: string }>
    revoke: (projectId: string, keyId: string) => Promise<void>
  }
  metrics: {
    summary: (projectId: string) => Promise<MetricsSummary>
    trend: (projectId: string) => Promise<TrendPoint[]>
  }
  databases: {
    list: (projectId: string) => Promise<DatabaseItem[]>
    create: (projectId: string, input: CreateDatabaseInput) => Promise<DatabaseItem>
    remove: (projectId: string, databaseId: string) => Promise<void>
    schema: (projectId: string, databaseId: string) => Promise<DatabaseSchema>
    tableRows: (
      projectId: string,
      databaseId: string,
      table: string,
      query: { limit: number; offset: number }
    ) => Promise<SchemaTableRows>
    createTable: (
      projectId: string,
      databaseId: string,
      table: SchemaTable
    ) => Promise<SchemaTable>
    addColumn: (
      projectId: string,
      databaseId: string,
      column: { table: string; name: string; type: string; nullable?: boolean }
    ) => Promise<SchemaColumn>
  }
  sql: {
    query: (projectId: string, databaseId: string, req: SqlRequest) => Promise<SqlQueryResult>
    execute: (projectId: string, databaseId: string, req: SqlRequest) => Promise<SqlExecuteResult>
    batch: (projectId: string, databaseId: string, req: SqlBatchRequest) => Promise<SqlBatchResult>
  }
  db: {
    collections: (projectId: string, databaseId: string) => Promise<string[]>
    createCollection: (projectId: string, databaseId: string, name: string) => Promise<void>
    rows: (
      projectId: string,
      databaseId: string,
      collection: string,
      query?: Record<string, unknown>
    ) => Promise<DbRow[]>
    insert: (
      projectId: string,
      databaseId: string,
      collection: string,
      payload: Record<string, unknown>
    ) => Promise<DbRow>
    update: (
      projectId: string,
      databaseId: string,
      collection: string,
      id: string,
      payload: Record<string, unknown>
    ) => Promise<DbRow>
    remove: (projectId: string, databaseId: string, collection: string, id: string) => Promise<void>
    collectionItems: (projectId: string, databaseId: string) => Promise<CollectionInfo[]>
    exportCollection: (projectId: string, databaseId: string, collection: string, format: 'csv' | 'ndjson' | 'json', query?: DocumentQuery) => Promise<Blob>
    importCollection: (projectId: string, databaseId: string, collection: string, file: File, format: 'csv' | 'ndjson', dryRun?: boolean) => Promise<CollectionImportResult>
    dryRunView: (projectId: string, databaseId: string, name: string, query: string) => Promise<ViewDryRunResult>
    documents: (
      projectId: string,
      databaseId: string,
      collection: string,
      query?: DocumentQuery
    ) => Promise<DocumentPage>
    document: (projectId: string, databaseId: string, collection: string, id: string) => Promise<DocumentItem>
    patch: (
      projectId: string,
      databaseId: string,
      collection: string,
      id: string,
      payload: Record<string, unknown>
    ) => Promise<DbRow>
    batch: (
      projectId: string,
      databaseId: string,
      collection: string,
      documents: Record<string, unknown>[]
    ) => Promise<DocumentItem[]>
    truncate: (projectId: string, databaseId: string, collection: string) => Promise<void>
    getCollectionSchema: (projectId: string, databaseId: string, collection: string) => Promise<CollectionSchema>
    putCollectionSchema: (
      projectId: string,
      databaseId: string,
      schema: CollectionSchemaInput,
      dryRun?: boolean
    ) => Promise<CollectionSchema | SchemaDryRunResult>
    deleteCollectionSchema: (projectId: string, databaseId: string, collection: string) => Promise<void>
  }
  /** Key-Value 数据服务：项目级单端点（key-value-ducklake-plan §3） */
  kv: {
    /** 单端点执行：body 为 {type:"cmd",argvs:[...]} 或 {type:"String|Hash|List|Set|ZSet",args:{...}}，
     *  返回该命令的 Redis 回复编码成 JSON（GET 缺失为 null）。 */
    exec: (projectId: string, body: KvExecBody) => Promise<unknown>
    /** 批量顺序执行多条命令（事务外逐条）；任一失败即停止，返回已完成的回复。 */
    execBatch: (projectId: string, bodies: KvExecBody[]) => Promise<unknown[]>
  }
  s3: {
    list: (projectId: string, prefix?: string) => Promise<S3Object[]>
    listDetailed: (projectId: string, prefix?: string) => Promise<S3ListResult>
    presign: (projectId: string, key: string) => Promise<{ url: string }>
    download: (projectId: string, key: string) => Promise<Blob>
    createLink: (projectId: string, key: string, ttlSeconds?: number) => Promise<S3Link>
    remove: (projectId: string, key: string) => Promise<void>
    upload: (
      projectId: string,
      key: string,
      file: File,
      onProgress?: (percent: number) => void
    ) => Promise<S3Object>
  }
  backups: {
    list: (projectId: string) => Promise<BackupItem[]>
    create: (projectId: string) => Promise<BackupItem>
    remove: (projectId: string, id: string) => Promise<void>
    restore: (projectId: string, id: string) => Promise<void>
    download: (projectId: string, id: string) => Promise<Blob>
  }
  logs: {
    list: (projectId: string, q?: LogQuery) => Promise<LogEvent[]>
    stats: (projectId: string, q?: LogQuery, groupBy?: 'level' | 'hour' | 'path') => Promise<LogStat[]>
    export: (projectId: string, q?: LogQuery) => Promise<Blob>
    getRetention: (projectId: string) => Promise<LogRetention>
    putRetention: (projectId: string, keepDays: number) => Promise<void>
  }
  quota: {
    status: (projectId: string) => Promise<QuotaStatus>
  }
  llm: {
    chat: (projectId: string, req: LlmChatRequest) => Promise<LlmChatResponse>
    stream: (projectId: string, req: LlmChatRequest, handlers: LlmStreamHandlers) => LlmStreamConnection
  }
  llmSettings: {
    get: (projectId: string) => Promise<LlmSettings>
    /** apiKey 留空表示沿用服务端既有值 */
    put: (projectId: string, settings: LlmSettings, apiKey?: string) => Promise<LlmSettings>
  }
  agents: {
    modules: (projectId: string) => Promise<AgentModuleInfo[]>
    list: (projectId: string) => Promise<CloudAgent[]>
    create: (projectId: string, body: Partial<CloudAgent>) => Promise<CloudAgent>
    patch: (projectId: string, agentId: string, body: Partial<CloudAgent>) => Promise<CloudAgent>
    remove: (projectId: string, agentId: string) => Promise<void>
    /** 可用模型列表（planv4.1 BUG-07）。 */
    models: (projectId: string) => Promise<AgentModelsResponse>
  }
  agentThreads: {
    list: (projectId: string, agentId?: string) => Promise<AgentThread[]>
    page: (projectId: string, agentId?: string, limit?: number, cursor?: string) => Promise<AgentThreadPage>
    create: (projectId: string, title?: string, agentId?: string) => Promise<AgentThread>
    rename: (projectId: string, threadId: string, title: string) => Promise<AgentThread>
    remove: (projectId: string, threadId: string) => Promise<void>
    runs: (projectId: string, threadId: string, limit?: number) => Promise<AgentRun[]>
    messages: (projectId: string, threadId: string) => Promise<AgentMessage[]>
    streamRun: (
      projectId: string,
      threadId: string,
      req: AgentRunRequest,
      handlers: AgentStreamHandlers
    ) => LlmStreamConnection
    /** 续接进行中的 run（回放事件缓存并跟随到 end）。 */
    attachRun: (projectId: string, runId: string, handlers: AgentStreamHandlers) => LlmStreamConnection
    cancel: (projectId: string, runId: string) => Promise<AgentRun>
    /** 项目内正在执行的 run。 */
    active: (projectId: string) => Promise<ActiveAgentRun[]>
    confirm: (projectId: string, runId: string, callId: string, approve: boolean) => Promise<void>
  }
  agentSchedules: {
    list: (projectId: string) => Promise<AgentSchedule[]>
    create: (projectId: string, body: AgentScheduleBody) => Promise<AgentSchedule>
    patch: (projectId: string, scheduleId: string, body: Partial<AgentScheduleBody>) => Promise<AgentSchedule>
    remove: (projectId: string, scheduleId: string) => Promise<void>
    runs: (projectId: string, scheduleId: string) => Promise<AgentScheduleRun[]>
    trigger: (projectId: string, scheduleId: string) => Promise<void>
  }
  /** 系统级存储设置（planv6.0 storage-local-first 阶段 E）。 */
  storage: {
    get: () => Promise<StorageSettings>
    /** confirmReset 必须为 true：服务端要求显式确认（修改后需 reset 清数据）。 */
    put: (body: { database?: string; files?: string; data_dir?: string; confirm_reset: boolean }) => Promise<StorageUpdateResult>
  }
}

/** GET /v1/system/storage 响应。 */
export interface StorageSettings {
  database: string
  files: string
  data_dir: string
  cache_dir: string
  bucket: string
  prefix: string
  /** 字段来源：env | yaml | default；env 时 UI 禁用对应输入。 */
  source: Record<string, string>
  config_path: string
  writable_config: boolean
  /** 本地/远端是否已有数据（提示需要 reset）。 */
  data_present: Record<string, boolean>
}

/** PUT /v1/system/storage 响应。 */
export interface StorageUpdateResult {
  settings: StorageSettings
  restart_required: boolean
  reset_required: boolean
  commands?: string[]
}
