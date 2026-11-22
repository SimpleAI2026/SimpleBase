// http-api-converters.ts — http-api 的命名转换层：
// UI camelCase ↔ 后端 snake_case 的集中转换函数与 wire 接口。
// 从 http-api.ts 提取（planv6.6 OPT-6）；仅被 http-api.ts 消费，
// 页面代码不直接 import 本文件。

import type {
  AgentApproval,
  AgentApprovalStatus,
  AgentBuiltinTagKey,
  AgentDispatchMode,
  AgentHumanTask,
  AgentMessage,
  AgentRouteKind,
  AgentRunMetrics,
  AgentRunRequest,
  AgentSchedule,
  AgentStreamHandlers,
  AgentTag,
  AgentTagInput,
  AgentTask,
  AgentTaskStatus,
  AgentThread,
  AgentThreadKind,
  AgentTurn,
  AgentTurnRequest,
  AgentUsageEventWire,
  AgentWorkspaceCapabilities,
  AggregateQuery,
  AggregateResult,
  BackupItem,
  CloudAgent,
  CollectionInfo,
  CollectionSchema,
  CronJobCreate,
  CronJobItem,
  CronJobRunItem,
  CronJobRunWire,
  CronJobWire,
  DatabaseItem,
  DatabaseSchema,
  DocumentItem,
  DocumentPage,
  GoFunctionItem,
  LlmSettings,
  LlmStreamConnection,
  LLMTrace,
  LLMTraceGroup,
  LLMTraceGroupListResult,
  LlmTraceGroupQuery,
  LogEvent,
  LogEventWire,
  LogQuery,
  LogRetention,
  LogRetentionWire,
  McpBuiltinServer,
  McpServer,
  McpToolPolicy,
  MetricsSummary,
  MetricsSummaryWire,
  SandboxFileEntry,
  SandboxItem,
  S3FolderMeta,
  S3PublicResult,
  S3SiteInfo,
  SchemaColumn,
  SchemaDryRunResult,
  SchemaTable,
  SchemaTableRows,
  SqlBatchResult,
  SqlExecuteResult,
  SqlQueryResult,
  SqlRequest,
  TokenPair,
  TrendPoint,
  TrendPointWire,
  UserItem,
  ApiKeyItem,
  AuthUser,
  FunctionTrigger,
  FunctionTriggerInput
} from './types'
import { AGENT_TASK_STATUSES } from './types'
import { t } from '@/locales'
import { http, baseURL, getAccessToken, getApiKey } from './http'


/** planv6.5：站点状态（S3 域后端已是 camelCase，这里只做类型收敛与兜底）。 */
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

/** DatabaseItem：后端 snake_case → UI camelCase */
export function toDatabaseItem(d: Record<string, any>): DatabaseItem {
  return {
    id: d.id,
    name: d.name,
    status: d.status,
    createdAt: d.created_at,
    updatedAt: d.updated_at,
    snapshot: d.snapshot
      ? { lastSyncedSnapshot: d.snapshot.last_synced_snapshot, syncLag: d.snapshot.sync_lag }
      : undefined,
    documentCount: typeof d.document_count === 'number' ? d.document_count : undefined,
    dataModel: d.data_model === 'sql' ? 'sql' : 'collection'
  }
}

