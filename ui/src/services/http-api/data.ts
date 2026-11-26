// 文件说明：http-api/data.ts — `httpApi` 的数据面（数据库、SQL、集合文档、KV）部分；只在这里出现对应的 URL 与 wire → UI 转换。
// 与 http-api.ts、其余域文件共同组成 `Api` 的唯一实现，不要在页面里直接 import 本文件。
import { http } from '@/services/http'
import type { CreateDatabaseInput, ViewDryRunResult, CollectionImportResult } from '@/services/types'
import { toS3FolderMeta, toS3PublicResult, toS3SiteInfo } from '@/services/converters/platform'
import {
  toCollectionAccessRule,
  toCollectionInfo,
  toCollectionSchema,
  toDatabaseItem,
  toDatabaseSchema,
  toDocumentItem,
  toDocumentPage,
  toAggregateParams,
  toAggregateResult,
  toExecuteResult,
  toQueryResult,
  toSchemaColumn,
  toSchemaDryRun,
  toSchemaTable,
  toSchemaTableRows,
  toBatchResult,
  toSqlPayload
} from '@/services/converters/data'
import { dataCollectionsPath, databaseResourcePath, kvPath, projectPath } from '@/services/paths'
import { listOf } from '@/services/wire'
import type { Api } from '@/services/types'


