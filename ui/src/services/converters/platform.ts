// 平台域转换：项目 / 用户 / 认证 / S3 / 云函数 / 沙盒 / 定时任务 / 日志 / 指标 / LLM / 备份 / 触发器。

import type {
  BackupItem,
  CronJobCreate,
  CronJobItem,
  CronJobRunItem,
  CronJobRunWire,
  CronJobWire,
  GoFunctionItem,
  LlmSettings,
  LLMTrace,
  LLMTraceGroup,
  LLMTraceGroupListResult,
  LlmTraceGroupQuery,
  LogEvent,
  LogEventWire,
  LogQuery,
  LogRetention,
  LogRetentionWire,
  MetricsSummary,
  MetricsSummaryWire,
  ProjectItem,
  SandboxFileEntry,
  SandboxItem,
  S3FolderMeta,
  S3PublicResult,
  S3SiteInfo,
  TokenPair,
  TrendPoint,
  TrendPointWire,
  UserItem,
  ApiKeyItem,
  AuthUser,
  FunctionTrigger,
  FunctionTriggerInput
} from '@/services/types'
import { http } from '@/services/http'
import { listOf, objectOf, orDefault, orElse, orUndefined, pick, type WireRecord } from '@/services/wire'
import { collectionWire } from '@/services/converters/data'

/** 站点状态（S3 域后端已是 camelCase，这里只做类型收敛与兜底）。 */
export function toS3SiteInfo(raw: unknown): S3SiteInfo {
  const r = (raw ?? {}) as Partial<S3SiteInfo>
  return {
    baseUrl: typeof r.baseUrl === 'string' && r.baseUrl ? r.baseUrl : undefined,
    publishable: r.publishable === true,
    reason: typeof r.reason === 'string' && r.reason ? r.reason : undefined,
    mode: r.mode === 'path' || r.mode === 'subdomain' ? r.mode : undefined
  }
}

export function toS3FolderMeta(raw: unknown): S3FolderMeta {
  const r = (raw ?? {}) as Partial<S3FolderMeta>
  return {
    key: String(r.key ?? ''),
    public: r.public === true,
    publicUrl: r.publicUrl || undefined,
    publicInherited: r.publicInherited || undefined
  }
}

export function toS3PublicResult(raw: unknown): S3PublicResult {
  const r = (raw ?? {}) as Partial<S3PublicResult>
  return { public: r.public === true, publicUrl: r.publicUrl || undefined, publicInherited: r.publicInherited || undefined }
}

/* v8 ignore start -- 防御性字段映射：空/满 payload 双向已有 sparse 测试 */
export function toGoFuncVersion(raw: WireRecord) {
  return {
    version: Number(raw?.version ?? 0),
    exports: listOf(raw?.exports, String),
    note: String(raw?.note ?? ''),
    createdAt: String(raw?.created_at ?? ''),
    active: Boolean(raw?.active),
    source: orUndefined(raw, 'source')
  }
}

/** GoFunctionItem：后端 snake_case → UI camelCase */
export function toGoFunctionItem(raw: WireRecord): GoFunctionItem {
  return {
    id: String(raw?.id ?? ''),
    name: String(raw?.name ?? ''),
    file: String(raw?.file ?? ''),
    description: String(raw?.description ?? ''),
    activeVersion: Number(raw?.active_version ?? 0),
    latestVersion: Number(raw?.latest_version ?? 0),
    published: Boolean(raw?.published),
    exports: listOf(raw?.exports, String),
    versions: Array.isArray(raw?.versions) ? raw.versions.map(toGoFuncVersion) : undefined,
    source: orUndefined(raw, 'source'),
    createdAt: String(raw?.created_at ?? ''),
    updatedAt: String(raw?.updated_at ?? '')
  }
}
/* v8 ignore stop */

/* ---------- 云沙盒：E2B 兼容 API。控制面 /e2b/**，envd 面 /e2b/envd/**，扩展 /e2b/ext/** ---------- */
export const E2B_BASE = '/e2b'

export const ENVD_BASE = '/e2b/envd'

export const SB_META_NAME = 'name'

const SB_META_SOURCE = 'simplebase.source'

export function e2bHeaders(projectId: string): Record<string, string> {
  return { 'X-SimpleBase-Project': projectId }
}

