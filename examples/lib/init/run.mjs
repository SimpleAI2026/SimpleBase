import { readFile, readdir, writeFile, mkdir, chmod, access as fsAccess } from 'node:fs/promises'
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
const cases = options.only ? options.only.split(',') : ['iot-telemetry', 'shop', 'jd', 'community', 'ops-assistant']
const allowed = new Set(['iot-telemetry', 'shop', 'jd', 'community', 'ops-assistant'])
if (cases.some(name => !allowed.has(name))) throw new Error('Unknown case in --only')
if (!/^[A-Za-z0-9-]{1,3}$/.test(options.prefix)) throw new Error('Invalid --project-prefix (1-3 alphanumeric/hyphen characters)')
const projectID = id => options.prefix === 'ex' ? id : `${options.prefix}-${id.slice(3).padEnd(7 - options.prefix.length, '0').slice(0, 7 - options.prefix.length)}`
const envName = name => name.toUpperCase().replace(/[^A-Z0-9]/g, '_')
// 库、迁移、集合、函数、定时任务由 `simplebase dev apply` 重放（planv6.4 P0b）；本脚本只负责
// 项目创建、Key 签发、kv/对象/助手种子和冒烟。二进制由 ./build.sh 生成在仓库根目录。
const simplebaseBin = process.env.SIMPLEBASE_BIN || join(root, '..', 'simplebase')

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
function checkDDL(text) { if (/\b(PRIMARY\s+KEY|UNIQUE|CREATE\s+INDEX|CREATE\s+SEQUENCE)\b/i.test(text)) throw new Error('Unsupported DuckLake constraint/index/sequence in migration') }

async function databaseIDs(id, manifest) {
  const listed = await request('GET', scoped(id, '/databases?limit=200'))
  const result = {}
  for (const config of manifest.databases || []) {
    const db = listed.databases.find(d => d.name === config.name)
    if (!db) throw new Error(`Database ${config.name} missing after dev apply`)
    result[config.name] = db.id
  }
  return result
}

// --reset-data：清空种子涉及的表和记账行，让随后的 `dev apply --seed` 重新写入种子。
// 表名从该库 migrations 目录下的全部 *.sql 推导（不只 001_init.sql），后续迁移新增的表也会被清空。
async function resetSeeds(id, dir, manifest) {
  if (!options.resetData) return
  const listed = await request('GET', scoped(id, '/databases?limit=200'))
  for (const config of manifest.databases || []) {
    const db = config.seed && listed.databases.find(d => d.name === config.name)
    if (!db || !config.migrations) continue
    const migrationDir = join(dir, config.migrations)
    let files = []
    try { files = (await readdir(migrationDir)).filter(file => file.endsWith('.sql')).sort() } catch { /* 没有迁移目录时只清种子记账 */ }
    const tables = []
    for (const file of files) {
      const text = await readFile(join(migrationDir, file), 'utf8')
      for (const match of text.matchAll(/CREATE TABLE IF NOT EXISTS\s+([A-Za-z_][A-Za-z_0-9]*)/gi)) tables.push(match[1])
    }
    for (const table of [...new Set(tables)].reverse()) {
      // 已有环境可能还没应用最新迁移，缺表时跳过，让接下来的 dev apply 建表。
      try { await execute(id, db.id, `DELETE FROM ${table}`) } catch { /* ignore missing table */ }
    }
    await execute(id, db.id, 'DELETE FROM _migrations WHERE name = ?', [`seed:${config.seed}`])
  }
}

