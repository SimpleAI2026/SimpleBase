import type { HttpClient } from './http.js'
import { projectPath } from './http.js'
import type { DatabaseInfo } from './types.js'

export interface DatabasesApi {
  list(opts?: { limit?: number; cursor?: string }): Promise<{ databases: DatabaseInfo[]; next_cursor?: string }>
  get(databaseId: string): Promise<DatabaseInfo>
  create(input: { name: string }): Promise<DatabaseInfo>
  remove(databaseId: string): Promise<DatabaseInfo | void>
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
    }
  }
}