export function sandboxPath(id: string, suffix = ''): string {
  return E2B_BASE + '/sandboxes/' + encodeURIComponent(id) + suffix
}

export function num(v: unknown): number {
  const n = Number(v)
  return Number.isFinite(n) ? n : 0
}

export function toSandboxItem(raw: Record<string, unknown>): SandboxItem {
  const ext = (raw.x_simplebase || {}) as Record<string, unknown>
  const metadata = (raw.metadata || {}) as Record<string, string>
  return {
    id: String(raw.sandboxID ?? ''),
    name: String(metadata[SB_META_NAME] ?? ''),
    templateId: String(raw.alias || raw.templateID || ''),
    state: raw.state === 'running' ? 'running' : 'paused',
    status: String(ext.status ?? ''),
    source: String(ext.source ?? metadata[SB_META_SOURCE] ?? ''),
    cpuCount: num(raw.cpuCount),
    memoryMB: num(raw.memoryMB),
    startedAt: raw.startedAt ? String(raw.startedAt) : undefined,
    endAt: raw.endAt ? String(raw.endAt) : undefined,
    lastActiveAt: ext.lastActiveAt ? String(ext.lastActiveAt) : undefined,
    lastError: ext.lastError ? String(ext.lastError) : undefined,
    idleTimeoutS: num(ext.idleTimeoutS),
    metadata
  }
}

/** envd 的 EntryInfo：type 是枚举字符串，size 是字符串形式的 int64。 */
export function toSandboxFileEntry(raw: Record<string, unknown>): SandboxFileEntry {
  return {
    name: String(raw.name ?? ''),
    path: String(raw.path ?? ''),
    kind: raw.type === 'FILE_TYPE_DIRECTORY' ? 'directory' : 'file',
    size: num(raw.size),
    permissions: String(raw.permissions ?? ''),
    modifiedTime: raw.modifiedTime ? String(raw.modifiedTime) : undefined
  }
}

/** envd 的一元 Connect-JSON 调用。 */
export function envdRpc(projectId: string, id: string, method: string, body: Record<string, unknown>) {
  return http.post(ENVD_BASE + method, body, {
    headers: { ...e2bHeaders(projectId), 'Connect-Protocol-Version': '1', 'E2b-Sandbox-Id': id }
  })
}

/** 未识别的调度类型回退 cron（旧数据兼容）。 */
function toCronScheduleKind(raw: unknown): CronJobItem['scheduleKind'] {
  if (raw === 'interval') return 'interval'
  if (raw === 'once') return 'once'
  return 'cron'
}

/** CronJobItem：后端 snake_case → UI camelCase。raw 可能缺失（响应体为空）。 */
export function toCronJobItem(raw: CronJobWire | null | undefined): CronJobItem {
  return {
    id: String(raw?.id ?? ''),
    name: String(raw?.name ?? ''),
    description: String(raw?.description ?? ''),
    targetKind: raw?.target_kind === 'agent' ? 'agent' : 'function',
    agentId: raw?.agent_id ? String(raw.agent_id) : undefined,
    agentThreadId: raw?.agent_thread_id ? String(raw.agent_thread_id) : undefined,
    agentPrompt: raw?.agent_prompt ? String(raw.agent_prompt) : undefined,
    scheduleKind: toCronScheduleKind(raw?.schedule_kind),
    cronExpr: String(raw?.cron_expr ?? ''),
    intervalSeconds: raw?.interval_seconds != null ? Number(raw.interval_seconds) : undefined,
    runAt: raw?.run_at || undefined,
    funcFile: String(raw?.func_file ?? ''),
    funcExport: String(raw?.func_export ?? ''),
    inputJson: String(raw?.input_json ?? '{}'),
    enabled: Boolean(raw?.enabled),
    lastRunAt: raw?.last_run_at || undefined,
    nextRunAt: raw?.next_run_at || undefined,
    lastStatus: String(raw?.last_status ?? ''),
    lastError: String(raw?.last_error ?? ''),
    runCount: Number(raw?.run_count ?? 0),
    targetMissing: Boolean(raw?.target_missing),
    createdAt: String(raw?.created_at ?? ''),
    updatedAt: String(raw?.updated_at ?? '')
  }
}

