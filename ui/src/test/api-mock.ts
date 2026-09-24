import { vi } from 'vitest'

let mockFlag = false

export function getIsMock() {
  return mockFlag
}

export function setIsMock(v: boolean) {
  mockFlag = v
}

export const api = {
  auth: {
    login: vi.fn(),
    refresh: vi.fn(),
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
    open: vi.fn(),
    close: vi.fn(),
    remove: vi.fn()
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
    remove: vi.fn()
  },
  s3: {
    list: vi.fn(),
    presign: vi.fn(),
    remove: vi.fn(),
    upload: vi.fn()
  },
  logs: {
    list: vi.fn(),
    getRetention: vi.fn(),
    putRetention: vi.fn()
  },
  quota: {
    status: vi.fn()
  },
  llm: {
    providers: vi.fn(),
    chat: vi.fn(),
    stream: vi.fn()
  },
  llmSettings: {
    get: vi.fn(),
    put: vi.fn()
  },
  agents: {
    modules: vi.fn(),
    list: vi.fn(),
    create: vi.fn(),
    get: vi.fn(),
    patch: vi.fn(),
    remove: vi.fn()
  },
  agentThreads: {
    list: vi.fn(),
    create: vi.fn(),
    get: vi.fn(),
    remove: vi.fn(),
    messages: vi.fn(),
    run: vi.fn(),
    streamRun: vi.fn(),
    cancel: vi.fn()
  },
  agentSchedules: {
    list: vi.fn(),
    create: vi.fn(),
    patch: vi.fn(),
    remove: vi.fn(),
    runs: vi.fn(),
    trigger: vi.fn()
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
  cronjobs: {
    list: vi.fn(),
    create: vi.fn(),
    get: vi.fn(),
    update: vi.fn(),
    remove: vi.fn(),
    runs: vi.fn(),
    trigger: vi.fn()
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
  api.auth.refresh.mockResolvedValue({
    tokenType: 'Bearer',
    accessToken: 'at2',
    expiresIn: 7200,
    refreshToken: 'rt2',
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
  api.users.get.mockResolvedValue({
    id: 'u1',
    username: 'simplebase2026',
    role: 'superadminl1',
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
  api.users.remove.mockResolvedValue(undefined)
  api.projects.list.mockResolvedValue([])
  api.projects.create.mockResolvedValue({ id: 'p-new', name: 'New', createdAt: 't' })
  api.metrics.summary.mockResolvedValue({
    totalRequests: 10,
    errorRate: 1.5,
    avgLatencyMs: 12.6,
    activeDatabases: 1
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
  api.databases.open.mockResolvedValue({
    id: 'db-1',
    name: 'demo',
    status: 'ready',
    createdAt: 't',
    updatedAt: 't'
  })
  api.databases.close.mockResolvedValue(undefined)
  api.databases.remove.mockResolvedValue(undefined)
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
  api.db.remove.mockResolvedValue(undefined)
  api.s3.list.mockResolvedValue([])
  api.s3.presign.mockResolvedValue({ url: 'https://example.test/obj' })
  api.s3.remove.mockResolvedValue(undefined)
  api.s3.upload.mockResolvedValue({ key: 'a.txt', size: 1 })
  api.logs.list.mockResolvedValue([])
  api.logs.getRetention.mockResolvedValue({ scope: 'project', keepDays: 14, updatedAt: '2024-01-01T00:00:00Z' })
  api.logs.putRetention.mockResolvedValue(undefined)
  api.quota.status.mockResolvedValue({ llmAllowed: true, databaseAllowed: true })
  api.llm.providers.mockResolvedValue(['openai'])
  api.llm.chat.mockResolvedValue({ content: 'hi', model: 'm', provider: 'p' })
  api.llm.stream.mockReturnValue({ close: vi.fn() })
  api.llmSettings.get.mockResolvedValue({ defaultProvider: 'openai', defaultModel: 'gpt-4o-mini', temperature: 0.7, maxTokens: 1024 })
  api.llmSettings.put.mockResolvedValue({ defaultProvider: 'openai' })
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
  api.agentThreads.create.mockResolvedValue({ id: 'th-1', title: '云 Agent', created_at: 't', updated_at: 't' })
  api.agentThreads.messages.mockResolvedValue([])
  api.agentThreads.streamRun.mockReturnValue({ close: vi.fn() })
  api.agentThreads.cancel.mockResolvedValue({ id: 'run-1', thread_id: 'th-1', agent_id: 'ag-1', status: 'canceled' })
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
  api.gofunctions.update.mockResolvedValue({
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
}

export function resetApiMocks() {
  for (const ns of Object.values(api)) {
    for (const fn of Object.values(ns)) {
      if (typeof fn === 'function' && 'mockReset' in fn) {
        ;(fn as ReturnType<typeof vi.fn>).mockReset()
      }
    }
  }
  mockFlag = false
  applyApiDefaults()
}

applyApiDefaults()
