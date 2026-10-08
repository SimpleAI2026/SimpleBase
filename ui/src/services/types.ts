/** 统一的后端接口类型定义与 API 抽象（契约对照 plan/planv2.0/proto-http.md） */

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

export interface DbRow {
  id: string
  [key: string]: unknown
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

export interface LlmSettings {
  defaultProvider?: string
  defaultModel?: string
  temperature?: number
  maxTokens?: number
}

/* ---------- Projects ---------- */

export interface ProjectItem {
  id: string
  name: string
  createdAt: string
  /** admin 管理数据库（系统项目）标记，super/admin 可见 */
  managed?: boolean
}

/* ---------- 项目 API Keys ---------- */

/** API Key 条目（列表返回；不含明文） */
export interface ApiKeyItem {
  id: string
  projectId: string
  permissions: string[]
  createdAt: string
  revokedAt?: string
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
  role: 'system' | 'user' | 'assistant' | string
  content: string
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

/* ---------- 云 Agent（proto-http.md §3.12） ---------- */

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
  created_at: string
  updated_at: string
}

export interface AgentThread {
  id: string
  title: string
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

export interface AgentRun extends Partial<AgentRunMetrics> {
  id: string
  thread_id: string
  agent_id: string
  status: string
  error?: string
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
  onToolResult?: (name: string, content: string, callId?: string, durationMs?: number) => void
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

/** 定时任务资源。目标为云函数导出函数；时间一律 UTC */
export interface CronJobItem {
  id: string
  name: string
  description: string
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

export interface CronJobCreate {
  name: string
  description: string
  scheduleKind: CronScheduleKind
  cronExpr: string
  intervalSeconds?: number
  /** scheduleKind=once 时必填 */
  runAt?: string
  funcFile: string
  funcExport: string
  inputJson: string
  enabled?: boolean
}

/* ---------- 云沙盒（planv4.0 cloud-sandbox-plan） ---------- */

export interface SandboxCapabilities {
  available: boolean
  backend: string
  images: string[]
  defaultImage: string
  cpusMax: number
  memoryMiBMax: number
  execTimeoutMaxS: number
  maxFileBytes: number
  maxOutputBytes: number
  maxPerProject: number
  networkOptions: string[]
}
export interface SandboxItem {
  id: string
  name: string
  cloudName: string
  source: string
  threadId?: string
  status: string
  image: string
  cpus: number
  memoryMiB: number
  network: string
  idleTimeoutS: number
  maxDurationS: number
  lastError?: string
  createdAt: string
  startedAt?: string
  lastActiveAt?: string
  expiresAt?: string
}
export interface SandboxCreate {
  name?: string
  image?: string
  cpus?: number
  memoryMiB?: number
  network?: string
  idleTimeoutS?: number
  start?: boolean
}
export interface SandboxExecInput {
  command: string
  timeoutS?: number
}
export interface SandboxExecResult {
  exitCode: number
  stdout: string
  stderr: string
  stdoutTruncated: boolean
  stderrTruncated: boolean
  timedOut: boolean
  durationMs: number
  status: string
}
export interface SandboxFileEntry {
  name: string
  path: string
  kind: string
  size: number
}
export interface SandboxFileContent {
  content: string
  encoding: string
  truncated: boolean
}

/**
 * API 统一抽象：http 实现与 mock 实现均遵循该接口。
 * projectId 一律为方法首个参数，由调用方从 stores/project.ts 读取后显式传入
 * （services 层不 import store，保持无状态可测）。
 */
export interface Api {
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
    list: (projectId: string, status?: string, source?: string) => Promise<SandboxItem[]>
    create: (projectId: string, body: SandboxCreate, idempotencyKey?: string) => Promise<SandboxItem>
    remove: (projectId: string, id: string) => Promise<void>
    start: (projectId: string, id: string) => Promise<SandboxItem>
    stop: (projectId: string, id: string) => Promise<SandboxItem>
    exec: (projectId: string, id: string, body: SandboxExecInput) => Promise<SandboxExecResult>
    files: {
      list: (projectId: string, id: string, path: string) => Promise<SandboxFileEntry[]>
      read: (projectId: string, id: string, path: string) => Promise<SandboxFileContent>
      download: (projectId: string, id: string, path: string) => Promise<Blob>
      write: (projectId: string, id: string, path: string, content: string) => Promise<void>
      upload: (projectId: string, id: string, path: string, content: Uint8Array) => Promise<void>
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
    create: (req: { name: string; id?: string; ownerUserId?: string }) => Promise<ProjectItem>
  }
  /** 项目 API Key 管理（创建/列表/吊销；明文仅创建响应一次性返回） */
  apiKeys: {
    list: (projectId: string) => Promise<ApiKeyItem[]>
    create: (projectId: string, permissions?: string[]) => Promise<ApiKeyItem & { secret: string }>
    revoke: (projectId: string, keyId: string) => Promise<void>
  }
  metrics: {
    summary: (projectId: string) => Promise<MetricsSummary>
    trend: (projectId: string) => Promise<TrendPoint[]>
  }
  databases: {
    list: (projectId: string) => Promise<DatabaseItem[]>
    create: (projectId: string, name: string) => Promise<DatabaseItem>
    remove: (projectId: string, databaseId: string) => Promise<void>
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
    presign: (projectId: string, key: string) => Promise<{ url: string }>
    remove: (projectId: string, key: string) => Promise<void>
    upload: (
      projectId: string,
      key: string,
      file: File,
      onProgress?: (percent: number) => void
    ) => Promise<S3Object>
  }
  logs: {
    list: (projectId: string, q?: LogQuery) => Promise<LogEvent[]>
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
    put: (projectId: string, settings: LlmSettings) => Promise<LlmSettings>
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
    list: (projectId: string) => Promise<AgentThread[]>
    page: (projectId: string, limit?: number, cursor?: string) => Promise<AgentThreadPage>
    create: (projectId: string, title?: string) => Promise<AgentThread>
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
    cancel: (projectId: string, runId: string) => Promise<AgentRun>
  }
  agentSchedules: {
    list: (projectId: string) => Promise<AgentSchedule[]>
    create: (projectId: string, body: AgentScheduleBody) => Promise<AgentSchedule>
    patch: (projectId: string, scheduleId: string, body: Partial<AgentScheduleBody>) => Promise<AgentSchedule>
    remove: (projectId: string, scheduleId: string) => Promise<void>
    runs: (projectId: string, scheduleId: string) => Promise<AgentScheduleRun[]>
    trigger: (projectId: string, scheduleId: string) => Promise<void>
  }
}
