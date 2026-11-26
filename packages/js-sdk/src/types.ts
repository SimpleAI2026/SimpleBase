export type Json =
  | string
  | number
  | boolean
  | null
  | { [key: string]: Json | undefined }
  | Json[]

export interface CreateClientOptions {
  /** API origin, e.g. http://127.0.0.1:8080 */
  url: string
  /** Bearer API key */
  apiKey: string
  /** Project UUID */
  projectId: string
  /** Default database id for sql/collection helpers */
  databaseId?: string
  /** Inject custom fetch (tests / polyfill) */
  fetch?: typeof fetch
  /** Extra headers on every request */
  headers?: Record<string, string>
}

export interface DatabaseInfo {
  id: string
  name?: string
  status?: string
  kind?: string
  /** collection（默认，含存量库）或 sql */
  data_model?: 'collection' | 'sql'
  [key: string]: unknown
}

/** SQL query response — rows are positional arrays matching columns. */
export interface QueryResult {
  columns: string[]
  rows: unknown[][]
  row_count: number
  duration_ms?: number
  request_id?: string
  [key: string]: unknown
}

export interface ExecuteResult {
  rows_affected: number
  last_insert_id?: number | null
  durability?: string
  duration_ms?: number
  request_id?: string
  [key: string]: unknown
}

export interface BatchResultItem {
  index: number
  rows_affected?: number
  last_insert_id?: number | null
  duration_ms?: number
  error_code?: string
  error_message?: string
}

export interface BatchResult {
  results: BatchResultItem[]
  durability?: string
  duration_ms?: number
  request_id?: string
  error?: { failed_index: number; code: string; message: string }
  [key: string]: unknown
}

export interface BatchStatement {
  sql: string
  args?: unknown[]
}

export interface S3ObjectMeta {
  key: string
  size?: number
  lastModified?: string
  etag?: string
  content_type?: string
  [key: string]: unknown
}

/** 站点发布状态；`mode` 由服务端按 sites 配置返回（path | subdomain）。 */
export interface S3SiteInfo {
  baseUrl?: string
  publishable: boolean
  reason?: string
  mode?: 'path' | 'subdomain'
}

/** 一条公开路径。 */
export interface S3PublicPath {
  path: string
  kind: string
  publicUrl?: string
}

/** GET /s3/public 的响应。 */
export interface S3PublicState {
  site: S3SiteInfo
  paths: S3PublicPath[]
}

/** PUT /s3/public 的响应。 */
export interface S3PublicUpdateResult {
  public: boolean
  publicUrl?: string
  publicInherited?: string
}
