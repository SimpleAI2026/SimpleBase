import { createClient } from '../../../packages/js-sdk/dist/index.js'
import { readFile, readdir, lstat } from 'node:fs/promises'
import { join, relative, basename } from 'node:path'
import { createHash } from 'node:crypto'

for (const key of ['SIMPLEBASE_URL', 'SIMPLEBASE_API_KEY', 'SIMPLEBASE_PROJECT_ID']) if (!process.env[key]) throw new Error(`Missing ${key}`)
const { SIMPLEBASE_URL: url, SIMPLEBASE_API_KEY: apiKey, SIMPLEBASE_PROJECT_ID: projectId } = process.env
const sb = createClient({ url, apiKey, projectId })
const path = (suffix) => `/v1/projects/${encodeURIComponent(projectId)}${suffix}`
const root = new URL('../', import.meta.url)
const source = await readFile(new URL('functions/ex_shop_metrics.go', root), 'utf8')
const name = 'ex_shop_metrics'
const jobName = 'ex_shop_daily'

async function publishFunction() {
  let fn
  try { fn = await sb.raw.request('GET', path(`/gofunctions/${name}`)) } catch (error) { if (error.status !== 404) throw error }
  if (fn && fn.source === source && fn.active_version > 0) return
  const response = fn
    ? await sb.raw.request('POST', path(`/gofunctions/${name}/versions`), { body: { source, activate: false } })
    : await sb.raw.request('POST', path('/gofunctions'), { body: { name, source, activate: false } })
  const version = response.latest_version || response.version || response.versions?.at(-1)?.version
  if (!Number.isInteger(Number(version)) || Number(version) < 1) throw new Error('Cannot read new function version from server')
  for (const [functionName, body] of [['Tick', {}], ['Compute', { rows: [{ qty: 2, amount_minor: 4900 }] }]]) {
    const test = await sb.raw.request('POST', path(`/gofunctions/${name}/versions/${version}/test`), { body: { function_name: functionName, body } })
    if (!test.ok || test.status_code !== 200) throw new Error(`Function test failed: ${functionName}: ${test.error || test.status_code}`)
  }
  await sb.raw.request('POST', path(`/gofunctions/${name}/versions/${version}/activate`))
  console.log(`Activated ${name} v${version}`)
}

async function publishCron() {
  const jobs = await sb.raw.request('GET', path('/cron-jobs'))
  const current = jobs.jobs?.find(job => job.name === jobName)
  const config = { name: jobName, schedule_kind: 'cron', cron_expr: '0 1 * * *', func_file: name, func_export: 'Tick', input_json: '{}', enabled: true }
  if (!current) await sb.raw.request('POST', path('/cron-jobs'), { body: config })
  else if (['schedule_kind', 'cron_expr', 'func_file', 'func_export', 'input_json', 'enabled'].some(key => current[key] !== config[key])) await sb.raw.request('PATCH', path(`/cron-jobs/${current.id}`), { body: config })
  console.log(`Cron ${jobName} configured`)
}

const mime = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.json': 'application/json' }
async function files(dir) {
  const output = []
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name)
    if ((await lstat(full)).isSymbolicLink()) throw new Error('Symlinks are not allowed')
    if (entry.isDirectory()) output.push(...await files(full))
    else if (entry.isFile()) output.push(full)
  }
  return output
}
async function upload() {
  const dist = new URL('dist/', root).pathname
  const paths = await files(dist)
  const content = await Promise.all(paths.map(p => readFile(p)))
  const buildId = createHash('sha256').update(Buffer.concat(content)).digest('hex').slice(0, 16)
  const remote = `shop-service/${buildId}/`
  const existing = await sb.storage.list({ prefix: remote, refresh: true })
  if (existing.length) {
    if (existing.length !== paths.length) throw new Error('Partial build already exists; choose a new build-id')
    console.log(`Already uploaded ${remote}`)
    return
  }
  const ordered = paths.map((p, i) => ({ p, bytes: content[i] })).sort((a, b) => (basename(a.p) === 'index.html') - (basename(b.p) === 'index.html'))
  for (const { p, bytes } of ordered) {
    const part = relative(dist, p).replaceAll('\\', '/')
    if (part.startsWith('..') || part.startsWith('/')) throw new Error('Invalid asset path')
    const ext = part.slice(part.lastIndexOf('.'))
    await sb.storage.upload(remote + part, bytes, { filename: basename(p), contentType: mime[ext] || 'application/octet-stream' })
  }
  const listed = await sb.storage.list({ prefix: remote, refresh: true })
  if (listed.length !== paths.length) throw new Error('Uploaded asset count mismatch')
  console.log(`Private build objects uploaded to ${remote}`)
}

await publishFunction()
await sb.raw.request('POST', `/go/${encodeURIComponent(projectId)}/${name}/Compute`, { body: { rows: [] } })
await publishCron()
if (process.argv.includes('--upload')) await upload()
