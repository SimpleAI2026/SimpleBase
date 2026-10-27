import { vi } from 'vitest'

export const api = {
  preview: {
    document: vi.fn(),
    request: vi.fn()
  },
  auth: {
    login: vi.fn(),
    logout: vi.fn(),
    me: vi.fn(),
    changePassword: vi.fn()
  },
  users: {
    list: vi.fn(),
    create: vi.fn(),
    get: vi.fn(),
    update: vi.fn(),
    remove: vi.fn()
  },
  projects: {
    list: vi.fn(),
    create: vi.fn()
  },
  metrics: {
    summary: vi.fn(),
    trend: vi.fn()
  },
  databases: {
    list: vi.fn(),
    create: vi.fn(),
    get: vi.fn(),
    remove: vi.fn(),
    schema: vi.fn(),
    tableRows: vi.fn(),
    createTable: vi.fn(),
    addColumn: vi.fn()
  },
  sql: {
    query: vi.fn(),
    execute: vi.fn(),
    batch: vi.fn()
  },
  db: {
    collections: vi.fn(),
    createCollection: vi.fn(),
    rows: vi.fn(),
    insert: vi.fn(),
    update: vi.fn(),
    remove: vi.fn(),
    collectionItems: vi.fn(),
    exportCollection: vi.fn(),
    importCollection: vi.fn(),
    dryRunView: vi.fn(),
    documents: vi.fn(),
    document: vi.fn(),
    patch: vi.fn(),
    batch: vi.fn(),
    truncate: vi.fn(),
    getCollectionSchema: vi.fn(),
    putCollectionSchema: vi.fn(),
    deleteCollectionSchema: vi.fn()
  },
  s3: {
    list: vi.fn(),
    listDetailed: vi.fn(),
    presign: vi.fn(),
    download: vi.fn(),
    createLink: vi.fn(),
    remove: vi.fn(),
    upload: vi.fn()
  },
  backups: {
    list: vi.fn(),
    create: vi.fn(),
    remove: vi.fn(),
    restore: vi.fn(),
    download: vi.fn()
  },
  logs: {
    list: vi.fn(),
    stats: vi.fn(),
    export: vi.fn(),
    getRetention: vi.fn(),
    putRetention: vi.fn()
  },
  quota: {
    status: vi.fn()
  },
  llm: {
    chat: vi.fn(),
    stream: vi.fn()
  },
  llmSettings: {
    get: vi.fn(),
    put: vi.fn()
  },
  agents: {
    modules: vi.fn(),
    models: vi.fn(),
    list: vi.fn(),
    create: vi.fn(),
    get: vi.fn(),
    patch: vi.fn(),
    remove: vi.fn()
  },
  agentThreads: {
    list: vi.fn(),
    page: vi.fn(),
    create: vi.fn(),
    get: vi.fn(),
    rename: vi.fn(),
    remove: vi.fn(),
    runs: vi.fn(),
    messages: vi.fn(),
    streamRun: vi.fn(),
    attachRun: vi.fn(),
    cancel: vi.fn(),
    active: vi.fn(),
    confirm: vi.fn()
  },
  agentSchedules: {
    list: vi.fn(),
    create: vi.fn(),
    patch: vi.fn(),
    remove: vi.fn(),
    runs: vi.fn(),
    trigger: vi.fn()
  },
  storage: {
    get: vi.fn(),
    put: vi.fn()
  },
  triggers: {
    list: vi.fn(),
    create: vi.fn(),
    patch: vi.fn(),
    remove: vi.fn(),
    invoke: vi.fn()
  },
  gofunctions: {
    list: vi.fn(),
    create: vi.fn(),
    get: vi.fn(),
    saveVersion: vi.fn(),
    update: vi.fn(),
    remove: vi.fn(),
    listVersions: vi.fn(),
    activate: vi.fn(),
    test: vi.fn()
  },
  sandboxes: {
    capabilities: vi.fn(), list: vi.fn(), create: vi.fn(), get: vi.fn(),
    remove: vi.fn(), connect: vi.fn(), pause: vi.fn(),
    terminalTicket: vi.fn(), terminalWebSocketUrl: vi.fn(),
    files: { list: vi.fn(), read: vi.fn(), write: vi.fn(), remove: vi.fn() }
  },
  cronjobs: {
    list: vi.fn(),
    create: vi.fn(),
    get: vi.fn(),
    update: vi.fn(),
    remove: vi.fn(),
    runs: vi.fn(),
    trigger: vi.fn()
  },
  apiKeys: {
    list: vi.fn(),
    create: vi.fn(),
    revoke: vi.fn()
  },
  kv: {
    /** 项目级单端点：测试里默认模拟「库为空」的语义回复 */
    exec: vi.fn(),
    execBatch: vi.fn()
  }
}

