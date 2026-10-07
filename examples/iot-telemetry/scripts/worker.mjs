import { createClient } from '../../../packages/js-sdk/dist/index.js'

const { SIMPLEBASE_URL: url, SIMPLEBASE_API_KEY: apiKey, SIMPLEBASE_PROJECT_ID: projectId, SIMPLEBASE_DATABASE_ID: databaseId } = process.env
if (![url, apiKey, projectId, databaseId].every(Boolean)) throw new Error('Source examples/iot-telemetry/.env first')
const sb = createClient({ url, apiKey, projectId, databaseId })
const sql = sb.database(databaseId).sql
const path = `/v1/projects/${projectId}`
const rows = result => result.rows.map(row => Object.fromEntries(result.columns.map((column, index) => [column, row[index]])))

export async function consume() {
  const { jobs } = await sb.raw.request('GET', `${path}/cron-jobs`)
  const job = jobs.find(j => j.name === 'iot_rollup')
  if (!job) throw new Error('iot_rollup missing; run examples/init.sh')
  const { runs } = await sb.raw.request('GET', `${path}/cron-jobs/${job.id}/runs?limit=100`)
  if (runs.length >= 100 || runs.some(run => run.status === 'completed' && run.response_json?.length >= 4096)) throw new Error('Run backlog or truncated result; manual recovery required')
  for (const run of [...runs].reverse().filter(r => r.status === 'completed')) {
    const seen = await sql.query('SELECT run_id FROM processed_runs WHERE run_id = ?', [run.id], { maxRows: 1 })
    if (seen.rows.length) continue
    const window = JSON.parse(run.response_json || '{}')
    if (!window.start || !window.end) throw new Error(`Invalid window in run ${run.id}`)
    const data = rows(await sql.query('SELECT device_id, metric, value FROM readings WHERE ts >= ? AND ts < ? ORDER BY device_id, metric', [window.start, window.end], { maxRows: 1000 }))
    const groups = new Map()
    for (const row of data) {
      const key = `${row.device_id}\0${row.metric}`
      if (!groups.has(key)) groups.set(key, { device_id: row.device_id, metric: row.metric, values: [] })
      groups.get(key).values.push(Number(row.value))
    }
    const statements = [...groups.values()].map(group => ({
      sql: 'INSERT INTO rollup_5m (device_id, bucket, metric, avg, max, count, source_run_id) SELECT ?, ?, ?, ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM rollup_5m WHERE device_id = ? AND bucket = ? AND metric = ?)',
      args: [group.device_id, window.start, group.metric, group.values.reduce((a, b) => a + b, 0) / group.values.length, Math.max(...group.values), group.values.length, run.id, group.device_id, window.start, group.metric]
    }))
    statements.push({ sql: 'INSERT INTO processed_runs (run_id, processed_at) VALUES (?, ?)', args: [run.id, new Date().toISOString()] })
    const result = await sql.batch(statements, { transactional: true })
    if (result.error || result.results?.some(item => item.error_code)) throw new Error(`Batch failed: ${JSON.stringify(result.error || result.results)}`)
    console.log(`run ${run.id}: ${statements.length - 1} groups`)
  }
}
if (process.argv[1] && import.meta.url === new URL(`file://${process.argv[1]}`).href) consume().catch(error => { console.error(error.message); process.exitCode = 1 })
