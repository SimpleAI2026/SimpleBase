import { http, baseURL, setTokens, clearTokens } from './http'
import type {
  Api,
  APIPreviewResult,
  OpenAPIDocument,
  AgentModuleInfo,
  AgentRun,
  ActiveAgentRun,
  ApiKeyItem,
  CreateUserRequest,
  CronJobRunWire,
  CronJobWire,
  SandboxCapabilities,
  SandboxItem,
  StorageSettings,
  StorageUpdateResult,
  CreateDatabaseInput,
  UpdateUserRequest,
  UserListResult,
  ViewDryRunResult,
  CollectionImportResult
} from './types'

/* ---------- 命名转换：UI camelCase ↔ 后端 snake_case（集中在此） ---------- */

/* 转换函数与 wire 接口已拆到 http-api-converters.ts（planv6.6 OPT-6）。 */
import {
  agentScheduleFromCronJob,
  agentPath,
  cronJobBody,
  cronJobsPath,
  dataCollectionsPath,
  databaseResourcePath,
  E2B_BASE,
  ENVD_BASE,
  e2bHeaders,
  envdRpc,
  gofunctionsPath,
  kvPath,
  llmTraceGroupParams,
  logQueryParams,
  num,
  SB_META_NAME,
  sandboxPath,
  attachAgentRun,
  collectionWire,
  streamAgentRun,
  toApiKeyItem,
  toAgentApproval,
  toAgentHumanTask,
  toAgentTag,
  toAgentTurn,
  streamAgentTurn,
  toWorkspaceCapabilities,
  fromAgentTagInput,
  toAgentMessage,
  toAgentThread,
  toAuthUser,
  toBackupItem,
  toCloudAgent,
  toCollectionInfo,
  toCollectionSchema,
  toCronJobItem,
  toCronJobRunItem,
  toDatabaseItem,
  toDatabaseSchema,
  toDocumentItem,
  toDocumentPage,
  toAggregateParams,
  toAggregateResult,
  toExecuteResult,
  toFunctionTrigger,
  toGoFunctionItem,
  toGoFuncVersion,
  toLlmSettings,
  toLLMTrace,
  toLLMTraceGroupListResult,
  LLMTraceWire,
  toLogEvent,
  toLogRetention,
  toMcpBuiltinServer,
  toMcpServer,
  toMetricsSummary,
  toProjectItem,
  toQueryResult,
  toS3FolderMeta,
  toS3PublicResult,
  toS3SiteInfo,
  toSandboxFileEntry,
  toSandboxItem,
  toSchemaColumn,
  toSchemaDryRun,
  toSchemaTable,
  toSchemaTableRows,
  toTokenPair,
  toTrendPoints,
  toTriggerWire,
  toUserItem,
  toBatchResult,
  toSqlPayload
} from './http-api-converters'

