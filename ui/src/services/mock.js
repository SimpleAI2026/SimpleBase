/**
 * Mock 数据实现（VITE_USE_MOCK=true 时生效）
 * 内置内存状态，支持增删查改的完整交互闭环
 */

const delay = (ms = 260) => new Promise((r) => setTimeout(r, ms))
const rand = (min, max) => Math.floor(Math.random() * (max - min + 1)) + min
const pick = (arr) => arr[rand(0, arr.length - 1)]
const genId = () => Math.random().toString(36).slice(2, 10)
const now = () => new Date().toISOString()

/* ---------- 初始数据 ---------- */

const state = {
  collections: ['users', 'orders', 'products', 'sessions'],
  db: {
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
  },
  objects: [
    { key: 'backups/2026-07-01.dump', size: 8388608, lastModified: now() },
    { key: 'images/logo.png', size: 24576, lastModified: now() },
    { key: 'images/banner.webp', size: 188416, lastModified: now() },
    { key: 'docs/getting-started.md', size: 5120, lastModified: now() }
  ],
  functions: [
    { name: 'resize-image', version: 'v1.4.2', runtime: 'node20', updatedAt: now() },
    { name: 'daily-report', version: 'v0.9.0', runtime: 'node20', updatedAt: now() },
    { name: 'webhook-relay', version: 'v2.1.0', runtime: 'node20', updatedAt: now() }
  ]
}

/* ---------- Mock API ---------- */

export const mockApi = {
  metrics: {
    async summary() {
      await delay()
      return {
        totalRequests: rand(8000, 20000),
        errorRate: +(Math.random() * 2).toFixed(2),
        avgLatencyMs: rand(18, 120),
        activeFunctions: state.functions.length
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

  db: {
    async collections() {
      await delay()
      return [...state.collections]
    },
    async createCollection(name) {
      await delay()
      if (!state.db[name]) {
        state.db[name] = []
        state.collections.push(name)
      }
    },
    async rows(collection) {
      await delay()
      return [...(state.db[collection] || [])]
    },
    async insert(collection, payload) {
      await delay()
      if (!state.db[collection]) {
        state.db[collection] = []
        state.collections.push(collection)
      }
      const row = { id: genId(), ...payload }
      state.db[collection].push(row)
      return row
    },
    async update(collection, id, payload) {
      await delay()
      const row = (state.db[collection] || []).find((item) => item.id === id)
      if (!row) throw new Error('文档不存在')
      Object.assign(row, { ...payload, id })
      return row
    },
    async remove(collection, id) {
      await delay()
      const list = state.db[collection] || []
      state.db[collection] = list.filter((r) => r.id !== id)
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
