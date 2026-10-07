import { projectPath, type HttpClient } from './http.js'

export interface SandboxInfo {
  id: string
  project_id: string
  name: string
  cloud_name: string
  source: string
  image: string
  cpus: number
  memory_mib: number
  network: string
  idle_timeout_s: number
  max_duration_s: number
  status: string
  created_at: string
  started_at: string | null
  last_active_at: string | null
  expires_at: string | null
}
export interface SandboxCapabilities {
  available: boolean
  backend: string
  images: string[]
  default_image: string
  cpus_max: number
  memory_mib_max: number
  exec_timeout_max_s: number
  max_file_bytes: number
  max_output_bytes: number
  max_per_project: number
  network_options: string[]
}
export interface SandboxCreateInput {
  name?: string
  image?: string
  cpus?: number
  memory_mib?: number
  network?: 'none' | 'public'
  idle_timeout_s?: number
  start?: boolean
}
export interface SandboxUpdateInput { name?: string; idle_timeout_s?: number }
export interface SandboxExecInput {
  command?: string
  cmd?: string
  args?: string[]
  cwd?: string
  env?: Record<string, string>
  timeout_s?: number
}
export interface SandboxExecResult {
  exit_code: number
  stdout: string
  stderr: string
  stdout_truncated: boolean
  stderr_truncated: boolean
  timed_out: boolean
  duration_ms: number
  status: string
  sandbox_id?: string
}
export interface SandboxFileEntry { name: string; path: string; kind: string; size: number }
export interface SandboxFileContent { content: string; encoding: 'utf8' | 'base64'; truncated: boolean }
export interface SandboxRunInput extends SandboxExecInput {
  image?: string
  files?: Array<{ path: string; content: string }>
  keep?: boolean
}
export interface SandboxListOptions { status?: string; source?: string; limit?: number; cursor?: string }
/** One page of sandboxes, newest first. `next_cursor` is absent on the last page. */
export interface SandboxPage { sandboxes: SandboxInfo[]; next_cursor?: string }
export interface SandboxesApi {
  capabilities(): Promise<SandboxCapabilities>
  list(opts?: SandboxListOptions): Promise<SandboxInfo[]>
  listPage(opts?: SandboxListOptions): Promise<SandboxPage>
  create(input: SandboxCreateInput, idempotencyKey?: string): Promise<SandboxInfo>
  get(id: string, refresh?: boolean): Promise<SandboxInfo>
  update(id: string, input: SandboxUpdateInput): Promise<SandboxInfo>
  delete(id: string): Promise<void>
  start(id: string): Promise<SandboxInfo>
  stop(id: string): Promise<SandboxInfo>
  exec(id: string, input: SandboxExecInput): Promise<SandboxExecResult>
  run(input: SandboxRunInput): Promise<SandboxExecResult>
  files: {
    list(id: string, path?: string): Promise<SandboxFileEntry[]>
    read(id: string, path: string): Promise<SandboxFileContent>
    write(id: string, path: string, content: string): Promise<void>
    remove(id: string, path: string): Promise<void>
  }
}

export function createSandboxesApi(http: HttpClient): SandboxesApi {
  const base = projectPath(http.projectId, '/sandboxes')
  const resource = (id: string) => `${base}/${encodeURIComponent(id)}`
  const content = (id: string) => `${resource(id)}/files/content`
  const listPage = (opts: SandboxListOptions = {}) => http.request<SandboxPage>('GET', base, {
    query: { status: opts.status, source: opts.source, limit: opts.limit, cursor: opts.cursor }
  })
  return {
    capabilities: () => http.request('GET', `${base}/capabilities`),
    async list(opts = {}) {
      return (await listPage(opts)).sandboxes
    },
    listPage,
    create: (input, key) => http.request('POST', base, { body: input, headers: key ? { 'Idempotency-Key': key } : undefined }),
    get: (id, refresh = false) => http.request('GET', resource(id), { query: refresh ? { refresh: 1 } : undefined }),
    update: (id, input) => http.request('PATCH', resource(id), { body: input }),
    delete: (id) => http.request('DELETE', resource(id)),
    start: (id) => http.request('POST', `${resource(id)}/start`),
    stop: (id) => http.request('POST', `${resource(id)}/stop`),
    exec: (id, input) => http.request('POST', `${resource(id)}/exec`, { body: input }),
    run: (input) => http.request('POST', `${base}/run`, { body: input }),
    files: {
      async list(id, path = '/workspace') {
        const res = await http.request<{ entries: SandboxFileEntry[] }>('GET', `${resource(id)}/files`, { query: { path } })
        return res.entries
      },
      read: (id, path) => http.request('GET', content(id), { query: { path } }),
      write: (id, path, text) => http.request('PUT', content(id), { query: { path }, body: { content: text } }),
      remove: (id, path) => http.request('DELETE', content(id), { query: { path } })
    }
  }
}