export function schemaText(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

export function toSchemaColumn(raw: unknown): SchemaColumn {
  const column = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : undefined
  const mapped: SchemaColumn = {
    name: schemaText(column?.name),
    type: schemaText(column?.type),
    nullable: column?.nullable !== false
  }
  if (column?.sensitive === true) mapped.sensitive = true
  return mapped
}

export function toSchemaTableRows(raw: unknown): SchemaTableRows {
  const body = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : undefined
  const columns = Array.isArray(body?.columns) ? body.columns.map(toSchemaColumn) : []
  const rows: unknown[][] = []
  if (Array.isArray(body?.rows)) {
    for (const row of body.rows) {
      rows.push(Array.isArray(row) ? Array.from(row) : [])
    }
  }
  return {
    table: schemaText(body?.table),
    columns,
    rows,
    limit: typeof body?.limit === 'number' ? body.limit : 0,
    offset: typeof body?.offset === 'number' ? body.offset : 0,
    total: typeof body?.total === 'number' ? body.total : 0
  }
}

export function toSchemaTable(raw: unknown): SchemaTable {
  const table = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : undefined
  const columns = Array.isArray(table?.columns) ? table.columns.map(toSchemaColumn) : []
  return { name: schemaText(table?.name), columns }
}

export function toDatabaseSchema(raw: unknown): DatabaseSchema {
  const body = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : undefined
  const tables = Array.isArray(body?.tables) ? body.tables.map(toSchemaTable) : []
  return { tables }
}

export function databaseResourcePath(projectId: string, databaseId: string, suffix: string) {
  return (
    '/v1/projects/' +
    encodeURIComponent(projectId) +
    '/databases/' +
    encodeURIComponent(databaseId) +
    suffix
  )
}

/** SQL 请求：UI camelCase → 后端 snake_case */
export function toSqlPayload(req: SqlRequest) {
  return { sql: req.sql, args: req.args, max_rows: req.maxRows }
}

/** SQL query 响应：后端 → UI（rows 保持二维数组，由页面按 columns zip） */
export function toQueryResult(r: Record<string, any>): SqlQueryResult {
  return {
    columns: r.columns ?? [],
    rows: r.rows ?? [],
    rowCount: r.row_count ?? 0,
    durationMs: r.duration_ms ?? 0,
    requestId: r.request_id ?? ''
  }
}

export function toExecuteResult(r: Record<string, any>): SqlExecuteResult {
  return {
    rowsAffected: r.rows_affected ?? 0,
    durability: r.durability ?? '',
    durationMs: r.duration_ms ?? 0,
    requestId: r.request_id ?? ''
  }
}

export function toBatchResult(r: Record<string, any>): SqlBatchResult {
  const items = (r.results ?? []).map((it: Record<string, any>) => ({
    index: it.index ?? 0,
    rowsAffected: it.rows_affected,
    durationMs: it.duration_ms,
    errorCode: it.error_code,
    errorMessage: it.error_message
  }))
  const err = r.error
    ? { failedIndex: r.error.failed_index, code: r.error.code, message: r.error.message }
    : undefined
  return {
    results: items,
    durability: r.durability ?? '',
    durationMs: r.duration_ms ?? 0,
    requestId: r.request_id ?? '',
    error: err
  }
}

/**
 * 逐帧解析 SSE 的 `data:` 行并回调 JSON 对象。
 *
 * `onDone` 仅供 OpenAI 兼容流使用：它的结束哨兵是字面量 `[DONE]`（不是 JSON），
 * 解析会抛错并被静默忽略，调用方因此永远等不到结束信号、界面一直转圈。
 * 云助手 run 流仍以 `{"type":"end"}` 收尾，不传 onDone，行为与之前完全一致。
 */
export async function readSse(
  resp: Response,
  onEvent: (event: Record<string, any>) => void,
  onDone?: () => void
): Promise<void> {
  const reader = resp.body!.getReader()
  const decoder = new TextDecoder()
  let buf = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buf += decoder.decode(value, { stream: true })
    let idx: number
    while ((idx = buf.indexOf('\n\n')) >= 0) {
      const raw = buf.slice(0, idx)
      buf = buf.slice(idx + 2)
      let eventId = ''
      for (const line of raw.split('\n')) {
        if (line.startsWith(':')) continue
        if (line.startsWith('id:')) {
          eventId = line.slice(3).trim()
          continue
        }
        if (!line.startsWith('data:')) continue
        const data = line.slice(5).trim()
        if (!data) continue
        if (onDone && data === '[DONE]') {
          onDone()
          continue
        }
        try {
          const parsed = JSON.parse(data) as Record<string, any>
          if (eventId) parsed._sse_id = eventId
          onEvent(parsed)
        } catch {
          /* 忽略非 JSON 帧 */
        }
      }
    }
  }
}

/** SSE 流式对话已随 planv6.6 DC-5 删除：聊天统一走 agentThreads.streamRun。 */

export function agentPath(projectId: string, rest = '') {
  return '/v1/projects/' + encodeURIComponent(projectId) + rest
}

/** 云函数路径 */
export function gofunctionsPath(projectId: string, name?: string, ver?: number | string, suffix?: string) {
  let path = '/v1/projects/' + encodeURIComponent(projectId) + '/gofunctions'
  if (name) path += '/' + encodeURIComponent(name)
  if (ver !== undefined) path += '/versions/' + encodeURIComponent(String(ver))
  if (suffix) path += '/' + suffix
  return path
}

/* v8 ignore start -- 防御性字段映射：空/满 payload 双向已有 sparse 测试 */
export function toGoFuncVersion(raw: Record<string, any>) {
  return {
    version: Number(raw?.version ?? 0),
    exports: Array.isArray(raw?.exports) ? raw.exports.map(String) : [],
    note: String(raw?.note ?? ''),
    createdAt: String(raw?.created_at ?? ''),
    active: Boolean(raw?.active),
    source: raw?.source || undefined
  }
}

/** GoFunctionItem：后端 snake_case → UI camelCase */
export function toGoFunctionItem(raw: Record<string, any>): GoFunctionItem {
  return {
    id: String(raw?.id ?? ''),
    name: String(raw?.name ?? ''),
    file: String(raw?.file ?? ''),
    description: String(raw?.description ?? ''),
    activeVersion: Number(raw?.active_version ?? 0),
    latestVersion: Number(raw?.latest_version ?? 0),
    published: Boolean(raw?.published),
    exports: Array.isArray(raw?.exports) ? raw.exports.map(String) : [],
    versions: Array.isArray(raw?.versions) ? raw.versions.map(toGoFuncVersion) : undefined,
    source: raw?.source || undefined,
    createdAt: String(raw?.created_at ?? ''),
    updatedAt: String(raw?.updated_at ?? '')
  }
}
/* v8 ignore stop */

