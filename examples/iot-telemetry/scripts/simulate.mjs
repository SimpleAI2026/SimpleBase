import { createClient } from '../../../packages/js-sdk/dist/index.js'
const { SIMPLEBASE_URL: url, SIMPLEBASE_API_KEY: apiKey, SIMPLEBASE_PROJECT_ID: projectId, SIMPLEBASE_DATABASE_ID: databaseId } = process.env
if (![url, apiKey, projectId, databaseId].every(Boolean)) throw new Error('Source .env first')
const sb = createClient({ url, apiKey, projectId, databaseId })
const now = new Date().toISOString()
const device = process.argv[2] || 'device-01'
const result = await sb.database(databaseId).sql.batch([{ sql: 'INSERT INTO readings (device_id, ts, metric, value) VALUES (?, ?, ?, ?)', args: [device, now, 'temperature', 20 + Math.random() * 10] }], { transactional: true })
if (result.error) throw new Error(JSON.stringify(result.error))
await sb.raw.request('POST', `/v1/projects/${projectId}/kv`, { body: { type: 'cmd', argvs: ['SET', `online:${device}`, '1', 'EX', '120'] } })
console.log(`${device} ${now}`)
