#!/usr/bin/env node
// scripts/examples-verify/bootstrap.mjs — 验证 harness 的「补位脚本」（planv6.4 §4.1/§4.2）
//
// skill（sbdev.py / simplebase dev）负责：库、迁移、集合、云函数、定时任务的重放。
// 本脚本只负责 skill 边界之外、需要超管 JWT 的动作：
//   1. 超管登录（默认种子账号，可用 SIMPLEBASE_ADMIN_USER/PASSWORD 覆盖；密钥只存在进程内存）；
//   2. 用指定 project id 建项目（本地与云端同 ID，POST /v1/projects {id,name}）；
//   3. 签发发布 Key（database:admin，写 .env / .env.cloud）与运行时 Key（read/write/readonly，
//      写 .env.runtime / .env.cloud.runtime，0600）；重复执行复用已有 Key，不重复签发；
//   4. --seed-data：KV 种子、对象上传、trigger_once（仅无运行记录时触发一次并等 completed）、
//      agents/schedules（requires.llm 且 /v1/models 有数据时；否则 skip 并写原因）；
//   5. --runtime-only：B 实例 apply 之后，把 .simplebase/env.<label> 里的库 ID 写进运行时 env。
//
// 输出（stdout 最后一行）是 JSON 摘要：{ok,target,project,databases,keys,skipped,notes}。
// 绝不打印 Key/JWT/.env 内容；错误只打印 HTTP 状态与 error.code/message。
// 记录 schema 与 lib.sh 一致：{suite,case,env,step,ok,ms,detail,status}。

