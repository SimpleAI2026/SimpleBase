import { createServer } from 'node:http'
import { randomUUID } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import { extname } from 'node:path'
import { createClient } from '../../../packages/js-sdk/dist/index.js'
import { parseOrder } from './validation.mjs'
import { startWorker } from './worker.mjs'

const required = ['SIMPLEBASE_URL', 'SIMPLEBASE_API_KEY', 'SIMPLEBASE_PROJECT_ID', 'SIMPLEBASE_DATABASE_ID']
for (const key of required) if (!process.env[key]) throw new Error(`Missing ${key}`)
const { SIMPLEBASE_URL: url, SIMPLEBASE_API_KEY: apiKey, SIMPLEBASE_PROJECT_ID: projectId, SIMPLEBASE_DATABASE_ID: databaseId } = process.env
const sb = createClient({ url, apiKey, projectId, databaseId })
const sql = sb.database(databaseId).sql
const prefix = `/v1/projects/${encodeURIComponent(projectId)}`
const kv = async (...argvs) => sb.raw.request('POST', `${prefix}/kv`, { body: { type: 'cmd', argvs: argvs.map(String) } })

export async function init() {
  for (const stmt of [
    'CREATE TABLE IF NOT EXISTS ex_shop_products (id VARCHAR PRIMARY KEY, name VARCHAR NOT NULL, price_minor BIGINT NOT NULL, stock BIGINT NOT NULL CHECK (stock >= 0))',
    'CREATE TABLE IF NOT EXISTS ex_shop_orders (id VARCHAR PRIMARY KEY, owner_id VARCHAR NOT NULL, total_minor BIGINT NOT NULL, created_at VARCHAR NOT NULL)',
    'CREATE TABLE IF NOT EXISTS ex_shop_order_items (order_id VARCHAR NOT NULL, product_id VARCHAR NOT NULL, qty BIGINT NOT NULL, price_minor BIGINT NOT NULL, PRIMARY KEY (order_id, product_id))',
    'CREATE TABLE IF NOT EXISTS ex_shop_daily_stats (window_start VARCHAR PRIMARY KEY, units BIGINT NOT NULL, amount_minor BIGINT NOT NULL, source_run_id VARCHAR NOT NULL UNIQUE)',
    'CREATE TABLE IF NOT EXISTS ex_shop_processed_runs (run_id VARCHAR PRIMARY KEY, window_start VARCHAR NOT NULL)'
  ]) await sql.execute(stmt)
  for (const [id, name, price] of [['tea', '山野乌龙', 4900], ['lamp', '纸艺台灯', 12900], ['book', '城市手帖', 3900]]) {
    await sql.execute('INSERT INTO ex_shop_products SELECT ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM ex_shop_products WHERE id = ?)', [id, name, price, 30, id])
  }
}

const rows = (result) => result.rows.map(row => Object.fromEntries(result.columns.map((column, i) => [column, row[i]])))
async function products() { return rows(await sql.query('SELECT id, name, price_minor, stock FROM ex_shop_products ORDER BY id', [], { maxRows: 100 })) }

async function order(input) {
  const { productId, qty } = parseOrder(input)
  const found = rows(await sql.query('SELECT price_minor, stock FROM ex_shop_products WHERE id = ?', [productId], { maxRows: 1 }))[0]
  if (!found || Number(found.stock) < qty) throw new Error('Insufficient stock')
  const id = randomUUID()
  const timestamp = new Date().toISOString()
  const result = await sql.batch([
    { sql: 'UPDATE ex_shop_products SET stock = stock - ? WHERE id = ?', args: [qty, productId] },
    { sql: 'INSERT INTO ex_shop_orders (id, owner_id, total_minor, created_at) VALUES (?, ?, ?, ?)', args: [id, 'local-demo', Number(found.price_minor) * qty, timestamp] },
    { sql: 'INSERT INTO ex_shop_order_items (order_id, product_id, qty, price_minor) VALUES (?, ?, ?, ?)', args: [id, productId, qty, Number(found.price_minor)] }
  ], { transactional: true })
  if (result.error || result.results?.some(item => item.error_code)) throw new Error('Order transaction failed')
  if (result.results?.[0]?.rows_affected !== 1) throw new Error('Insufficient stock')
  await kv('DEL', 'ex:shop:popular').catch(() => {})
  return { id, productId, qty, totalMinor: Number(found.price_minor) * qty }
}

async function body(req) {
  let text = ''
  for await (const chunk of req) {
    text += chunk
    if (text.length > 8192) throw new Error('Request too large')
  }
  return JSON.parse(text || '{}')
}
const reply = (res, status, data) => { res.writeHead(status, { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' }); res.end(JSON.stringify(data)) }

export async function handle(req, res) {
  try {
    const path = new URL(req.url, 'http://localhost').pathname
    if (req.method === 'GET' && ['/', '/index.html', '/app.js', '/style.css'].includes(path)) {
      const file = path === '/' ? 'index.html' : path.slice(1)
      const bytes = await readFile(new URL(`../dist/${file}`, import.meta.url))
      res.writeHead(200, { 'content-type': extname(file) === '.html' ? 'text/html; charset=utf-8' : extname(file) === '.css' ? 'text/css' : 'text/javascript', 'x-content-type-options': 'nosniff' })
      return res.end(bytes)
    }
    if (path === '/health' && req.method === 'GET') return reply(res, 200, { ok: true })
    if (!req.headers.origin || new URL(req.headers.origin).host !== req.headers.host || req.headers['sec-fetch-site'] === 'cross-site') throw new Error('Invalid origin; local demo only')
    if (path === '/api/products' && req.method === 'GET') return reply(res, 200, await products())
    if (path === '/api/cart' && req.method === 'PUT') {
      const input = parseOrder(await body(req))
      await kv('SET', 'ex:shop:cart:local-demo', JSON.stringify(input), 'EX', '1800')
      return reply(res, 200, input)
    }
    if (path === '/api/cart' && req.method === 'GET') return reply(res, 200, JSON.parse(await kv('GET', 'ex:shop:cart:local-demo') || 'null'))
    if (path === '/api/orders' && req.method === 'POST') return reply(res, 201, await order(await body(req)))
    if (path.startsWith('/api/orders/') && req.method === 'GET') {
      const id = path.slice('/api/orders/'.length)
      if (!/^[0-9a-f-]{36}$/.test(id)) return reply(res, 400, { error: 'Invalid ID' })
      const found = rows(await sql.query('SELECT id, total_minor, created_at FROM ex_shop_orders WHERE id = ? AND owner_id = ?', [id, 'local-demo'], { maxRows: 1 }))[0]
      return reply(res, found ? 200 : 404, found || { error: 'Not found' })
    }
    return reply(res, 404, { error: 'Not found' })
  } catch (error) {
    return reply(res, error instanceof SyntaxError || /Invalid|large|Insufficient/.test(error.message) ? 400 : 500, { error: error instanceof SyntaxError ? 'Invalid JSON' : error.message })
  }
}

if (process.argv[1] && import.meta.url === new URL(`file://${process.argv[1]}`).href) {
  await init()
  if (process.env.EXAMPLE_WORKER === '1') startWorker({ sb, sql, projectId, onError: message => console.error(message) })
  const port = Number(process.env.EXAMPLE_PORT || 8092)
  createServer(handle).listen(port, '127.0.0.1', () => console.log(`Shop demo listening on http://127.0.0.1:${port}`))
}