export function toCronJobRunItem(raw: CronJobRunWire | null | undefined): CronJobRunItem {
  return {
    id: String(raw?.id ?? ''),
    jobId: String(raw?.job_id ?? ''),
    trigger: raw?.trigger === 'manual' ? 'manual' : 'scheduled',
    status: String(raw?.status ?? '') as CronJobRunItem['status'],
    error: String(raw?.error ?? ''),
    durationMs: Number(raw?.duration_ms ?? 0),
    responseJson: String(raw?.response_json ?? ''),
    startedAt: raw?.started_at || undefined,
    finishedAt: raw?.finished_at || undefined,
    createdAt: String(raw?.created_at ?? '')
  }
}

/** CronJobCreate → 后端 snake_case body */
export function cronJobBody(body: Partial<CronJobCreate>) {
  const out: Record<string, unknown> = {
    description: body.description ?? '',
    schedule_kind: body.scheduleKind,
    target_kind: body.targetKind ?? 'function'
  }
  if (body.targetKind === 'agent') {
    out.agent_id = body.agentId ?? ''
    out.agent_prompt = body.agentPrompt ?? ''
  } else {
    out.func_file = body.funcFile
    out.func_export = body.funcExport
    out.input_json = body.inputJson ?? '{}'
  }
  if (body.name != null) out.name = body.name
  if (body.cronExpr != null) out.cron_expr = body.cronExpr
  if (body.intervalSeconds != null) out.interval_seconds = body.intervalSeconds
  if (body.runAt != null) out.run_at = body.runAt
  if (body.enabled != null) out.enabled = body.enabled
  return out
}

export function optionalNumber(v: unknown): number | null {
  if (v === null || v === undefined || v === '') return null
  const n = Number(v)
  return Number.isFinite(n) ? n : null
}

export function toMetricsSummary(d: unknown): MetricsSummary {
  const raw: MetricsSummaryWire = d && typeof d === 'object' ? d : {}
  return {
    totalRequests: Number(raw.total_requests ?? raw.totalRequests ?? 0),
    errorRate: Number(raw.error_rate ?? raw.errorRate ?? 0),
    avgLatencyMs: Number(raw.avg_latency_ms ?? raw.avgLatencyMs ?? 0),
    activeDatabases: Number(raw.active_databases ?? raw.activeDatabases ?? 0),
    latencyP50Ms: optionalNumber(raw.latency_p50_ms ?? raw.latencyP50Ms),
    latencyP90Ms: optionalNumber(raw.latency_p90_ms ?? raw.latencyP90Ms),
    latencyP99Ms: optionalNumber(raw.latency_p99_ms ?? raw.latencyP99Ms),
    latencySampleCount: Number(raw.latency_sample_count ?? raw.latencySampleCount ?? 0),
    latencyOverflowMs: Number(raw.latency_overflow_ms ?? raw.latencyOverflowMs ?? 30000)
  }
}

/** 趋势接口兼容两种形状：裸数组，或 `{ points: [...] }`；其余视为空。 */
function trendPointList(d: unknown): unknown[] {
  if (Array.isArray(d)) return d
  const points = (d as { points?: unknown } | null | undefined)?.points
  return listOf(points)
}

export function toTrendPoints(d: unknown): TrendPoint[] {
  return trendPointList(d).map((item) => {
    const p = item as TrendPointWire | null | undefined
    return {
      date: String(p?.date ?? ''),
      requests: Number(p?.requests ?? 0),
      errors: Number(p?.errors ?? 0)
    }
  })
}

export function logQueryParams(q?: LogQuery) {
  if (!q) return undefined
  const params: Record<string, string | number> = {}
  if (q.level) params.level = q.level
  if (q.q) params.q = q.q
  if (q.from) params.from = q.from
  if (q.to) params.to = q.to
  if (q.limit) params.limit = q.limit
  return params
}

