// 文件说明：http-api/platform.ts — `httpApi` 的平台与资源（认证、用户、云函数、沙盒、定时任务、项目、API Key、指标、备份、日志、配额、LLM 设置与 trace、存储设置）部分；只在这里出现对应的 URL 与 wire → UI 转换。
// 与 http-api.ts、其余域文件共同组成 `Api` 的唯一实现，不要在页面里直接 import 本文件。
import { http, baseURL, setTokens, clearTokens } from '@/services/http'
import type {
  APIPreviewResult,
  OpenAPIDocument,
  ApiKeyItem,
  CreateUserRequest,
  CronJobRunWire,
  CronJobWire,
  SandboxCapabilities,
  SandboxItem,
  StorageSettings,
  StorageUpdateResult,
  UpdateUserRequest,
  UserListResult
} from '@/services/types'
import {
  cronJobBody,
  E2B_BASE,
  ENVD_BASE,
  e2bHeaders,
  envdRpc,
  llmTraceGroupParams,
  logQueryParams,
  num,
  SB_META_NAME,
  sandboxPath,
  toApiKeyItem,
  toAuthUser,
  toBackupItem,
  toCronJobItem,
  toCronJobRunItem,
  toFunctionTrigger,
  toGoFunctionItem,
  toGoFuncVersion,
  toLlmSettings,
  toLLMTrace,
  toLLMTraceGroupListResult,
  LLMTraceWire,
  toLogEvent,
  toLogRetention,
  toMetricsSummary,
  toProjectItem,
  toSandboxFileEntry,
  toSandboxItem,
  toTokenPair,
  toTrendPoints,
  toTriggerWire,
  toUserItem
} from '@/services/converters/platform'
import { collectionWire } from '@/services/converters/data'
import { cronJobsPath, gofunctionsPath, projectPath } from '@/services/paths'
import { listOf, type WireRecord } from '@/services/wire'
import type { Api } from '@/services/types'


