// 数据域转换：数据库 / SQL / Schema / 集合 / 文档 / 聚合。

import type {
  AggregateQuery,
  AggregateResult,
  CollectionAccessRule,
  CollectionInfo,
  CollectionSchema,
  DatabaseItem,
  DatabaseSchema,
  DocumentItem,
  DocumentPage,
  SchemaColumn,
  SchemaDryRunResult,
  SchemaTable,
  SchemaTableRows,
  SqlBatchResult,
  SqlExecuteResult,
  SqlQueryResult,
  SqlRequest
} from '@/services/types'
import { listOf, objectOf, orDefault, pick, type WireRecord } from '@/services/wire'

/** DatabaseItem：后端 snake_case → UI camelCase */
export function toDatabaseItem(d: WireRecord): DatabaseItem {
  const snapshot = d.snapshot ? objectOf(d.snapshot) : undefined
  return {
    id: pick(d, 'id'),
    name: pick(d, 'name'),
    status: pick(d, 'status'),
    createdAt: pick(d, 'created_at'),
    updatedAt: pick(d, 'updated_at'),
    snapshot: d.snapshot
      ? { lastSyncedSnapshot: pick(snapshot, 'last_synced_snapshot'), syncLag: pick(snapshot, 'sync_lag') }
      : undefined,
    documentCount: typeof d.document_count === 'number' ? d.document_count : undefined,
    dataModel: d.data_model === 'sql' ? 'sql' : 'collection'
  }
}

