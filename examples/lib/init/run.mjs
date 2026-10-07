import { createHash } from 'node:crypto'
import { readFile, readdir, writeFile, chmod } from 'node:fs/promises'
import { join, resolve } from 'node:path'
import { promisify } from 'node:util'
import { execFile } from 'node:child_process'

const runCommand = promisify(execFile)

const root = resolve(import.meta.dirname, '../..')
const options = { url: process.env.SIMPLEBASE_URL || 'http://127.0.0.1:8080', user: process.env.SIMPLEBASE_ADMIN_USER || 'simplebase2026', password: process.env.SIMPLEBASE_ADMIN_PASSWORD || 'simplebase2026', only: '', prefix: 'ex', skipBuild: false, resetData: false, dryRun: false }
for (let i = 2; i < process.argv.length; i++) {
  const arg = process.argv[i]
  if (['--url', '--user', '--password', '--only', '--project-prefix'].includes(arg)) {
    if (!process.argv[i + 1]) throw new Error(`${arg} needs a value`)
    options[{ '--url': 'url', '--user': 'user', '--password': 'password', '--only': 'only', '--project-prefix': 'prefix' }[arg]] = process.argv[++i]
  } else if (['--skip-build', '--reset-data', '--dry-run'].includes(arg)) options[{ '--skip-build': 'skipBuild', '--reset-data': 'resetData', '--dry-run': 'dryRun' }[arg]] = true
  else throw new Error(`Unknown option ${arg}`)
}
options.url = options.url.replace(/\/$/, '')
const cases = options.only ? options.only.split(',') : ['iot-telemetry', 'shop', 'community', 'ops-assistant']
const allowed = new Set(['iot-telemetry', 'shop', 'community', 'ops-assistant'])
if (cases.some(name => !allowed.has(name))) throw new Error('Unknown case in --only')
if (!/^[A-Za-z0-9-]{1,3}$/.test(options.prefix)) throw new Error('Invalid --project-prefix (1-3 alphanumeric/hyphen characters)')
const projectID = id => options.prefix === 'ex' ? id : `${options.prefix}-${id.slice(3).padEnd(7 - options.prefix.length, '0').slice(0, 7 - options.prefix.length)}`