export function applyApiDefaults() {
  api.auth.login.mockResolvedValue({
    tokenType: 'Bearer',
    accessToken: 'at',
    expiresIn: 7200,
    refreshToken: 'rt',
    user: {
      id: 'u1',
      username: 'simplebase2026',
      role: 'superadminl1',
      displayName: 'Super',
      email: '',
      status: 'active',
      mustChangePassword: false
    }
  })
  api.auth.logout.mockResolvedValue(undefined)
  api.auth.me.mockResolvedValue({
    id: 'u1',
    username: 'simplebase2026',
    role: 'superadminl1',
    displayName: 'Super',
    email: '',
    status: 'active',
    mustChangePassword: false,
    projects: []
  })
  api.auth.changePassword.mockResolvedValue(undefined)
  api.users.list.mockResolvedValue({ users: [], nextCursor: '' })
  api.users.create.mockResolvedValue({
    id: 'u-new',
    username: 'alice',
    role: 'user',
    displayName: '',
    email: '',
    status: 'active',
    mustChangePassword: false,
    projectCount: 0
  })
  api.users.update.mockResolvedValue({
    id: 'u1',
    username: 'simplebase2026',
    role: 'superadminl1',
    displayName: '',
    email: '',
    status: 'active',
    mustChangePassword: false,
    projectCount: 0
  })
  api.projects.list.mockResolvedValue([])
  api.projects.create.mockResolvedValue({ id: 'p-new', name: 'New', createdAt: 't' })
  api.metrics.summary.mockResolvedValue({
    totalRequests: 10,
    errorRate: 0.015,
    avgLatencyMs: 12.6,
    activeDatabases: 1,
    latencyP50Ms: 8.4,
    latencyP90Ms: 45,
    latencyP99Ms: 320,
    latencySampleCount: 10,
    latencyOverflowMs: 30000
  })
  api.metrics.trend.mockResolvedValue([
    { date: '01-01', requests: 10, errors: 1 },
    { date: '01-02', requests: 0, errors: 0 }
  ])
  api.databases.list.mockResolvedValue([])
  api.databases.create.mockResolvedValue({
    id: 'db-1',
    name: 'demo',
    status: 'ready',
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z'
  })
  api.databases.remove.mockResolvedValue(undefined)
  api.databases.schema.mockResolvedValue({ tables: [] })
  api.databases.tableRows.mockResolvedValue({
    table: '',
    columns: [],
    rows: [],
    limit: 50,
    offset: 0,
    total: 0
  })
  api.databases.createTable.mockResolvedValue({ name: 't', columns: [] })
  api.databases.addColumn.mockResolvedValue({ name: 'c', type: 'VARCHAR', nullable: true })
  api.sql.query.mockResolvedValue({
    columns: ['id'],
    rows: [['1']],
    rowCount: 1,
    durationMs: 2,
    requestId: 'r1'
  })
  api.sql.execute.mockResolvedValue({
    rowsAffected: 1,
    durability: 'ok',
    durationMs: 3,
    requestId: 'r2'
  })
  api.sql.batch.mockResolvedValue({
    results: [{ rowsAffected: 1, durationMs: 1 }],
    durability: 'ok',
    durationMs: 4,
    requestId: 'r3'
  })
  api.db.collections.mockResolvedValue(['users'])
  api.db.createCollection.mockResolvedValue(undefined)
  api.db.rows.mockResolvedValue([{ id: 'row-1', name: 'Ada' }])
  api.db.insert.mockResolvedValue({ id: 'row-2' })
  api.db.update.mockResolvedValue({ id: 'row-1' })
  api.db.remove.mockResolvedValue(undefined)
  api.db.collectionItems.mockResolvedValue([])
  api.db.documents.mockResolvedValue({ rows: [], items: [] })
  api.db.document.mockResolvedValue({ id: 'row-1', createdAt: '', updatedAt: '', data: {} })
  api.db.patch.mockResolvedValue({ id: 'row-1' })
  api.db.batch.mockResolvedValue([])
  api.db.truncate.mockResolvedValue(undefined)
  api.db.getCollectionSchema.mockResolvedValue({ name: 'users', fields: [] })
  api.db.putCollectionSchema.mockResolvedValue({ name: 'users', fields: [] })
  api.db.deleteCollectionSchema.mockResolvedValue(undefined)
  api.s3.list.mockResolvedValue([])
  api.s3.listDetailed.mockResolvedValue({ folders: [], objects: [] })
  api.s3.presign.mockResolvedValue({ url: 'https://example.test/obj' })
  api.s3.download.mockResolvedValue(new Blob(['x']))
  api.s3.createLink.mockResolvedValue({ url: '/files/token', expiresAt: '2026-01-01T00:00:00Z' })
  api.s3.remove.mockResolvedValue(undefined)
  api.s3.upload.mockResolvedValue({ key: 'a.txt', size: 1 })
  api.backups.list.mockResolvedValue([])
  api.backups.create.mockResolvedValue({ id: 'b1', projectId: 'p', sizeBytes: 1, sha256: 'x', engine: 'duckdb', formatVersion: 1, status: 'ready', createdAt: '2026-01-01T00:00:00Z' })
  api.backups.remove.mockResolvedValue(undefined)
  api.backups.restore.mockResolvedValue(undefined)
  api.backups.download.mockResolvedValue(new Blob(['x']))
  api.logs.list.mockResolvedValue([])
  api.logs.stats.mockResolvedValue([])
  api.logs.export.mockResolvedValue(new Blob(['id\n']))
  api.logs.getRetention.mockResolvedValue({ scope: 'project', keepDays: 14, updatedAt: '2024-01-01T00:00:00Z' })
  api.logs.putRetention.mockResolvedValue(undefined)
  api.quota.status.mockResolvedValue({ llmAllowed: true, databaseAllowed: true })
  api.llm.chat.mockResolvedValue({ content: 'hi', model: 'm', provider: 'p' })
  api.llm.stream.mockReturnValue({ close: vi.fn() })
  api.llmSettings.get.mockResolvedValue({
    defaultProvider: 'openai',
    defaultModel: 'gpt-4o-mini',
    baseUrl: '',
    apiKeyMasked: '',
    hasApiKey: false,
    temperature: 0.7,
    maxTokens: 1024
  })
  api.llmSettings.put.mockResolvedValue({
    defaultProvider: 'openai',
    defaultModel: 'gpt-4o-mini',
    baseUrl: '',
    apiKeyMasked: '',
    hasApiKey: false,
    temperature: 0.7,
    maxTokens: 1024
  })
  api.agents.modules.mockResolvedValue([
    {
      id: 'database',
      name: 'Database',
      description: 'db helper',
      default_tools: ['list_databases'],
      team_supported: false
    },
    {
      id: 's3',
      name: 'S3',
      description: 'objects',
      default_tools: ['list_objects'],
      team_supported: false
    }
  ])
  api.agents.models.mockResolvedValue({ default_model: '', models: [] })
  api.agents.list.mockResolvedValue([])
  api.agents.create.mockResolvedValue({
    id: 'ag-1',
    name: 'Database',
    module: 'database',
    description: '',
    system_prompt: '',
    tool_ids: ['list_databases'],
    team_enabled: false,
    created_at: 't',
    updated_at: 't'
  })
  api.agents.patch.mockResolvedValue({})
  api.agents.remove.mockResolvedValue(undefined)
  api.agentThreads.list.mockResolvedValue([])
  api.agentThreads.page.mockResolvedValue({ threads: [], next_cursor: '' })
  api.agentThreads.rename.mockResolvedValue({ id: 'th-1', title: '已重命名', created_at: 't', updated_at: 't' })
  api.agentThreads.runs.mockResolvedValue([])
  api.agentThreads.create.mockResolvedValue({ id: 'th-1', title: '云助手', created_at: 't', updated_at: 't' })
  api.agentThreads.messages.mockResolvedValue([])
  api.agentThreads.streamRun.mockReturnValue({ close: vi.fn() })
  api.agentThreads.attachRun.mockReturnValue({ close: vi.fn() })
  api.agentThreads.active.mockResolvedValue([])
  api.agentThreads.cancel.mockResolvedValue({ id: 'run-1', thread_id: 'th-1', agent_id: 'ag-1', status: 'canceled' })
  api.agentThreads.confirm.mockResolvedValue(undefined)
  api.agentSchedules.list.mockResolvedValue([])
  api.agentSchedules.create.mockResolvedValue({
    id: 'sch-1',
    agent_id: 'ag-1',
    thread_id: 'th-1',
    prompt: 'p',
    cron_expr: '0 8 * * *',
    enabled: true,
    created_at: 't',
    updated_at: 't'
  })
  api.agentSchedules.patch.mockResolvedValue({
    id: 'sch-1',
    agent_id: 'ag-1',
    thread_id: 'th-1',
    prompt: 'p',
    cron_expr: '0 8 * * *',
    enabled: true,
    created_at: 't',
    updated_at: 't'
  })
  api.agentSchedules.remove.mockResolvedValue(undefined)
  api.agentSchedules.runs.mockResolvedValue([])
  api.storage.get.mockResolvedValue({
    database: 'local',
    files: 'local',
    data_dir: '/tmp/simplebase-data',
    cache_dir: '/tmp/simplebase-cache',
    bucket: '',
    prefix: '',
    source: { database: 'yaml', files: 'yaml', data_dir: 'default' },
    config_path: 'config.yaml',
    writable_config: true,
    data_present: { local: false }
  })
  api.storage.put.mockResolvedValue({
    settings: {},
    restart_required: true,
    reset_required: true,
    commands: ['./build.sh reset', './build.sh dev']
  })
  api.agentSchedules.trigger.mockResolvedValue(undefined)
  api.gofunctions.list.mockResolvedValue([])
  api.gofunctions.create.mockResolvedValue({
    id: 'gf-1',
    name: 'hello',
    file: 'hello.go',
    exports: ['Hello'],
    createdAt: 't',
    updatedAt: 't'
  })
  api.gofunctions.get.mockResolvedValue({
    id: 'gf-1',
    name: 'hello',
    file: 'hello.go',
    source: 'package main\nfunc Hello() string { return "hi" }\n',
    exports: ['Hello'],
    createdAt: 't',
    updatedAt: 't'
  })
  api.gofunctions.saveVersion.mockResolvedValue({
    id: 'gf-1',
    name: 'hello',
    file: 'hello.go',
    activeVersion: 2,
    latestVersion: 2,
    published: true,
    exports: ['Hello'],
    createdAt: 't',
    updatedAt: 't'
  })
  api.gofunctions.listVersions.mockResolvedValue({
    activeVersion: 1,
    versions: [{ version: 1, exports: ['Hello'], note: '', createdAt: 't', active: true }]
  })
  api.gofunctions.activate.mockResolvedValue({ activeVersion: 1 })
  api.gofunctions.test.mockResolvedValue({
    ok: true,
    statusCode: 200,
    durationMs: 5,
    version: 1,
    activeVersion: 1,
    functionName: 'Hello',
    data: {},
    error: ''
  })
  api.gofunctions.remove.mockResolvedValue(undefined)
  api.sandboxes.capabilities.mockResolvedValue({ available: false, backend: 'local', images: [], defaultImage: '',
    cpus: 1, memoryMiB: 512, timeoutDefaultS: 300, timeoutMaxS: 3600, maxFileBytes: 1048576, maxOutputBytes: 65536,
    maxPerProject: 5, terminal: false })
  api.sandboxes.list.mockResolvedValue([])
  api.sandboxes.create.mockResolvedValue({ id: 'sbx-1', name: 'test', state: 'running', status: 'running', source: 'api' })
  api.sandboxes.connect.mockResolvedValue({ id: 'sbx-1', name: 'test', state: 'running', status: 'running', source: 'api' })
  api.sandboxes.pause.mockResolvedValue(undefined)
  api.sandboxes.remove.mockResolvedValue(undefined)
  api.sandboxes.terminalTicket.mockResolvedValue({ ticket: 'tk', expiresIn: 30 })
  api.sandboxes.terminalWebSocketUrl.mockImplementation((id: string, ticket: string) => `ws://localhost/e2b/ext/sandboxes/${id}/terminal?ticket=${ticket}`)
  api.sandboxes.files.list.mockResolvedValue([])
  api.sandboxes.files.read.mockResolvedValue(new Blob([]))
  api.sandboxes.files.write.mockResolvedValue(undefined)
  api.sandboxes.files.remove.mockResolvedValue(undefined)
  api.cronjobs.list.mockResolvedValue([])
  api.cronjobs.create.mockResolvedValue({
    id: 'cj-1',
    name: 'nightly',
    description: '',
    scheduleKind: 'cron',
    cronExpr: '0 2 * * *',
    funcFile: 'hello',
    funcExport: 'Hello',
    inputJson: '{}',
    enabled: true,
    lastStatus: '',
    lastError: '',
    runCount: 0,
    targetMissing: false,
    createdAt: 't',
    updatedAt: 't',
    nextRunAt: '2024-01-02T00:00:00Z'
  })
  api.cronjobs.update.mockResolvedValue({
    id: 'cj-1',
    name: 'nightly',
    description: '',
    scheduleKind: 'cron',
    cronExpr: '0 2 * * *',
    funcFile: 'hello',
    funcExport: 'Hello',
    inputJson: '{}',
    enabled: false,
    lastStatus: '',
    lastError: '',
    runCount: 0,
    targetMissing: false,
    createdAt: 't',
    updatedAt: 't'
  })
  api.cronjobs.remove.mockResolvedValue(undefined)
  api.cronjobs.runs.mockResolvedValue([])
  api.cronjobs.trigger.mockResolvedValue(undefined)
  api.apiKeys.list.mockResolvedValue([])
  api.apiKeys.create.mockResolvedValue({ id: 'k1', secret: 'sb_live_mock_secret', permissions: [] })
  api.apiKeys.revoke.mockResolvedValue(undefined)
  // kv 默认值：空库语义（SCAN 返回空页；读命令返回空回复），避免组件挂载时报错。
  // 测试按需 mockResolvedValueOnce 覆盖；exec 的第一参数是 projectId。
  api.kv.exec.mockImplementation(async (_pid: string, body: { type: string; argvs?: string[] }) => {
    const argvs = body.argvs ?? []
    switch (argvs[0]?.toUpperCase()) {
      case 'SCAN':
        return ['0', []]
      case 'TYPE':
        return 'none'
      case 'PTTL':
      case 'TTL':
        return -2
      case 'DBSIZE':
        return 0
      case 'HLEN':
      case 'LLEN':
      case 'SCARD':
      case 'ZCARD':
        return 0
      default:
        return null
    }
  })
  api.kv.execBatch.mockImplementation(async (_pid: string, bodies: { type: string; argvs?: string[] }[]) =>
    // 批量默认与单条同语义：逐条委托 exec 的默认实现
    Promise.all(
      bodies.map(async (b) => {
        try {
          return await (api.kv.exec as any)(_pid, b)
        } catch {
          return null
        }
      })
    )
  )
}

export function resetApiMocks() {
  for (const ns of Object.values(api)) {
    for (const fn of Object.values(ns)) {
      if (typeof fn === 'function' && 'mockReset' in fn) {
        ;(fn as ReturnType<typeof vi.fn>).mockReset()
      } else if (fn && typeof fn === 'object') {
        for (const nested of Object.values(fn)) {
          if (typeof nested === 'function' && 'mockReset' in nested) (nested as ReturnType<typeof vi.fn>).mockReset()
        }
      }
    }
  }
  applyApiDefaults()
}

applyApiDefaults()
