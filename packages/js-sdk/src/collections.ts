import type { HttpClient } from './http.js'
import { projectPath } from './http.js'
import type { Json } from './types.js'

const NAME_RE = /^[A-Za-z][A-Za-z0-9_]{0,62}$/

function assertCollectionName(name: string) {
  if (!NAME_RE.test(name)) throw new Error(`invalid collection name: ${name}`)
}

export interface DocumentRecord {
  id: string
  created_at: string
  updated_at: string
  data: Record<string, Json>
}

export interface DocumentList {
  rows: Record<string, unknown>[]
  items: DocumentRecord[]
  next_cursor?: string
  total?: number
  request_id?: string
}

export interface ListDocumentsOptions {
  limit?: number
  cursor?: string
  offset?: number
  sort?: string
  filter?: string
  fields?: string[]
  count?: boolean
}

export interface CollectionField {
  name: string
  type: 'text' | 'number' | 'bool' | 'date' | 'email' | 'url' | 'select' | 'json'
  required?: boolean
  unique?: boolean
  default?: Json
  options?: {
    min?: number | string
    max?: number | string
    pattern?: string
    values?: string[]
    max_select?: number
    max_size?: number
    only_int?: boolean
    except_domains?: string[]
    only_domains?: string[]
  }
  hidden?: boolean
  readonly?: boolean
}

export interface CollectionSchema {
  id?: string
  project_id?: string
  name: string
  schema_mode?: 'json' | 'view'
  view_query?: string
  fields: CollectionField[]
}

export interface ViewDryRunResult {
  columns: string[]
  fields: CollectionField[]
  items: DocumentRecord[]
  scanned: number
}

export interface CollectionInfo {
  name: string
  field_count: number
  document_count: number
  registered: boolean
}

export interface CollectionList {
  collections: string[]
  items: CollectionInfo[]
}

export interface SchemaDryRunResult {
  invalid: { id: string; field: string; reason: string }[]
  scanned: number
  truncated: boolean
}

export interface ImportCollectionOptions {
  body: BodyInit
  format: 'csv' | 'ndjson'
  dryRun?: boolean
}

export interface ImportCollectionResult {
  dry_run: boolean
  scanned: number
  imported: number
}

export interface ExportCollectionOptions {
  format: 'csv' | 'ndjson'
  filter?: string
  sort?: string
  fields?: string[]
}

export interface CollectionApi {
  list(options?: ListDocumentsOptions): Promise<DocumentList>
  import(options: ImportCollectionOptions): Promise<ImportCollectionResult>
  export(options: ExportCollectionOptions): Promise<Response>
  get(id: string): Promise<DocumentRecord>
  insert(doc: Record<string, Json | undefined>): Promise<Record<string, unknown>>
  update(id: string, doc: Record<string, Json | undefined>): Promise<Record<string, unknown>>
  patch(id: string, doc: Record<string, Json | undefined>): Promise<Record<string, unknown>>
  batch(documents: Record<string, Json | undefined>[]): Promise<{ items: DocumentRecord[] }>
  remove(id: string): Promise<void>
  truncate(): Promise<void>
  getSchema(): Promise<CollectionSchema>
  putSchema(schema: CollectionSchema): Promise<CollectionSchema>
  dryRunSchema(schema: CollectionSchema): Promise<SchemaDryRunResult>
  dryRunView(query: string): Promise<ViewDryRunResult>
  deleteSchema(): Promise<void>
}

export interface CollectionsApi {
  list(): Promise<CollectionList>
  create(name: string): Promise<void>
  collection(name: string): CollectionApi
}

export function createCollectionsApi(http: HttpClient, databaseId: string): CollectionsApi {
  const db = encodeURIComponent(databaseId)
  const root = () => projectPath(http.projectId, `/databases/${db}/data/collections`)

  function collection(name: string): CollectionApi {
    assertCollectionName(name)
    const base = `${root()}/${encodeURIComponent(name)}`
    const document = (id: string) => `${base}/documents/${encodeURIComponent(id)}`
    return {
      list(options = {}) {
        return http.request('GET', base, { query: { ...options, fields: options.fields?.join(','), count: options.count ? 1 : undefined } })
      },
      async import(options) {
        const response = await http.requestRaw('POST', `${base}/import`, {
          query: { dry_run: options.dryRun ? 1 : undefined },
          headers: { 'Content-Type': options.format === 'csv' ? 'text/csv' : 'application/x-ndjson' },
          body: options.body
        })
        return await response.json() as ImportCollectionResult
      },
      export(options) {
        return http.requestRaw('GET', `${base}/export`, {
          query: { format: options.format, filter: options.filter, sort: options.sort, fields: options.fields?.join(',') },
          headers: { Accept: options.format === 'csv' ? 'text/csv' : 'application/x-ndjson' }
        })
      },
      get(id) {
        return http.request('GET', document(id))
      },
      insert(doc) {
        return http.request('POST', `${base}/documents`, { body: doc })
      },
      update(id, doc) {
        return http.request('PUT', document(id), { body: doc })
      },
      patch(id, doc) {
        return http.request('PATCH', document(id), { body: doc })
      },
      batch(documents) {
        return http.request('POST', `${base}/documents/batch`, { body: { documents } })
      },
      async remove(id) {
        await http.request('DELETE', document(id))
      },
      async truncate() {
        await http.request('DELETE', base)
      },
      getSchema() {
        return http.request('GET', `${base}/meta`)
      },
      putSchema(schema) {
        return http.request('PUT', `${base}/meta`, { body: schema })
      },
      dryRunSchema(schema) {
        return http.request('PUT', `${base}/meta`, { query: { dry_run: 1 }, body: schema })
      },
      dryRunView(query) {
        return http.request('POST', `${root()}/meta/dry-run-view`, { body: { name, query } })
      },
      async deleteSchema() {
        await http.request('DELETE', `${base}/meta`)
      }
    }
  }

  return {
    list() {
      return http.request('GET', root())
    },
    async create(name) {
      assertCollectionName(name)
      await http.request('POST', root(), { body: { name } })
    },
    collection
  }
}
