import { http, baseURL, getApiKey } from './http'
import type {
  Api,
  AgentMessage,
  AgentModuleInfo,
  AgentRun,
  AgentRunRequest,
  AgentStreamHandlers,
  AgentThread,
  CloudAgent,
  DatabaseItem,
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
  TrendPoint
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
      : undefined
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
    fieldsJson: raw?.fields_json ?? raw?.fieldsJson ?? raw?.FieldsJSON || undefined,
    requestId: raw?.request_id ?? raw?.requestId ?? raw?.RequestID || undefined,
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

/** 真实后端实现（路径契约见 plan/planv2.0/proto-http.md） */
export const httpApi: Api = {
  projects: {
    list: () =>
      http.get('/v1/projects').then((r) => {
        const list = Array.isArray(r.data?.projects) ? r.data.projects : []
        return list.map(toProjectItem)
      }),
    create: (req) =>
      http.post('/v1/projects', { name: req.name, id: req.id || undefined }).then((r) =>
        toProjectItem(r.data)
      )
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
    open: (projectId, databaseId) =>
      http
        .post(
          '/v1/projects/' +
            encodeURIComponent(projectId) +
            '/databases/' +
            encodeURIComponent(databaseId) +
            '/open'
        )
        .then((r) => toDatabaseItem(r.data)),
    close: (projectId, databaseId) =>
      http
        .post(
          '/v1/projects/' +
            encodeURIComponent(projectId) +
            '/databases/' +
            encodeURIComponent(databaseId) +
            '/close'
        )
        .then(() => undefined),
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
  }
}
