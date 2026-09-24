import { http, baseURL, getApiKey, setTokens, clearTokens } from './http'
import type {
  Api,
  AgentMessage,
  AgentModuleInfo,
  AgentRun,
  AgentRunRequest,
  AgentSchedule,
  AgentScheduleRun,
  AgentStreamHandlers,
  AgentThread,
  AuthUser,
  CloudAgent,
  CreateUserRequest,
  CronJobCreate,
  CronJobItem,
  CronJobRunItem,
  DatabaseItem,
  GoFunctionItem,
  GoFuncVersionCreate,
  GoFuncVersionSummary,
  LlmChatRequest,
  LlmSettings,
  LlmStreamHandlers,
  LlmStreamConnection,
  LogEvent,
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
  UserListResult
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
    documentCount: typeof d.document_count === 'number' ? d.document_count : undefined
  }
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
        `${baseURL}/v1/projects/${encodeURIComponent(projectId)}/llm/stream`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${getApiKey()}`
          },
          body: JSON.stringify(toLlmPayload(req)),
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
      const reader = resp.body.getReader()
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
          for (const line of raw.split('\n')) {
            if (!line.startsWith('data:')) continue
            const data = line.slice(5).trim()
            if (!data) continue
            try {
              const obj = JSON.parse(data)
              if (obj?.type === 'end') {
                fireEnd()
                continue
              }
              // 兼容多种 chunk 结构：{delta} / {content} / OpenAI 风格 choices
              const text = obj?.delta ?? obj?.content ?? obj?.choices?.[0]?.delta?.content ?? ''
              if (text) handlers.onChunk?.(text)
            } catch {
              /* 忽略非 JSON 帧 */
            }
          }
        }
      }
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

/** 定时任务路径：/v1/projects/:pid/cron-jobs[/:jobId[/runs|/trigger]] */
function cronJobsPath(projectId: string, jobId?: string, suffix?: 'runs' | 'trigger') {
  let path = '/v1/projects/' + encodeURIComponent(projectId) + '/cron-jobs'
  if (jobId) {
    path += '/' + encodeURIComponent(jobId)
    if (suffix) path += '/' + suffix
  }
  return path
}

/** GoFunctionItem：后端 snake_case → UI camelCase（§3.13） */
function toCronJobItem(raw: Record<string, any>): CronJobItem {
  return {
    id: String(raw?.id ?? ''),
    name: String(raw?.name ?? ''),
    description: String(raw?.description ?? ''),
    scheduleKind: raw?.schedule_kind === 'interval' ? 'interval' : raw?.schedule_kind === 'once' ? 'once' : 'cron',
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

function toCronJobRunItem(raw: Record<string, any>): CronJobRunItem {
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
    func_file: body.funcFile,
    func_export: body.funcExport,
    input_json: body.inputJson ?? '{}'
  }
  if (body.name != null) out.name = body.name
  if (body.cronExpr != null) out.cron_expr = body.cronExpr
  if (body.intervalSeconds != null) out.interval_seconds = body.intervalSeconds
  if (body.runAt != null) out.run_at = body.runAt
  if (body.enabled != null) out.enabled = body.enabled
  return out
}

function toMetricsSummary(d: Record<string, any> | undefined): MetricsSummary {
  const raw = d && typeof d === 'object' ? d : {}
  return {
    totalRequests: Number(raw.total_requests ?? raw.totalRequests ?? 0),
    errorRate: Number(raw.error_rate ?? raw.errorRate ?? 0),
    avgLatencyMs: Number(raw.avg_latency_ms ?? raw.avgLatencyMs ?? 0),
    activeDatabases: Number(raw.active_databases ?? raw.activeDatabases ?? 0)
  }
}

function toTrendPoints(d: Record<string, any> | unknown[] | undefined): TrendPoint[] {
  const list = Array.isArray(d) ? d : Array.isArray((d as any)?.points) ? (d as any).points : []
  return list.map((p: Record<string, any>) => ({
    date: String(p?.date ?? ''),
    requests: Number(p?.requests ?? 0),
    errors: Number(p?.errors ?? 0)
  }))
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

function toLogEvent(raw: Record<string, any>): LogEvent {
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

function toLogRetention(raw: Record<string, any> | undefined): LogRetention {
  const d = raw && typeof raw === 'object' ? raw : {}
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
    team_enabled: !!raw?.team_enabled,
    created_at: String(raw?.created_at ?? ''),
    updated_at: String(raw?.updated_at ?? '')
  }
}

function toAgentThread(raw: Record<string, any>): AgentThread {
  return {
    id: String(raw?.id ?? ''),
    title: String(raw?.title ?? ''),
    created_at: String(raw?.created_at ?? ''),
    updated_at: String(raw?.updated_at ?? '')
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

function streamAgentRun(
  projectId: string,
  threadId: string,
  req: AgentRunRequest,
  handlers: AgentStreamHandlers
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
        `${baseURL}${agentPath(projectId, '/agent-threads/' + encodeURIComponent(threadId) + '/runs')}`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${getApiKey()}`
          },
          body: JSON.stringify({ content: req.content, mentions: req.mentions, stream: true }),
          signal: controller.signal
        }
      )
      const ct = resp.headers.get('content-type') || ''
      if (ct.includes('text/html')) {
        throw new Error('接口不存在或返回了 HTML 页面')
      }
      if (!resp.ok || !resp.body) {
        const data = await resp.json().catch(() => null)
        throw new Error(
          (data as any)?.error?.message || (data as any)?.message || `请求失败 (${resp.status})`
        )
      }
      const reader = resp.body.getReader()
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
          for (const line of raw.split('\n')) {
            if (!line.startsWith('data:')) continue
            const data = line.slice(5).trim()
            if (!data) continue
            try {
              const obj = JSON.parse(data)
              if (obj?.type === 'end') {
                fireEnd()
                continue
              }
              if (obj?.type === 'run' && obj.run_id) {
                handlers.onRun?.(String(obj.run_id))
                continue
              }
              if (obj?.type === 'error') {
                handlers.onError?.(new Error(obj.message || 'run failed'))
                continue
              }
              if (obj?.type === 'token' || obj?.type === 'chunk') {
                const text = obj.content || obj.delta || ''
                if (text) handlers.onToken?.(text)
                continue
              }
              if (obj?.type === 'tool_call') {
                handlers.onToolCall?.(obj.name || '', obj.arguments || '')
                continue
              }
              if (obj?.type === 'tool_result') {
                handlers.onToolResult?.(obj.name || '', obj.content || '')
              }
            } catch {
              /* ignore */
            }
          }
        }
      }
      fireEnd()
    } catch (e) {
      if (!controller.signal.aborted) handlers.onError?.(e)
    }
  })()
  return { close: () => controller.abort() }
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