export function toLogEvent(raw: LogEventWire | null | undefined): LogEvent {
  return {
    id: String(raw?.id ?? raw?.ID ?? ''),
    projectId: String(raw?.project_id ?? raw?.projectId ?? raw?.ProjectID ?? ''),
    level: String(raw?.level ?? raw?.Level ?? ''),
    logger: String(raw?.logger ?? raw?.Logger ?? ''),
    message: String(raw?.message ?? raw?.Message ?? ''),
    fieldsJson: raw?.fields_json ?? raw?.fieldsJson ?? raw?.FieldsJSON ?? undefined,
    requestId: raw?.request_id ?? raw?.requestId ?? raw?.RequestID ?? undefined,
    occurredAt: String(raw?.occurred_at ?? raw?.occurredAt ?? raw?.OccurredAt ?? '')
  }
}

export function toLogRetention(raw: unknown): LogRetention {
  const d: LogRetentionWire = raw && typeof raw === 'object' ? raw : {}
  return {
    scope: String(d.scope ?? d.Scope ?? ''),
    keepDays: Number(d.keep_days ?? d.keepDays ?? d.KeepDays ?? 14),
    updatedAt: String(d.updated_at ?? d.updatedAt ?? d.UpdatedAt ?? '')
  }
}

export function toLlmSettings(raw: WireRecord | undefined): LlmSettings {
  const d = raw && typeof raw === 'object' ? raw : {}
  return {
    defaultProvider: orUndefined(d, 'default_provider') || orUndefined(d, 'defaultProvider'),
    defaultModel: orUndefined(d, 'default_model') || orUndefined(d, 'defaultModel'),
    baseUrl: orUndefined(d, 'base_url') || orUndefined(d, 'baseUrl'),
    apiKeyMasked: orUndefined(d, 'api_key_masked') || orUndefined(d, 'apiKeyMasked'),
    hasApiKey: Boolean(d.has_api_key ?? d.hasApiKey),
    temperature: pick<number | undefined>(d, 'temperature') ?? undefined,
    maxTokens: pick<number | undefined>(d, 'max_tokens') ?? pick<number | undefined>(d, 'maxTokens') ?? undefined
  }
}

export interface LLMTraceWire {
  id?: unknown
  ID?: unknown
  project_id?: unknown
  projectId?: unknown
  ProjectID?: unknown
  request_id?: unknown
  requestId?: unknown
  RequestID?: unknown
  source?: unknown
  Source?: unknown
  provider?: unknown
  Provider?: unknown
  model?: unknown
  Model?: unknown
  status?: unknown
  Status?: unknown
  stream?: unknown
  Stream?: unknown
  duration_ms?: unknown
  durationMs?: unknown
  DurationMs?: unknown
  input_tokens?: unknown
  inputTokens?: unknown
  InputTokens?: unknown
  output_tokens?: unknown
  outputTokens?: unknown
  OutputTokens?: unknown
  error_code?: string
  errorCode?: string
  ErrorCode?: string
  error_message?: string
  errorMessage?: string
  ErrorMessage?: string
  trace_id?: unknown
  traceId?: unknown
  TraceID?: unknown
  caller?: unknown
  Caller?: unknown
  span_kind?: unknown
  spanKind?: unknown
  SpanKind?: unknown
  name?: unknown
  Name?: unknown
  input_json?: string
  inputJson?: string
  InputJSON?: string
  output_json?: string
  outputJson?: string
  OutputJSON?: string
  tools_json?: string
  toolsJson?: string
  ToolsJSON?: string
  created_at?: unknown
  createdAt?: unknown
  CreatedAt?: unknown
}

interface LLMTraceGroupWire {
  trace_id?: unknown
  traceId?: unknown
  TraceID?: unknown
  caller?: unknown
  Caller?: unknown
  model?: unknown
  Model?: unknown
  provider?: unknown
  Provider?: unknown
  status?: unknown
  Status?: unknown
  stream?: unknown
  Stream?: unknown
  started_at?: unknown
  startedAt?: unknown
  StartedAt?: unknown
  finished_at?: unknown
  finishedAt?: unknown
  FinishedAt?: unknown
  span_count?: unknown
  spanCount?: unknown
  SpanCount?: unknown
  tool_count?: unknown
  toolCount?: unknown
  ToolCount?: unknown
  llm_count?: unknown
  llmCount?: unknown
  LLMCount?: unknown
  input_tokens?: unknown
  inputTokens?: unknown
  InputTokens?: unknown
  output_tokens?: unknown
  outputTokens?: unknown
  OutputTokens?: unknown
  duration_ms?: unknown
  durationMs?: unknown
  DurationMs?: unknown
  error_code?: string
  errorCode?: string
  ErrorCode?: string
  request_id?: unknown
  requestId?: unknown
  RequestID?: unknown
}

