import { createHttpClient, type HttpClient } from './http.js'
import { createDatabasesApi, type DatabasesApi } from './databases.js'
import { createSqlApi, type SqlApi } from './sql.js'
import { createCollectionsApi, type CollectionsApi, type CollectionApi } from './collections.js'
import { createStorageApi, type StorageApi } from './storage.js'
import type { CreateClientOptions } from './types.js'
import { SimpleBaseError } from './errors.js'

export interface DatabaseRef {
  readonly id: string
  sql: SqlApi
  collections: CollectionsApi
  collection(name: string): CollectionApi
}

export interface SimpleBaseClient {
  readonly projectId: string
  readonly url: string
  databases: DatabasesApi
  storage: StorageApi
  /** Default-database SQL helpers (requires databaseId in createClient or .database()) */
  sql: SqlApi
  collection(name: string): CollectionApi
  database(id: string): DatabaseRef
  raw: {
    request: HttpClient['request']
  }
}

function requireDbId(id: string | undefined, action: string): string {
  if (!id) {
    throw new SimpleBaseError({
      message: `${action} requires databaseId — pass createClient({ databaseId }) or use client.database(id)`,
      status: 0,
      code: 'database_id_required'
    })
  }
  return id
}

export function createClient(opts: CreateClientOptions): SimpleBaseClient {
  if (!opts?.url) throw new Error('url is required')
  if (!opts?.apiKey) throw new Error('apiKey is required')
  if (!opts?.projectId) throw new Error('projectId is required')

  const http = createHttpClient(opts)
  const databases = createDatabasesApi(http)
  const storage = createStorageApi(http)
  let defaultDatabaseId = opts.databaseId

  const database = (id: string): DatabaseRef => {
    const sql = createSqlApi(http, id)
    const collections = createCollectionsApi(http, id)
    return {
      id,
      sql,
      collections,
      collection: (name) => collections.collection(name)
    }
  }

  const client: SimpleBaseClient = {
    projectId: opts.projectId,
    url: http.baseUrl,
    databases,
    storage,
    get sql() {
      return createSqlApi(http, requireDbId(defaultDatabaseId, 'sql'))
    },
    collection(name) {
      return createCollectionsApi(http, requireDbId(defaultDatabaseId, 'collection')).collection(name)
    },
    database,
    raw: {
      request: http.request.bind(http)
    }
  }

  return client
}