// 助手定时摘要的映射函数对外可用（AgentManager 直接消费；planv6.6 DC-5）。
export { agentScheduleFromCronJob }

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
        .then(() => undefined),
    initialPassword: (req) =>
      http
        .post('/v1/auth/password/initial', {
          username: req.username,
          old_password: req.oldPassword,
          new_password: req.newPassword
        })
        .then((r) => {
          // 端点直接返回正式 TokenPair（§8.2.2），与 login 同样落盘。
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
          // filter 为空时不带该键，请求与改造前逐字一致。
          params: { limit: query.limit, offset: query.offset, ...(query.filter ? { filter: query.filter } : {}) }
        })
        .then((r) => toSchemaTableRows(r.data)),
    aggregateTable: (projectId, databaseId, table, query) =>
      http
        .get(databaseResourcePath(projectId, databaseId, '/schema/tables/' + encodeURIComponent(table) + '/aggregate'), {
          params: toAggregateParams(query)
        })
        .then((r) => toAggregateResult(r.data)),
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
    aggregate: (projectId, databaseId, collection, query) =>
      http
        .get(dataCollectionsPath(projectId, databaseId, collection) + '/aggregate', {
          params: toAggregateParams(query)
        })
        .then((r) => toAggregateResult(r.data)),
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
        .then((r) => ({
          folders: Array.isArray(r.data?.folders) ? r.data.folders : [],
          objects: Array.isArray(r.data?.objects) ? r.data.objects : [],
          folderMeta: Array.isArray(r.data?.folderMeta) ? r.data.folderMeta.map(toS3FolderMeta) : [],
          ...(r.data?.site ? { site: toS3SiteInfo(r.data.site) } : {})
        })),
    getSite: (projectId) =>
      http.get('/v1/projects/' + encodeURIComponent(projectId) + '/s3/site').then((r) => toS3SiteInfo(r.data)),
    setPublic: (projectId, key, kind, isPublic) =>
      http
        .put('/v1/projects/' + encodeURIComponent(projectId) + '/s3/public', { key, kind, public: isPublic })
        .then((r) => toS3PublicResult(r.data)),
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

  /** planv6.6 DC-5：llm.chat/llm.stream 已删除（零生产引用）。 */
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

  llmTraces: {
    /** llmTraces.list 已随 planv6.6 DC-5 删除（零生产引用）。 */
    groups: (projectId, q) =>
      http
        .get('/v1/projects/' + encodeURIComponent(projectId) + '/llm/trace-groups', {
          params: llmTraceGroupParams(q)
        })
        .then((r) => toLLMTraceGroupListResult(r.data)),
    detail: (projectId, traceId) =>
      http
        .get(
          '/v1/projects/' +
            encodeURIComponent(projectId) +
            '/llm/trace-groups/' +
            encodeURIComponent(traceId)
        )
        .then((r) => {
          const list: LLMTraceWire[] = Array.isArray(r.data?.spans) ? r.data.spans : []
          return list.map((row) => toLLMTrace(row))
        })
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
    /** 按标签分页。 */
    pageBy: (projectId, q = {}) => {
      const params: Record<string, unknown> = {}
      if (q.limit !== undefined) params.limit = q.limit
      if (q.cursor) params.cursor = q.cursor
      if (q.agentId) params.agent_id = q.agentId
      if (q.tagId) params.tag_id = q.tagId
      return http.get(agentPath(projectId, '/agent-threads'), { params }).then((r) => ({
        threads: (Array.isArray(r.data?.threads) ? r.data.threads : []).map(toAgentThread),
        next_cursor: String(r.data?.next_cursor || '')
      }))
    },
    create: (projectId, titleOrOpts, agentId = '') => {
      // create 第二参数兼容 string（旧：title）与 AgentThreadCreateOptions。
      const body: Record<string, unknown> =
        typeof titleOrOpts === 'string'
          ? { title: titleOrOpts, agent_id: agentId }
          : {
              ...(titleOrOpts?.title !== undefined ? { title: titleOrOpts.title } : {}),
              ...(titleOrOpts?.agentId ? { agent_id: titleOrOpts.agentId } : {}),
              ...(titleOrOpts?.tagId ? { tag_id: titleOrOpts.tagId } : {}),
              ...(titleOrOpts?.dispatchMode ? { dispatch_mode: titleOrOpts.dispatchMode } : {}),
              ...(titleOrOpts?.threadKind ? { thread_kind: titleOrOpts.threadKind } : {}),
              ...(titleOrOpts?.source ? { source: titleOrOpts.source } : {})
            }
      return http.post(agentPath(projectId, '/agent-threads'), body).then((r) => toAgentThread(r.data))
    },
    /** 单个会话详情（深链补齐）。 */
    get: (projectId, threadId) =>
      http.get(agentPath(projectId, '/agent-threads/' + encodeURIComponent(threadId))).then((r) => toAgentThread(r.data)),
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
    /** v2：协作轮次（多目标 @）。 */
    streamTurn: streamAgentTurn,
    attachRun: attachAgentRun,
    /** v2：会话的轮次列表（新→旧）。 */
    turns: (projectId, threadId) =>
      http.get(agentPath(projectId, '/agent-threads/' + encodeURIComponent(threadId) + '/turns'))
        .then((r) => (Array.isArray(r.data?.turns) ? r.data.turns : []).map(toAgentTurn)),
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

  /** 工作台标签 CRUD（planv6.7 AGC-NAV）。 */
  agentTags: {
    list: (projectId) =>
      http.get(agentPath(projectId, '/agent-tags')).then((r) => {
        const list = Array.isArray(r.data?.tags) ? r.data.tags : []
        return list.map(toAgentTag)
      }),
    create: (projectId, input) =>
      http.post(agentPath(projectId, '/agent-tags'), fromAgentTagInput(input)).then((r) => toAgentTag(r.data)),
    patch: (projectId, tagId, input) =>
      http
        .patch(agentPath(projectId, '/agent-tags/' + encodeURIComponent(tagId)), fromAgentTagInput(input))
        .then((r) => toAgentTag(r.data)),
    remove: (projectId, tagId) =>
      http.delete(agentPath(projectId, '/agent-tags/' + encodeURIComponent(tagId))).then(() => undefined),
    /** v2：加入已有成员，或「新建助手并加入」（二者互斥，newAgent 优先）。 */
    addMembers: (projectId, tagId, body) =>
      http.post(agentPath(projectId, '/agent-tags/' + encodeURIComponent(tagId) + '/members'), body)
        .then((r) => toAgentTag(r.data)),
    /** v2：移出成员（不删除助手）。 */
    removeMember: (projectId, tagId, agentId, expectedVersion) =>
      http.delete(agentPath(projectId, '/agent-tags/' + encodeURIComponent(tagId) + '/members/' + encodeURIComponent(agentId)), {
        params: expectedVersion !== undefined ? { expected_version: expectedVersion } : {}
      }).then((r) => toAgentTag(r.data)),
    /** v2：告警/统计输入入口（按 external_id 幂等）。 */
    submitEvent: (projectId, tagId, body) =>
      http.post(agentPath(projectId, '/agent-tags/' + encodeURIComponent(tagId) + '/events'), body)
        .then((r) => toAgentTurn(r.data))
  },

  /** v2：协作轮次与人工待办。 */
  agentTurns: {
    get: (projectId, turnId) =>
      http.get(agentPath(projectId, '/agent-turns/' + encodeURIComponent(turnId))).then((r) => toAgentTurn(r.data)),
    /** 取消整轮 / 重试单个任务 / 恢复。 */
    act: (projectId, turnId, body) =>
      http.post(agentPath(projectId, '/agent-turns/' + encodeURIComponent(turnId) + '/actions'), body)
        .then((r) => toAgentTurn(r.data)),
    /** 人工回复（continue / unable）；与工具审批严格分离。 */
    reply: (projectId, taskId, body) =>
      http.post(agentPath(projectId, '/agent-tasks/' + encodeURIComponent(taskId) + '/replies'), body)
        .then((r) => toAgentTurn(r.data)),
    /** 待办列表与角标；assignee='me' 表示当前登录用户。 */
    humanTasks: (projectId, assignee = 'me', status = 'pending') =>
      http.get(agentPath(projectId, '/agent-human-tasks'), { params: { assignee, status } })
        .then((r) => ({
          tasks: (Array.isArray(r.data?.tasks) ? r.data.tasks : []).map(toAgentHumanTask),
          pending: Number(r.data?.pending || 0)
        })),
    /** 能力探测：collaboration=false 时不露出协作控件。 */
    capabilities: (projectId) =>
      http.get(agentPath(projectId, '/agent-workspace-capabilities')).then((r) => toWorkspaceCapabilities(r.data))
  },

  agentApprovals: {
    list: (projectId, status = '', limit = 50) =>
      http
        .get(agentPath(projectId, '/agent-approvals'), {
          params: { ...(status ? { status } : {}), limit }
        })
        .then((r) => ({
          approvals: (Array.isArray(r.data?.approvals) ? r.data.approvals : []).map(toAgentApproval),
          pending: Number(r.data?.pending || 0)
        })),
    resolve: (projectId, runId, callId, approve) =>
      http
        .post(
          agentPath(
            projectId,
            '/agent-runs/' + encodeURIComponent(runId) + '/confirmations/' + encodeURIComponent(callId)
          ),
          { approve }
        )
        .then(() => undefined)
  },

  mcpServers: {
    list: (projectId) =>
      http.get(agentPath(projectId, '/mcp/servers')).then((r) => {
        const servers = Array.isArray(r.data?.servers) ? r.data.servers : []
        const builtin = Array.isArray(r.data?.builtin) ? r.data.builtin : []
        return { servers: servers.map(toMcpServer), builtin: builtin.map(toMcpBuiltinServer) }
      }),
    create: (projectId, body) =>
      http.post(agentPath(projectId, '/mcp/servers'), body).then((r) => toMcpServer(r.data)),
    update: (projectId, serverId, body) =>
      http
        .patch(agentPath(projectId, '/mcp/servers/' + encodeURIComponent(serverId)), body)
        .then((r) => toMcpServer(r.data)),
    remove: (projectId, serverId) =>
      http.delete(agentPath(projectId, '/mcp/servers/' + encodeURIComponent(serverId))).then(() => undefined),
    test: (projectId, serverId) =>
      http
        .post(agentPath(projectId, '/mcp/servers/' + encodeURIComponent(serverId) + '/test'))
        .then((r) => ({
          ok: Boolean(r.data?.ok),
          tools: (Array.isArray(r.data?.tools) ? r.data.tools : []).map(String),
          error: r.data?.error ? String(r.data.error) : undefined
        })),
    setBuiltinEnabled: (projectId, name, enabled) =>
      http
        .put(agentPath(projectId, '/mcp/builtin/' + encodeURIComponent(name) + '/state'), { enabled })
        .then(() => undefined),
    testBuiltin: (projectId, name) =>
      http
        .post(agentPath(projectId, '/mcp/builtin/' + encodeURIComponent(name) + '/test'))
        .then((r) => ({
          ok: Boolean(r.data?.ok),
          tools: (Array.isArray(r.data?.tools) ? r.data.tools : []).map(String),
          error: r.data?.error ? String(r.data.error) : undefined
        }))
  },

  /** planv6.6 DC-5：agentSchedules 整组 API 已删除（旧 /agent-schedules 路由下线）。
   *  助手卡片的定时摘要改由 AgentManager 从 cronjobs.list（targetKind=agent）映射。 */
  storage: {
    get: () => http.get('/v1/system/storage').then((r) => r.data as StorageSettings),
    put: (body) => http.put('/v1/system/storage', body).then((r) => r.data as StorageUpdateResult)
  }
}
