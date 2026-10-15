import { spawn } from 'node:child_process'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const url = (process.env.SIMPLEBASE_URL || '').replace(/\/$/, '')
const token = process.env.SIMPLEBASE_TOKEN || ''
const project = process.env.SIMPLEBASE_PROJECT_ID || ''
const bin = process.env.SIMPLEBASE_BIN || 'simplebase'
const runID = process.env.RUN_ID || 'demo'
const resultFile = process.env.RESULT_FILE || '/tmp/simplebase-examples/results.jsonl'
if (!url || !token || !project) {
  console.error('agent runner needs SIMPLEBASE_URL, SIMPLEBASE_TOKEN, SIMPLEBASE_PROJECT_ID')
  process.exit(1)
}

const aliases = {
  '数据库': 'database', database: 'database', db: 'database',
  '键值': 'kv', kv: 'kv',
  '对象存储': 's3', s3: 's3', '对象': 's3',
  '云函数': 'gofunction', '函数': 'gofunction', gofunction: 'gofunction',
  '定时任务': 'cron', '定时': 'cron', cron: 'cron',
  '沙盒': 'sandbox', sandbox: 'sandbox',
  '日志': 'logs', logs: 'logs',
  '项目': 'project', project: 'project'
}

function skillsInText(text) {
  const ids = []
  for (const match of text.matchAll(/@([^\s@]+)/g)) {
    const id = aliases[match[1]] || aliases[match[1].toLowerCase()]
    if (id && !ids.includes(id)) ids.push(id)
  }
  return ids
}

