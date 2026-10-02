import { runtime, rows, transaction, serve, startWorker } from '../../shared/node/runtime.mjs'
import { validateTicket } from './validation.mjs'
const rt = runtime('ticket')
const { sql, kv } = rt
async function init() {
  for (const ddl of [
    'CREATE TABLE IF NOT EXISTS ex_ticket_tickets (id VARCHAR PRIMARY KEY, owner_id VARCHAR NOT NULL, title VARCHAR NOT NULL, status VARCHAR NOT NULL, priority BIGINT NOT NULL, due_at VARCHAR NOT NULL, version BIGINT NOT NULL)',
    'CREATE TABLE IF NOT EXISTS ex_ticket_events (id VARCHAR PRIMARY KEY, ticket_id VARCHAR NOT NULL, action VARCHAR NOT NULL, at VARCHAR NOT NULL)',
    'CREATE TABLE IF NOT EXISTS ex_ticket_sla_marks (ticket_id VARCHAR NOT NULL, window_start VARCHAR NOT NULL, severity VARCHAR NOT NULL, source_run_id VARCHAR NOT NULL, PRIMARY KEY (ticket_id, window_start))',
    'CREATE TABLE IF NOT EXISTS ex_ticket_processed_runs (run_id VARCHAR PRIMARY KEY, window_start VARCHAR NOT NULL)'
  ]) await sql.execute(ddl)
}
async function handler(method, url, input) {
  if (url.pathname === '/api/tickets' && method === 'GET') return { data: rows(await sql.query('SELECT id, title, status, priority, due_at, version FROM ex_ticket_tickets WHERE owner_id = ? ORDER BY due_at DESC LIMIT 100', ['local-demo'], { maxRows: 100 })) }
  if (url.pathname === '/api/tickets' && method === 'POST') {
    const { title, priority } = validateTicket(input)
    const id = rt.uuid(), at = new Date().toISOString()
    await transaction(sql, [
      { sql: 'INSERT INTO ex_ticket_tickets (id, owner_id, title, status, priority, due_at, version) VALUES (?, ?, ?, ?, ?, ?, ?)', args: [id, 'local-demo', title, 'open', priority, new Date(Date.now() + 3600000 * priority).toISOString(), 1] },
      { sql: 'INSERT INTO ex_ticket_events (id, ticket_id, action, at) VALUES (?, ?, ?, ?)', args: [rt.uuid(), id, 'created', at] }
    ])
    return { status: 201, data: { id, title, priority, status: 'open' } }
  }
  if (url.pathname.startsWith('/api/tickets/') && method === 'PATCH') {
    const id = url.pathname.slice('/api/tickets/'.length)
    if (!/^[0-9a-f-]{36}$/.test(id) || !['open', 'closed'].includes(input.status) || !Number.isSafeInteger(input.version)) throw new Error('Invalid status or version')
    const found = rows(await sql.query('SELECT version FROM ex_ticket_tickets WHERE id = ? AND owner_id = ?', [id, 'local-demo'], { maxRows: 1 }))[0]
    if (!found || Number(found.version) !== input.version) throw new Error('Conflict: stale version')
    const result = await transaction(sql, [
      { sql: 'UPDATE ex_ticket_tickets SET status = ?, version = version + 1 WHERE id = ? AND owner_id = ? AND version = ?', args: [input.status, id, 'local-demo', input.version] },
      { sql: "INSERT INTO ex_ticket_events SELECT ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM ex_ticket_tickets WHERE id = ? AND owner_id = ? AND version = ?)", args: [rt.uuid(), id, input.status, new Date().toISOString(), id, 'local-demo', input.version + 1] }
    ])
    if (result.results?.[0]?.rows_affected !== 1) throw new Error('Conflict: stale version')
    await kv('DEL', 'ex:ticket:pending')
    return { data: { id, status: input.status, version: input.version + 1 } }
  }
  if (url.pathname === '/api/overdue' && method === 'GET') return { data: rows(await sql.query('SELECT ticket_id, severity, window_start FROM ex_ticket_sla_marks ORDER BY window_start DESC LIMIT 100', [], { maxRows: 100 })) }
  return { status: 404, data: { error: 'Not found' } }
}
async function query(window) {
  const data = rows(await sql.query("SELECT id, status, due_at, priority FROM ex_ticket_tickets WHERE status = 'open' AND due_at < ? LIMIT 101", [window.window_end], { maxRows: 101 }))
  if (data.length > 100) throw new Error('Too many overdue tickets; manual backfill required')
  return { now: window.window_end, rows: data.map(item => ({ ticket_id: item.id, ...item })) }
}
async function apply(window, run, output) {
  if (!Array.isArray(output.items) || output.items.length > 100) throw new Error('Invalid SLA result')
  const statements = output.items.map(item => ({ sql: "INSERT INTO ex_ticket_sla_marks SELECT ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM ex_ticket_tickets WHERE id = ? AND status = 'open' AND due_at < ?) AND NOT EXISTS (SELECT 1 FROM ex_ticket_sla_marks WHERE ticket_id = ? AND window_start = ?)", args: [item.ticket_id, window.window_start, item.severity, run.id, item.ticket_id, window.window_end, item.ticket_id, window.window_start] }))
  statements.push({ sql: 'INSERT INTO ex_ticket_processed_runs (run_id, window_start) VALUES (?, ?)', args: [run.id, window.window_start] })
  await transaction(sql, statements)
  await kv('SET', 'ex:ticket:pending', JSON.stringify(output.items.length), 'EX', '3600')
}
if (process.argv[1] && import.meta.url === new URL(`file://${process.argv[1]}`).href) {
  await init()
  if (process.env.EXAMPLE_WORKER === '1') startWorker(rt, { jobName: 'ex_ticket_sla_scan', fn: 'ex_ticket_sla', query, apply })
  serve(handler, Number(process.env.EXAMPLE_PORT || 8095), 'ticket-service')
}
