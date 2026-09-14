import { http, wsBase, baseURL } from './http'
import type { Api, LlmChatRequest, LlmStreamHandlers, LlmStreamConnection } from './types'

/** 将 UI camelCase 请求映射为后端 snake_case DTO */
function toLlmPayload(req: LlmChatRequest) {
  return {
    model: req.model,
    messages: req.messages,
    max_tokens: req.maxTokens,
    temperature: req.temperature
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
        const data = await resp.json().catch(() => ({}))
        throw new Error(data?.error?.message || data?.message || `请求失败 (${resp.status})`)
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

/** 真实后端实现 */
export const httpApi: Api = {
  metrics: {
    summary: () =>
      http
        .get('/metrics/summary')
        .then((r) => (r.data && typeof r.data === 'object' ? r.data : {})),
    trend: () =>
      http
        .get('/metrics/trend')
        .then((r) => (Array.isArray(r.data) ? r.data : []))
  },
  db: {
    collections: () =>
      http
        .get('/v1/projects/proj-01/data/collections')
        .then((r) => (Array.isArray(r.data?.collections) ? r.data.collections : [])),
    createCollection: (name) =>
      http.post('/v1/projects/proj-01/data/collections', { name }).then(() => undefined),
    rows: (collection, query) =>
      http
        .get('/v1/projects/proj-01/data/collections/' + encodeURIComponent(collection), { params: query })
        .then((r) => (Array.isArray(r.data?.rows) ? r.data.rows : [])),
    insert: (collection, payload) =>
      http
        .post('/v1/projects/proj-01/data/collections/' + encodeURIComponent(collection) + '/documents', payload)
        .then((r) => r.data),
    update: (collection, id, payload) =>
      http
        .put(
          '/v1/projects/proj-01/data/collections/' +
            encodeURIComponent(collection) +
            '/documents/' +
            encodeURIComponent(id),
          payload
        )
        .then((r) => r.data),
    remove: (collection, id) =>
      http
        .delete(
          '/v1/projects/proj-01/data/collections/' +
            encodeURIComponent(collection) +
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
  faas: {
    list: () =>
      http.get('/faas/functions').then((r) => (Array.isArray(r.data) ? r.data : [])),
    deploy: (name, file) => {
      const fd = new FormData()
      fd.append('name', name)
      fd.append('file', file)
      return http.post('/faas/deploy', fd).then((r) => r.data)
    },
    invoke: (name, payload) =>
      http.post('/faas/invoke/' + encodeURIComponent(name), payload).then((r) => r.data)
  },
  logs: {
    connect({ onOpen, onMessage, onClose, onError }) {
      const ws = new WebSocket(wsBase + '/ws/logs')
      ws.onopen = () => onOpen?.()
      ws.onmessage = (ev) => onMessage?.(ev.data)
      ws.onerror = (e) => onError?.(e)
      ws.onclose = () => onClose?.()
      return { close: () => ws.close() }
    }
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
  }
}