export const platformApi = {
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
          ? d.projects.map((p: WireRecord) => ({
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
        .then(() => undefined),
    initialPassword: (req) =>
      http
        .post('/v1/auth/password/initial', {
          username: req.username,
          old_password: req.oldPassword,
          new_password: req.newPassword
        })
        .then((r) => {
          // 端点直接返回正式 TokenPair，与 login 同样落盘。
          const pair = toTokenPair(r.data)
          if (pair.accessToken) setTokens(pair.accessToken, pair.refreshToken)
          return pair
        })
  },

  users: {
    list: (limit, cursor) =>
      http
        .get('/v1/users', { params: { limit: limit || 20, cursor: cursor || undefined } })
        .then((r): UserListResult => {
          const list = listOf(r.data?.users)
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
    list: (projectId) => http.get(projectPath(projectId, 'function-triggers'))
      .then((r) => listOf(r.data?.items, toFunctionTrigger)),
    create: (projectId, input) => http.post(projectPath(projectId, 'function-triggers'), toTriggerWire(input))
      .then((r) => toFunctionTrigger(r.data)),
    patch: (projectId, id, input) => http.patch(projectPath(projectId, 'function-triggers', id), toTriggerWire(input))
      .then((r) => toFunctionTrigger(r.data)),
    remove: (projectId, id) => http.delete(projectPath(projectId, 'function-triggers', id)).then(() => undefined),
    invoke: (projectId, id, body) => http.post(projectPath(projectId, 'function-triggers', id, 'invoke'), body).then(() => undefined)
  },

  gofunctions: {
    list: (projectId) =>
      http.get(gofunctionsPath(projectId)).then((r) => listOf(r.data?.functions, toGoFunctionItem)),
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
        versions: listOf(r.data?.versions, toGoFuncVersion)
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
        images: listOf(raw.images, String),
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
        const rows: Record<string, unknown>[] = listOf(r.data)
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
        .then((r) => listOf(r.data?.entries, (raw: Record<string, unknown>) => toSandboxFileEntry(raw))),
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
        const list = listOf(r.data?.jobs)
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
          const list = listOf(r.data?.runs)
          return list.map((raw: CronJobRunWire) => toCronJobRunItem(raw))
        }),
    trigger: (projectId, jobId) =>
      http.post(cronJobsPath(projectId, jobId, 'trigger')).then(() => undefined)
  },

  projects: {
    list: () =>
      http.get('/v1/projects').then((r) => listOf(r.data?.projects, toProjectItem)),
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
        .get(projectPath(projectId, 'api-keys'))
        .then((r) => listOf(r.data?.keys, toApiKeyItem)),
    create: (projectId, permissions, scope) =>
      http
        .post(projectPath(projectId, 'api-keys'), {
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
        .delete(projectPath(projectId, 'api-keys', keyId))
        .then(() => undefined)
  },

  metrics: {
    summary: (projectId) =>
      http
        .get(projectPath(projectId, 'metrics', 'summary'))
        .then((r) => toMetricsSummary(r.data)),
    trend: (projectId) =>
      http
        .get(projectPath(projectId, 'metrics', 'trend'))
        .then((r) => toTrendPoints(r.data))
  },

  backups: {
    list: (projectId) =>
      http
        .get(projectPath(projectId, 'backups'))
        .then((r) => listOf(r.data?.items, toBackupItem)),
    create: (projectId) =>
      http.post(projectPath(projectId, 'backups'), {}).then((r) => toBackupItem(r.data)),
    remove: (projectId, id) =>
      http.delete(projectPath(projectId, 'backups', id)).then(() => undefined),
    restore: (projectId, id) =>
      http.post(projectPath(projectId, 'backups', id, 'restore'), {}).then(() => undefined),
    download: (projectId, id) =>
      http
        .get(projectPath(projectId, 'backups', id, 'download'), { responseType: 'blob' })
        .then((r) => r.data as Blob)
  },

  logs: {
    list: (projectId, q) =>
      http
        .get(projectPath(projectId, 'logs'), {
          params: logQueryParams(q)
        })
        .then((r) => {
          return listOf(Array.isArray(r.data?.events) ? r.data.events : r.data, toLogEvent)
        }),
    stats: (projectId, q, groupBy = 'hour') =>
      http
        .get(projectPath(projectId, 'logs', 'stats'), { params: { ...logQueryParams(q), group_by: groupBy } })
        .then((r) => Array.isArray(r.data?.items) ? r.data.items.map((item: unknown) => {
          const stat = collectionWire(item)
          return { key: String(stat.key ?? ''), count: Number(stat.count ?? 0) }
        }) : []),
    export: (projectId, q) =>
      http
        .get(projectPath(projectId, 'logs', 'export'), { params: logQueryParams(q), responseType: 'blob' })
        .then((r) => r.data as Blob),
    getRetention: (projectId) =>
      http
        .get(projectPath(projectId, 'logs', 'retention'))
        .then((r) => toLogRetention(r.data)),
    putRetention: (projectId, keepDays) =>
      http
        .put(projectPath(projectId, 'logs', 'retention'), { keep_days: keepDays })
        .then(() => undefined)
  },

  quota: {
    status: (projectId) =>
      http
        .get(projectPath(projectId, 'quota'))
        .then((r) => ({
          llmAllowed: !!r.data?.llm_allowed,
          databaseAllowed: !!r.data?.database_allowed
        }))
  },

  llmSettings: {
    get: (projectId) =>
      http
        .get(projectPath(projectId, 'llm', 'settings'))
        .then((r) => toLlmSettings(r.data)),
    put: (projectId, settings, apiKey) =>
      http
        .put(projectPath(projectId, 'llm', 'settings'), {
          default_provider: settings.defaultProvider || '',
          default_model: settings.defaultModel || '',
          api_key: apiKey || '',
          base_url: settings.baseUrl || '',
          temperature: settings.temperature,
          max_tokens: settings.maxTokens
        })
        .then((r) => toLlmSettings(r.data))
  },

  llmTraces: {
    groups: (projectId, q) =>
      http
        .get(projectPath(projectId, 'llm', 'trace-groups'), {
          params: llmTraceGroupParams(q)
        })
        .then((r) => toLLMTraceGroupListResult(r.data)),
    detail: (projectId, traceId) =>
      http
        .get(projectPath(projectId, 'llm', 'trace-groups', traceId))
        .then((r) => {
          const list: LLMTraceWire[] = listOf(r.data?.spans)
          return list.map((row) => toLLMTrace(row))
        })
  },

  storage: {
    get: () => http.get('/v1/system/storage').then((r) => r.data as StorageSettings),
    put: (body) => http.put('/v1/system/storage', body).then((r) => r.data as StorageUpdateResult)
  },
} satisfies Partial<Api>
