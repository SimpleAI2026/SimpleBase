import { http, baseURL, getApiKey, getAccessToken, setTokens, clearTokens } from './http'
import { t } from '@/locales'
import type {
  Api,
  APIPreviewResult,
  OpenAPIDocument,
  AgentMessage,
  AgentModuleInfo,
  AgentRun,
  ActiveAgentRun,
  AgentRunMetrics,
  AgentRunRequest,
  AgentSchedule,
  AgentScheduleRun,
  AgentStreamHandlers,
  AgentUsageEventWire,
  AgentThread,
  ApiKeyItem,
  AuthUser,
  CloudAgent,
  CreateUserRequest,
  CronJobCreate,
  CronJobItem,
  CronJobRunItem,
  CronJobRunWire,
  CronJobWire,
  SandboxCapabilities,
  SandboxItem,
  SandboxFileEntry,
  StorageSettings,
  StorageUpdateResult,
  CreateDatabaseInput,
  DatabaseItem,
  BackupItem,
  CollectionInfo,
  CollectionSchema,
  DocumentItem,
  DocumentPage,
  SchemaDryRunResult,
  DatabaseSchema,
  SchemaColumn,
  SchemaTable,
  SchemaTableRows,
  GoFunctionItem,
  GoFuncVersionCreate,
  GoFuncVersionSummary,
  LlmChatRequest,
  LlmChatResponse,
  LlmSettings,
  LlmStreamHandlers,
  LlmStreamConnection,
  LogEvent,
  LogEventWire,
  LogRetentionWire,
  MetricsSummaryWire,
  TrendPointWire,
  LogQuery,
  LogRetention,
  MetricsSummary,
  SqlBatchRequest,
  SqlBatchResult,
  SqlExecuteResult,
  SqlQueryResult,
  SqlRequest,
  TokenPair,
  TrendPoint,
  UpdateUserRequest,
  UserItem,
  UserListResult,
  ViewDryRunResult,
  CollectionImportResult,
  FunctionTrigger,
  FunctionTriggerInput
} from './types'

/* ---------- 命名转换：UI camelCase ↔ 后端 snake_case（集中在此） ---------- */

function toLlmPayload(req: LlmChatRequest) {
  return {
    model: req.model,
    messages: req.messages,
    max_tokens: req.maxTokens,
    temperature: req.temperature
  }
}

/**
 * OpenAI chat.completion → UI 的 LlmChatResponse。
 *
 * 仓库约定「跨层契约集中在 types.ts，不透传原始结构」（ui/AGENTS.md），因此把
 * choices[0] 展平成界面已有的扁平类型，useAiChat 与测试桩都无需感知嵌套。
 */
function toLlmChatResponse(raw: Record<string, any>): LlmChatResponse {
  const choice = raw?.choices?.[0]
  const usage = raw?.usage
  return {
    content: choice?.message?.content ?? '',
    usage: usage
      ? {
          prompt_tokens: usage.prompt_tokens,
          completion_tokens: usage.completion_tokens,
          total_tokens: usage.total_tokens
        }
      : undefined,
    model: raw?.model ?? '',
    provider: raw?.provider ?? '',
    finish_reason: choice?.finish_reason
  }
}