function schemaText(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

export function toSchemaColumn(raw: unknown): SchemaColumn {
  const column = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : undefined
  const mapped: SchemaColumn = {
    name: schemaText(column?.name),
    type: schemaText(column?.type),
    nullable: column?.nullable !== false
  }
  if (column?.sensitive === true) mapped.sensitive = true
  return mapped
}

export function toSchemaTableRows(raw: unknown): SchemaTableRows {
  const body = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : undefined
  const columns = listOf(body?.columns, toSchemaColumn)
  const rows: unknown[][] = []
  if (Array.isArray(body?.rows)) {
    for (const row of body.rows) {
      rows.push(Array.isArray(row) ? Array.from(row) : [])
    }
  }
  return {
    table: schemaText(body?.table),
    columns,
    rows,
    limit: typeof body?.limit === 'number' ? body.limit : 0,
    offset: typeof body?.offset === 'number' ? body.offset : 0,
    total: typeof body?.total === 'number' ? body.total : 0
  }
}

export function toSchemaTable(raw: unknown): SchemaTable {
  const table = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : undefined
  const columns = listOf(table?.columns, toSchemaColumn)
  return { name: schemaText(table?.name), columns }
}

export function toDatabaseSchema(raw: unknown): DatabaseSchema {
  const body = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : undefined
  const tables = listOf(body?.tables, toSchemaTable)
  return { tables }
}

/** SQL 请求：UI camelCase → 后端 snake_case */
export function toSqlPayload(req: SqlRequest) {
  return { sql: req.sql, args: req.args, max_rows: req.maxRows }
}

/** SQL query 响应：后端 → UI（rows 保持二维数组，由页面按 columns zip） */
export function toQueryResult(r: WireRecord): SqlQueryResult {
  return {
    columns: orDefault(r, 'columns', []),
    rows: orDefault(r, 'rows', []),
    rowCount: orDefault(r, 'row_count', 0),
    durationMs: orDefault(r, 'duration_ms', 0),
    requestId: orDefault(r, 'request_id', '')
  }
}

export function toExecuteResult(r: WireRecord): SqlExecuteResult {
  return {
    rowsAffected: orDefault(r, 'rows_affected', 0),
    durability: orDefault(r, 'durability', ''),
    durationMs: orDefault(r, 'duration_ms', 0),
    requestId: orDefault(r, 'request_id', '')
  }
}

export function toBatchResult(r: WireRecord): SqlBatchResult {
  const items = orDefault<WireRecord[]>(r, 'results', []).map((it) => ({
    index: orDefault(it, 'index', 0),
    rowsAffected: pick<number>(it, 'rows_affected'),
    durationMs: pick<number>(it, 'duration_ms'),
    errorCode: pick<string>(it, 'error_code'),
    errorMessage: pick<string>(it, 'error_message')
  }))
  const error = r.error ? objectOf(r.error) : undefined
  const err = r.error
    ? { failedIndex: pick<number>(error, 'failed_index'), code: pick<string>(error, 'code'), message: pick<string>(error, 'message') }
    : undefined
  return {
    results: items,
    durability: orDefault(r, 'durability', ''),
    durationMs: orDefault(r, 'duration_ms', 0),
    requestId: orDefault(r, 'request_id', ''),
    error: err
  }
}

export function collectionWire(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : {}
}

export function toCollectionInfo(value: unknown): CollectionInfo {
  const raw = collectionWire(value)
  return {
    name: String(raw.name ?? ''),
    schemaMode: raw.schema_mode === 'view' ? 'view' : 'json',
    fieldCount: Number(raw.field_count ?? 0),
    documentCount: Number(raw.document_count ?? 0),
    registered: raw.registered === true
  }
}

/** CollectionAccessRule：后端 `owner_field` → UI `ownerField`。 */
export function toCollectionAccessRule(value: unknown): CollectionAccessRule {
  const raw = objectOf(value) ?? {}
  const mode = raw.mode === 'owner_read' || raw.mode === 'owner_write' ? raw.mode : 'off'
  return {
    mode,
    ownerField: String(raw.owner_field ?? 'owner_id'),
    enforced: raw.enforced === true
  }
}

export function toCollectionSchema(value: unknown): CollectionSchema {
  const raw = collectionWire(value)
  const fields = listOf(raw.fields)
  return {
    id: raw.id ? String(raw.id) : undefined,
    projectId: raw.project_id ? String(raw.project_id) : undefined,
    name: String(raw.name ?? ''),
    schemaMode: raw.schema_mode === 'view' ? 'view' : 'json',
    viewQuery: raw.view_query ? String(raw.view_query) : undefined,
    fields: fields.map((entry) => {
      const field = collectionWire(entry)
      const options = collectionWire(field.options)
      const type = String(field.type ?? 'text') as CollectionSchema['fields'][number]['type']
      return {
        name: String(field.name ?? ''),
        type,
        required: field.required === true,
        unique: field.unique === true,
        default: field.default,
        hidden: field.hidden === true,
        readonly: field.readonly === true,
        options: {
          min: typeof options.min === 'number' || typeof options.min === 'string' ? options.min : undefined,
          max: typeof options.max === 'number' || typeof options.max === 'string' ? options.max : undefined,
          pattern: options.pattern ? String(options.pattern) : undefined,
          values: Array.isArray(options.values) ? options.values.map(String) : undefined,
          maxSelect: options.max_select ? Number(options.max_select) : undefined,
          maxSize: options.max_size ? Number(options.max_size) : undefined,
          onlyInt: options.only_int === true,
          exceptDomains: Array.isArray(options.except_domains) ? options.except_domains.map(String) : undefined,
          onlyDomains: Array.isArray(options.only_domains) ? options.only_domains.map(String) : undefined
        }
      }
    })
  }
}

export function toDocumentItem(value: unknown): DocumentItem {
  const raw = collectionWire(value)
  return {
    id: String(raw.id ?? ''),
    createdAt: String(raw.created_at ?? ''),
    updatedAt: String(raw.updated_at ?? raw.created_at ?? ''),
    data: collectionWire(raw.data)
  }
}

export function toDocumentPage(value: unknown): DocumentPage {
  const raw = collectionWire(value)
  return {
    rows: Array.isArray(raw.rows) ? (raw.rows as DocumentPage['rows']) : [],
    items: listOf(raw.items, toDocumentItem),
    nextCursor: raw.next_cursor ? String(raw.next_cursor) : undefined,
    total: typeof raw.total === 'number' ? raw.total : undefined
  }
}

/** 聚合请求 → query string 参数。数组与驼峰在这里转成后端的 snake_case；空值整键丢弃。 */
export function toAggregateParams(query: AggregateQuery): Record<string, string | number> {
  const params: Record<string, string | number> = {}
  const put = (key: string, value: string | number | undefined) => {
    if (value === undefined || value === '') return
    params[key] = value
  }
  put('mode', query.mode)
  put('metrics', query.metrics && query.metrics.length ? query.metrics.join(',') : undefined)
  put('group_by', query.groupBy)
  put('series', query.series)
  put('x', query.x)
  put('y', query.y)
  put('size', query.size)
  put('filter', query.filter)
  put('sort', query.sort)
  put('limit', query.limit)
  if (query.withTotal) params.with_total = 1
  return params
}

export function toAggregateResult(value: unknown): AggregateResult {
  const raw = collectionWire(value)
  const total = Number(raw.total)
  return {
    columns: listOf(raw.columns, String),
    rows: Array.isArray(raw.rows) ? (raw.rows as unknown[][]) : [],
    // 后端不提供时是 undefined（不是 0）：调用方要区分「总量为 0」与「没有总量」。
    ...(raw.total === undefined || raw.total === null || !Number.isFinite(total) ? {} : { total }),
    truncated: raw.truncated === true,
    scanned: Number(raw.scanned ?? 0),
    durationMs: Number(raw.duration_ms ?? 0),
    requestId: String(raw.request_id ?? '')
  }
}

export function toSchemaDryRun(value: unknown): SchemaDryRunResult {
  const raw = collectionWire(value)
  const invalid = listOf(raw.invalid)
  return {
    invalid: invalid.map((entry) => {
      const item = collectionWire(entry)
      return { id: String(item.id ?? ''), field: String(item.field ?? ''), reason: String(item.reason ?? '') }
    }),
    scanned: Number(raw.scanned ?? 0),
    truncated: raw.truncated === true
  }
}