export const dataApi = {
  databases: {
    list: (projectId) =>
      http
        .get(projectPath(projectId, 'databases'), { params: { limit: 200 } })
        .then((r) => listOf(r.data?.databases, toDatabaseItem)),
    create: (projectId, input: CreateDatabaseInput) =>
      http
        .post(projectPath(projectId, 'databases'), {
          name: input.name,
          data_model: input.dataModel,
          init_sql: input.initSql
        })
        .then((r) => toDatabaseItem(r.data)),
    remove: (projectId, databaseId) =>
      http
        .delete(projectPath(projectId, 'databases', databaseId))
        .then(() => undefined),
    schema: (projectId, databaseId) =>
      http
        .get(databaseResourcePath(projectId, databaseId, '/schema'))
        .then((r) => toDatabaseSchema(r.data)),
    tableRows: (projectId, databaseId, table, query) =>
      http
        .get(projectPath(projectId, 'databases', databaseId, 'schema', 'tables', table, 'rows'), {
          // filter 为空时不带该键，请求与改造前逐字一致。
          params: { limit: query.limit, offset: query.offset, ...(query.filter ? { filter: query.filter } : {}) }
        })
        .then((r) => toSchemaTableRows(r.data)),
    aggregateTable: (projectId, databaseId, table, query) =>
      http
        .get(projectPath(projectId, 'databases', databaseId, 'schema', 'tables', table, 'aggregate'), {
          params: toAggregateParams(query)
        })
        .then((r) => toAggregateResult(r.data)),
    createTable: (projectId, databaseId, table) =>
      http
        .post(databaseResourcePath(projectId, databaseId, '/schema/tables'), {
          name: table.name,
          columns: table.columns.map((column) => ({
            name: column.name,
            type: column.type,
            nullable: column.nullable
          }))
        })
        .then((r) => toSchemaTable(r.data)),
    addColumn: (projectId, databaseId, column) =>
      http
        .post(databaseResourcePath(projectId, databaseId, '/schema/columns'), {
          table: column.table,
          name: column.name,
          type: column.type,
          nullable: column.nullable
        })
        .then((r) => toSchemaColumn(r.data))
  },

  sql: {
    query: (projectId, databaseId, req) =>
      http
        .post(projectPath(projectId, 'databases', databaseId, 'query'), toSqlPayload(req))
        .then((r) => toQueryResult(r.data)),
    execute: (projectId, databaseId, req) =>
      http
        .post(projectPath(projectId, 'databases', databaseId, 'execute'), toSqlPayload(req))
        .then((r) => toExecuteResult(r.data)),
    batch: (projectId, databaseId, req) =>
      http
        .post(projectPath(projectId, 'databases', databaseId, 'batch'), {
          statements: req.statements.map(toSqlPayload),
          transactional: req.transactional
        })
        .then((r) => toBatchResult(r.data))
  },

  db: {
    collections: (projectId, databaseId) =>
      http.get(dataCollectionsPath(projectId, databaseId)).then((r) =>
        listOf(r.data?.collections)
      ),
    createCollection: (projectId, databaseId, name) =>
      http.post(dataCollectionsPath(projectId, databaseId), { name }).then(() => undefined),
    rows: (projectId, databaseId, collection, query) =>
      http
        .get(dataCollectionsPath(projectId, databaseId, collection), { params: query })
        .then((r) => (listOf(r.data?.rows))),
    insert: (projectId, databaseId, collection, payload) =>
      http
        .post(dataCollectionsPath(projectId, databaseId, collection) + '/documents', payload)
        .then((r) => r.data),
    update: (projectId, databaseId, collection, id, payload) =>
      http
        .put(
          dataCollectionsPath(projectId, databaseId, collection) +
            '/documents/' +
            encodeURIComponent(id),
          payload
        )
        .then((r) => r.data),
    remove: (projectId, databaseId, collection, id) =>
      http
        .delete(
          dataCollectionsPath(projectId, databaseId, collection) +
            '/documents/' +
            encodeURIComponent(id)
        )
        .then(() => undefined),
    collectionItems: (projectId, databaseId) =>
      http.get(dataCollectionsPath(projectId, databaseId)).then((r) =>
        listOf(r.data?.items, toCollectionInfo)
      ),
    exportCollection: (projectId, databaseId, collection, format, query) =>
      http.get(dataCollectionsPath(projectId, databaseId, collection) + '/export', {
        params: { ...query, format }, responseType: 'blob'
      }).then((r) => r.data as Blob),
    importCollection: (projectId, databaseId, collection, file, format, dryRun = false) =>
      http.post(dataCollectionsPath(projectId, databaseId, collection) + '/import', file, {
        params: { dry_run: dryRun ? 1 : undefined },
        headers: { 'Content-Type': format === 'csv' ? 'text/csv' : 'application/x-ndjson' }
      }).then((r): CollectionImportResult => ({
        dryRun: r.data?.dry_run === true,
        scanned: Number(r.data?.scanned ?? 0),
        imported: Number(r.data?.imported ?? 0)
      })),
    dryRunView: (projectId, databaseId, name, query) =>
      http.post(dataCollectionsPath(projectId, databaseId) + '/meta/dry-run-view', { name, query })
        .then((r): ViewDryRunResult => ({
          columns: listOf(r.data?.columns, String),
          fields: Array.isArray(r.data?.fields) ? r.data.fields.map((field: unknown) => toCollectionSchema({ fields: [field] }).fields[0]) : [],
          items: listOf(r.data?.items, toDocumentItem),
          scanned: Number(r.data?.scanned ?? 0)
        })),
    documents: (projectId, databaseId, collection, query) =>
      http
        .get(dataCollectionsPath(projectId, databaseId, collection), { params: query })
        .then((r) => toDocumentPage(r.data)),
    document: (projectId, databaseId, collection, id) =>
      http
        .get(
          dataCollectionsPath(projectId, databaseId, collection) + '/documents/' + encodeURIComponent(id)
        )
        .then((r) => toDocumentItem(r.data)),
    aggregate: (projectId, databaseId, collection, query) =>
      http
        .get(dataCollectionsPath(projectId, databaseId, collection) + '/aggregate', {
          params: toAggregateParams(query)
        })
        .then((r) => toAggregateResult(r.data)),
    patch: (projectId, databaseId, collection, id, payload) =>
      http
        .patch(
          dataCollectionsPath(projectId, databaseId, collection) + '/documents/' + encodeURIComponent(id),
          payload
        )
        .then((r) => r.data),
    batch: (projectId, databaseId, collection, documents) =>
      http
        .post(dataCollectionsPath(projectId, databaseId, collection) + '/documents/batch', { documents })
        .then((r) => listOf(r.data?.items, toDocumentItem)),
    truncate: (projectId, databaseId, collection) =>
      http.delete(dataCollectionsPath(projectId, databaseId, collection)).then(() => undefined),
    getCollectionSchema: (projectId, databaseId, collection) =>
      http
        .get(dataCollectionsPath(projectId, databaseId, collection) + '/meta')
        .then((r) => toCollectionSchema(r.data)),
    putCollectionSchema: (projectId, databaseId, schema, dryRun = false) =>
      http
        .put(
          dataCollectionsPath(projectId, databaseId, schema.name) + '/meta',
          {
            name: schema.name,
            schema_mode: schema.schemaMode || 'json',
            view_query: schema.viewQuery,
            fields: schema.fields.map((field) => ({
              ...field,
              options: {
                min: field.options.min,
                max: field.options.max,
                pattern: field.options.pattern,
                values: field.options.values,
                max_select: field.options.maxSelect,
                max_size: field.options.maxSize,
                only_int: field.options.onlyInt,
                except_domains: field.options.exceptDomains,
                only_domains: field.options.onlyDomains
              }
            }))
          },
          { params: { dry_run: dryRun ? 1 : undefined } }
        )
        .then((r) => Array.isArray(r.data?.invalid) ? toSchemaDryRun(r.data) : toCollectionSchema(r.data)),
    deleteCollectionSchema: (projectId, databaseId, collection) =>
      http.delete(dataCollectionsPath(projectId, databaseId, collection) + '/meta').then(() => undefined),
    getCollectionAccess: (projectId, databaseId, collection) =>
      http
        .get(dataCollectionsPath(projectId, databaseId, collection) + '/access')
        .then((r) => toCollectionAccessRule(r.data)),
    putCollectionAccess: (projectId, databaseId, collection, rule) =>
      http
        .put(dataCollectionsPath(projectId, databaseId, collection) + '/access', {
          mode: rule.mode,
          owner_field: rule.ownerField
        })
        .then((r) => toCollectionAccessRule(r.data)),
    deleteCollectionAccess: (projectId, databaseId, collection) =>
      http
        .delete(dataCollectionsPath(projectId, databaseId, collection) + '/access')
        .then((r) => toCollectionAccessRule(r.data))
  },

  kv: {
    /** 项目级单端点：读/结构命令 type=cmd，按类型写入 type=String 等 */
    exec: (projectId, body) =>
      http.post(kvPath(projectId), body).then((r) => r.data),
    /** 顺序执行多条命令（非事务）；任一失败即停止 */
    execBatch: (projectId, bodies) =>
      Promise.all(bodies.map((b) => http.post(kvPath(projectId), b).then((r) => r.data)))
  },

  s3: {
    list: (projectId, prefix) =>
      http
        .get(projectPath(projectId, 's3', 'objects'), {
          params: { prefix }
        })
        .then((r) => listOf(r.data)),
    listDetailed: (projectId, prefix) =>
      http
        .get(projectPath(projectId, 's3', 'objects'), { params: { prefix, delimiter: '/' } })
        .then((r) => ({
          folders: listOf(r.data?.folders),
          objects: listOf(r.data?.objects),
          folderMeta: listOf(r.data?.folderMeta, toS3FolderMeta),
          ...(r.data?.site ? { site: toS3SiteInfo(r.data.site) } : {})
        })),
    getSite: (projectId) =>
      http.get(projectPath(projectId, 's3', 'site')).then((r) => toS3SiteInfo(r.data)),
    setPublic: (projectId, key, kind, isPublic) =>
      http
        .put(projectPath(projectId, 's3', 'public'), { key, kind, public: isPublic })
        .then((r) => toS3PublicResult(r.data)),
    presign: (projectId, key) =>
      http
        .get(projectPath(projectId, 's3', 'presign'), { params: { key } })
        .then((r) => r.data),
    download: (projectId, key) =>
      http
        .get(projectPath(projectId, 's3', 'objects', 'content'), { params: { key }, responseType: 'blob' })
        .then((r) => r.data as Blob),
    createLink: (projectId, key, ttlSeconds = 900) =>
      http
        .post(projectPath(projectId, 's3', 'link'), { key, ttl_seconds: ttlSeconds })
        .then((r) => ({ url: String(r.data?.url ?? ''), expiresAt: String(r.data?.expires_at ?? '') })),
    remove: (projectId, key) =>
      http
        .delete(projectPath(projectId, 's3', 'objects'), { params: { key } })
        .then(() => undefined),
    upload: (projectId, key, file, onProgress, signal) => {
      const fd = new FormData()
      fd.append('key', key)
      fd.append('file', file)
      return http
        .post(projectPath(projectId, 's3', 'objects'), fd, {
          // 实例默认 15s，50MB 上传会先被掐断。
          timeout: 0,
          signal,
          onUploadProgress: (e) => {
            if (onProgress && e.total) onProgress(Math.round((e.loaded / e.total) * 100))
          }
        })
        .then((r) => r.data)
    }
  },
} satisfies Partial<Api>
