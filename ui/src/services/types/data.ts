// 数据域契约：数据库 / SQL / Schema / 集合 / 文档 / 聚合 / KV / 触发器。

export interface DbRow {
  id: string
  [key: string]: unknown
}

export interface CollectionInfo {
  name: string
  schemaMode: 'json' | 'view'
  fieldCount: number
  documentCount: number
  registered: boolean
}

export interface CollectionFieldOptions {
  min?: number | string
  max?: number | string
  pattern?: string
  values?: string[]
  maxSelect?: number
  maxSize?: number
  onlyInt?: boolean
  exceptDomains?: string[]
  onlyDomains?: string[]
}

export interface CollectionField {
  name: string
  type: 'text' | 'number' | 'bool' | 'date' | 'email' | 'url' | 'select' | 'json'
  required: boolean
  unique: boolean
  default?: unknown
  options: CollectionFieldOptions
  hidden: boolean
  readonly: boolean
}

export interface CollectionSchema {
  id?: string
  projectId?: string
  name: string
  schemaMode: 'json' | 'view'
  viewQuery?: string
  fields: CollectionField[]
}

export interface CollectionSchemaInput {
  name: string
  schemaMode?: 'json' | 'view'
  viewQuery?: string
  fields: CollectionField[]
}

export type CollectionAccessMode = 'off' | 'owner_read' | 'owner_write'

export interface CollectionAccessRule {
  mode: CollectionAccessMode
  ownerField: string
  enforced?: boolean
}

export interface CollectionAccessRuleInput {
  mode: CollectionAccessMode
  ownerField: string
}

export interface ViewDryRunResult {
  columns: string[]
  fields: CollectionField[]
  items: DocumentItem[]
  scanned: number
}

export interface CollectionImportResult {
  dryRun: boolean
  scanned: number
  imported: number
}

