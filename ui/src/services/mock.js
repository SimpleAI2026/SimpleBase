/**
 * Mock 数据实现（VITE_USE_MOCK=true 时生效）
 * 内置内存状态，支持增删查改的完整交互闭环
 */

const delay = (ms = 260) => new Promise((r) => setTimeout(r, ms))
const rand = (min, max) => Math.floor(Math.random() * (max - min + 1)) + min
const pick = (arr) => arr[rand(0, arr.length - 1)]
const genId = () => Math.random().toString(36).slice(2, 10)
const now = () => new Date().toISOString()

function ensureMockLogs() {
  if (state.logEvents.length) return
  const levels = ['info', 'info', 'info', 'warn', 'error']
  const sources = ['http', 'db-engine', 's3-store', 'auth']
  const actions = [
    'GET /v1/projects/:projectID/databases',
    'POST /v1/projects/:projectID/databases/:id/query',
    'cache hit',
    'object uploaded',
    'slow query detected'
  ]
  for (let i = 0; i < 24; i++) {
    const occurred = new Date(Date.now() - i * 7 * 60 * 1000)
    state.logEvents.push({
      id: 'log-' + genId(),
      projectId: DEFAULT_PROJECT,
      level: pick(levels),
      logger: pick(sources),
      message: pick(actions),
      requestId: genId(),
      occurredAt: occurred.toISOString()
    })
  }
}

function requireDbStore(databaseId) {
  const db = state.databases.find((d) => d.id === databaseId)
  if (!db) throw new Error('数据库不存在')
  if (!state.dbStores[databaseId]) {
    state.dbStores[databaseId] = { collections: [], docs: {} }
  }
  return state.dbStores[databaseId]
}

/* ---------- 初始数据 ---------- */

const DEFAULT_PROJECT = '00000000-0000-0000-0000-000000000002'

const state = {
  databases: [
    { id: 'db-default', name: 'default', status: 'ready', createdAt: now(), updatedAt: now() },
    { id: 'db-analytics', name: 'analytics', status: 'creating', createdAt: now(), updatedAt: now() }
  ],
  dbStores: {
    'db-default': {
      collections: ['users', 'orders', 'products', 'sessions'],
      docs: {
        users: [
          { id: 'u1001', name: 'Alice', role: 'admin', age: 28 },
          { id: 'u1002', name: 'Bob', role: 'member', age: 34 },
          { id: 'u1003', name: 'Carol', role: 'member', age: 25 }
        ],
        orders: [
          { id: 'o9001', user: 'u1001', amount: 199.5, status: 'paid' },
          { id: 'o9002', user: 'u1002', amount: 59.0, status: 'pending' }
        ],
        products: [
          { id: 'p3001', title: '机械键盘', price: 399, stock: 42 },
          { id: 'p3002', title: '无线鼠标', price: 129, stock: 187 }
        ],
        sessions: [{ id: 's5001', user: 'u1001', ttl: 3600 }]
      }
    },
    'db-analytics': { collections: [], docs: {} }
  },
  // 注意：mock 的 S3 list 按 _projectId 过滤，种子数据必须带该字段（否则列表恒空）
  objects: [
    { key: 'backups/2026-07-01.dump', size: 8388608, lastModified: now(), _projectId: DEFAULT_PROJECT },
    { key: 'images/logo.png', size: 24576, lastModified: now(), _projectId: DEFAULT_PROJECT },
    { key: 'images/banner.webp', size: 188416, lastModified: now(), _projectId: DEFAULT_PROJECT },
    { key: 'docs/getting-started.md', size: 5120, lastModified: now(), _projectId: DEFAULT_PROJECT }
  ],
  projects: [
    { id: DEFAULT_PROJECT, name: '商城后台', createdAt: now() },
    { id: '11111111-1111-1111-1111-111111111111', name: '示例项目', createdAt: now() }
  ],
  logEvents: [],
  logRetention: { [DEFAULT_PROJECT]: { scope: 'project', keepDays: 14, updatedAt: now() } },
  llmSettings: {},
  agents: [
    {
      id: 'ag-db',
      name: 'Database',
      module: 'database',
      description: 'Inspect project databases and run readonly SQL',
      system_prompt: '',
      tool_ids: ['list_databases', 'list_collections', 'readonly_sql'],
      team_enabled: false,
      created_at: now(),
      updated_at: now(),
      _projectId: DEFAULT_PROJECT
    },
    {
      id: 'ag-s3',
      name: 'S3',
      module: 's3',
      description: 'List and inspect project object storage',
      system_prompt: '',
      tool_ids: ['list_objects', 'head_object'],
      team_enabled: false,
      created_at: now(),
      updated_at: now(),
      _projectId: DEFAULT_PROJECT
    },
    {
      id: 'ag-logs',
      name: 'Logs',
      module: 'logs',
      description: 'Search project logs and summarize levels',
      system_prompt: '',
      tool_ids: ['search_logs', 'log_level_stats'],
      team_enabled: false,
      created_at: now(),
      updated_at: now(),
      _projectId: DEFAULT_PROJECT
    }
  ],
  agentThreads: [],
  agentMessages: {},
  agentSchedules: [],
  agentScheduleRuns: {}
}

