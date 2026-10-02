import { createServer } from 'node:http'
import { randomUUID } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import { extname } from 'node:path'
import { createClient } from '../../../packages/js-sdk/dist/index.js'

export function runtime(name) {
  for (const key of ['SIMPLEBASE_URL', 'SIMPLEBASE_API_KEY', 'SIMPLEBASE_PROJECT_ID', 'SIMPLEBASE_DATABASE_ID']) if (!process.env[key]) throw new Error(`Missing ${key}`)
  const { SIMPLEBASE_URL: url, SIMPLEBASE_API_KEY: apiKey, SIMPLEBASE_PROJECT_ID: projectId, SIMPLEBASE_DATABASE_ID: databaseId } = process.env
  const sb = createClient({ url, apiKey, projectId, databaseId })
  const sql = sb.database(databaseId).sql
  const project = `/v1/projects/${encodeURIComponent(projectId)}`
  return {
    name, sb, sql, projectId, uuid: randomUUID,
    kv: (...argvs) => sb.raw.request('POST', `${project}/kv`, { body: { type: 'cmd', argvs: argvs.map(String) } }),
    manage: (method, suffix, body) => sb.raw.request(method, `${project}${suffix}`, body === undefined ? {} : { body }),
    compute: (filename, payload) => sb.raw.request('POST', `/go/${encodeURIComponent(projectId)}/${filename}/Compute`, { body: payload })
  }
}
export function rows(result) { return result.rows.map(row => Object.fromEntries(result.columns.map((column, index) => [column, row[index]]))) }
export async function transaction(sql, statements) {
  const result = await sql.batch(statements, { transactional: true })
  if (result.error || result.results?.some(entry => entry.error_code)) throw new Error('Transaction failed')
  return result
}
export async function body(req) {
  let data = ''
  for await (const part of req) { data += part; if (data.length > 8192) throw new Error('Request too large') }
  return JSON.parse(data || '{}')
}
export function respond(res, status, data) {
  res.writeHead(status, { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' })
  res.end(JSON.stringify(data))
}
export function serve(handler, port, caseName) {
  createServer(async (req, res) => {
    try {
      const url = new URL(req.url, 'http://localhost')
      if (req.method === 'GET' && ['/', '/index.html', '/app.js', '/style.css'].includes(url.pathname)) {
        const file = url.pathname === '/' ? 'index.html' : url.pathname.slice(1)
        const bytes = await readFile(new URL(`../../${caseName}/dist/${file}`, import.meta.url))
        res.writeHead(200, { 'content-type': extname(file) === '.html' ? 'text/html; charset=utf-8' : extname(file) === '.css' ? 'text/css' : 'text/javascript', 'x-content-type-options': 'nosniff' })
        return res.end(bytes)
      }
      if (url.pathname === '/health') return respond(res, 200, { ok: true })
      if (!req.headers.origin || new URL(req.headers.origin).host !== req.headers.host || req.headers['sec-fetch-site'] === 'cross-site') throw new Error('Invalid origin; local demo only')
      const result = await handler(req.method, url, await (req.method === 'GET' ? Promise.resolve({}) : body(req)))
      respond(res, result.status || 200, result.data)
    } catch (error) {
      respond(res, error instanceof SyntaxError || /Invalid|large|expired|unavailable|Conflict|full/.test(error.message) ? 400 : 500, { error: error instanceof SyntaxError ? 'Invalid JSON' : error.message })
    }
  }).listen(port, '127.0.0.1', () => console.log(`Local example listening at http://127.0.0.1:${port}`))
}

export function startWorker(rt, { jobName, fn, query, apply }) {
  let busy = false
  let lastSeen = null
  const timer = setInterval(async () => {
    if (busy) return
    busy = true
    try {
      const jobs = await rt.manage('GET', '/cron-jobs')
      const job = jobs.jobs?.find(entry => entry.name === jobName)
      if (!job) return
      const { runs } = await rt.manage('GET', `/cron-jobs/${job.id}/runs?limit=100`)
      if (!Array.isArray(runs)) throw new Error('Invalid runs response')
      if (runs.length === 100 && (!lastSeen || !runs.some(run => run.id === lastSeen))) throw new Error('Run history gap; manual reconciliation required')
      for (const run of [...runs].reverse()) {
        if (run.status !== 'completed') continue
        const processed = rows(await rt.sql.query(`SELECT run_id FROM ex_${rt.name}_processed_runs WHERE run_id = ?`, [run.id], { maxRows: 1 }))
        if (processed.length) continue
        const window = JSON.parse(run.response_json || '')
        const start = Date.parse(window.window_start), end = Date.parse(window.window_end)
        if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start || end - start > 86400000 || end > Date.now() + 60000) throw new Error('Invalid cron window')
        const data = await query(window)
        if (JSON.stringify(data).length > 30000) throw new Error('Compute input too large; manual backfill required')
        const computed = await rt.compute(fn, data)
        await apply(window, run, computed)
      }
      if (runs.length) lastSeen = runs[0].id
    } catch (error) { console.error(`${jobName} worker stopped: ${error.message}`); clearInterval(timer) } finally { busy = false }
  }, 15000)
  timer.unref()
  return () => clearInterval(timer)
}