export interface FunctionTrigger {
  id: string
  projectId: string
  kind: 'record_change' | 'object_event' | 'http'
  target: string
  events: string[]
  funcFile: string
  funcExport: string
  filter?: string
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export interface FunctionTriggerInput {
  kind: FunctionTrigger['kind']
  target: string
  events: string[]
  funcFile: string
  funcExport: string
  filter?: string
  enabled: boolean
}

export interface DocumentItem {
  id: string
  createdAt: string
  updatedAt: string
  data: Record<string, unknown>
}

export interface DocumentQuery {
  limit?: number
  cursor?: string
  offset?: number
  sort?: string
  filter?: string
  fields?: string
  count?: boolean
}

/**
 * 数据看板聚合请求。聚合模式用 metrics / groupBy / series；
 * 散点原始点模式用 mode='points' + x / y / size / series，两种模式互斥。
 */
export interface AggregateQuery {
  /** 聚合度量，如 ['count', 'sum:amount']；points 模式不填。 */
  metrics?: string[]
  /** 主维度：field / field:day（时间桶）/ field:bin=10（数值分箱）。 */
  groupBy?: string
  series?: string
  mode?: 'points'
  x?: string
  y?: string
  size?: string
  filter?: string
  /** 度量别名或 x / series，`-` 前缀降序。 */
  sort?: string
  limit?: number
  /**
   * 额外返回同一 filter 下的同口径总量（不分组的可累加度量）。
   * 仅供占比类图表算「其他」与真实分母；后端只在分组 + 单度量 count/sum 时提供。
   */
  withTotal?: boolean
}

/** 聚合响应：columns 与 rows 按下标对齐；主维度列固定叫 x，次维度列固定叫 series。 */
export interface AggregateResult {
  columns: string[]
  rows: unknown[][]
  /** 同口径总量（仅请求 withTotal 且后端可累加时存在）；用于占比的真实分母。 */
  total?: number
  truncated: boolean
  scanned: number
  durationMs: number
  requestId: string
}

export interface DocumentPage {
  rows: DbRow[]
  items: DocumentItem[]
  nextCursor?: string
  total?: number
}

export interface SchemaDryRunResult {
  invalid: { id: string; field: string; reason: string }[]
  scanned: number
  truncated: boolean
}

export type KvType = 'string' | 'list' | 'set' | 'hash' | 'zset'

/** SCAN 回复里的一条 key 元信息（cmd SCAN 展开用） */
export interface KvKeyMeta {
  key: string
  type: KvType
  /** 元素计数；string 类型为 null */
  len: number | null
  /** 剩余 TTL 毫秒；null = 永久 */
  ttl_ms: number | null
  mtime_ms: number
  version: number
}

/** cmd 请求体：argvs[0] 是命令名，其余为参数（全部字符串，数字由服务端解析） */
export interface KvCmdBody {
  type: 'cmd'
  argvs: string[]
}

/** 按类型写入一份数据的 args（key 必填；其余字段按类型取用） */
export interface KvTypedArgs {
  key: string
  /** String：文本值 */
  value?: string
  /** Hash：字段表（至少一个） */
  fields?: Record<string, string>
  /** List / Set：元素数组（非空） */
  elems?: string[]
  /** List：back（默认）| front */
  side?: 'back' | 'front'
  /** ZSet：成员与分数（非空） */
  items?: { elem: string; score: number }[]
  /** 可选：写成功后按毫秒设置过期；0 = 立即过期 */
  ttl_ms?: number
  /** String：仅不存在时写入 */
  nx?: boolean
  /** String：仅已存在时写入 */
  xx?: boolean
  /** String：保留已有 TTL */
  keep_ttl?: boolean
}

export interface KvTypedBody {
  type: 'String' | 'Hash' | 'List' | 'Set' | 'ZSet'
  args: KvTypedArgs
}

/** POST /v1/projects/:projectId/kv 的请求体 */
export type KvExecBody = KvCmdBody | KvTypedBody

/** 数据库资源。成功创建对 UI 是 ready。creating 只在创建过程中短暂出现。 */
export interface DatabaseItem {
  id: string
  name: string
  status: 'creating' | 'ready' | 'degraded' | 'deleting' | 'deleted'
  createdAt: string
  updatedAt: string
  /** 仅详情接口可能返回（服务端注入 SnapshotFor 时） */
  snapshot?: { lastSyncedSnapshot: number; syncLag: number }
  /** 库内用户表总行数；未知/未就绪时省略 */
  documentCount?: number
  /** 数据形态。缺省按集合文档处理，与存量库迁移默认一致 */
  dataModel?: 'collection' | 'sql'
}

/** 创建数据库。省略 dataModel 时服务端按集合文档创建。 */
export interface CreateDatabaseInput {
  name: string
  dataModel?: 'collection' | 'sql'
  initSql?: string
}

/** 表结构列。nullable 省略视为可空。sensitive 仅系统库凭证列。 */
export interface SchemaColumn {
  name: string
  type: string
  nullable?: boolean
  sensitive?: boolean
}

/** 一张表的分页行。rows 与 columns 按下标对齐。 */
export interface SchemaTableRows {
  table: string
  columns: SchemaColumn[]
  rows: unknown[][]
  limit: number
  offset: number
  total: number
}

export interface SchemaTable {
  name: string
  columns: SchemaColumn[]
}

export interface DatabaseSchema {
  tables: SchemaTable[]
}

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

/**
 * 数据看板图表类型。
 * column 纵向柱状；bar / grouped_bar / stacked_bar 横向；auto 由 bi-auto 推断。
 */
export type BiChartType =
  | 'auto'
  | 'line'
  | 'column'
  | 'bar'
  | 'pie'
  | 'table'
  | 'grouped_bar'
  | 'stacked_bar'
  | 'scatter'

/** 栅格宽度档位（md 以上 12 栏）。 */
export type BiSpan = 4 | 6 | 8 | 12

export interface BiWidget {
  id: string
  title: string
  type: BiChartType
  /** 聚合度量，如 ['count', 'sum:amount']；散点为空。 */
  metrics: string[]
  /** 主维度：field / field:day / field:bin=10。 */
  groupBy?: string
  series?: string
  /** 仅散点。 */
  x?: string
  y?: string
  size?: string
  filter?: string
  sort?: string
  limit?: number
  /** 折线面积 / 饼图环形。 */
  area?: boolean
  donut?: boolean
  span: BiSpan
}

/** 看板配置（落 localStorage，按「项目 + 库 + 集合」分键）。 */
export interface BiDashboardConfig {
  version: 1
  widgets: BiWidget[]
}
