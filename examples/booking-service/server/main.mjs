import { runtime, rows, transaction, serve, startWorker } from '../../shared/node/runtime.mjs'
import { validSlot } from './validation.mjs'
const rt = runtime('booking')
const { sql, kv } = rt

async function init() {
  for (const ddl of [
    'CREATE TABLE IF NOT EXISTS ex_booking_slots (id VARCHAR PRIMARY KEY, starts_at VARCHAR NOT NULL, capacity BIGINT NOT NULL)',
    'CREATE TABLE IF NOT EXISTS ex_booking_reservations (id VARCHAR PRIMARY KEY, slot_id VARCHAR NOT NULL, owner_id VARCHAR NOT NULL, status VARCHAR NOT NULL, expires_at VARCHAR NOT NULL, created_at VARCHAR NOT NULL)',
    'CREATE TABLE IF NOT EXISTS ex_booking_processed_runs (run_id VARCHAR PRIMARY KEY, window_start VARCHAR NOT NULL)'
  ]) await sql.execute(ddl)
  const id = 'demo-slot'
  await sql.execute('INSERT INTO ex_booking_slots SELECT ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM ex_booking_slots WHERE id = ?)', [id, new Date(Date.now() + 86400000).toISOString(), 5, id])
}

async function handler(method, url, input) {
  if (url.pathname === '/api/slots' && method === 'GET') return { data: rows(await sql.query('SELECT id, starts_at, capacity FROM ex_booking_slots LIMIT 50', [], { maxRows: 50 })) }
  if (url.pathname === '/api/holds' && method === 'POST') {
    const slotId = validSlot(input)
    const found = rows(await sql.query('SELECT id FROM ex_booking_slots WHERE id = ?', [slotId], { maxRows: 1 }))
    if (!found.length) throw new Error('Invalid slot')
    const token = rt.uuid()
    await kv('SET', `ex:booking:hold:${token}`, slotId, 'EX', '120')
    return { status: 201, data: { token, expiresIn: 120 } }
  }
  if (url.pathname === '/api/bookings' && method === 'POST') {
    if (typeof input?.token !== 'string' || !/^[0-9a-f-]{36}$/.test(input.token)) throw new Error('Invalid token')
    const slotId = await kv('GET', `ex:booking:hold:${input.token}`)
    if (!slotId) throw new Error('Hold expired')
    const slot = rows(await sql.query('SELECT capacity FROM ex_booking_slots WHERE id = ?', [slotId], { maxRows: 1 }))[0]
    if (!slot) throw new Error('Invalid slot')
    const id = rt.uuid(), now = new Date().toISOString()
    const saved = await sql.execute("INSERT INTO ex_booking_reservations SELECT ?, ?, ?, ?, ?, ? WHERE (SELECT count(*) FROM ex_booking_reservations WHERE slot_id = ? AND status = 'confirmed') < (SELECT capacity FROM ex_booking_slots WHERE id = ?)", [id, slotId, 'local-demo', 'confirmed', new Date(Date.now() + 3600000).toISOString(), now, slotId, slotId])
    if (saved.rows_affected !== 1) throw new Error('Slot full')
    await kv('DEL', `ex:booking:hold:${input.token}`)
    return { status: 201, data: { id, slotId, status: 'confirmed' } }
  }
  if (url.pathname.startsWith('/api/bookings/') && method === 'DELETE') {
    const id = url.pathname.slice('/api/bookings/'.length)
    if (!/^[0-9a-f-]{36}$/.test(id)) throw new Error('Invalid ID')
    await sql.execute("UPDATE ex_booking_reservations SET status = 'cancelled' WHERE id = ? AND owner_id = ? AND status = 'confirmed'", [id, 'local-demo'])
    return { data: { cancelled: true } }
  }
  return { status: 404, data: { error: 'Not found' } }
}

async function query(window) {
  const candidates = rows(await sql.query("SELECT id, expires_at, status FROM ex_booking_reservations WHERE status = 'confirmed' AND expires_at < ? LIMIT 101", [window.window_end], { maxRows: 101 }))
  if (candidates.length > 100) throw new Error('Too many expired reservations; manual backfill required')
  return { cutoff: window.window_end, rows: candidates }
}
async function apply(window, run, output) {
  if (!Array.isArray(output.ids) || output.ids.length > 100) throw new Error('Invalid expiry result')
  const statements = output.ids.map(id => ({ sql: "UPDATE ex_booking_reservations SET status = 'expired' WHERE id = ? AND owner_id = ? AND status = 'confirmed' AND expires_at < ?", args: [id, 'local-demo', window.window_end] }))
  statements.push({ sql: 'INSERT INTO ex_booking_processed_runs (run_id, window_start) VALUES (?, ?)', args: [run.id, window.window_start] })
  await transaction(sql, statements)
}

if (process.argv[1] && import.meta.url === new URL(`file://${process.argv[1]}`).href) {
  await init()
  if (process.env.EXAMPLE_WORKER === '1') startWorker(rt, { jobName: 'ex_booking_expired', fn: 'ex_booking_expiry', query, apply })
  serve(handler, Number(process.env.EXAMPLE_PORT || 8093), 'booking-service')
}