// 调用 `simplebase dev apply`。清单必须自洽：库显式写 data_model，once 任务显式写 run_at，
// 本地脚本不再运行时改写清单，避免它与 skill 重放建出的库类型/任务时间不一致。
async function devApply(id, dir) {
  const existing = (await request('GET', scoped(id, '/cron-jobs'))).jobs || []
  const env = { ...process.env, SIMPLEBASE_URL: options.url, SIMPLEBASE_TOKEN: access, SIMPLEBASE_PROJECT_ID: id }
  delete env.SIMPLEBASE_API_KEY
  let stdout
  try {
    ({ stdout } = await runCommand(simplebaseBin, ['dev', 'apply', '--dir', dir, '--seed', '--yes'], { env, timeout: 300_000, maxBuffer: 16 << 20 }))
  } catch (error) {
    const last = String(error.stderr || '').trim().split('\n').pop()
    let detail = last
    try { detail = JSON.parse(last).error.message } catch {}
    throw new Error(`simplebase dev apply: ${detail || error.message}`)
  }
  const result = JSON.parse(stdout.trim().split('\n').pop()).data
  for (const action of result.actions) console.log(`  ${action.verb.padEnd(7)} ${action.kind.padEnd(10)} ${action.name}${action.detail ? `  (${action.detail})` : ''}`)
  return new Set(existing.map(job => job.name))
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
      // 服务端按内容嗅探 MIME（http.DetectContentType）并与客户端声明比对：CSV 文本会探测成
      // text/plain，声明 text/csv 会被 415 mime_mismatch 拒绝；webp 之外的二进制统一 octet-stream。
      const mime = filename.endsWith('.csv') ? 'text/plain' : filename.endsWith('.webp') ? 'image/webp' : 'application/octet-stream'
      form.set('file', new Blob([bytes], { type: mime }), filename)
      await request('POST', scoped(id, '/s3/objects'), form)
    }
  }
}
// 事件触发器只存在于系统库（不进 dev apply），由脚本按 kind/target/func 三元组幂等对齐：
// 已存在且字段一致就跳过，不一致就 PATCH，缺失就创建。状态另存一份快照给示例 BFF 展示
// （管理面需要 project:admin，运行时 Key 读不到）。
async function functionTriggers(id, dir, configs = []) {
  if (!configs.length) return
  const listed = await request('GET', scoped(id, '/function-triggers'))
  const items = listed.items || []
  const snapshot = []
  for (const config of configs) {
    const key = `${config.kind}|${config.target || ''}|${config.func_file}|${config.func_export || ''}`
    const payload = {
      kind: config.kind,
      target: config.target || '',
      events: config.events || [],
      func_file: config.func_file,
      func_export: config.func_export || 'Handle',
      filter: config.filter || '',
      enabled: config.enabled !== false
    }
    let existing = items.find(item => `${item.kind}|${item.target || ''}|${item.func_file}|${item.func_export || ''}` === key)
    if (!existing) {
      existing = await request('POST', scoped(id, '/function-triggers'), payload)
      console.log(`  create  trigger    ${config.kind}:${config.target || '-'} -> ${config.func_file}`)
    } else {
      // 远端未设置 filter 时返回 null/undefined，统一按空串比较，避免每次都判成 update。
      const same = existing.enabled === payload.enabled && (existing.filter || '') === payload.filter &&
        JSON.stringify(existing.events || []) === JSON.stringify(payload.events)
      if (!same) {
        await request('PATCH', scoped(id, `/function-triggers/${existing.id}`), payload)
        console.log(`  update  trigger    ${config.kind}:${config.target || '-'} -> ${config.func_file}`)
      } else {
        console.log(`  skip    trigger    ${config.kind}:${config.target || '-'} -> ${config.func_file}`)
      }
    }
    snapshot.push({
      id: existing.id,
      kind: config.kind,
      target: config.target || '',
      events: payload.events,
      func_file: payload.func_file,
      func_export: payload.func_export,
      filter: payload.filter,
      enabled: payload.enabled,
      export_source: config.export_source || config.kind
    })
  }
  await mkdir(join(dir, '.simplebase'), { recursive: true })
  await writeFile(join(dir, '.simplebase', 'triggers.json'), JSON.stringify({ triggers: snapshot, dropped: listed.dropped || 0 }, null, 2))
}