/** DatabaseItem：后端 snake_case → UI camelCase */
function toDatabaseItem(d: Record<string, any>): DatabaseItem {
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

function schemaText(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function toSchemaColumn(raw: unknown): SchemaColumn {
  const column = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : undefined
  const mapped: SchemaColumn = {
    name: schemaText(column?.name),
    type: schemaText(column?.type),
    nullable: column?.nullable !== false
  }
  if (column?.sensitive === true) mapped.sensitive = true
  return mapped
}

function toSchemaTableRows(raw: unknown): SchemaTableRows {
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

function toSchemaTable(raw: unknown): SchemaTable {
  const table = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : undefined
  const columns = Array.isArray(table?.columns) ? table.columns.map(toSchemaColumn) : []
  return { name: schemaText(table?.name), columns }
}

function toDatabaseSchema(raw: unknown): DatabaseSchema {
  const body = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : undefined
  const tables = Array.isArray(body?.tables) ? body.tables.map(toSchemaTable) : []
  return { tables }
}

function databaseResourcePath(projectId: string, databaseId: string, suffix: string) {
  return (
    '/v1/projects/' +
    encodeURIComponent(projectId) +
    '/databases/' +
    encodeURIComponent(databaseId) +
    suffix
  )
}

/** SQL 请求：UI camelCase → 后端 snake_case */
function toSqlPayload(req: SqlRequest) {
  return { sql: req.sql, args: req.args, max_rows: req.maxRows }
}

/** SQL query 响应：后端 → UI（rows 保持二维数组，由页面按 columns zip） */
function toQueryResult(r: Record<string, any>): SqlQueryResult {
  return {
    columns: r.columns ?? [],
    rows: r.rows ?? [],
    rowCount: r.row_count ?? 0,
    durationMs: r.duration_ms ?? 0,
    requestId: r.request_id ?? ''
  }
}

function toExecuteResult(r: Record<string, any>): SqlExecuteResult {
  return {
    rowsAffected: r.rows_affected ?? 0,
    durability: r.durability ?? '',
    durationMs: r.duration_ms ?? 0,
    requestId: r.request_id ?? ''
  }
}

function toBatchResult(r: Record<string, any>): SqlBatchResult {
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
async function readSse(
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

/** SSE 流式对话：fetch + ReadableStream 逐行解析 `data: {...}` 帧 */
function llmStream(
  projectId: string,
  req: LlmChatRequest,
  handlers: LlmStreamHandlers
): LlmStreamConnection {
  const controller = new AbortController()
  let ended = false
  const fireEnd = () => {
    if (!ended) {
      ended = true
      handlers.onEnd?.()
    }
  }
  ;(async () => {
    try {
      const resp = await fetch(
        `${baseURL}/v1/projects/${encodeURIComponent(projectId)}/v1/chat/completions`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${getApiKey()}`
          },
          body: JSON.stringify({ ...toLlmPayload(req), stream: true }),
          signal: controller.signal
        }
      )
      if (!resp.ok || !resp.body) {
        // 非 JSON 响应（如 SPA fallback HTML）时 json() 会抛错，需 catch 兜底
        const data = await resp.json().catch(() => null)
        throw new Error(
          (data as any)?.error?.message || (data as any)?.message || `请求失败 (${resp.status})`
        )
      }
      await readSse(
        resp,
        (obj) => {
          // OpenAI 分块：正文在 choices[0].delta.content。
          const text = obj?.choices?.[0]?.delta?.content ?? ''
          if (text) handlers.onChunk?.(text)
        },
        // 结束哨兵 `[DONE]` 是字面量而非 JSON，必须显式识别，否则 onEnd 永不触发。
        fireEnd
      )
      fireEnd()
    } catch (e) {
      if (!controller.signal.aborted) handlers.onError?.(e)
    }
  })()
  return { close: () => controller.abort() }
}

function agentPath(projectId: string, rest = '') {
  return '/v1/projects/' + encodeURIComponent(projectId) + rest
}

/** 云函数路径 */
function gofunctionsPath(projectId: string, name?: string, ver?: number | string, suffix?: string) {
  let path = '/v1/projects/' + encodeURIComponent(projectId) + '/gofunctions'
  if (name) path += '/' + encodeURIComponent(name)
  if (ver !== undefined) path += '/versions/' + encodeURIComponent(String(ver))
  if (suffix) path += '/' + suffix
  return path
}

/* v8 ignore start -- 防御性字段映射：空/满 payload 双向已有 sparse 测试 */
function toGoFuncVersion(raw: Record<string, any>) {
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
function toGoFunctionItem(raw: Record<string, any>): GoFunctionItem {
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
const E2B_BASE = '/e2b'
const ENVD_BASE = '/e2b/envd'
const SB_META_NAME = 'name'
const SB_META_SOURCE = 'simplebase.source'

function e2bHeaders(projectId: string): Record<string, string> {
  return { 'X-SimpleBase-Project': projectId }
}

function sandboxPath(id: string, suffix = ''): string {
  return E2B_BASE + '/sandboxes/' + encodeURIComponent(id) + suffix
}

function num(v: unknown): number {
  const n = Number(v)
  return Number.isFinite(n) ? n : 0
}

function toSandboxItem(raw: Record<string, unknown>): SandboxItem {
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
function toSandboxFileEntry(raw: Record<string, unknown>): SandboxFileEntry {
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
function envdRpc(projectId: string, id: string, method: string, body: Record<string, unknown>) {
  return http.post(ENVD_BASE + method, body, {
    headers: { ...e2bHeaders(projectId), 'Connect-Protocol-Version': '1', 'E2b-Sandbox-Id': id }
  })
}

/** 定时任务路径：/v1/projects/:pid/cron-jobs[/:jobId[/runs|/trigger]] */
function cronJobsPath(projectId: string, jobId?: string, suffix?: 'runs' | 'trigger') {
  let path = '/v1/projects/' + encodeURIComponent(projectId) + '/cron-jobs'
  if (jobId) {
    path += '/' + encodeURIComponent(jobId)
    if (suffix) path += '/' + suffix
  }
  return path
}

/** 未识别的调度类型回退 cron（旧数据兼容）。 */
function toCronScheduleKind(raw: unknown): CronJobItem['scheduleKind'] {
  if (raw === 'interval') return 'interval'
  if (raw === 'once') return 'once'
  return 'cron'
}

/** CronJobItem：后端 snake_case → UI camelCase（§3.13）。raw 可能缺失（响应体为空）。 */
function toCronJobItem(raw: CronJobWire | null | undefined): CronJobItem {
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

function toCronJobRunItem(raw: CronJobRunWire | null | undefined): CronJobRunItem {
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
function cronJobBody(body: Partial<CronJobCreate>) {
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

function optionalNumber(v: unknown): number | null {
  if (v === null || v === undefined || v === '') return null
  const n = Number(v)
  return Number.isFinite(n) ? n : null
}

function toMetricsSummary(d: unknown): MetricsSummary {
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
  return Array.isArray(points) ? points : []
}

function toTrendPoints(d: unknown): TrendPoint[] {
  return trendPointList(d).map((item) => {
    const p = item as TrendPointWire | null | undefined
    return {
      date: String(p?.date ?? ''),
      requests: Number(p?.requests ?? 0),
      errors: Number(p?.errors ?? 0)
    }
  })
}

function logQueryParams(q?: LogQuery) {
  if (!q) return undefined
  const params: Record<string, string | number> = {}
  if (q.level) params.level = q.level
  if (q.q) params.q = q.q
  if (q.from) params.from = q.from
  if (q.to) params.to = q.to
  if (q.limit) params.limit = q.limit
  return params
}

function toLogEvent(raw: LogEventWire | null | undefined): LogEvent {
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

function toLogRetention(raw: unknown): LogRetention {
  const d: LogRetentionWire = raw && typeof raw === 'object' ? raw : {}
  return {
    scope: String(d.scope ?? d.Scope ?? ''),
    keepDays: Number(d.keep_days ?? d.keepDays ?? d.KeepDays ?? 14),
    updatedAt: String(d.updated_at ?? d.updatedAt ?? d.UpdatedAt ?? '')
  }
}

function toLlmSettings(raw: Record<string, any> | undefined): LlmSettings {
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

function toCloudAgent(raw: Record<string, any>): CloudAgent {
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
    writable: !!raw?.writable,
    created_at: String(raw?.created_at ?? ''),
    updated_at: String(raw?.updated_at ?? '')
  }
}

function toAgentThread(raw: Record<string, any>): AgentThread {
  return {
    id: String(raw?.id ?? ''),
    title: String(raw?.title ?? ''),
    agent_id: raw?.agent_id ? String(raw.agent_id) : undefined,
    created_at: String(raw?.created_at ?? ''),
    updated_at: String(raw?.updated_at ?? ''),
    last_message_preview: raw?.last_message_preview ? String(raw.last_message_preview) : undefined
  }
}

function toAgentMessage(raw: Record<string, any>): AgentMessage {
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
    created_at: String(raw?.created_at ?? '')
  }
}

function toAgentSchedule(raw: Record<string, any>): AgentSchedule {
  return {
    id: String(raw?.id ?? ''),
    agent_id: String(raw?.agent_id ?? ''),
    agent_name: raw?.agent_name || undefined,
    thread_id: String(raw?.thread_id ?? ''),
    prompt: String(raw?.prompt ?? ''),
    cron_expr: String(raw?.cron_expr ?? ''),
    enabled: !!raw?.enabled,
    last_run_at: raw?.last_run_at || undefined,
    next_run_at: raw?.next_run_at || undefined,
    created_by: raw?.created_by || undefined,
    created_at: String(raw?.created_at ?? ''),
    updated_at: String(raw?.updated_at ?? '')
  }
}

function toAgentScheduleRun(raw: Record<string, any>): AgentScheduleRun {
  return {
    id: String(raw?.id ?? ''),
    schedule_id: String(raw?.schedule_id ?? ''),
    run_id: String(raw?.run_id ?? ''),
    trigger: String(raw?.trigger ?? ''),
    status: String(raw?.status ?? ''),
    error: raw?.error || undefined,
    started_at: raw?.started_at || undefined,
    finished_at: raw?.finished_at || undefined,
    created_at: String(raw?.created_at ?? '')
  }
}

/** usage 帧 → 运行指标。`||` 兜底仅覆盖 falsy；非数字字符串仍得 NaN（保持现状）。 */
function toAgentRunMetrics(obj: AgentUsageEventWire): AgentRunMetrics {
  return {
    duration_ms: Number(obj.duration_ms || 0),
    prompt_tokens: Number(obj.prompt_tokens || 0),
    completion_tokens: Number(obj.completion_tokens || 0),
    reasoning_tokens: Number(obj.reasoning_tokens || 0),
    tool_calls: Number(obj.tool_calls || 0)
  }
}

function streamAgentRun(
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
        ...(req.retry_of_run_id ? { retry_of_run_id: req.retry_of_run_id } : {})
      }),
      signal
    })
  )
}

/** 续接进行中的 run：从头回放事件缓存（after=0）并跟随直到 end（切换界面后回到会话时使用）。 */
function attachAgentRun(projectId: string, runId: string, handlers: AgentStreamHandlers): LlmStreamConnection {
  return openAgentRunStream(projectId, handlers, runId, (signal) => fetchAgentRunEvents(projectId, runId, '', signal))
}

function fetchAgentRunEvents(projectId: string, runId: string, after: string, signal: AbortSignal) {
  return fetch(
    `${baseURL}${agentPath(projectId, '/agent-runs/' + encodeURIComponent(runId) + '/events')}?after=${encodeURIComponent(after)}`,
    {
      method: 'GET',
      headers: { Authorization: `Bearer ${getAccessToken() || getApiKey()}` },
      signal
    }
  )
}

function openAgentRunStream(
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
    if (obj?.type === 'end') {
      fireEnd(typeof obj.reason === 'string' ? obj.reason : undefined)
      return
    }
    if (obj?.type === 'run' && obj.run_id) {
      runId = String(obj.run_id)
      handlers.onRun?.(runId)
      return
    }
    if (obj?.type === 'error') {
      failed = true
      const error = new Error(String(obj.message || '运行失败')) as Error & { code?: string }
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
        throw new Error('接口不存在或返回了 HTML 页面')
      }
      if (!resp.ok || !resp.body) {
        const data = await resp.json().catch(() => null)
        const detail = (data as { error?: { code?: string; message?: string }; message?: string } | null)
        const error = new Error(detail?.error?.message || detail?.message || `请求失败 (${resp.status})`) as Error & { code?: string }
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
          if (!resp.ok || !resp.body) throw new Error(`请求失败 (${resp.status})`)
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

function collectionWire(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : {}
}

function toBackupItem(value: unknown): BackupItem {
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

function toCollectionInfo(value: unknown): CollectionInfo {
  const raw = collectionWire(value)
  return {
    name: String(raw.name ?? ''),
    schemaMode: raw.schema_mode === 'view' ? 'view' : 'json',
    fieldCount: Number(raw.field_count ?? 0),
    documentCount: Number(raw.document_count ?? 0),
    registered: raw.registered === true
  }
}

function toCollectionSchema(value: unknown): CollectionSchema {
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

function toFunctionTrigger(value: unknown): FunctionTrigger {
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

function toTriggerWire(input: Partial<FunctionTriggerInput>) {
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

function toDocumentItem(value: unknown): DocumentItem {
  const raw = collectionWire(value)
  return {
    id: String(raw.id ?? ''),
    createdAt: String(raw.created_at ?? ''),
    updatedAt: String(raw.updated_at ?? raw.created_at ?? ''),
    data: collectionWire(raw.data)
  }
}

function toDocumentPage(value: unknown): DocumentPage {
  const raw = collectionWire(value)
  return {
    rows: Array.isArray(raw.rows) ? (raw.rows as DocumentPage['rows']) : [],
    items: Array.isArray(raw.items) ? raw.items.map(toDocumentItem) : [],
    nextCursor: raw.next_cursor ? String(raw.next_cursor) : undefined,
    total: typeof raw.total === 'number' ? raw.total : undefined
  }
}

function toSchemaDryRun(value: unknown): SchemaDryRunResult {
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

function dataCollectionsPath(projectId: string, databaseId: string, collection?: string) {
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
function kvPath(projectId: string) {
  return '/v1/projects/' + encodeURIComponent(projectId) + '/kv'
}

function toProjectItem(raw: any): import('./types').ProjectItem {
  return {
    id: String(raw?.id ?? ''),
    name: String(raw?.name ?? ''),
    engine: raw?.engine === 'sqlite' ? 'sqlite' : raw?.engine === 'duckdb' ? 'duckdb' : undefined,
    createdAt: String(raw?.created_at ?? raw?.createdAt ?? '')
  }
}

/** ApiKeyItem：后端 snake_case → UI camelCase */
function toApiKeyItem(raw: any): ApiKeyItem {
  return {
    id: String(raw?.id ?? ''),
    projectId: String(raw?.project_id ?? ''),
    permissions: Array.isArray(raw?.permissions) ? raw.permissions.map(String) : [],
    createdAt: String(raw?.created_at ?? ''),
    revokedAt: raw?.revoked_at || undefined
  }
}

/** AuthUser：后端 snake_case → UI camelCase */
function toAuthUser(d: Record<string, any>): AuthUser {
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

function toTokenPair(d: Record<string, any>): TokenPair {
  return {
    tokenType: d.token_type || 'Bearer',
    accessToken: d.access_token || '',
    expiresIn: d.expires_in || 0,
    refreshToken: d.refresh_token || '',
    user: toAuthUser(d.user || {})
  }
}

function toUserItem(d: Record<string, any>): UserItem {
  return { ...toAuthUser(d), projectCount: d.project_count ?? 0 }
}

/** 真实后端实现（路径契约见 plan/planv2.0/proto-http.md） */
export const httpApi: Api = {
  preview: {
    document: () => http.get<OpenAPIDocument>('/v1/openapi.json').then((r) => r.data),
    request: (method, path, body) =>
      http.request({ method, url: path, data: body, validateStatus: () => true }).then((r): APIPreviewResult => ({
        status: r.status,
        body: typeof r.data === 'string' ? r.data : JSON.stringify(r.data ?? null, null, 2)
      }))
  },
  auth: {
    login: (req) =>
      http
        .post('/v1/auth/login', { username: req.username, password: req.password })
        .then((r) => {
          const pair = toTokenPair(r.data)
          if (pair.accessToken) setTokens(pair.accessToken, pair.refreshToken)
          return pair
        }),
    logout: (refreshToken) =>
      http
        .post('/v1/auth/logout', { refresh_token: refreshToken || undefined })
        .then(() => {
          clearTokens()
        }),
    me: () =>
      http.get('/v1/auth/me').then((r) => {
        const d = r.data || {}
        const projects = Array.isArray(d.projects)
          ? d.projects.map((p: Record<string, any>) => ({
              id: p.id,
              name: p.name || '',
              owner: Boolean(p.owner)
            }))
          : []
        return { ...toAuthUser(d), projects }
      }),
    changePassword: (oldPassword, newPassword) =>
      http
        .put('/v1/auth/password', { old_password: oldPassword, new_password: newPassword })
        .then(() => undefined)
  },
  users: {
    list: (limit, cursor) =>
      http
        .get('/v1/users', { params: { limit: limit || 20, cursor: cursor || undefined } })
        .then((r): UserListResult => {
          const list = Array.isArray(r.data?.users) ? r.data.users : []
          return { users: list.map(toUserItem), nextCursor: r.data?.next_cursor || '' }
        }),
    create: (req: CreateUserRequest) =>
      http
        .post('/v1/users', {
          username: req.username,
          password: req.password,
          role: req.role,
          display_name: req.displayName || '',
          email: req.email || ''
        })
        .then((r) => toUserItem({ ...r.data, project_count: 0 })),
    update: (id, req: UpdateUserRequest) =>
      http
        .patch('/v1/users/' + encodeURIComponent(id), {
          role: req.role,
          display_name: req.displayName,
          email: req.email,
          status: req.status,
          password: req.password,
          must_change_password: req.mustChangePassword
        })
        .then((r) => toUserItem(r.data))
  },
  triggers: {
    list: (projectId) => http.get(`/v1/projects/${encodeURIComponent(projectId)}/function-triggers`)
      .then((r) => Array.isArray(r.data?.items) ? r.data.items.map(toFunctionTrigger) : []),
    create: (projectId, input) => http.post(`/v1/projects/${encodeURIComponent(projectId)}/function-triggers`, toTriggerWire(input))
      .then((r) => toFunctionTrigger(r.data)),
    patch: (projectId, id, input) => http.patch(`/v1/projects/${encodeURIComponent(projectId)}/function-triggers/${encodeURIComponent(id)}`, toTriggerWire(input))
      .then((r) => toFunctionTrigger(r.data)),
    remove: (projectId, id) => http.delete(`/v1/projects/${encodeURIComponent(projectId)}/function-triggers/${encodeURIComponent(id)}`).then(() => undefined),
    invoke: (projectId, id, body) => http.post(`/v1/projects/${encodeURIComponent(projectId)}/function-triggers/${encodeURIComponent(id)}/invoke`, body).then(() => undefined)
  },
  gofunctions: {
    list: (projectId) =>
      http.get(gofunctionsPath(projectId)).then((r) => {
        const list = Array.isArray(r.data?.functions) ? r.data.functions : []
        return list.map((raw: Record<string, any>) => toGoFunctionItem(raw))
      }),
    create: (projectId, body) =>
      http
        .post(gofunctionsPath(projectId), {
          name: body.name,
          source: body.source,
          description: body.description,
          note: body.note,
          activate: body.activate
        })
        .then((r) => toGoFunctionItem(r.data)),
    get: (projectId, name) =>
      http.get(gofunctionsPath(projectId, name)).then((r) => toGoFunctionItem(r.data)),
    saveVersion: (projectId, name, body) =>
      http
        .post(gofunctionsPath(projectId, name, undefined, 'versions'), {
          source: body.source,
          note: body.note,
          activate: body.activate
        })
        .then((r) => toGoFunctionItem(r.data)),
    remove: (projectId, name) =>
      http.delete(gofunctionsPath(projectId, name)).then(() => undefined),
    listVersions: (projectId, name) =>
      http.get(gofunctionsPath(projectId, name, undefined, 'versions')).then((r) => ({
        activeVersion: Number(r.data?.active_version ?? 0),
        versions: Array.isArray(r.data?.versions) ? r.data.versions.map(toGoFuncVersion) : []
      })),
    activate: (projectId, name, version) =>
      http
        .post(gofunctionsPath(projectId, name, version, 'activate'))
        .then((r) => ({ activeVersion: Number(r.data?.active_version ?? version) })),
    test: (projectId, name, version, functionName, body) =>
      http
        .post(gofunctionsPath(projectId, name, version, 'test'), {
          function_name: functionName,
          body
        })
        .then((r) => ({
          ok: Boolean(r.data?.ok),
          statusCode: Number(r.data?.status_code ?? 0),
          durationMs: Number(r.data?.duration_ms ?? 0),
          version: Number(r.data?.version ?? version),
          activeVersion: Number(r.data?.active_version ?? 0),
          functionName: String(r.data?.function_name ?? functionName),
          data: r.data?.data,
          error: r.data?.error || ''
        }))
  },
  sandboxes: {
    capabilities: (projectId) => http.get(E2B_BASE + '/ext/capabilities', { headers: e2bHeaders(projectId) }).then((r) => {
      const raw = r.data as Record<string, unknown>
      const ssh = raw.ssh as Record<string, unknown> | undefined
      return {
        available: Boolean(raw.available),
        backend: String(raw.backend ?? ''),
        reason: raw.reason ? String(raw.reason) : undefined,
        images: Array.isArray(raw.images) ? raw.images.map(String) : [],
        defaultImage: String(raw.default_image ?? ''),
        defaultTemplate: raw.default_template ? String(raw.default_template) : undefined,
        cpus: num(raw.cpus),
        memoryMiB: num(raw.memory_mib),
        timeoutDefaultS: num(raw.timeout_default_s),
        timeoutMaxS: num(raw.timeout_max_s),
        maxFileBytes: num(raw.max_file_bytes),
        maxOutputBytes: num(raw.max_output_bytes),
        maxPerProject: num(raw.max_per_project),
        terminal: Boolean(raw.terminal),
        ssh: ssh
          ? {
              enabled: Boolean(ssh.enabled),
              listen: ssh.listen ? String(ssh.listen) : undefined,
              hostKeyFingerprint: ssh.host_key_fingerprint ? String(ssh.host_key_fingerprint) : undefined
            }
          : undefined,
        envdVersion: raw.envd_version ? String(raw.envd_version) : undefined
      } satisfies SandboxCapabilities
    }),
    // GET /e2b/v2/sandboxes：X-Next-Token 分页，单页上限 100；项目上限同为 100，这里取完全部页。
    list: async (projectId, state, metadataFilter) => {
      const out: SandboxItem[] = []
      let nextToken: string | undefined
      const metadata = metadataFilter
        ? Object.entries(metadataFilter).map(([k, v]) => encodeURIComponent(k) + '=' + encodeURIComponent(v)).join('&')
        : undefined
      for (let page = 0; page < 20; page++) {
        const r = await http.get(E2B_BASE + '/v2/sandboxes', {
          headers: e2bHeaders(projectId),
          params: { state, metadata, limit: 100, nextToken }
        })
        const rows: Record<string, unknown>[] = Array.isArray(r.data) ? r.data : []
        out.push(...rows.map((raw) => toSandboxItem(raw)))
        const next = r.headers?.['x-next-token']
        nextToken = next ? String(next) : undefined
        if (!nextToken) break
      }
      return out
    },
    get: (projectId, id) => http.get(sandboxPath(id), { headers: e2bHeaders(projectId) }).then((r) => toSandboxItem(r.data)),
    create: (projectId, body) => {
      const metadata: Record<string, string> = { ...(body.metadata || {}) }
      if (body.name) metadata[SB_META_NAME] = body.name
      return http.post(E2B_BASE + '/sandboxes', {
        templateID: body.templateId || '',
        timeout: body.timeoutS,
        metadata
      }, { headers: e2bHeaders(projectId), timeout: 90000 }).then((r) => toSandboxItem(r.data))
    },
    remove: (projectId, id) => http.delete(sandboxPath(id), { headers: e2bHeaders(projectId) }).then(() => undefined),
    connect: (projectId, id, timeoutS) => http.post(sandboxPath(id, '/connect'), { timeout: timeoutS ?? 300 }, {
      headers: e2bHeaders(projectId), timeout: 90000
    }).then((r) => toSandboxItem(r.data)),
    pause: (projectId, id) => http.post(sandboxPath(id, '/pause'), undefined, { headers: e2bHeaders(projectId) }).then(() => undefined),
    terminalTicket: (projectId, id) => http.post(E2B_BASE + '/ext/sandboxes/' + encodeURIComponent(id) + '/terminal-tickets', undefined, {
      headers: e2bHeaders(projectId)
    }).then((r) => ({ ticket: String(r.data?.ticket ?? ''), expiresIn: num(r.data?.expires_in) })),
    // 浏览器 WebSocket 不能带 Authorization，凭一次性票据升级；协议随页面（https→wss）。
    terminalWebSocketUrl: (id, ticket) => {
      const base = new URL(baseURL || window.location.origin, window.location.origin)
      base.protocol = base.protocol === 'https:' ? 'wss:' : 'ws:'
      base.pathname = E2B_BASE + '/ext/sandboxes/' + encodeURIComponent(id) + '/terminal'
      base.search = '?ticket=' + encodeURIComponent(ticket)
      return base.toString()
    },
    files: {
      // envd filesystem.Filesystem/ListDir（Connect-JSON）
      list: (projectId, id, path, depth) => envdRpc(projectId, id, '/filesystem.Filesystem/ListDir', { path, depth: depth ?? 1 })
        .then((r) => (Array.isArray(r.data?.entries) ? r.data.entries : []).map((raw: Record<string, unknown>) => toSandboxFileEntry(raw))),
      // envd GET /files?path=
      read: (projectId, id, path) => http.get<Blob>(ENVD_BASE + '/files', {
        headers: { ...e2bHeaders(projectId), 'E2b-Sandbox-Id': id },
        params: { path }, responseType: 'blob'
      }).then((r) => r.data),
      // envd POST /files?path=（application/octet-stream，自动建父目录、覆盖已有文件）
      write: (projectId, id, path, content) => http.post(
        ENVD_BASE + '/files',
        typeof content === 'string' ? new Blob([content], { type: 'application/octet-stream' }) : content,
        {
          headers: { ...e2bHeaders(projectId), 'E2b-Sandbox-Id': id, 'Content-Type': 'application/octet-stream' },
          params: { path }
        }
      ).then(() => undefined),
      remove: (projectId, id, path) => envdRpc(projectId, id, '/filesystem.Filesystem/Remove', { path }).then(() => undefined)
    }
  },
  cronjobs: {
    list: (projectId) =>
      http.get(cronJobsPath(projectId)).then((r) => {
        const list = Array.isArray(r.data?.jobs) ? r.data.jobs : []
        return list.map((raw: CronJobWire) => toCronJobItem(raw))
      }),
    create: (projectId, body) =>
      http.post(cronJobsPath(projectId), cronJobBody(body)).then((r) => toCronJobItem(r.data)),
    update: (projectId, jobId, body) =>
      http.patch(cronJobsPath(projectId, jobId), cronJobBody(body)).then((r) => toCronJobItem(r.data)),
    remove: (projectId, jobId) =>
      http.delete(cronJobsPath(projectId, jobId)).then(() => undefined),
    runs: (projectId, jobId, limit) =>
      http
        .get(cronJobsPath(projectId, jobId, 'runs'), { params: limit ? { limit } : undefined })
        .then((r) => {
          const list = Array.isArray(r.data?.runs) ? r.data.runs : []
          return list.map((raw: CronJobRunWire) => toCronJobRunItem(raw))
        }),
    trigger: (projectId, jobId) =>
      http.post(cronJobsPath(projectId, jobId, 'trigger')).then(() => undefined)
  },
  projects: {
    list: () =>
      http.get('/v1/projects').then((r) => {
        const list = Array.isArray(r.data?.projects) ? r.data.projects : []
        return list.map(toProjectItem)
      }),
    create: (req) =>
      http
        .post('/v1/projects', {
          name: req.name,
          id: req.id || undefined,
          engine: req.engine || undefined,
          owner_user_id: req.ownerUserId || undefined
        })
        .then((r) => toProjectItem(r.data))
  },
  apiKeys: {
    list: (projectId) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/api-keys')
        .then((r) => {
          const list = Array.isArray(r.data?.keys) ? r.data.keys : []
          return list.map(toApiKeyItem)
        }),
    create: (projectId, permissions, scope) =>
      http
        .post('/v1/projects/' + encodeURIComponent(projectId) + '/api-keys', {
          permissions: permissions && permissions.length ? permissions : undefined,
          scope: scope ? {
            resources: scope.resources,
            actions: scope.actions,
            hidden_fields: scope.hiddenFields,
            readonly_fields: scope.readonlyFields,
            expires_at: scope.expiresAt
          } : undefined
        })
        .then((r) => {
          const item = toApiKeyItem(r.data) as ApiKeyItem & { secret: string }
          return { ...item, secret: String(r.data?.secret || '') }
        }),
    revoke: (projectId, keyId) =>
      http
        .delete('/v1/projects/' + encodeURIComponent(projectId) + '/api-keys/' + encodeURIComponent(keyId))
        .then(() => undefined)
  },
  metrics: {
    summary: (projectId) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/metrics/summary')
        .then((r) => toMetricsSummary(r.data)),
    trend: (projectId) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/metrics/trend')
        .then((r) => toTrendPoints(r.data))
  },

  databases: {
    list: (projectId) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/databases', { params: { limit: 200 } })
        .then((r) => {
          const list = Array.isArray(r.data?.databases) ? r.data.databases : []
          return list.map(toDatabaseItem)
        }),
    create: (projectId, input: CreateDatabaseInput) =>
      http
        .post('/v1/projects/' + encodeURIComponent(projectId) + '/databases', {
          name: input.name,
          data_model: input.dataModel,
          init_sql: input.initSql
        })
        .then((r) => toDatabaseItem(r.data)),
    remove: (projectId, databaseId) =>
      http
        .delete(
          '/v1/projects/' +
            encodeURIComponent(projectId) +
            '/databases/' +
            encodeURIComponent(databaseId)
        )
        .then(() => undefined),
    schema: (projectId, databaseId) =>
      http
        .get(databaseResourcePath(projectId, databaseId, '/schema'))
        .then((r) => toDatabaseSchema(r.data)),
    tableRows: (projectId, databaseId, table, query) =>
      http
        .get(databaseResourcePath(projectId, databaseId, '/schema/tables/' + encodeURIComponent(table) + '/rows'), {
          params: { limit: query.limit, offset: query.offset }
        })
        .then((r) => toSchemaTableRows(r.data)),
    createTable: (projectId, databaseId, table) =>
      http
        .post(databaseResourcePath(projectId, databaseId, '/schema/tables'), {
          name: table.name,
          columns: table.columns.map((column) => ({
            name: column.name,
            type: column.type,
            nullable: column.nullable
          }))
        })
        .then((r) => toSchemaTable(r.data)),
    addColumn: (projectId, databaseId, column) =>
      http
        .post(databaseResourcePath(projectId, databaseId, '/schema/columns'), {
          table: column.table,
          name: column.name,
          type: column.type,
          nullable: column.nullable
        })
        .then((r) => toSchemaColumn(r.data))
  },

  sql: {
    query: (projectId, databaseId, req) =>
      http
        .post(
          '/v1/projects/' +
            encodeURIComponent(projectId) +
            '/databases/' +
            encodeURIComponent(databaseId) +
            '/query',
          toSqlPayload(req)
        )
        .then((r) => toQueryResult(r.data)),
    execute: (projectId, databaseId, req) =>
      http
        .post(
          '/v1/projects/' +
            encodeURIComponent(projectId) +
            '/databases/' +
            encodeURIComponent(databaseId) +
            '/execute',
          toSqlPayload(req)
        )
        .then((r) => toExecuteResult(r.data)),
    batch: (projectId, databaseId, req) =>
      http
        .post(
          '/v1/projects/' +
            encodeURIComponent(projectId) +
            '/databases/' +
            encodeURIComponent(databaseId) +
            '/batch',
          {
            statements: req.statements.map(toSqlPayload),
            transactional: req.transactional
          }
        )
        .then((r) => toBatchResult(r.data))
  },

  db: {
    collections: (projectId, databaseId) =>
      http.get(dataCollectionsPath(projectId, databaseId)).then((r) =>
        Array.isArray(r.data?.collections) ? r.data.collections : []
      ),
    createCollection: (projectId, databaseId, name) =>
      http.post(dataCollectionsPath(projectId, databaseId), { name }).then(() => undefined),
    rows: (projectId, databaseId, collection, query) =>
      http
        .get(dataCollectionsPath(projectId, databaseId, collection), { params: query })
        .then((r) => (Array.isArray(r.data?.rows) ? r.data.rows : [])),
    insert: (projectId, databaseId, collection, payload) =>
      http
        .post(dataCollectionsPath(projectId, databaseId, collection) + '/documents', payload)
        .then((r) => r.data),
    update: (projectId, databaseId, collection, id, payload) =>
      http
        .put(
          dataCollectionsPath(projectId, databaseId, collection) +
            '/documents/' +
            encodeURIComponent(id),
          payload
        )
        .then((r) => r.data),
    remove: (projectId, databaseId, collection, id) =>
      http
        .delete(
          dataCollectionsPath(projectId, databaseId, collection) +
            '/documents/' +
            encodeURIComponent(id)
        )
        .then(() => undefined),
    collectionItems: (projectId, databaseId) =>
      http.get(dataCollectionsPath(projectId, databaseId)).then((r) =>
        Array.isArray(r.data?.items) ? r.data.items.map(toCollectionInfo) : []
      ),
    exportCollection: (projectId, databaseId, collection, format, query) =>
      http.get(dataCollectionsPath(projectId, databaseId, collection) + '/export', {
        params: { ...query, format }, responseType: 'blob'
      }).then((r) => r.data as Blob),
    importCollection: (projectId, databaseId, collection, file, format, dryRun = false) =>
      http.post(dataCollectionsPath(projectId, databaseId, collection) + '/import', file, {
        params: { dry_run: dryRun ? 1 : undefined },
        headers: { 'Content-Type': format === 'csv' ? 'text/csv' : 'application/x-ndjson' }
      }).then((r): CollectionImportResult => ({
        dryRun: r.data?.dry_run === true,
        scanned: Number(r.data?.scanned ?? 0),
        imported: Number(r.data?.imported ?? 0)
      })),
    dryRunView: (projectId, databaseId, name, query) =>
      http.post(dataCollectionsPath(projectId, databaseId) + '/meta/dry-run-view', { name, query })
        .then((r): ViewDryRunResult => ({
          columns: Array.isArray(r.data?.columns) ? r.data.columns.map(String) : [],
          fields: Array.isArray(r.data?.fields) ? r.data.fields.map((field: unknown) => toCollectionSchema({ fields: [field] }).fields[0]) : [],
          items: Array.isArray(r.data?.items) ? r.data.items.map(toDocumentItem) : [],
          scanned: Number(r.data?.scanned ?? 0)
        })),
    documents: (projectId, databaseId, collection, query) =>
      http
        .get(dataCollectionsPath(projectId, databaseId, collection), { params: query })
        .then((r) => toDocumentPage(r.data)),
    document: (projectId, databaseId, collection, id) =>
      http
        .get(
          dataCollectionsPath(projectId, databaseId, collection) + '/documents/' + encodeURIComponent(id)
        )
        .then((r) => toDocumentItem(r.data)),
    patch: (projectId, databaseId, collection, id, payload) =>
      http
        .patch(
          dataCollectionsPath(projectId, databaseId, collection) + '/documents/' + encodeURIComponent(id),
          payload
        )
        .then((r) => r.data),
    batch: (projectId, databaseId, collection, documents) =>
      http
        .post(dataCollectionsPath(projectId, databaseId, collection) + '/documents/batch', { documents })
        .then((r) => Array.isArray(r.data?.items) ? r.data.items.map(toDocumentItem) : []),
    truncate: (projectId, databaseId, collection) =>
      http.delete(dataCollectionsPath(projectId, databaseId, collection)).then(() => undefined),
    getCollectionSchema: (projectId, databaseId, collection) =>
      http
        .get(dataCollectionsPath(projectId, databaseId, collection) + '/meta')
        .then((r) => toCollectionSchema(r.data)),
    putCollectionSchema: (projectId, databaseId, schema, dryRun = false) =>
      http
        .put(
          dataCollectionsPath(projectId, databaseId, schema.name) + '/meta',
          {
            name: schema.name,
            schema_mode: schema.schemaMode || 'json',
            view_query: schema.viewQuery,
            fields: schema.fields.map((field) => ({
              ...field,
              options: {
                min: field.options.min,
                max: field.options.max,
                pattern: field.options.pattern,
                values: field.options.values,
                max_select: field.options.maxSelect,
                max_size: field.options.maxSize,
                only_int: field.options.onlyInt,
                except_domains: field.options.exceptDomains,
                only_domains: field.options.onlyDomains
              }
            }))
          },
          { params: { dry_run: dryRun ? 1 : undefined } }
        )
        .then((r) => Array.isArray(r.data?.invalid) ? toSchemaDryRun(r.data) : toCollectionSchema(r.data)),
    deleteCollectionSchema: (projectId, databaseId, collection) =>
      http.delete(dataCollectionsPath(projectId, databaseId, collection) + '/meta').then(() => undefined)
  },

  kv: {
    /** 项目级单端点：读/结构命令 type=cmd，按类型写入 type=String 等 */
    exec: (projectId, body) =>
      http.post(kvPath(projectId), body).then((r) => r.data),
    /** 顺序执行多条命令（非事务）；任一失败即停止 */
    execBatch: (projectId, bodies) =>
      Promise.all(bodies.map((b) => http.post(kvPath(projectId), b).then((r) => r.data)))
  },

  s3: {
    list: (projectId, prefix) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/s3/objects', {
          params: { prefix }
        })
        .then((r) => (Array.isArray(r.data) ? r.data : [])),
    listDetailed: (projectId, prefix) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/s3/objects', { params: { prefix, delimiter: '/' } })
        .then((r) => ({ folders: Array.isArray(r.data?.folders) ? r.data.folders : [], objects: Array.isArray(r.data?.objects) ? r.data.objects : [] })),
    presign: (projectId, key) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/s3/presign', { params: { key } })
        .then((r) => r.data),
    download: (projectId, key) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/s3/objects/content', { params: { key }, responseType: 'blob' })
        .then((r) => r.data as Blob),
    createLink: (projectId, key, ttlSeconds = 900) =>
      http
        .post('/v1/projects/' + encodeURIComponent(projectId) + '/s3/link', { key, ttl_seconds: ttlSeconds })
        .then((r) => ({ url: String(r.data?.url ?? ''), expiresAt: String(r.data?.expires_at ?? '') })),
    remove: (projectId, key) =>
      http
        .delete('/v1/projects/' + encodeURIComponent(projectId) + '/s3/objects', { params: { key } })
        .then(() => undefined),
    upload: (projectId, key, file, onProgress) => {
      const fd = new FormData()
      fd.append('key', key)
      fd.append('file', file)
      return http
        .post('/v1/projects/' + encodeURIComponent(projectId) + '/s3/objects', fd, {
          // 实例默认 15s，50MB 上传会先被掐断。
          timeout: 0,
          onUploadProgress: (e) => {
            if (onProgress && e.total) onProgress(Math.round((e.loaded / e.total) * 100))
          }
        })
        .then((r) => r.data)
    }
  },

  backups: {
    list: (projectId) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/backups')
        .then((r) => Array.isArray(r.data?.items) ? r.data.items.map(toBackupItem) : []),
    create: (projectId) =>
      http.post('/v1/projects/' + encodeURIComponent(projectId) + '/backups', {}).then((r) => toBackupItem(r.data)),
    remove: (projectId, id) =>
      http.delete('/v1/projects/' + encodeURIComponent(projectId) + '/backups/' + encodeURIComponent(id)).then(() => undefined),
    restore: (projectId, id) =>
      http.post('/v1/projects/' + encodeURIComponent(projectId) + '/backups/' + encodeURIComponent(id) + '/restore', {}).then(() => undefined),
    download: (projectId, id) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/backups/' + encodeURIComponent(id) + '/download', { responseType: 'blob' })
        .then((r) => r.data as Blob)
  },

  logs: {
    list: (projectId, q) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/logs', {
          params: logQueryParams(q)
        })
        .then((r) => {
          const list = Array.isArray(r.data?.events) ? r.data.events : Array.isArray(r.data) ? r.data : []
          return list.map(toLogEvent)
        }),
    stats: (projectId, q, groupBy = 'hour') =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/logs/stats', { params: { ...logQueryParams(q), group_by: groupBy } })
        .then((r) => Array.isArray(r.data?.items) ? r.data.items.map((item: unknown) => {
          const stat = collectionWire(item)
          return { key: String(stat.key ?? ''), count: Number(stat.count ?? 0) }
        }) : []),
    export: (projectId, q) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/logs/export', { params: logQueryParams(q), responseType: 'blob' })
        .then((r) => r.data as Blob),
    getRetention: (projectId) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/logs/retention')
        .then((r) => toLogRetention(r.data)),
    putRetention: (projectId, keepDays) =>
      http
        .put('/v1/projects/' + encodeURIComponent(projectId) + '/logs/retention', { keep_days: keepDays })
        .then(() => undefined)
  },

  quota: {
    status: (projectId) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/quota')
        .then((r) => ({
          llmAllowed: !!r.data?.llm_allowed,
          databaseAllowed: !!r.data?.database_allowed
        }))
  },

  llm: {
    chat: (projectId, req) =>
      http
        .post(
          '/v1/projects/' + encodeURIComponent(projectId) + '/v1/chat/completions',
          toLlmPayload(req)
        )
        .then((r) => toLlmChatResponse(r.data)),
    stream: llmStream
  },

  llmSettings: {
    get: (projectId) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/llm/settings')
        .then((r) => toLlmSettings(r.data)),
    put: (projectId, settings, apiKey) =>
      http
        .put('/v1/projects/' + encodeURIComponent(projectId) + '/llm/settings', {
          default_provider: settings.defaultProvider || '',
          default_model: settings.defaultModel || '',
          api_key: apiKey || '',
          base_url: settings.baseUrl || '',
          temperature: settings.temperature,
          max_tokens: settings.maxTokens
        })
        .then((r) => toLlmSettings(r.data))
  },

  agents: {
    modules: (projectId) =>
      http.get(agentPath(projectId, '/agents/modules')).then((r) => {
        const list = Array.isArray(r.data?.modules) ? r.data.modules : []
        return list as AgentModuleInfo[]
      }),
    list: (projectId) =>
      http.get(agentPath(projectId, '/agents')).then((r) => {
        const list = Array.isArray(r.data?.agents) ? r.data.agents : []
        return list.map(toCloudAgent)
      }),
    create: (projectId, body) =>
      http.post(agentPath(projectId, '/agents'), body).then((r) => toCloudAgent(r.data)),
    patch: (projectId, agentId, body) =>
      http
        .patch(agentPath(projectId, '/agents/' + encodeURIComponent(agentId)), body)
        .then((r) => toCloudAgent(r.data)),
    remove: (projectId, agentId) =>
      http.delete(agentPath(projectId, '/agents/' + encodeURIComponent(agentId))).then(() => undefined),
    models: (projectId) =>
      http.get(agentPath(projectId, '/agents/models')).then((r) => ({
        default_model: String(r.data?.default_model || ''),
        models: (Array.isArray(r.data?.models) ? r.data.models : []).map((m: Record<string, any>) => ({
          provider: String(m?.provider || ''),
          name: String(m?.name || '')
        }))
      }))
  },

  agentThreads: {
    list: (projectId, agentId = '') =>
      http.get(agentPath(projectId, '/agent-threads'), { params: agentId ? { agent_id: agentId } : {} }).then((r) => {
        const list = Array.isArray(r.data?.threads) ? r.data.threads : []
        return list.map(toAgentThread)
      }),
    page: (projectId, agentId = '', limit = 50, cursor = '') =>
      http.get(agentPath(projectId, '/agent-threads'), { params: { limit, cursor, ...(agentId ? { agent_id: agentId } : {}) } }).then((r) => ({
        threads: (Array.isArray(r.data?.threads) ? r.data.threads : []).map(toAgentThread),
        next_cursor: String(r.data?.next_cursor || '')
      })),
    create: (projectId, title, agentId = '') =>
      http.post(agentPath(projectId, '/agent-threads'), { title, agent_id: agentId }).then((r) => toAgentThread(r.data)),
    rename: (projectId, threadId, title) =>
      http.patch(agentPath(projectId, '/agent-threads/' + encodeURIComponent(threadId)), { title }).then((r) => toAgentThread(r.data)),
    remove: (projectId, threadId) =>
      http
        .delete(agentPath(projectId, '/agent-threads/' + encodeURIComponent(threadId)))
        .then(() => undefined),
    runs: (projectId, threadId, limit = 20) =>
      http.get(agentPath(projectId, '/agent-threads/' + encodeURIComponent(threadId) + '/runs'), { params: { limit } })
        .then((r) => (Array.isArray(r.data?.runs) ? r.data.runs : []) as AgentRun[]),
    messages: (projectId, threadId) =>
      http
        .get(agentPath(projectId, '/agent-threads/' + encodeURIComponent(threadId) + '/messages'))
        .then((r) => {
          const list = Array.isArray(r.data?.messages) ? r.data.messages : []
          return list.map(toAgentMessage)
        }),
    streamRun: streamAgentRun,
    attachRun: attachAgentRun,
    cancel: (projectId, runId) =>
      http
        .post(agentPath(projectId, '/agent-runs/' + encodeURIComponent(runId) + '/cancel'))
        .then((r) => r.data as AgentRun),
    active: (projectId) =>
      http
        .get(agentPath(projectId, '/agent-runs/active'))
        .then((r) => (Array.isArray(r.data?.runs) ? r.data.runs : []) as ActiveAgentRun[]),
    confirm: (projectId, runId, callId, approve) =>
      http
        .post(agentPath(projectId, '/agent-runs/' + encodeURIComponent(runId) + '/confirmations/' + encodeURIComponent(callId)), { approve })
        .then(() => undefined)
  },

  agentSchedules: {
    list: (projectId) =>
      http.get(agentPath(projectId, '/agent-schedules')).then((r) => {
        const list = Array.isArray(r.data?.schedules) ? r.data.schedules : []
        return list.map((s: Record<string, any>) => toAgentSchedule(s))
      }),
    create: (projectId, body) =>
      http.post(agentPath(projectId, '/agent-schedules'), body).then((r) => toAgentSchedule(r.data)),
    patch: (projectId, scheduleId, body) =>
      http
        .patch(agentPath(projectId, '/agent-schedules/' + encodeURIComponent(scheduleId)), body)
        .then((r) => toAgentSchedule(r.data)),
    remove: (projectId, scheduleId) =>
      http
        .delete(agentPath(projectId, '/agent-schedules/' + encodeURIComponent(scheduleId)))
        .then(() => undefined),
    runs: (projectId, scheduleId) =>
      http
        .get(agentPath(projectId, '/agent-schedules/' + encodeURIComponent(scheduleId) + '/runs'))
        .then((r) => {
          const list = Array.isArray(r.data?.runs) ? r.data.runs : []
          return list.map((x: Record<string, any>) => toAgentScheduleRun(x))
        }),
    trigger: (projectId, scheduleId) =>
      http
        .post(agentPath(projectId, '/agent-schedules/' + encodeURIComponent(scheduleId) + '/run'))
        .then(() => undefined)
  },
  storage: {
    get: () => http.get('/v1/system/storage').then((r) => r.data as StorageSettings),
    put: (body) => http.put('/v1/system/storage', body).then((r) => r.data as StorageUpdateResult)
  }
}