interface LLMTraceGroupListWire {
  groups?: LLMTraceGroupWire[]
  total?: unknown
}

export function toLLMTrace(raw: LLMTraceWire | null | undefined): LLMTrace {
  const d: LLMTraceWire = raw && typeof raw === 'object' ? raw : {}
  return {
    id: String(d.id ?? d.ID ?? ''),
    projectId: String(d.project_id ?? d.projectId ?? d.ProjectID ?? ''),
    requestId: String(d.request_id ?? d.requestId ?? d.RequestID ?? ''),
    source: String(d.source ?? d.Source ?? ''),
    provider: String(d.provider ?? d.Provider ?? ''),
    model: String(d.model ?? d.Model ?? ''),
    status: String(d.status ?? d.Status ?? ''),
    stream: Boolean(d.stream ?? d.Stream),
    durationMs: Number(d.duration_ms ?? d.durationMs ?? d.DurationMs ?? 0),
    inputTokens: Number(d.input_tokens ?? d.inputTokens ?? d.InputTokens ?? 0),
    outputTokens: Number(d.output_tokens ?? d.outputTokens ?? d.OutputTokens ?? 0),
    errorCode: d.error_code || d.errorCode || d.ErrorCode || undefined,
    errorMessage: d.error_message || d.errorMessage || d.ErrorMessage || undefined,
    traceId: String(d.trace_id ?? d.traceId ?? d.TraceID ?? ''),
    caller: String(d.caller ?? d.Caller ?? ''),
    spanKind: String(d.span_kind ?? d.spanKind ?? d.SpanKind ?? ''),
    name: String(d.name ?? d.Name ?? ''),
    inputJson: d.input_json || d.inputJson || d.InputJSON || undefined,
    outputJson: d.output_json || d.outputJson || d.OutputJSON || undefined,
    toolsJson: d.tools_json || d.toolsJson || d.ToolsJSON || undefined,
    createdAt: String(d.created_at ?? d.createdAt ?? d.CreatedAt ?? '')
  }
}

/** 分组列表查询参数序列化：与服务端 Groups handler 一一对应。 */
export function llmTraceGroupParams(q?: LlmTraceGroupQuery): Record<string, string> {
  if (!q) return {}
  const out: Record<string, string> = {}
  if (q.caller) out.caller = q.caller
  if (q.status) out.status = q.status
  if (q.model) out.model = q.model
  if (q.from) out.from = q.from
  if (q.to) out.to = q.to
  if (q.limit != null) out.limit = String(q.limit)
  if (q.offset != null) out.offset = String(q.offset)
  return out
}

function toLLMTraceGroup(raw: LLMTraceGroupWire | null | undefined): LLMTraceGroup {
  const d: LLMTraceGroupWire = raw && typeof raw === 'object' ? raw : {}
  return {
    traceId: String(d.trace_id ?? d.traceId ?? d.TraceID ?? ''),
    caller: String(d.caller ?? d.Caller ?? ''),
    model: String(d.model ?? d.Model ?? ''),
    provider: String(d.provider ?? d.Provider ?? ''),
    status: String(d.status ?? d.Status ?? ''),
    stream: Boolean(d.stream ?? d.Stream),
    startedAt: String(d.started_at ?? d.startedAt ?? d.StartedAt ?? ''),
    finishedAt: String(d.finished_at ?? d.finishedAt ?? d.FinishedAt ?? ''),
    spanCount: Number(d.span_count ?? d.spanCount ?? d.SpanCount ?? 0),
    toolCount: Number(d.tool_count ?? d.toolCount ?? d.ToolCount ?? 0),
    llmCount: Number(d.llm_count ?? d.llmCount ?? d.LLMCount ?? 0),
    inputTokens: Number(d.input_tokens ?? d.inputTokens ?? d.InputTokens ?? 0),
    outputTokens: Number(d.output_tokens ?? d.outputTokens ?? d.OutputTokens ?? 0),
    durationMs: Number(d.duration_ms ?? d.durationMs ?? d.DurationMs ?? 0),
    errorCode: d.error_code || d.errorCode || d.ErrorCode || undefined,
    requestId: String(d.request_id ?? d.requestId ?? d.RequestID ?? '')
  }
}

