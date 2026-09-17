/** 统一的后端接口类型定义与 API 抽象（契约对照 plan/planv2.0/proto-http.md） */

export interface MetricsSummary {
  totalRequests: number
  errorRate: number
  avgLatencyMs: number
  activeDatabases: number
}

export interface TrendPoint {
  date: string
  requests: number
  errors: number
}

export interface DbRow {
  id: string
  [key: string]: unknown
}

export interface S3Object {
  key: string
  size: number
  /** 注意：S3 域是后端 snake_case 命名的例外（proto-http.md §3.4） */
  lastModified?: string
}

export interface FaasFunction {
  name: string
  version: string
  runtime?: string
  updatedAt?: string
}

export interface LogHandlers {
  onOpen?: () => void
  onMessage?: (line: string) => void
  onClose?: () => void
  onError?: (e: unknown) => void
}

export interface LogConnection {
  close: () => void
}

/* ---------- Projects ---------- */

export interface ProjectItem {
  id: string
  name: string
  createdAt: string
}

/* ---------- Databases（proto-http.md §3.1） ---------- */

/** 数据库资源。status 枚举共 9 值，新建时为 creating（不是 active） */
export interface DatabaseItem {
  id: string
  name: string
  status:
    | 'creating'
    | 'opening'
    | 'ready'
    | 'closing'
    | 'closed'
    | 'degraded'
    | 'deleting'
    | 'deleted'
    | 'recovering'
  createdAt: string
  updatedAt: string
  /** 仅详情接口可能返回（服务端注入 SnapshotFor 时） */
  snapshot?: { lastSyncedSnapshot: number; syncLag: number }
}

export interface DatabaseListResult {
  databases: DatabaseItem[]
  nextCursor: string
}

/* ---------- SQL（proto-http.md §3.2） ---------- */

/** query/execute 单语句请求 */
export interface SqlRequest {
  sql: string
  args?: unknown[]
  /** 仅 query 有效，缺省 1000 */
  maxRows?: number
}

export interface SqlBatchRequest {
  statements: SqlRequest[]
  transactional: boolean
}

/** query 结果。rows 是二维数组，列名需与 columns 按下标 zip */
export interface SqlQueryResult {
  columns: string[]
  rows: unknown[][]
  rowCount: number
  durationMs: number
  requestId: string
}

export interface SqlExecuteResult {
  rowsAffected: number
  durability: string
  durationMs: number
  requestId: string
}

export interface SqlBatchResultItem {
  index: number
  rowsAffected?: number
  durationMs?: number
  errorCode?: string
  errorMessage?: string
}

export interface SqlBatchResult {
  results: SqlBatchResultItem[]
  durability: string
  durationMs: number
  requestId: string
  error?: { failedIndex: number; code: string; message: string }
}

/* ---------- Quota（proto-http.md §3.6） ---------- */

export interface QuotaStatus {
  llmAllowed: boolean
  databaseAllowed: boolean
}

/* ---------- LLM Gateway（proto-http.md §3.5） ---------- */

export interface LlmMessage {
  role: 'system' | 'user' | 'assistant' | string
  content: string
}

export interface LlmChatRequest {
  model?: string
  messages: LlmMessage[]
  maxTokens?: number
  temperature?: number
}

export interface LlmTokenUsage {
  prompt_tokens?: number
  completion_tokens?: number
  total_tokens?: number
}

export interface LlmChatResponse {
  content: string
  usage?: LlmTokenUsage
  model: string
  provider: string
  finish_reason?: string
}

export interface LlmStreamHandlers {
  onChunk?: (text: string) => void
  onEnd?: () => void
  onError?: (e: unknown) => void
}

export interface LlmStreamConnection {
  close: () => void
}

/**
 * API 统一抽象：http 实现与 mock 实现均遵循该接口。
 * projectId 一律为方法首个参数，由调用方从 stores/project.ts 读取后显式传入
 * （services 层不 import store，保持无状态可测）。
 */
export interface Api {
  projects: {
    list: () => Promise<ProjectItem[]>
    create: (req: { name: string; id?: string }) => Promise<ProjectItem>
  }
  metrics: {
    summary: () => Promise<MetricsSummary>
    trend: () => Promise<TrendPoint[]>
  }
  databases: {
    list: (projectId: string) => Promise<DatabaseItem[]>
    create: (projectId: string, name: string) => Promise<DatabaseItem>
    get: (projectId: string, databaseId: string) => Promise<DatabaseItem>
    open: (projectId: string, databaseId: string) => Promise<DatabaseItem>
    close: (projectId: string, databaseId: string) => Promise<void>
    remove: (projectId: string, databaseId: string) => Promise<void>
  }
  sql: {
    query: (projectId: string, databaseId: string, req: SqlRequest) => Promise<SqlQueryResult>
    execute: (projectId: string, databaseId: string, req: SqlRequest) => Promise<SqlExecuteResult>
    batch: (projectId: string, databaseId: string, req: SqlBatchRequest) => Promise<SqlBatchResult>
  }
  db: {
    collections: (projectId: string, databaseId: string) => Promise<string[]>
    createCollection: (projectId: string, databaseId: string, name: string) => Promise<void>
    rows: (
      projectId: string,
      databaseId: string,
      collection: string,
      query?: Record<string, unknown>
    ) => Promise<DbRow[]>
    insert: (
      projectId: string,
      databaseId: string,
      collection: string,
      payload: Record<string, unknown>
    ) => Promise<DbRow>
    update: (
      projectId: string,
      databaseId: string,
      collection: string,
      id: string,
      payload: Record<string, unknown>
    ) => Promise<DbRow>
    remove: (projectId: string, databaseId: string, collection: string, id: string) => Promise<void>
  }
  s3: {
    list: (projectId: string, prefix?: string) => Promise<S3Object[]>
    presign: (projectId: string, key: string) => Promise<{ url: string }>
    remove: (projectId: string, key: string) => Promise<void>
    upload: (projectId: string, key: string, file: File) => Promise<S3Object>
  }
  faas: {
    list: () => Promise<FaasFunction[]>
    deploy: (name: string, file: File) => Promise<FaasFunction>
    invoke: (name: string, payload: unknown) => Promise<unknown>
  }
  logs: {
    connect: (handlers: LogHandlers) => LogConnection
  }
  quota: {
    status: (projectId: string) => Promise<QuotaStatus>
  }
  llm: {
    providers: (projectId: string) => Promise<string[]>
    chat: (projectId: string, req: LlmChatRequest) => Promise<LlmChatResponse>
    stream: (projectId: string, req: LlmChatRequest, handlers: LlmStreamHandlers) => LlmStreamConnection
  }
}
