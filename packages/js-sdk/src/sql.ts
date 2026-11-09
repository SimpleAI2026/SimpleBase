import type { HttpClient } from './http.js'
import { projectPath } from './http.js'
import type { BatchResult, BatchStatement, ExecuteResult, QueryResult } from './types.js'

export interface SqlApi {
  query(
    sql: string,
    args?: unknown[],
    opts?: { maxRows?: number }
  ): Promise<QueryResult>
  execute(sql: string, args?: unknown[]): Promise<ExecuteResult>
  batch(
    statements: BatchStatement[],
    opts?: { transactional?: boolean }
  ): Promise<BatchResult>
}

export function createSqlApi(http: HttpClient, databaseId: string): SqlApi {
  const db = encodeURIComponent(databaseId)
  const p = (suffix: string) =>
    projectPath(http.projectId, `/databases/${db}${suffix}`)

  return {
    query(sql, args = [], opts) {
      if (!sql?.trim()) throw new Error('sql is required')
      return http.request('POST', p('/query'), {
        body: { sql, args, max_rows: opts?.maxRows }
      })
    },
    execute(sql, args = []) {
      if (!sql?.trim()) throw new Error('sql is required')
      return http.request('POST', p('/execute'), { body: { sql, args } })
    },
    batch(statements, opts) {
      return http.request('POST', p('/batch'), {
        body: {
          statements: statements.map((s) => ({ sql: s.sql, args: s.args ?? [] })),
          transactional: opts?.transactional ?? true
        }
      })
    }
  }
}
