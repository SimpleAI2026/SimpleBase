import { createServer } from 'node:http'
import { randomUUID } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import { createClient } from '../../../packages/js-sdk/dist/index.js'
import { realpathSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const { SIMPLEBASE_URL: url, SIMPLEBASE_API_KEY: apiKey, SIMPLEBASE_PROJECT_ID: projectId, SIMPLEBASE_DATABASE_ID: databaseId } = process.env
if (![url, apiKey, projectId, databaseId].every(Boolean)) throw new Error('Source examples/shop/.env first')
const sb = createClient({ url, apiKey, projectId, databaseId })
const sql = sb.database(databaseId).sql
const events = sb.database(databaseId).collection('shop_events')
const base = `/v1/projects/${projectId}`
const rows = result => result.rows.map(row => Object.fromEntries(result.columns.map((column, index) => [column, row[index]])))
const kv = (...argvs) => sb.raw.request('POST', `${base}/kv`, { body: { type: 'cmd', argvs } })
const reply = (res, status, value) => { res.writeHead(status, { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' }); res.end(JSON.stringify(value)) }

// 写一条集合文档，驱动 record_change 触发器（SQL 直写不产生文档事件）。
async function emitEvent(kind, action, detail) {
  try {
    await events.insert({ kind, action, detail: String(detail).slice(0, 200), at: new Date().toISOString() })
  } catch (error) { console.warn('emitEvent failed:', error.message) }
}

// 触发器管理面需要 project:admin，运行时 Key 读不到，改读初始化脚本写下的快照。
async function readTriggersSnapshot() {
  try {
    const snapshot = JSON.parse(await readFile(new URL('../.simplebase/triggers.json', import.meta.url), 'utf8'))
    return { triggers: snapshot.triggers || [], dropped: snapshot.dropped || 0 }
  } catch { return { triggers: [], dropped: 0 } }
}

async function auditList() {
  const list = rows(await sql.query('SELECT source, kind, action, detail, created_at FROM audit_log ORDER BY created_at DESC LIMIT 30', [], { maxRows: 30 }))
  const calls = {}
  for (const row of rows(await sql.query('SELECT source, COUNT(*) AS c FROM audit_log GROUP BY source'))) calls[row.source] = Number(row.c)
  const total = Number(rows(await sql.query('SELECT COUNT(*) AS c FROM audit_log'))[0]?.c || 0)
  return { items: list, total, calls }
}

async function platform() {
  const [objects, functions, jobs, audit] = await Promise.all([
    sb.storage.list({ prefix: 'products/', refresh: true }).catch(() => []),
    sb.raw.request('GET', `${base}/gofunctions`).catch(() => ({ functions: [] })),
    sb.raw.request('GET', `${base}/cron-jobs`).catch(() => ({ jobs: [] })),
    auditList()
  ])
  const keys = []
  let cursor = '0'
  for (let page = 0; page < 10 && keys.length < 40; page++) {
    const replyValue = await kv('SCAN', cursor, 'MATCH', '*', 'COUNT', '50').catch(() => null)
    if (!Array.isArray(replyValue)) break
    cursor = String(replyValue[0] ?? '0')
    for (const key of replyValue[1] || []) keys.push(key)
    if (cursor === '0') break
  }
  const kvRows = []
  for (const key of keys.slice(0, 20)) {
    const [type, ttl] = await Promise.all([kv('TYPE', key).catch(() => ''), kv('TTL', key).catch(() => -1)])
    let value = ''
    if (type === 'string') value = String(await kv('GET', key).catch(() => ''))
    else if (type === 'hash') value = JSON.stringify(await kv('HGETALL', key).catch(() => []))
    else if (type === 'list') value = JSON.stringify(await kv('LRANGE', key, '0', '5').catch(() => []))
    else if (type === 'set') value = JSON.stringify(await kv('SMEMBERS', key).catch(() => []))
    else if (type === 'zset') value = JSON.stringify(await kv('ZRANGE', key, '0', '5', 'WITHSCORES').catch(() => []))
    kvRows.push({ key, type: type || 'unknown', ttl: Number(ttl) > 0 ? Number(ttl) : 0, value: String(value).slice(0, 120) })
  }
  const snapshot = await readTriggersSnapshot()
  const lastRuns = {}
  for (const job of jobs.jobs || []) {
    const history = await sb.raw.request('GET', `${base}/cron-jobs/${job.id}/runs?limit=1`).catch(() => ({ runs: [] }))
    lastRuns[job.name] = history.runs?.[0]?.status || ''
  }
  return {
    kv: kvRows,
    objects: (objects || []).map(item => ({ key: item.key, size: item.size })),
    functions: (functions.functions || []).map(fn => ({ name: fn.name, latest_version: fn.latest_version, published: fn.published, exports: fn.exports })),
    cron_jobs: (jobs.jobs || []).map(job => ({ name: job.name, schedule_kind: job.schedule_kind, cron_expr: job.cron_expr, func_file: job.func_file, func_export: job.func_export, last_run_status: lastRuns[job.name] })),
    triggers: snapshot.triggers.map(item => ({ ...item, calls: audit.calls[item.export_source || item.kind] || 0 })),
    dropped: snapshot.dropped,
    audit
  }
}

async function platformAction(action, body) {
  if (action === 'emit-event') {
    await emitEvent('demo', 'create', body.detail || '平台页手动写入的演示事件')
    return { ok: true }
  }
  if (action === 'upload') {
    const key = typeof body.key === 'string' && body.key.startsWith('products/') ? body.key : 'products/demo/note.txt'
    await sb.storage.upload(key, String(body.content || `SimpleBase 示例对象 ${new Date().toISOString()}`), { contentType: 'text/plain' })
    return { ok: true, key }
  }
  if (action === 'invoke-trigger') {
    const snapshot = await readTriggersSnapshot()
    const httpTrigger = snapshot.triggers.find(item => item.kind === 'http' && item.enabled)
    if (!httpTrigger?.id) throw new Error('没有启用的 HTTP 触发器')
    await sb.raw.request('POST', `${base}/function-triggers/${httpTrigger.id}/invoke`, { body: { source: 'http', detail: '平台页手动 invoke' } })
    return { ok: true, queued: true }
  }
  if (action === 'run-cron') {
    const jobs = (await sb.raw.request('GET', `${base}/cron-jobs`)).jobs || []
    const job = jobs.find(item => item.name === 'shop_daily') || jobs[0]
    if (!job) throw new Error('没有定时任务')
    await sb.raw.request('POST', `${base}/cron-jobs/${job.id}/trigger`, { body: {} })
    return { ok: true, name: job.name }
  }
  throw new Error('Unknown action')
}
async function input(req) {
  let body = ''
  for await (const chunk of req) { body += chunk; if (body.length > 8192) throw new Error('Request too large') }
  return JSON.parse(body || '{}')
}
async function order(data, idem) {
  if (!/^[0-9a-f-]{36}$/i.test(idem)) throw new Error('Invalid Idempotency-Key (UUID required)')
  const previous = rows(await sql.query('SELECT id, total_minor FROM orders WHERE idem_key = ?', [idem], { maxRows: 1 }))[0]
  if (previous) return previous
  const productId = data.productId
  const qty = data.qty
  if (typeof productId !== 'string' || !Number.isSafeInteger(qty) || qty < 1 || qty > 10) throw new Error('Invalid order')
  const product = rows(await sql.query('SELECT id, price_minor, stock FROM products WHERE sku = ?', [productId], { maxRows: 1 }))[0]
  if (!product || Number(product.stock) < qty) throw new Error('Insufficient stock')
  const reservation = await kv('SET', `order:idem:${idem}`, '1', 'NX', 'EX', '600')
  if (reservation === null) {
    const again = rows(await sql.query('SELECT id, total_minor FROM orders WHERE idem_key = ?', [idem], { maxRows: 1 }))[0]
    if (again) return again
    throw new Error('Order in progress; retry')
  }
  try {
    const quote = await sb.raw.request('POST', `/go/${projectId}/pricing/Quote`, { body: { items: [{ price_minor: Number(product.price_minor), qty }] } })
    const id = randomUUID()
    const result = await sql.batch([
      { sql: 'UPDATE products SET stock = stock - ? WHERE id = ? AND stock >= ?', args: [qty, product.id, qty] },
      { sql: 'INSERT INTO orders (id, user_id, status, total_minor, idem_key, created_at) SELECT ?, ?, ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM orders WHERE idem_key = ?)', args: [id, 'demo-user', 'created', quote.total_minor, idem, new Date().toISOString(), idem] },
      { sql: 'INSERT INTO order_items (order_id, product_id, qty, price_minor) VALUES (?, ?, ?, ?)', args: [id, product.id, qty, Number(product.price_minor)] }
    ], { transactional: true })
    if (result.error || result.results?.some(item => item.error_code) || result.results?.[0]?.rows_affected !== 1 || result.results?.[1]?.rows_affected !== 1) throw new Error('Order transaction failed')
    await emitEvent('order', 'create', `${id} ¥${(quote.total_minor / 100).toFixed(2)} x${qty}`)
    return { id, total_minor: quote.total_minor }
  } catch (error) { await kv('DEL', `order:idem:${idem}`); throw error }
}
export async function handle(req, res) {
  try {
    const path = new URL(req.url, 'http://localhost').pathname
    if (req.method === 'GET' && path === '/') { res.writeHead(200, { 'content-type': 'text/html; charset=utf-8' }); return res.end(await readFile(new URL('../web/index.html', import.meta.url))) }
    if (req.method === 'GET' && path === '/health') return reply(res, 200, { ok: true })
    if (req.headers.origin && (new URL(req.headers.origin).host !== req.headers.host || req.headers['sec-fetch-site'] === 'cross-site')) throw new Error('Invalid origin')
    if (req.method === 'GET' && path === '/api/products') {
      const products = rows(await sql.query('SELECT sku, name, price_minor, stock, image_key FROM products ORDER BY sku LIMIT 50', [], { maxRows: 50 }))
      for (const product of products) product.image_url = (await sb.raw.request('GET', `${base}/s3/presign?key=${encodeURIComponent(product.image_key)}`).catch(() => null))?.url
      return reply(res, 200, products)
    }
    if (req.method === 'PUT' && path === '/api/cart') {
      const data = await input(req)
      if (typeof data.sku !== 'string' || !Number.isSafeInteger(data.qty) || data.qty < 1) throw new Error('Invalid cart')
      await kv('HSET', 'cart:demo', data.sku, String(data.qty)); await kv('EXPIRE', 'cart:demo', '1800')
      return reply(res, 200, data)
    }
    if (req.method === 'POST' && path === '/api/orders') return reply(res, 201, await order(await input(req), req.headers['idempotency-key']))
    if (req.method === 'GET' && path.startsWith('/api/orders/')) {
      const id = path.slice('/api/orders/'.length)
      if (!/^[0-9a-f-]{36}$/i.test(id)) throw new Error('Invalid order ID')
      const record = rows(await sql.query('SELECT id, status, total_minor FROM orders WHERE id = ? AND user_id = ?', [id, 'demo-user'], { maxRows: 1 }))[0]
      return reply(res, record ? 200 : 404, record || { error: 'Not found' })
    }
    if (req.method === 'GET' && path === '/api/stats') return reply(res, 200, rows(await sql.query('SELECT day, orders, gmv_minor FROM daily_stats ORDER BY day DESC LIMIT 30', [], { maxRows: 30 })))
    if (req.method === 'POST' && path === '/api/quote') {
      const body = await input(req)
      const result = await sb.raw.request('POST', `/go/${projectId}/pricing/Quote`, { body })
      return reply(res, 200, result)
    }
    if (req.method === 'GET' && path === '/api/platform') return reply(res, 200, await platform())
    if (req.method === 'POST' && /^\/api\/platform\/[a-z-]+$/.test(path)) return reply(res, 200, await platformAction(path.slice('/api/platform/'.length), await input(req)))
    if (req.method === 'POST' && path === '/api/hooks/audit') {
      const remote = req.socket.remoteAddress || ''
      if (!['127.0.0.1', '::1', '::ffff:127.0.0.1'].includes(remote)) return reply(res, 403, { error: 'Forbidden' })
      const body = await input(req)
      await sql.execute('INSERT INTO audit_log (id, source, kind, target, action, detail, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)',
        [randomUUID(), String(body.source || ''), String(body.kind || ''), String(body.target || ''), String(body.action || ''), String(body.detail || '').slice(0, 200), new Date().toISOString()])
      return reply(res, 200, { ok: true })
    }
    return reply(res, 404, { error: 'Not found' })
  } catch (error) { return reply(res, /Invalid|Insufficient|progress|transaction/.test(error.message) ? 400 : 500, { error: error.message }) }
}

// 直接用 node 运行本文件时才启动；相对路径、符号链接（如 macOS /tmp → /private/tmp）都按真实路径比较。
const isMain = (url) => { try { return Boolean(process.argv[1]) && realpathSync(process.argv[1]) === realpathSync(fileURLToPath(url)) } catch { return false } }
if (isMain(import.meta.url)) createServer(handle).listen(Number(process.env.EXAMPLE_PORT || 8092), '127.0.0.1')
