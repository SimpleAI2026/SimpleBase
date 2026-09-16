/**
 * Mock 数据实现（VITE_USE_MOCK=true 时生效）
 * 内置内存状态，支持增删查改的完整交互闭环
 */

const delay = (ms = 260) => new Promise((r) => setTimeout(r, ms))
const rand = (min, max) => Math.floor(Math.random() * (max - min + 1)) + min
const pick = (arr) => arr[rand(0, arr.length - 1)]
const genId = () => Math.random().toString(36).slice(2, 10)
const now = () => new Date().toISOString()

function requireDbStore(databaseId) {
  const db = state.databases.find((d) => d.id === databaseId)
  if (!db) throw new Error('数据库不存在')
  if (!state.dbStores[databaseId]) {
    state.dbStores[databaseId] = { collections: [], docs: {} }
  }
  return state.dbStores[databaseId]
}

/* ---------- 初始数据 ---------- */

const DEFAULT_PROJECT = 'proj-01'

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
  functions: [
    { name: 'resize-image', version: 'v1.4.2', runtime: 'node20', updatedAt: now() },
    { name: 'daily-report', version: 'v0.9.0', runtime: 'node20', updatedAt: now() },
    { name: 'webhook-relay', version: 'v2.1.0', runtime: 'node20', updatedAt: now() }
  ]
}

/* ---------- Mock API ---------- */

export const mockApi = {
  projects: {
    list: async () => [
      { id: 'proj-01', name: '商城后台', createdAt: new Date().toISOString() },
      { id: 'proj-02', name: '示例项目', createdAt: new Date().toISOString() }
    ]
  },
  metrics: {
    async summary() {
      await delay()
      return {
        totalRequests: rand(8000, 20000),
        errorRate: +(Math.random() * 2).toFixed(2),
        avgLatencyMs: rand(18, 120),
        activeDatabases: state.databases.length
      }
    },
    async trend() {
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

  faas: {
    async list() {
      await delay()
      return [...state.functions]
    },
    async deploy(name, file) {
      await delay(600)
      const existing = state.functions.find((f) => f.name === name)
      if (existing) {
        const [major, minor, patch] = existing.version.replace('v', '').split('.').map(Number)
        existing.version = `v${major}.${minor}.${patch + 1}`
        existing.updatedAt = now()
        return existing
      }
      const fn = { name, version: 'v0.1.0', runtime: 'node20', updatedAt: now(), size: file.size }
      state.functions.push(fn)
      return fn
    },
    async invoke(name, payload) {
      await delay(400)
      return {
        ok: true,
        function: name,
        durationMs: rand(5, 180),
        echo: payload,
        requestId: genId()
      }
    }
  },

  logs: {
    connect({ onOpen, onMessage, onClose }) {
      const levels = ['INFO', 'INFO', 'INFO', 'WARN', 'ERROR']
      const sources = ['gateway', 'db-engine', 's3-store', 'faas-runtime', 'auth']
      const actions = [
        'request completed',
        'connection established',
        'cache hit',
        'retry upstream',
        'slow query detected',
        'function invoked',
        'object uploaded'
      ]
      const timer = setInterval(() => {
        const ts = new Date().toLocaleTimeString('zh-CN', { hour12: false })
        onMessage?.(
          `${ts} [${pick(levels)}] [${pick(sources)}] ${pick(actions)} cost=${rand(1, 900)}ms`
        )
      }, 800)
      setTimeout(() => onOpen?.(), 120)
      return {
        close() {
          clearInterval(timer)
          onClose?.()
        }
      }
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
  }
}