function redact(s) {
  return String(s || '')
    .replace(/(secret|token|password|api_key|authorization|refresh_token)("?:?\s*"?)[^\s",}]+/gi, '$1$2[redacted]')
    .replace(/sb_live_[A-Za-z0-9_-]+/g, 'sb_live_[redacted]')
    .replace(/eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/g, '[jwt]')
    .slice(0, 900)
}

async function record(row) {
  const { appendFile } = await import('node:fs/promises')
  await appendFile(resultFile, JSON.stringify(row) + '\n')
  console.log(`${row.ok ? 'PASS' : 'FAIL'} agent/${row.step} (${row.ms}ms)${row.ok ? '' : ' ' + redact(row.detail).slice(0, 180)}`)
}

async function api(method, path, body, timeoutMs = 30000) {
  const res = await fetch(url + path, {
    method,
    headers: {
      authorization: `Bearer ${token}`,
      ...(body === undefined ? {} : { 'content-type': 'application/json' })
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(timeoutMs)
  })
  const text = await res.text()
  let data = null
  try { data = text ? JSON.parse(text) : null } catch { data = text }
  if (!res.ok) {
    const err = new Error(`${method} ${path} HTTP ${res.status} ${redact(typeof data === 'string' ? data : JSON.stringify(data))}`)
    err.status = res.status
    err.data = data
    throw err
  }
  return data
}

function sb(args, timeoutMs = 60000) {
  return new Promise((resolve) => {
    const child = spawn(bin, args, {
      env: {
        ...process.env,
        SIMPLEBASE_URL: url,
        SIMPLEBASE_TOKEN: token,
        SIMPLEBASE_PROJECT_ID: project,
        SIMPLEBASE_OUTPUT: 'json'
      }
    })
    let out = ''
    let err = ''
    const timer = setTimeout(() => child.kill('SIGKILL'), timeoutMs)
    child.stdout.on('data', (b) => { out += b })
    child.stderr.on('data', (b) => { err += b })
    child.on('close', (code) => {
      clearTimeout(timer)
      let data = null
      try { data = JSON.parse(out) } catch { data = null }
      resolve({ code: code ?? 1, data, out, err })
    })
  })
}

function parseSSE(buffer) {
  const events = []
  let rest = buffer
  while (true) {
    const idx = rest.indexOf('\n\n')
    if (idx < 0) break
    const block = rest.slice(0, idx)
    rest = rest.slice(idx + 2)
    const data = block.split('\n').filter((line) => line.startsWith('data:')).map((line) => line.slice(5).trimStart()).join('\n')
    if (!data) continue
    try { events.push(JSON.parse(data)) } catch { /* 心跳或半包之外的噪声 */ }
  }
  return { events, rest }
}

function argvOf(event) {
  if (Array.isArray(event.argv)) return event.argv.map(String)
  if (!event.arguments) return []
  try {
    const parsed = JSON.parse(event.arguments)
    if (Array.isArray(parsed?.argv)) return parsed.argv.map(String)
  } catch { /* 增量参数不是完整 JSON */ }
  return []
}

async function runCase(agentID, spec) {
  const started = Date.now()
  const prompt = spec.prompt.replaceAll('{{RUN_ID}}', runID)
  const skills = skillsInText(prompt)
  const thread = await api('POST', `/v1/projects/${project}/agent-threads`, {
    title: spec.id,
    agent_id: agentID
  })
  const commands = []
  const confirmations = []
  const errors = []
  let assistant = ''
  let reason = ''
  const ac = new AbortController()
  const timer = setTimeout(() => ac.abort(), (spec.timeoutSec || 180) * 1000)
  let runIDHeader = ''
  try {
    const res = await fetch(`${url}/v1/projects/${project}/agent-threads/${thread.id}/runs`, {
      method: 'POST',
      headers: { authorization: `Bearer ${token}`, 'content-type': 'application/json' },
      body: JSON.stringify({ content: prompt, skills, stream: true }),
      signal: ac.signal
    })
    runIDHeader = res.headers.get('x-run-id') || ''
    if (!res.ok || !res.body) {
      const text = await res.text()
      throw new Error(`run HTTP ${res.status} ${redact(text)}`)
    }
    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    let ended = false
    while (!ended) {
      const chunk = await reader.read()
      if (chunk.done) break
      buffer += decoder.decode(chunk.value, { stream: true })
      const parsed = parseSSE(buffer)
      buffer = parsed.rest
      for (const event of parsed.events) {
        if (event.type === 'tool_call' || event.type === 'confirmation_required') {
          const argv = argvOf(event)
          if (argv.length && event.type === 'tool_call' && (event.name === 'simplebase' || !event.name)) commands.push(argv)
          if (event.type === 'confirmation_required') {
            confirmations.push(argv)
            const callID = event.call_id
            const run = event.run_id || runIDHeader
            if (callID && run) {
              await api('POST', `/v1/projects/${project}/agent-runs/${encodeURIComponent(run)}/confirmations/${encodeURIComponent(callID)}`, { approve: true }, 20000)
            }
          }
        }
        if (event.type === 'tool_result' && event.is_error) errors.push(redact(event.content || event.message || ''))
        if (event.type === 'error') errors.push(redact(event.code || event.message || 'error'))
        if (event.type === 'token' && event.content) assistant += event.content
        if (event.type === 'end') {
          reason = event.reason || ''
          ended = true
        }
      }
    }
  } finally {
    clearTimeout(timer)
  }
  const verdict = await verify(spec.verify, { commands, confirmations, errors, assistant, reason })
  const row = {
    suite: 'agent',
    case: 'agent',
    step: spec.id,
    command: prompt.split('\n')[0].slice(0, 180),
    ok: verdict.ok,
    ms: Date.now() - started,
    detail: verdict.detail,
    cli: commands.map((argv) => argv.join(' ')).slice(0, 12),
    confirmations: confirmations.map((argv) => argv.join(' ')),
    skills,
    reason
  }
  await record(row)
  return row.ok
}

async function verify(kind, ctx) {
  const calledPrefix = (...parts) => ctx.commands.some((argv) => parts.every((part, i) => argv[i] === part))
  const confirmed = (...parts) => ctx.confirmations.some((argv) => parts.every((part, i) => argv[i] === part))
  const joinedErrors = ctx.errors.join('\n')
  if (kind === 'readonly') {
    const listed = await sb(['database', 'list', '--limit', '100'])
    const names = listed.data?.data?.databases?.map((db) => db.name) || []
    const created = names.includes(`ro-${runID}`)
    const rejected = joinedErrors.includes('write_not_allowed') || ctx.assistant.includes('write_not_allowed')
    const tried = calledPrefix('database', 'create')
    return {
      ok: tried && rejected && !created && ctx.confirmations.length === 0,
      detail: `tried=${tried} rejected=${rejected} created=${created} confirmations=${ctx.confirmations.length} errors=${joinedErrors.slice(0, 240)}`
    }
  }
  if (kind === 'sql') {
    const listed = await sb(['database', 'list', '--limit', '100'])
    const db = (listed.data?.data?.databases || []).find((item) => item.name === `sql-agent-${runID}`)
    if (!db) return { ok: false, detail: `database missing; cli=${ctx.commands.map((a) => a.slice(0, 3).join(' ')).join(' | ')}` }
    const schema = await sb(['schema', 'show', '--database', db.id])
    const table = (schema.data?.data?.tables || []).find((item) => item.name === 'items')
    const cols = (table?.columns || []).map((col) => col.name)
    const query = await sb(['sql', 'query', '--database', db.id, '--statement', 'SELECT id, title, qty FROM items', '--max-rows', '10'])
    const row = query.data?.data?.rows?.[0]
    const qty = row ? row[2] : undefined
    const used = ctx.commands.some((argv) => argv[0] === 'database' || argv[0] === 'sql' || argv[0] === 'schema')
    const ok = used && cols.includes('id') && cols.includes('qty') && (qty === 3 || qty === '3')
    return { ok, detail: `cols=${cols.join(',')} qty=${qty} createTable=${calledPrefix('schema', 'create-table')} cli=${ctx.commands.length}` }
  }
  if (kind === 'documents') {
    const listed = await sb(['database', 'list', '--limit', '100'])
    const db = (listed.data?.data?.databases || []).find((item) => item.name === `doc-agent-${runID}`)
    if (!db) return { ok: false, detail: 'collection database missing' }
    const docs = await sb(['document', 'list', '--database', db.id, '--collection', 'notes'])
    const left = (docs.data?.data?.rows || []).filter((row) => row.title === 'hello' || row.title === 'world')
    const ok = calledPrefix('document', 'insert') && confirmed('document', 'delete') && left.length === 0 && (docs.data?.ok === true)
    return { ok, detail: `left=${left.length} confirmed=${confirmed('document', 'delete')} listOk=${docs.data?.ok === true}` }
  }
  if (kind === 'kv') {
    const got = await sb(['kv', 'exec', '--command', 'GET', '--arg', `ex:agent:${runID}`])
    const ok = calledPrefix('kv', 'exec') && confirmed('kv', 'exec') && got.data?.ok === true && got.data?.data == null
    return { ok, detail: `get=${JSON.stringify(got.data?.data)} confirmed=${confirmed('kv', 'exec')} code=${got.code}` }
  }
  if (kind === 'function') {
    const invoked = await sb(['function', 'invoke', '--name', `agreet${runID}`, '--export', 'Greet', '--input', '{"name":"simplebase"}'])
    const message = invoked.data?.data?.message
    const ok = calledPrefix('function', 'create') && calledPrefix('function', 'invoke') && message === 'hello simplebase'
    return { ok, detail: `message=${message || ''} invokeCode=${invoked.code} ${redact(invoked.err).slice(0, 180)}` }
  }
  if (kind === 'object') {
    const key = `examples/agent-${runID}/note.txt`
    const listed = await sb(['object', 'list', '--prefix', `examples/agent-${runID}/`])
    const still = Array.isArray(listed.data?.data) && listed.data.data.some((item) => item.key === key)
    const ok = calledPrefix('object', 'upload') && confirmed('object', 'delete') && !still
    return { ok, detail: `still=${still} confirmed=${confirmed('object', 'delete')}` }
  }
  if (kind === 'quota') {
    const ok = calledPrefix('quota', 'get') && !ctx.errors.some((item) => item.includes('skill_not_loaded'))
    return { ok, detail: `called=${calledPrefix('quota', 'get')} errors=${joinedErrors.slice(0, 200)} reason=${ctx.reason}` }
  }
  if (kind === 'cron') {
    const listed = await sb(['cron', 'list'])
    const jobs = listed.data?.data?.jobs || listed.data?.data || []
    const job = (Array.isArray(jobs) ? jobs : []).find((item) => item.name === `tick-agent-${runID}`)
    const ok = Boolean(job) && job.enabled === false && job.func_export === 'Tick' && calledPrefix('cron', 'create')
    return { ok, detail: job ? `id=${job.id} enabled=${job.enabled}` : `job missing code=${listed.code}` }
  }
  return { ok: false, detail: 'unknown verify ' + kind }
}

const cases = JSON.parse(await readFile(join(here, 'cases.json'), 'utf8'))
const only = (process.env.AGENT_ONLY || '').split(',').map((s) => s.trim()).filter(Boolean)
const agents = await api('GET', `/v1/projects/${project}/agents`)
const general = (agents.agents || []).find((agent) => agent.builtin_key === 'general' || agent.name === '通用助手')
if (!general) {
  await record({ suite: 'agent', case: 'agent', step: 'list-agents', command: 'GET /agents', ok: false, ms: 0, detail: 'general agent missing' })
  process.exit(1)
}
let failed = 0
for (const spec of cases) {
  if (only.length && !only.includes(spec.id)) continue
  if (spec.needsWritable) {
    await api('PATCH', `/v1/projects/${project}/agents/${general.id}`, { writable: true })
  }
  try {
    const ok = await runCase(general.id, spec)
    if (!ok) failed += 1
  } catch (err) {
    failed += 1
    await record({
      suite: 'agent', case: 'agent', step: spec.id, command: spec.id,
      ok: false, ms: 0, detail: redact(err.message || err)
    })
  }
}
process.exit(failed ? 1 : 0)
