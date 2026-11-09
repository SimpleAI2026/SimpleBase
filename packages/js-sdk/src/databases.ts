import type { HttpClient } from './http.js'
import { projectPath } from './http.js'
import type { DatabaseInfo } from './types.js'

export interface CreateDatabaseInput {
  name: string
  data_model?: 'collection' | 'sql'
  init_sql?: string
}

export interface SchemaColumn {
  name: string
  type: string
  nullable?: boolean
}

export interface SchemaTable {
  name: string
  columns: SchemaColumn[]
}

export interface DatabaseSchema {
  tables: SchemaTable[]
}

export interface AddColumnInput {
  table: string
  name: string
  type: string
  nullable?: boolean
}

export interface DatabasesApi {
  list(opts?: { limit?: number; cursor?: string }): Promise<{ databases: DatabaseInfo[]; next_cursor?: string }>
  get(databaseId: string): Promise<DatabaseInfo>
  create(input: CreateDatabaseInput): Promise<DatabaseInfo>
  remove(databaseId: string): Promise<DatabaseInfo | void>
  schema(databaseId: string): Promise<DatabaseSchema>
  createTable(databaseId: string, table: SchemaTable): Promise<SchemaTable>
  addColumn(databaseId: string, column: AddColumnInput): Promise<SchemaColumn>
}

export function createDatabasesApi(http: HttpClient): DatabasesApi {
  const base = (s: string) => projectPath(http.projectId, s)
  return {
    list(opts) {
      return http.request('GET', base('/databases'), {
        query: { limit: opts?.limit, cursor: opts?.cursor }
      })
    },
    get(databaseId) {
      return http.request('GET', base(`/databases/${encodeURIComponent(databaseId)}`))
    },
    create(input) {
      return http.request('POST', base('/databases'), { body: input })
    },
    remove(databaseId) {
      return http.request('DELETE', base(`/databases/${encodeURIComponent(databaseId)}`))
    },
    schema(databaseId) {
      return http.request('GET', base(`/databases/${encodeURIComponent(databaseId)}/schema`))
    },
    createTable(databaseId, table) {
      return http.request('POST', base(`/databases/${encodeURIComponent(databaseId)}/schema/tables`), { body: table })
    },
    addColumn(databaseId, column) {
      return http.request('POST', base(`/databases/${encodeURIComponent(databaseId)}/schema/columns`), { body: column })
    }
  }
}