let access, refresh, expires = 0
async function request(method, path, body, auth = true) {
  if (auth && refresh && Date.now() > expires - 30 * 60_000) {
    const pair = await request('POST', '/v1/auth/refresh', { refresh_token: refresh }, false)
    access = pair.access_token; refresh = pair.refresh_token; expires = Date.now() + pair.expires_in * 1000
  }
  const headers = auth ? { authorization: `Bearer ${access}` } : {}
  if (body !== undefined && !(body instanceof FormData)) headers['content-type'] = 'application/json'
  const res = await fetch(`${options.url}${path}`, { method, headers, body: body === undefined ? undefined : body instanceof FormData ? body : JSON.stringify(body), signal: AbortSignal.timeout(30_000) })
  const text = await res.text()
  let data
  try { data = text ? JSON.parse(text) : null } catch { data = text }
  if (!res.ok) throw new Error(`${method} ${path}: HTTP ${res.status} ${JSON.stringify(data?.error || data).slice(0, 250)}`)
  return data
}
const scoped = (id, suffix) => `/v1/projects/${id}${suffix}`
const sql = (id, db, action, body) => request('POST', scoped(id, `/databases/${db}/${action}`), body)
async function execute(id, db, statement, args = []) { return sql(id, db, 'execute', { sql: statement, args }) }
async function query(id, db, statement, args = []) { return sql(id, db, 'query', { sql: statement, args }) }
function statements(text) { return text.split(';').map(s => s.trim()).filter(Boolean) }
function checkDDL(text) { if (/\b(PRIMARY\s+KEY|UNIQUE|CREATE\s+INDEX|CREATE\s+SEQUENCE)\b/i.test(text)) throw new Error('Unsupported DuckLake constraint/index/sequence in migration') }
async function applySQL(id, db, dir, file, marker, isMigration = false) {
  const text = await readFile(join(dir, file), 'utf8')
  if (isMigration) checkDDL(text)
  const old = await query(id, db, 'SELECT name FROM _migrations WHERE name = ?', [marker])
  if (old.rows?.length) return
  const items = statements(text).map(s => ({ sql: s, args: [] }))
  items.push({ sql: 'INSERT INTO _migrations (name, applied_at) VALUES (?, ?)', args: [marker, new Date().toISOString()] })
  const result = await sql(id, db, 'batch', { statements: items, transactional: true })
  if (result.error || result.results?.some(r => r.error_code)) throw new Error(`Migration ${file}: ${JSON.stringify(result.error || result.results)}`)
}
async function databases(id, dir, manifest) {
  const result = {}
  for (const config of manifest.databases || []) {
    const listed = await request('GET', scoped(id, '/databases?limit=200'))
    let db = listed.databases.find(d => d.name === config.name)
    if (!db) db = await request('POST', scoped(id, '/databases'), { name: config.name })
    result[config.name] = db.id
    await execute(id, db.id, 'CREATE TABLE IF NOT EXISTS _migrations (name VARCHAR, applied_at VARCHAR)')
    if (config.migrations) for (const file of (await readdir(join(dir, config.migrations))).filter(f => f.endsWith('.sql')).sort()) await applySQL(id, db.id, join(dir, config.migrations), file, file, true)
    if (config.seed) {
      if (options.resetData) {
        const text = await readFile(join(dir, config.migrations, '001_init.sql'), 'utf8')
        const tables = [...text.matchAll(/CREATE TABLE IF NOT EXISTS\s+([A-Za-z_][A-Za-z_0-9]*)/gi)].map(match => match[1])
        for (const table of tables.reverse()) await execute(id, db.id, `DELETE FROM ${table}`)
        await execute(id, db.id, 'DELETE FROM _migrations WHERE name = ?', [`seed:${config.seed}`])
      }
      await applySQL(id, db.id, dir, config.seed, `seed:${config.seed}`)
    }
  }
  return result
}
async function kv(id, dir, file) {
  if (!file) return
  const commands = JSON.parse(await readFile(join(dir, file), 'utf8'))
  for (const argvs of commands) await request('POST', scoped(id, '/kv'), { type: 'cmd', argvs: argvs.map(String) })
}
async function objects(id, dir, configs = []) {
  for (const config of configs) {
    for (const filename of await readdir(join(dir, config.dir))) {
      const bytes = await readFile(join(dir, config.dir, filename))
      const form = new FormData()
      form.set('key', `${config.prefix || ''}${filename}`)
      form.set('file', new Blob([bytes], { type: filename.endsWith('.csv') ? 'text/csv' : filename.endsWith('.webp') ? 'image/webp' : 'application/octet-stream' }), filename)
      await request('POST', scoped(id, '/s3/objects'), form)
    }
  }
}
async function functions(id, dir, configs = []) {
  for (const config of configs) {
    const source = await readFile(join(dir, config.file), 'utf8')
    const existing = (await request('GET', scoped(id, '/gofunctions'))).functions.find(f => f.name === config.name)
    let version
    if (!existing) {
      const created = await request('POST', scoped(id, '/gofunctions'), { name: config.name, source, activate: false })
      version = created.latest_version
    } else {
      const versions = await request('GET', scoped(id, `/gofunctions/${config.name}/versions`))
      const latest = versions.versions.find(v => v.version === existing.latest_version)
      const saved = latest && await request('GET', scoped(id, `/gofunctions/${config.name}/versions/${latest.version}`))
      if (saved?.source && createHash('sha256').update(saved.source).digest('hex') === createHash('sha256').update(source).digest('hex')) {
        if (existing.active_version) continue
        version = latest.version
      } else {
        const created = await request('POST', scoped(id, `/gofunctions/${config.name}/versions`), { source, activate: false })
        version = created.latest_version
      }
    }
    if (!version) throw new Error(`No version for ${config.name}`)
    const test = await request('POST', scoped(id, `/gofunctions/${config.name}/versions/${version}/test`), config.test)
    if (!test.ok) throw new Error(`Function ${config.name} v${version} test failed: ${test.error}`)
    await request('POST', scoped(id, `/gofunctions/${config.name}/versions/${version}/activate`), {})
  }
}
async function jobs(id, configs = []) {
  for (const config of configs) {
    const listed = await request('GET', scoped(id, '/cron-jobs'))
    const old = listed.jobs.find(j => j.name === config.name)
    const body = { ...config }; delete body.trigger_once
    if (body.schedule_kind === 'once') body.run_at = new Date(Date.now() + 600_000).toISOString()
    const fields = ['schedule_kind', 'cron_expr', 'interval_seconds', 'func_file', 'func_export', 'input_json', 'enabled']
    const changed = old && fields.some(field => (old[field] ?? null) !== (body[field] ?? null))
    const job = !old ? await request('POST', scoped(id, '/cron-jobs'), body) : changed ? await request('PATCH', scoped(id, `/cron-jobs/${old.id}`), body) : old
    if (config.trigger_once && (!old || !(await request('GET', scoped(id, `/cron-jobs/${job.id}/runs?limit=1`))).runs.length)) {
      await request('POST', scoped(id, `/cron-jobs/${job.id}/trigger`), {})
      let done = false
      for (let i = 0; i < 30; i++) {
        await new Promise(r => setTimeout(r, 500))
        const runs = (await request('GET', scoped(id, `/cron-jobs/${job.id}/runs?limit=1`))).runs
        if (runs[0]?.status === 'failed') throw new Error(`Cron ${job.name} failed: ${runs[0].error}`)
        if (runs[0]?.status === 'completed') { done = true; break }
      }
      if (!done) throw new Error(`Cron ${job.name} did not complete`)
    }
  }
}
async function agents(id, configs = []) {
  for (const config of configs) {
    const existing = (await request('GET', scoped(id, '/agents'))).agents.find(a => a.name === config.name)
    const { schedule, ...body } = config
    const agent = existing || await request('POST', scoped(id, '/agents'), body)
    if (schedule) {
      const listed = await request('GET', scoped(id, '/agent-schedules'))
      const old = listed.schedules.find(s => s.agent_id === agent.id)
      if (!old) await request('POST', scoped(id, '/agent-schedules'), { agent_id: agent.id, ...schedule })
      else if (old.cron_expr !== schedule.cron_expr || old.prompt !== schedule.prompt) await request('PATCH', scoped(id, `/agent-schedules/${old.id}`), { agent_id: agent.id, ...schedule })
    }
  }
}
async function keys(id, dir, dbs, configs = []) {
  const envFile = join(dir, '.env')
  let previous = ''
  try { previous = await readFile(envFile, 'utf8') } catch {}
  const ids = [...previous.matchAll(/^export SIMPLEBASE_(?:API_KEY|READONLY_KEY)_ID="([^"]+)"/gm)].map(m => m[1])
  const records = []
  for (const cfg of configs) records.push({ label: cfg.label, ...await request('POST', scoped(id, '/api-keys'), { permissions: cfg.permissions }) })
  const lines = [`export SIMPLEBASE_URL=${JSON.stringify(options.url)}`, `export SIMPLEBASE_PROJECT_ID=${JSON.stringify(id)}`, `export SIMPLEBASE_DATABASE_ID=${JSON.stringify(Object.values(dbs)[0])}`]
  for (const record of records) {
    const name = record.label === 'readonly' ? 'READONLY_KEY' : 'API_KEY'
    lines.push(`export SIMPLEBASE_${name}=${JSON.stringify(record.secret)}`, `export SIMPLEBASE_${name}_ID=${JSON.stringify(record.id)}`)
  }
  await writeFile(envFile, lines.join('\n') + '\n', { mode: 0o600 })
  await chmod(envFile, 0o600)
  for (const old of ids) if (!records.some(r => r.id === old)) await request('DELETE', scoped(id, `/api-keys/${old}`))
  return records
}
async function build(name) {
  if (options.skipBuild) return
  if (name === 'shop' || name === 'iot-telemetry') {
    try { await readFile(join(root, '..', 'packages/js-sdk/dist/index.js')) }
    catch { await runCommand('npm', ['run', 'build'], { cwd: join(root, '..', 'packages/js-sdk'), timeout: 120_000 }) }
  }
  if (name === 'community' || name === 'ops-assistant') await runCommand('go', ['build', `./examples/${name}/...`], { cwd: join(root, '..'), timeout: 120_000 })
}
async function main() {
  if (options.dryRun) {
    for (const name of cases) {
      const manifest = JSON.parse(await readFile(join(root, name, 'simplebase.json'), 'utf8'))
      console.log(`${name}: ${projectID(manifest.project.id)} (${manifest.databases.map(d => d.name).join(', ')})`)
      for (const db of manifest.databases) if (db.migrations) for (const file of (await readdir(join(root, name, db.migrations))).filter(f => f.endsWith('.sql'))) checkDDL(await readFile(join(root, name, db.migrations, file), 'utf8'))
    }
    return
  }
  const ready = await fetch(`${options.url}/health/ready`, { signal: AbortSignal.timeout(5000) })
  if (!ready.ok) throw new Error('SimpleBase not ready; run ./build.sh dev')
  const login = await request('POST', '/v1/auth/login', { username: options.user, password: options.password }, false)
  if (login.user?.role !== 'superadminl1') throw new Error('Superadmin login required')
  access = login.access_token; refresh = login.refresh_token; expires = Date.now() + login.expires_in * 1000
  const created = []
  for (const name of cases) {
    const dir = join(root, name)
    const manifest = JSON.parse(await readFile(join(dir, 'simplebase.json'), 'utf8'))
    const id = projectID(manifest.project.id)
    if (!/^[A-Za-z0-9-]{8}$/.test(id) || ['dev-shop', 'sb-admin'].includes(id)) throw new Error(`Invalid project ID: ${id}`)
    console.log(`[${name}] project ${id}`)
    const listed = await request('GET', '/v1/projects')
    if (!listed.projects.some(p => p.id === id)) await request('POST', '/v1/projects', { id, name: manifest.project.name })
    const dbs = await databases(id, dir, manifest)
    for (const collection of manifest.collections || []) {
      const db = dbs[collection.database]
      if (!db) throw new Error(`Missing collection database ${collection.database}`)
      const path = scoped(id, `/databases/${db}/data/collections`)
      const listedCollections = await request('GET', path)
      if (!listedCollections.collections.includes(collection.name)) await request('POST', path, { name: collection.name })
      for (const document of collection.seed || []) {
        const docs = await request('GET', `${path}/${collection.name}`)
        if (!docs.rows.some(row => row.id === document.id)) await request('POST', `${path}/${collection.name}/documents`, document)
      }
    }
    await kv(id, dir, manifest.kv)
    await objects(id, dir, manifest.objects)
    await functions(id, dir, manifest.functions)
    await jobs(id, manifest.cron_jobs)
    if (manifest.requires?.llm) {
      const providers = await request('GET', scoped(id, '/llm/providers'))
      if (!providers || !Object.values(providers).some(v => Array.isArray(v) && v.length)) console.log(`[${name}] LLM / agent: 未验证（无 provider）`)
      else await agents(id, manifest.agents)
    }
    const records = await keys(id, dir, dbs, manifest.api_keys)
    await build(name)
    const count = await query(id, Object.values(dbs)[0], 'SELECT COUNT(*) FROM _migrations')
    if (!count.rows?.length) throw new Error(`Smoke SQL failed for ${name}`)
    const own = await fetch(`${options.url}${scoped(id, '/s3/objects')}`, { headers: { authorization: `Bearer ${records[0].secret}` } })
    if (!own.ok) throw new Error(`Smoke S3 failed for ${name}: ${own.status}`)
    created.push({ id, name, key: records[0].secret })
    console.log(`[${name}] ready: http://127.0.0.1:5173/?project=${id}`)
  }
  if (created.length > 1) {
    for (const target of created) {
      const source = created.find(item => item.id !== target.id)
      for (const suffix of ['/s3/objects', '/kv', '/gofunctions']) {
        const res = await fetch(`${options.url}${scoped(target.id, suffix)}`, {
          method: suffix === '/kv' ? 'POST' : 'GET',
          headers: { authorization: `Bearer ${source.key}`, 'content-type': 'application/json' },
          body: suffix === '/kv' ? JSON.stringify({ type: 'cmd', argvs: ['GET', 'health'] }) : undefined
        })
        if (res.status !== 403) throw new Error(`Cross-project smoke ${suffix} expected 403, got ${res.status}; restart the server with M1 security fix`)
      }
    }
  }
  console.log('Initialization complete. Runtime keys are in each case .env (mode 600).')
}
main().catch(error => { console.error(`init: ${error.message}`); process.exitCode = 1 })
