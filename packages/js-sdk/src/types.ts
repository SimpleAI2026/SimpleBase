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
