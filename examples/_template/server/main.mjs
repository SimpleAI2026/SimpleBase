import { runtime, rows, transaction, serve, startWorker } from '../../shared/node/runtime.mjs'
import { validateItem } from './validation.mjs'

const rt = runtime('__prefix__')
const { sql, kv } = rt

async function init() {
  for (const ddl of [
    'CREATE TABLE IF NOT EXISTS ex___prefix___items (id VARCHAR PRIMARY KEY, owner_id VARCHAR NOT NULL, title VARCHAR NOT NULL, created_at VARCHAR NOT NULL)',
    'CREATE TABLE IF NOT EXISTS ex___prefix___hourly_stats (window_start VARCHAR PRIMARY KEY, count BIGINT NOT NULL, source_run_id VARCHAR NOT NULL)',
    'CREATE TABLE IF NOT EXISTS ex___prefix___processed_runs (run_id VARCHAR PRIMARY KEY, window_start VARCHAR NOT NULL)'
  ]) await sql.execute(ddl)
}

async function handler(method, url, input) {
  if (url.pathname === '/api/items' && method === 'GET') {
    const views = await kv('INCR', 'ex:__prefix__:views')
    const data = rows(await sql.query('SELECT id, title, created_at FROM ex___prefix___items WHERE owner_id = ? ORDER BY created_at DESC LIMIT 100', ['local-demo'], { maxRows: 100 }))
    return { data: { views: Number(views || 0), items: data } }
  }
  if (url.pathname === '/api/items' && method === 'POST') {
    const title = validateItem(input)
    const id = rt.uuid()
    await sql.execute('INSERT INTO ex___prefix___items (id, owner_id, title, created_at) VALUES (?, ?, ?, ?)', [id, 'local-demo', title, new Date().toISOString()])
    return { status: 201, data: { id, title } }
  }
  if (url.pathname === '/api/stats' && method === 'GET') {
    return { data: rows(await sql.query('SELECT window_start, count FROM ex___prefix___hourly_stats ORDER BY window_start DESC LIMIT 24', [], { maxRows: 24 })) }
  }
  return { status: 404, data: { error: 'Not found' } }
}

async function query(window) {
  const data = rows(await sql.query('SELECT created_at FROM ex___prefix___items WHERE created_at >= ? AND created_at < ? LIMIT 101', [window.window_start, window.window_end], { maxRows: 101 }))
  if (data.length > 100) throw new Error('Too many items; manual backfill required')
  return { rows: data }
}

async function apply(window, run, output) {
  if (!Number.isSafeInteger(output.count) || output.count < 0) throw new Error('Invalid count result')
  await transaction(sql, [
    { sql: 'INSERT INTO ex___prefix___hourly_stats SELECT ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM ex___prefix___hourly_stats WHERE window_start = ?)', args: [window.window_start, output.count, run.id, window.window_start] },
    { sql: 'INSERT INTO ex___prefix___processed_runs (run_id, window_start) VALUES (?, ?)', args: [run.id, window.window_start] }
  ])
}

if (process.argv[1] && import.meta.url === new URL(`file://${process.argv[1]}`).href) {
  await init()
  if (process.env.EXAMPLE_WORKER === '1') startWorker(rt, { jobName: 'ex___prefix___tick', fn: 'ex___prefix___job', query, apply })
  serve(handler, Number(process.env.EXAMPLE_PORT || 8100), '__name__')
}
