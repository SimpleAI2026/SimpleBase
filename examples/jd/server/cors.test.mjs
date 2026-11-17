import { test, before, after } from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'node:http'

Object.assign(process.env, {
  SIMPLEBASE_URL: 'http://127.0.0.1:1',
  SIMPLEBASE_API_KEY: 'offline-test',
  SIMPLEBASE_PROJECT_ID: 'ex-jd001',
  SIMPLEBASE_DATABASE_ID: 'offline'
})
delete process.env.JD_PLATFORM_WRITE

const { handle } = await import('./main.mjs')

let server
let base
before(async () => {
  server = createServer(handle)
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  base = `http://127.0.0.1:${server.address().port}`
})
after(() => new Promise(resolve => server.close(resolve)))

test('任意 Origin 的普通请求都带 CORS 头', async () => {
  const res = await fetch(`${base}/health`, { headers: { origin: 'http://evil.example' } })
  assert.equal(res.status, 200)
  assert.equal(res.headers.get('access-control-allow-origin'), '*')
  assert.equal(res.headers.get('access-control-expose-headers'), '*')
  assert.equal(res.headers.get('access-control-allow-credentials'), null)
})

test('预检返回 204 并回显请求头', async () => {
  const res = await fetch(`${base}/api/orders`, {
    method: 'OPTIONS',
    headers: {
      origin: 'http://ex-jd001.localhost:8080',
      'access-control-request-method': 'POST',
      'access-control-request-headers': 'content-type,idempotency-key,x-custom'
    }
  })
  assert.equal(res.status, 204)
  assert.equal(res.headers.get('access-control-allow-origin'), '*')
  assert.equal(res.headers.get('access-control-allow-headers'), 'content-type,idempotency-key,x-custom')
  assert.match(res.headers.get('access-control-allow-methods'), /POST/)
  assert.match(res.headers.get('access-control-allow-methods'), /DELETE/)
  assert.equal(res.headers.get('access-control-max-age'), '600')
})

test('预检没带请求头时回退为 *', async () => {
  const res = await fetch(`${base}/api/cart`, { method: 'OPTIONS' })
  assert.equal(res.status, 204)
  assert.equal(res.headers.get('access-control-allow-headers'), '*')
})

test('404 也带 CORS 头', async () => {
  const res = await fetch(`${base}/nope`, { headers: { origin: 'http://evil.example' } })
  assert.equal(res.status, 404)
  assert.equal(res.headers.get('access-control-allow-origin'), '*')
})

test('平台写入动作默认 403，且带 CORS 头', async () => {
  const res = await fetch(`${base}/api/platform/emit-event`, {
    method: 'POST',
    headers: { origin: 'http://evil.example', 'content-type': 'application/json' },
    body: '{}'
  })
  assert.equal(res.status, 403)
  assert.equal(res.headers.get('access-control-allow-origin'), '*')
  assert.match((await res.json()).error, /JD_PLATFORM_WRITE/)
})

test('同源请求不受影响：/ 与 /config.js 可访问', async () => {
  const page = await fetch(`${base}/`)
  assert.equal(page.status, 200)
  assert.match(page.headers.get('content-type'), /text\/html/)
  const config = await fetch(`${base}/config.js`)
  assert.equal(config.status, 200)
  assert.match(config.headers.get('content-type'), /text\/javascript/)
  assert.match(await config.text(), /__SB_CONFIG__/)
})
