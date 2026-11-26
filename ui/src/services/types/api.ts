// `Api` 接口：控制台所有后端调用的唯一契约（实现见 http-api.ts）。

import type {
  APIPreviewResult,
  ApiKeyItem,
  ApiKeyScope,
  AuthUser,
  BackupItem,
  CreateUserRequest,
  CronJobCreate,
  CronJobItem,
  CronJobRunItem,
  GoFuncTestResult,
  GoFuncVersionCreate,
  GoFuncVersionSummary,
  GoFunctionCreate,
  GoFunctionItem,
  InitialPasswordRequest,
  LLMTrace,
  LLMTraceGroupListResult,
  LlmSettings,
  LlmTraceGroupQuery,
  LogEvent,
  LogQuery,
  LogRetention,
  LogStat,
  LoginRequest,
  MetricsSummary,
  OpenAPIDocument,
  ProjectItem,
  QuotaStatus,
  S3Link,
  S3ListResult,
  S3Object,
  S3PublicResult,
  S3SiteInfo,
  SandboxCapabilities,
  SandboxCreate,
  SandboxFileEntry,
  SandboxItem,
  SandboxTerminalTicket,
  StorageSettings,
  StorageUpdateResult,
  TokenPair,
  TrendPoint,
  UpdateUserRequest,
  UserItem,
  UserListResult
} from '@/services/types/platform'
import type {
  AggregateQuery,
  AggregateResult,
  CollectionAccessRule,
  CollectionAccessRuleInput,
  CollectionImportResult,
  CollectionInfo,
  CollectionSchema,
  CollectionSchemaInput,
  CreateDatabaseInput,
  DatabaseItem,
  DatabaseSchema,
  DbRow,
  DocumentItem,
  DocumentPage,
  DocumentQuery,
  FunctionTrigger,
  FunctionTriggerInput,
  KvExecBody,
  SchemaColumn,
  SchemaDryRunResult,
  SchemaTable,
  SchemaTableRows,
  SqlBatchRequest,
  SqlBatchResult,
  SqlExecuteResult,
  SqlQueryResult,
  SqlRequest,
  ViewDryRunResult
} from '@/services/types/data'
import type {
  ActiveAgentRun,
  AgentApprovalList,
  AgentGroupMembersInput,
  AgentHumanReplyInput,
  AgentHumanTaskList,
  AgentMessage,
  AgentModelsResponse,
  AgentModuleInfo,
  AgentPlanConfirmationRequest,
  AgentRun,
  AgentRunRequest,
  AgentStreamHandlers,
  AgentTag,
  AgentTagInput,
  AgentThread,
  AgentThreadCreateOptions,
  AgentThreadPage,
  AgentThreadPageQuery,
  AgentTurn,
  AgentTurnActionRequest,
  AgentTurnRequest,
  AgentWorkspaceCapabilities,
  CloudAgent,
  LlmStreamConnection,
  McpServer,
  McpServerBody,
  McpServerList,
  McpTestResult
} from '@/services/types/agent'

/**
 * API 统一抽象：http 实现与 mock 实现均遵循该接口。
 * projectId 一律为方法首个参数，由调用方从 stores/project.ts 读取后显式传入
 * （services 层不 import store，保持无状态可测）。
 */