/* ---------- 云沙盒：E2B 兼容 API（planv6.2）。控制面 /e2b/**，envd 面 /e2b/envd/**，扩展 /e2b/ext/** ---------- */
export const E2B_BASE = '/e2b'
export const ENVD_BASE = '/e2b/envd'
export const SB_META_NAME = 'name'
export const SB_META_SOURCE = 'simplebase.source'

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

/** 定时任务路径：/v1/projects/:pid/cron-jobs[/:jobId[/runs|/trigger]] */
export function cronJobsPath(projectId: string, jobId?: string, suffix?: 'runs' | 'trigger') {
  let path = '/v1/projects/' + encodeURIComponent(projectId) + '/cron-jobs'
  if (jobId) {
    path += '/' + encodeURIComponent(jobId)
    if (suffix) path += '/' + suffix
  }
  return path
}

/** 未识别的调度类型回退 cron（旧数据兼容）。 */
export function toCronScheduleKind(raw: unknown): CronJobItem['scheduleKind'] {
  if (raw === 'interval') return 'interval'
  if (raw === 'once') return 'once'
  return 'cron'
}

/** CronJobItem：后端 snake_case → UI camelCase（§3.13）。raw 可能缺失（响应体为空）。 */
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
export function trendPointList(d: unknown): unknown[] {
  if (Array.isArray(d)) return d
  const points = (d as { points?: unknown } | null | undefined)?.points
  return Array.isArray(points) ? points : []
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

export function toLlmSettings(raw: Record<string, any> | undefined): LlmSettings {
  const d = raw && typeof raw === 'object' ? raw : {}
  return {
    defaultProvider: d.default_provider || d.defaultProvider || undefined,
    defaultModel: d.default_model || d.defaultModel || undefined,
    baseUrl: d.base_url || d.baseUrl || undefined,
    apiKeyMasked: d.api_key_masked || d.apiKeyMasked || undefined,
    hasApiKey: Boolean(d.has_api_key ?? d.hasApiKey),
    temperature: d.temperature ?? undefined,
    maxTokens: d.max_tokens ?? d.maxTokens ?? undefined
  }
}

/** llmTraces.list 已随 planv6.6 DC-5 删除（零生产引用）；groups/detail 仍在。 */

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

export interface LLMTraceGroupWire {
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

export interface LLMTraceGroupListWire {
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

export function toLLMTraceGroup(raw: LLMTraceGroupWire | null | undefined): LLMTraceGroup {
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
  const list = Array.isArray(d.groups) ? d.groups : []
  return {
    groups: list.map((r) => toLLMTraceGroup(r)),
    total: Number(d.total ?? 0)
  }
}

export function toCloudAgent(raw: Record<string, any>): CloudAgent {
  return {
    id: String(raw?.id ?? ''),
    name: String(raw?.name ?? ''),
    module: String(raw?.module ?? ''),
    description: String(raw?.description ?? ''),
    system_prompt: String(raw?.system_prompt ?? ''),
    tool_ids: Array.isArray(raw?.tool_ids) ? raw.tool_ids.map(String) : [],
    model_override: raw?.model_override || undefined,
    builtin_key: raw?.builtin_key || undefined,
    team_enabled: !!raw?.team_enabled,
    created_at: String(raw?.created_at ?? ''),
    updated_at: String(raw?.updated_at ?? '')
  }
}

/** planv6.7 AGC-ROUTE：route_kind 合法值守卫（不用 any）。 */
export function isRouteKind(v: unknown): v is AgentRouteKind {
  return v === 'auto' || v === 'mention' || v === 'fallback' || v === 'retry'
}

function isAgentTaskStatus(value: unknown): value is AgentTaskStatus {
  return AGENT_TASK_STATUSES.some((status) => status === value)
}

/**
 * v2 §5.1：存储里的 auto 在读时统一映射为 collaborative（群会话）。
 * 旧会话因此获得群路由与多助手能力，界面不再出现「一轮一助手」的旧语义。
 */
export function toAgentThread(raw: Record<string, any>): AgentThread {
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

/** 调度模式守卫：auto 一律折叠为 collaborative（v2 读时统一）。 */
export function isDispatchMode(v: unknown): v is AgentDispatchMode {
  return v === 'auto' || v === 'collaborative'
}

/** 会话类型守卫。 */
export function isThreadKind(v: unknown): v is AgentThreadKind {
  return v === 'chat' || v === 'alert' || v === 'statistics' || v === 'scheduled'
}

/** 内置群 key 守卫。 */
export function isBuiltinTagKey(v: unknown): v is AgentBuiltinTagKey {
  return v === 'general' || v === 'alerts' || v === 'stats' || v === 'scheduled'
}

export function toAgentMessage(raw: Record<string, any>): AgentMessage {
  return {
    id: String(raw?.id ?? ''),
    role: String(raw?.role ?? ''),
    content: String(raw?.content ?? ''),
    agent_id: raw?.agent_id || undefined,
    mentions: Array.isArray(raw?.mentions) ? raw.mentions : [],
    tool_calls: Array.isArray(raw?.tool_calls) ? raw.tool_calls : [],
    run_id: raw?.run_id || undefined,
    run_status: raw?.run_status || undefined,
    error_code: raw?.error_code || undefined,
    route_kind: isRouteKind(raw?.route_kind) ? raw.route_kind : undefined,
    created_at: String(raw?.created_at ?? '')
  }
}

/** planv6.7 AGC-NAV：工作台标签（snake → camel，数组兜底 []）。 */
export function toAgentTag(raw: Record<string, any>): AgentTag {
  const prompts = Array.isArray(raw?.prompts)
    ? raw.prompts.map((p: Record<string, any>) => ({
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
    agentIds: Array.isArray(raw?.agent_ids) ? raw.agent_ids.map(String) : [],
    prompts,
    unavailableAgentIds: Array.isArray(raw?.unavailable_agent_ids) ? raw.unavailable_agent_ids.map(String) : [],
    routingPrompt: String(raw?.routing_prompt ?? ''),
    fallbackAgentId: raw?.fallback_agent_id ? String(raw.fallback_agent_id) : undefined,
    humanPolicy: raw?.human_policy === 'disabled' ? 'disabled' : undefined,
    threadKinds,
    version: Number(raw?.version ?? 0),
    updatedAt: raw?.updated_at ? String(raw.updated_at) : undefined
  }
}

/** planv6.7 AGC-NAV：标签输入（camel → snake）。 */
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
  return body
}

export function toAgentApproval(raw: Record<string, any>): AgentApproval {
  return {
    id: String(raw?.id ?? ''),
    run_id: String(raw?.run_id ?? ''),
    call_id: String(raw?.call_id ?? ''),
    thread_id: raw?.thread_id || undefined,
    agent_id: raw?.agent_id || undefined,
    tool_name: raw?.tool_name || undefined,
    argv: Array.isArray(raw?.argv) ? raw.argv.map(String) : [],
    message: raw?.message || undefined,
    status: String(raw?.status ?? 'pending') as AgentApprovalStatus,
    initiator_id: raw?.initiator_id || undefined,
    resolved_by: raw?.resolved_by || undefined,
    created_at: String(raw?.created_at ?? ''),
    expires_at: raw?.expires_at || undefined,
    resolved_at: raw?.resolved_at || undefined
  }
}

export function toMcpServer(raw: Record<string, any>): McpServer {
  const policy = raw?.tools_policy
  return {
    id: String(raw?.id ?? ''),
    name: String(raw?.name ?? ''),
    transport: String(raw?.transport ?? 'http'),
    url: String(raw?.url ?? ''),
    timeout_s: Number(raw?.timeout_s || 60),
    enabled: Boolean(raw?.enabled),
    tools_policy: policy && typeof policy === 'object' ? (policy as Record<string, McpToolPolicy>) : {},
    header_names: Array.isArray(raw?.header_names) ? raw.header_names.map(String) : [],
    has_credential: Boolean(raw?.has_credential),
    name_conflict: raw?.name_conflict ? true : undefined,
    created_at: String(raw?.created_at ?? ''),
    updated_at: String(raw?.updated_at ?? '')
  }
}

export function toMcpBuiltinServer(raw: Record<string, any>): McpBuiltinServer {
  const tools = Array.isArray(raw?.tools) ? raw.tools : []
  return {
    name: String(raw?.name ?? ''),
    enabled: Boolean(raw?.enabled),
    transport: String(raw?.transport ?? 'http'),
    url: String(raw?.url ?? ''),
    tools: tools.map((x: Record<string, any>) => ({
      name: String(x?.name ?? ''),
      read_only: Boolean(x?.read_only),
      destructive: Boolean(x?.destructive)
    }))
  }
}

/**
 * planv6.6 DC-5：旧 /agent-schedules 兼容层已下线，助手卡片的定时摘要
 * 由 agentScheduleFromCronJob 从 cronjobs.list(targetKind=agent) 条目映射。
 */
/** cronjobs.list 中 targetKind=agent 的条目 → AgentSchedule（助手卡片摘要数据源）。 */
export function agentScheduleFromCronJob(job: CronJobItem): AgentSchedule {
  return {
    id: job.id,
    agent_id: job.agentId ?? '',
    agent_name: undefined,
    thread_id: job.agentThreadId ?? '',
    prompt: job.agentPrompt ?? '',
    cron_expr: job.scheduleKind === 'cron' ? job.cronExpr : '',
    enabled: job.enabled,
    last_run_at: job.lastRunAt,
    next_run_at: job.nextRunAt,
    created_at: job.createdAt,
    updated_at: job.updatedAt
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

/** 轮次（v2）：snake → camel；任务数组兜底 []。 */
export function toAgentTurn(raw: Record<string, any>): AgentTurn {
  const tasks = Array.isArray(raw?.tasks) ? raw.tasks.map(toAgentTask) : []
  return {
    id: String(raw?.id ?? ''),
    threadId: String(raw?.thread_id ?? ''),
    tagId: raw?.tag_id ? String(raw.tag_id) : undefined,
    status: String(raw?.status ?? '') as AgentTurn['status'],
    errorCode: raw?.error_code ? String(raw.error_code) : undefined,
    route: raw?.route ? String(raw.route) : undefined,
    version: Number(raw?.version ?? 0),
    createdAt: String(raw?.created_at ?? ''),
    updatedAt: String(raw?.updated_at ?? ''),
    tasks,
    lastEventSeq: Number(raw?.last_event_seq ?? 0)
  }
}

export function toAgentTask(raw: Record<string, any>): AgentTask {
  return {
    id: String(raw?.id ?? ''),
    turnId: String(raw?.turn_id ?? ''),
    seq: Number(raw?.seq ?? 0),
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
    dependsOn: Array.isArray(raw?.depends_on) ? raw.depends_on.map(String) : [],
    errorCode: raw?.error_code ? String(raw.error_code) : undefined,
    version: Number(raw?.version ?? 0),
    createdAt: String(raw?.created_at ?? '')
  }
}

/** 人工待办：snake → camel。 */
export function toAgentHumanTask(raw: Record<string, any>): AgentHumanTask {
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

/** 协作能力：未知字段一律取保守默认（不露出协作控件）。 */
export function toWorkspaceCapabilities(raw: Record<string, any>): AgentWorkspaceCapabilities {
  const kinds = Array.isArray(raw?.thread_kinds) ? raw.thread_kinds.filter(isThreadKind) : []
  return {
    protocolVersion: Number(raw?.protocol_version ?? 0),
    collaboration: raw?.collaboration === true,
    threadKinds: kinds,
    maxTasks: Number(raw?.max_tasks ?? 0),
    maxDepth: Number(raw?.max_depth ?? 0),
    maxInstruction: Number(raw?.max_instruction ?? 0),
    missingTables: Array.isArray(raw?.missing_tables) ? raw.missing_tables.map(String) : undefined
  }
}

/** 发起一轮（v2）：走 /turns，支持多目标 @。 */
export function streamAgentTurn(
  projectId: string,
  threadId: string,
  req: AgentTurnRequest,
  handlers: AgentStreamHandlers
): LlmStreamConnection {
  return openAgentRunStream(projectId, handlers, '', (signal) =>
    fetch(`${baseURL}${agentPath(projectId, '/agent-threads/' + encodeURIComponent(threadId) + '/turns')}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${getAccessToken() || getApiKey()}`
      },
      body: JSON.stringify({
        content: req.content,
        mentions: req.mentions,
        stream: true,
        ...(req.idempotencyKey ? { idempotency_key: req.idempotencyKey } : {}),
        ...(req.threadKind ? { thread_kind: req.threadKind } : {})
      }),
      signal
    })
  )
}

export function streamAgentRun(
  projectId: string,
  threadId: string,
  req: AgentRunRequest,
  handlers: AgentStreamHandlers
): LlmStreamConnection {
  return openAgentRunStream(projectId, handlers, '', (signal) =>
    fetch(`${baseURL}${agentPath(projectId, '/agent-threads/' + encodeURIComponent(threadId) + '/runs')}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${getAccessToken() || getApiKey()}`
      },
      body: JSON.stringify({
        content: req.content,
        mentions: req.mentions,
        stream: true,
        ...(req.retry_of_run_id ? { retry_of_run_id: req.retry_of_run_id } : {}),
        // 数据看板生成图表：必须显式透传，否则被这里的字段白名单静默丢弃，助手拿不到字段清单。
      }),
      signal
    })
  )
}

/** 续接进行中的 run：从头回放事件缓存（after=0）并跟随直到 end（切换界面后回到会话时使用）。 */
export function attachAgentRun(projectId: string, runId: string, handlers: AgentStreamHandlers): LlmStreamConnection {
  return openAgentRunStream(projectId, handlers, runId, (signal) => fetchAgentRunEvents(projectId, runId, '', signal))
}

export function fetchAgentRunEvents(projectId: string, runId: string, after: string, signal: AbortSignal) {
  return fetch(
    `${baseURL}${agentPath(projectId, '/agent-runs/' + encodeURIComponent(runId) + '/events')}?after=${encodeURIComponent(after)}`,
    {
      method: 'GET',
      headers: { Authorization: `Bearer ${getAccessToken() || getApiKey()}` },
      signal
    }
  )
}

export function openAgentRunStream(
  projectId: string,
  handlers: AgentStreamHandlers,
  initialRunId: string,
  open: (signal: AbortSignal) => Promise<Response>
): LlmStreamConnection {
  const controller = new AbortController()
  let ended = false
  let failed = false
  let lastId = ''
  let runId = initialRunId
  const fireEnd = (reason?: string) => {
    if (!ended) {
      ended = true
      if (!failed) handlers.onEnd?.(reason)
    }
  }
  const onEvent = (obj: Record<string, any>) => {
    if (obj?._sse_id != null && String(obj._sse_id) !== '') lastId = String(obj._sse_id)
    // v2：轮次模式下每帧携带归属（哪个子 run / 哪个助手），供协作状态索引任务。
    if (obj?.run_id || obj?.agent_id) {
      handlers.onFrameContext?.({
        runId: obj?.run_id ? String(obj.run_id) : undefined,
        agentId: obj?.agent_id ? String(obj.agent_id) : undefined
      })
    }
    if (obj?.type === 'end') {
      fireEnd(typeof obj.reason === 'string' ? obj.reason : undefined)
      return
    }
    if (obj?.type === 'run' && obj.run_id) {
      runId = String(obj.run_id)
      handlers.onRun?.(
        runId,
        obj.agent_id ? String(obj.agent_id) : undefined,
        isRouteKind(obj.route_kind) ? obj.route_kind : undefined
      )
      return
    }
    if (obj?.type === 'route') {
      // planv6.7 AGC-ROUTE：routing（正在选择助手）→ done（agent_id + route_kind）；
      // v2：done 帧可带分工步骤（多人/多助手）。
      handlers.onRoute?.({
        status: obj.status === 'done' ? 'done' : 'routing',
        agentId: obj.agent_id ? String(obj.agent_id) : undefined,
        routeKind: isRouteKind(obj.route_kind) ? obj.route_kind : undefined,
        steps: Array.isArray(obj.steps)
          ? obj.steps.map((s: Record<string, any>) => ({
              kind: s?.kind === 'human' ? 'human' : 'agent',
              agentId: s?.agent_id ? String(s.agent_id) : undefined,
              instruction: String(s?.instruction ?? ''),
              routeKind: isRouteKind(s?.route_kind) ? s.route_kind : undefined,
              taskId: s?.task_id ? String(s.task_id) : undefined
            }))
          : undefined
      })
      return
    }
    if (obj?.type === 'task') {
      handlers.onTask?.({
        taskId: obj.task_id ? String(obj.task_id) : undefined,
        agentId: obj.agent_id ? String(obj.agent_id) : undefined,
        index: Number(obj.index || 0),
        total: Number(obj.total || 0),
        status: isAgentTaskStatus(obj.status) ? obj.status : 'pending',
        message: obj.message ? String(obj.message) : undefined,
        runId: obj.run_id ? String(obj.run_id) : undefined
      })
      return
    }
    if (obj?.type === 'human') {
      handlers.onHuman?.({
        taskId: obj.task_id ? String(obj.task_id) : undefined,
        humanId: obj.human_id ? String(obj.human_id) : undefined,
        humanName: obj.human_name ? String(obj.human_name) : undefined,
        index: Number(obj.index || 0),
        message: obj.message ? String(obj.message) : undefined
      })
      return
    }
    if (obj?.type === 'turn') {
      handlers.onTurn?.({
        status: String(obj.status ?? ''),
        index: Number(obj.index || 0),
        total: Number(obj.total || 0)
      })
      return
    }
    if (obj?.type === 'error') {
      failed = true
      const error = new Error(String(obj.message || t('error.runFailed'))) as Error & { code?: string }
      error.code = typeof obj.code === 'string' ? obj.code : undefined
      handlers.onError?.(error)
      return
    }
    if (obj?.type === 'thinking') {
      handlers.onThinking?.(Number(obj.elapsed_ms || 0), obj.content ? String(obj.content) : undefined)
      return
    }
    if (obj?.type === 'usage') {
      handlers.onUsage?.(toAgentRunMetrics(obj))
      return
    }
    if (obj?.type === 'token' || obj?.type === 'chunk') {
      const text = obj.content || obj.delta || ''
      if (text) handlers.onToken?.(text)
      return
    }
    if (obj?.type === 'tool_call_delta') {
      handlers.onToolCallDelta?.(obj.name || '', obj.arguments || '', obj.call_id || undefined)
      return
    }
    if (obj?.type === 'tool_start') {
      handlers.onToolStart?.(obj.name || '', obj.call_id || undefined)
      return
    }
    if (obj?.type === 'confirmation_required') {
      const argv = Array.isArray(obj.argv) ? obj.argv.map((part: unknown) => String(part)) : []
      handlers.onConfirmationRequired?.(String(obj.call_id || ''), argv, String(obj.message || ''))
      return
    }
    if (obj?.type === 'confirmation_resolved') {
      handlers.onConfirmationResolved?.(String(obj.call_id || ''), obj.approve === true)
      return
    }
    if (obj?.type === 'tool_call') {
      handlers.onToolCall?.(obj.name || '', obj.arguments || '', obj.call_id || undefined)
      return
    }
    if (obj?.type === 'tool_progress') {
      handlers.onToolProgress?.(obj.name || '', Number(obj.elapsed_ms || 0), obj.call_id || undefined)
      return
    }
    if (obj?.type === 'tool_result') {
      handlers.onToolResult?.(obj.name || '', obj.content || '', obj.call_id || undefined, Number(obj.duration_ms || 0), obj.is_error === true, obj.truncated === true)
    }
  }
  ;(async () => {
    try {
      const resp = await open(controller.signal)
      const ct = resp.headers.get('content-type') || ''
      if (ct.includes('text/html')) {
        throw new Error(t('error.htmlResponse'))
      }
      if (!resp.ok || !resp.body) {
        const data = await resp.json().catch(() => null)
        const detail = (data as { error?: { code?: string; message?: string }; message?: string } | null)
        const error = new Error(detail?.error?.message || detail?.message || t('error.requestFailedWithStatus', { status: resp.status })) as Error & { code?: string }
        error.code = detail?.error?.code
        throw error
      }
      // BUG-B5（planv6.0 v2 §2.2）：runID 此前只由首个 SSE 帧下发，连接在 run 帧
      // 到达前断开时无法续传。响应头先给一份，后续 run 事件仍会覆盖同一值。
      const headerRunId = resp.headers.get('x-run-id')
      if (headerRunId) {
        runId = headerRunId
        handlers.onRun?.(runId)
      }
      await readSse(resp, onEvent)
      if (!ended && !failed) throw new Error(t('error.streamFailed'))
    } catch (e) {
      if (controller.signal.aborted || failed) return
      let delay = 1000
      let err: unknown = e
      for (let attempt = 0; attempt < 3 && runId && !ended; attempt++) {
        try {
          await new Promise<void>((resolve, reject) => {
            const timer = setTimeout(resolve, delay)
            controller.signal.addEventListener('abort', () => {
              clearTimeout(timer)
              reject(e)
            }, { once: true })
          })
          if (controller.signal.aborted) return
          const resp = await fetchAgentRunEvents(projectId, runId, lastId, controller.signal)
          if (!resp.ok || !resp.body) throw new Error(t('error.requestFailedWithStatus', { status: resp.status }))
          await readSse(resp, onEvent)
          if (!ended && !failed) throw new Error(t('error.streamFailed'))
          return
        } catch (next) {
          if (controller.signal.aborted) return
          err = next
          delay *= 2
        }
      }
      if (!controller.signal.aborted && !failed) handlers.onError?.(err)
    }
  })()
  return { close: () => controller.abort() }
}

export function collectionWire(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : {}
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

export function toCollectionInfo(value: unknown): CollectionInfo {
  const raw = collectionWire(value)
  return {
    name: String(raw.name ?? ''),
    schemaMode: raw.schema_mode === 'view' ? 'view' : 'json',
    fieldCount: Number(raw.field_count ?? 0),
    documentCount: Number(raw.document_count ?? 0),
    registered: raw.registered === true
  }
}

export function toCollectionSchema(value: unknown): CollectionSchema {
  const raw = collectionWire(value)
  const fields = Array.isArray(raw.fields) ? raw.fields : []
  return {
    id: raw.id ? String(raw.id) : undefined,
    projectId: raw.project_id ? String(raw.project_id) : undefined,
    name: String(raw.name ?? ''),
    schemaMode: raw.schema_mode === 'view' ? 'view' : 'json',
    viewQuery: raw.view_query ? String(raw.view_query) : undefined,
    fields: fields.map((entry) => {
      const field = collectionWire(entry)
      const options = collectionWire(field.options)
      const type = String(field.type ?? 'text') as CollectionSchema['fields'][number]['type']
      return {
        name: String(field.name ?? ''),
        type,
        required: field.required === true,
        unique: field.unique === true,
        default: field.default,
        hidden: field.hidden === true,
        readonly: field.readonly === true,
        options: {
          min: typeof options.min === 'number' || typeof options.min === 'string' ? options.min : undefined,
          max: typeof options.max === 'number' || typeof options.max === 'string' ? options.max : undefined,
          pattern: options.pattern ? String(options.pattern) : undefined,
          values: Array.isArray(options.values) ? options.values.map(String) : undefined,
          maxSelect: options.max_select ? Number(options.max_select) : undefined,
          maxSize: options.max_size ? Number(options.max_size) : undefined,
          onlyInt: options.only_int === true,
          exceptDomains: Array.isArray(options.except_domains) ? options.except_domains.map(String) : undefined,
          onlyDomains: Array.isArray(options.only_domains) ? options.only_domains.map(String) : undefined
        }
      }
    })
  }
}

export function toFunctionTrigger(value: unknown): FunctionTrigger {
  const raw = collectionWire(value)
  return {
    id: String(raw.id ?? ''),
    projectId: String(raw.project_id ?? ''),
    kind: raw.kind === 'http' || raw.kind === 'object_event' ? raw.kind : 'record_change',
    target: String(raw.target ?? ''),
    events: Array.isArray(raw.events) ? raw.events.map(String) : [],
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

export function toDocumentItem(value: unknown): DocumentItem {
  const raw = collectionWire(value)
  return {
    id: String(raw.id ?? ''),
    createdAt: String(raw.created_at ?? ''),
    updatedAt: String(raw.updated_at ?? raw.created_at ?? ''),
    data: collectionWire(raw.data)
  }
}

export function toDocumentPage(value: unknown): DocumentPage {
  const raw = collectionWire(value)
  return {
    rows: Array.isArray(raw.rows) ? (raw.rows as DocumentPage['rows']) : [],
    items: Array.isArray(raw.items) ? raw.items.map(toDocumentItem) : [],
    nextCursor: raw.next_cursor ? String(raw.next_cursor) : undefined,
    total: typeof raw.total === 'number' ? raw.total : undefined
  }
}

/** 聚合请求 → query string 参数。数组与驼峰在这里转成后端的 snake_case；空值整键丢弃。 */
export function toAggregateParams(query: AggregateQuery): Record<string, string | number> {
  const params: Record<string, string | number> = {}
  const put = (key: string, value: string | number | undefined) => {
    if (value === undefined || value === '') return
    params[key] = value
  }
  put('mode', query.mode)
  put('metrics', query.metrics && query.metrics.length ? query.metrics.join(',') : undefined)
  put('group_by', query.groupBy)
  put('series', query.series)
  put('x', query.x)
  put('y', query.y)
  put('size', query.size)
  put('filter', query.filter)
  put('sort', query.sort)
  put('limit', query.limit)
  if (query.withTotal) params.with_total = 1
  return params
}

export function toAggregateResult(value: unknown): AggregateResult {
  const raw = collectionWire(value)
  const total = Number(raw.total)
  return {
    columns: Array.isArray(raw.columns) ? raw.columns.map(String) : [],
    rows: Array.isArray(raw.rows) ? (raw.rows as unknown[][]) : [],
    // 后端不提供时是 undefined（不是 0）：调用方要区分「总量为 0」与「没有总量」。
    ...(raw.total === undefined || raw.total === null || !Number.isFinite(total) ? {} : { total }),
    truncated: raw.truncated === true,
    scanned: Number(raw.scanned ?? 0),
    durationMs: Number(raw.duration_ms ?? 0),
    requestId: String(raw.request_id ?? '')
  }
}

export function toSchemaDryRun(value: unknown): SchemaDryRunResult {
  const raw = collectionWire(value)
  const invalid = Array.isArray(raw.invalid) ? raw.invalid : []
  return {
    invalid: invalid.map((entry) => {
      const item = collectionWire(entry)
      return { id: String(item.id ?? ''), field: String(item.field ?? ''), reason: String(item.reason ?? '') }
    }),
    scanned: Number(raw.scanned ?? 0),
    truncated: raw.truncated === true
  }
}

export function dataCollectionsPath(projectId: string, databaseId: string, collection?: string) {
  let path =
    '/v1/projects/' +
    encodeURIComponent(projectId) +
    '/databases/' +
    encodeURIComponent(databaseId) +
    '/data/collections'
  if (collection) path += '/' + encodeURIComponent(collection)
  return path
}

/** KV 单端点路径（key-value-ducklake-plan §3：项目级，无 databaseId） */
export function kvPath(projectId: string) {
  return '/v1/projects/' + encodeURIComponent(projectId) + '/kv'
}

export function toProjectItem(raw: any): import('./types').ProjectItem {
  return {
    id: String(raw?.id ?? ''),
    name: String(raw?.name ?? ''),
    engine: raw?.engine === 'sqlite' ? 'sqlite' : raw?.engine === 'duckdb' ? 'duckdb' : undefined,
    createdAt: String(raw?.created_at ?? raw?.createdAt ?? '')
  }
}

/** ApiKeyItem：后端 snake_case → UI camelCase */
export function toApiKeyItem(raw: any): ApiKeyItem {
  return {
    id: String(raw?.id ?? ''),
    projectId: String(raw?.project_id ?? ''),
    permissions: Array.isArray(raw?.permissions) ? raw.permissions.map(String) : [],
    createdAt: String(raw?.created_at ?? ''),
    revokedAt: raw?.revoked_at || undefined
  }
}

/** AuthUser：后端 snake_case → UI camelCase */
export function toAuthUser(d: Record<string, any>): AuthUser {
  return {
    id: d.id,
    username: d.username,
    role: d.role,
    displayName: d.display_name || '',
    email: d.email || '',
    status: d.status,
    mustChangePassword: Boolean(d.must_change_password),
    createdBy: d.created_by || '',
    createdAt: d.created_at || '',
    lastLoginAt: d.last_login_at || ''
  }
}

export function toTokenPair(d: Record<string, any>): TokenPair {
  return {
    tokenType: d.token_type || 'Bearer',
    accessToken: d.access_token || '',
    expiresIn: d.expires_in || 0,
    refreshToken: d.refresh_token || '',
    user: toAuthUser(d.user || {})
  }
}

export function toUserItem(d: Record<string, any>): UserItem {
  return { ...toAuthUser(d), projectCount: d.project_count ?? 0 }
}

/** 真实后端实现（路径契约以 internal/api/router.go 为准，见 plan/planv6.7/legacy-plan-consolidation-plan.md §4.2） */
