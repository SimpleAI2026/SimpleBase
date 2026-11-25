/**
 * 项目级 REST 路径的唯一构造入口：每个动态段都经 encodeURIComponent，
 * 页面与 API 层不再手拼 `'/v1/projects/' + encodeURIComponent(...)`。
 */

export type PathSegment = string | number

/** `/v1/projects/:projectId/<seg>/<seg>…`；每段单独编码，段内的 `/` 会被转义。 */
export function projectPath(projectId: string, ...segments: PathSegment[]): string {
  let path = '/v1/projects/' + encodeURIComponent(projectId)
  for (const seg of segments) path += '/' + encodeURIComponent(String(seg))
  return path
}

/** 云助手 / MCP 等需要「前缀 + 已拼好的子路径」的写法；rest 须以 `/` 开头且自行编码。 */
export function agentPath(projectId: string, rest = ''): string {
  return projectPath(projectId) + rest
}

/** KV 单端点路径（项目级，无 databaseId）。 */
export function kvPath(projectId: string): string {
  return projectPath(projectId, 'kv')
}

/** 定时任务路径：/v1/projects/:pid/cron-jobs[/:jobId[/runs|/trigger]] */
export function cronJobsPath(projectId: string, jobId?: string, suffix?: 'runs' | 'trigger'): string {
  let path = projectPath(projectId, 'cron-jobs')
  if (jobId) {
    path += '/' + encodeURIComponent(jobId)
    if (suffix) path += '/' + suffix
  }
  return path
}

/** 云函数路径 */
export function gofunctionsPath(projectId: string, name?: string, ver?: number | string, suffix?: string): string {
  let path = projectPath(projectId, 'gofunctions')
  if (name) path += '/' + encodeURIComponent(name)
  if (ver !== undefined) path += '/versions/' + encodeURIComponent(String(ver))
  if (suffix) path += '/' + suffix
  return path
}

/** 数据库子资源：suffix 由调用方给出（以 `/` 开头，自行编码）。 */
export function databaseResourcePath(projectId: string, databaseId: string, suffix: string): string {
  return projectPath(projectId, 'databases', databaseId) + suffix
}

/** 集合数据面：/databases/:db/data/collections[/:collection] */
export function dataCollectionsPath(projectId: string, databaseId: string, collection?: string): string {
  let path = projectPath(projectId, 'databases', databaseId) + '/data/collections'
  if (collection) path += '/' + encodeURIComponent(collection)
  return path
}
