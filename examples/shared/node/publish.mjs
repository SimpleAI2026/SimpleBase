import { createClient } from '../../../packages/js-sdk/dist/index.js'
import { readFile, readdir, lstat } from 'node:fs/promises'
import { createHash } from 'node:crypto'
import { join, relative, basename, extname } from 'node:path'

const metadata = {
  'booking-service': { fn: 'ex_booking_expiry', job: 'ex_booking_expired', cron: '*/5 * * * *', sample: { cutoff: '', rows: [] } },
  'ticket-service': { fn: 'ex_ticket_sla', job: 'ex_ticket_sla_scan', cron: '0 * * * *', sample: { now: '', rows: [] } }
}
const name = process.argv[2]
if (!/^[a-z][a-z0-9-]{0,38}$/.test(name)) throw new Error('Specify a case name (kebab-case)')
for (const key of ['SIMPLEBASE_URL', 'SIMPLEBASE_API_KEY', 'SIMPLEBASE_PROJECT_ID']) if (!process.env[key]) throw new Error(`Missing ${key}`)
const { SIMPLEBASE_URL: url, SIMPLEBASE_API_KEY: apiKey, SIMPLEBASE_PROJECT_ID: projectId } = process.env
const sb = createClient({ url, apiKey, projectId })
const project = `/v1/projects/${encodeURIComponent(projectId)}`
const api = (method, suffix, body) => sb.raw.request(method, `${project}${suffix}`, body === undefined ? {} : { body })
const root = new URL(`../../${name}/`, import.meta.url)
let entry = metadata[name]
if (!entry) {
  try { entry = JSON.parse(await readFile(new URL('case.json', root), 'utf8')) } catch { throw new Error(`Unknown example '${name}'; add examples/${name}/case.json`) }
}
const { fn, job, cron, sample } = entry
const probe = { ...(sample ?? { rows: [] }) }
if (probe.cutoff === '') probe.cutoff = new Date().toISOString()
if (probe.now === '') probe.now = new Date().toISOString()
const source = await readFile(new URL(`functions/${fn}.go`, root), 'utf8')
let current
try { current = await api('GET', `/gofunctions/${fn}`) } catch (error) { if (error.status !== 404) throw error }
if (!current || current.source !== source || !current.active_version) {
  const created = await api('POST', current ? `/gofunctions/${fn}/versions` : '/gofunctions', { ...(current ? {} : { name: fn }), source, activate: false })
  const version = Number(created.latest_version)
  if (!Number.isInteger(version) || version < 1) throw new Error('Invalid function version response')
  for (const [functionName, input] of [['Tick', {}], ['Compute', probe]]) {
    const result = await api('POST', `/gofunctions/${fn}/versions/${version}/test`, { function_name: functionName, body: input })
    if (!result.ok || result.status_code !== 200) throw new Error(`Function test failed: ${functionName}`)
  }
  await api('POST', `/gofunctions/${fn}/versions/${version}/activate`)
}
await sb.raw.request('POST', `/go/${encodeURIComponent(projectId)}/${fn}/Compute`, { body: probe })
const jobs = await api('GET', '/cron-jobs')
const old = jobs.jobs?.find(entry => entry.name === job)
const config = { name: job, schedule_kind: 'cron', cron_expr: cron, func_file: fn, func_export: 'Tick', input_json: '{}', enabled: true }
if (!old) await api('POST', '/cron-jobs', config)
else if (['schedule_kind', 'cron_expr', 'func_file', 'func_export', 'input_json', 'enabled'].some(key => old[key] !== config[key])) await api('PATCH', `/cron-jobs/${old.id}`, config)
console.log(`Function ${fn} and cron ${job} configured`)
if (process.argv.includes('--upload')) {
  const dist = new URL('dist/', root).pathname
  async function scan(dir) {
    const output = []
    for (const entry of await readdir(dir, { withFileTypes: true })) {
      const filename = join(dir, entry.name)
      if ((await lstat(filename)).isSymbolicLink()) throw new Error('Symbolic links cannot be uploaded')
      if (entry.isDirectory()) output.push(...await scan(filename))
      else if (entry.isFile()) output.push(filename)
    }
    return output
  }
  const paths = await scan(dist)
  const digest = createHash('sha256')
  for (const file of paths.sort()) digest.update(await readFile(file))
  const prefix = `${name}/${digest.digest('hex').slice(0, 16)}/`
  const existing = await sb.storage.list({ prefix, refresh: true })
  if (existing.length && existing.length !== paths.length) throw new Error('Partial upload; manual inspection required')
  if (!existing.length) {
    const mime = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.svg': 'image/svg+xml' }
    for (const file of paths.sort((a, b) => (basename(a) === 'index.html') - (basename(b) === 'index.html'))) {
      const key = relative(dist, file).replaceAll('\\', '/')
      if (key.startsWith('..') || key.startsWith('/')) throw new Error('Invalid asset path')
      await sb.storage.upload(prefix + key, await readFile(file), { filename: basename(file), contentType: mime[extname(file)] || 'application/octet-stream' })
    }
  }
  console.log(`Private objects: ${prefix}`)
}
