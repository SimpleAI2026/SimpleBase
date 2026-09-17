import { errorFromResponse, SimpleBaseError } from './errors.js'
import type { CreateClientOptions } from './types.js'

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'

export interface HttpClient {
  readonly baseUrl: string
  readonly projectId: string
  request<T = unknown>(
    method: HttpMethod,
    path: string,
    opts?: {
      query?: Record<string, string | number | boolean | undefined | null>
      body?: unknown
      formData?: FormData
      headers?: Record<string, string>
    }
  ): Promise<T>
}

function joinUrl(base: string, path: string): string {
  const b = base.replace(/\/+$/, '')
  const p = path.startsWith('/') ? path : `/${path}`
  return `${b}${p}`
}

function buildQuery(query?: Record<string, string | number | boolean | undefined | null>): string {
  if (!query) return ''
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(query)) {
    if (v === undefined || v === null) continue
    sp.set(k, String(v))
  }
  const s = sp.toString()
  return s ? `?${s}` : ''
}

export function createHttpClient(opts: CreateClientOptions): HttpClient {
  const baseUrl = opts.url.replace(/\/+$/, '')
  const f = opts.fetch ?? fetch

  return {
    baseUrl,
    projectId: opts.projectId,
    async request<T>(
      method: HttpMethod,
      path: string,
      req: {
        query?: Record<string, string | number | boolean | undefined | null>
        body?: unknown
        formData?: FormData
        headers?: Record<string, string>
      } = {}
    ): Promise<T> {
      const url = joinUrl(baseUrl, path) + buildQuery(req.query)
      const headers: Record<string, string> = {
        Authorization: `Bearer ${opts.apiKey}`,
        Accept: 'application/json',
        ...(opts.headers || {}),
        ...(req.headers || {})
      }

      let body: BodyInit | undefined
      if (req.formData) {
        body = req.formData
        // let runtime set multipart boundary
      } else if (req.body !== undefined) {
        headers['Content-Type'] = 'application/json'
        body = JSON.stringify(req.body)
      }

      let res: Response
      try {
        res = await f(url, { method, headers, body })
      } catch (e) {
        throw new SimpleBaseError({
          message: e instanceof Error ? e.message : 'network error',
          status: 0,
          code: 'network_error'
        })
      }

      const requestId = res.headers.get('x-request-id') || undefined
      const text = await res.text()
      let parsed: unknown = undefined
      if (text) {
        try {
          parsed = JSON.parse(text)
        } catch {
          parsed = { message: text }
        }
      }

      if (!res.ok) {
        throw errorFromResponse(res.status, parsed, requestId)
      }

      if (res.status === 204 || text === '') {
        return undefined as T
      }
      return parsed as T
    }
  }
}

export function projectPath(projectId: string, suffix: string): string {
  const s = suffix.startsWith('/') ? suffix : `/${suffix}`
  return `/v1/projects/${encodeURIComponent(projectId)}${s}`
}
