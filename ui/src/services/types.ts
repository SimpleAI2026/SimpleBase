/** 统一的后端接口类型定义与 API 抽象（契约对照 plan/planv2.0/proto-http.md） */

export interface MetricsSummary {
  totalRequests: number
  errorRate: number
  avgLatencyMs: number
  activeDatabases: number
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
}

/* ---------- Databases（proto-http.md §3.1） ---------- */

/** 数据库资源。status 枚举共 9 值，新建时为 creating（不是 active） */
export interface DatabaseItem {
  id: string
  name: string
  status:
    | 'creating'
    | 'opening'
    | 'ready'
    | 'closing'
    | 'closed'
    | 'degraded'
    | 'deleting'
    | 'deleted'
    | 'recovering'
  createdAt: string
  updatedAt: string
  /** 仅详情接口可能返回（服务端注入 SnapshotFor 时） */
  snapshot?: { lastSyncedSnapshot: number; syncLag: number }
}

export interface DatabaseListResult {
  databases: DatabaseItem[]
  nextCursor: string
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
}

export interface CloudAgent {
  id: string
  name: string
  module: string
  description: string
  system_prompt: string
  tool_ids: string[]
  model_override?: string
  team_enabled: boolean
  created_at: string
  updated_at: string
}

export interface AgentThread {
  id: string
  title: string
  created_at: string
  updated_at: string
}

export interface AgentMention {
  agent_id: string
}

export interface AgentToolCallCard {
  name?: string
  content?: string
  arguments?: string
}

export interface AgentMessage {
  id: string
  role: string
  content: string
  agent_id?: string
  mentions?: AgentMention[]
  tool_calls?: AgentToolCallCard[]
  run_id?: string
  created_at: string
}

export interface AgentRun {
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
}

export interface AgentStreamHandlers {
  onRun?: (runId: string) => void
  onToken?: (text: string) => void
  onToolCall?: (name: string, args: string) => void
  onToolResult?: (name: string, content: string) => void
  onEnd?: () => void
  onError?: (e: unknown) => void
}

/* ---------- GoFunctions（proto-http.md §3.13） ---------- */

/** 云函数资源。file 由服务端派生 = name + ".go" */
export interface GoFunctionItem {
  id: string
  name: string
  file: string
  /** 列表接口省略；详情 / 创建 / 更新返回完整源码 */
  source?: string
  exports: string[]
  createdAt: string
  updatedAt: string
}

export interface GoFunctionCreate {
  name: string
  source: string
}

/* ---------- CronJobs（proto-http.md §3.14） ---------- */

/** 定时任务调度模式：cron 定时执行 / interval 固定间隔 */
export type CronScheduleKind = 'cron' | 'interval'

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
  funcFile: string
  funcExport: string
  inputJson: string
  enabled?: boolean
}

/**
 * API 统一抽象：http 实现与 mock 实现均遵循该接口。
 * projectId 一律为方法首个参数，由调用方从 stores/project.ts 读取后显式传入
 * （services 层不 import store，保持无状态可测）。
 */
export interface Api {
  gofunctions: {
    list: (projectId: string) => Promise<GoFunctionItem[]>
    create: (projectId: string, body: GoFunctionCreate) => Promise<GoFunctionItem>
    get: (projectId: string, name: string) => Promise<GoFunctionItem>
    update: (projectId: string, name: string, source: string) => Promise<GoFunctionItem>
    remove: (projectId: string, name: string) => Promise<void>
  }
  cronjobs: {
    list: (projectId: string) => Promise<CronJobItem[]>
    create: (projectId: string, body: CronJobCreate) => Promise<CronJobItem>
    get: (projectId: string, jobId: string) => Promise<CronJobItem>
    /** PATCH：name 不可改；调度变更后服务端重算 nextRunAt */
    update: (projectId: string, jobId: string, body: Partial<CronJobCreate>) => Promise<CronJobItem>
    remove: (projectId: string, jobId: string) => Promise<void>
    runs: (projectId: string, jobId: string, limit?: number) => Promise<CronJobRunItem[]>
    /** 手动立即执行（异步 202；不改排期） */
    trigger: (projectId: string, jobId: string) => Promise<void>
  }
  projects: {
    list: () => Promise<ProjectItem[]>
    create: (req: { name: string; id?: string }) => Promise<ProjectItem>
  }
  metrics: {
    summary: (projectId: string) => Promise<MetricsSummary>
    trend: (projectId: string) => Promise<TrendPoint[]>
  }
  databases: {
    list: (projectId: string) => Promise<DatabaseItem[]>
    create: (projectId: string, name: string) => Promise<DatabaseItem>
    get: (projectId: string, databaseId: string) => Promise<DatabaseItem>
    open: (projectId: string, databaseId: string) => Promise<DatabaseItem>
    close: (projectId: string, databaseId: string) => Promise<void>
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
  s3: {
    list: (projectId: string, prefix?: string) => Promise<S3Object[]>
    presign: (projectId: string, key: string) => Promise<{ url: string }>
    remove: (projectId: string, key: string) => Promise<void>
    upload: (projectId: string, key: string, file: File) => Promise<S3Object>
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
    providers: (projectId: string) => Promise<string[]>
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
    get: (projectId: string, agentId: string) => Promise<CloudAgent>
    patch: (projectId: string, agentId: string, body: Partial<CloudAgent>) => Promise<CloudAgent>
    remove: (projectId: string, agentId: string) => Promise<void>
  }
  agentThreads: {
    list: (projectId: string) => Promise<AgentThread[]>
    create: (projectId: string, title?: string) => Promise<AgentThread>
    get: (projectId: string, threadId: string) => Promise<AgentThread>
    remove: (projectId: string, threadId: string) => Promise<void>
    messages: (projectId: string, threadId: string) => Promise<AgentMessage[]>
    run: (projectId: string, threadId: string, req: AgentRunRequest) => Promise<{ run: AgentRun; message: AgentMessage }>
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