export interface Api {
  preview: {
    document: () => Promise<OpenAPIDocument>
    request: (method: string, path: string, body?: unknown) => Promise<APIPreviewResult>
  }
  auth: {
    login: (req: LoginRequest) => Promise<TokenPair>
    logout: (refreshToken?: string) => Promise<void>
    me: () => Promise<AuthUser & { projects: { id: string; name?: string; owner: boolean }[] }>
    changePassword: (oldPassword: string, newPassword: string) => Promise<void>
    /** 未登录的强制改密：验旧密 → 改密 → 直接返回正式 TokenPair。 */
    initialPassword: (req: InitialPasswordRequest) => Promise<TokenPair>
  }
  users: {
    list: (limit?: number, cursor?: string) => Promise<UserListResult>
    create: (req: CreateUserRequest) => Promise<UserItem>
    update: (id: string, req: UpdateUserRequest) => Promise<UserItem>
  }
  triggers: {
    list: (projectId: string) => Promise<FunctionTrigger[]>
    create: (projectId: string, input: FunctionTriggerInput) => Promise<FunctionTrigger>
    patch: (projectId: string, id: string, input: Partial<FunctionTriggerInput>) => Promise<FunctionTrigger>
    remove: (projectId: string, id: string) => Promise<void>
    invoke: (projectId: string, id: string, body: Record<string, unknown>) => Promise<void>
  }
  gofunctions: {
    list: (projectId: string) => Promise<GoFunctionItem[]>
    create: (projectId: string, body: GoFunctionCreate) => Promise<GoFunctionItem>
    get: (projectId: string, name: string) => Promise<GoFunctionItem>
    /** 保存为新版本（可选设为生效） */
    saveVersion: (projectId: string, name: string, body: GoFuncVersionCreate) => Promise<GoFunctionItem>
    remove: (projectId: string, name: string) => Promise<void>
    listVersions: (
      projectId: string,
      name: string
    ) => Promise<{ activeVersion: number; versions: GoFuncVersionSummary[] }>
    activate: (projectId: string, name: string, version: number) => Promise<{ activeVersion: number }>
    /** 调试台试跑指定版本 */
    test: (
      projectId: string,
      name: string,
      version: number,
      functionName: string,
      body: unknown
    ) => Promise<GoFuncTestResult>
  }
  sandboxes: {
    capabilities: (projectId: string) => Promise<SandboxCapabilities>
    list: (projectId: string, state?: string, metadataFilter?: Record<string, string>) => Promise<SandboxItem[]>
    get: (projectId: string, id: string) => Promise<SandboxItem>
    create: (projectId: string, body: SandboxCreate) => Promise<SandboxItem>
    remove: (projectId: string, id: string) => Promise<void>
    connect: (projectId: string, id: string, timeoutS?: number) => Promise<SandboxItem>
    pause: (projectId: string, id: string) => Promise<void>
    terminalTicket: (projectId: string, id: string) => Promise<SandboxTerminalTicket>
    terminalWebSocketUrl: (id: string, ticket: string) => string
    files: {
      list: (projectId: string, id: string, path: string, depth?: number) => Promise<SandboxFileEntry[]>
      read: (projectId: string, id: string, path: string) => Promise<Blob>
      write: (projectId: string, id: string, path: string, content: Blob | string) => Promise<void>
      remove: (projectId: string, id: string, path: string) => Promise<void>
    }
  }
  cronjobs: {
    list: (projectId: string) => Promise<CronJobItem[]>
    create: (projectId: string, body: CronJobCreate) => Promise<CronJobItem>
    /** PATCH：name 不可改；调度变更后服务端重算 nextRunAt */
    update: (projectId: string, jobId: string, body: Partial<CronJobCreate>) => Promise<CronJobItem>
    remove: (projectId: string, jobId: string) => Promise<void>
    runs: (projectId: string, jobId: string, limit?: number) => Promise<CronJobRunItem[]>
    /** 手动立即执行（异步 202；不改排期） */
    trigger: (projectId: string, jobId: string) => Promise<void>
  }
  projects: {
    list: () => Promise<ProjectItem[]>
    create: (req: { name: string; id?: string; engine?: 'duckdb' | 'sqlite'; ownerUserId?: string }) => Promise<ProjectItem>
  }
  /** 项目 API Key 管理（创建/列表/吊销；明文仅创建响应一次性返回） */
  apiKeys: {
    list: (projectId: string) => Promise<ApiKeyItem[]>
    create: (projectId: string, permissions?: string[], scope?: ApiKeyScope) => Promise<ApiKeyItem & { secret: string }>
    revoke: (projectId: string, keyId: string) => Promise<void>
  }
  metrics: {
    summary: (projectId: string) => Promise<MetricsSummary>
    trend: (projectId: string) => Promise<TrendPoint[]>
  }
  databases: {
    list: (projectId: string) => Promise<DatabaseItem[]>
    create: (projectId: string, input: CreateDatabaseInput) => Promise<DatabaseItem>
    remove: (projectId: string, databaseId: string) => Promise<void>
    schema: (projectId: string, databaseId: string) => Promise<DatabaseSchema>
    tableRows: (
      projectId: string,
      databaseId: string,
      table: string,
      /** filter 可选：与聚合接口同一套语法；数据看板的明细表据此跟随全局条件。 */
      query: { limit: number; offset: number; filter?: string }
    ) => Promise<SchemaTableRows>
    /** 数据看板：SQL 表聚合（GET .../schema/tables/:table/aggregate），只读；与集合聚合同一套参数与响应。 */
    aggregateTable: (projectId: string, databaseId: string, table: string, query: AggregateQuery) => Promise<AggregateResult>
    createTable: (
      projectId: string,
      databaseId: string,
      table: SchemaTable
    ) => Promise<SchemaTable>
    addColumn: (
      projectId: string,
      databaseId: string,
      column: { table: string; name: string; type: string; nullable?: boolean }
    ) => Promise<SchemaColumn>
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
    collectionItems: (projectId: string, databaseId: string) => Promise<CollectionInfo[]>
    exportCollection: (projectId: string, databaseId: string, collection: string, format: 'csv' | 'ndjson' | 'json', query?: DocumentQuery) => Promise<Blob>
    importCollection: (projectId: string, databaseId: string, collection: string, file: File, format: 'csv' | 'ndjson', dryRun?: boolean) => Promise<CollectionImportResult>
    dryRunView: (projectId: string, databaseId: string, name: string, query: string) => Promise<ViewDryRunResult>
    documents: (
      projectId: string,
      databaseId: string,
      collection: string,
      query?: DocumentQuery
    ) => Promise<DocumentPage>
    /** 数据看板聚合：GET .../aggregate，只读。 */
    aggregate: (projectId: string, databaseId: string, collection: string, query: AggregateQuery) => Promise<AggregateResult>
    document: (projectId: string, databaseId: string, collection: string, id: string) => Promise<DocumentItem>
    patch: (
      projectId: string,
      databaseId: string,
      collection: string,
      id: string,
      payload: Record<string, unknown>
    ) => Promise<DbRow>
    batch: (
      projectId: string,
      databaseId: string,
      collection: string,
      documents: Record<string, unknown>[]
    ) => Promise<DocumentItem[]>
    truncate: (projectId: string, databaseId: string, collection: string) => Promise<void>
    getCollectionSchema: (projectId: string, databaseId: string, collection: string) => Promise<CollectionSchema>
    putCollectionSchema: (
      projectId: string,
      databaseId: string,
      schema: CollectionSchemaInput,
      dryRun?: boolean
    ) => Promise<CollectionSchema | SchemaDryRunResult>
    deleteCollectionSchema: (projectId: string, databaseId: string, collection: string) => Promise<void>
    getCollectionAccess: (
      projectId: string,
      databaseId: string,
      collection: string
    ) => Promise<CollectionAccessRule>
    putCollectionAccess: (
      projectId: string,
      databaseId: string,
      collection: string,
      rule: CollectionAccessRuleInput
    ) => Promise<CollectionAccessRule>
    deleteCollectionAccess: (
      projectId: string,
      databaseId: string,
      collection: string
    ) => Promise<CollectionAccessRule>
  }
  /** Key-Value 数据服务：项目级单端点 */
  kv: {
    /** 单端点执行：body 为 {type:"cmd",argvs:[...]} 或 {type:"String|Hash|List|Set|ZSet",args:{...}}，
     *  返回该命令的 Redis 回复编码成 JSON（GET 缺失为 null）。 */
    exec: (projectId: string, body: KvExecBody) => Promise<unknown>
    /** 批量顺序执行多条命令（事务外逐条）；任一失败即停止，返回已完成的回复。 */
    execBatch: (projectId: string, bodies: KvExecBody[]) => Promise<unknown[]>
  }
  s3: {
    list: (projectId: string, prefix?: string) => Promise<S3Object[]>
    listDetailed: (projectId: string, prefix?: string) => Promise<S3ListResult>
    presign: (projectId: string, key: string) => Promise<{ url: string }>
    download: (projectId: string, key: string) => Promise<Blob>
    createLink: (projectId: string, key: string, ttlSeconds?: number) => Promise<S3Link>
    remove: (projectId: string, key: string) => Promise<void>
    upload: (
      projectId: string,
      key: string,
      file: File,
      onProgress?: (percent: number) => void,
      signal?: AbortSignal
    ) => Promise<S3Object>
    /** 站点（发布网站）状态 */
    getSite: (projectId: string) => Promise<S3SiteInfo>
    /** 设置文件 / 文件夹外部访问；folder 的 key 以 "/" 结尾 */
    setPublic: (projectId: string, key: string, kind: 'file' | 'folder', isPublic: boolean) => Promise<S3PublicResult>
  }
  backups: {
    list: (projectId: string) => Promise<BackupItem[]>
    create: (projectId: string) => Promise<BackupItem>
    remove: (projectId: string, id: string) => Promise<void>
    restore: (projectId: string, id: string) => Promise<void>
    download: (projectId: string, id: string) => Promise<Blob>
  }
  logs: {
    list: (projectId: string, q?: LogQuery) => Promise<LogEvent[]>
    stats: (projectId: string, q?: LogQuery, groupBy?: 'level' | 'hour' | 'path') => Promise<LogStat[]>
    export: (projectId: string, q?: LogQuery) => Promise<Blob>
    getRetention: (projectId: string) => Promise<LogRetention>
    putRetention: (projectId: string, keepDays: number) => Promise<void>
  }
  quota: {
    status: (projectId: string) => Promise<QuotaStatus>
  }
  /** LLM 对话走 agentThreads.streamRun；定时摘要走 cronjobs.list。 */
  llmSettings: {
    get: (projectId: string) => Promise<LlmSettings>
    /** apiKey 留空表示沿用服务端既有值 */
    put: (projectId: string, settings: LlmSettings, apiKey?: string) => Promise<LlmSettings>
  }
  llmTraces: {
    /** 模型调用 trace 查询（默认窗口 24h） */
    /** 按 trace_id 聚合的顶层调用列表 */
    groups: (projectId: string, q?: LlmTraceGroupQuery) => Promise<LLMTraceGroupListResult>
    /** 一个 trace 的全部 span（时间升序） */
    detail: (projectId: string, traceId: string) => Promise<LLMTrace[]>
  }
  agents: {
    modules: (projectId: string) => Promise<AgentModuleInfo[]>
    list: (projectId: string) => Promise<CloudAgent[]>
    create: (projectId: string, body: Partial<CloudAgent>) => Promise<CloudAgent>
    patch: (projectId: string, agentId: string, body: Partial<CloudAgent>) => Promise<CloudAgent>
    remove: (projectId: string, agentId: string) => Promise<void>
    /** 可用模型列表。 */
    models: (projectId: string) => Promise<AgentModelsResponse>
  }
  agentThreads: {
    capabilities: (projectId: string) => Promise<AgentWorkspaceCapabilities>
    list: (projectId: string, agentId?: string) => Promise<AgentThread[]>
    /** 分页（可按助手或标签过滤）。 */
    pageBy: (projectId: string, q?: AgentThreadPageQuery) => Promise<AgentThreadPage>
    create: (projectId: string, opts?: AgentThreadCreateOptions) => Promise<AgentThread>
    /** 单个会话详情（深链补齐）。 */
    get: (projectId: string, threadId: string) => Promise<AgentThread>
    rename: (projectId: string, threadId: string, title: string) => Promise<AgentThread>
    remove: (projectId: string, threadId: string) => Promise<void>
    runs: (projectId: string, threadId: string, limit?: number) => Promise<AgentRun[]>
    turns: (projectId: string, threadId: string, limit?: number) => Promise<AgentTurn[]>
    messages: (projectId: string, threadId: string) => Promise<AgentMessage[]>
    streamRun: (
      projectId: string,
      threadId: string,
      req: AgentRunRequest,
      handlers: AgentStreamHandlers
    ) => LlmStreamConnection
    /** v2：协作轮次（多目标 @）。 */
    streamTurn: (
      projectId: string,
      threadId: string,
      req: AgentTurnRequest,
      handlers: AgentStreamHandlers
    ) => LlmStreamConnection
    /** 按轮次 last_event_seq 续接 v3 事件。 */
    attachTurn: (
      projectId: string,
      turnId: string,
      after: number,
      handlers: AgentStreamHandlers
    ) => LlmStreamConnection
    /** 续接进行中的 run（回放事件缓存并跟随到 end）。 */
    attachRun: (projectId: string, runId: string, handlers: AgentStreamHandlers) => LlmStreamConnection
    cancel: (projectId: string, runId: string) => Promise<AgentRun>
    /** 项目内正在执行的 run。 */
    active: (projectId: string) => Promise<ActiveAgentRun[]>
    confirm: (projectId: string, runId: string, callId: string, approve: boolean) => Promise<void>
  }
  /** 工作台标签（群）CRUD。 */
  agentTags: {
    list: (projectId: string) => Promise<AgentTag[]>
    create: (projectId: string, input: AgentTagInput) => Promise<AgentTag>
    patch: (projectId: string, tagId: string, input: AgentTagInput) => Promise<AgentTag>
    remove: (projectId: string, tagId: string) => Promise<void>
    /** 加入已有成员，或「新建助手并加入」。 */
    addMembers: (projectId: string, tagId: string, body: AgentGroupMembersInput) => Promise<AgentTag>
  }
  /** 协作轮次与人工待办。 */
  agentTurns: {
    get: (projectId: string, turnId: string) => Promise<AgentTurn>
    act: (projectId: string, turnId: string, body: AgentTurnActionRequest) => Promise<AgentTurn>
    reply: (projectId: string, taskId: string, body: AgentHumanReplyInput) => Promise<AgentTurn>
    confirmPlan: (projectId: string, turnId: string, body: AgentPlanConfirmationRequest) => Promise<AgentTurn>
    humanTasks: (projectId: string, assignee?: string, status?: string) => Promise<AgentHumanTaskList>
  }
  /** 审批队列：跨 run 的破坏性工具调用待办。 */
  agentApprovals: {
    list: (projectId: string, status?: string, limit?: number) => Promise<AgentApprovalList>
    /** 复用 run 的确认端点，放行/拒绝后队列同步收敛。 */
    resolve: (projectId: string, runId: string, callId: string, approve: boolean) => Promise<void>
  }
  /** 项目级 MCP Server 配置。 */
  mcpServers: {
    list: (projectId: string) => Promise<McpServerList>
    create: (projectId: string, body: McpServerBody) => Promise<McpServer>
    update: (projectId: string, serverId: string, body: McpServerBody) => Promise<McpServer>
    remove: (projectId: string, serverId: string) => Promise<void>
    /** 连通性测试：只回工具名，不回凭据。 */
    test: (projectId: string, serverId: string) => Promise<McpTestResult>
    /** 内置 MCP 的项目级开关（PUT /mcp/builtin/:name/state）。 */
    setBuiltinEnabled: (projectId: string, name: string, enabled: boolean) => Promise<void>
    /** 内置 MCP 进程内连通性测试，不依赖 agent.mcp.enabled。 */
    testBuiltin: (projectId: string, name: string) => Promise<McpTestResult>
  }
  /** 系统级存储设置。 */
  storage: {
    get: () => Promise<StorageSettings>
    /** confirmReset 必须为 true：服务端要求显式确认（修改后需 reset 清数据）。 */
    put: (body: { database?: string; files?: string; data_dir?: string; confirm_reset: boolean }) => Promise<StorageUpdateResult>
  }
}
