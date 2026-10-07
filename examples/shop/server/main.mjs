import { createServer } from 'node:http'
import { randomUUID } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import { createClient } from '../../../packages/js-sdk/dist/index.js'

const { SIMPLEBASE_URL: url, SIMPLEBASE_API_KEY: apiKey, SIMPLEBASE_PROJECT_ID: projectId, SIMPLEBASE_DATABASE_ID: databaseId } = process.env
if (![url, apiKey, projectId, databaseId].every(Boolean)) throw new Error('Source examples/shop/.env first')
const sb = createClient({ url, apiKey, projectId, databaseId })
const sql = sb.database(databaseId).sql
const base = `/v1/projects/${projectId}`
const rows = result => result.rows.map(row => Object.fromEntries(result.columns.map((column, index) => [column, row[index]])))
const kv = (...argvs) => sb.raw.request('POST', `${base}/kv`, { body: { type: 'cmd', argvs } })
const reply = (res, status, value) => { res.writeHead(status, { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' }); res.end(JSON.stringify(value)) }
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
    return reply(res, 404, { error: 'Not found' })
  } catch (error) { return reply(res, /Invalid|Insufficient|progress|transaction/.test(error.message) ? 400 : 500, { error: error.message }) }
}
if (process.argv[1] && import.meta.url === new URL(`file://${process.argv[1]}`).href) createServer(handle).listen(Number(process.env.EXAMPLE_PORT || 8092), '127.0.0.1')
