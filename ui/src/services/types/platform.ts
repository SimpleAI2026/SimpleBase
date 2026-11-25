// 平台域契约：项目 / 用户认证 / S3 / 日志 / 指标 / LLM 设置 / 云函数 / 定时任务 / 沙盒。

/**
 * 请求失败错误：在 message 之外保留 HTTP status 与后端业务 code。
 *
 * `http.ts` 的拦截器此前只 reject `new Error(message)`，status 和 code 全部丢失，
 * 调用方无法区分「资源未建立（应当展示为空）」与「真正的失败」。
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

export interface S3Object {
  key: string
  size: number
  /** 注意：S3 域是后端 snake_case 命名的例外 */
  lastModified?: string
  /** 是否允许外部访问（含继承自文件夹） */
  public?: boolean
  /** 站点已发布且路径公开时的访问 URL */
  publicUrl?: string
  /** 继承来源文件夹（"/" 表示整个存储桶）；空表示自身开关 */
  publicInherited?: string
}

/** 文件夹的外部访问状态 */
export interface S3FolderMeta {
  key: string
  public: boolean
  publicUrl?: string
  publicInherited?: string
}

/** 项目站点（发布网站）状态 */
export interface S3SiteInfo {
  baseUrl?: string
  publishable: boolean
  /** 不可发布原因：site_unavailable / site_domain_unconfigured / system_project / project_id_not_dns_safe */
  reason?: string
  /** 访问方式：path 为默认，subdomain 需显式开启；前端不读环境变量，以此为准 */
  mode?: 'path' | 'subdomain'
}

/** PUT /s3/public 的结果 */
export interface S3PublicResult {
  public: boolean
  publicUrl?: string
  publicInherited?: string
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

/** 一次模型调用或工具执行的 span 观测记录（服务端 sys_llm_traces）。 */
export interface LLMTrace {
  id: string
  projectId: string
  requestId: string
  source: string
  provider: string
  model: string
  status: 'ok' | 'error' | string
  stream: boolean
  durationMs: number
  inputTokens: number
  outputTokens: number
  errorCode?: string
  errorMessage?: string
  /** 二期：顶层 trace 标识（agent=run_id；API 直调=request_id） */
  traceId: string
  /** 来源："api" | "agent" | "cron" */
  caller: string
  /** span 类型："llm" | "tool" */
  spanKind: string
  /** llm: 模型名；tool: 工具名 */
  name: string
  /** llm: 请求消息快照 JSON；tool: arguments JSON */
  inputJson?: string
  /** llm: {content, tool_calls[]}；tool: 结果文本 */
  outputJson?: string
  /** 仅 llm span：声明的工具 [{name, description}] */
  toolsJson?: string
  createdAt: string
}

/** 顶层调用聚合视图（列表行）。 */
export interface LLMTraceGroup {
  traceId: string
  caller: string
  model: string
  provider: string
  status: 'ok' | 'error' | string
  stream: boolean
  startedAt: string
  finishedAt: string
  spanCount: number
  toolCount: number
  llmCount: number
  inputTokens: number
  outputTokens: number
  durationMs: number
  errorCode?: string
  requestId: string
}

/** 分组列表查询条件。 */
export interface LlmTraceGroupQuery {
  caller?: 'api' | 'agent' | 'cron'
  status?: 'ok' | 'error'
  model?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
}

export interface LLMTraceGroupListResult {
  groups: LLMTraceGroup[]
  total: number
}

export interface ProjectItem {
  id: string
  name: string
  /** 项目单文件数据库引擎；物理路径不对外暴露 */
  engine?: 'duckdb' | 'sqlite'
  createdAt: string
  /** admin 管理数据库（系统项目）标记，super/admin 可见 */
  managed?: boolean
}

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
  /** 与 folders 一一对应的外部访问状态；站点功能未开启时为空 */
  folderMeta: S3FolderMeta[]
  /** 站点状态；站点功能未开启时为 undefined */
  site?: S3SiteInfo
}

export interface S3Link {
  url: string
  expiresAt: string
}

export interface LogStat {
  key: string
  count: number
}

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

/** 强制改密（must_change_password）的免认证改密请求。 */
export interface InitialPasswordRequest {
  username: string
  oldPassword: string
  newPassword: string
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

export interface QuotaStatus {
  llmAllowed: boolean
  databaseAllowed: boolean
}

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

/** API 预览：参数取值来源（后端 route_meta 的 x-sb-source 扩展）。 */
export interface OpenAPIParamSource {
  kind: string
  depends_on?: string[]
}

export interface OpenAPIParameter {
  name: string
  in: 'path' | 'query'
  required: boolean
  'x-sb-source'?: OpenAPIParamSource
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
