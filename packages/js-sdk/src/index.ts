export { createClient } from './client.js'
export type { SimpleBaseClient, DatabaseRef } from './client.js'
export { SimpleBaseError } from './errors.js'
export type {
  CreateClientOptions,
  DatabaseInfo,
  QueryResult,
  ExecuteResult,
  BatchStatement,
  BatchResult,
  BatchResultItem,
  S3ObjectMeta,
  Json
} from './types.js'
export type { DatabasesApi } from './databases.js'
export type { SqlApi } from './sql.js'
export type { CollectionsApi, CollectionApi } from './collections.js'
export type { StorageApi, UploadBody } from './storage.js'