import { readFile, readdir, writeFile, chmod, access as fsAccess } from 'node:fs/promises'
import { appendFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { parseArgs } from 'node:util'

const startedAt = Date.now()

const USAGE = `用法: node bootstrap.mjs --url <url> --dir <workspace> [选项]

  --url <url>          目标实例（A: http://127.0.0.1:18090；B: http://localhost.:18091）
  --dir <workspace>    工作区副本（verify.sh 会先 cp -R 到 /tmp 下）
  --target A|B         A 写 .env/.env.runtime；B 写 .env.cloud/.env.cloud.runtime（默认 A）
  --project <id>       项目 ID（默认读 simplebase.json；必须恰好 8 位）
  --keys <list>        追加签发的运行时 Key 标签，逗号分隔（publish 恒签发；默认清单 api_keys）
  --seed-data          补 KV/对象/trigger_once/agents 种子（需要库已 apply）
  --runtime-only       只按远端库 ID 重写运行时 env（不签新 Key、不写种子）
  --case <name>        站点名（写记录用）
  --records <jsonl>    结果记录文件（与 verify.sh 共享）
  --user / --password  超管账号（默认种子账号）
  --help               显示本帮助`

const options = {
  url: '',
  dir: '',
  target: 'A',
  project: '',
  keys: '',
  seedData: false,
  runtimeOnly: false,
  case: '',
  records: '',
  user: process.env.SIMPLEBASE_ADMIN_USER || 'simplebase2026',
  password: process.env.SIMPLEBASE_ADMIN_PASSWORD || 'simplebase2026',
  help: false,
}

try {
  const { values } = parseArgs({
    options: {
      url: { type: 'string' },
      dir: { type: 'string' },
      target: { type: 'string' },
      project: { type: 'string' },
      keys: { type: 'string' },
      'seed-data': { type: 'boolean' },
      'runtime-only': { type: 'boolean' },
      case: { type: 'string' },
      records: { type: 'string' },
      user: { type: 'string' },
      password: { type: 'string' },
      help: { type: 'boolean', short: 'h' },
    },
    allowPositionals: false,
  })
  if (values.url !== undefined) options.url = values.url
  if (values.dir !== undefined) options.dir = values.dir
  if (values.target !== undefined) options.target = values.target.toUpperCase()
  if (values.project !== undefined) options.project = values.project
  if (values.keys !== undefined) options.keys = values.keys
  if (values['seed-data'] === true) options.seedData = true
  if (values['runtime-only'] === true) options.runtimeOnly = true
  if (values.case !== undefined) options.case = values.case
  if (values.records !== undefined) options.records = values.records
  if (values.user !== undefined) options.user = values.user
  if (values.password !== undefined) options.password = values.password
  if (values.help === true) options.help = true
} catch (error) {
  fail(`参数错误：${error.message}`)
}

function say(message) {
  process.stderr.write(`[bootstrap] ${message}\n`)
}

let finished = false
function fail(message, code = 1) {
  say(`错误：${message}`)
  if (!finished) {
    recordRow('bootstrap', options.target === 'A' ? 'S2' : 'S9', 'fail', Date.now() - startedAt, message)
    finished = true
  }
  process.exitCode = code
}
let stepStartedAt = Date.now()

// ── 结果记录（与 lib.sh 的 record 同 schema） ─────────────
function recordRow(suite, step, status, ms, detail) {
  if (!options.records) return
  const row = {
    suite,
    case: options.case || '',
    env: options.target,
    step,
    ok: status !== 'fail',
    ms,
    detail: String(detail || '').replace(/sb_live_[A-Za-z0-9_-]+/g, 'sb_live_[redacted]').slice(0, 900),
    status,
  }
  try {
    appendFileSync(options.records, JSON.stringify(row) + '\n')
  } catch {
    // 记录失败不影响主流程。
  }
}

if (options.help) {
  process.stdout.write(USAGE + '\n')
  process.exit(0)
}
if (!options.url || !options.dir) {
  process.stdout.write(USAGE + '\n')
  fail('--url 与 --dir 必填')
}
if (!['A', 'B'].includes(options.target)) fail('--target 只能是 A 或 B')
if (options.seedData && options.runtimeOnly) fail('--seed-data 与 --runtime-only 互斥')
if (options.seedData || options.runtimeOnly) {
  // 这两种模式要和已 apply 的库交互，必须带项目 ID / 工作区。
  if (!options.project) options.project = ''
}

const base = options.url.replace(/\/$/, '')
const dir = resolve(options.dir)
const envBase = options.target === 'A' ? '.env' : '.env.cloud'
const envRuntime = options.target === 'A' ? '.env.runtime' : '.env.cloud.runtime'

// ── 登录态 ───────────────────────────────────────────────
let access = ''
let refresh = ''
let expires = 0

async function request(method, path, body, { auth = true, retry = true } = {}) {
  if (auth && refresh && Date.now() > expires - 30 * 60_000) {
    const pair = await request('POST', '/v1/auth/refresh', { refresh_token: refresh }, { auth: false, retry: false })
    access = pair.access_token
    refresh = pair.refresh_token
    expires = Date.now() + pair.expires_in * 1000
  }
  const headers = auth ? { authorization: `Bearer ${access}` } : {}
  if (body !== undefined && !(body instanceof FormData)) headers['content-type'] = 'application/json'
  let res
  try {
    res = await fetch(`${base}${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : body instanceof FormData ? body : JSON.stringify(body),
      signal: AbortSignal.timeout(60_000),
    })
  } catch (error) {
    throw new Error(`${method} ${path}: ${error.message}`)
  }
  const text = await res.text()
  let data = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = text
  }
  if (!res.ok) {
    const err = data && typeof data === 'object' ? data.error || data : data
    const code = err && typeof err === 'object' ? err.code || '' : ''
    const message = err && typeof err === 'object' ? err.message || '' : String(err || '').slice(0, 200)
    const wrapped = new Error(`${method} ${path}: HTTP ${res.status} ${code ? code + ': ' : ''}${message}`)
    wrapped.status = res.status
    wrapped.code = code
    if (retry && res.status === 401 && refresh) {
      access = ''
      expires = 0
      return request(method, path, body, { auth, retry: false })
    }
    throw wrapped
  }
  return data
}

const scoped = (id, suffix) => `/v1/projects/${encodeURIComponent(id)}${suffix}`
const sqlPath = (id, db, action) => scoped(id, `/databases/${encodeURIComponent(db)}/${action}`)

// ── 清单 / 工作区 ────────────────────────────────────────
async function loadManifest() {
  const text = await readFile(join(dir, 'simplebase.json'), 'utf8')
  return JSON.parse(text)
}

function projectIdOf(manifest) {
  const id = options.project || manifest?.project?.id || ''
  if (!/^[A-Za-z0-9-]{8}$/.test(id)) {
    throw new Error(`项目 ID "${id}" 不是恰好 8 位字母/数字/-（POST /v1/projects 会失败）`)
  }
  if (['dev-shop', 'sb-admin'].includes(id)) throw new Error(`项目 ID ${id} 是保留 ID`)
  return id
}

// ── 项目 ────────────────────────────────────────────────
async function ensureProject(id, manifest) {
  const listed = await request('GET', '/v1/projects')
  const projects = (listed && listed.projects) || []
  const hit = projects.find(project => project.id === id)
  if (hit) {
    say(`项目 ${id} 已存在（engine=${hit.engine || '?'}）`)
    return hit
  }
  const created = await request('POST', '/v1/projects', { id, name: manifest?.project?.name || id })
  say(`创建项目 ${id}（engine=${created.engine || '?'}）`)
  return created
}

// ── Key 签发 ─────────────────────────────────────────────
function parseEnvText(text) {
  const out = {}
  for (const raw of text.split('\n')) {
    let line = raw.trim()
    if (!line || line.startsWith('#')) continue
    if (line.startsWith('export ')) line = line.slice(7).trim()
    const eq = line.indexOf('=')
    if (eq < 0) continue
    const key = line.slice(0, eq).trim()
    let value = line.slice(eq + 1).trim()
    if (value.length >= 2 && value[0] === value[value.length - 1] && (value[0] === '"' || value[0] === "'")) {
      value = value.slice(1, -1)
    }
    out[key] = value
  }
  return out
}

async function readEnvFile(name) {
  try {
    const text = await readFile(join(dir, name), 'utf8')
    return { text, values: parseEnvText(text) }
  } catch {
    return null
  }
}

function labelEnvName(label) {
  return `${String(label).replace(/[^A-Za-z0-9]+/g, '_').toUpperCase()}_KEY`
}

function permissionsFor(label, manifest) {
  const declared = (manifest?.api_keys || []).find(item => item.label === label)
  if (declared && Array.isArray(declared.permissions) && declared.permissions.length) return declared.permissions
  // 发布 Key 必须同时带 read/write：admin 只是建库权限，读写各路由分别校验（HasPermission 精确匹配）。
  if (label === 'publish') return ['database:admin', 'database:read', 'database:write']
  if (label === 'readonly') return ['database:read']
  return ['database:read', 'database:write']
}

// 签发一组 Key；返回 [{label,permissions,id,secret}]。secret 只写文件，不打印。
async function issueKeys(id, labels, manifest) {
  const records = []
  for (const label of labels) {
    const permissions = permissionsFor(label, manifest)
    const created = await request('POST', scoped(id, '/api-keys'), { permissions })
    records.push({ label, permissions, id: created.id, secret: created.secret })
  }
  return records
}

async function revokeStale(id, previousValues, records) {
  const keep = new Set(records.map(record => record.id))
  const stale = []
  for (const [key, value] of Object.entries(previousValues || {})) {
    if (!/_ID$/.test(key)) continue
    if (/API_KEY_ID$|READONLY_KEY_ID$|_KEY_ID$/.test(key) && value && !keep.has(value)) stale.push(value)
  }
  for (const keyId of [...new Set(stale)]) {
    try {
      await request('DELETE', scoped(id, `/api-keys/${encodeURIComponent(keyId)}`))
      say(`吊销旧 Key ${String(keyId).slice(0, 8)}…`)
    } catch (error) {
      say(`旧 Key 吊销失败（忽略）：${error.message}`)
    }
  }
}

// ── 库 ID ───────────────────────────────────────────────
async function databaseIDs(id, manifest) {
  const listed = await request('GET', scoped(id, '/databases?limit=200'))
  const databases = listed.databases || []
  const result = {}
  for (const config of manifest?.databases || []) {
    const hit = databases.find(item => item.name === config.name)
    if (!hit) throw new Error(`apply 之后仍找不到库 ${config.name}`)
    result[config.name] = hit.id
  }
  return result
}

// .simplebase/env.<label>（sbdev / dev apply 生成）作为兜底来源。
async function envHintIDs() {
  const labels = options.target === 'A' ? ['local'] : ['localhost']
  for (const label of labels) {
    const hint = await readEnvFile(join('.simplebase', `env.${label}`))
    if (!hint) continue
    const ids = {}
    for (const [key, value] of Object.entries(hint.values)) {
      const match = key.match(/^SIMPLEBASE_DATABASE_ID(?:_(.+))?$/)
      if (!match) continue
      const name = match[1] ? match[1].toLowerCase() : 'main'
      ids[name] = value
    }
    if (Object.keys(ids).length) return ids
  }
  return null
}

function databaseEnvLines(ids) {
  const lines = []
  const entries = Object.entries(ids)
  // 单库时才写不带后缀的 SIMPLEBASE_DATABASE_ID（多库容易用错，见 sbdev env 提示规则）。
  if (entries.length === 1) lines.push(`export SIMPLEBASE_DATABASE_ID=${JSON.stringify(entries[0][1])}`)
  for (const [name, value] of entries) {
    lines.push(`export SIMPLEBASE_DATABASE_ID_${name.replace(/[^A-Za-z0-9]/g, '_').toUpperCase()}=${JSON.stringify(value)}`)
  }
  return lines
}

async function writeEnvFiles({ publish, runtimeRecords, ids }) {
  const lines = [
    '# examples-verify bootstrap 生成；含密钥，勿提交（0600）。',
    `export SIMPLEBASE_URL=${JSON.stringify(base)}`,
    `export SIMPLEBASE_PROJECT_ID=${JSON.stringify(options.project)}`,
  ]
  if (publish) {
    lines.push(`export SIMPLEBASE_API_KEY=${JSON.stringify(publish.secret)}`)
    lines.push(`export SIMPLEBASE_API_KEY_ID=${JSON.stringify(publish.id)}`)
  }
  if (ids) lines.push(...databaseEnvLines(ids))
  const envPath = join(dir, envBase)
  await writeFile(envPath, lines.join('\n') + '\n', { mode: 0o600 })
  await chmod(envPath, 0o600)
  say(`写入 ${envBase}（0600）`)

  if (runtimeRecords && runtimeRecords.length) {
    const runtimeLines = [
      '# examples-verify bootstrap 生成；含运行时 Key，勿提交（0600）。',
      `export SIMPLEBASE_URL=${JSON.stringify(base)}`,
      `export SIMPLEBASE_PROJECT_ID=${JSON.stringify(options.project)}`,
    ]
    const primary = runtimeRecords.find(record => record.label !== 'readonly') || runtimeRecords[0]
    runtimeLines.push(`export SIMPLEBASE_API_KEY=${JSON.stringify(primary.secret)}`)
    runtimeLines.push(`export SIMPLEBASE_API_KEY_ID=${JSON.stringify(primary.id)}`)
    for (const record of runtimeRecords) {
      const name = labelEnvName(record.label)
      runtimeLines.push(`export SIMPLEBASE_${name}=${JSON.stringify(record.secret)}`)
      runtimeLines.push(`export SIMPLEBASE_${name}_ID=${JSON.stringify(record.id)}`)
      if (record.label === 'readonly') {
        runtimeLines.push(`export SIMPLEBASE_READONLY_KEY=${JSON.stringify(record.secret)}`)
        runtimeLines.push(`export SIMPLEBASE_READONLY_KEY_ID=${JSON.stringify(record.id)}`)
      }
    }
    if (ids) runtimeLines.push(...databaseEnvLines(ids))
    const runtimePath = join(dir, envRuntime)
    await writeFile(runtimePath, runtimeLines.join('\n') + '\n', { mode: 0o600 })
    await chmod(runtimePath, 0o600)
    say(`写入 ${envRuntime}（0600，${runtimeRecords.length} 把运行时 Key）`)
  }
}

// ── 种子（KV / 对象 / trigger_once / agents） ─────────────
async function seedKV(id, manifest) {
  if (!manifest.kv) return 0
  const commands = JSON.parse(await readFile(join(dir, manifest.kv), 'utf8'))
  for (const argvs of commands) {
    await request('POST', scoped(id, '/kv'), { type: 'cmd', argvs: argvs.map(String) })
  }
  return commands.length
}

async function seedObjects(id, manifest) {
  let count = 0
  for (const config of manifest.objects || []) {
    let files
    try {
      files = await readdir(join(dir, config.dir))
    } catch (error) {
      throw new Error(`对象种子目录不可读（${config.dir}）：${error.message}`)
    }
    for (const filename of files) {
      const bytes = await readFile(join(dir, config.dir, filename))
      const form = new FormData()
      form.set('key', `${config.prefix || ''}${filename}`)
      // 一律用 application/octet-stream：/s3/objects 的 sniffUploadMIME 会比对「声明 MIME vs
      // http.DetectContentType vs 扩展名」。SVG/CSV 嗅探出来是 text/plain，声明 image/svg+xml
      // 或 text/csv 会 415 mime_mismatch（run.mjs 的 csv 声明在当前平台同样会失败，见 report
      // 的已知缺陷）；octet-stream 跳过声明比对，扩展名比对允许 text/plain。
      form.set('file', new Blob([bytes], { type: 'application/octet-stream' }), filename)
      await request('POST', scoped(id, '/s3/objects'), form)
      count += 1
    }
  }
  return count
}

// trigger_once 有副作用，不进入重放：只在任务没有运行记录时触发一次并等 completed。
async function triggerOnce(id, manifest) {
  const listed = await request('GET', scoped(id, '/cron-jobs'))
  const jobs = listed.jobs || []
  const triggered = []
  const skipped = []
  for (const config of manifest.cron_jobs || []) {
    if (!config.trigger_once) continue
    const job = jobs.find(item => item.name === config.name)
    if (!job) {
      skipped.push(`${config.name}: 任务不存在`)
      continue
    }
    const runs = (await request('GET', scoped(id, `/cron-jobs/${job.id}/runs?limit=1`))).runs || []
    if (runs.length) {
      skipped.push(`${config.name}: 已有运行记录`)
      continue
    }
    await request('POST', scoped(id, `/cron-jobs/${job.id}/trigger`), {})
    let done = false
    let lastStatus = 'unknown'
    for (let index = 0; index < 40; index += 1) {
      await new Promise(resolvePromise => setTimeout(resolvePromise, 500))
      const list = (await request('GET', scoped(id, `/cron-jobs/${job.id}/runs?limit=1`))).runs || []
      lastStatus = list[0]?.status || 'unknown'
      if (lastStatus === 'failed') throw new Error(`cron ${config.name} 运行失败：${String(list[0].error || '').slice(0, 200)}`)
      if (lastStatus === 'completed') {
        done = true
        break
      }
    }
    if (!done) throw new Error(`cron ${config.name} 未在 20s 内完成（status=${lastStatus}）`)
    triggered.push(config.name)
  }
  return { triggered, skipped }
}

async function seedAgents(id, manifest) {
  if (!manifest.requires?.llm) return { status: 'n/a' }
  const models = await request('GET', scoped(id, '/v1/models'))
  if (!models || !Array.isArray(models.data) || models.data.length === 0) {
    return { status: 'skip', reason: '无可用 LLM provider（/v1/models 为空），助手/定时未创建' }
  }
  for (const config of manifest.agents || []) {
    const existing = ((await request('GET', scoped(id, '/agents'))).agents || []).find(agent => agent.name === config.name)
    const { schedule, ...body } = config
    const agent = existing || await request('POST', scoped(id, '/agents'), body)
    if (schedule) {
      const listed = await request('GET', scoped(id, '/agent-schedules'))
      const old = (listed.schedules || []).find(item => item.agent_id === agent.id)
      if (!old) {
        await request('POST', scoped(id, '/agent-schedules'), { agent_id: agent.id, ...schedule })
      } else if (old.cron_expr !== schedule.cron_expr || old.prompt !== schedule.prompt) {
        await request('PATCH', scoped(id, `/agent-schedules/${old.id}`), { agent_id: agent.id, ...schedule })
      }
    }
  }
  return { status: 'ok', count: (manifest.agents || []).length }
}

async function main() {
  const manifest = await loadManifest()
  const id = projectIdOf(manifest)
  options.project = id
  stepStartedAt = Date.now()

  say(`目标 ${options.target}：${base}`)
  try {
    await fsAccess(join(dir, 'simplebase.json'))
  } catch {
    throw new Error(`工作区 ${dir} 没有 simplebase.json`)
  }
  try {
    const ready = await fetch(`${base}/health/ready`, { signal: AbortSignal.timeout(5000) })
    if (!ready.ok) throw new Error(`HTTP ${ready.status}`)
  } catch (error) {
    throw new Error(`${base}/health/ready 不可用：${error.message}`)
  }

  const login = await request('POST', '/v1/auth/login', { username: options.user, password: options.password }, { auth: false })
  if (login.user?.role !== 'superadminl1') throw new Error(`需要超管账号，实际 role=${login.user?.role || '?'}`)
  access = login.access_token
  refresh = login.refresh_token
  expires = Date.now() + login.expires_in * 1000
  say(`超管登录 ok（role=${login.user.role}，must_change_password=${login.user.must_change_password === true}）`)

  await ensureProject(id, manifest)
  // 跨项目负向用例（ops verify-perms.sh）需要一个真实存在的「另一个项目」，
  // 否则 projectContext 会先 404 而不是 403。用固定 8 位 ID，重复执行幂等。
  if (id !== 'ex-xproj') await ensureProject('ex-xproj', { project: { name: 'cross-project probe' } })

  // 运行时 Key 标签：清单 api_keys + --keys 里除 publish 之外的标签。
  const requested = (options.keys ? options.keys.split(',') : [])
    .map(item => item.trim()).filter(Boolean)
  const runtimeLabels = [...new Set([
    ...(manifest.api_keys || []).map(item => item.label),
    ...requested.filter(label => label !== 'publish'),
  ])]
  const publishRequested = requested.length === 0 || requested.includes('publish')

  const baseEnvFile = await readEnvFile(envBase)
  const runtimeEnvFile = await readEnvFile(envRuntime)
  const skipped = []
  const notes = []

  if (options.runtimeOnly) {
    // 只把库 ID 写进运行时 env：Key 从已有 .env.runtime 复用，绝不重复签发。
    const runtimeValues = runtimeEnvFile?.values || {}
    const runtimeRecords = []
    for (const label of runtimeLabels) {
      const name = labelEnvName(label)
      const secret = runtimeValues[`SIMPLEBASE_${name}`]
      const keyId = runtimeValues[`SIMPLEBASE_${name}_ID`]
      if (secret) runtimeRecords.push({ label, id: keyId || '', secret })
    }
    if (!runtimeRecords.length) throw new Error(`${envRuntime} 里没有可复用的运行时 Key`)
    const ids = await databaseIDs(id, manifest)
    await writeEnvFiles({ publish: null, runtimeRecords, ids })
  } else if (options.seedData) {
    const ids = await databaseIDs(id, manifest)
    // 复用 S2/S9 已签发的运行时 Key；缺失时才补签（例如手工只跑了 apply）。
    const runtimeValues = runtimeEnvFile?.values || {}
    const runtimeRecords = []
    const missingLabels = []
    for (const label of runtimeLabels) {
      const name = labelEnvName(label)
      const secret = runtimeValues[`SIMPLEBASE_${name}`]
      if (secret) runtimeRecords.push({ label, id: runtimeValues[`SIMPLEBASE_${name}_ID`] || '', secret })
      else missingLabels.push(label)
    }
    if (missingLabels.length) {
      const issued = await issueKeys(id, missingLabels, manifest)
      runtimeRecords.push(...issued)
    }
    const publishValues = baseEnvFile?.values || {}
    const publish = publishValues.SIMPLEBASE_API_KEY
      ? { id: publishValues.SIMPLEBASE_API_KEY_ID || '', secret: publishValues.SIMPLEBASE_API_KEY }
      : await issueKeys(id, ['publish'], manifest).then(records => records[0])
    await writeEnvFiles({ publish, runtimeRecords, ids })
    const kvCount = await seedKV(id, manifest)
    const objectCount = await seedObjects(id, manifest)
    const cron = await triggerOnce(id, manifest)
    const agents = await seedAgents(id, manifest)
    const detail = [
      `kv ${kvCount}`,
      `objects ${objectCount}`,
      `trigger [${cron.triggered.join(',') || '无'}]`,
      `trigger-skip [${cron.skipped.join(',') || '无'}]`,
      `agents ${agents.status}${agents.reason ? `: ${agents.reason}` : ''}`,
    ].join('; ')
    if (agents.status === 'skip') {
      recordRow('bootstrap', 'S8-llm', 'skip', Date.now() - stepStartedAt, agents.reason)
      notes.push(agents.reason)
    } else if (agents.status === 'ok') {
      recordRow('bootstrap', 'S8-llm', 'pass', Date.now() - stepStartedAt, `agents ${agents.count} 个已创建`)
    }
    recordRow('bootstrap', 'S8', 'pass', Date.now() - stepStartedAt, detail)
    say(detail)
    process.stdout.write(JSON.stringify({ ok: true, target: options.target, project: id, databases: ids, triggered: cron.triggered, skipped, notes }) + '\n')
    finished = true
    return
  } else {
    // 默认：建项目 + 签发发布 Key（database:admin）与运行时 Key。
    const previous = { ...(baseEnvFile?.values || {}), ...(runtimeEnvFile?.values || {}) }
    const publish = await issueKeys(id, ['publish'], manifest).then(records => records[0])
    const runtimeRecords = await issueKeys(id, runtimeLabels, manifest)
    await revokeStale(id, previous, [publish, ...runtimeRecords])
    let ids = null
    try {
      ids = await databaseIDs(id, manifest)
    } catch {
      // 首次建项目时库还没 apply，库 ID 由 S8/S10 的 --seed-data/--runtime-only 再补。
    }
    if (!ids) {
      ids = await envHintIDs()
      if (ids) notes.push(`${envBase} 的库 ID 来自工作区 .simplebase/env.<label>（apply 之前生成）`)
    }
    await writeEnvFiles({ publish, runtimeRecords, ids })
    const detail = [
      `project ${id}`,
      `keys publish+${runtimeLabels.join('+')}`,
      ids ? `databases ${Object.entries(ids).map(([name, value]) => `${name}=${value}`).join(',')}` : 'databases 待 apply',
      publishRequested ? '' : '（未显式请求 publish）',
    ].filter(Boolean).join('; ')
    recordRow('bootstrap', options.target === 'A' ? 'S2' : 'S9', 'pass', Date.now() - stepStartedAt, detail)
    say(detail)
    process.stdout.write(JSON.stringify({ ok: true, target: options.target, project: id, databases: ids || {}, keys: ['publish', ...runtimeLabels], skipped, notes }) + '\n')
    finished = true
    return
  }
}

main().catch(error => {
  const detail = error.message || String(error)
  if (!finished) {
    recordRow('bootstrap', options.target === 'A' ? 'S2' : 'S9', 'fail', Date.now() - startedAt, detail.slice(0, 500))
    finished = true
  }
  say(`失败：${detail}`)
  process.exitCode = 1
})