export function toLLMTraceGroupListResult(raw: LLMTraceGroupListWire | undefined): LLMTraceGroupListResult {
  const d: LLMTraceGroupListWire = raw && typeof raw === 'object' ? raw : {}
  const list = listOf(d.groups)
  return {
    groups: list.map((r) => toLLMTraceGroup(r)),
    total: Number(d.total ?? 0)
  }
}

export function toBackupItem(value: unknown): BackupItem {
  const raw = collectionWire(value)
  return {
    id: String(raw.id ?? ''),
    projectId: String(raw.project_id ?? ''),
    sizeBytes: Number(raw.size_bytes ?? 0),
    sha256: String(raw.sha256 ?? ''),
    engine: String(raw.engine ?? ''),
    formatVersion: Number(raw.format_version ?? 0),
    status: String(raw.status ?? ''),
    createdAt: String(raw.created_at ?? ''),
    expiresAt: raw.expires_at ? String(raw.expires_at) : undefined
  }
}

export function toFunctionTrigger(value: unknown): FunctionTrigger {
  const raw = collectionWire(value)
  return {
    id: String(raw.id ?? ''),
    projectId: String(raw.project_id ?? ''),
    kind: raw.kind === 'http' || raw.kind === 'object_event' ? raw.kind : 'record_change',
    target: String(raw.target ?? ''),
    events: listOf(raw.events, String),
    funcFile: String(raw.func_file ?? ''),
    funcExport: String(raw.func_export ?? ''),
    filter: raw.filter ? String(raw.filter) : undefined,
    enabled: raw.enabled === true,
    createdAt: String(raw.created_at ?? ''),
    updatedAt: String(raw.updated_at ?? '')
  }
}

export function toTriggerWire(input: Partial<FunctionTriggerInput>) {
  return {
    kind: input.kind,
    target: input.target,
    events: input.events,
    func_file: input.funcFile,
    func_export: input.funcExport,
    filter: input.filter,
    enabled: input.enabled
  }
}

export function toProjectItem(raw: WireRecord): ProjectItem {
  return {
    id: String(raw?.id ?? ''),
    name: String(raw?.name ?? ''),
    engine: raw?.engine === 'sqlite' ? 'sqlite' : raw?.engine === 'duckdb' ? 'duckdb' : undefined,
    createdAt: String(raw?.created_at ?? raw?.createdAt ?? '')
  }
}

/** ApiKeyItem：后端 snake_case → UI camelCase */
export function toApiKeyItem(raw: WireRecord): ApiKeyItem {
  return {
    id: String(raw?.id ?? ''),
    projectId: String(raw?.project_id ?? ''),
    permissions: listOf(raw?.permissions, String),
    createdAt: String(raw?.created_at ?? ''),
    revokedAt: orUndefined(raw, 'revoked_at')
  }
}

/** AuthUser：后端 snake_case → UI camelCase */
export function toAuthUser(d: WireRecord): AuthUser {
  return {
    id: pick(d, 'id'),
    username: pick(d, 'username'),
    role: pick(d, 'role'),
    displayName: orElse(d, 'display_name', ''),
    email: orElse(d, 'email', ''),
    status: pick(d, 'status'),
    mustChangePassword: Boolean(d.must_change_password),
    createdBy: orElse(d, 'created_by', ''),
    createdAt: orElse(d, 'created_at', ''),
    lastLoginAt: orElse(d, 'last_login_at', '')
  }
}

export function toTokenPair(d: WireRecord): TokenPair {
  return {
    tokenType: orElse(d, 'token_type', 'Bearer'),
    accessToken: orElse(d, 'access_token', ''),
    expiresIn: orElse(d, 'expires_in', 0),
    refreshToken: orElse(d, 'refresh_token', ''),
    user: toAuthUser(objectOf(d.user) ?? {})
  }
}

export function toUserItem(d: WireRecord): UserItem {
  return { ...toAuthUser(d), projectCount: orDefault(d, 'project_count', 0) }
}
