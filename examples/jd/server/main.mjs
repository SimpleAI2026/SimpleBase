import { createServer } from 'node:http'
import { randomUUID } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import { realpathSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { createClient } from '../../../packages/js-sdk/dist/index.js'

const { SIMPLEBASE_URL: url, SIMPLEBASE_API_KEY: apiKey, SIMPLEBASE_PROJECT_ID: projectId, SIMPLEBASE_DATABASE_ID: databaseId } = process.env
if (![url, apiKey, projectId, databaseId].every(Boolean)) throw new Error('Source examples/jd/.env first')

const sb = createClient({ url, apiKey, projectId, databaseId })
const sql = sb.database(databaseId).sql
const events = sb.database(databaseId).collection('jd_events')
const apiBase = `/v1/projects/${projectId}`

// 本地模式固定使用演示用户，不做登录。
const USER_ID = 'user-demo-0001'
const USER_NICKNAME = '京东体验官'

const rows = result => result.rows.map(row => Object.fromEntries(result.columns.map((column, index) => [column, row[index]])))
const kv = (...argvs) => sb.raw.request('POST', `${apiBase}/kv`, { body: { type: 'cmd', argvs: argvs.map(String) } })
const callFunction = (name, exportName, input) => sb.raw.request('POST', `/go/${projectId}/${name}/${exportName}`, { body: input })
const reply = (res, status, value) => {
  res.writeHead(status, { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' })
  res.end(JSON.stringify(value))
}

async function input(req) {
  let body = ''
  for await (const chunk of req) {
    body += chunk
    if (body.length > 65536) throw new Error('Request too large')
  }
  return JSON.parse(body || '{}')
}

class BadRequest extends Error {}

function bad(message) { throw new BadRequest(message) }
function parseId(value, field) {
  if (typeof value !== 'string' || !/^[0-9a-f-]{36}$/i.test(value)) bad(`Invalid ${field}`)
  return value
}
// 购物车与地址的种子行用的是可读 ID（如 addr-demo-0001），放宽到普通主键格式。
function parseKey(value, field) {
  if (typeof value !== 'string' || !/^[A-Za-z0-9_-]{1,64}$/.test(value)) bad(`Invalid ${field}`)
  return value
}
function parseQty(value, field = 'qty') {
  const qty = Number(value)
  if (!Number.isSafeInteger(qty) || qty < 1 || qty > 99) bad(`Invalid ${field}`)
  return qty
}

// ── 商品 ────────────────────────────────────────────────

async function attachImages(items) {
  for (const item of items) {
    if (!item.image_key) continue
    const presigned = await sb.raw
      .request('GET', `${apiBase}/s3/presign?key=${encodeURIComponent(item.image_key)}`)
      .catch(() => null)
    item.image_url = presigned?.url || null
  }
  return items
}

function shapeProduct(row) {
  const now = Date.now()
  const flashPrice = Number(row.flash_price_minor) || 0
  const flashActive = Boolean(row.is_flash) && flashPrice > 0 && (!row.flash_end_at || new Date(row.flash_end_at).getTime() > now)
  return {
    id: row.id,
    sku: row.sku,
    name: row.name,
    subtitle: row.subtitle,
    category_id: row.category_id,
    price_minor: flashActive ? flashPrice : Number(row.price_minor),
    orig_price_minor: Number(row.orig_price_minor),
    stock: Number(row.stock),
    sales: Number(row.sales),
    rating: Number(row.rating),
    review_count: Number(row.review_count),
    image_key: row.image_key,
    image_url: row.image_url,
    tags: String(row.tags || '').split(',').filter(Boolean),
    specs: String(row.specs || '').split(';').filter(Boolean).map(pair => {
      const [label, value] = pair.split(':')
      return { label, value }
    }),
    flash: flashActive,
    flash_end_at: row.flash_end_at || null
  }
}

const SORTS = {
  default: 'sales DESC, review_count DESC',
  sales: 'sales DESC',
  price_asc: 'price_minor ASC',
  price_desc: 'price_minor DESC',
  rating: 'rating DESC, review_count DESC',
  newest: 'created_at DESC'
}

async function listProducts(query) {
  const category = typeof query.get('category') === 'string' ? query.get('category') : ''
  const keyword = typeof query.get('keyword') === 'string' ? query.get('keyword').trim() : ''
  const sort = SORTS[query.get('sort')] || SORTS.default
  const page = Math.max(1, Number(query.get('page')) || 1)
  const size = Math.min(24, Math.max(1, Number(query.get('size')) || 12))

  const where = []
  const args = []
  if (category) { where.push('category_id = ?'); args.push(category) }
  if (keyword) { where.push('(name LIKE ? OR subtitle LIKE ? OR tags LIKE ?)'); args.push(`%${keyword}%`, `%${keyword}%`, `%${keyword}%`) }
  const clause = where.length ? `WHERE ${where.join(' AND ')}` : ''

  const total = Number(rows(await sql.query(`SELECT COUNT(*) AS c FROM products ${clause}`, args))[0]?.c || 0)
  const items = rows(await sql.query(`SELECT * FROM products ${clause} ORDER BY ${sort} LIMIT ${size} OFFSET ${(page - 1) * size}`, args, { maxRows: size }))
  return { total, page, size, items: (await attachImages(items)).map(shapeProduct) }
}

async function productDetail(sku) {
  const row = rows(await sql.query('SELECT * FROM products WHERE sku = ?', [sku], { maxRows: 1 }))[0]
  if (!row) return null
  const [product] = (await attachImages([row])).map(shapeProduct)
  const reviews = rows(await sql.query('SELECT nickname, rating, content, created_at, image_key FROM reviews WHERE product_id = ? ORDER BY created_at DESC LIMIT 20', [row.id], { maxRows: 20 }))
  for (const review of reviews) {
    review.image_url = review.image_key
      ? (await sb.raw.request('GET', `${apiBase}/s3/presign?key=${encodeURIComponent(review.image_key)}`).catch(() => null))?.url || null
      : null
  }
  const similarRows = rows(await sql.query('SELECT * FROM products WHERE category_id = ? AND id <> ? ORDER BY sales DESC LIMIT 4', [row.category_id, row.id], { maxRows: 4 }))
  const similar = (await attachImages(similarRows)).map(shapeProduct)
  return { ...product, reviews, similar }
}

async function categories() {
  return rows(await sql.query('SELECT id, name, icon, sort_order FROM categories ORDER BY sort_order'))
}

async function hotSearch() {
  const cached = await kv('GET', 'jd:hot:search').catch(() => null)
  if (typeof cached === 'string' && cached.startsWith('[')) {
    try { return JSON.parse(cached) } catch { /* 落到默认值 */ }
  }
  return ['手机', '耳机', '笔记本', '扫地机器人']
}

async function home() {
  const [cats, hot, flash, best] = await Promise.all([
    categories(),
    hotSearch(),
    sql.query('SELECT * FROM products WHERE is_flash = TRUE ORDER BY sales DESC LIMIT 6', [], { maxRows: 6 }),
    sql.query('SELECT * FROM products ORDER BY sales DESC LIMIT 12', [], { maxRows: 12 })
  ])
  const flashRows = rows(flash)
  const bestRows = rows(best)
  const ranked = await callFunction('recommend', 'Rank', {
    seed: randomUUID().slice(0, 8),
    items: flashRows.concat(bestRows).map(item => ({
      id: item.id,
      name: item.name,
      sales: Number(item.sales),
      rating: Number(item.rating),
      price_minor: Number(item.price_minor),
      category_id: item.category_id,
      flash: Boolean(item.is_flash)
    })),
    limit: 8
  }).catch(() => ({ items: [] }))
  const byId = new Map([...flashRows, ...bestRows].map(item => [item.id, item]))
  const recommendRows = (ranked.items || []).map(item => byId.get(item.id)).filter(Boolean)
  const picked = recommendRows.length ? recommendRows : bestRows.slice(0, 8)
  return {
    categories: cats,
    hot_search: hot,
    flash: (await attachImages(flashRows)).map(shapeProduct),
    recommend: (await attachImages(picked)).map(shapeProduct),
    flash_end_at: flashRows.find(item => item.flash_end_at)?.flash_end_at || null
  }
}

// ── 购物车 ──────────────────────────────────────────────

async function cartState() {
  const items = rows(await sql.query(
    'SELECT c.id AS cart_id, c.qty, c.checked, p.* FROM cart_items c JOIN products p ON p.id = c.product_id WHERE c.user_id = ? ORDER BY c.created_at',
    [USER_ID], { maxRows: 100 }
  ))
  await attachImages(items)
  const shaped = items.map(row => {
    const product = shapeProduct(row)
    return { id: row.cart_id, qty: Number(row.qty), checked: Boolean(row.checked), available: product.stock > 0, product }
  })
  const checked = shaped.filter(item => item.checked && item.available)
  const subtotal = checked.reduce((sum, item) => sum + item.product.price_minor * item.qty, 0)
  const totalQty = shaped.reduce((sum, item) => sum + item.qty, 0)
  return { items: shaped, subtotal_minor: subtotal, total_qty: totalQty }
}

async function cartAdd(body) {
  const sku = typeof body.sku === 'string' ? body.sku : bad('Invalid sku')
  const qty = parseQty(body.qty)
  const product = rows(await sql.query('SELECT id, stock FROM products WHERE sku = ?', [sku], { maxRows: 1 }))[0]
  if (!product) bad('Product not found')
  const existing = rows(await sql.query('SELECT id, qty FROM cart_items WHERE user_id = ? AND product_id = ?', [USER_ID, product.id], { maxRows: 1 }))[0]
  if (existing) {
    const next = Math.min(99, Number(existing.qty) + qty)
    await sql.execute('UPDATE cart_items SET qty = ?, checked = TRUE WHERE id = ?', [next, existing.id])
    return { id: existing.id, qty: next }
  }
  if (Number(product.stock) < qty) bad('Insufficient stock')
  const id = randomUUID()
  await sql.execute('INSERT INTO cart_items (id, user_id, product_id, qty, checked, created_at) VALUES (?, ?, ?, ?, TRUE, ?)', [id, USER_ID, product.id, qty, new Date().toISOString()])
  return { id, qty }
}

async function cartPatch(body) {
  const id = parseKey(body.id, 'cart id')
  const checked = Boolean(body.checked)
  const result = await sql.execute('UPDATE cart_items SET checked = ? WHERE id = ? AND user_id = ?', [checked, id, USER_ID])
  if (result.rows_affected !== 1) bad('Cart item not found')
  return { id, checked }
}

async function cartQty(body) {
  const id = parseKey(body.id, 'cart id')
  const qty = parseQty(body.qty)
  const result = await sql.execute('UPDATE cart_items SET qty = ? WHERE id = ? AND user_id = ?', [qty, id, USER_ID])
  if (result.rows_affected !== 1) bad('Cart item not found')
  return { id, qty }
}

async function cartRemove(id) {
  parseKey(id, 'cart id')
  await sql.execute('DELETE FROM cart_items WHERE id = ? AND user_id = ?', [id, USER_ID])
  return { ok: true }
}

async function cartCheckAll(checked) {
  await sql.execute('UPDATE cart_items SET checked = ? WHERE user_id = ?', [Boolean(checked), USER_ID])
  return { checked: Boolean(checked) }
}

// ── 优惠券与地址 ────────────────────────────────────────

async function couponList() {
  const mine = rows(await sql.query(
    'SELECT uc.id AS user_coupon_id, uc.status, c.id, c.code, c.title, c.kind, c.value, c.min_minor, c.start_at, c.end_at FROM user_coupons uc ' +
    'JOIN coupons c ON c.id = uc.coupon_id WHERE uc.user_id = ? ORDER BY uc.claimed_at DESC',
    [USER_ID], { maxRows: 50 }
  ))
  const center = rows(await sql.query('SELECT id, code, title, kind, value, min_minor, total, remain, start_at, end_at FROM coupons WHERE remain > 0 ORDER BY value DESC', [], { maxRows: 50 }))
  const owned = new Set(mine.map(row => row.code))
  return {
    mine: mine.map(row => ({ ...row, value: Number(row.value), min_minor: Number(row.min_minor) })),
    center: center.map(row => ({ ...row, value: Number(row.value), min_minor: Number(row.min_minor), total: Number(row.total), remain: Number(row.remain), claimed: owned.has(row.code) }))
  }
}

async function couponClaim(code) {
  const coupon = rows(await sql.query('SELECT id, remain FROM coupons WHERE code = ?', [code], { maxRows: 1 }))[0]
  if (!coupon) bad('Coupon not found')
  if (Number(coupon.remain) <= 0) bad('Coupon sold out')
  const owned = rows(await sql.query('SELECT id FROM user_coupons WHERE user_id = ? AND coupon_id = ?', [USER_ID, coupon.id], { maxRows: 1 }))[0]
  if (owned) bad('Coupon already claimed')
  const id = randomUUID()
  const result = await sql.batch([
    { sql: 'UPDATE coupons SET remain = remain - 1 WHERE id = ? AND remain > 0', args: [coupon.id] },
    { sql: 'INSERT INTO user_coupons (id, user_id, coupon_id, status, order_id, claimed_at, used_at) VALUES (?, ?, ?, ?, ?, ?, ?)', args: [id, USER_ID, coupon.id, 'unused', '', new Date().toISOString(), ''] }
  ], { transactional: true })
  if (result.error || result.results?.[0]?.rows_affected !== 1) bad('Claim failed, retry')
  return { user_coupon_id: id }
}

async function addressList() {
  const list = rows(await sql.query('SELECT id, receiver, phone, province, city, district, detail, is_default FROM addresses WHERE user_id = ? ORDER BY is_default DESC, created_at', [USER_ID], { maxRows: 50 }))
  return list.map(row => ({ ...row, is_default: Boolean(row.is_default), text: `${row.province}${row.city}${row.district} ${row.detail}` }))
}

async function addressCreate(body) {
  const fields = ['receiver', 'phone', 'province', 'city', 'district', 'detail']
  for (const field of fields) if (typeof body[field] !== 'string' || !body[field].trim()) bad(`Missing ${field}`)
  const id = randomUUID()
  const isDefault = Boolean(body.is_default)
  const statements = []
  if (isDefault) statements.push({ sql: 'UPDATE addresses SET is_default = FALSE WHERE user_id = ?', args: [USER_ID] })
  statements.push({
    sql: 'INSERT INTO addresses (id, user_id, receiver, phone, province, city, district, detail, is_default, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)',
    args: [id, USER_ID, body.receiver, body.phone, body.province, body.city, body.district, body.detail, isDefault, new Date().toISOString()]
  })
  const result = await sql.batch(statements, { transactional: true })
  if (result.error) bad('Create address failed')
  return { id }
}

async function addressDefault(id) {
  parseKey(id, 'address id')
  const result = await sql.batch([
    { sql: 'UPDATE addresses SET is_default = FALSE WHERE user_id = ?', args: [USER_ID] },
    { sql: 'UPDATE addresses SET is_default = TRUE WHERE id = ? AND user_id = ?', args: [id, USER_ID] }
  ], { transactional: true })
  if (result.error || result.results?.[1]?.rows_affected !== 1) bad('Address not found')
  return { ok: true }
}

// ── 算价与下单 ──────────────────────────────────────────

function couponLabel(coupon) {
  if (!coupon) return ''
  return coupon.kind === 'percent' ? `${coupon.value}% 折扣` : `减 ¥${(Number(coupon.value) / 100).toFixed(2)}`
}

async function quote(items, userCouponId) {
  let coupon = null
  if (userCouponId) {
    coupon = rows(await sql.query(
      'SELECT uc.id, uc.status, c.kind, c.value, c.min_minor FROM user_coupons uc JOIN coupons c ON c.id = uc.coupon_id WHERE uc.id = ? AND uc.user_id = ?',
      [userCouponId, USER_ID], { maxRows: 1 }
    ))[0]
    if (!coupon || coupon.status !== 'unused') bad('Coupon unavailable')
  }
  const lines = items.map(item => ({ price_minor: item.product.price_minor, qty: item.qty }))
  const result = await callFunction('pricing', 'Quote', {
    items: lines,
    coupon_kind: coupon?.kind || '',
    coupon_value: coupon ? Number(coupon.value) : 0,
    min_minor: coupon ? Number(coupon.min_minor) : 0,
    plus_member: true
  })
  if (result.error) bad(result.error)
  return { ...result, coupon_id: coupon?.id || '', coupon_label: couponLabel(coupon) }
}

async function pricePreview(body) {
  const cart = await cartState()
  if (!cart.items.length) bad('Cart is empty')
  const selected = body.cart_ids
    ? cart.items.filter(item => body.cart_ids.includes(item.id))
    : cart.items.filter(item => item.checked)
  if (!selected.length) bad('Select at least one item')
  for (const item of selected) if (item.qty > item.product.stock) bad(`${item.product.name} 库存不足`)
  const quoted = await quote(selected, body.user_coupon_id)
  return {
    items: selected.map(item => ({ id: item.id, name: item.product.name, image_url: item.product.image_url, price_minor: item.product.price_minor, qty: item.qty })),
    subtotal_minor: quoted.total_minor,
    discount_minor: quoted.discount_minor,
    freight_minor: quoted.freight_minor,
    pay_minor: quoted.pay_minor,
    coupon_id: quoted.coupon_id,
    coupon_label: quoted.coupon_label
  }
}

async function createOrder(body, idem) {
  if (!/^[0-9a-f-]{36}$/i.test(idem || '')) bad('Invalid Idempotency-Key (UUID required)')
  const previous = rows(await sql.query('SELECT id, pay_minor FROM orders WHERE idem_key = ?', [idem], { maxRows: 1 }))[0]
  if (previous) return { id: previous.id, pay_minor: Number(previous.pay_minor), replay: true }

  const cart = await cartState()
  const selected = body.cart_ids ? cart.items.filter(item => body.cart_ids.includes(item.id)) : cart.items.filter(item => item.checked)
  if (!selected.length) bad('Select at least one item')
  for (const item of selected) if (item.qty > item.product.stock) bad(`${item.product.name} 库存不足`)

  const address = rows(await sql.query('SELECT id, province, city, district, detail, receiver, phone FROM addresses WHERE id = ? AND user_id = ?', [body.address_id, USER_ID], { maxRows: 1 }))[0]
  if (!address) bad('Address not found')

  const reservation = await kv('SET', `jd:order:idem:${idem}`, '1', 'NX', 'EX', '600').catch(() => null)
  if (reservation === null) {
    const again = rows(await sql.query('SELECT id, pay_minor FROM orders WHERE idem_key = ?', [idem], { maxRows: 1 }))[0]
    if (again) return { id: again.id, pay_minor: Number(again.pay_minor), replay: true }
    bad('Order in progress; retry')
  }

  try {
    const quoted = await quote(selected, body.user_coupon_id)
    const orderId = randomUUID()
    const now = new Date().toISOString()
    const addressText = `${address.province}${address.city}${address.district} ${address.detail} ${address.receiver} ${address.phone}`
    const statements = []
    for (const item of selected) {
      statements.push({ sql: 'UPDATE products SET stock = stock - ? WHERE id = ? AND stock >= ?', args: [item.qty, item.product.id, item.qty] })
    }
    statements.push({
      sql: 'INSERT INTO orders (id, user_id, status, subtotal_minor, discount_minor, freight_minor, pay_minor, address_id, address_text, coupon_id, idem_key, remark, created_at, paid_at, canceled_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)',
      args: [orderId, USER_ID, 'pending', quoted.total_minor, quoted.discount_minor, quoted.freight_minor, quoted.pay_minor, address.id, addressText, quoted.coupon_id, idem, String(body.remark || '').slice(0, 100), now, '', '']
    })
    for (const item of selected) {
      statements.push({
        sql: 'INSERT INTO order_items (order_id, product_id, sku, name, price_minor, qty, image_key) VALUES (?, ?, ?, ?, ?, ?, ?)',
        args: [orderId, item.product.id, item.product.sku, item.product.name, item.product.price_minor, item.qty, item.product.image_key]
      })
      statements.push({ sql: 'UPDATE products SET sales = sales + ? WHERE id = ?', args: [item.qty, item.product.id] })
    }
    if (quoted.coupon_id) {
      statements.push({ sql: 'UPDATE user_coupons SET status = ?, order_id = ?, used_at = ? WHERE id = ? AND status = ?', args: ['locked', orderId, now, quoted.coupon_id, 'unused'] })
    }
    for (const item of selected) {
      statements.push({ sql: 'DELETE FROM cart_items WHERE id = ?', args: [item.id] })
    }
    const result = await sql.batch(statements, { transactional: true })
    if (result.error) bad('下单失败，库存可能已变化，请重试')
    const failed = (result.results || []).findIndex(item => item.error_code)
    if (failed >= 0) bad('下单失败，库存可能已变化，请重试')
    if (statements.slice(0, selected.length).some((_, index) => result.results?.[index]?.rows_affected !== 1)) bad('库存不足，请调整数量')
    await emitEvent('order', 'create', `${orderId} ¥${(quoted.pay_minor / 100).toFixed(2)} ${selected.length} 件`)
    return { id: orderId, pay_minor: quoted.pay_minor, subtotal_minor: quoted.total_minor, discount_minor: quoted.discount_minor, freight_minor: quoted.freight_minor }
  } catch (error) {
    await kv('DEL', `jd:order:idem:${idem}`).catch(() => {})
    throw error
  }
}

async function orderList(status) {
  const args = [USER_ID]
  let clause = 'WHERE user_id = ?'
  if (status && status !== 'all') { clause += ' AND status = ?'; args.push(status) }
  const orders = rows(await sql.query(`SELECT * FROM orders ${clause} ORDER BY created_at DESC LIMIT 50`, args, { maxRows: 50 }))
  for (const order of orders) {
    const items = rows(await sql.query('SELECT product_id, sku, name, price_minor, qty, image_key FROM order_items WHERE order_id = ?', [order.id], { maxRows: 50 }))
    await attachImages(items)
    order.items = items.map(item => ({ ...item, price_minor: Number(item.price_minor), qty: Number(item.qty) }))
    order.pay_minor = Number(order.pay_minor)
    order.subtotal_minor = Number(order.subtotal_minor)
    order.discount_minor = Number(order.discount_minor)
    order.freight_minor = Number(order.freight_minor)
  }
  return orders
}

async function orderDetail(id) {
  parseId(id, 'order id')
  const order = rows(await sql.query('SELECT * FROM orders WHERE id = ? AND user_id = ?', [id, USER_ID], { maxRows: 1 }))[0]
  if (!order) return null
  const items = rows(await sql.query('SELECT product_id, sku, name, price_minor, qty, image_key FROM order_items WHERE order_id = ?', [id], { maxRows: 50 }))
  await attachImages(items)
  return {
    ...order,
    pay_minor: Number(order.pay_minor),
    subtotal_minor: Number(order.subtotal_minor),
    discount_minor: Number(order.discount_minor),
    freight_minor: Number(order.freight_minor),
    items: items.map(item => ({ ...item, price_minor: Number(item.price_minor), qty: Number(item.qty) }))
  }
}

async function orderPay(id) {
  parseId(id, 'order id')
  const now = new Date().toISOString()
  const result = await sql.execute('UPDATE orders SET status = ?, paid_at = ? WHERE id = ? AND user_id = ? AND status = ?', ['paid', now, id, USER_ID, 'pending'])
  if (result.rows_affected !== 1) bad('Order not payable')
  const order = rows(await sql.query('SELECT coupon_id FROM orders WHERE id = ?', [id], { maxRows: 1 }))[0]
  if (order?.coupon_id) await sql.execute('UPDATE user_coupons SET status = ? WHERE order_id = ? AND status = ?', ['used', id, 'locked'])
  await emitEvent('order', 'pay', id)
  return { id, status: 'paid' }
}

async function orderComplete(id) {
  parseId(id, 'order id')
  const result = await sql.execute('UPDATE orders SET status = ? WHERE id = ? AND user_id = ? AND status = ?', ['completed', id, USER_ID, 'paid'])
  if (result.rows_affected !== 1) bad('Order not receivable')
  await emitEvent('order', 'complete', id)
  return { id, status: 'completed' }
}

async function orderCancel(id) {
  parseId(id, 'order id')
  const order = rows(await sql.query('SELECT status, coupon_id FROM orders WHERE id = ? AND user_id = ?', [id, USER_ID], { maxRows: 1 }))[0]
  if (!order) bad('Order not found')
  if (!['pending', 'paid'].includes(order.status)) bad('Order cannot be canceled')
  const items = rows(await sql.query('SELECT product_id, qty FROM order_items WHERE order_id = ?', [id], { maxRows: 50 }))
  const statements = [{ sql: 'UPDATE orders SET status = ?, canceled_at = ? WHERE id = ? AND status IN (?, ?)', args: ['canceled', new Date().toISOString(), id, 'pending', 'paid'] }]
  for (const item of items) {
    statements.push({ sql: 'UPDATE products SET stock = stock + ?, sales = CASE WHEN sales >= ? THEN sales - ? ELSE 0 END WHERE id = ?', args: [Number(item.qty), Number(item.qty), Number(item.qty), item.product_id] })
  }
  if (order.coupon_id) statements.push({ sql: 'UPDATE user_coupons SET status = ?, order_id = ?, used_at = ? WHERE order_id = ? AND status IN (?, ?)', args: ['unused', '', '', id, 'locked', 'used'] })
  const result = await sql.batch(statements, { transactional: true })
  if (result.error || result.results?.[0]?.rows_affected !== 1) bad('Cancel failed')
  await emitEvent('order', 'cancel', id)
  return { id, status: 'canceled' }
}

// ── 收藏与评价 ──────────────────────────────────────────

async function favoriteList() {
  const list = rows(await sql.query(
    'SELECT p.*, f.id AS favorite_id, f.created_at AS favorited_at FROM favorites f JOIN products p ON p.id = f.product_id WHERE f.user_id = ? ORDER BY f.created_at DESC',
    [USER_ID], { maxRows: 50 }
  ))
  return (await attachImages(list)).map(row => ({ favorite_id: row.favorite_id, favorited_at: row.favorited_at, ...shapeProduct(row) }))
}

async function favoriteToggle(body) {
  const sku = typeof body.sku === 'string' ? body.sku : bad('Invalid sku')
  const product = rows(await sql.query('SELECT id FROM products WHERE sku = ?', [sku], { maxRows: 1 }))[0]
  if (!product) bad('Product not found')
  const existing = rows(await sql.query('SELECT id FROM favorites WHERE user_id = ? AND product_id = ?', [USER_ID, product.id], { maxRows: 1 }))[0]
  if (existing) {
    await sql.execute('DELETE FROM favorites WHERE id = ?', [existing.id])
    return { favorited: false }
  }
  await sql.execute('INSERT INTO favorites (id, user_id, product_id, created_at) VALUES (?, ?, ?, ?)', [randomUUID(), USER_ID, product.id, new Date().toISOString()])
  return { favorited: true }
}

async function reviewCreate(body) {
  const sku = typeof body.sku === 'string' ? body.sku : bad('Invalid sku')
  const rating = Number(body.rating)
  if (!Number.isInteger(rating) || rating < 1 || rating > 5) bad('Invalid rating')
  const content = String(body.content || '').trim()
  if (content.length < 2 || content.length > 300) bad('内容长度需在 2-300 字之间')
  const imageKey = typeof body.image_key === 'string' ? body.image_key : ''
  if (imageKey && !/^jd\/reviews\/[A-Za-z0-9_-]+\.(svg|png|jpg|jpeg|webp)$/.test(imageKey)) bad('Invalid image key')
  const product = rows(await sql.query('SELECT id, review_count FROM products WHERE sku = ?', [sku], { maxRows: 1 }))[0]
  if (!product) bad('Product not found')
  const result = await sql.batch([
    { sql: 'INSERT INTO reviews (id, product_id, user_id, nickname, rating, content, created_at, image_key) VALUES (?, ?, ?, ?, ?, ?, ?, ?)', args: [randomUUID(), product.id, USER_ID, USER_NICKNAME, rating, content, new Date().toISOString(), imageKey] },
    { sql: 'UPDATE products SET review_count = review_count + 1 WHERE id = ?', args: [product.id] }
  ], { transactional: true })
  if (result.error) bad('Review failed')
  const avg = rows(await sql.query('SELECT AVG(rating) AS avg_rating FROM reviews WHERE product_id = ?', [product.id]))[0]
  if (avg?.avg_rating != null) await sql.execute('UPDATE products SET rating = ? WHERE id = ?', [Math.round(Number(avg.avg_rating) * 10) / 10, product.id])
  return { ok: true, image_key: imageKey }
}

// 评价晒图：原始字节上传到 jd/reviews/，对象事件触发器会把这次写入记进审计。
async function uploadReviewImage(req) {
  const key = `jd/reviews/${Date.now()}-${randomUUID().slice(0, 8)}.svg`
  const chunks = []
  let size = 0
  for await (const chunk of req) {
    size += chunk.length
    if (size > 512 * 1024) bad('Image too large (512KB max)')
    chunks.push(chunk)
  }
  if (!size) bad('Empty body')
  const buffer = Buffer.concat(chunks)
  const text = buffer.toString('utf8')
  if (!text.includes('<svg') && !text.startsWith('\x89PNG') && !text.startsWith('\xff\xd8')) bad('Only svg/png/jpg accepted')
  const meta = await sb.storage.upload(key, buffer, { contentType: text.includes('<svg') ? 'image/svg+xml' : 'application/octet-stream', filename: key.split('/').pop() })
  return { key, size: meta.size ?? size }
}

async function stats() {
  const daily = rows(await sql.query('SELECT day, orders, gmv_minor, buyers FROM daily_stats ORDER BY day DESC LIMIT 14', [], { maxRows: 14 }))
  const realtime = rows(await sql.query('SELECT status, COUNT(*) AS c FROM orders GROUP BY status'))[0]
  const summary = rows(await sql.query("SELECT COUNT(*) AS orders, COALESCE(SUM(pay_minor), 0) AS gmv FROM orders WHERE status IN ('paid', 'completed')"))[0]
  return {
    daily: daily.map(row => ({ ...row, orders: Number(row.orders), gmv_minor: Number(row.gmv_minor), buyers: Number(row.buyers) })),
    summary: { orders: Number(summary?.orders || 0), gmv_minor: Number(summary?.gmv || 0), status: realtime?.status || '' }
  }
}

// 文档写入会触发 record_change 事件，云函数 audit.OnRecord 再转发回 BFF 落 audit_log。
async function emitEvent(kind, action, detail) {
  try {
    await events.insert({ kind, action, detail: String(detail).slice(0, 200), at: new Date().toISOString() })
  } catch (error) {
    console.warn('emitEvent failed:', error.message)
  }
}

// ── 触发器回调 ───────────────────────────────────────
// 云函数 audit.OnRecord / OnObject / OnHttp 把事件转发到这里落库，供页面与 CLI 核对。
async function recordAudit(body) {
  const source = String(body.source || 'record_change').slice(0, 32)
  const kind = String(body.kind || '').slice(0, 32)
  const target = String(body.target || '').slice(0, 128)
  const action = String(body.action || '').slice(0, 32)
  const recordId = String(body.record_id || '').slice(0, 64)
  const key = String(body.key || '').slice(0, 256)
  const detail = String(body.detail || '').slice(0, 256)
  await sql.execute(
    'INSERT INTO audit_log (id, source, kind, target, action, detail, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)',
    [randomUUID(), source, kind, target, action, detail || `${recordId}${key}`, new Date().toISOString()]
  )
  return { ok: true }
}

async function auditList() {
  const list = rows(await sql.query('SELECT source, kind, action, detail, created_at FROM audit_log ORDER BY created_at DESC LIMIT 30', [], { maxRows: 30 }))
  const total = Number(rows(await sql.query('SELECT COUNT(*) AS c FROM audit_log'))[0]?.c || 0)
  const bySource = rows(await sql.query('SELECT source, COUNT(*) AS c FROM audit_log GROUP BY source'))
  const calls = {}
  for (const row of bySource) calls[row.source] = Number(row.c)
  return { items: list, total, calls }
}

// 平台能力总览：KV、对象、函数、定时任务、事件触发器，加上审计轨迹。
async function platform() {
  const [objects, functions, jobs, audit] = await Promise.all([
    sb.storage.list({ refresh: true }).catch(() => []),
    sb.raw.request('GET', `${apiBase}/gofunctions`).catch(() => ({ functions: [] })),
    sb.raw.request('GET', `${apiBase}/cron-jobs`).catch(() => ({ jobs: [] })),
    auditList()
  ])

  // KEYS 未实现，用 SCAN 游标遍历（服务端返回 [cursor, keys]）。
  const keys = []
  let cursor = '0'
  for (let page = 0; page < 10 && keys.length < 40; page++) {
    const reply = await kv('SCAN', cursor, 'MATCH', '*', 'COUNT', '50').catch(() => null)
    if (!Array.isArray(reply)) break
    cursor = String(reply[0] ?? '0')
    for (const key of reply[1] || []) keys.push(key)
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

  // 触发器管理面要求 project:admin，运行时 Key 读不到；改为读初始化脚本写下的清单快照，
  // 再按审计轨迹里的 source 统计实际调用次数。
  const snapshot = await readTriggersSnapshot()
  let triggers = snapshot.triggers
  const dropped = snapshot.dropped
  triggers = triggers.map(item => ({ ...item, calls: audit.calls[item.export_source || item.kind] || 0 }))

  const jobList = jobs.jobs || []
  const lastRuns = {}
  for (const job of jobList) {
    const history = await sb.raw.request('GET', `${apiBase}/cron-jobs/${job.id}/runs?limit=1`).catch(() => ({ runs: [] }))
    lastRuns[job.name] = history.runs?.[0]?.status || ''
  }

  return {
    kv: kvRows,
    objects: (objects || []).map(item => ({ key: item.key, size: item.size })),
    functions: (functions.functions || []).map(fn => ({ name: fn.name, latest_version: fn.latest_version, published: fn.published, exports: fn.exports })),
    cron_jobs: jobList.map(job => ({
      name: job.name,
      schedule_kind: job.schedule_kind,
      cron_expr: job.cron_expr,
      interval_seconds: job.interval_seconds,
      run_at: job.run_at,
      func_file: job.func_file,
      func_export: job.func_export,
      last_run_status: lastRuns[job.name]
    })),
    triggers,
    dropped,
    audit
  }
}

// 触发器管理面要求 project:admin，运行时 Key 读不到，改读初始化脚本写下的快照。
async function readTriggersSnapshot() {
  try {
    const snapshot = JSON.parse(await readFile(new URL('../.simplebase/triggers.json', import.meta.url), 'utf8'))
    return { triggers: snapshot.triggers || [], dropped: snapshot.dropped || 0 }
  } catch {
    return { triggers: [], dropped: 0 }
  }
}

// 平台演示动作：写文档事件、上传对象、invoke HTTP 触发器、手动触发定时任务。
async function platformAction(action, body) {
  if (action === 'emit-event') {
    await emitEvent('demo', 'create', body.detail || '平台页手动写入的演示事件')
    return { ok: true }
  }
  if (action === 'upload') {
    const key = typeof body.key === 'string' && body.key.startsWith('jd/') ? body.key : bad('key 必须以 jd/ 开头')
    if (!/^jd\/[A-Za-z0-9_.\-/]{1,120}$/.test(key)) bad('Invalid key')
    const content = String(body.content || `示例对象 ${new Date().toISOString()}`)
    await sb.storage.upload(key, content, { contentType: typeof content === 'string' && content.includes('<svg') ? 'image/svg+xml' : 'text/plain' })
    return { ok: true, key }
  }
  if (action === 'invoke-trigger') {
    // 管理面需要 project:admin，运行时 Key 读不到列表；从初始化脚本写下的快照取 ID。
    const snapshot = await readTriggersSnapshot()
    const httpTrigger = snapshot.triggers.find(item => item.kind === 'http' && item.enabled)
    if (!httpTrigger?.id) bad('没有启用的 HTTP 触发器')
    const result = await sb.raw.request('POST', `${apiBase}/function-triggers/${httpTrigger.id}/invoke`, { body: { source: 'http', detail: '平台页手动 invoke' } })
    return { ok: true, ...result }
  }
  if (action === 'run-cron') {
    const jobs = (await sb.raw.request('GET', `${apiBase}/cron-jobs`)).jobs || []
    const job = jobs.find(item => item.name === 'jd_daily') || jobs[0]
    if (!job) bad('没有定时任务')
    await sb.raw.request('POST', `${apiBase}/cron-jobs/${job.id}/trigger`, { body: {} })
    return { ok: true, name: job.name }
  }
  bad('Unknown action')
}

// ── 路由 ────────────────────────────────────────────────

async function handle(req, res) {
  try {
    const parsed = new URL(req.url, 'http://localhost')
    const path = parsed.pathname
    const query = parsed.searchParams

    if (req.method === 'GET' && (path === '/' || path === '/index.html')) {
      res.writeHead(200, { 'content-type': 'text/html; charset=utf-8' })
      return res.end(await readFile(new URL('../web/index.html', import.meta.url)))
    }
    if (req.method === 'GET' && path === '/health') return reply(res, 200, { ok: true })
    if (req.headers.origin && (new URL(req.headers.origin).host !== req.headers.host || req.headers['sec-fetch-site'] === 'cross-site')) bad('Invalid origin')

    if (req.method === 'GET' && path === '/api/home') return reply(res, 200, await home())
    if (req.method === 'GET' && path === '/api/categories') return reply(res, 200, await categories())
    if (req.method === 'GET' && path === '/api/products') return reply(res, 200, await listProducts(query))
    if (req.method === 'GET' && path.startsWith('/api/products/')) {
      const detail = await productDetail(decodeURIComponent(path.slice('/api/products/'.length)))
      return reply(res, detail ? 200 : 404, detail || { error: 'Not found' })
    }

    if (req.method === 'GET' && path === '/api/cart') return reply(res, 200, await cartState())
    if (req.method === 'POST' && path === '/api/cart') return reply(res, 201, await cartAdd(await input(req)))
    if (req.method === 'POST' && path === '/api/cart/check-all') return reply(res, 200, await cartCheckAll((await input(req)).checked))
    if (req.method === 'PATCH' && path === '/api/cart') {
      const body = await input(req)
      if (body.qty !== undefined) return reply(res, 200, await cartQty(body))
      return reply(res, 200, await cartPatch(body))
    }
    if (req.method === 'DELETE' && path === '/api/cart') return reply(res, 200, await cartRemove(query.get('id')))

    if (req.method === 'GET' && path === '/api/coupons') return reply(res, 200, await couponList())
    if (req.method === 'POST' && path.startsWith('/api/coupons/')) {
      return reply(res, 201, await couponClaim(decodeURIComponent(path.slice('/api/coupons/'.length))))
    }

    if (req.method === 'GET' && path === '/api/addresses') return reply(res, 200, await addressList())
    if (req.method === 'POST' && path === '/api/addresses') return reply(res, 201, await addressCreate(await input(req)))
    if (req.method === 'POST' && path.startsWith('/api/addresses/') && path.endsWith('/default')) {
      return reply(res, 200, await addressDefault(path.slice('/api/addresses/'.length, -'/default'.length)))
    }

    if (req.method === 'POST' && path === '/api/price') return reply(res, 200, await pricePreview(await input(req)))
    if (req.method === 'POST' && path === '/api/orders') {
      const body = await input(req)
      return reply(res, 201, await createOrder(body, req.headers['idempotency-key']))
    }
    if (req.method === 'GET' && path === '/api/orders') return reply(res, 200, await orderList(query.get('status')))
    if (req.method === 'GET' && path.startsWith('/api/orders/')) {
      const detail = await orderDetail(path.slice('/api/orders/'.length))
      return reply(res, detail ? 200 : 404, detail || { error: 'Not found' })
    }
    if (req.method === 'POST' && /^\/api\/orders\/[^/]+\/pay$/.test(path)) {
      return reply(res, 200, await orderPay(path.split('/')[3]))
    }
    if (req.method === 'POST' && /^\/api\/orders\/[^/]+\/complete$/.test(path)) {
      return reply(res, 200, await orderComplete(path.split('/')[3]))
    }
    if (req.method === 'POST' && /^\/api\/orders\/[^/]+\/cancel$/.test(path)) {
      return reply(res, 200, await orderCancel(path.split('/')[3]))
    }

    if (req.method === 'GET' && path === '/api/favorites') return reply(res, 200, await favoriteList())
    if (req.method === 'POST' && path === '/api/favorites') return reply(res, 200, await favoriteToggle(await input(req)))

    if (req.method === 'POST' && path === '/api/reviews') return reply(res, 201, await reviewCreate(await input(req)))
    if (req.method === 'POST' && path === '/api/reviews/image') return reply(res, 201, await uploadReviewImage(req))
    if (req.method === 'GET' && path === '/api/stats') return reply(res, 200, await stats())
    if (req.method === 'GET' && path === '/api/audit') return reply(res, 200, await auditList())
    if (req.method === 'GET' && path === '/api/platform') return reply(res, 200, await platform())
    if (req.method === 'POST' && /^\/api\/platform\/[a-z-]+$/.test(path)) {
      return reply(res, 200, await platformAction(path.slice('/api/platform/'.length), await input(req)))
    }
    // 触发器回调：只接受本机来源，避免外部伪造审计。
    if (req.method === 'POST' && path === '/api/hooks/audit') {
      const remote = req.socket.remoteAddress || ''
      if (!['127.0.0.1', '::1', '::ffff:127.0.0.1'].includes(remote)) return reply(res, 403, { error: 'Forbidden' })
      return reply(res, 200, await recordAudit(await input(req)))
    }

    return reply(res, 404, { error: 'Not found' })
  } catch (error) {
    if (error instanceof BadRequest) return reply(res, 400, { error: error.message })
    return reply(res, 500, { error: error.message })
  }
}

const isMain = (moduleUrl) => {
  try { return Boolean(process.argv[1]) && realpathSync(process.argv[1]) === realpathSync(fileURLToPath(moduleUrl)) } catch { return false }
}
if (isMain(import.meta.url)) {
  createServer(handle).listen(Number(process.env.JD_PORT || 8093), '127.0.0.1', () => {
    console.log('京东示例 BFF: http://127.0.0.1:' + (process.env.JD_PORT || 8093))
  })
}