// trigger_once 有副作用，不进 dev apply：仅在任务首次创建、且还没有运行记录时触发一次并等待完成。
async function triggerOnce(id, configs = [], existedBefore = new Set()) {
  const listed = (await request('GET', scoped(id, '/cron-jobs'))).jobs || []
  for (const config of configs) {
    if (!config.trigger_once) continue
    const job = listed.find(j => j.name === config.name)
    if (!job) continue
    if (existedBefore.has(config.name) && (await request('GET', scoped(id, `/cron-jobs/${job.id}/runs?limit=1`))).runs.length) continue
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
async function agents(id, configs = []) {
  for (const config of configs) {
    const existing = (await request('GET', scoped(id, '/agents'))).agents.find(a => a.name === config.name)
    const { schedule, ...body } = config
    const agent = existing || await request('POST', scoped(id, '/agents'), body)
    if (!schedule) continue
    // Agent 定时任务已统一到 /cron-jobs（target_kind=agent）；旧 /agent-schedules 写入返回 410。
    const jobs = (await request('GET', scoped(id, '/cron-jobs'))).jobs || []
    const old = jobs.find(job => job.agent_id === agent.id)
    const payload = { name: `${config.name}-schedule`, schedule_kind: 'cron', cron_expr: schedule.cron_expr, target_kind: 'agent', agent_id: agent.id, agent_prompt: schedule.prompt, enabled: true }
    if (!old) await request('POST', scoped(id, '/cron-jobs'), payload)
    else if (old.cron_expr !== schedule.cron_expr || old.agent_prompt !== schedule.prompt) await request('PATCH', scoped(id, `/cron-jobs/${old.id}`), payload)
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
  // 多库站点按清单顺序补 SIMPLEBASE_DATABASE_ID_<NAME>，命名与 `dev apply` 生成的 .simplebase/env.<label> 一致。
  for (const [name, dbID] of Object.entries(dbs)) lines.push(`export SIMPLEBASE_DATABASE_ID_${envName(name)}=${JSON.stringify(dbID)}`)
  for (const record of records) {
    const name = record.label === 'readonly' ? 'READONLY_KEY' : 'API_KEY'
    lines.push(`export SIMPLEBASE_${name}=${JSON.stringify(record.secret)}`, `export SIMPLEBASE_${name}_ID=${JSON.stringify(record.id)}`)
  }
  await writeFile(envFile, lines.join('\n') + '\n', { mode: 0o600 })
  await chmod(envFile, 0o600)
  for (const old of ids) {
    if (records.some(r => r.id === old)) continue
    try {
      await request('DELETE', scoped(id, `/api-keys/${old}`))
    } catch (error) {
      // .env 可能来自另一个实例（项目同 ID 但 Key 不同）；旧 Key 不存在时不必中断初始化。
      if (!String(error.message).includes('HTTP 404')) throw error
    }
  }
  return records
}
async function build(name) {
  if (options.skipBuild) return
  if (name === 'shop' || name === 'iot-telemetry' || name === 'jd') {
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
  try { await fsAccess(simplebaseBin) } catch { throw new Error(`simplebase binary not found at ${simplebaseBin}; run ./build.sh or set SIMPLEBASE_BIN`) }
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
    await resetSeeds(id, dir, manifest)
    const existedJobs = await devApply(id, dir)
    const dbs = await databaseIDs(id, manifest)
    await kv(id, dir, manifest.kv)
    await objects(id, dir, manifest.objects)
    await functionTriggers(id, dir, manifest.function_triggers)
    await triggerOnce(id, manifest.cron_jobs, existedJobs)
    if (manifest.requires?.llm) {
      const models = await request('GET', scoped(id, '/v1/models'))
      if (!models?.data?.length) console.log(`[${name}] LLM / agent: 未验证（无可用模型）`)
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