/* ---------- Mock API ---------- */

export const mockApi = {
  projects: {
    async list() {
      await delay(80)
      return state.projects.map((p) => ({ ...p }))
    },
    async create(req) {
      await delay(80)
      const name = String(req?.name || '').trim()
      if (!name) throw new Error('name is required')
      const id = String(req?.id || '').trim() || crypto.randomUUID()
      if (state.projects.some((p) => p.id === id || p.name === name)) {
        throw new Error('project already exists')
      }
      const item = { id, name, createdAt: now() }
      state.projects.push(item)
      return { ...item }
    }
  },
  metrics: {
    async summary(_projectId) {
      await delay()
      return {
        totalRequests: rand(8000, 20000),
        errorRate: +(Math.random() * 2).toFixed(2),
        avgLatencyMs: rand(18, 120),
        activeDatabases: state.databases.length
      }
    },
    async trend(_projectId) {
      await delay()
      const days = []
      for (let i = 6; i >= 0; i--) {
        const d = new Date(Date.now() - i * 86400000)
        days.push({
          date: `${d.getMonth() + 1}/${d.getDate()}`,
          requests: rand(400, 2400),
          errors: rand(0, 60)
        })
      }
      return days
    }
  },

  databases: {
    async list() {
      await delay()
      return state.databases.map((d) => ({ ...d }))
    },
    async create(projectId, name) {
      await delay()
      const db = { id: 'db-' + genId(), name, status: 'creating', createdAt: now(), updatedAt: now() }
      state.databases.push(db)
      state.dbStores[db.id] = { collections: [], docs: {} }
      return { ...db }
    },
    async get(projectId, databaseId) {
      await delay()
      const db = state.databases.find((d) => d.id === databaseId)
      if (!db) throw new Error('数据库不存在')
      return { ...db }
    },
    async open(projectId, databaseId) {
      await delay(400)
      const db = state.databases.find((d) => d.id === databaseId)
      if (!db) throw new Error('数据库不存在')
      db.status = 'ready'
      db.updatedAt = now()
      return { ...db }
    },
    async close(projectId, databaseId) {
      await delay(300)
      const db = state.databases.find((d) => d.id === databaseId)
      if (!db) throw new Error('数据库不存在')
      db.status = 'closed'
      db.updatedAt = now()
    },
    async remove(projectId, databaseId) {
      await delay(400)
      const idx = state.databases.findIndex((d) => d.id === databaseId)
      if (idx >= 0) state.databases.splice(idx, 1)
      delete state.dbStores[databaseId]
    }
  },

  sql: {
    async query(projectId, databaseId, req) {
      await delay(rand(80, 400))
      const store = requireDbStore(databaseId)
      if (/from\s+"?users"?/i.test(req.sql)) {
        const rows = (store.docs.users || []).map((u) => [u.id, JSON.stringify(u), now()])
        return {
          columns: ['id', 'data', 'created_at'],
          rows,
          rowCount: rows.length,
          durationMs: rand(2, 30),
          requestId: genId()
        }
      }
      if (/information_schema/i.test(req.sql)) {
        const rows = store.collections.map((c) => [c])
        return { columns: ['table_name'], rows, rowCount: rows.length, durationMs: 3, requestId: genId() }
      }
      return { columns: [], rows: [], rowCount: 0, durationMs: rand(1, 10), requestId: genId() }
    },
    async execute(projectId, databaseId, req) {
      await delay(rand(100, 500))
      return { rowsAffected: rand(0, 3), durability: 'committed_local', durationMs: rand(2, 40), requestId: genId() }
    },
    async batch(projectId, databaseId, req) {
      await delay(rand(200, 600))
      return {
        results: req.statements.map((s, i) => ({
          index: i,
          rowsAffected: rand(0, 3),
          durationMs: rand(2, 20)
        })),
        durability: 'committed_local',
        durationMs: rand(10, 80),
        requestId: genId()
      }
    }
  },

  db: {
    async collections(projectId, databaseId) {
      await delay()
      return [...requireDbStore(databaseId).collections]
    },
    async createCollection(projectId, databaseId, name) {
      await delay()
      const store = requireDbStore(databaseId)
      if (!store.docs[name]) {
        store.docs[name] = []
        store.collections.push(name)
      }
    },
    async rows(projectId, databaseId, collection) {
      await delay()
      const store = requireDbStore(databaseId)
      return [...(store.docs[collection] || [])]
    },
    async insert(projectId, databaseId, collection, payload) {
      await delay()
      const store = requireDbStore(databaseId)
      if (!store.docs[collection]) {
        store.docs[collection] = []
        store.collections.push(collection)
      }
      const row = { id: genId(), ...payload }
      store.docs[collection].push(row)
      return row
    },
    async update(projectId, databaseId, collection, id, payload) {
      await delay()
      const store = requireDbStore(databaseId)
      const row = (store.docs[collection] || []).find((item) => item.id === id)
      if (!row) throw new Error('文档不存在')
      Object.assign(row, { ...payload, id })
      return row
    },
    async remove(projectId, databaseId, collection, id) {
      await delay()
      const store = requireDbStore(databaseId)
      const list = store.docs[collection] || []
      store.docs[collection] = list.filter((r) => r.id !== id)
    }
  },

  s3: {
    async list(projectId, prefix) {
      await delay()
      return state.objects.filter(
        (o) => o._projectId === projectId && (!prefix || o.key.startsWith(prefix))
      )
    },
    async presign(projectId, key) {
      await delay()
      return { url: `https://mock-cdn.local/${encodeURIComponent(key)}?token=${genId()}` }
    },
    async remove(projectId, key) {
      await delay()
      state.objects = state.objects.filter((o) => !(o._projectId === projectId && o.key === key))
    },
    async upload(projectId, key, file) {
      await delay(500)
      const obj = { key, size: file.size, lastModified: now(), _projectId: projectId }
      state.objects.unshift(obj)
      return obj
    }
  },

  logs: {
    async list(projectId, q = {}) {
      await delay()
      ensureMockLogs()
      const level = String(q.level || '').toLowerCase()
      const keyword = String(q.q || '').toLowerCase()
      const from = q.from ? Date.parse(q.from) : 0
      const to = q.to ? Date.parse(q.to) : 0
      const limit = q.limit || 100
      return state.logEvents
        .filter((e) => e.projectId === projectId)
        .filter((e) => !level || e.level === level)
        .filter((e) => !keyword || e.message.toLowerCase().includes(keyword))
        .filter((e) => !from || Date.parse(e.occurredAt) >= from)
        .filter((e) => !to || Date.parse(e.occurredAt) <= to)
        .slice(0, limit)
    },
    async getRetention(projectId) {
      await delay(80)
      return state.logRetention[projectId] || { scope: 'project', keepDays: 14, updatedAt: now() }
    },
    async putRetention(projectId, keepDays) {
      await delay(80)
      state.logRetention[projectId] = { scope: 'project', keepDays, updatedAt: now() }
    }
  },

  quota: {
    async status() {
      await delay(200)
      return { llmAllowed: true, databaseAllowed: true }
    }
  },

  llm: {
    async providers(projectId) {
      await delay()
      return ['openai', 'anthropic', 'gemini']
    },
    async chat(projectId, req) {
      await delay(rand(500, 1200))
      const last = req.messages?.[req.messages.length - 1]?.content || ''
      const prompt = req.messages?.reduce((sum, m) => sum + (m.content?.length || 0), 0) || 0
      const content = `这是 Mock 回复（项目 ${projectId}）。你刚才说：「${last.slice(0, 80)}」`
      const completion = content.length
      return {
        content,
        model: req.model || 'mock-gpt-4o',
        provider: 'openai',
        finish_reason: 'stop',
        usage: {
          prompt_tokens: Math.ceil(prompt / 4),
          completion_tokens: Math.ceil(completion / 4),
          total_tokens: Math.ceil((prompt + completion) / 4)
        }
      }
    },
    stream(projectId, req, { onChunk, onEnd, onError }) {
      const last = req.messages?.[req.messages.length - 1]?.content || ''
      const text = `这是 Mock 流式回复（项目 ${projectId}）。你刚才说：「${last.slice(0, 80)}」。以下内容按块推送，用于验证 SSE 渲染逻辑是否正常。`
      const chunks = text.match(/[\s\S]{1,4}/g) || []
      let i = 0
      let closed = false
      const timer = setInterval(() => {
        if (closed) return
        if (i >= chunks.length) {
          clearInterval(timer)
          closed = true
          onEnd?.()
          return
        }
        try {
          onChunk?.(chunks[i++])
        } catch (e) {
          clearInterval(timer)
          closed = true
          onError?.(e)
        }
      }, 60)
      return {
        close() {
          if (!closed) {
            closed = true
            clearInterval(timer)
          }
        }
      }
    }
  },

  llmSettings: {
    async get(projectId) {
      await delay(80)
      return {
        defaultProvider: 'openai',
        defaultModel: 'gpt-4o-mini',
        temperature: 0.7,
        maxTokens: 1024,
        ...(state.llmSettings[projectId] || {})
      }
    },
    async put(projectId, settings) {
      await delay(80)
      state.llmSettings[projectId] = {
        defaultProvider: settings.defaultProvider,
        defaultModel: settings.defaultModel,
        temperature: settings.temperature ?? 0.7,
        maxTokens: settings.maxTokens ?? 1024
      }
      return { ...state.llmSettings[projectId] }
    }
  },

  agents: {
    async modules() {
      await delay(80)
      return [
        { id: 'database', name: 'Database', description: 'Readonly SQL', default_tools: ['list_databases', 'list_collections', 'readonly_sql'], team_supported: false },
        { id: 's3', name: 'S3', description: 'List objects', default_tools: ['list_objects', 'head_object'], team_supported: false },
        { id: 'logs', name: 'Logs', description: 'Search logs', default_tools: ['search_logs', 'log_level_stats'], team_supported: false },
        { id: 'general', name: 'General', description: 'Custom prompt', default_tools: [], team_supported: false }
      ]
    },
    async list(projectId) {
      await delay()
      return state.agents.filter((a) => a._projectId === projectId).map((a) => ({ ...a }))
    },
    async create(projectId, body) {
      await delay()
      const row = {
        id: 'ag-' + genId(),
        name: body.name || 'Agent',
        module: body.module || 'general',
        description: body.description || '',
        system_prompt: body.system_prompt || '',
        tool_ids: body.tool_ids || [],
        team_enabled: false,
        created_at: now(),
        updated_at: now(),
        _projectId: projectId
      }
      state.agents.push(row)
      return { ...row }
    },
    async get(projectId, agentId) {
      await delay(80)
      const row = state.agents.find((a) => a.id === agentId && a._projectId === projectId)
      if (!row) throw new Error('agent not found')
      return { ...row }
    },
    async patch(projectId, agentId, body) {
      await delay()
      const row = state.agents.find((a) => a.id === agentId && a._projectId === projectId)
      if (!row) throw new Error('agent not found')
      Object.assign(row, body, { updated_at: now() })
      return { ...row }
    },
    async remove(projectId, agentId) {
      await delay()
      state.agents = state.agents.filter((a) => !(a.id === agentId && a._projectId === projectId))
    }
  },

  agentThreads: {
    async list(projectId) {
      await delay()
      return state.agentThreads.filter((t) => t._projectId === projectId).map((t) => ({ ...t }))
    },
    async create(projectId, title) {
      await delay()
      const row = { id: 'th-' + genId(), title: title || 'New thread', created_at: now(), updated_at: now(), _projectId: projectId }
      state.agentThreads.unshift(row)
      state.agentMessages[row.id] = []
      return { ...row }
    },
    async get(projectId, threadId) {
      const row = state.agentThreads.find((t) => t.id === threadId && t._projectId === projectId)
      if (!row) throw new Error('thread not found')
      return { ...row }
    },
    async remove(projectId, threadId) {
      state.agentThreads = state.agentThreads.filter((t) => t.id !== threadId)
    },
    async messages(projectId, threadId) {
      await delay()
      return (state.agentMessages[threadId] || []).map((m) => ({ ...m }))
    },
    async run(projectId, threadId, req) {
      await delay(400)
      const msgs = state.agentMessages[threadId] || (state.agentMessages[threadId] = [])
      msgs.push({
        id: 'm-' + genId(),
        role: 'user',
        content: req.content,
        mentions: req.mentions,
        created_at: now()
      })
      const reply = {
        id: 'm-' + genId(),
        role: 'assistant',
        content: `Mock Cloud Agent（项目 ${projectId}）：已收到「${String(req.content).slice(0, 80)}」`,
        tool_calls: [{ name: 'list_databases', content: '[{"name":"default"}]' }],
        created_at: now()
      }
      msgs.push(reply)
      return { run: { id: 'run-' + genId(), thread_id: threadId, agent_id: req.mentions?.[0]?.agent_id, status: 'completed' }, message: reply }
    },
    streamRun(projectId, threadId, req, { onToken, onToolCall, onToolResult, onEnd, onError }) {
      const text = `Mock 流式回复：${String(req.content || '').slice(0, 40)}`
      const chunks = text.match(/[\s\S]{1,4}/g) || []
      let i = 0
      let closed = false
      onToolCall?.('list_databases', '{}')
      onToolResult?.('list_databases', '[{"name":"default"}]')
      const timer = setInterval(() => {
        if (closed) return
        if (i >= chunks.length) {
          clearInterval(timer)
          closed = true
          const msgs = state.agentMessages[threadId] || (state.agentMessages[threadId] = [])
          msgs.push({ id: 'm-' + genId(), role: 'user', content: req.content, mentions: req.mentions, created_at: now() })
          msgs.push({ id: 'm-' + genId(), role: 'assistant', content: text, created_at: now() })
          onEnd?.()
          return
        }
        try {
          onToken?.(chunks[i++])
        } catch (e) {
          clearInterval(timer)
          closed = true
          onError?.(e)
        }
      }, 50)
      return {
        close() {
          if (!closed) {
            closed = true
            clearInterval(timer)
          }
        }
      }
    },
    async cancel() {
      await delay(80)
      return { id: '', thread_id: '', agent_id: '', status: 'canceled' }
    }
  },

  agentSchedules: {
    async list(projectId) {
      await delay()
      return state.agentSchedules
        .filter((s) => s._projectId === projectId)
        .map((s) => {
          const agent = state.agents.find((a) => a.id === s.agent_id)
          return { ...s, agent_name: agent ? agent.name : '' }
        })
    },
    async create(projectId, body) {
      await delay()
      const agent = state.agents.find((a) => a.id === body.agent_id && a._projectId === projectId)
      if (!agent) throw new Error('agent not found')
      if (state.agentSchedules.some((s) => s.agent_id === body.agent_id && s._projectId === projectId)) {
        const err = new Error('schedule already exists')
        err.status = 409
        throw err
      }
      const thread = { id: 'th-' + genId(), title: 'Scheduled: ' + agent.name, created_at: now(), updated_at: now(), _projectId: projectId }
      state.agentThreads.unshift(thread)
      state.agentMessages[thread.id] = []
      const row = {
        id: 'sch-' + genId(),
        agent_id: body.agent_id,
        agent_name: agent.name,
        thread_id: thread.id,
        prompt: body.prompt,
        cron_expr: body.cron_expr,
        enabled: body.enabled !== false,
        next_run_at: now(),
        created_at: now(),
        updated_at: now(),
        _projectId: projectId
      }
      state.agentSchedules.push(row)
      return { ...row }
    },
    async patch(projectId, scheduleId, body) {
      await delay()
      const row = state.agentSchedules.find((s) => s.id === scheduleId && s._projectId === projectId)
      if (!row) throw new Error('schedule not found')
      if (body.prompt !== undefined) row.prompt = body.prompt
      if (body.cron_expr !== undefined) row.cron_expr = body.cron_expr
      if (body.enabled !== undefined) {
        row.enabled = body.enabled
        row.next_run_at = body.enabled ? now() : undefined
      }
      row.updated_at = now()
      return { ...row }
    },
    async remove(projectId, scheduleId) {
      await delay()
      state.agentSchedules = state.agentSchedules.filter((s) => s.id !== scheduleId)
    },
    async runs(projectId, scheduleId) {
      await delay()
      return (state.agentScheduleRuns[scheduleId] || []).map((r) => ({ ...r }))
    },
    async trigger(projectId, scheduleId) {
      await delay()
      const row = state.agentSchedules.find((s) => s.id === scheduleId)
      if (!row) throw new Error('schedule not found')
      const runs = state.agentScheduleRuns[scheduleId] || (state.agentScheduleRuns[scheduleId] = [])
      const run = {
        id: 'srun-' + genId(),
        schedule_id: scheduleId,
        run_id: 'run-' + genId(),
        trigger: 'manual',
        status: 'running',
        started_at: now(),
        created_at: now()
      }
      runs.unshift(run)
      // 异步完成：写入结果 thread 消息并把状态推进为 completed。
      setTimeout(() => {
        const msgs = state.agentMessages[row.thread_id] || (state.agentMessages[row.thread_id] = [])
        msgs.push({ id: 'm-' + genId(), role: 'user', content: '[scheduled] ' + row.prompt, created_at: now() })
        msgs.push({
          id: 'm-' + genId(),
          role: 'assistant',
          content: `Mock 定时执行：已按提示词「${String(row.prompt).slice(0, 60)}」完成巡检。`,
          tool_calls: [{ name: 'list_databases', content: '[{"name":"default"}]' }],
          created_at: now()
        })
        run.status = 'completed'
        run.finished_at = now()
      }, 1200)
    }
  }
}