function toProjectItem(raw: any): import('./types').ProjectItem {
  return {
    id: String(raw?.id ?? ''),
    name: String(raw?.name ?? ''),
    createdAt: String(raw?.created_at ?? raw?.createdAt ?? '')
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
  auth: {
    login: (req) =>
      http
        .post('/v1/auth/login', { username: req.username, password: req.password })
        .then((r) => {
          const pair = toTokenPair(r.data)
          if (pair.accessToken) setTokens(pair.accessToken, pair.refreshToken)
          return pair
        }),
    refresh: (refreshToken) =>
      http.post('/v1/auth/refresh', { refresh_token: refreshToken }).then((r) => {
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
    get: (id) => http.get('/v1/users/' + encodeURIComponent(id)).then((r) => toUserItem(r.data)),
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
        .then((r) => toUserItem(r.data)),
    remove: (id) => http.delete('/v1/users/' + encodeURIComponent(id)).then(() => undefined)
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
    /** 兼容旧签名：保存为新版本并生效 */
    update: (projectId: string, name: string, source: string) =>
      http
        .post(gofunctionsPath(projectId, name, undefined, 'versions'), { source, activate: true })
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
  cronjobs: {
    list: (projectId) =>
      http.get(cronJobsPath(projectId)).then((r) => {
        const list = Array.isArray(r.data?.jobs) ? r.data.jobs : []
        return list.map((raw: Record<string, any>) => toCronJobItem(raw))
      }),
    create: (projectId, body) =>
      http.post(cronJobsPath(projectId), cronJobBody(body)).then((r) => toCronJobItem(r.data)),
    get: (projectId, jobId) =>
      http.get(cronJobsPath(projectId, jobId)).then((r) => toCronJobItem(r.data)),
    update: (projectId, jobId, body) =>
      http.patch(cronJobsPath(projectId, jobId), cronJobBody(body)).then((r) => toCronJobItem(r.data)),
    remove: (projectId, jobId) =>
      http.delete(cronJobsPath(projectId, jobId)).then(() => undefined),
    runs: (projectId, jobId, limit) =>
      http
        .get(cronJobsPath(projectId, jobId, 'runs'), { params: limit ? { limit } : undefined })
        .then((r) => {
          const list = Array.isArray(r.data?.runs) ? r.data.runs : []
          return list.map((raw: Record<string, any>) => toCronJobRunItem(raw))
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
          owner_user_id: req.ownerUserId || undefined
        })
        .then((r) => toProjectItem(r.data))
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
    create: (projectId, name) =>
      http
        .post('/v1/projects/' + encodeURIComponent(projectId) + '/databases', { name })
        .then((r) => toDatabaseItem(r.data)),
    get: (projectId, databaseId) =>
      http
        .get(
          '/v1/projects/' +
            encodeURIComponent(projectId) +
            '/databases/' +
            encodeURIComponent(databaseId)
        )
        .then((r) => toDatabaseItem(r.data)),
    remove: (projectId, databaseId) =>
      http
        .delete(
          '/v1/projects/' +
            encodeURIComponent(projectId) +
            '/databases/' +
            encodeURIComponent(databaseId)
        )
        .then(() => undefined)
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
        .then(() => undefined)
  },

  s3: {
    list: (projectId, prefix) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/s3/objects', {
          params: { prefix }
        })
        .then((r) => (Array.isArray(r.data) ? r.data : [])),
    presign: (projectId, key) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/s3/presign', { params: { key } })
        .then((r) => r.data),
    remove: (projectId, key) =>
      http
        .delete('/v1/projects/' + encodeURIComponent(projectId) + '/s3/objects', { params: { key } })
        .then(() => undefined),
    upload: (projectId, key, file) => {
      const fd = new FormData()
      fd.append('key', key)
      fd.append('file', file)
      return http
        .post('/v1/projects/' + encodeURIComponent(projectId) + '/s3/objects', fd)
        .then((r) => r.data)
    }
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
    providers: (projectId) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/llm/providers')
        .then((r) => {
          const d = r.data
          return Array.isArray(d) ? d : Array.isArray(d?.providers) ? d.providers : []
        }),
    chat: (projectId, req) =>
      http
        .post('/v1/projects/' + encodeURIComponent(projectId) + '/llm/chat', toLlmPayload(req))
        .then((r) => r.data),
    stream: llmStream
  },

  llmSettings: {
    get: (projectId) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/llm/settings')
        .then((r) => toLlmSettings(r.data)),
    put: (projectId, settings) =>
      http
        .put('/v1/projects/' + encodeURIComponent(projectId) + '/llm/settings', {
          default_provider: settings.defaultProvider || '',
          default_model: settings.defaultModel || '',
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
    get: (projectId, agentId) =>
      http
        .get(agentPath(projectId, '/agents/' + encodeURIComponent(agentId)))
        .then((r) => toCloudAgent(r.data)),
    patch: (projectId, agentId, body) =>
      http
        .patch(agentPath(projectId, '/agents/' + encodeURIComponent(agentId)), body)
        .then((r) => toCloudAgent(r.data)),
    remove: (projectId, agentId) =>
      http.delete(agentPath(projectId, '/agents/' + encodeURIComponent(agentId))).then(() => undefined)
  },

  agentThreads: {
    list: (projectId) =>
      http.get(agentPath(projectId, '/agent-threads')).then((r) => {
        const list = Array.isArray(r.data?.threads) ? r.data.threads : []
        return list.map(toAgentThread)
      }),
    create: (projectId, title) =>
      http.post(agentPath(projectId, '/agent-threads'), { title }).then((r) => toAgentThread(r.data)),
    get: (projectId, threadId) =>
      http
        .get(agentPath(projectId, '/agent-threads/' + encodeURIComponent(threadId)))
        .then((r) => toAgentThread(r.data)),
    remove: (projectId, threadId) =>
      http
        .delete(agentPath(projectId, '/agent-threads/' + encodeURIComponent(threadId)))
        .then(() => undefined),
    messages: (projectId, threadId) =>
      http
        .get(agentPath(projectId, '/agent-threads/' + encodeURIComponent(threadId) + '/messages'))
        .then((r) => {
          const list = Array.isArray(r.data?.messages) ? r.data.messages : []
          return list.map(toAgentMessage)
        }),
    run: (projectId, threadId, req) =>
      http
        .post(agentPath(projectId, '/agent-threads/' + encodeURIComponent(threadId) + '/runs'), {
          content: req.content,
          mentions: req.mentions,
          stream: false
        })
        .then((r) => ({
          run: r.data?.run as AgentRun,
          message: toAgentMessage(r.data?.message || {})
        })),
    streamRun: streamAgentRun,
    cancel: (projectId, runId) =>
      http
        .post(agentPath(projectId, '/agent-runs/' + encodeURIComponent(runId) + '/cancel'))
        .then((r) => r.data as AgentRun)
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
  }
}
