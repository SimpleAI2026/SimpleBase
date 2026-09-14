/** 统一的后端接口类型定义与 API 抽象 */

export interface MetricsSummary {
  totalRequests: number
  errorRate: number
  avgLatencyMs: number
  activeFunctions: number
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

/* ---------- LLM Gateway ---------- */

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

/** API 统一抽象：http 实现与 mock 实现均遵循该接口 */
export interface Api {
  metrics: {
    summary: () => Promise<MetricsSummary>
    trend: () => Promise<TrendPoint[]>
  }
  db: {
    collections: () => Promise<string[]>
    createCollection: (name: string) => Promise<void>
    rows: (collection: string, query?: Record<string, unknown>) => Promise<DbRow[]>
    insert: (collection: string, payload: Record<string, unknown>) => Promise<DbRow>
    update: (collection: string, id: string, payload: Record<string, unknown>) => Promise<DbRow>
    remove: (collection: string, id: string) => Promise<void>
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
  llm: {
    providers: (projectId: string) => Promise<string[]>
    chat: (projectId: string, req: LlmChatRequest) => Promise<LlmChatResponse>
    stream: (projectId: string, req: LlmChatRequest, handlers: LlmStreamHandlers) => LlmStreamConnection
  }
}
